package server

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"
)

var pngBytes = []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")

// fakeIconSource stands in for jsDelivr and counts the requests it receives.
type fakeIconSource struct {
	*httptest.Server
	hits  atomic.Int32
	files map[string][]byte
	gate  chan struct{}
}

func newFakeIconSource(t *testing.T, files map[string][]byte) *fakeIconSource {
	t.Helper()
	f := &fakeIconSource{files: files}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.hits.Add(1)
		if f.gate != nil {
			<-f.gate
		}
		if strings.HasPrefix(r.URL.Path, "/broken/") {
			http.Error(w, "down", http.StatusBadGateway)
			return
		}
		data, ok := f.files[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(data)
	}))
	t.Cleanup(f.Close)
	return f
}

func newTestDownloader(t *testing.T, src *fakeIconSource) (*IconDownloader, *os.Root) {
	t.Helper()
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { root.Close() })
	d := NewIconDownloader(root, slog.New(slog.NewTextHandler(io.Discard, nil)))
	d.DashboardIconsURL = src.URL + "/di"
	d.MDIURL = src.URL + "/mdi"
	return d, root
}

func iconServer(t *testing.T, local fstest.MapFS, d *IconDownloader, cache *os.Root) *Server {
	t.Helper()
	opts := Options{IconDownloader: d}
	if local != nil {
		opts.Icons = local
	}
	if cache != nil {
		opts.IconCache = cache.FS()
	}
	return newTestServer(t, NewAuth(false, "", "", 0, true), opts)
}

func getIcon(s *Server, name string) *httptest.ResponseRecorder {
	return serve(s, httptest.NewRequest(http.MethodGet, "/icons/"+name, nil))
}

func TestIconIsDownloadedOnceAndServedFromCache(t *testing.T) {
	src := newFakeIconSource(t, map[string][]byte{
		"/di/png/plex.png":       pngBytes,
		"/mdi/svg/home.svg":      []byte(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg"/>`),
		"/di/svg/jellyfin.svg":   []byte("<svg/>"),
		"/di/webp/jellyfin.webp": []byte("unused"),
		"/di/svg/fake.svg":       []byte("<html>nope</html>"),
		"/di/png/fake.png":       []byte("GIF89a not a png"),
		"/di/webp/fake.webp":     []byte("RIFF"),
		"/di/svg/huge.svg":       []byte("<svg>" + strings.Repeat(" ", maxIconSize) + "</svg>"),
		"/di/png/only-png.png":   pngBytes,
	})
	d, cache := newTestDownloader(t, src)
	s := iconServer(t, nil, d, cache)

	for name, want := range map[string]string{
		"plex":         "image/png",
		"mdi-home":     "image/svg+xml",
		"jellyfin":     "image/svg+xml",
		"only-png.png": "image/png",
	} {
		w := getIcon(s, name)
		if w.Code != http.StatusOK || w.Header().Get("Content-Type") != want {
			t.Errorf("%s: got %d %q", name, w.Code, w.Header().Get("Content-Type"))
		}
		if !strings.Contains(w.Header().Get("Content-Security-Policy"), "sandbox") {
			t.Errorf("%s: downloaded icons need the sandboxing CSP", name)
		}
	}
	hits := src.hits.Load()
	for _, name := range []string{"plex", "mdi-home", "jellyfin", "only-png.png"} {
		if w := getIcon(s, name); w.Code != http.StatusOK {
			t.Errorf("%s from cache: got %d", name, w.Code)
		}
	}
	if src.hits.Load() != hits {
		t.Fatalf("cached icons must not be downloaded again: %d -> %d requests", hits, src.hits.Load())
	}
	if _, err := cache.Stat("plex.png"); err != nil {
		t.Fatalf("icon not stored in cache: %v", err)
	}

	for _, name := range []string{"fake", "huge.svg", "mdi-home.png", "missing"} {
		if w := getIcon(s, name); w.Code != http.StatusNotFound {
			t.Errorf("%s: got %d, want 404", name, w.Code)
		}
	}
	entries, _ := os.ReadDir(cache.Name())
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".tmp-") || strings.HasPrefix(e.Name(), "fake") || strings.HasPrefix(e.Name(), "huge") {
			t.Errorf("rejected download left %s in the cache", e.Name())
		}
	}
}

