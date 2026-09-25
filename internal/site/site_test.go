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

// failingSink is a declarative sink that always fails, proving a sink error
// does not record dump.sha256 (next run stays a full redeploy).
type failingSinkSpec struct{ SinkSpec }

func TestNormalizeSinksValidatesAndDefaults(t *testing.T) {
	if _, err := NormalizeSinks(nil); err != nil {
		t.Fatal(err)
	}
	// makers defaults
	specs, err := NormalizeSinks([]SinkSpec{{Type: SinkMakers, ProjectID: "p1"}})
	if err != nil {
		t.Fatal(err)
	}
	if specs[0].Region != "china" || specs[0].TokenEnv != "EDGEONE_PAGES_API_TOKEN" {
		t.Fatalf("makers defaults: %+v", specs[0])
	}
	for _, bad := range []SinkSpec{
		{Type: "makers"},
		{Type: "makers", ProjectID: "p", Region: "ap-guangzhou"},
		{Type: "backend"},
		{Type: "directory"},
		{Type: "directory", Path: "relative/path"},
		{Type: "ftp"},
	} {
		if _, err := NormalizeSinks([]SinkSpec{bad}); err == nil {
			t.Fatalf("expected error for %+v", bad)
		}
	}
	// duplicate detection
	dup := []SinkSpec{
		{Type: SinkDirectory, Path: "/srv/site"},
		{Type: SinkDirectory, Path: "/srv/site/"},
	}
	if _, err := NormalizeSinks(dup); err == nil {
		t.Fatal("expected duplicate sink error")
	}
}

func TestRebuildDeploysAllProductsAndSkipsUnchanged(t *testing.T) {
	shared := map[string][]byte{}
	putWebmeta(t, shared, "dec", "1.0.0", "dev")
	putWebmeta(t, shared, "cronkit", "2.0.0", "beta")
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
	cfg := Config{StateDir: t.TempDir(), Sinks: []SinkSpec{{Type: SinkBackend, Backend: "prod"}}}
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
	// The state-dir dump copy exists and is servable (no browse/ prefix).
	stateIndex, err := os.ReadFile(filepath.Join(cfg.StateDir, "site", "dump", "index.html"))
	if err != nil || len(stateIndex) == 0 {
		t.Fatalf("state dump index.html missing: %v", err)
	}
	firstPutCount := putCount
	changed, err = Rebuild(cfg, products, nil)
	if err != nil || changed {
		t.Fatalf("second rebuild changed=%v err=%v", changed, err)
	}
	if putCount != firstPutCount {
		t.Fatalf("unchanged rebuild wrote %d more pointers", putCount-firstPutCount)
	}

	delete(shared, webmeta.SiteKey("dec"))
	delete(shared, webmeta.LatestKey("dec", "dev"))
	changed, err = Rebuild(cfg, products, nil)
	if err == nil || !strings.Contains(err.Error(), "incomplete snapshot") {
		t.Fatalf("missing prior product changed=%v err=%v", changed, err)
	}
	if putCount != firstPutCount {
		t.Fatal("incomplete snapshot was deployed")
	}
}

func TestRebuildRequiresExplicitSinks(t *testing.T) {
	// A HostsBrowse backend alone is no longer a sink: rebuild must refuse.
	shared := map[string][]byte{}
	putWebmeta(t, shared, "dec", "1.0.0", "dev")
	oldFactory := createBackend
	createBackend = func(_ string, cfg *config.Config, _ string) (backends.Backend, error) {
		return &memoryBackend{product: cfg.Product, data: shared, puts: new(int)}, nil
	}
	t.Cleanup(func() { createBackend = oldFactory })
	dir := t.TempDir()
	profile := filepath.Join(dir, "profile.json")
	raw := map[string]any{
		"product": "dec", "signing": map[string]any{"keyId": "k1"},
		"backends":  map[string]any{"prod": map[string]any{"type": "s3-compatible"}},
		"publishTo": []string{"prod"},
	}
	data, _ := json.Marshal(raw)
	if err := os.WriteFile(profile, data, 0o644); err != nil {
		t.Fatal(err)
	}
	products := []Product{{ID: "dec", Root: dir, Profile: profile}}
	_, err := Rebuild(Config{StateDir: t.TempDir()}, products, nil)
	if err == nil || !strings.Contains(err.Error(), "no destination") {
		t.Fatalf("expected no-destination error, got %v", err)
	}
}

func TestRebuildRejectsBackendSinkForUnknownOrIncapableBackend(t *testing.T) {
	shared := map[string][]byte{}
	putWebmeta(t, shared, "dec", "1.0.0", "dev")
	putCount := new(int)
	oldFactory := createBackend
	createBackend = func(name string, cfg *config.Config, _ string) (backends.Backend, error) {
		b := &memoryBackend{product: cfg.Product, data: shared, puts: putCount}
		if name == "noserve" {
			// Pretend this backend cannot serve the browse dump.
			return &noBrowseBackend{b}, nil
		}
		return b, nil
	}
	t.Cleanup(func() { createBackend = oldFactory })
	dir := t.TempDir()
	profile := filepath.Join(dir, "profile.json")
	raw := map[string]any{
		"product": "dec", "signing": map[string]any{"keyId": "k1"},
		"backends": map[string]any{
			"prod":    map[string]any{"type": "s3-compatible"},
			"noserve": map[string]any{"type": "s3-compatible"},
		},
		"publishTo": []string{"prod", "noserve"},
	}
	data, _ := json.Marshal(raw)
	if err := os.WriteFile(profile, data, 0o644); err != nil {
		t.Fatal(err)
	}
	products := []Product{{ID: "dec", Root: dir, Profile: profile}}

	_, err := Rebuild(Config{StateDir: t.TempDir(), Sinks: []SinkSpec{{Type: SinkBackend, Backend: "missing"}}}, products, nil)
	if err == nil || !strings.Contains(err.Error(), "not defined") {
		t.Fatalf("expected undefined-backend error, got %v", err)
	}
	_, err = Rebuild(Config{StateDir: t.TempDir(), Sinks: []SinkSpec{{Type: SinkBackend, Backend: "noserve"}}}, products, nil)
	if err == nil || !strings.Contains(err.Error(), "cannot serve the browse dump") {
		t.Fatalf("expected HostsBrowse rejection, got %v", err)
	}
}

