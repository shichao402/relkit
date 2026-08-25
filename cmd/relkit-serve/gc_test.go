package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	rupv2 "cnb.cool/shichao402/relkit/api/rup/v2"
	"cnb.cool/shichao402/relkit/internal/webmeta"
)

func mustEnvelope(t *testing.T, index *rupv2.Index) []byte {
	t.Helper()
	payload, err := rupv2.MarshalIndex(index)
	if err != nil {
		t.Fatal(err)
	}
	env := &rupv2.Envelope{
		Schema:  rupv2.SchemaEnvelope,
		Payload: payload,
		Signatures: []*rupv2.Signature{
			{KeyId: "test", Alg: "ed25519", Sig: make([]byte, 64)},
		},
	}
	raw, err := rupv2.MarshalEnvelope(env)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func indexDoc(product, channel, version string, code int, manifestURL string) *rupv2.Index {
	return &rupv2.Index{
		Schema:      rupv2.SchemaIndex,
		Product:     product,
		Channel:     channel,
		Sequence:    int64(code),
		GeneratedAt: "2026-08-01T00:00:00Z",
		Versions: []*rupv2.VersionNode{
			{
				Version: version,
				Code:    int64(code),
				Manifest: &rupv2.DigestRef{
					Sha256: strings.Repeat("a", 64),
					Size:   1,
					Urls:   []string{manifestURL},
				},
			},
		},
	}
}

func manifestDoc(product, version string, code int, artifactURL string) []byte {
	raw, _ := rupv2.MarshalManifest(&rupv2.Manifest{
		Schema:  rupv2.SchemaManifest,
		Product: product,
		Version: version,
		Code:    int64(code),
		Artifacts: []*rupv2.Artifact{
			{
				Id:       "app",
				Filename: "app.zip",
				Size:     3,
				Sha256:   strings.Repeat("b", 64),
				Kind:     "archive",
				Urls:     []string{artifactURL},
			},
		},
	})
	return raw
}

func writeRelease(t *testing.T, dir, product, channel, version string, code int) {
	t.Helper()
	base := "http://example.com"
	manURL := fmt.Sprintf("%s/manifest/%s/%s.pb", base, product, version)
	artURL := fmt.Sprintf("%s/artifact/%s/%s/app.zip", base, product, version)
	writeFile(t, dir, fmt.Sprintf("manifest/%s/%s.pb", product, version), manifestDoc(product, version, code, artURL))
	writeFile(t, dir, fmt.Sprintf("artifact/%s/%s/app.zip", product, version), []byte("pkg"))
	writeFile(t, dir, fmt.Sprintf("index/%s/%s.pb", product, channel),
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
	writeFile(t, dir, "manifest/app/1.0.0.pb", manifestDoc("app", "1.0.0", 100,
		"http://example.com/artifact/app/1.0.0/app.zip"))
	writeFile(t, dir, "artifact/app/1.0.0/app.zip", []byte("old"))

	result, err := cfg.gcOnce()
	if err != nil {
		t.Fatalf("gcOnce: %v", err)
	}
	if result.filesRemoved < 2 {
		t.Fatalf("filesRemoved = %d, want at least manifest+artifact", result.filesRemoved)
	}
	if !fileExists(dir, "manifest/app/2.0.0.pb") || !fileExists(dir, "artifact/app/2.0.0/app.zip") {
		t.Fatal("live release was deleted")
	}
	if fileExists(dir, "manifest/app/1.0.0.pb") || fileExists(dir, "artifact/app/1.0.0/app.zip") {
		t.Fatal("orphan release was kept")
	}
	if !fileExists(dir, "index/app/stable.pb") {
		t.Fatal("index was deleted")
	}
}

func TestGCKeepsArtifactBehindStaleLatestPointer(t *testing.T) {
	cfg, dir := newTestConfig(t, false)
	writeRelease(t, dir, "app", "stable", "2.0.0", 200)
	writeFile(t, dir, "artifact/app/1.0.0/app.zip", []byte("old"))

	raw, err := webmeta.MarshalLatest(webmeta.Latest{
		Product: "app", Channel: "stable", Version: "1.0.0", Code: 100,
		PublishedAt: "2026-08-01T00:00:00Z",
		Artifacts: []webmeta.Artifact{{
			ID: "app", Filename: "app.zip",
			URLs: []string{"http://example.com/artifact/app/1.0.0/app.zip"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, webmeta.LatestKey("app", "stable"), raw)

	if _, err := cfg.gcOnce(); err != nil {
		t.Fatalf("gcOnce: %v", err)
	}
	if !fileExists(dir, "artifact/app/1.0.0/app.zip") {
		t.Fatal("artifact referenced by latest pointer was deleted")
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
		"manifest/app/2.0.0.pb",
		"artifact/app/2.0.0/app.zip",
		"manifest/app/1.5.0.pb",
		"artifact/app/1.5.0/app.zip",
	} {
		if !fileExists(dir, name) {
			t.Fatalf("missing live object %s", name)
		}
	}
}

func TestGCAbortsWithoutIndex(t *testing.T) {
	cfg, dir := newTestConfig(t, false)
	writeFile(t, dir, "manifest/app/1.0.0.pb", []byte("x"))
	writeFile(t, dir, "artifact/app/1.0.0/app.zip", []byte("x"))

	if _, err := cfg.gcOnce(); err == nil {
		t.Fatal("expected abort with no index")
	}
	if !fileExists(dir, "manifest/app/1.0.0.pb") || !fileExists(dir, "artifact/app/1.0.0/app.zip") {
		t.Fatal("objects were deleted despite abort")
	}
}

func TestGCAbortsOnBadEnvelope(t *testing.T) {
	cfg, dir := newTestConfig(t, false)
	writeFile(t, dir, "index/app/stable.pb", []byte("not a protobuf"))
	writeFile(t, dir, "manifest/app/1.0.0.pb", []byte("x"))
	writeFile(t, dir, "artifact/app/1.0.0/app.zip", []byte("x"))

	if _, err := cfg.gcOnce(); err == nil {
		t.Fatal("expected abort on bad envelope")
	}
	if !fileExists(dir, "manifest/app/1.0.0.pb") {
		t.Fatal("manifest deleted after abort")
	}
}

func TestGCAbortsWhenReferencedManifestMissing(t *testing.T) {
	cfg, dir := newTestConfig(t, false)
	writeFile(t, dir, "index/app/stable.pb", mustEnvelope(t, indexDoc("app", "stable", "1.0.0", 100,
		"http://example.com/manifest/app/1.0.0.pb")))
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
		{"https://cdn.example.com/manifest/app/1.pb", "manifest/app/1.pb", true},
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
	writeFile(t, dir, "manifest/app/0.9.0.pb", manifestDoc("app", "0.9.0", 90,
		"http://example.com/artifact/app/0.9.0/app.zip"))
	writeFile(t, dir, "artifact/app/0.9.0/app.zip", []byte("orphan"))

	srv := newLocalServer(t, cfg)
	t.Cleanup(srv.Close)

	body := mustEnvelope(t, indexDoc("app", "stable", "1.0.0", 100,
		"http://example.com/manifest/app/1.0.0.pb"))
	req, _ := http.NewRequest("PUT", srv.URL+"/index/app/stable.pb", strings.NewReader(string(body)))
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
		if !fileExists(dir, "manifest/app/0.9.0.pb") && !fileExists(dir, "artifact/app/0.9.0/app.zip") {
			if !fileExists(dir, "manifest/app/1.0.0.pb") {
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
