package site

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.firoyang.com/relkit/internal/backends"
	"go.firoyang.com/relkit/internal/browse"
	"go.firoyang.com/relkit/internal/config"
	"go.firoyang.com/relkit/internal/webmeta"
)

type memoryBackend struct {
	product string
	data    map[string][]byte
	puts    *int
}

func (b *memoryBackend) Name() string                        { return b.product }
func (b *memoryBackend) Type() string                        { return "memory" }
func (b *memoryBackend) Describe() string                    { return "shared-memory" }
func (b *memoryBackend) URLsAreLive() bool                   { return true }
func (b *memoryBackend) Writable() bool                      { return true }
func (b *memoryBackend) HostsBrowse() bool                   { return true }
func (b *memoryBackend) URLFor(string) *string               { return nil }
func (b *memoryBackend) Probe(string) (bool, *int64, string) { return false, nil, "" }
func (b *memoryBackend) Get(key string) ([]byte, error)      { return b.data[key], nil }
func (b *memoryBackend) PutArtifact(string, string) ([]string, error) {
	return nil, nil
}
func (b *memoryBackend) PutImmutable([]byte, string) ([]string, error) {
	return nil, nil
}
func (b *memoryBackend) PutPointer(data []byte, key string) ([]string, error) {
	*b.puts++
	b.data[key] = append([]byte(nil), data...)
	return nil, nil
}

func TestRebuildDeploysAllProductsAndSkipsUnchanged(t *testing.T) {
	shared := map[string][]byte{}
	putWebmeta(t, shared, "dec", "1.0.0")
	putWebmeta(t, shared, "cronkit", "2.0.0")
	putCount := 0
	oldFactory := createBackend
	createBackend = func(_ string, cfg *config.Config, _ string) (backends.Backend, error) {
		return &memoryBackend{product: cfg.Product, data: shared, puts: &putCount}, nil
	}
	t.Cleanup(func() { createBackend = oldFactory })

	var products []Product
	for _, id := range []string{"dec", "cronkit"} {
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
	cfg := Config{StateDir: t.TempDir()}
	changed, err := Rebuild(cfg, products, nil)
	if err != nil || !changed {
		t.Fatalf("first rebuild changed=%v err=%v", changed, err)
	}
	catalog, err := browse.UnmarshalCatalog(shared[browse.CatalogKey()])
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Products) != 2 || catalog.Products[0].ID != "cronkit" || catalog.Products[1].ID != "dec" {
		t.Fatalf("catalog=%+v", catalog.Products)
	}
	firstPutCount := putCount
	changed, err = Rebuild(cfg, products, nil)
	if err != nil || changed {
		t.Fatalf("second rebuild changed=%v err=%v", changed, err)
	}
	if putCount != firstPutCount {
		t.Fatalf("unchanged rebuild wrote %d more pointers", putCount-firstPutCount)
	}
}

func putWebmeta(t *testing.T, data map[string][]byte, id, version string) {
	t.Helper()
	siteRaw, _ := webmeta.MarshalSite(webmeta.Site{
		Product: id, Title: strings.ToUpper(id), Channels: []string{"stable"},
		UpdatedAt: "2026-09-16T00:00:00Z",
	})
	latestRaw, _ := webmeta.MarshalLatest(webmeta.Latest{
		Product: id, Channel: "stable", Version: version, Code: 1,
		PublishedAt: "2026-09-16T00:00:00Z",
		Artifacts:   []webmeta.Artifact{{ID: "app", Filename: id + ".zip"}},
	})
	data[webmeta.SiteKey(id)] = siteRaw
	data[webmeta.LatestKey(id, "stable")] = latestRaw
}
