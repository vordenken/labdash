package server

import (
	"bytes"
	"io"
	"io/fs"
	"net/http"
	"path"
	"regexp"
	"strings"
	"time"
)

// iconExts lists the image types served from the local icon directory, in
// the order tried for names given without an extension.
var iconExts = []string{".svg", ".png", ".webp", ".jpg", ".jpeg", ".gif", ".ico", ".avif"}

// iconTypes avoids depending on the host's MIME database, which the scratch
// image does not have.
var iconTypes = map[string]string{
	".svg": "image/svg+xml", ".png": "image/png", ".webp": "image/webp", ".jpg": "image/jpeg",
	".jpeg": "image/jpeg", ".gif": "image/gif", ".ico": "image/x-icon", ".avif": "image/avif",
}

var iconNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

const maxIconSize = 2 << 20

// iconPolicy prevents an SVG icon opened directly in the browser from running
// scripts on the dashboard's origin.
const iconPolicy = "default-src 'none'; style-src 'unsafe-inline'; sandbox"

// handleIcon serves a tile image. A name such as "jellyfin" resolves to
// jellyfin.svg, .png, ... in that order. The user's icon folder wins over
// previously downloaded icons; if neither has the icon and downloads are
// enabled, the server fetches it once and serves it from the cache.
func (s *Server) handleIcon(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !iconNameRe.MatchString(name) || strings.Contains(name, "..") {
		http.NotFound(w, r)
		return
	}
	candidates := []string{name}
	if !hasIconExt(name) {
		candidates = candidates[:0]
		for _, ext := range iconExts {
			candidates = append(candidates, name+ext)
		}
	}
	if s.serveIconFrom(w, r, candidates, s.opts.Icons, s.opts.IconCache) {
		return
	}
	if s.opts.IconDownloader != nil && s.opts.IconDownloader.Fetch(name) == nil &&
		s.serveIconFrom(w, r, candidates, s.opts.IconCache) {
		return
	}
	http.NotFound(w, r)
}

func (s *Server) serveIconFrom(w http.ResponseWriter, r *http.Request, candidates []string, dirs ...fs.FS) bool {
	for _, dir := range dirs {
		if dir == nil {
			continue
		}
		for _, file := range candidates {
			data, modTime, err := readIcon(dir, file)
			if err != nil {
				continue
			}
			w.Header().Set("Content-Type", iconTypes[strings.ToLower(path.Ext(file))])
			w.Header().Set("Content-Security-Policy", iconPolicy)
			w.Header().Set("Cache-Control", "private, max-age=86400")
			http.ServeContent(w, r, file, modTime, bytes.NewReader(data))
			return true
		}
	}
	return false
}

func hasIconExt(name string) bool {
	_, ok := iconTypes[strings.ToLower(path.Ext(name))]
	return ok
}

func readIcon(fsys fs.FS, name string) ([]byte, time.Time, error) {
	f, err := fsys.Open(name)
	if err != nil {
		return nil, time.Time{}, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, time.Time{}, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxIconSize {
		return nil, time.Time{}, fs.ErrNotExist
	}
	data, err := io.ReadAll(io.LimitReader(f, maxIconSize))
	return data, info.ModTime(), err
}
