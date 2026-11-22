// Package config holds the dashboard layout configuration and its persistence.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

const CurrentVersion = 1

// Config is the persisted dashboard layout. It is readable by every signed-in
// user and must not contain credentials.
type Config struct {
	Version       int        `json:"version"`
	Title         string     `json:"title"`
	Theme         string     `json:"theme"`
	Appearance    Appearance `json:"appearance"`
	Pages         []Page     `json:"pages"`
	Discovery     Discovery  `json:"discovery"`
	SetupComplete bool       `json:"setupComplete,omitempty"`
}

// Appearance holds purely visual settings editable in the browser.
type Appearance struct {
	// Accent is a preset name (indigo, teal, amber, rose, violet, emerald)
	// or a #rrggbb color.
	Accent string `json:"accent"`
	Hero   bool   `json:"hero"`
	Name   string `json:"name"`
}

type Page struct {
	ID     string  `json:"id"`
	Title  string  `json:"title"`
	Groups []Group `json:"groups"`
}

type Group struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Tiles []Tile `json:"tiles"`
}

// Tile is a single card on the dashboard. Type selects the frontend widget
// module; Params are passed to it. Container optionally binds the tile to a
// Docker container for live status.
type Tile struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	Name        string `json:"name,omitempty"`
	Href        string `json:"href,omitempty"`
	Icon        string `json:"icon,omitempty"`
	Description string `json:"description,omitempty"`
	Container   string `json:"container,omitempty"`
	// Size is "" (normal) or "wide" (spans two columns).
	Size   string         `json:"size,omitempty"`
	Params map[string]any `json:"params,omitempty"`
	// Source is set at runtime ("config" or "discovery"); never persisted.
	Source string `json:"source,omitempty"`
}

type Discovery struct {
	Docker        bool     `json:"docker"`
	Homepage      bool     `json:"homepage"`
	LabelPrefixes []string `json:"labelPrefixes"`
	// Hidden lists container names that discovery must not show.
	Hidden []string `json:"hidden"`
}

func Default() Config {
	return Config{
		Version: CurrentVersion,
		Title:   "labdash",
		Theme:   "dark",
		Appearance: Appearance{
			Accent: "indigo",
			Hero:   true,
		},
		Pages: []Page{{
			ID:     "home",
			Title:  "Home",
			Groups: []Group{},
		}},
		Discovery: Discovery{
			Docker:        true,
			LabelPrefixes: []string{"labdash"},
			Hidden:        []string{},
		},
	}
}

// NeedsSetup excludes existing custom layouts, even those without any tiles.
func (cfg Config) NeedsSetup() bool {
	return !cfg.SetupComplete && cfg.Title == "labdash" && cfg.Theme == "dark" &&
		cfg.Appearance == (Appearance{Accent: "indigo", Hero: true}) &&
		len(cfg.Pages) == 1 && cfg.Pages[0].ID == "home" && cfg.Pages[0].Title == "Home" &&
		len(cfg.Pages[0].Groups) == 0
}

var ErrSetupUnavailable = errors.New("first-run setup is no longer available")

// Store persists a Config to a JSON file with atomic writes.
type Store struct {
	path string
	mu   sync.RWMutex
	cfg  Config
}

// Open loads the config at path, creating a default file if none exists.
func Open(path string) (*Store, error) {
	s := &Store{path: path}
	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		s.cfg = Default()
		if err := s.write(s.cfg); err != nil {
			return nil, fmt.Errorf("write default config: %w", err)
		}
		return s, nil
	case err != nil:
		return nil, err
	}
	cfg, err := Parse(data)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	s.cfg = cfg
	return s, nil
}

