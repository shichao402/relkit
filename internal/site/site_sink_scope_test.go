package site

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shichao402/relkit/internal/backends"
	"github.com/shichao402/relkit/internal/config"
)

// writeProfile writes a machine publish profile for one product.
func writeProfile(t *testing.T, dir, product string, backends map[string]any) string {
	t.Helper()
	names := make([]string, 0, len(backends))
	for name := range backends {
		names = append(names, name)
	}
	raw := map[string]any{
		"product":   product,
		"signing":   map[string]any{"keyId": "k1"},
		"backends":  backends,
		"publishTo": names,
	}
	data, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, product+".json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestSinkIgnoresBackendDeclaredOnlyByUnservedProduct is the 2026-10-21
// regression: a product that is registered with the agent but has never
// published a site document must not be able to make the shared sink backend
// ambiguous for the products that ARE served. Before the fix, one staging
// test product pointing "serve" at a different data plane aborted every
// rebuild, which silently froze the public site for two weeks.
func TestSinkIgnoresBackendDeclaredOnlyByUnservedProduct(t *testing.T) {
	shared := map[string][]byte{}
	putWebmeta(t, shared, "dec", "1.0.0", "stable")

	type plane struct {
		product string
		baseURL string
	}
	var seen []plane

	oldFactory := createBackend
	createBackend = func(name string, cfg *config.Config, _ string) (backends.Backend, error) {
		baseURL, _ := cfg.Backends[name]["baseUrl"].(string)
		seen = append(seen, plane{product: cfg.Product, baseURL: baseURL})
		return &planeBackend{
			memoryBackend: &memoryBackend{product: cfg.Product, data: shared, puts: new(int)},
			baseURL:       baseURL,
		}, nil
	}
	t.Cleanup(func() { createBackend = oldFactory })

	dir := t.TempDir()
	servedProfile := writeProfile(t, dir, "dec", map[string]any{
		"serve": map[string]any{"type": "relkit-compatible", "baseUrl": "https://update-internal.example.com/"},
	})
	// A registered but never-published product pointing at a different plane.
	strayProfile := writeProfile(t, dir, "sim-s1", map[string]any{
		"serve": map[string]any{"type": "relkit-compatible", "baseUrl": "https://update-staging.example.com/"},
	})

	products := []Product{
		{ID: "dec", Root: dir, Profile: servedProfile},
		{ID: "sim-s1", Root: dir, Profile: strayProfile},
	}
	cfg := Config{
		StateDir: t.TempDir(),
		Sinks:    []SinkSpec{{Type: SinkBackend, Backend: "serve"}},
	}
	var log []string
	changed, err := Rebuild(cfg, products, func(line string) { log = append(log, line) })
	if err != nil {
		t.Fatalf("rebuild failed because an unserved product declared the sink backend: %v", err)
	}
	if !changed {
		t.Fatal("rebuild reported no change on first deploy")
	}
	if !containsLine(log, "skip sim-s1 (no site document)") {
		t.Fatalf("expected sim-s1 to be skipped, log=%v", log)
	}
	if !containsLine(log, "deploying backend:serve") {
		t.Fatalf("expected the served product to deploy, log=%v", log)
	}
	// The dump must come from the served product's plane, never the stray one.
	if len(seen) < 2 {
		t.Fatalf("expected both profiles to be probed, saw %v", seen)
	}
	if _, ok := shared["browse/catalog.json"]; !ok {
		t.Fatalf("catalog not written to the served plane; keys=%v", keysOf(shared))
	}
}

// TestSinkAmbiguityNamesTheOffendingProducts keeps the genuine conflict
// diagnosable: when two SERVED products really disagree about a sink backend,
// the error must name both products and both data planes instead of the old
// anonymous "products define it as different data planes".
func TestSinkAmbiguityNamesTheOffendingProducts(t *testing.T) {
	shared := map[string][]byte{}
	putWebmeta(t, shared, "dec", "1.0.0", "stable")
	putWebmeta(t, shared, "cronkit", "1.0.0", "stable")

	oldFactory := createBackend
	createBackend = func(name string, cfg *config.Config, _ string) (backends.Backend, error) {
		baseURL, _ := cfg.Backends[name]["baseUrl"].(string)
		return &planeBackend{
			memoryBackend: &memoryBackend{product: cfg.Product, data: shared, puts: new(int)},
			baseURL:       baseURL,
		}, nil
	}
	t.Cleanup(func() { createBackend = oldFactory })

	dir := t.TempDir()
	firstProfile := writeProfile(t, dir, "dec", map[string]any{
		"serve": map[string]any{"type": "relkit-compatible", "baseUrl": "https://update-internal.example.com/"},
	})
	secondProfile := writeProfile(t, dir, "cronkit", map[string]any{
		"serve": map[string]any{"type": "relkit-compatible", "baseUrl": "https://other-internal.example.com/"},
	})

	products := []Product{
		{ID: "dec", Root: dir, Profile: firstProfile},
		{ID: "cronkit", Root: dir, Profile: secondProfile},
	}
	cfg := Config{
		StateDir: t.TempDir(),
		Sinks:    []SinkSpec{{Type: SinkBackend, Backend: "serve"}},
	}
	_, err := Rebuild(cfg, products, nil)
	if err == nil {
		t.Fatal("expected an ambiguity error")
	}
	message := err.Error()
	for _, want := range []string{"ambiguous", "dec", "cronkit", "update-internal.example.com", "other-internal.example.com"} {
		if !strings.Contains(message, want) {
			t.Fatalf("ambiguity error %q does not mention %q", message, want)
		}
	}
}

// planeBackend reports its data plane in Describe(), exactly like the real
// relkit-compatible backend, which is what makes two products pointing at
// different planes register as a conflict.
type planeBackend struct {
	*memoryBackend
	baseURL string
}

func (b *planeBackend) Describe() string { return "relkit-compatible " + b.baseURL }

func containsLine(lines []string, want string) bool {
	for _, line := range lines {
		if strings.Contains(line, want) {
			return true
		}
	}
	return false
}

func keysOf(data map[string][]byte) []string {
	keys := make([]string, 0, len(data))
	for key := range data {
		keys = append(keys, key)
	}
	return keys
}
