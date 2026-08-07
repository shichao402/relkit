package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadRetainVersions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ConfigName)
	raw := `{
  "product": "demo",
  "codeStrategy": "explicit",
  "retainVersions": 1,
  "backends": {
    "local": {"type": "local", "outputDir": "out", "baseUrl": "https://example.invalid/"}
  }
}`
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RetainVersions != 1 {
		t.Fatalf("RetainVersions = %d, want 1", cfg.RetainVersions)
	}
}

func TestLoadRetainVersionsDefaultZero(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ConfigName)
	raw := `{
  "product": "demo",
  "backends": {
    "local": {"type": "local", "outputDir": "out", "baseUrl": "https://example.invalid/"}
  }
}`
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RetainVersions != 0 {
		t.Fatalf("RetainVersions = %d, want 0", cfg.RetainVersions)
	}
}

func TestLoadRetainVersionsRejectsNegative(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ConfigName)
	raw := `{
  "product": "demo",
  "retainVersions": -1,
  "backends": {
    "local": {"type": "local", "outputDir": "out", "baseUrl": "https://example.invalid/"}
  }
}`
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected error for negative retainVersions")
	}
}
