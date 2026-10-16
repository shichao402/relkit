package ops

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestGenerateAndFingerprintKeyPair(t *testing.T) {
	root := t.TempDir()
	privatePath := filepath.Join(root, "private", "distribution.private.pb")
	publicPath := filepath.Join(root, "public", "distribution.public.pb")
	generated, err := GenerateKeyPair("distribution-root-2026", privatePath, publicPath)
	if err != nil {
		t.Fatal(err)
	}
	inspected, err := FingerprintKey(publicPath)
	if err != nil {
		t.Fatal(err)
	}
	if inspected.KeyID != generated.KeyID || inspected.Fingerprint != generated.Fingerprint {
		t.Fatalf("generated=%+v inspected=%+v", generated, inspected)
	}
	if info, err := os.Stat(privatePath); err != nil {
		t.Fatal(err)
	} else if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("private key permissions are %o, expected owner-only", info.Mode().Perm())
	}
	if _, err := GenerateKeyPair("replacement", privatePath, publicPath); err == nil {
		t.Fatal("expected existing key paths to be rejected")
	}
}
