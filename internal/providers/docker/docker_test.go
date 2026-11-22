package docker

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// fakeDaemon serves a tiny subset of the Docker API on a unix socket.
func fakeDaemon(t *testing.T, h http.Handler) string {
	t.Helper()
	// macOS limits unix socket paths to 104 bytes; t.TempDir() is too long.
	dir, err := os.MkdirTemp("/tmp", "ld")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	sock := filepath.Join(dir, "d.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewUnstartedServer(h)
	srv.Listener = ln
	srv.Start()
	t.Cleanup(srv.Close)
	return sock
}

func TestListParsesContainers(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /containers/json", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("all") != "1" {
			t.Errorf("expected all=1, got %s", r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"Id": "abc", "Names": []string{"/jellyfin"}, "Image": "jellyfin:1", "State": "running",
				"Status": "Up 2 hours (healthy)", "Created": 1,
				"Labels": map[string]string{"com.docker.compose.project": "media", "sablier.enable": "true"}},
			{"Id": "def", "Names": []string{"/adguard"}, "Image": "adguard:1", "State": "exited",
				"Status": "Exited (0) 1 hour ago", "Created": 2, "Labels": map[string]string{}},
		})
	})
	c := NewClient(fakeDaemon(t, mux))
	list, err := c.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].Name != "adguard" || list[1].Name != "jellyfin" {
		t.Fatalf("unexpected list: %+v", list)
	}
	j := list[1]
	if j.Health != "healthy" || j.Project != "media" || !j.Sablier {
		t.Fatalf("unexpected parse: %+v", j)
	}
}

func TestActionHandlesNotModifiedAndErrors(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /containers/abc/start", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotModified)
	})
	mux.HandleFunc("POST /containers/abc/stop", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("t") != "10" {
			t.Errorf("expected t=10")
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /containers/missing/restart", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"No such container"}`))
	})
	c := NewClient(fakeDaemon(t, mux))
	ctx := context.Background()
	if err := c.Action(ctx, "abc", "start"); err != nil {
		t.Fatalf("304 should be success: %v", err)
	}
	if err := c.Action(ctx, "abc", "stop"); err != nil {
		t.Fatal(err)
	}
	err := c.Action(ctx, "missing", "restart")
	var ae *APIError
	if !errors.As(err, &ae) || ae.Status != 404 || ae.Message != "No such container" {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := c.Action(ctx, "abc", "kill"); err == nil {
		t.Fatal("kill must be rejected")
	}
	if err := c.Action(ctx, "../x", "start"); err == nil {
		t.Fatal("path traversal id must be rejected")
	}
}

func TestWatcherPublishesSnapshotsAndRefreshesOnEvents(t *testing.T) {
	mux := http.NewServeMux()
	lists := make(chan struct{}, 10)
	mux.HandleFunc("GET /containers/json", func(w http.ResponseWriter, r *http.Request) {
		lists <- struct{}{}
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"Id": "abc", "Names": []string{"/x"}, "State": "running", "Status": "Up", "Labels": map[string]string{}},
		})
	})
	mux.HandleFunc("GET /events", func(w http.ResponseWriter, r *http.Request) {
		fl := w.(http.Flusher)
		_, _ = w.Write([]byte(`{"Action":"die","Actor":{"Attributes":{"name":"x"}}}` + "\n"))
		fl.Flush()
		<-r.Context().Done()
	})
	c := NewClient(fakeDaemon(t, mux))
	w := NewWatcher(c, time.Hour, discardLogger())
	ch, unsub := w.Subscribe()
	defer unsub()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Run(ctx)

	select {
	case snap := <-ch:
		if snap.Containers["x"] == nil {
			t.Fatalf("snapshot missing container: %+v", snap)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no snapshot")
	}
	// Initial poll + event-triggered refresh → at least two list calls.
	deadline := time.After(3 * time.Second)
	count := 0
	for count < 2 {
		select {
		case <-lists:
			count++
		case <-deadline:
			t.Fatalf("expected ≥2 list calls, got %d", count)
		}
	}
}
