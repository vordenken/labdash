package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/vordenken/labdash/internal/config"
)

func TestFirstRunSetup(t *testing.T) {
	store, err := config.Open(filepath.Join(t.TempDir(), "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	auth := NewAuth(true, "sample", "", 0, true)
	s := New(Options{Store: store, Auth: auth})
	call := func(method, path string, body []byte, authenticated bool, want int) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, path, bytes.NewReader(body))
		if authenticated {
			r.AddCookie(&http.Cookie{Name: sessionCookie, Value: auth.issue()})
		}
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s %s: got HTTP %d, want %d: %s", method, path, w.Code, want, w.Body)
		}
		return w
	}
	checkLayout := func(wantSetup bool, wantPage string) {
		t.Helper()
		w := call(http.MethodGet, "/api/layout", nil, true, http.StatusOK)
		var got layoutResponse
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if got.NeedsSetup != wantSetup || len(got.Pages) != 1 || got.Pages[0].Title != wantPage {
			t.Fatalf("unexpected layout: %+v", got)
		}
	}
	checkLayout(true, "Home")
	cfg := store.Get()
	cfg.Pages[0].Title = "Overview"
	body, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	call(http.MethodPut, "/api/setup", body, false, http.StatusUnauthorized)
	call(http.MethodPut, "/api/setup", []byte(`{`), true, http.StatusBadRequest)
	call(http.MethodPut, "/api/setup", []byte(`{"version":123}`), true, http.StatusUnprocessableEntity)
	call(http.MethodPut, "/api/setup", body, true, http.StatusOK)
	call(http.MethodPut, "/api/setup", body, true, http.StatusConflict)
	checkLayout(false, "Overview")
}

func TestSaveConfigReturnsCommittedLayoutAfterPageRemoval(t *testing.T) {
	store, err := config.Open(filepath.Join(t.TempDir(), "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	cfg := store.Get()
	cfg.Pages = append(cfg.Pages, config.Page{ID: "tools", Title: "Tools"})
	if err := store.Set(cfg); err != nil {
		t.Fatal(err)
	}
	cfg.Pages = cfg.Pages[:1]
	cfg.SetupComplete = true
	body, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	s := New(Options{Store: store, Auth: NewAuth(false, "", "", 0, true)})
	w := httptest.NewRecorder()
	s.ServeHTTP(w, httptest.NewRequest(http.MethodPut, "/api/config", bytes.NewReader(body)))
	if w.Code != http.StatusOK {
		t.Fatalf("got HTTP %d: %s", w.Code, w.Body)
	}
	expected, err := json.Marshal(s.layout())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bytes.TrimSpace(w.Body.Bytes()), expected) {
		t.Fatalf("save did not return the persisted dashboard layout: %s", w.Body)
	}
	if saved := store.Get(); len(saved.Pages) != 1 || saved.Pages[0].ID != "home" {
		t.Fatalf("save did not remove the page: %+v", saved.Pages)
	}
}

func TestSaveConfigPreservesEmptyGroupsWithDockerDiscovery(t *testing.T) {
	_, watcher := fakeDocker(t)
	path := filepath.Join(t.TempDir(), "config.json")
	store, err := config.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg := store.Get()
	cfg.Discovery.Docker = true
	cfg.Pages = []config.Page{
		{ID: "home", Title: "Home", Groups: []config.Group{
			{ID: "tools", Title: "Tools", Tiles: []config.Tile{}},
		}},
		{ID: "media", Title: "Media", Groups: []config.Group{
			{ID: "services", Title: "Services", Tiles: []config.Tile{}},
		}},
	}
	body, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	s := New(Options{Store: store, Auth: NewAuth(false, "", "", 0, true), Docker: watcher})
	for _, method := range []string{http.MethodPut, http.MethodGet} {
		endpoint := "/api/config"
		if method == http.MethodGet {
			endpoint = "/api/layout"
		}
		w := httptest.NewRecorder()
		s.ServeHTTP(w, httptest.NewRequest(method, endpoint, bytes.NewReader(body)))
		if w.Code != http.StatusOK {
			t.Fatalf("%s %s: got HTTP %d: %s", method, endpoint, w.Code, w.Body)
		}
		var got layoutResponse
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if len(got.Pages) != 2 {
			t.Fatalf("expected two pages: %s", w.Body)
		}
		for i, page := range got.Pages {
			if page.ID != cfg.Pages[i].ID || len(page.Groups) == 0 {
				t.Fatalf("configured page or group missing: %s", w.Body)
			}
			group := page.Groups[0]
			if group.ID != cfg.Pages[i].Groups[0].ID || group.Tiles == nil || len(group.Tiles) != 0 {
				t.Fatalf("%s %s: empty group must retain tiles: []: %s", method, endpoint, w.Body)
			}
		}
		groups := got.Pages[0].Groups
		if len(groups) != 2 || len(groups[1].Tiles) != 1 || groups[1].Tiles[0].Container != "media" {
			t.Fatalf("Docker discovery tile missing: %s", w.Body)
		}
	}
	reopened, err := config.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	saved := reopened.Get()
	if len(saved.Pages) != 2 {
		t.Fatalf("expected two persisted pages: %+v", saved.Pages)
	}
	for _, page := range saved.Pages {
		if len(page.Groups) != 1 || page.Groups[0].Tiles == nil || len(page.Groups[0].Tiles) != 0 {
			t.Fatalf("empty group changed on disk: %+v", page)
		}
	}
}
