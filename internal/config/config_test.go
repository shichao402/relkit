package config

import (
	"crypto/ed25519"
	"os"
	"path/filepath"
	"strings"
	"testing"

	rupv2 "github.com/shichao402/relkit/api/rup/v2"
	"github.com/shichao402/relkit/internal/keys"
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

func TestLoadSignersSupportsRotationWindow(t *testing.T) {
	root := t.TempDir()
	var entries []any
	for index, keyID := range []string{"old", "new"} {
		seed := make([]byte, ed25519.SeedSize)
		for offset := range seed {
			seed[offset] = byte(index*40 + offset + 1)
		}
		value := keys.PrivateKeyDocument(keyID, seed)
		data, err := rupv2.MarshalPrivateKey(&value)
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(root, keyID+".private.pb")
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		entries = append(entries, map[string]any{"keyId": keyID, "privateKeyPath": path})
	}
	cfg := &Config{Root: root, Signing: map[string]any{"keys": entries}}
	signers, err := cfg.LoadSigners()
	if err != nil {
		t.Fatal(err)
	}
	if len(signers) != 2 || signers[0].KeyID != "old" || signers[1].KeyID != "new" {
		t.Fatalf("unexpected signers: %#v", signers)
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

func TestLoadVersionSyncScript(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ConfigName)
	raw := `{
  "product": "demo",
  "version": {"syncScript": "scripts/sync-version.mjs"},
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
	if cfg.Version.SyncScript != "scripts/sync-version.mjs" {
		t.Fatalf("Version.SyncScript = %q", cfg.Version.SyncScript)
	}
}

func TestLoadVersionSyncScriptRejectsEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ConfigName)
	raw := `{
  "product": "demo",
  "version": {"syncScript": "  "},
  "backends": {
    "serve": {"type": "relkit-compatible", "baseUrl": "https://example.invalid/", "tokenEnv": "RELKIT_SERVE_TOKEN"}
  }
}`
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "version.syncScript") {
		t.Fatalf("expected version.syncScript empty rejection, got %v", err)
	}
}

func TestLoadVersionSyncScriptRejectsNonString(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ConfigName)
	raw := `{
  "product": "demo",
  "version": {"syncScript": 42},
  "backends": {
    "serve": {"type": "relkit-compatible", "baseUrl": "https://example.invalid/", "tokenEnv": "RELKIT_SERVE_TOKEN"}
  }
}`
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "version.syncScript") {
		t.Fatalf("expected version.syncScript type rejection, got %v", err)
	}
}
