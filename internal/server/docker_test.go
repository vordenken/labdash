package server

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vordenken/labdash/internal/providers/docker"
)

// fakeDocker serves the Docker API subset labdash uses on a unix socket.
func fakeDocker(t *testing.T) (*docker.Client, *docker.Watcher) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /containers/json", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{{
			"Id": "0123456789ab", "Names": []string{"/media"}, "Image": "media:1", "State": "running",
			"Status": "Up 1 hour", "Labels": map[string]string{"labdash.name": "Media", "labdash.widget.key": "secret"},
		}})
	})
	mux.HandleFunc("POST /containers/{id}/{action}", func(w http.ResponseWriter, r *http.Request) {
		if r.PathValue("action") == "stop" {
			http.Error(w, `{"message":"nope"}`, http.StatusForbidden)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /events", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	})
	// macOS limits unix socket paths to 104 bytes; t.TempDir() is too long.
	dir, err := os.MkdirTemp("/tmp", "lds")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	sock := filepath.Join(dir, "d.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewUnstartedServer(mux)
	srv.Listener = ln
	srv.Start()
	t.Cleanup(srv.Close)

	client := docker.NewClient(sock)
	watcher := docker.NewWatcher(client, time.Hour, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go watcher.Run(ctx)
	deadline := time.Now().Add(5 * time.Second)
	for len(watcher.Snapshot().Containers) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("watcher did not load containers")
		}
		time.Sleep(10 * time.Millisecond)
	}
	return client, watcher
}

func TestDockerEndpoints(t *testing.T) {
	client, watcher := fakeDocker(t)
	auth := NewAuth(true, "pw", "", 0, true)
	s := newTestServer(t, auth, Options{Docker: watcher, DockerClient: client, AllowContainerActions: true})
	call := func(method, path string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, nil)
		r.AddCookie(&http.Cookie{Name: sessionCookie, Value: auth.issue()})
		return serve(s, r)
	}
	w := call(http.MethodGet, "/api/docker/containers")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"media"`) || strings.Contains(w.Body.String(), "secret") {
		t.Fatalf("containers must be listed without labels: %d %s", w.Code, w.Body)
	}
	if body := call(http.MethodGet, "/api/layout").Body.String(); !strings.Contains(body, `"Media"`) {
		t.Fatalf("discovered tile missing from layout: %s", body)
	}
	for path, want := range map[string]int{
		"/api/docker/media/restart": http.StatusOK,
		"/api/docker/media/stop":    http.StatusForbidden,
		"/api/docker/media/kill":    http.StatusBadGateway,
		"/api/docker/ghost/restart": http.StatusNotFound,
	} {
		if w := call(http.MethodPost, path); w.Code != want {
			t.Errorf("%s: got %d, want %d: %s", path, w.Code, want, w.Body)
		}
	}

	readOnly := newTestServer(t, auth, Options{Docker: watcher, DockerClient: client})
	r := httptest.NewRequest(http.MethodPost, "/api/docker/media/restart", nil)
	r.AddCookie(&http.Cookie{Name: sessionCookie, Value: auth.issue()})
	if w := serve(readOnly, r); w.Code != http.StatusForbidden {
		t.Fatalf("actions must be off by default: %d", w.Code)
	}

	disabled := newTestServer(t, auth, Options{})
	for _, req := range []*http.Request{
		httptest.NewRequest(http.MethodGet, "/api/docker/containers", nil),
		httptest.NewRequest(http.MethodPost, "/api/docker/media/restart", nil),
	} {
		req.AddCookie(&http.Cookie{Name: sessionCookie, Value: auth.issue()})
		if w := serve(disabled, req); w.Code != http.StatusNotFound {
			t.Errorf("%s without Docker: got %d", req.URL.Path, w.Code)
		}
	}
}

func TestStreamSendsLayoutAndDockerState(t *testing.T) {
	client, watcher := fakeDocker(t)
	s := newTestServer(t, NewAuth(false, "", "", 0, true), Options{Docker: watcher, DockerClient: client})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx)
	srv := httptest.NewServer(s)
	defer srv.Close()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/api/stream", nil)
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	seen := map[string]bool{}
	sc := bufio.NewScanner(res.Body)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	watcher.Refresh()
	for sc.Scan() && !(seen["layout"] && seen["docker"]) {
		if name, ok := strings.CutPrefix(sc.Text(), "event: "); ok {
			seen[name] = true
		}
	}
	if !seen["layout"] || !seen["docker"] {
		t.Fatalf("stream events: %v", seen)
	}
}
