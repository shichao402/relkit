package browse

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shichao402/relkit/internal/webmeta"
)

func TestBuildEmitsSPAAndDataSnapshotDeterministically(t *testing.T) {
	input := []ProductData{
		{Site: &webmeta.Site{Title: "Demo", Product: "demo", UpdatedAt: "2026-01-02T00:00:00Z"},
			Latests: []webmeta.Latest{{Product: "demo", Channel: "dev", Version: "1.1.0", Code: 110, PublishedAt: "2026-01-02T00:00:00Z", Artifacts: []webmeta.Artifact{{ID: "win", Filename: "demo-dev.zip", URLs: []string{"https://raw.example/dev.zip"}}}}},
			Channels: []webmeta.Channel{
				{Product: "demo", Channel: "dev", UpdatedAt: "2026-01-02T00:00:00Z", Latest: webmeta.ChannelEntry{Version: "1.1.0", Code: 110, ReleasedAt: "2026-01-02T00:00:00Z"}, Versions: []webmeta.ChannelEntry{{Version: "1.1.0", Code: 110, ReleasedAt: "2026-01-02T00:00:00Z"}}},
				{Product: "demo", Channel: "stable", UpdatedAt: "2026-01-01T00:00:00Z", Latest: webmeta.ChannelEntry{Version: "1.0.0", Code: 100, ReleasedAt: "2026-01-01T00:00:00Z"}, Versions: []webmeta.ChannelEntry{{Version: "1.0.0", Code: 100, ReleasedAt: "2026-01-01T00:00:00Z"}}},
			},
			Releases: []webmeta.Release{
				{Product: "demo", Channel: "dev", Version: "1.1.0", Code: 110, ReleasedAt: "2026-01-02T00:00:00Z", Artifacts: []webmeta.Artifact{{ID: "win", Filename: "demo-dev.zip", URLs: []string{"https://raw.example/dev.zip"}}}},
			}},
		{Site: &webmeta.Site{Title: "Other", Product: "other", UpdatedAt: "2026-01-03T00:00:00Z"},
			Latests: []webmeta.Latest{{Product: "other", Channel: "stable", Version: "2.0.0", Code: 200, PublishedAt: "2026-01-03T00:00:00Z", Artifacts: []webmeta.Artifact{{ID: "app", Filename: "other.zip"}}}},
			Channels: []webmeta.Channel{
				{Product: "other", Channel: "stable", UpdatedAt: "2026-01-03T00:00:00Z", Latest: webmeta.ChannelEntry{Version: "2.0.0", Code: 200, ReleasedAt: "2026-01-03T00:00:00Z"}, Versions: []webmeta.ChannelEntry{{Version: "2.0.0", Code: 200, ReleasedAt: "2026-01-03T00:00:00Z"}}},
			},
			Releases: []webmeta.Release{
				{Product: "other", Channel: "stable", Version: "2.0.0", Code: 200, ReleasedAt: "2026-01-03T00:00:00Z", Artifacts: []webmeta.Artifact{{ID: "app", Filename: "other.zip"}}},
			}},
	}
	first, err := Build(input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Build(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != len(second) {
		t.Fatalf("dump size changed: %d vs %d", len(first), len(second))
	}
	for key, body := range first {
		if string(second[key]) != string(body) {
			t.Fatalf("%s changed across identical builds", key)
		}
	}

	// SPA shell and assets ship with the dump.
	for _, key := range []string{IndexKey(), AssetKey("app.js"), AssetKey("style.css"), CatalogKey()} {
		if len(first[key]) == 0 {
			t.Fatalf("dump missing %s", key)
		}
	}
	// Data snapshot: channel and release documents copied verbatim.
	for _, key := range []string{
		webmeta.ChannelKey("demo", "stable"), webmeta.ChannelKey("demo", "dev"), webmeta.ChannelKey("other", "stable"),
		webmeta.ReleaseKey("demo", "dev", "1.1.0"), webmeta.ReleaseKey("other", "stable", "2.0.0"),
	} {
		if len(first[key]) == 0 {
			t.Fatalf("dump missing data snapshot %s", key)
		}
	}

	catalog, err := UnmarshalCatalog(first[CatalogKey()])
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Products) != 2 || len(catalog.Products[0].Channels) != 2 {
		t.Fatalf("got %+v", catalog.Products)
	}
	if catalog.Products[0].Channels[0].Name != "stable" {
		t.Fatalf("stable should sort first: %+v", catalog.Products[0].Channels)
	}

	shell := string(first[IndexKey()])
	for _, want := range []string{"assets/app.js", "assets/style.css", "id=\"view\""} {
		if !strings.Contains(shell, want) {
			t.Errorf("spa shell missing %q\n%s", want, shell)
		}
	}
}

func TestBuildSkipsProductsWithoutChannels(t *testing.T) {
	input := []ProductData{
		{Site: &webmeta.Site{Product: "empty", UpdatedAt: "2026-01-02T00:00:00Z"}},
	}
	dump, err := Build(input)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := UnmarshalCatalog(dump[CatalogKey()])
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Products) != 0 {
		t.Fatalf("expected empty catalog, got %+v", catalog.Products)
	}
	// Site document still ships verbatim even when the product has no channels.
	if len(dump[webmeta.SiteKey("empty")]) == 0 {
		t.Fatal("site document missing from data snapshot")
	}
}

func TestWriteSampleDump(t *testing.T) {
	dir := t.TempDir()
	if err := WriteSampleDump(dir); err != nil {
		t.Fatal(err)
	}
	index, err := os.ReadFile(filepath.Join(dir, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(index), "assets/app.js") {
		t.Fatalf("sample shell missing app reference\n%s", index)
	}
	for _, rel := range []string{
		"assets/app.js", "assets/style.css", "catalog.json",
		"channel/svn-auto-merge/stable.json", "channel/svn-auto-merge/dev.json",
		"site/svn-auto-merge.json", "latest/svn-auto-merge/stable.json",
		"release/svn-auto-merge/stable/0.2.0+100.json",
		"release/svn-auto-merge/dev/0.2.0+106.json",
	} {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("sample dump missing %s: %v", rel, err)
		}
	}
	catalog, err := os.ReadFile(filepath.Join(dir, "catalog.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(catalog), "SVN Auto Merge") {
		t.Fatalf("sample catalog missing title\n%s", catalog)
	}
}
