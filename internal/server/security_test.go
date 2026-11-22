package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/vordenken/labdash/internal/config"
)

func newTestServer(t *testing.T, auth *Auth, opts Options) *Server {
	t.Helper()
	store, err := config.Open(filepath.Join(t.TempDir(), "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	opts.Store = store
	opts.Auth = auth
	opts.Log = slog.New(slog.NewTextHandler(io.Discard, nil))
	return New(opts)
}

func serve(s *Server, r *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	return w
}

func TestClientIPOnlyTrustsConfiguredProxies(t *testing.T) {
	proxies, err := ParseTrustedProxies(" 172.18.0.0/16 , 10.0.0.1,::ffff:192.168.1.1 ")
	if err != nil {
		t.Fatal(err)
	}
	direct := NewAuth(true, "pw", "", 0, true)
	behindProxy := NewAuth(true, "pw", "", 0, true)
	behindProxy.TrustedProxies = proxies
	cases := []struct {
		name   string
		auth   *Auth
		remote string
		xff    []string
		want   string
	}{
		{"header ignored without trusted proxies", direct, "203.0.113.9:1234", []string{"198.51.100.1"}, "203.0.113.9"},
		{"header ignored from untrusted peer", behindProxy, "203.0.113.9:1234", []string{"198.51.100.1"}, "203.0.113.9"},
		{"rightmost untrusted hop wins", behindProxy, "172.18.0.5:80", []string{"6.6.6.6, 198.51.100.1", "10.0.0.1"}, "198.51.100.1"},
		{"only proxies in chain", behindProxy, "172.18.0.5:80", []string{"10.0.0.1"}, "10.0.0.1"},
		{"garbage stops the walk", behindProxy, "172.18.0.5:80", []string{"not-an-ip"}, "172.18.0.5"},
		{"ipv4-mapped peer is unmapped", behindProxy, "[::ffff:192.168.1.1]:80", []string{"198.51.100.7"}, "198.51.100.7"},
		{"unparseable remote address", direct, "pipe", nil, "::"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.RemoteAddr = tc.remote
			for _, v := range tc.xff {
				r.Header.Add("X-Forwarded-For", v)
			}
			if got := tc.auth.ClientIP(r); got != tc.want {
				t.Fatalf("ClientIP = %q, want %q", got, tc.want)
			}
		})
	}
	for _, bad := range []string{"10.0.0.0/33", "proxy.local"} {
		if _, err := ParseTrustedProxies(bad); err == nil {
			t.Errorf("ParseTrustedProxies(%q) should fail", bad)
		}
	}
}

func TestLoginThrottleCannotBeBypassedWithForwardedFor(t *testing.T) {
	s := newTestServer(t, NewAuth(true, "right", "", 0, true), Options{})
	login := func(password, xff string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"password":"`+password+`"}`))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Forwarded-For", xff)
		return serve(s, r)
	}
	for i := range 3 {
		if w := login("wrong", fmt.Sprintf("198.51.100.%d", i)); w.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: got %d", i, w.Code)
		}
	}
	w := login("right", "198.51.100.200")
	if w.Code != http.StatusTooManyRequests || w.Header().Get("Retry-After") == "" {
		t.Fatalf("spoofed header must not reset the limit: %d %v", w.Code, w.Header())
	}
}

func TestLoginSucceedsAndLogoutClearsCookie(t *testing.T) {
	auth := NewAuth(true, "right", "", 0, true)
	s := newTestServer(t, auth, Options{})
	r := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"password":"right"}`))
	w := serve(s, r)
	if w.Code != http.StatusOK || len(w.Result().Cookies()) != 1 {
		t.Fatalf("login failed: %d", w.Code)
	}
	cookie := w.Result().Cookies()[0]
	if !cookie.HttpOnly || !cookie.Secure {
		t.Fatalf("insecure session cookie: %+v", cookie)
	}
	r = httptest.NewRequest(http.MethodGet, "/api/session", nil)
	r.AddCookie(cookie)
	if body := serve(s, r).Body.String(); !strings.Contains(body, `"authenticated":true`) {
		t.Fatalf("session not recognised: %s", body)
	}
	if w := serve(s, httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{`))); w.Code != http.StatusBadRequest {
		t.Fatalf("malformed login: %d", w.Code)
	}
	w = serve(s, httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil))
	if c := w.Result().Cookies(); len(c) != 1 || c[0].MaxAge >= 0 {
		t.Fatalf("logout must expire the cookie: %+v", c)
	}
	open := newTestServer(t, NewAuth(false, "", "", 0, true), Options{})
	if w := serve(open, httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)); w.Code != http.StatusOK {
		t.Fatalf("login without auth: %d", w.Code)
	}
}

func TestFailureTrackingIsBounded(t *testing.T) {
	a := NewAuth(true, "pw", "", 0, true)
	now := time.Now()
	for i := range maxTrackedFailures {
		a.failures[netip.AddrFrom4([4]byte{10, byte(i >> 16), byte(i >> 8), byte(i)}).String()] = &failure{count: 1, last: now.Add(-time.Duration(i) * time.Second)}
	}
	a.failures["stale"] = &failure{count: 1, last: now.Add(-time.Hour)}
	a.recordFailure("203.0.113.1", now)
	if len(a.failures) > maxTrackedFailures {
		t.Fatalf("failure map grew to %d", len(a.failures))
	}
	if _, ok := a.failures["stale"]; ok {
		t.Fatal("stale entry must be pruned")
	}
	// A full map of recent failures evicts the oldest entry.
	a.recordFailure("203.0.113.2", now)
	if len(a.failures) > maxTrackedFailures || a.failures["203.0.113.2"] == nil {
		t.Fatalf("eviction failed: %d entries", len(a.failures))
	}
	// Failures older than failureMemory no longer count towards the backoff.
	a.failures["203.0.113.1"].count = 5
	a.failures["203.0.113.1"].last = now.Add(-2 * failureMemory)
	a.recordFailure("203.0.113.1", now)
	if f := a.failures["203.0.113.1"]; f.count != 1 || !f.until.IsZero() {
		t.Fatalf("old failures must be forgotten: %+v", f)
	}
}

func TestCrossOriginWritesAreRejected(t *testing.T) {
	auth := NewAuth(true, "pw", "", 0, true)
	s := newTestServer(t, auth, Options{})
	body, _ := json.Marshal(s.opts.Store.Get())
	put := func(headers map[string]string) int {
		r := httptest.NewRequest(http.MethodPut, "http://dash.example.org/api/config", bytes.NewReader(body))
		r.AddCookie(&http.Cookie{Name: sessionCookie, Value: auth.issue()})
		for k, v := range headers {
			r.Header.Set(k, v)
		}
		return serve(s, r).Code
	}
	if code := put(map[string]string{"Sec-Fetch-Site": "same-site", "Origin": "https://evil.example.org"}); code != http.StatusForbidden {
		t.Fatalf("sibling subdomain write: got %d", code)
	}
	if code := put(map[string]string{"Origin": "https://evil.example.org"}); code != http.StatusForbidden {
		t.Fatalf("foreign origin write: got %d", code)
	}
	if code := put(map[string]string{"Sec-Fetch-Site": "same-origin"}); code != http.StatusOK {
		t.Fatalf("same-origin write: got %d", code)
	}
	if code := put(nil); code != http.StatusOK {
		t.Fatalf("non-browser client write: got %d", code)
	}
}

func TestSecurityHeaders(t *testing.T) {
	s := newTestServer(t, NewAuth(false, "", "", 0, true), Options{Version: "1.2.3"})
	w := serve(s, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	csp := w.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "default-src 'self'") || strings.Contains(csp, "http") {
		t.Fatalf("CSP must only allow this origin: %q", csp)
	}
	if !strings.Contains(w.Body.String(), `"version":"1.2.3"`) {
		t.Fatalf("healthz: %s", w.Body)
	}
}

func TestIconsAreServedLocallyAndSafely(t *testing.T) {
	icons := fstest.MapFS{
		"jellyfin.png":  {Data: []byte("png"), ModTime: time.Unix(1, 0)},
		"jellyfin.svg":  {Data: []byte("<svg/>")},
		"mdi-home.svg":  {Data: []byte("<svg/>")},
		"huge.png":      {Data: make([]byte, maxIconSize+1)},
		"folder.png/x":  {Data: []byte("x")},
		"notes.txt":     {Data: []byte("secret")},
		"notes.txt.svg": {Data: []byte("<svg/>")},
	}
	auth := NewAuth(true, "pw", "", 0, true)
	s := newTestServer(t, auth, Options{Icons: icons})
	get := func(name string, authenticated bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, "/icons/"+name, nil)
		if authenticated {
			r.AddCookie(&http.Cookie{Name: sessionCookie, Value: auth.issue()})
		}
		return serve(s, r)
	}
	for name, want := range map[string]string{
		"jellyfin":     "image/svg+xml",
		"jellyfin.png": "image/png",
		"mdi-home":     "image/svg+xml",
		"notes.txt":    "image/svg+xml",
	} {
		w := get(name, true)
		if w.Code != http.StatusOK || w.Header().Get("Content-Type") != want {
			t.Errorf("%s: got %d %q", name, w.Code, w.Header().Get("Content-Type"))
		}
		if !strings.Contains(w.Header().Get("Content-Security-Policy"), "sandbox") {
			t.Errorf("%s: icons need a sandboxing CSP", name)
		}
	}
	for _, name := range []string{"missing", "huge.png", "folder.png", "..%2Fconfig.json", ".hidden"} {
		if w := get(name, true); w.Code != http.StatusNotFound {
			t.Errorf("%s: got %d, want 404", name, w.Code)
		}
	}
	if w := get("jellyfin", false); w.Code != http.StatusUnauthorized {
		t.Errorf("icons require a session: got %d", w.Code)
	}
	none := newTestServer(t, NewAuth(false, "", "", 0, true), Options{})
	if w := serve(none, httptest.NewRequest(http.MethodGet, "/icons/jellyfin", nil)); w.Code != http.StatusNotFound {
		t.Errorf("without an icon directory: got %d", w.Code)
	}
}
