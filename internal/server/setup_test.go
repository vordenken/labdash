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
