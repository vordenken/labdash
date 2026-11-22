// Command labdash serves a self-hosted home lab dashboard.
package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/vordenken/labdash/internal/config"
	"github.com/vordenken/labdash/internal/providers/docker"
	"github.com/vordenken/labdash/internal/server"
	"github.com/vordenken/labdash/web"
)

var version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "labdash:", err)
		os.Exit(1)
	}
}

func run() error {
	// Keep the build identifiable even when startup fails or normal info logs are filtered.
	slog.New(slog.NewTextHandler(os.Stdout, nil)).Info("starting labdash", "version", version)
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel()}))
	slog.SetDefault(log)

	port := env("LABDASH_PORT", "8080")
	title := env("LABDASH_TITLE", "labdash")
	configPath := env("LABDASH_CONFIG", "/data/config.json")
	authMode := strings.ToLower(env("LABDASH_AUTH", "password"))
	password := os.Getenv("LABDASH_AUTH_PASSWORD")
	sessionSecret := os.Getenv("LABDASH_SESSION_SECRET")
	socket := env("LABDASH_DOCKER_SOCKET", "/var/run/docker.sock")
	iconDir := env("LABDASH_ICONS", "/data/icons")
	iconCacheDir := env("LABDASH_ICON_CACHE", "/data/icon-cache")
	// A mistyped switch must not silently fall back to its default, e.g.
	// LABDASH_ICON_DOWNLOAD=nein would otherwise keep downloads enabled.
	allowActions, errActions := envBool("LABDASH_ALLOW_CONTAINER_ACTIONS", false)
	secureCookie, errCookie := envBool("LABDASH_SECURE_COOKIE", true)
	iconDownload, errDownload := envBool("LABDASH_ICON_DOWNLOAD", true)
	pollInterval, errPoll := envDuration("LABDASH_DOCKER_POLL", 15*time.Second)
	if err := errors.Join(errActions, errCookie, errDownload, errPoll); err != nil {
		return err
	}
	trustedProxies, err := server.ParseTrustedProxies(os.Getenv("LABDASH_TRUSTED_PROXIES"))
	if err != nil {
		return fmt.Errorf("LABDASH_TRUSTED_PROXIES: %w", err)
	}
	containerLink := strings.TrimSpace(os.Getenv("LABDASH_CONTAINER_LINK"))
	if err := server.ValidateContainerLink(containerLink); err != nil {
		return fmt.Errorf("LABDASH_CONTAINER_LINK: %w", err)
	}

	var authEnabled bool
	switch authMode {
	case "password":
		if password == "" {
			return errors.New("LABDASH_AUTH=password requires LABDASH_AUTH_PASSWORD (or set LABDASH_AUTH=none)")
		}
		authEnabled = true
	case "none":
		if allowActions {
			log.Warn("LABDASH_ALLOW_CONTAINER_ACTIONS ignored because LABDASH_AUTH=none")
			allowActions = false
		}
	default:
		return fmt.Errorf("unknown LABDASH_AUTH %q (password|none)", authMode)
	}
	if authEnabled && sessionSecret == "" {
		log.Warn("LABDASH_SESSION_SECRET not set; sessions will be invalidated on restart")
	}
	if authEnabled && !secureCookie {
		log.Warn("LABDASH_SECURE_COOKIE=false sends the session cookie over plain HTTP; only use this on localhost or a trusted network")
	}

	store, err := config.Open(configPath)
	if err != nil {
		return err
	}
	log.Info("config loaded", "path", configPath)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	var (
		watcher *docker.Watcher
		client  *docker.Client
	)
	if socket != "" && socket != "off" {
		if _, err := os.Stat(socket); err != nil {
			log.Warn("docker socket not available, docker features disabled", "socket", socket, "err", err)
		} else {
			client = docker.NewClient(socket)
			watcher = docker.NewWatcher(client, pollInterval, log)
			go watcher.Run(ctx)
			log.Info("docker watcher started", "socket", socket, "interval", pollInterval)
		}
	}

	static, err := fs.Sub(web.Files, "static")
	if err != nil {
		return err
	}

	auth := server.NewAuth(authEnabled, password, sessionSecret, 30*24*time.Hour, secureCookie)
	auth.TrustedProxies = trustedProxies

	iconCache, downloader := openIconCache(iconCacheDir, iconDownload, log)
	srv := server.New(server.Options{
		Title:                 title,
		Auth:                  auth,
		AllowContainerActions: allowActions,
		Store:                 store,
		Docker:                watcher,
		DockerClient:          client,
		Static:                static,
		Log:                   log,
		Version:               version,
		Icons:                 openIcons(iconDir, log),
		IconCache:             iconCache,
		IconDownloader:        downloader,
		ContainerLink:         containerLink,
	})
	go srv.Run(ctx)

	httpSrv := &http.Server{
		Addr:              net.JoinHostPort("", port),
		Handler:           srv,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() {
		log.Info("listening", "addr", httpSrv.Addr, "auth", authMode, "containerActions", allowActions)
		errCh <- httpSrv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		log.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return httpSrv.Shutdown(shutdownCtx)
	}
}

// openIcons confines icon lookups to dir, including symlinks, so a tile icon
// name can never read files elsewhere on the host.
func openIcons(dir string, log *slog.Logger) fs.FS {
	root, err := os.OpenRoot(dir)
	if err != nil {
		log.Info("no local icon directory", "dir", dir, "err", err)
		return nil
	}
	log.Info("serving local icons", "dir", dir)
	return root.FS()
}

// openIconCache returns the folder of downloaded icons and, when downloads
// are enabled, a downloader that fills it. Icons downloaded earlier are still
// served after downloads are switched off.
func openIconCache(dir string, download bool, log *slog.Logger) (fs.FS, *server.IconDownloader) {
	if download {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			log.Warn("icon cache not writable; icon downloads disabled", "dir", dir, "err", err)
			download = false
		}
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		log.Info("icon downloads disabled", "cache", dir)
		return nil, nil
	}
	if !download {
		log.Info("icon downloads disabled; serving previously downloaded icons", "cache", dir)
		return root.FS(), nil
	}
	log.Info("icon downloads enabled", "cache", dir, "sources", []string{server.DefaultDashboardIconsURL, server.DefaultMDIURL})
	return root.FS(), server.NewIconDownloader(root, log)
}

func env(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

// envBool accepts the values of strconv.ParseBool plus yes/no and on/off.
// Unset or empty means def.
func envBool(key string, def bool) (bool, error) {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def, nil
	}
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "yes", "on":
		return true, nil
	case "no", "off":
		return false, nil
	}
	b, err := strconv.ParseBool(strings.TrimSpace(v))
	if err != nil {
		return def, fmt.Errorf("%s=%q is not a boolean (use true or false)", key, v)
	}
	return b, nil
}

// envDuration requires a positive Go duration such as 15s. Unset or empty
// means def.
func envDuration(key string, def time.Duration) (time.Duration, error) {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def, nil
	}
	d, err := time.ParseDuration(strings.TrimSpace(v))
	if err != nil || d <= 0 {
		return def, fmt.Errorf("%s=%q is not a positive duration (for example 15s or 1m)", key, v)
	}
	return d, nil
}

func logLevel() slog.Level {
	switch strings.ToLower(os.Getenv("LABDASH_LOG_LEVEL")) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	}
	return slog.LevelInfo
}
