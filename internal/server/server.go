// Package server exposes the dashboard UI and JSON API.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/vordenken/labdash/internal/config"
	"github.com/vordenken/labdash/internal/discovery"
	"github.com/vordenken/labdash/internal/providers/docker"
)

// Options configures a Server.
type Options struct {
	Title                 string
	Auth                  *Auth
	AllowContainerActions bool
	Store                 *config.Store
	Docker                *docker.Watcher // may be nil when Docker is disabled
	DockerClient          *docker.Client
	Static                fs.FS
	Log                   *slog.Logger
	Version               string
	// Icons serves locally provided tile images under /icons/. May be nil.
	Icons fs.FS
	// IconCache holds icons downloaded earlier. May be nil.
	IconCache fs.FS
	// IconDownloader fetches missing icons into IconCache. Nil disables
	// downloads.
	IconDownloader *IconDownloader
	// ContainerLink is a URL template that opens a container in an external
	// management tool such as Arcane. Empty disables the link.
	ContainerLink string
}

// contentSecurityPolicy keeps the browser on this origin: the browser never
// contacts a third party (icons are downloaded by the server), so scripts,
// styles, images and connections are all limited to 'self'.
const contentSecurityPolicy = "default-src 'self'; img-src 'self'; style-src 'self'; script-src 'self'; " +
	"connect-src 'self'; object-src 'none'; base-uri 'none'; form-action 'self'; frame-ancestors 'self'"

type Server struct {
	opts    Options
	hub     *Hub
	mux     *http.ServeMux
	handler http.Handler
}

func New(opts Options) *Server {
	if opts.Log == nil {
		opts.Log = slog.Default()
	}
	s := &Server{opts: opts, hub: NewHub(), mux: http.NewServeMux()}
	s.routes()
	// Rejects state-changing browser requests from other origins (CSRF),
	// including sibling subdomains that SameSite=Lax cookies do not cover.
	cop := http.NewCrossOriginProtection()
	cop.SetDenyHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.opts.Log.Warn("cross-origin request rejected", "method", r.Method, "path", r.URL.Path,
			"origin", r.Header.Get("Origin"), "ip", s.opts.Auth.ClientIP(r))
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "cross-origin request rejected"})
	}))
	s.handler = cop.Handler(s.mux)
	return s
}

// Run starts background fan-out (Docker snapshots → SSE) until ctx ends.
func (s *Server) Run(ctx context.Context) {
	if s.opts.Docker == nil {
		return
	}
	ch, unsub := s.opts.Docker.Subscribe()
	defer unsub()
	for {
		select {
		case <-ctx.Done():
			return
		case snap := <-ch:
			s.hub.Broadcast("docker", publicSnapshot(snap))
			s.hub.Broadcast("layout", s.layout())
		}
	}
}

func (s *Server) routes() {
	m := s.mux
	m.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "version": s.opts.Version})
	})
	m.HandleFunc("GET /api/session", s.handleSession)
	m.HandleFunc("POST /api/auth/login", s.handleLogin)
	m.HandleFunc("POST /api/auth/logout", s.handleLogout)

	m.Handle("GET /api/layout", s.protect(http.HandlerFunc(s.handleLayout)))
	m.Handle("GET /api/config", s.protect(http.HandlerFunc(s.handleGetConfig)))
	m.Handle("PUT /api/config", s.protect(http.HandlerFunc(s.handlePutConfig)))
	m.Handle("PUT /api/setup", s.protect(http.HandlerFunc(s.handleSetup)))
	m.Handle("GET /api/stream", s.protect(http.HandlerFunc(s.handleStream)))
	m.Handle("GET /api/docker/containers", s.protect(http.HandlerFunc(s.handleContainers)))
	m.Handle("POST /api/docker/{name}/{action}", s.protect(http.HandlerFunc(s.handleContainerAction)))
	m.Handle("GET /icons/{name}", s.protect(http.HandlerFunc(s.handleIcon)))

	m.Handle("/", http.FileServerFS(s.opts.Static))
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "SAMEORIGIN")
	w.Header().Set("Referrer-Policy", "same-origin")
	w.Header().Set("Content-Security-Policy", contentSecurityPolicy)
	s.handler.ServeHTTP(w, r)
}

func (s *Server) protect(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.opts.Auth.Authenticated(r) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// --- session & auth -------------------------------------------------------

type sessionInfo struct {
	Title           string `json:"title"`
	Version         string `json:"version"`
	AuthRequired    bool   `json:"authRequired"`
	Authenticated   bool   `json:"authenticated"`
	ContainerAction bool   `json:"containerActions"`
	DockerEnabled   bool   `json:"dockerEnabled"`
	// ContainerLink is only sent to signed-in users because it reveals the
	// address of the management tool.
	ContainerLink string `json:"containerLink,omitempty"`
}

func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	info := sessionInfo{
		Title:           s.opts.Title,
		Version:         s.opts.Version,
		AuthRequired:    s.opts.Auth.Enabled,
		Authenticated:   s.opts.Auth.Authenticated(r),
		ContainerAction: s.opts.AllowContainerActions && s.opts.Auth.Enabled && s.opts.Docker != nil,
		DockerEnabled:   s.opts.Docker != nil,
	}
	if info.Authenticated && info.DockerEnabled {
		info.ContainerLink = s.opts.ContainerLink
	}
	writeJSON(w, http.StatusOK, info)
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if !s.opts.Auth.Enabled {
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	var body struct {
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &body, 4096); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad request"})
		return
	}
	ok, retry := s.opts.Auth.Login(w, r, body.Password)
	switch {
	case retry > 0:
		w.Header().Set("Retry-After", strconv.Itoa(int(retry.Seconds())+1))
		s.opts.Log.Warn("login rate limited", "ip", s.opts.Auth.ClientIP(r))
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "too many attempts"})
	case !ok:
		s.opts.Log.Warn("login failed", "ip", s.opts.Auth.ClientIP(r))
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid password"})
	default:
		s.opts.Log.Info("login ok", "ip", s.opts.Auth.ClientIP(r))
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}
}