type noBrowseBackend struct{ inner *memoryBackend }

func (b *noBrowseBackend) Name() string      { return b.inner.Name() }
func (b *noBrowseBackend) Type() string      { return b.inner.Type() }
func (b *noBrowseBackend) Describe() string  { return b.inner.Describe() }
func (b *noBrowseBackend) URLsAreLive() bool { return b.inner.URLsAreLive() }
func (b *noBrowseBackend) Writable() bool    { return b.inner.Writable() }
func (b *noBrowseBackend) HostsBrowse() bool { return false }
func (b *noBrowseBackend) URLFor(k string) *string {
	return b.inner.URLFor(k)
}
func (b *noBrowseBackend) Probe(k string) (bool, *int64, string) {
	return b.inner.Probe(k)
}
func (b *noBrowseBackend) Get(k string) ([]byte, error) { return b.inner.Get(k) }
func (b *noBrowseBackend) PutArtifact(a, c string) ([]string, error) {
	return b.inner.PutArtifact(a, c)
}
func (b *noBrowseBackend) PutImmutable(d []byte, c string) ([]string, error) {
	return b.inner.PutImmutable(d, c)
}
func (b *noBrowseBackend) PutPointer(d []byte, k string) ([]string, error) {
	return b.inner.PutPointer(d, k)
}

func TestDirectorySinkAtomicReplaceAndPrefixStrip(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "site-root")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(dir, "stale.html")
	if err := os.WriteFile(stale, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	dump := map[string][]byte{
		browse.IndexKey():        []byte("<html>index</html>"),
		browse.CatalogKey():      []byte("{}"),
		browse.ProductKey("dec"): []byte("<html>dec</html>"),
	}
	if err := writeDumpDir(dir, dump); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatal("stale file survived atomic replace")
	}
	for name, body := range map[string]string{
		"index.html":   "<html>index</html>",
		"catalog.json": "{}",
		"dec.html":     "<html>dec</html>",
	} {
		got, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || string(got) != body {
			t.Fatalf("%s = %q err=%v", name, got, err)
		}
	}
	// No temp/backup dirs left behind.
	entries, err := os.ReadDir(filepath.Dir(dir))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".tmp-") || strings.Contains(entry.Name(), ".previous") {
			t.Fatalf("leftover %s", entry.Name())
		}
	}
}

func TestDirectorySinkServesFullRebuild(t *testing.T) {
	shared := map[string][]byte{}
	putWebmeta(t, shared, "dec", "1.0.0", "dev")
	oldFactory := createBackend
	createBackend = func(_ string, cfg *config.Config, _ string) (backends.Backend, error) {
		return &memoryBackend{product: cfg.Product, data: shared, puts: new(int)}, nil
	}
	t.Cleanup(func() { createBackend = oldFactory })
	dir := t.TempDir()
	profile := filepath.Join(dir, "profile.json")
	raw := map[string]any{
		"product": "dec", "signing": map[string]any{"keyId": "k1"},
		"backends":  map[string]any{"prod": map[string]any{"type": "s3-compatible"}},
		"publishTo": []string{"prod"},
	}
	data, _ := json.Marshal(raw)
	if err := os.WriteFile(profile, data, 0o644); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "public")
	products := []Product{{ID: "dec", Root: dir, Profile: profile}}
	cfg := Config{StateDir: t.TempDir(), Sinks: []SinkSpec{{Type: SinkDirectory, Path: root}}}
	changed, err := Rebuild(cfg, products, nil)
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	if _, err := os.Stat(filepath.Join(root, "dec.html")); err != nil {
		t.Fatalf("directory sink missing product page: %v", err)
	}
	// Second run: same sinks + same dump is skipped.
	changed, err = Rebuild(cfg, products, nil)
	if err != nil || changed {
		t.Fatalf("second rebuild changed=%v err=%v", changed, err)
	}
	// Changing the sink set forces a full redeploy (sink names are hashed).
	cfg.Sinks = append(cfg.Sinks, SinkSpec{Type: SinkDirectory, Path: filepath.Join(t.TempDir(), "second")})
	changed, err = Rebuild(cfg, products, nil)
	if err != nil || !changed {
		t.Fatalf("sink-set change should redeploy: changed=%v err=%v", changed, err)
	}
}

func putWebmeta(t *testing.T, data map[string][]byte, id, version, channel string) {
	t.Helper()
	siteRaw, _ := webmeta.MarshalSite(webmeta.Site{
		Product: id, Title: strings.ToUpper(id),
		UpdatedAt: "2026-09-16T00:00:00Z",
	})
	latestRaw, _ := webmeta.MarshalLatest(webmeta.Latest{
		Product: id, Channel: channel, Version: version, Code: 1,
		PublishedAt: "2026-09-16T00:00:00Z",
		Artifacts:   []webmeta.Artifact{{ID: "app", Filename: id + ".zip"}},
	})
	data[webmeta.SiteKey(id)] = siteRaw
	data[webmeta.LatestKey(id, channel)] = latestRaw
}