func TestLocalIconWinsAndDownloadsCanBeDisabled(t *testing.T) {
	src := newFakeIconSource(t, map[string][]byte{"/di/svg/jellyfin.svg": []byte("<svg/>")})
	d, cache := newTestDownloader(t, src)

	local := fstest.MapFS{"jellyfin.png": {Data: pngBytes}}
	if w := getIcon(iconServer(t, local, d, cache), "jellyfin"); w.Code != http.StatusOK || w.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("local icon: got %d %q", w.Code, w.Header().Get("Content-Type"))
	}
	if src.hits.Load() != 0 {
		t.Fatal("an icon from the local folder must not be downloaded")
	}

	if w := getIcon(iconServer(t, nil, nil, cache), "jellyfin"); w.Code != http.StatusNotFound {
		t.Fatalf("downloads disabled: got %d", w.Code)
	}
	if src.hits.Load() != 0 {
		t.Fatal("disabled downloads must not contact the icon source")
	}

	if w := getIcon(iconServer(t, nil, d, cache), "jellyfin"); w.Code != http.StatusOK {
		t.Fatalf("download: got %d", w.Code)
	}
	if w := getIcon(iconServer(t, nil, nil, cache), "jellyfin"); w.Code != http.StatusOK {
		t.Fatalf("previously downloaded icons stay available when downloads are off: got %d", w.Code)
	}
}

func TestIconMissesAreRemembered(t *testing.T) {
	src := newFakeIconSource(t, nil)
	d, cache := newTestDownloader(t, src)
	now := time.Unix(1000, 0)
	d.now = func() time.Time { return now }
	s := iconServer(t, nil, d, cache)

	getIcon(s, "missing")
	hits := src.hits.Load()
	getIcon(s, "missing")
	if src.hits.Load() != hits {
		t.Fatal("a missing icon must not be requested again right away")
	}
	now = now.Add(missMemory)
	getIcon(s, "missing")
	if src.hits.Load() == hits {
		t.Fatal("a missing icon is retried after a day")
	}

	d.DashboardIconsURL = src.URL + "/broken"
	d.MDIURL = src.URL + "/broken"
	if err := d.Fetch("offline.png"); err == nil {
		t.Fatal("a failing source must report an error")
	}
	if until := d.misses["offline.png"]; until != now.Add(errorMemory) {
		t.Fatalf("server errors are retried sooner: remembered until %v", until)
	}
}

func TestConcurrentIconRequestsShareOneDownload(t *testing.T) {
	src := newFakeIconSource(t, map[string][]byte{"/di/svg/grafana.svg": []byte("<svg/>")})
	src.gate = make(chan struct{})
	d, cache := newTestDownloader(t, src)
	s := iconServer(t, nil, d, cache)

	var wg sync.WaitGroup
	codes := make([]int, 5)
	for i := range codes {
		wg.Go(func() { codes[i] = getIcon(s, "grafana").Code })
	}
	for {
		d.mu.Lock()
		started := d.inflight["grafana"] != nil
		d.mu.Unlock()
		if started && src.hits.Load() == 1 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	time.Sleep(20 * time.Millisecond)
	close(src.gate)
	wg.Wait()
	for i, code := range codes {
		if code != http.StatusOK {
			t.Errorf("request %d: got %d", i, code)
		}
	}
	if n := src.hits.Load(); n != 1 {
		t.Fatalf("got %d downloads, want 1", n)
	}
}

func TestRememberedMissesAreBounded(t *testing.T) {
	d := NewIconDownloader(nil, nil)
	now := time.Unix(0, 0)
	d.now = func() time.Time { return now }
	for i := range maxTrackedMiss {
		d.rememberMiss(fmt.Sprint("old-", i), time.Minute)
	}
	now = now.Add(time.Hour)
	d.rememberMiss("fresh", time.Minute)
	if len(d.misses) != 1 {
		t.Fatalf("expired entries must be pruned when full: %d left", len(d.misses))
	}
	now = time.Unix(0, 0)
	for i := range maxTrackedMiss + 1 {
		d.rememberMiss(fmt.Sprint("live-", i), time.Hour)
	}
	if len(d.misses) > maxTrackedMiss {
		t.Fatalf("misses grew beyond the limit: %d", len(d.misses))
	}
}

func TestIconRedirectsStayOnTheSourceHost(t *testing.T) {
	other := newFakeIconSource(t, map[string][]byte{"/x.svg": []byte("<svg/>")})
	src := newFakeIconSource(t, map[string][]byte{"/di/svg/moved.svg": []byte("<svg/>")})
	src.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/di/svg/away.svg":
			http.Redirect(w, r, other.URL+"/x.svg", http.StatusFound)
		case "/di/svg/local.svg":
			http.Redirect(w, r, "/di/svg/moved.svg", http.StatusFound)
		case "/di/svg/moved.svg":
			_, _ = w.Write([]byte("<svg/>"))
		default:
			http.NotFound(w, r)
		}
	})
	d, _ := newTestDownloader(t, src)
	if err := d.Fetch("away.svg"); err == nil {
		t.Fatal("a redirect to another host must not be followed")
	}
	if other.hits.Load() != 0 {
		t.Fatal("the other host must not be contacted")
	}
	if err := d.Fetch("local.svg"); err != nil {
		t.Fatalf("a redirect on the same host must work: %v", err)
	}
}