func (s *Server) handleLogout(w http.ResponseWriter, _ *http.Request) {
	s.opts.Auth.Logout(w)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// --- layout & config ------------------------------------------------------

type layoutResponse struct {
	Title      string            `json:"title"`
	Theme      string            `json:"theme"`
	Appearance config.Appearance `json:"appearance"`
	Pages      []config.Page     `json:"pages"`
	NeedsSetup bool              `json:"needsSetup"`
}

func (s *Server) layout() layoutResponse {
	cfg := s.opts.Store.Get()
	pages := cfg.Pages
	if s.opts.Docker != nil {
		pages = discovery.Merge(cfg, s.opts.Docker.Snapshot())
	}
	// Tile params are passed through unchanged; no built-in widget takes
	// credentials.
	return layoutResponse{Title: cfg.Title, Theme: cfg.Theme, Appearance: cfg.Appearance, Pages: pages, NeedsSetup: cfg.NeedsSetup()}
}

func (s *Server) handleLayout(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.layout())
}

func (s *Server) handleGetConfig(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.opts.Store.Get())
}

func (s *Server) handlePutConfig(w http.ResponseWriter, r *http.Request) {
	s.saveConfig(w, r, s.opts.Store.Set)
}

func (s *Server) handleSetup(w http.ResponseWriter, r *http.Request) {
	s.saveConfig(w, r, s.opts.Store.CompleteSetup)
}

func (s *Server) saveConfig(w http.ResponseWriter, r *http.Request, save func(config.Config) error) {
	var cfg config.Config
	if err := decodeJSON(r, &cfg, 2<<20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json: " + err.Error()})
		return
	}
	if err := save(cfg); errors.Is(err, config.ErrSetupUnavailable) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	} else if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	s.opts.Log.Info("config saved", "ip", s.opts.Auth.ClientIP(r))
	layout := s.layout()
	s.hub.Broadcast("layout", layout)
	writeJSON(w, http.StatusOK, layout)
}

// --- stream ---------------------------------------------------------------

func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	initial := []event{mustEvent("layout", s.layout())}
	if s.opts.Docker != nil {
		initial = append(initial, mustEvent("docker", publicSnapshot(s.opts.Docker.Snapshot())))
	}
	s.hub.ServeHTTP(w, r, initial)
}

func mustEvent(name string, v any) event {
	data, _ := json.Marshal(v)
	return event{name: name, data: data}
}

// --- docker ---------------------------------------------------------------

// publicContainer omits labels (which may carry widget secrets).
type publicContainer struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Image   string `json:"image"`
	State   string `json:"state"`
	Status  string `json:"status"`
	Health  string `json:"health"`
	Project string `json:"project"`
	Sablier bool   `json:"sablier"`
	Created int64  `json:"created"`
}

type publicSnap struct {
	Taken      time.Time                  `json:"taken"`
	Error      string                     `json:"error,omitempty"`
	Containers map[string]publicContainer `json:"containers"`
}

func publicSnapshot(snap docker.Snapshot) publicSnap {
	out := publicSnap{Taken: snap.Taken, Error: snap.Err, Containers: make(map[string]publicContainer, len(snap.Containers))}
	for name, c := range snap.Containers {
		out.Containers[name] = publicContainer{
			ID: c.ID, Name: c.Name, Image: c.Image, State: c.State, Status: c.Status,
			Health: c.Health, Project: c.Project, Sablier: c.Sablier, Created: c.Created,
		}
	}
	return out
}

func (s *Server) handleContainers(w http.ResponseWriter, _ *http.Request) {
	if s.opts.Docker == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "docker disabled"})
		return
	}
	writeJSON(w, http.StatusOK, publicSnapshot(s.opts.Docker.Snapshot()))
}

func (s *Server) handleContainerAction(w http.ResponseWriter, r *http.Request) {
	if s.opts.Docker == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "docker disabled"})
		return
	}
	if !s.opts.AllowContainerActions || !s.opts.Auth.Enabled {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "container actions disabled"})
		return
	}
	name := r.PathValue("name")
	action := r.PathValue("action")
	c, ok := s.opts.Docker.Snapshot().Containers[name]
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "unknown container"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	err := s.opts.DockerClient.Action(ctx, c.ID, action)
	s.opts.Log.Info("container action", "container", name, "action", action, "ip", s.opts.Auth.ClientIP(r), "err", err)
	if err != nil {
		status := http.StatusBadGateway
		var ae *docker.APIError
		if errors.As(err, &ae) {
			status = ae.Status
		}
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	s.opts.Docker.Refresh()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// --- helpers --------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func decodeJSON(r *http.Request, v any, limit int64) error {
	if ct := r.Header.Get("Content-Type"); ct != "" && !strings.HasPrefix(ct, "application/json") {
		return errors.New("expected application/json")
	}
	dec := json.NewDecoder(io.LimitReader(r.Body, limit))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}
