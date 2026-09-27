package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// Shared fixtures for the storage-plane tests. The admin-panel half of serve's
// test support (cookies, login) stayed with relkit-console; the store's config
// struct has no panel at all.

const testToken = "s3cr3t-token"

var testPanelCookies sync.Map

func newTestConfig(t *testing.T, withToken bool) (*config, string) {
	t.Helper()
	dir := t.TempDir()

	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatalf("OpenRoot: %v", err)
	}
	t.Cleanup(func() { root.Close() })

	cfg := &config{
		root:          root,
		rootPath:      dir,
		maxUpload:     1 << 20,
		noCache:       []string{"index/"},
		immutable:     []string{"manifest/", "artifact/"},
		defaultMaxAge: 60,
		stats:         newDownloadStats(defaultStatsPath(dir), dir),
		casSecret:     bytes.Repeat([]byte{7}, 32),
		gc:            newGCState(true, time.Hour, defaultGCDebounce, defaultCASGrace),
	}
	if withToken {
		cfg.credentials = []credential{{hash: hashToken(testToken)}}
	}
	t.Cleanup(cfg.stats.stop)
	return cfg, dir
}

func newLocalServer(t testing.TB, cfg *config) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(cfg.handler())
	t.Cleanup(srv.Close)
	return srv
}

// getBody is the plain storage-plane reader: no panel cookie, no login.
func getBody(t *testing.T, rawURL string) string {
	t.Helper()
	resp, err := http.Get(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: status = %d, want 200", rawURL, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func writeFile(t *testing.T, dir, name string, content []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), content, 0o644); err != nil {
		t.Fatal(err)
	}
}
