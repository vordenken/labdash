package server

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path"
	"strings"
	"sync"
	"time"
)

// Public icon collections that labdash may download from. Only these fixed
// sources are used, so a tile icon can never make the server request an
// arbitrary (for example internal) address.
const (
	DefaultDashboardIconsURL = "https://cdn.jsdelivr.net/gh/homarr-labs/dashboard-icons"
	DefaultMDIURL            = "https://cdn.jsdelivr.net/npm/@mdi/svg"
)

const (
	missMemory      = 24 * time.Hour
	errorMemory     = time.Minute
	maxTrackedMiss  = 10000
	downloadTimeout = 5 * time.Second
)

var errIconUnavailable = errors.New("icon unavailable")

// IconDownloader fetches a missing tile icon once from a public icon
// collection and keeps it in a local cache directory. Afterwards the icon is
// served from disk, so the dashboard keeps working offline and the browser
// never contacts a third party.
type IconDownloader struct {
	cache             *os.Root
	client            *http.Client
	log               *slog.Logger
	DashboardIconsURL string
	MDIURL            string

	mu       sync.Mutex
	inflight map[string]*iconCall
	misses   map[string]time.Time
	now      func() time.Time
}

type iconCall struct {
	done chan struct{}
	err  error
}

// NewIconDownloader stores downloaded icons in cache.
func NewIconDownloader(cache *os.Root, log *slog.Logger) *IconDownloader {
	if log == nil {
		log = slog.Default()
	}
	return &IconDownloader{
		cache: cache,
		client: &http.Client{
			Timeout: downloadTimeout,
			// Fixed sources only: a redirect must not lead the server to
			// another host, such as an address on the internal network.
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 3 || req.URL.Host != via[0].URL.Host || req.URL.Scheme != via[0].URL.Scheme {
					return errors.New("icon redirect to another host refused")
				}
				return nil
			},
		},
		log:               log,
		DashboardIconsURL: DefaultDashboardIconsURL,
		MDIURL:            DefaultMDIURL,
		inflight:          map[string]*iconCall{},
		misses:            map[string]time.Time{},
		now:               time.Now,
	}
}

// Fetch downloads name into the cache unless it is known to be missing.
// Concurrent requests for the same name share one download.
func (d *IconDownloader) Fetch(name string) error {
	d.mu.Lock()
	if until, ok := d.misses[name]; ok && d.now().Before(until) {
		d.mu.Unlock()
		return errIconUnavailable
	}
	if c, ok := d.inflight[name]; ok {
		d.mu.Unlock()
		<-c.done
		return c.err
	}
	c := &iconCall{done: make(chan struct{})}
	d.inflight[name] = c
	d.mu.Unlock()

	notFound, err := d.download(name)
	c.err = err

	d.mu.Lock()
	delete(d.inflight, name)
	if err != nil {
		memory := errorMemory
		if notFound {
			memory = missMemory
		}
		d.rememberMiss(name, memory)
	}
	d.mu.Unlock()
	close(c.done)
	return err
}

// rememberMiss avoids asking the icon source again for every page view.
// The map is bounded; the caller holds d.mu.
func (d *IconDownloader) rememberMiss(name string, memory time.Duration) {
	now := d.now()
	if len(d.misses) >= maxTrackedMiss {
		for k, until := range d.misses {
			if !now.Before(until) {
				delete(d.misses, k)
			}
		}
		if len(d.misses) >= maxTrackedMiss {
			clear(d.misses)
		}
	}
	d.misses[name] = now.Add(memory)
}

// sources maps an icon name to candidate URLs and the cache file name each
// one is stored as. "mdi-<name>" is a Material Design icon; anything else is
// looked up in Dashboard Icons, preferring SVG.
func (d *IconDownloader) sources(name string) [][2]string {
	ext := strings.ToLower(path.Ext(name))
	base := name
	if hasIconExt(name) {
		base = strings.TrimSuffix(name, path.Ext(name))
	} else {
		ext = ""
	}
	if mdi, ok := strings.CutPrefix(base, "mdi-"); ok && mdi != "" {
		if ext != "" && ext != ".svg" {
			return nil
		}
		return [][2]string{{d.MDIURL + "/svg/" + mdi + ".svg", base + ".svg"}}
	}
	var out [][2]string
	for _, e := range []string{".svg", ".png", ".webp"} {
		if ext == "" || ext == e {
			out = append(out, [2]string{d.DashboardIconsURL + "/" + e[1:] + "/" + base + e, base + e})
		}
	}
	return out
}

// download tries each source in turn. notFound reports that every source
// answered "not found", as opposed to a network or server error.
func (d *IconDownloader) download(name string) (notFound bool, err error) {
	srcs := d.sources(name)
	if len(srcs) == 0 {
		return true, errIconUnavailable
	}
	notFound = true
	for _, src := range srcs {
		data, status, err := d.get(src[0])
		if err != nil || status != http.StatusOK {
			if err != nil || status != http.StatusNotFound {
				notFound = false
				d.log.Warn("icon download failed", "url", src[0], "status", status, "err", err)
			}
			continue
		}
		if !looksLikeImage(path.Ext(src[1]), data) {
			notFound = false
			d.log.Warn("icon download rejected: not an image", "url", src[0])
			continue
		}
		if err := d.store(src[1], data); err != nil {
			d.log.Warn("icon could not be cached", "file", src[1], "err", err)
			return false, err
		}
		d.log.Info("icon downloaded", "name", name, "url", src[0])
		return false, nil
	}
	return notFound, errIconUnavailable
}

func (d *IconDownloader) get(url string) ([]byte, int, error) {
	resp, err := d.client.Get(url)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return nil, resp.StatusCode, nil
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxIconSize+1))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	if len(data) > maxIconSize {
		return nil, resp.StatusCode, fmt.Errorf("icon larger than %d bytes", maxIconSize)
	}
	return data, resp.StatusCode, nil
}

// looksLikeImage checks the downloaded bytes instead of trusting the
// response's Content-Type.
func looksLikeImage(ext string, data []byte) bool {
	switch ext {
	case ".svg":
		head := data[:min(len(data), 1024)]
		return bytes.Contains(bytes.ToLower(head), []byte("<svg"))
	case ".png":
		return http.DetectContentType(data) == "image/png"
	case ".webp":
		return http.DetectContentType(data) == "image/webp"
	}
	return false
}

// store writes atomically so a concurrent reader never sees a partial file.
func (d *IconDownloader) store(name string, data []byte) error {
	tmp := ".tmp-" + name
	if err := d.cache.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := d.cache.Rename(tmp, name); err != nil {
		_ = d.cache.Remove(tmp)
		return err
	}
	return nil
}
