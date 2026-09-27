package config

import (
	"os"
	"path/filepath"
	"strings"
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
    "serve": {"type": "relkit-compatible", "baseUrl": "https://example.invalid/", "tokenEnv": "RELKIT_SERVE_TOKEN"}
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
    "serve": {"type": "relkit-compatible", "baseUrl": "https://example.invalid/", "tokenEnv": "RELKIT_SERVE_TOKEN"}
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

func TestLoadRejectsProductSiteMakers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ConfigName)
	raw := `{
  "product": "demo",
  "backends": {
    "serve": {"type": "relkit-compatible", "baseUrl": "https://example.invalid/", "tokenEnv": "RELKIT_SERVE_TOKEN"}
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
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "relkit-agent") {
		t.Fatalf("expected ownership error, got %v", err)
	}
}

func TestLoadSiteRequiresProductCopy(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ConfigName)
	raw := `{
  "product": "demo",
  "backends": {
    "serve": {"type": "relkit-compatible", "baseUrl": "https://example.invalid/", "tokenEnv": "RELKIT_SERVE_TOKEN"}
  },
  "site": {}
}`
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected empty site error")
	}
}

func TestLoadSiteMakersAlwaysRejected(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ConfigName)
	raw := `{
  "product": "demo",
  "backends": {
    "serve": {"type": "relkit-compatible", "baseUrl": "https://example.invalid/", "tokenEnv": "RELKIT_SERVE_TOKEN"}
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

func TestLoadSiteSinksAlwaysRejected(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ConfigName)
	raw := `{
  "product": "demo",
  "backends": {
    "serve": {"type": "relkit-compatible", "baseUrl": "https://example.invalid/", "tokenEnv": "RELKIT_SERVE_TOKEN"}
  },
  "site": {
    "title": "Demo",
    "sinks": [{"type": "makers", "projectId": "makers-x"}]
  }
}`
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "site.sinks") {
		t.Fatalf("expected site.sinks rejection, got %v", err)
	}
}

func TestLoadSignersIgnoresEnvironmentVariable(t *testing.T) {
	t.Setenv("RELKIT_PRIVATE_KEY", "not-a-real-seed-but-must-be-ignored")
	cfg := &Config{
		Root: t.TempDir(),
		Signing: map[string]any{
			"keyId":         "k1",
			"privateKeyEnv": "RELKIT_PRIVATE_KEY",
		},
	}
	_, err := cfg.LoadSigners()
	if err == nil {
		t.Fatal("expected error when privateKeyPath is missing")
	}
	msg := err.Error()
	if !strings.Contains(msg, "privateKeyPath") {
		t.Fatalf("error = %q, want mention of privateKeyPath", msg)
	}
	if strings.Contains(msg, "RELKIT_PRIVATE_KEY") {
		t.Fatalf("error still points at the env var: %q", msg)
	}
}

func TestSkeletonHasNoPrivateKeyEnv(t *testing.T) {
	doc := Skeleton("demo")
	signing, _ := doc["signing"].(map[string]any)
	if _, ok := signing["privateKeyEnv"]; ok {
		t.Fatalf("skeleton still has privateKeyEnv: %+v", signing)
	}
	path, _ := signing["privateKeyPath"].(string)
	if path == "" {
		t.Fatalf("skeleton privateKeyPath = %q", path)
	}
}

func TestLoadRetainVersionsRejectsNegative(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ConfigName)
	raw := `{
  "product": "demo",
  "retainVersions": -1,
  "backends": {
    "serve": {"type": "relkit-compatible", "baseUrl": "https://example.invalid/", "tokenEnv": "RELKIT_SERVE_TOKEN"}
  }
}`
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected error for negative retainVersions")
	}
}
