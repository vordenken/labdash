// Package discovery turns Docker container labels into dashboard tiles and
// merges them with the user's configured layout.
package discovery

import (
	"sort"
	"strings"

	"github.com/vordenken/labdash/internal/config"
	"github.com/vordenken/labdash/internal/providers/docker"
)

// Discovered describes a tile derived from container labels.
type Discovered struct {
	Container string
	Group     string
	Tile      config.Tile
	// Widget params found under <prefix>.widget.* (may contain secrets;
	// resolved server-side only).
	Widget map[string]string
}

// FromLabels extracts a tile from a container's labels using the first
// prefix (e.g. "labdash", "homepage") that defines a name or href.
func FromLabels(c *docker.Container, prefixes []string) (Discovered, bool) {
	for _, p := range prefixes {
		pfx := p + "."
		name := c.Labels[pfx+"name"]
		href := c.Labels[pfx+"href"]
		if !config.SafeHref(href) {
			href = ""
		}
		if name == "" && href == "" {
			continue
		}
		if name == "" {
			name = c.Name
		}
		d := Discovered{
			Container: c.Name,
			Group:     c.Labels[pfx+"group"],
			Tile: config.Tile{
				ID:          "disc-" + sanitizeID(c.Name),
				Type:        "link",
				Name:        name,
				Href:        href,
				Icon:        c.Labels[pfx+"icon"],
				Description: c.Labels[pfx+"description"],
				Container:   c.Name,
				Source:      "discovery",
			},
			Widget: map[string]string{},
		}
		wpfx := pfx + "widget."
		for k, v := range c.Labels {
			if strings.HasPrefix(k, wpfx) {
				d.Widget[strings.TrimPrefix(k, wpfx)] = v
			}
		}
		if d.Group == "" {
			d.Group = "Discovered"
		}
		return d, true
	}
	return Discovered{}, false
}

func sanitizeID(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	out := b.String()
	if out == "" {
		out = "x"
	}
	if len(out) > 60 {
		out = out[:60]
	}
	return out
}

// Merge returns the pages of cfg with discovered tiles appended. Containers
// that are already bound to a configured tile, or listed in
// cfg.Discovery.Hidden, are skipped. Discovered groups that match a
// configured group title (case-insensitive) are merged into it; others are
// appended to the first page in alphabetical order.
func Merge(cfg config.Config, snap docker.Snapshot) []config.Page {
	pages := clonePages(cfg.Pages)
	if !cfg.Discovery.Docker || len(pages) == 0 {
		return pages
	}
	bound := map[string]bool{}
	hidden := map[string]bool{}
	for _, h := range cfg.Discovery.Hidden {
		hidden[h] = true
	}
	for _, p := range pages {
		for _, g := range p.Groups {
			for _, t := range g.Tiles {
				if t.Container != "" {
					bound[t.Container] = true
				}
			}
		}
	}

	// Older configs included homepage in the default list. Only the explicit
	// opt-in enables it, and its labels are always a fallback.
	prefixes := make([]string, 0, len(cfg.Discovery.LabelPrefixes)+1)
	for _, prefix := range cfg.Discovery.LabelPrefixes {
		if prefix != "homepage" {
			prefixes = append(prefixes, prefix)
		}
	}
	if cfg.Discovery.Homepage {
		prefixes = append(prefixes, "homepage")
	}

	byGroup := map[string][]config.Tile{}
	for _, c := range snap.Containers {
		if bound[c.Name] || hidden[c.Name] {
			continue
		}
		d, ok := FromLabels(c, prefixes)
		if !ok {
			continue
		}
		byGroup[d.Group] = append(byGroup[d.Group], d.Tile)
	}
	if len(byGroup) == 0 {
		return pages
	}

	// Locate existing groups by title.
	type loc struct{ p, g int }
	existing := map[string]loc{}
	for pi, p := range pages {
		for gi, g := range p.Groups {
			existing[strings.ToLower(g.Title)] = loc{pi, gi}
		}
	}

	names := make([]string, 0, len(byGroup))
	for n := range byGroup {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		tiles := byGroup[name]
		sort.Slice(tiles, func(i, j int) bool { return strings.ToLower(tiles[i].Name) < strings.ToLower(tiles[j].Name) })
		if l, ok := existing[strings.ToLower(name)]; ok {
			g := &pages[l.p].Groups[l.g]
			g.Tiles = append(g.Tiles, tiles...)
			continue
		}
		pages[0].Groups = append(pages[0].Groups, config.Group{
			ID:    "disc-" + sanitizeID(strings.ToLower(name)),
			Title: name,
			Tiles: tiles,
		})
	}
	return pages
}

func clonePages(in []config.Page) []config.Page {
	out := make([]config.Page, len(in))
	for i, p := range in {
		out[i] = p
		out[i].Groups = make([]config.Group, len(p.Groups))
		for j, g := range p.Groups {
			out[i].Groups[j] = g
			out[i].Groups[j].Tiles = append([]config.Tile(nil), g.Tiles...)
		}
	}
	return out
}
