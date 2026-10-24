package config

import (
	"crypto/ed25519"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSkeletonDeclaresOneRoot(t *testing.T) {
	doc := Skeleton("demo")
	if _, ok := doc["signing"]; ok {
		t.Fatalf("skeleton still declares a signing block: %+v", doc["signing"])
	}
	if doc["rootKeyId"] == "" {
		t.Fatalf("skeleton rootKeyId = %v", doc["rootKeyId"])
	}
}

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

func TestSignersRequireResolvedRoot(t *testing.T) {
	cfg := &Config{Root: t.TempDir(), RootKeyIDValue: "server-root"}
	_, err := cfg.Signers()
	if err == nil {
		t.Fatal("expected an error when the root was never resolved")
	}
	if !strings.Contains(err.Error(), "server-root") || !strings.Contains(err.Error(), "not resolved") {
		t.Fatalf("error = %q, want it to name the unresolved root", err.Error())
	}
	if _, err := cfg.TrustedPublicKeys(); err == nil {
		t.Fatal("expected trusted keys to be unavailable without a root")
	}
}

func TestSignersComeFromTheOneRoot(t *testing.T) {
	seed := make([]byte, ed25519.SeedSize)
	for offset := range seed {
		seed[offset] = byte(offset + 11)
	}
	public := ed25519.NewKeyFromSeed(seed).Public().(ed25519.PublicKey)
	cfg := &Config{Root: t.TempDir(), RootKeyIDValue: "server-root"}
	cfg.SetRoot(&RootKey{KeyID: "server-root", Seed: seed, PublicKey: public})

	signers, err := cfg.Signers()
	if err != nil {
		t.Fatal(err)
	}
	if len(signers) != 1 || signers[0].KeyID != "server-root" {
		t.Fatalf("unexpected signers: %#v", signers)
	}
	trusted, err := cfg.TrustedPublicKeys()
	if err != nil {
		t.Fatal(err)
	}
	if len(trusted) != 1 || !trusted["server-root"].Equal(public) {
		t.Fatalf("unexpected trusted keys: %#v", trusted)
	}
}

func TestSignersRejectWrongSeedLength(t *testing.T) {
	cfg := &Config{Root: t.TempDir(), RootKeyIDValue: "server-root"}
	cfg.SetRoot(&RootKey{KeyID: "server-root", Seed: []byte{1, 2, 3}})
	if _, err := cfg.Signers(); err == nil || !strings.Contains(err.Error(), "seed bytes") {
		t.Fatalf("expected a seed length error, got %v", err)
	}
}

func TestLoadRejectsSigningBlock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ConfigName)
	raw := `{
  "product": "demo",
  "rootKeyId": "server-root",
  "signing": {"keyId": "k1", "privateKeyPath": "k1.private.pb"},
  "backends": {
    "serve": {"type": "relkit-compatible", "baseUrl": "https://example.invalid/", "tokenEnv": "RELKIT_SERVE_TOKEN"}
  }
}`
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "rootKeyId") {
		t.Fatalf("expected the signing block to be rejected, got %v", err)
	}
}
