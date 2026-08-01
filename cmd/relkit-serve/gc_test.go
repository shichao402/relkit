package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func mustEnvelope(t *testing.T, index any) []byte {
	t.Helper()
	payload, err := json.Marshal(index)
	if err != nil {
		t.Fatal(err)
	}
	env := map[string]any{
		"schema":  "rup.envelope/1",
		"payload": base64.StdEncoding.EncodeToString(payload),
		"signatures": []map[string]string{
			{"keyId": "test", "alg": "ed25519", "sig": base64.StdEncoding.EncodeToString(make([]byte, 64))},
		},
	}
	raw, err := json.Marshal(env)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func indexDoc(product, channel, version string, code int, manifestURL string) map[string]any {
	return map[string]any{
		"schema":      "rup.index/1",
		"product":     product,
		"channel":     channel,
		"sequence":    code,
		"generatedAt": "2026-08-01T00:00:00Z",
		"versions": []map[string]any{
			{
				"version": version,
				"code":    code,
				"manifest": map[string]any{
					"sha256": strings.Repeat("a", 64),
					"size":   1,
					"urls":   []string{manifestURL},
				},
			},
		},
	}
}

func manifestDoc(product, version string, code int, artifactURL string) []byte {
	raw, _ := json.Marshal(map[string]any{
		"schema":  "rup.manifest/1",
		"product": product,
		"version": version,
		"code":    code,
		"artifacts": []map[string]any{
			{
				"id":       "app",
				"filename": "app.zip",
				"size":     3,
				"sha256":   strings.Repeat("b", 64),
				"kind":     "archive",
				"selectors": map[string]string{},
				"urls":     []string{artifactURL},
			},
		},
	})
	return raw
}

func writeRelease(t *testing.T, dir, product, channel, version string, code int) {
	t.Helper()
	base := "http://example.com"
	manURL := fmt.Sprintf("%s/manifest/%s/%s.json", base, product, version)
	artURL := fmt.Sprintf("%s/artifact/%s/%s/app.zip", base, product, version)
	writeFile(t, dir, fmt.Sprintf("manifest/%s/%s.json", product, version), manifestDoc(product, version, code, artURL))
	writeFile(t, dir, fmt.Sprintf("artifact/%s/%s/app.zip", product, version), []byte("pkg"))
	writeFile(t, dir, fmt.Sprintf("index/%s/%s.json", product, channel),
		mustEnvelope(t, indexDoc(product, channel, version, code, manURL)))
}

func fileExists(dir, name string) bool {
	_, err := os.Stat(filepath.Join(dir, filepath.FromSlash(name)))
	return err == nil
}

func TestGCRemovesUnreferencedRelease(t *testing.T) {
	cfg, dir := newTestConfig(t, false)
	writeRelease(t, dir, "app", "stable", "1.0.0", 100)
	writeRelease(t, dir, "app", "stable", "2.0.0", 200) // overwrites index to 2.0.0 only

	// Recreate old objects that the overwritten index no longer references.
	writeFile(t, dir, "manifest/app/1.0.0.json", manifestDoc("app", "1.0.0", 100,
		"http://example.com/artifact/app/1.0.0/app.zip"))
	writeFile(t, dir, "artifact/app/1.0.0/app.zip", []byte("old"))

	result, err := cfg.gcOnce()
	if err != nil {
		t.Fatalf("gcOnce: %v", err)
	}
	if result.filesRemoved < 2 {
		t.Fatalf("filesRemoved = %d, want at least manifest+artifact", result.filesRemoved)
	}
	if !fileExists(dir, "manifest/app/2.0.0.json") || !fileExists(dir, "artifact/app/2.0.0/app.zip") {
		t.Fatal("live release was deleted")
	}
	if fileExists(dir, "manifest/app/1.0.0.json") || fileExists(dir, "artifact/app/1.0.0/app.zip") {
		t.Fatal("orphan release was kept")
	}
	if !fileExists(dir, "index/app/stable.json") {
		t.Fatal("index was deleted")
	}
}

func TestGCKeepsUnionAcrossChannels(t *testing.T) {
	cfg, dir := newTestConfig(t, false)
	writeRelease(t, dir, "app", "stable", "2.0.0", 200)
	writeRelease(t, dir, "app", "beta", "1.5.0", 150)

	result, err := cfg.gcOnce()
	if err != nil {
		t.Fatalf("gcOnce: %v", err)
	}
	if result.filesRemoved != 0 {
		t.Fatalf("filesRemoved = %d, want 0", result.filesRemoved)
	}
	for _, name := range []string{
		"manifest/app/2.0.0.json",
		"artifact/app/2.0.0/app.zip",
		"manifest/app/1.5.0.json",
		"artifact/app/1.5.0/app.zip",
	} {
		if !fileExists(dir, name) {
			t.Fatalf("missing live object %s", name)
		}
	}
}

func TestGCAbortsWithoutIndex(t *testing.T) {
	cfg, dir := newTestConfig(t, false)
	writeFile(t, dir, "manifest/app/1.0.0.json", []byte(`{}`))
	writeFile(t, dir, "artifact/app/1.0.0/app.zip", []byte("x"))

	if _, err := cfg.gcOnce(); err == nil {
		t.Fatal("expected abort with no index")
	}
	if !fileExists(dir, "manifest/app/1.0.0.json") || !fileExists(dir, "artifact/app/1.0.0/app.zip") {
		t.Fatal("objects were deleted despite abort")
	}
}

func TestGCAbortsOnBadEnvelope(t *testing.T) {
	cfg, dir := newTestConfig(t, false)
	writeFile(t, dir, "index/app/stable.json", []byte(`{"schema":"nope"}`))
	writeFile(t, dir, "manifest/app/1.0.0.json", []byte(`{}`))
	writeFile(t, dir, "artifact/app/1.0.0/app.zip", []byte("x"))

	if _, err := cfg.gcOnce(); err == nil {
		t.Fatal("expected abort on bad envelope")
	}
	if !fileExists(dir, "manifest/app/1.0.0.json") {
		t.Fatal("manifest deleted after abort")
	}
}

func TestGCAbortsWhenReferencedManifestMissing(t *testing.T) {
	cfg, dir := newTestConfig(t, false)
	writeFile(t, dir, "index/app/stable.json", mustEnvelope(t, indexDoc("app", "stable", "1.0.0", 100,
		"http://example.com/manifest/app/1.0.0.json")))
	writeFile(t, dir, "artifact/app/1.0.0/app.zip", []byte("x"))

	if _, err := cfg.gcOnce(); err == nil {
		t.Fatal("expected abort when referenced manifest is missing")
	}
	if !fileExists(dir, "artifact/app/1.0.0/app.zip") {
		t.Fatal("artifact deleted after abort")
	}
}

func TestLocalKeyFromURL(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"https://cdn.example.com/manifest/app/1.json", "manifest/app/1.json", true},
		{"http://x/artifact/app/1/a.zip", "artifact/app/1/a.zip", true},
		{"https://cdn.example.com/other/x", "", false},
		{"not a url", "", false},
	}
	for _, tc := range cases {
		got, ok := localKeyFromURL(tc.in)
		if ok != tc.ok || got != tc.want {
			t.Errorf("localKeyFromURL(%q) = %q,%v; want %q,%v", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

func TestIndexPutSchedulesGC(t *testing.T) {
	cfg, dir := newTestConfig(t, true)
	cfg.gc = newGCState(true, time.Hour, 20*time.Millisecond)

	writeRelease(t, dir, "app", "stable", "1.0.0", 100)
	// Leave an orphan that the upcoming index will not reference.
	writeFile(t, dir, "manifest/app/0.9.0.json", manifestDoc("app", "0.9.0", 90,
		"http://example.com/artifact/app/0.9.0/app.zip"))
	writeFile(t, dir, "artifact/app/0.9.0/app.zip", []byte("orphan"))

	srv := newLocalServer(t, cfg)
	t.Cleanup(srv.Close)

	body := mustEnvelope(t, indexDoc("app", "stable", "1.0.0", 100,
		"http://example.com/manifest/app/1.0.0.json"))
	req, _ := http.NewRequest("PUT", srv.URL+"/index/app/stable.json", strings.NewReader(string(body)))
	req.Header.Set("Authorization", "Bearer "+testToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d", resp.StatusCode)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if !fileExists(dir, "manifest/app/0.9.0.json") && !fileExists(dir, "artifact/app/0.9.0/app.zip") {
			if !fileExists(dir, "manifest/app/1.0.0.json") {
				t.Fatal("live manifest was removed")
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("timed out waiting for scheduled GC to remove orphans")
}

func TestSkeletonIncludesGC(t *testing.T) {
	raw := Skeleton("/srv/releases")
	var cfg FileConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.GC == nil || cfg.GC.Enabled == nil || !*cfg.GC.Enabled || cfg.GC.Interval != "1h" {
		t.Fatalf("skeleton gc = %+v", cfg.GC)
	}
}
