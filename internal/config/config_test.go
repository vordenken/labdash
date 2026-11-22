package config

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestCompleteSetupPersistsSkipAndRejectsStaleSubmission(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg := s.Get()
	if !cfg.NeedsSetup() {
		t.Fatal("fresh config should need setup")
	}
	if err := s.CompleteSetup(cfg); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := reopened.Get(); got.NeedsSetup() || got.Pages[0].Title != cfg.Pages[0].Title {
		t.Fatalf("skipping setup should persist without changing the page: %+v", got)
	}
	cfg.Title = "Stale edit"
	if err := reopened.CompleteSetup(cfg); !errors.Is(err, ErrSetupUnavailable) {
		t.Fatalf("expected a stale setup to be rejected, got %v", err)
	}
}

func TestConfiguredLayoutDoesNotNeedSetup(t *testing.T) {
	for _, customize := range []func(*Config){
		func(c *Config) { c.Pages[0].Title = "Overview" },
		func(c *Config) { c.Pages[0].Groups = []Group{{ID: "tools"}} },
		func(c *Config) { c.Appearance.Name = "Morgan" },
		func(c *Config) { c.Pages = append(c.Pages, Page{ID: "ops"}) },
	} {
		cfg := Default()
		customize(&cfg)
		if cfg.NeedsSetup() {
			t.Fatalf("custom layout should not need setup: %+v", cfg)
		}
	}
}

func TestOpenCreatesDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "config.json")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Get(); got.Title != "labdash" || len(got.Pages) != 1 {
		t.Fatalf("unexpected default: %+v", got)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("default file not written: %v", err)
	}
}

func TestSetWritesBackupAndRejectsInvalid(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}

	cfg := s.Get()
	cfg.Pages[0].Groups = []Group{{ID: "g1", Title: "G", Tiles: []Tile{{ID: "t1", Name: "A"}}}}
	if err := s.Set(cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path + ".bak"); err != nil {
		t.Fatalf("backup missing: %v", err)
	}
	if got := s.Get().Pages[0].Groups[0].Tiles[0].Type; got != "link" {
		t.Fatalf("type default not applied: %q", got)
	}

	bad := s.Get()
	bad.Pages[0].Groups = append(bad.Pages[0].Groups, Group{ID: "g1"})
	if err := s.Set(bad); err == nil {
		t.Fatal("expected duplicate id error")
	}
	bad = s.Get()
	bad.Pages[0].ID = "bad id!"
	if err := s.Set(bad); err == nil {
		t.Fatal("expected invalid id error")
	}
}

func TestHomepageOptInPersistsWithoutChangingOtherDiscoverySettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"discovery":{"docker":true,"labelPrefixes":["homepage","labdash"],"hidden":["hidden-app"]}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg := s.Get()
	if cfg.Discovery.Homepage {
		t.Fatal("legacy label prefixes must not opt in to Homepage discovery")
	}
	cfg.Discovery.Homepage = true
	if err := s.Set(cfg); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	got := reopened.Get().Discovery
	if !got.Docker || !got.Homepage || !slices.Equal(got.Hidden, cfg.Discovery.Hidden) || !slices.Equal(got.LabelPrefixes, cfg.Discovery.LabelPrefixes) {
		t.Fatalf("unexpected discovery settings after reopening: %+v", got)
	}
}

func TestValidateAppearanceAndSize(t *testing.T) {
	cfg := Default()
	cfg.Appearance.Accent = "#12abEF"
	if err := Validate(cfg); err != nil {
		t.Fatalf("hex accent should be valid: %v", err)
	}
	cfg.Appearance.Accent = "neon"
	if err := Validate(cfg); err == nil {
		t.Fatal("unknown accent must fail")
	}
	cfg = Default()
	cfg.Pages[0].Groups = []Group{{ID: "g", Tiles: []Tile{{ID: "t", Type: "link", Size: "huge"}}}}
	if err := Validate(cfg); err == nil {
		t.Fatal("invalid size must fail")
	}
	// Older config without appearance gets full defaults (hero on).
	parsed, err := Parse([]byte(`{"version":1,"pages":[{"id":"home","groups":[]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if !parsed.Appearance.Hero || parsed.Appearance.Accent != "indigo" {
		t.Fatalf("defaults not applied: %+v", parsed.Appearance)
	}
}

func TestValidateRejectsScriptLinks(t *testing.T) {
	for href, want := range map[string]bool{
		"":                        true,
		"https://media.example":   true,
		"/relative/path":          true,
		"smb://nas/share":         true,
		"javascript:alert(1)":     false,
		"JavaScript:alert(1)":     false,
		"java\tscript:alert(1)":   false,
		" javascript:alert(1)":    false,
		"data:text/html,<b>x</b>": false,
		"vbscript:msgbox":         false,
		"http://[::1":             false,
	} {
		if got := SafeHref(href); got != want {
			t.Errorf("SafeHref(%q) = %v, want %v", href, got, want)
		}
	}
	cfg := Default()
	cfg.Pages[0].Groups = []Group{{ID: "g", Tiles: []Tile{{ID: "t", Type: "link", Href: "javascript:alert(1)"}}}}
	if err := Validate(cfg); err == nil {
		t.Fatal("script link must fail validation")
	}
}
