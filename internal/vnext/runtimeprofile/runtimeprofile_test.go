package runtimeprofile

import (
	"crypto/ed25519"
	"testing"
)

func TestProfileRequiresExternalTrustedRoot(t *testing.T) {
	seed := make([]byte, ed25519.SeedSize)
	for i := range seed {
		seed[i] = byte(i + 31)
	}
	value := Document{
		Schema:          Schema,
		Product:         "editor",
		ProfileVersion:  1,
		AllowedChannels: []string{"stable"},
		DirectoryBootstrap: DirectoryBootstrap{
			EntryURLs: []string{"https://updates.example.invalid/directory/editor.pb"},
		},
		RUPSchemaSet: "rup.v2",
	}
	if err := value.Sign("editor-2026", seed); err != nil {
		t.Fatal(err)
	}
	publicKey := ed25519.NewKeyFromSeed(seed).Public().(ed25519.PublicKey)
	if err := value.Verify(map[string]ed25519.PublicKey{"editor-2026": publicKey}); err != nil {
		t.Fatal(err)
	}
	if err := value.Verify(nil); err == nil {
		t.Fatal("profile unexpectedly self-certified without an external trusted root")
	}
}

func TestProfileDetectsBootstrapMutation(t *testing.T) {
	seed := make([]byte, ed25519.SeedSize)
	value := Document{
		Schema:          Schema,
		Product:         "editor",
		ProfileVersion:  1,
		AllowedChannels: []string{"dev"},
		DirectoryBootstrap: DirectoryBootstrap{
			EntryURLs: []string{"https://one.example.invalid/directory.pb"},
		},
		RUPSchemaSet: "rup.v2",
	}
	if err := value.Sign("editor", seed); err != nil {
		t.Fatal(err)
	}
	value.DirectoryBootstrap.EntryURLs[0] = "https://attacker.invalid/directory.pb"
	publicKey := ed25519.NewKeyFromSeed(seed).Public().(ed25519.PublicKey)
	if err := value.Verify(map[string]ed25519.PublicKey{"editor": publicKey}); err == nil {
		t.Fatal("mutated profile unexpectedly verified")
	}
}
