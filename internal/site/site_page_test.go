package site

// The human-facing update page has never had an end-to-end assertion. Every
// existing test stops at "the dump was produced"; nothing checked that the
// page a person actually opens is complete. That is how the 2026-10 outage
// stayed invisible: site/<product>.json stopped being written, the SPA loaded
// fine, returned HTTP 200 with a valid shell, and simply listed fewer
// products. A 200 with a plausible-looking page is not evidence of a working
// page, so these tests assert on the rendered content itself.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shichao402/relkit/internal/backends"
	"github.com/shichao402/relkit/internal/browse"
	"github.com/shichao402/relkit/internal/config"
	"github.com/shichao402/relkit/internal/webmeta"
)

// buildSiteDir runs a full rebuild into a directory sink and returns the
// document root an external static host would serve.
func buildSiteDir(t *testing.T, products []Product, seed func(map[string][]byte)) string {
	t.Helper()
	shared := map[string][]byte{}
	seed(shared)
	oldFactory := createBackend
	createBackend = func(_ string, cfg *config.Config, _ string) (backends.Backend, error) {
		return &memoryBackend{product: cfg.Product, data: shared, puts: new(int)}, nil
	}
	t.Cleanup(func() { createBackend = oldFactory })

	siteDir := filepath.Join(t.TempDir(), "site-root")
	cfg := Config{StateDir: t.TempDir(), Sinks: []SinkSpec{{Type: SinkDirectory, Path: siteDir}}}
	if _, err := Rebuild(cfg, products, nil); err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	return siteDir
}

// productProfiles writes one minimal publish profile per product id.
func productProfiles(t *testing.T, ids ...string) []Product {
	t.Helper()
	products := make([]Product, 0, len(ids))
	for _, id := range ids {
		dir := t.TempDir()
		profile := filepath.Join(dir, "profile.json")
		raw := map[string]any{
			"product": id, "signing": map[string]any{"keyId": "k1"},
			"backends":  map[string]any{"prod": map[string]any{"type": "s3-compatible"}},
			"publishTo": []string{"prod"},
		}
		data, _ := json.Marshal(raw)
		if err := os.WriteFile(profile, data, 0o644); err != nil {
			t.Fatal(err)
		}
		products = append(products, Product{ID: id, Root: dir, Profile: profile})
	}
	return products
}

func readServed(t *testing.T, dir, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("served file %q missing: %v", rel, err)
	}
	return string(data)
}

// TestUpdatePageIsServableAndListsPublishedProducts is the missing acceptance
// test: open the page the way a person does and confirm the products are on it.
func TestUpdatePageIsServableAndListsPublishedProducts(t *testing.T) {
	products := productProfiles(t, "dec", "cronkit")
	dir := buildSiteDir(t, products, func(shared map[string][]byte) {
		putWebmeta(t, shared, "dec", "1.13.125", "stable")
		putWebmeta(t, shared, "cronkit", "0.1.0+36", "stable")
	})

	// 1. The document root must serve a real shell, not an empty file.
	index := readServed(t, dir, "index.html")
	if len(index) < 200 {
		t.Fatalf("index.html looks empty (%d bytes)", len(index))
	}
	if !strings.Contains(index, `id="view"`) || !strings.Contains(index, "app.js") {
		t.Fatalf("index.html is not the SPA shell:\n%s", index)
	}
	// 2. The shell references assets that must exist next to it.
	for _, asset := range []string{"assets/app.js", "assets/style.css"} {
		if body := readServed(t, dir, asset); len(body) == 0 {
			t.Fatalf("%s is empty", asset)
		}
	}

	// 3. The catalog the SPA loads first must list every published product.
	catalog, err := browse.UnmarshalCatalog([]byte(readServed(t, dir, "catalog.json")))
	if err != nil {
		t.Fatalf("catalog.json does not parse: %v", err)
	}
	if len(catalog.Products) != 2 {
		t.Fatalf("catalog lists %d product(s), want 2: %+v", len(catalog.Products), catalog.Products)
	}
	listed := map[string]browse.Product{}
	for _, product := range catalog.Products {
		listed[product.ID] = product
	}
	for _, want := range []string{"cronkit", "dec"} {
		product, ok := listed[want]
		if !ok {
			t.Fatalf("product %q missing from catalog: %+v", want, catalog.Products)
		}
		if len(product.Channels) == 0 {
			t.Fatalf("product %q has no channels on the page", want)
		}
	}
	if got := listed["dec"].Channels[0].Version; got != "1.13.125" {
		t.Fatalf("dec channel version on page = %q, want 1.13.125", got)
	}

	// 4. The data documents the SPA fetches at runtime must ship with the dump,
	//    otherwise the page renders and then fails to show any release.
	if body := readServed(t, dir, webmeta.SiteKey("dec")); !strings.Contains(body, `"product": "dec"`) {
		t.Fatalf("site document for dec is missing or wrong:\n%s", body)
	}
	if body := readServed(t, dir, webmeta.ChannelKey("dec", "stable")); !strings.Contains(body, "1.13.125") {
		t.Fatalf("channel document for dec/stable is missing the version:\n%s", body)
	}
}

