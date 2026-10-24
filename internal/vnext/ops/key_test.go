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

// TestReissueKeyPairKeepsSeed pins the property the epoch rename depends on: the
// key material is untouched, so the fingerprint a host pinned in its project
// configuration keeps verifying, while the keyId in both documents changes.
func TestReissueKeyPairKeepsSeed(t *testing.T) {
	root := t.TempDir()
	oldPrivate := filepath.Join(root, "old", "candidate.private.pb")
	oldPublic := filepath.Join(root, "old", "candidate.public.pb")
	generated, err := GenerateKeyPair("relkit-candidate-2026b", oldPrivate, oldPublic)
	if err != nil {
		t.Fatal(err)
	}
	newPrivate := filepath.Join(root, "new", "server.private.pb")
	newPublic := filepath.Join(root, "new", "server.public.pb")
	renamed, err := ReissueKeyPair("relkit-candidate-2026b", "intranet-server-root", oldPrivate, newPrivate, newPublic)
	if err != nil {
		t.Fatal(err)
	}
	if renamed.KeyID != "intranet-server-root" {
		t.Fatalf("keyId is %q", renamed.KeyID)
	}
	if renamed.Fingerprint != generated.Fingerprint {
		t.Fatalf("fingerprint changed: %q -> %q; a rename must keep the key material", generated.Fingerprint, renamed.Fingerprint)
	}
	inspected, err := FingerprintKey(newPublic)
	if err != nil {
		t.Fatal(err)
	}
	if inspected.KeyID != "intranet-server-root" || inspected.Fingerprint != generated.Fingerprint {
		t.Fatalf("public document=%+v", inspected)
	}
	// The old document must still be readable; a rename never rewrites in place.
	if _, err := FingerprintKey(oldPublic); err != nil {
		t.Fatalf("old key document was disturbed: %v", err)
	}
	if _, err := ReissueKeyPair("relkit-candidate-2026b", "another-root", oldPrivate, newPrivate, newPublic); err == nil {
		t.Fatal("expected an existing destination to be rejected")
	}
	if _, err := ReissueKeyPair("wrong-key-id", "intranet-server-root", oldPrivate,
		filepath.Join(root, "third", "s.private.pb"), filepath.Join(root, "third", "s.public.pb")); err == nil {
		t.Fatal("expected a mismatched --old-key-id to be rejected")
	}
}

