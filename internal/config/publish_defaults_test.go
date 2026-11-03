package config

import (
	"strings"
	"testing"
)

func testDefaults() *PublishDefaults {
	return &PublishDefaults{
		Backends: map[string]map[string]any{
			"serve": {"type": "relkit-compatible", "baseUrl": "https://updates.example/", "tokenEnv": "RELKIT_SERVE_TOKEN"},
		},
		PublishTo: []string{"serve"},
		Directory: &PublishDirectoryProfile{PublishTo: []string{"serve"}},
	}
}

// TestPublishDefaultsDeriveProductProfile: the only per-product fact in a
// derived profile is the product id, and the result is a copy the caller may
// mutate without touching the server defaults.
func TestPublishDefaultsDeriveProductProfile(t *testing.T) {
	defaults := testDefaults()
	profile, err := defaults.ForProduct("demo")
	if err != nil {
		t.Fatal(err)
	}
	if profile.Product != "demo" || profile.PublishTo[0] != "serve" || profile.Directory.PublishTo[0] != "serve" {
		t.Fatalf("profile=%+v", profile)
	}
	profile.Backends["serve"]["baseUrl"] = "https://mutated.example/"
	profile.PublishTo[0] = "mutated"
	profile.Directory.PublishTo[0] = "mutated"
	if defaults.Backends["serve"]["baseUrl"] != "https://updates.example/" || defaults.PublishTo[0] != "serve" || defaults.Directory.PublishTo[0] != "serve" {
		t.Fatalf("derived profile aliases the defaults: %+v", defaults)
	}
}

// TestPublishDefaultsRejectProductPaths: entry URLs and services name one
// product's files, so they cannot be shared by every product.
func TestPublishDefaultsRejectProductPaths(t *testing.T) {
	defaults := testDefaults()
	defaults.Directory.EntryURLs = []string{"https://updates.example/directory/demo.pb"}
	if _, err := defaults.ForProduct("demo"); err == nil || !strings.Contains(err.Error(), "entryUrls") {
		t.Fatalf("err=%v", err)
	}
	var missing *PublishDefaults
	if _, err := missing.ForProduct("demo"); err == nil {
		t.Fatal("nil defaults must not derive a profile")
	}
}
