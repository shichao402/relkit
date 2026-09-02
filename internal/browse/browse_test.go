package browse

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cnb.cool/shichao402/relkit/internal/webmeta"
)

func TestApplyPublishMergesChannels(t *testing.T) {
	first := ApplyPublish(nil, &webmeta.Site{Title: "Demo", Product: "demo"}, webmeta.Latest{
		Product: "demo", Channel: "stable", Version: "1.0.0", Code: 100,
		Artifacts: []webmeta.Artifact{{ID: "win", Filename: "demo.zip", URLs: []string{"https://raw.example/demo.zip"}}},
	}, "2026-01-01T00:00:00Z")
	second := ApplyPublish(first, &webmeta.Site{Title: "Demo", Product: "demo"}, webmeta.Latest{
		Product: "demo", Channel: "dev", Version: "1.1.0", Code: 110,
		Artifacts: []webmeta.Artifact{{ID: "win", Filename: "demo-dev.zip", URLs: []string{"https://raw.example/dev.zip"}}},
	}, "2026-01-02T00:00:00Z")
	if len(second.Products) != 1 || len(second.Products[0].Channels) != 2 {
		t.Fatalf("got %+v", second.Products)
	}
	if second.Products[0].Channels[0].Name != "stable" {
		t.Fatalf("stable should sort first: %+v", second.Products[0].Channels)
	}

	indexHTML, err := RenderIndex(second)
	if err != nil {
		t.Fatal(err)
	}
	body := string(indexHTML)
	for _, want := range []string{"Demo", "stable", "1.0.0", "dev", "1.1.0", "demo.html", "https://raw.example/demo.zip"} {
		if !strings.Contains(body, want) {
			t.Errorf("index missing %q\n%s", want, body)
		}
	}
	if strings.Contains(body, ".pb") {
		t.Errorf("human index must not use .pb as navigation\n%s", body)
	}
	if strings.Contains(body, "fonts.google") || strings.Contains(body, "http://") && strings.Contains(body, "font") {
		t.Errorf("must not load external fonts\n%s", body)
	}

	productHTML, err := RenderProduct(ProductPage(second, "demo"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(productHTML), "Download") {
		t.Fatalf("product page missing download\n%s", productHTML)
	}
}

func TestHumanPagePrefersUserFacingArtifacts(t *testing.T) {
	catalog := ApplyPublish(nil, nil, webmeta.Latest{
		Product: "dec", Channel: "stable", Version: "1.0.0", Code: 1,
		Artifacts: []webmeta.Artifact{
			{ID: "runtime", Filename: "dec-server-linux-amd64", Selectors: map[string]string{"audience": "runtime"}},
			{ID: "console", Filename: "dec-console-linux-amd64.AppImage", Selectors: map[string]string{"audience": "user"}},
		},
	}, "now")
	artifacts := catalog.Products[0].Channels[0].Artifacts
	if len(artifacts) != 1 || artifacts[0].Filename != "dec-console-linux-amd64.AppImage" {
		t.Fatalf("human artifacts = %+v", artifacts)
	}
}

func TestHumanPageKeepsLegacyArtifacts(t *testing.T) {
	catalog := ApplyPublish(nil, nil, webmeta.Latest{
		Product: "old", Channel: "stable", Version: "1.0.0", Code: 1,
		Artifacts: []webmeta.Artifact{{ID: "legacy", Filename: "old.bin"}},
	}, "now")
	if got := len(catalog.Products[0].Channels[0].Artifacts); got != 1 {
		t.Fatalf("legacy artifact count = %d, want 1", got)
	}
}

func TestWriteDumpRoundTrip(t *testing.T) {
	dir := t.TempDir()
	cat := ApplyPublish(nil, nil, webmeta.Latest{
		Product: "app", Channel: "stable", Version: "2.0.0", Code: 2,
		Artifacts: []webmeta.Artifact{{Filename: "a.bin", URLs: []string{"https://x/a.bin"}}},
	}, "now")
	raw, err := MarshalCatalog(cat)
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteDump(dir, map[string][]byte{CatalogKey(): raw}); err != nil {
		t.Fatal(err)
	}
	got := ReadDumpCatalog(dir)
	if got == nil || got.Products[0].ID != "app" {
		t.Fatalf("dump catalog = %+v", got)
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
	if !strings.Contains(string(index), "SVN Auto Merge") {
		t.Fatalf("sample index missing title\n%s", index)
	}
	product, err := os.ReadFile(filepath.Join(dir, "svn-auto-merge.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(product), "Download") {
		t.Fatalf("sample product missing download\n%s", product)
	}
}
