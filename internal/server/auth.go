package server

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"
)

const sessionCookie = "labdash_session"

const (
	// maxTrackedFailures bounds memory used by the login rate limiter.
	maxTrackedFailures = 10000
	// failureMemory is how long a client's failed attempts are remembered.
	failureMemory = 15 * time.Minute
)

// Auth implements an optional single-password login with HMAC-signed
// session cookies and a per-IP login rate limit.
type Auth struct {
	Enabled  bool
	password []byte
	secret   []byte
	ttl      time.Duration
	secure   bool
	// TrustedProxies lists reverse proxies whose X-Forwarded-For header is
	// honoured. Empty means the header is ignored.
	TrustedProxies []netip.Prefix

	mu       sync.Mutex
	failures map[string]*failure
}

type failure struct {
	count int
	until time.Time
	last  time.Time
}

func NewAuth(enabled bool, password, secret string, ttl time.Duration, secureCookie bool) *Auth {
	if secret == "" {
		b := make([]byte, 32)
		_, _ = rand.Read(b)
		secret = base64.RawStdEncoding.EncodeToString(b)
	}
	if ttl <= 0 {
		ttl = 30 * 24 * time.Hour
	}
	return &Auth{
		Enabled:  enabled,
		password: []byte(password),
		secret:   []byte(secret),
		ttl:      ttl,
		secure:   secureCookie,
		failures: map[string]*failure{},
	}
}

// token = base64(expiryUnix || nonce) + "." + base64(hmac)
func (a *Auth) issue() string {
	payload := make([]byte, 8+16)
	binary.BigEndian.PutUint64(payload, uint64(time.Now().Add(a.ttl).Unix()))
	_, _ = rand.Read(payload[8:])
	mac := hmac.New(sha256.New, a.secret)
	mac.Write(payload)
	return base64.RawURLEncoding.EncodeToString(payload) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (a *Auth) verify(token string) bool {
	p, sig, ok := strings.Cut(token, ".")
	if !ok {
		return false
	}
	payload, err := base64.RawURLEncoding.DecodeString(p)
	if err != nil || len(payload) != 24 {
		return false
	}
	want, err := base64.RawURLEncoding.DecodeString(sig)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, a.secret)
	mac.Write(payload)
	if !hmac.Equal(mac.Sum(nil), want) {
		return false
	}
	exp := int64(binary.BigEndian.Uint64(payload))
	return time.Now().Unix() < exp
}

// Authenticated reports whether r carries a valid session (or auth is off).
func (a *Auth) Authenticated(r *http.Request) bool {
	if !a.Enabled {
		return true
	}
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return false
	}
	return a.verify(c.Value)
}

// Login checks the password and, on success, sets a session cookie.
func (a *Auth) Login(w http.ResponseWriter, r *http.Request, password string) (ok bool, retryAfter time.Duration) {
	ip := a.ClientIP(r)
	now := time.Now()
	a.mu.Lock()
	f := a.failures[ip]
	if f != nil && now.Before(f.until) {
		a.mu.Unlock()
		return false, f.until.Sub(now)
	}
	a.mu.Unlock()

	if subtle.ConstantTimeCompare([]byte(password), a.password) != 1 || len(a.password) == 0 {
		a.recordFailure(ip, now)
		return false, 0
	}
	a.mu.Lock()
	delete(a.failures, ip)
	a.mu.Unlock()
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    a.issue(),
		Path:     "/",
		HttpOnly: true,
		Secure:   a.secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(a.ttl.Seconds()),
	})
	return true, 0
}

// recordFailure counts a failed login and applies exponential backoff after
// three failures, capped at five minutes.
func (a *Auth) recordFailure(ip string, now time.Time) {
	a.mu.Lock()
	defer a.mu.Unlock()
	f := a.failures[ip]
	if f != nil && now.Sub(f.last) > failureMemory {
		f = nil
	}
	if f == nil {
		if len(a.failures) >= maxTrackedFailures {
			a.pruneFailures(now)
		}
		f = &failure{}
		a.failures[ip] = f
	}
	f.count++
	f.last = now
	if f.count >= 3 {
		d := time.Duration(1<<uint(min(f.count-3, 8))) * time.Second
		f.until = now.Add(min(d, 5*time.Minute))
	}
}

// pruneFailures drops forgotten clients. If the map is still full, it
// evicts the least recently failing client so memory stays bounded.
// Callers must hold a.mu.
func (a *Auth) pruneFailures(now time.Time) {
	var oldestIP string
	var oldest time.Time
	for ip, f := range a.failures {
		if now.Sub(f.last) > failureMemory && now.After(f.until) {
			delete(a.failures, ip)
			continue
		}
		if oldestIP == "" || f.last.Before(oldest) {
			oldestIP, oldest = ip, f.last
		}
	}
	if len(a.failures) >= maxTrackedFailures {
		delete(a.failures, oldestIP)
	}
}

func (a *Auth) Logout(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: "", Path: "/", HttpOnly: true, Secure: a.secure,
		SameSite: http.SameSiteLaxMode, MaxAge: -1,
	})
}

// ClientIP returns the address used for rate limiting and logs. The
// X-Forwarded-For header is only trusted when the direct peer is a configured
// proxy; otherwise any client could spoof it to evade the login limit.
func (a *Auth) ClientIP(r *http.Request) string {
	peer := remoteAddr(r)
	if len(a.TrustedProxies) == 0 || !a.trusted(peer) {
		return peer.String()
	}
	// Walk the chain from the nearest hop and return the first address not
	// belonging to a trusted proxy. Entries left of it are client-controlled.
	hops := strings.Split(strings.Join(r.Header.Values("X-Forwarded-For"), ","), ",")
	for i := len(hops) - 1; i >= 0; i-- {
		addr, err := netip.ParseAddr(strings.TrimSpace(hops[i]))
		if err != nil {
			break
		}
		addr = addr.Unmap()
		if !a.trusted(addr) {
			return addr.String()
		}
		peer = addr
	}
	return peer.String()
}

func (a *Auth) trusted(addr netip.Addr) bool {
	for _, p := range a.TrustedProxies {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

func remoteAddr(r *http.Request) netip.Addr {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return netip.IPv6Unspecified()
	}
	return addr.Unmap()
}

// ParseTrustedProxies parses a comma-separated list of IP addresses and CIDR
// prefixes, as used by LABDASH_TRUSTED_PROXIES.
func ParseTrustedProxies(list string) ([]netip.Prefix, error) {
	var out []netip.Prefix
	for _, item := range strings.Split(list, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if strings.Contains(item, "/") {
			p, err := netip.ParsePrefix(item)
			if err != nil {
				return nil, fmt.Errorf("invalid trusted proxy %q: %w", item, err)
			}
			out = append(out, p.Masked())
			continue
		}
		addr, err := netip.ParseAddr(item)
		if err != nil {
			return nil, fmt.Errorf("invalid trusted proxy %q: %w", item, err)
		}
		addr = addr.Unmap()
		out = append(out, netip.PrefixFrom(addr, addr.BitLen()))
	}
	return out, nil
}
