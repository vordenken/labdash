package discovery

import (
	"slices"
	"testing"

	"github.com/vordenken/labdash/internal/config"
	"github.com/vordenken/labdash/internal/providers/docker"
)

func ctr(name string, labels map[string]string) *docker.Container {
	return &docker.Container{Name: name, Labels: labels}
}

func TestFromLabelsPrefixPriorityAndWidget(t *testing.T) {
	c := ctr("jellyfin", map[string]string{
		"homepage.name":        "Jellyfin",
		"homepage.group":       "Medien",
		"homepage.href":        "https://j",
		"homepage.icon":        "jellyfin.png",
		"homepage.widget.type": "jellyfin",
		"homepage.widget.key":  "secret",
		"labdash.name":         "JF",
	})
	d, ok := FromLabels(c, []string{"labdash", "homepage"})
	if !ok || d.Tile.Name != "JF" {
		t.Fatalf("labdash prefix should win: %+v", d)
	}
	d, _ = FromLabels(c, []string{"homepage"})
	if d.Group != "Medien" || d.Tile.Href != "https://j" || d.Widget["key"] != "secret" || d.Tile.Container != "jellyfin" {
		t.Fatalf("homepage parse: %+v", d)
	}
	if _, ok := FromLabels(ctr("plain", map[string]string{"foo": "bar"}), []string{"homepage"}); ok {
		t.Fatal("unlabeled container must not be discovered")
	}
}

func TestMergeSkipsBoundAndHiddenAndMergesGroups(t *testing.T) {
	cfg := config.Default()
	cfg.Discovery.Homepage = true
	cfg.Discovery.Hidden = []string{"hidden"}
	cfg.Pages[0].Groups = []config.Group{{ID: "media", Title: "medien", Tiles: []config.Tile{
		{ID: "t1", Type: "link", Name: "Mine", Container: "bound"},
	}}}
	snap := docker.Snapshot{Containers: map[string]*docker.Container{
		"bound":  ctr("bound", map[string]string{"homepage.name": "Bound", "homepage.group": "Medien"}),
		"hidden": ctr("hidden", map[string]string{"homepage.name": "Hidden", "homepage.group": "Medien"}),
		"b":      ctr("b", map[string]string{"homepage.name": "Beta", "homepage.group": "Medien"}),
		"a":      ctr("a", map[string]string{"homepage.name": "Alpha", "homepage.group": "Medien"}),
		"n":      ctr("n", map[string]string{"homepage.name": "Net", "homepage.group": "Netzwerk"}),
		"u":      ctr("u", map[string]string{"homepage.href": "https://u"}),
	}}
	pages := Merge(cfg, snap)
	groups := pages[0].Groups
	if len(groups) != 3 {
		t.Fatalf("expected 3 groups, got %d: %+v", len(groups), groups)
	}

	media := groups[0]
	if len(media.Tiles) != 3 || media.Tiles[1].Name != "Alpha" || media.Tiles[2].Name != "Beta" {
		t.Fatalf("media merge wrong: %+v", media.Tiles)
	}
	if media.Tiles[1].Source != "discovery" || media.Tiles[0].Source != "" {
		t.Fatal("source flag wrong")
	}
	if groups[1].Title != "Discovered" || groups[1].Tiles[0].Name != "u" {
		t.Fatalf("fallback group wrong: %+v", groups[1])
	}
	if groups[2].Title != "Netzwerk" {
		t.Fatalf("new group wrong: %+v", groups[2])
	}
	// Original config must be untouched (Merge works on a copy of pages slice elements).
	if len(cfg.Pages[0].Groups[0].Tiles) != 1 {
		t.Fatal("Merge mutated config tiles")
	}
}

func TestHomepageDiscoveryRequiresOptIn(t *testing.T) {
	snap := docker.Snapshot{Containers: map[string]*docker.Container{
		"native":   ctr("native", map[string]string{"labdash.name": "Native"}),
		"imported": ctr("imported", map[string]string{"homepage.name": "Homepage"}),
		"both":     ctr("both", map[string]string{"labdash.name": "Native wins", "homepage.name": "Wrong name", "homepage.href": "https://fallback.example.org"}),
		"saved":    ctr("saved", map[string]string{"homepage.name": "Wrong duplicate"}),
		"other":    ctr("other", map[string]string{"custom.name": "Custom"}),
	}}
	for _, tt := range []struct {
		name       string
		discovery  string
		containers []string
	}{
		{"new config", `{"docker":true}`, []string{"both", "native"}},
		{"legacy config", `{"docker":true,"labelPrefixes":["homepage","labdash","custom"]}`, []string{"both", "native", "other"}},
		{"explicit opt-in", `{"docker":true,"homepage":true,"labelPrefixes":["homepage","labdash","custom"]}`, []string{"both", "imported", "native", "other"}},
		{"opt-in without legacy prefix", `{"docker":true,"homepage":true}`, []string{"both", "imported", "native"}},
		{"disabled discovery", `{"docker":false,"homepage":true}`, nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := config.Parse([]byte(`{"discovery":` + tt.discovery + `}`))
			if err != nil {
				t.Fatal(err)
			}
			cfg.Pages[0].Groups = []config.Group{{ID: "tools", Title: "Tools", Tiles: []config.Tile{{ID: "saved", Type: "link", Name: "My tile", Container: "saved"}}}}
			var containers []string
			pages := Merge(cfg, snap)
			for _, group := range pages[0].Groups {
				for _, tile := range group.Tiles {
					if tile.Source == "discovery" {
						containers = append(containers, tile.Container)
						if tile.Container == "both" && (tile.Name != "Native wins" || tile.Href != "") {
							t.Fatalf("unexpected mix of label sources: %+v", tile)
						}
					}
				}
			}
			slices.Sort(containers)
			if !slices.Equal(containers, tt.containers) {
				t.Fatalf("discovered %v, want %v", containers, tt.containers)
			}
			if tile := pages[0].Groups[0].Tiles[0]; tile.Name != "My tile" || tile.Container != "saved" {
				t.Fatalf("saved tile changed: %+v", tile)
			}
		})
	}
}

func TestFromLabelsDropsScriptLinks(t *testing.T) {
	d, ok := FromLabels(ctr("evil", map[string]string{
		"labdash.name": "Evil",
		"labdash.href": "javascript:alert(1)",
	}), []string{"labdash"})
	if !ok || d.Tile.Href != "" || d.Tile.Name != "Evil" {
		t.Fatalf("script link must be dropped but the tile kept: %+v", d)
	}
}