func Parse(data []byte) (Config, error) {
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	normalize(&cfg)
	if err := Validate(cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func normalize(cfg *Config) {
	if cfg.Version == 0 {
		cfg.Version = CurrentVersion
	}
	if cfg.Title == "" {
		cfg.Title = "labdash"
	}
	if cfg.Theme == "" {
		cfg.Theme = "dark"
	}
	if cfg.Appearance.Accent == "" {
		// Appearance absent entirely (older config): apply full defaults.
		cfg.Appearance = Default().Appearance
	}
	if cfg.Pages == nil {
		cfg.Pages = Default().Pages
	}
	if cfg.Discovery.LabelPrefixes == nil {
		cfg.Discovery.LabelPrefixes = Default().Discovery.LabelPrefixes
	}
	if cfg.Discovery.Hidden == nil {
		cfg.Discovery.Hidden = []string{}
	}
	for pi := range cfg.Pages {
		p := &cfg.Pages[pi]
		if p.Groups == nil {
			p.Groups = []Group{}
		}
		for gi := range p.Groups {
			g := &p.Groups[gi]
			if g.Tiles == nil {
				g.Tiles = []Tile{}
			}
			for ti := range g.Tiles {
				t := &g.Tiles[ti]
				t.Source = ""
				if t.Type == "" {
					t.Type = "link"
				}
			}
		}
	}
}

var (
	idRe     = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)
	hexRe    = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
	accents  = map[string]bool{"indigo": true, "teal": true, "amber": true, "rose": true, "violet": true, "emerald": true}
	tileSize = map[string]bool{"": true, "wide": true}
)

// unsafeSchemes can run code or render attacker-controlled content in the
// dashboard's context when used as a tile link.
var unsafeSchemes = map[string]bool{"javascript": true, "data": true, "vbscript": true, "blob": true}

// SafeHref reports whether href may be rendered as a tile link. Relative
// links and schemes like http, https, ssh or smb are allowed.
func SafeHref(href string) bool {
	if href == "" {
		return true
	}
	// Browsers strip whitespace and control characters inside schemes
	// ("java\tscript:"), so reject them instead of normalising.
	for _, r := range href {
		if r <= ' ' || r == 0x7f {
			return false
		}
	}
	u, err := url.Parse(href)
	if err != nil {
		return false
	}
	return !unsafeSchemes[strings.ToLower(u.Scheme)]
}

// Validate checks structural invariants: unique, well-formed IDs and a
// supported version.
func Validate(cfg Config) error {
	if cfg.Version != CurrentVersion {
		return fmt.Errorf("unsupported config version %d", cfg.Version)
	}
	if len(cfg.Pages) == 0 {
		return errors.New("at least one page is required")
	}
	if a := cfg.Appearance.Accent; !accents[a] && !hexRe.MatchString(a) {
		return fmt.Errorf("unknown accent %q", a)
	}
	if len(cfg.Appearance.Name) > 64 {
		return errors.New("greeting name too long")
	}
	seen := map[string]bool{}
	check := func(kind, id string) error {
		if !idRe.MatchString(id) {
			return fmt.Errorf("%s id %q is invalid", kind, id)
		}
		key := kind + ":" + id
		if seen[key] {
			return fmt.Errorf("duplicate %s id %q", kind, id)
		}
		seen[key] = true
		return nil
	}
	for _, p := range cfg.Pages {
		if err := check("page", p.ID); err != nil {
			return err
		}
		for _, g := range p.Groups {
			if err := check("group", g.ID); err != nil {
				return err
			}
			for _, t := range g.Tiles {
				if err := check("tile", t.ID); err != nil {
					return err
				}
				if t.Type == "" {
					return fmt.Errorf("tile %q has no type", t.ID)
				}
				if !tileSize[t.Size] {
					return fmt.Errorf("tile %q has invalid size %q", t.ID, t.Size)
				}
				if !SafeHref(t.Href) {
					return fmt.Errorf("tile %q has an unsupported link %q; use http(s) or another non-script URL", t.ID, t.Href)
				}
			}
		}
	}
	return nil
}

func (s *Store) Get() Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return clone(s.cfg)
}

// Set validates, persists and activates a new config. The previous file is
// kept as <path>.bak.
func (s *Store) Set(cfg Config) error {
	return s.set(cfg, false)
}

func (s *Store) CompleteSetup(cfg Config) error {
	cfg.SetupComplete = true
	return s.set(cfg, true)
}

func (s *Store) set(cfg Config, firstRun bool) error {
	normalize(&cfg)
	if err := Validate(cfg); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if firstRun && !s.cfg.NeedsSetup() {
		return ErrSetupUnavailable
	}
	if err := s.write(cfg); err != nil {
		return err
	}
	s.cfg = cfg
	return nil
}

func (s *Store) write(cfg Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".config-*.json")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0o600); err != nil {
		return err
	}
	if _, err := os.Stat(s.path); err == nil {
		_ = os.Rename(s.path, s.path+".bak")
	}
	return os.Rename(tmpName, s.path)
}

func clone(cfg Config) Config {
	data, _ := json.Marshal(cfg)
	var out Config
	_ = json.Unmarshal(data, &out)
	return out
}
