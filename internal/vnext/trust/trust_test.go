package trust

import (
	"crypto/ed25519"
	"testing"
)

func TestSignVerifyAndFingerprint(t *testing.T) {
	seed := make([]byte, ed25519.SeedSize)
	for i := range seed {
		seed[i] = byte(i + 1)
	}
	publicKey := ed25519.NewKeyFromSeed(seed).Public().(ed25519.PublicKey)
	value := map[string]any{"schema": "example/1", "sequence": 2}

	signature, err := Sign(value, "distribution-2026", seed)
	if err != nil {
		t.Fatal(err)
	}
	if err := Verify(value, signature, map[string]ed25519.PublicKey{"distribution-2026": publicKey}); err != nil {
		t.Fatal(err)
	}
	value["sequence"] = 3
	if err := Verify(value, signature, map[string]ed25519.PublicKey{"distribution-2026": publicKey}); err == nil {
		t.Fatal("modified document unexpectedly verified")
	}
	fingerprint, err := Fingerprint(publicKey)
	if err != nil {
		t.Fatal(err)
	}
	if len(fingerprint) != len("sha256:")+64 {
		t.Fatalf("unexpected fingerprint %q", fingerprint)
	}
}
