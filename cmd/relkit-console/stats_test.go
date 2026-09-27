package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDownloadStatsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := defaultStatsPath(dir)
	first := newDownloadStats(path, dir)
	first.since = time.Date(2026, 8, 25, 16, 11, 0, 0, time.Local)
	first.record("artifact/app/1.0.0/app.zip")
	first.record("artifact/app/1.0.0/app.zip")
	if err := first.flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	loaded := newDownloadStats(path, dir)
	if got := loaded.count("artifact/app/1.0.0/app.zip"); got != 2 {
		t.Errorf("count = %d, want 2", got)
	}
	if loaded.startedAt() != first.startedAt() {
		t.Errorf("since = %q, want %q", loaded.startedAt(), first.startedAt())
	}
}

func TestResolveStatsPath(t *testing.T) {
	root := t.TempDir()
	if got := resolveStatsPath(root, ""); got != defaultStatsPath(root) {
		t.Errorf("default = %q", got)
	}
	abs := filepath.Join(root, "outside-stats.json")
	if got := resolveStatsPath(root, abs); got != filepath.Clean(abs) {
		t.Errorf("abs = %q, want %q", got, abs)
	}
	if got := resolveStatsPath(root, "state.json"); got != filepath.Join(root, "state.json") {
		t.Errorf("rel = %q", got)
	}
}

func TestReservedServeKey(t *testing.T) {
	for _, name := range []string{statsFileName, statsFileName + ".tmp~"} {
		if !reservedServeKey(name) {
			t.Errorf("%s should be reserved", name)
		}
	}
	if reservedServeKey("artifact/app/1.0.0/app.zip") {
		t.Error("artifact key should not be reserved")
	}
}

func TestCorruptStatsFileDoesNotWipeUntilWrite(t *testing.T) {
	dir := t.TempDir()
	path := defaultStatsPath(dir)
	if err := os.WriteFile(path, []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded := newDownloadStats(path, dir)
	if loaded.count("artifact/app/1.0.0/app.zip") != 0 {
		t.Fatal("corrupt file should load as empty")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "not json" {
		t.Fatalf("corrupt file was overwritten: %q", raw)
	}
}
