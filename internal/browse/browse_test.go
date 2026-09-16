package browse

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.firoyang.com/relkit/internal/webmeta"
)

func TestBuildRendersAllProductsAndChannelsDeterministically(t *testing.T) {
	input := []ProductData{
		{Site: &webmeta.Site{Title: "Demo", Product: "demo", UpdatedAt: "2026-01-02T00:00:00Z"}, Latests: []webmeta.Latest{
			{Product: "demo", Channel: "dev", Version: "1.1.0", Code: 110, PublishedAt: "2026-01-02T00:00:00Z", Artifacts: []webmeta.Artifact{{ID: "win", Filename: "demo-dev.zip", URLs: []string{"https://raw.example/dev.zip"}}}},
			{Product: "demo", Channel: "stable", Version: "1.0.0", Code: 100, PublishedAt: "2026-01-01T00:00:00Z", Artifacts: []webmeta.Artifact{{ID: "win", Filename: "demo.zip", URLs: []string{"https://raw.example/demo.zip"}}}},
		}},
		{Site: &webmeta.Site{Title: "Other", Product: "other"}, Latests: []webmeta.Latest{
			{Product: "other", Channel: "stable", Version: "2.0.0", Code: 200, PublishedAt: "2026-01-03T00:00:00Z", Artifacts: []webmeta.Artifact{{ID: "app", Filename: "other.zip"}}},
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
	for key, body := range first {
		if string(second[key]) != string(body) {
			t.Fatalf("%s changed across identical builds", key)
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

	body := string(first[IndexKey()])
	for _, want := range []string{"Demo", "Other", "stable", "1.0.0", "dev", "1.1.0", "demo.html", "other.html"} {
		if !strings.Contains(body, want) {
			t.Errorf("index missing %q\n%s", want, body)
		}
	}
	if strings.Contains(body, "https://raw.example/") || strings.Contains(body, ">Download<") {
		t.Errorf("index must link to product pages instead of guessing a platform download\n%s", body)
	}
	if strings.Contains(body, ".pb") {
		t.Errorf("human index must not use .pb as navigation\n%s", body)
	}
	if strings.Contains(body, "fonts.google") || strings.Contains(body, "http://") && strings.Contains(body, "font") {
		t.Errorf("must not load external fonts\n%s", body)
	}

	if !strings.Contains(string(first[ProductKey("demo")]), "Download") {
		t.Fatalf("product page missing download\n%s", first[ProductKey("demo")])
	}
}

func TestHumanPagePrefersUserFacingArtifacts(t *testing.T) {
	catalog := productFromData(ProductData{Latests: []webmeta.Latest{{
		Product: "dec", Channel: "stable", Version: "1.0.0", Code: 1,
		Artifacts: []webmeta.Artifact{
			{ID: "runtime", Filename: "dec-server-linux-amd64", Selectors: map[string]string{"audience": "runtime"}},
			{ID: "console", Filename: "dec-console-linux-amd64.AppImage", Selectors: map[string]string{"audience": "user"}},
		},
	}}})
	artifacts := catalog.Channels[0].Artifacts
	if len(artifacts) != 1 || artifacts[0].Filename != "dec-console-linux-amd64.AppImage" {
		t.Fatalf("human artifacts = %+v", artifacts)
	}
}

func TestHumanPageKeepsLegacyArtifacts(t *testing.T) {
	catalog := productFromData(ProductData{Latests: []webmeta.Latest{{
		Product: "old", Channel: "stable", Version: "1.0.0", Code: 1,
		Artifacts: []webmeta.Artifact{{ID: "legacy", Filename: "old.bin"}},
	}}})
	if got := len(catalog.Channels[0].Artifacts); got != 1 {
		t.Fatalf("legacy artifact count = %d, want 1", got)
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