// TestUpdatePageRefusesToDropAProduct: the exact failure mode of 2026-10. When
// a previously published product stops producing a site document, the page
// must not silently render a shorter catalog. A product that still publishes
// protocol documents but has no site declaration is a regression, and the
// rebuild has to fail loudly instead of serving a page that quietly omits it.
func TestUpdatePageRefusesToDropAProduct(t *testing.T) {
	products := productProfiles(t, "dec", "cronkit")
	shared := map[string][]byte{}
	putWebmeta(t, shared, "dec", "1.0.0", "stable")
	putWebmeta(t, shared, "cronkit", "2.0.0", "stable")

	oldFactory := createBackend
	createBackend = func(_ string, cfg *config.Config, _ string) (backends.Backend, error) {
		return &memoryBackend{product: cfg.Product, data: shared, puts: new(int)}, nil
	}
	t.Cleanup(func() { createBackend = oldFactory })

	cfg := Config{StateDir: t.TempDir(), Sinks: []SinkSpec{{Type: SinkDirectory, Path: filepath.Join(t.TempDir(), "root")}}}
	if _, err := Rebuild(cfg, products, nil); err != nil {
		t.Fatalf("first rebuild: %v", err)
	}

	// dec keeps publishing protocol data but its site document disappears,
	// which is precisely what a product losing its site block looks like.
	delete(shared, webmeta.SiteKey("dec"))
	changed, err := Rebuild(cfg, products, nil)
	if err == nil {
		t.Fatalf("rebuild accepted a catalog that dropped dec (changed=%v)", changed)
	}
	if !strings.Contains(err.Error(), "incomplete snapshot") && !strings.Contains(err.Error(), "dec") {
		t.Fatalf("unexpected error, want an explicit refusal naming dec: %v", err)
	}
}

// TestUpdatePageSkipsProductWithoutSiteDocument pins the intended opt-out:
// a configured product that never published a site document is skipped, not an
// error. Products that only ship protocol documents are legitimate (a managed
// component that is never offered for manual download). The distinction from
// the test above is that dec had already appeared on the page.
func TestUpdatePageSkipsProductWithoutSiteDocument(t *testing.T) {
	products := productProfiles(t, "dec", "internal-component")
	dir := buildSiteDir(t, products, func(shared map[string][]byte) {
		putWebmeta(t, shared, "dec", "1.0.0", "stable")
	})

	var log []string
	_ = log
	catalog, err := browse.UnmarshalCatalog([]byte(readServed(t, dir, "catalog.json")))
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Products) != 1 || catalog.Products[0].ID != "dec" {
		t.Fatalf("catalog=%+v, want only dec", catalog.Products)
	}
}
