package tools

import (
	"crypto/ed25519"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/shichao402/relkit/internal/vnext/trust"
)

func testSeed() []byte {
	seed := make([]byte, ed25519.SeedSize)
	for index := range seed {
		seed[index] = byte(index + 23)
	}
	return seed
}

func fingerprintOf(publicKey ed25519.PublicKey) (string, error) {
	return trust.Fingerprint(publicKey)
}

func TestSSHSignRoundTripsThroughOurVerifier(t *testing.T) {
	seed := testSeed()
	publicKey := ed25519.NewKeyFromSeed(seed).Public().(ed25519.PublicKey)
	message := []byte("relkit-updater payload bytes")
	armored, err := SSHSign(seed, message)
	if err != nil {
		t.Fatal(err)
	}
	encoded := Armor(armored)
	if err := VerifySSHSignature(encoded, publicKey, message); err != nil {
		t.Fatalf("verify: %v", err)
	}
	if err := VerifySSHSignature(encoded, publicKey, []byte("other bytes")); err == nil {
		t.Fatal("a signature must not verify against different bytes")
	}
	otherSeed := make([]byte, ed25519.SeedSize)
	otherPublic := ed25519.NewKeyFromSeed(otherSeed).Public().(ed25519.PublicKey)
	if err := VerifySSHSignature(encoded, otherPublic, message); err == nil {
		t.Fatal("a signature must not verify against a different key")
	}
}

// TestSSHSignIsAcceptedByOpenSSH is the load-bearing test for requirement
// 12.3: host CI verifies with the building machine's own ssh-keygen, so the
// signature this package produces must be accepted by the real tool. The test
// skips when ssh-keygen is absent rather than silently passing.
func TestSSHSignIsAcceptedByOpenSSH(t *testing.T) {
	sshKeygen, err := exec.LookPath("ssh-keygen")
	if err != nil {
		t.Skip("ssh-keygen is not installed on this machine")
	}
	seed := testSeed()
	publicKey := ed25519.NewKeyFromSeed(seed).Public().(ed25519.PublicKey)
	message := []byte("#!/bin/sh\necho relkit-updater\n")
	blob, err := SSHSign(seed, message)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	payloadPath := filepath.Join(dir, "payload")
	signaturePath := filepath.Join(dir, "payload.sig")
	allowlistPath := filepath.Join(dir, AllowlistName)
	if err := os.WriteFile(payloadPath, message, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(signaturePath, []byte(Armor(blob)), 0o644); err != nil {
		t.Fatal(err)
	}
	line := AllowedSignerLine("server-root", publicKey) + "\n"
	if err := os.WriteFile(allowlistPath, []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}

	verify := exec.Command(sshKeygen, "-Y", "verify",
		"-f", allowlistPath,
		"-I", "server-root",
		"-n", SignatureNamespace,
		"-s", signaturePath,
	)
	payloadFile, err := os.Open(payloadPath)
	if err != nil {
		t.Fatal(err)
	}
	defer payloadFile.Close()
	verify.Stdin = payloadFile
	output, err := verify.CombinedOutput()
	if err != nil {
		t.Fatalf("ssh-keygen -Y verify rejected the signature: %v\n%s", err, output)
	}
}

// TestSSHSignRejectsForeignNamespace proves the namespace really is covered by
// the signature: a blob signed for relkit-tool must not verify as another
// purpose.
func TestSSHSignRejectsWrongIdentity(t *testing.T) {
	seed := testSeed()
	publicKey := ed25519.NewKeyFromSeed(seed).Public().(ed25519.PublicKey)
	message := []byte("payload")
	blob, err := SSHSign(seed, message)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifySSHSignature(Armor(blob), publicKey, message); err != nil {
		t.Fatalf("baseline verify: %v", err)
	}
	if err := VerifySSHSignature("-----BEGIN SSH SIGNATURE-----\nAAAA\n-----END SSH SIGNATURE-----", publicKey, message); err == nil {
		t.Fatal("a malformed blob must be rejected")
	}
}

func TestManifestSignAndVerify(t *testing.T) {
	seed := testSeed()
	publicKey := ed25519.NewKeyFromSeed(seed).Public().(ed25519.PublicKey)
	encoded, err := EncodePublicKey(publicKey)
	if err != nil {
		t.Fatal(err)
	}
	fingerprint, err := fingerprintOf(publicKey)
	if err != nil {
		t.Fatal(err)
	}
	document := Document{
		Schema:          Schema,
		Version:         "0.7.0+20261022abc",
		Commit:          "abcdef",
		RootKeyID:       "server-root",
		RootPublicKey:   encoded,
		RootFingerprint: fingerprint,
		Tools: []Tool{{
			Component: "cli",
			Name:      "relkit-windows-amd64.exe", Version: "0.7.0+20261022abc", Platform: "windows-amd64",
			OS: "windows", Arch: "amd64", Size: 12, SHA256: Sum256([]byte("payload")),
			URL:          "https://updates.example/v1/tools/0.7.0/relkit-windows-amd64.exe",
			SignatureURL: "https://updates.example/v1/tools/0.7.0/relkit-windows-amd64.exe.sig",
		}},
	}
	if err := document.Sign("server-root", seed); err != nil {
		t.Fatal(err)
	}
	if err := document.Verify(map[string]ed25519.PublicKey{"server-root": publicKey}); err != nil {
		t.Fatalf("verify: %v", err)
	}
	otherSeed := make([]byte, ed25519.SeedSize)
	otherPublic := ed25519.NewKeyFromSeed(otherSeed).Public().(ed25519.PublicKey)
	if err := document.Verify(map[string]ed25519.PublicKey{"server-root": otherPublic}); err == nil {
		t.Fatal("a manifest signed by another root must be rejected")
	}
	if _, ok := document.Tool("relkit-windows-amd64.exe", "windows-amd64"); !ok {
		t.Fatal("tool lookup failed")
	}
	if _, ok := document.Tool("relkit-windows-amd64.exe", "linux-amd64"); ok {
		t.Fatal("absent tool must not be found")
	}
	// Host CI asks for a component, never for a file name.
	if _, ok := document.Component("cli", "windows-amd64"); !ok {
		t.Fatal("component lookup failed")
	}
	// The Catalog is projected from the same rows, so it must carry both the
	// registry name and the server-served URL.
	release := document.CatalogRelease()
	if release.Version != document.Version || release.Commit != document.Commit {
		t.Fatalf("catalog release=%+v", release)
	}
	if len(release.Components) != 1 || release.Components[0].Name != "cli" ||
		release.Components[0].Source.Kind != "server" {
		t.Fatalf("catalog components=%+v", release.Components)
	}
}

func TestManifestValidateRejectsMismatchedVersion(t *testing.T) {
	seed := testSeed()
	publicKey := ed25519.NewKeyFromSeed(seed).Public().(ed25519.PublicKey)
	encoded, _ := EncodePublicKey(publicKey)
	document := Document{
		Schema: Schema, Version: "1.0.0", Commit: "abc",
		RootKeyID: "server-root", RootPublicKey: encoded,
		Tools: []Tool{{
			Component: "cli",
			Name:      "relkit-windows-amd64.exe", Version: "2.0.0", Platform: "windows-amd64",
			OS: "windows", Arch: "amd64", Size: 1, SHA256: Sum256([]byte("x")),
			URL:          "https://updates.example/x",
			SignatureURL: "https://updates.example/x.sig",
		}},
	}
	if err := document.Validate(); err == nil {
		t.Fatal("a tool row from another version must be rejected")
	}
}
