package main

import (
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestEnvHelpers(t *testing.T) {
	t.Setenv("LD_TEST_STR", "")
	t.Setenv("LD_TEST_BOOL", "")
	t.Setenv("LD_TEST_DUR", "")
	b, errB := envBool("LD_TEST_BOOL", true)
	d, errD := envDuration("LD_TEST_DUR", time.Second)
	if env("LD_TEST_STR", "def") != "def" || !b || d != time.Second || errB != nil || errD != nil {
		t.Fatal("empty values must use the defaults")
	}
	t.Setenv("LD_TEST_STR", "set")
	t.Setenv("LD_TEST_DUR", "2m")
	if env("LD_TEST_STR", "def") != "set" {
		t.Fatal("set strings must be used")
	}
	if d, err := envDuration("LD_TEST_DUR", time.Second); err != nil || d != 2*time.Minute {
		t.Fatalf("envDuration = %v, %v", d, err)
	}
	for value, want := range map[string]bool{"false": false, "0": false, "No": false, "off": false, "TRUE": true, "1": true, "yes": true, " on ": true} {
		t.Setenv("LD_TEST_BOOL", value)
		if got, err := envBool("LD_TEST_BOOL", !want); err != nil || got != want {
			t.Errorf("envBool(%q) = %v, %v; want %v", value, got, err, want)
		}
	}
	for _, value := range []string{"nein", "maybe"} {
		t.Setenv("LD_TEST_BOOL", value)
		if _, err := envBool("LD_TEST_BOOL", true); err == nil || !strings.Contains(err.Error(), "LD_TEST_BOOL") {
			t.Errorf("envBool(%q) must fail naming the variable, got %v", value, err)
		}
	}
	for _, value := range []string{"soon", "0s", "-5s"} {
		t.Setenv("LD_TEST_DUR", value)
		if _, err := envDuration("LD_TEST_DUR", time.Second); err == nil {
			t.Errorf("envDuration(%q) must fail", value)
		}
	}
	for value, want := range map[string]slog.Level{"debug": slog.LevelDebug, "WARN": slog.LevelWarn, "error": slog.LevelError, "": slog.LevelInfo} {
		t.Setenv("LABDASH_LOG_LEVEL", value)
		if got := logLevel(); got != want {
			t.Errorf("logLevel(%q) = %v, want %v", value, got, want)
		}
	}
}

func TestOpenIconsStaysInsideDirectory(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	if openIcons(filepath.Join(t.TempDir(), "missing"), log) != nil {
		t.Fatal("a missing directory must disable icons")
	}
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret.svg")
	if err := os.WriteFile(outside, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "escape.svg")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "app.svg"), []byte("<svg/>"), 0o600); err != nil {
		t.Fatal(err)
	}
	icons := openIcons(dir, log)
	if _, err := icons.Open("app.svg"); err != nil {
		t.Fatalf("icon inside the directory: %v", err)
	}
	if _, err := icons.Open("escape.svg"); err == nil {
		t.Fatal("symlinks must not escape the icon directory")
	}
}

func TestRunRejectsInvalidSettings(t *testing.T) {
	for name, env := range map[string]map[string]string{
		"missing password": {"LABDASH_AUTH": "password", "LABDASH_AUTH_PASSWORD": ""},
		"unknown auth":     {"LABDASH_AUTH": "oauth"},
		"bad proxy":        {"LABDASH_AUTH": "none", "LABDASH_TRUSTED_PROXIES": "proxy.local"},
		"bad boolean":      {"LABDASH_AUTH": "none", "LABDASH_ICON_DOWNLOAD": "nein"},
		"bad duration":     {"LABDASH_AUTH": "none", "LABDASH_DOCKER_POLL": "-1s"},
		"bad link":         {"LABDASH_AUTH": "none", "LABDASH_CONTAINER_LINK": "arcane/containers/{id}"},
	} {
		t.Run(name, func(t *testing.T) {
			for k, v := range env {
				t.Setenv(k, v)
			}
			if err := run(); err == nil {
				t.Fatal("run must fail")
			}
		})
	}
}

func TestRunStartsUntilListenFails(t *testing.T) {
	// Occupy a port so run() exercises the full startup and returns the
	// listen error instead of serving forever.
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	dir := t.TempDir()
	t.Setenv("LABDASH_PORT", strconv.Itoa(ln.Addr().(*net.TCPAddr).Port))
	t.Setenv("LABDASH_CONFIG", filepath.Join(dir, "config.json"))
	t.Setenv("LABDASH_ICONS", dir)
	t.Setenv("LABDASH_AUTH", "password")
	t.Setenv("LABDASH_AUTH_PASSWORD", "pw")
	t.Setenv("LABDASH_SESSION_SECRET", "")
	t.Setenv("LABDASH_SECURE_COOKIE", "false")
	t.Setenv("LABDASH_TRUSTED_PROXIES", "172.18.0.0/16")
	t.Setenv("LABDASH_DOCKER_SOCKET", filepath.Join(dir, "missing.sock"))
	if err := run(); err == nil || !strings.Contains(err.Error(), "address already in use") {
		t.Fatalf("expected listen error, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "config.json")); err != nil {
		t.Fatalf("default config not written: %v", err)
	}
}

func TestOpenIconCache(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	dir := filepath.Join(t.TempDir(), "icon-cache")
	if cache, d := openIconCache(dir, false, log); cache != nil || d != nil {
		t.Fatal("a missing cache with downloads off disables icon caching")
	}
	if cache, d := openIconCache(dir, true, log); cache == nil || d == nil {
		t.Fatal("downloads on must create the cache and a downloader")
	}
	if cache, d := openIconCache(dir, false, log); cache == nil || d != nil {
		t.Fatal("downloads off must still serve previously downloaded icons")
	}
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if cache, d := openIconCache(filepath.Join(file, "sub"), true, log); cache != nil || d != nil {
		t.Fatal("an unusable cache path disables downloads")
	}
}
