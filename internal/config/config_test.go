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

func TestLoadSiteMakers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ConfigName)
	raw := `{
  "product": "demo",
  "backends": {
    "local": {"type": "local", "outputDir": "out", "baseUrl": "https://example.invalid/"}
  },
  "site": {
    "title": "Demo",
    "makers": {
      "projectId": "makers-9qaqgz7dfhz8"
    }
  }
}`
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Site.Makers == nil || cfg.Site.Makers.ProjectID != "makers-9qaqgz7dfhz8" {
		t.Fatalf("makers=%+v", cfg.Site.Makers)
	}
	if cfg.Site.Makers.TokenEnv != DefaultMakersTokenEnv {
		t.Fatalf("tokenEnv=%q", cfg.Site.Makers.TokenEnv)
	}
	if cfg.Site.Makers.Region != "china" {
		t.Fatalf("region=%q", cfg.Site.Makers.Region)
	}
}

func TestLoadSiteMakersOnly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ConfigName)
	raw := `{
  "product": "demo",
  "backends": {
    "local": {"type": "local", "outputDir": "out", "baseUrl": "https://example.invalid/"}
  },
  "site": {
    "makers": {"projectId": "makers-only", "region": "global", "tokenEnv": "PAGES_TOKEN"}
  }
}`
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Site.Title != "" || cfg.Site.Makers == nil || cfg.Site.Makers.Region != "global" || cfg.Site.Makers.TokenEnv != "PAGES_TOKEN" {
		t.Fatalf("site=%+v makers=%+v", cfg.Site, cfg.Site.Makers)
	}
}

func TestLoadSiteMakersRejectsBadRegion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ConfigName)
	raw := `{
  "product": "demo",
  "backends": {
    "local": {"type": "local", "outputDir": "out", "baseUrl": "https://example.invalid/"}
  },
  "site": {
    "makers": {"projectId": "makers-x", "region": "ap-guangzhou"}
  }
}`
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected error for bad site.makers.region")
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
