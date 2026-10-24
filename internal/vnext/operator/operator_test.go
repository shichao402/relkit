package operator

import (
	"crypto/ed25519"
	"strings"
	"testing"
)

func signedProfile(t *testing.T) (Document, ed25519.PublicKey) {
	t.Helper()
	seed := make([]byte, ed25519.SeedSize)
	for i := range seed {
		seed[i] = byte(100 + i)
	}
	value := Document{
		Schema:       Schema,
		DeploymentID: "prod-public",
		ServerRelease: ServerRelease{
			Version: "v1.2.3",
			Commit:  strings.Repeat("a", 40),
		},
		Endpoints: map[string]string{
			"api": "https://agent.example/api",
		},
		// A credential reference names where a client finds the token; it never
		// holds the token itself.
		SecretRefs: map[string]string{"token": "env:RELKIT_TOKEN"},
		Sequence:   4,
	}
	if err := value.Sign("server-root", seed); err != nil {
		t.Fatal(err)
	}
	return value, ed25519.NewKeyFromSeed(seed).Public().(ed25519.PublicKey)
}

func TestSignedProfileDetectsMutation(t *testing.T) {
	value, publicKey := signedProfile(t)
	trusted := map[string]ed25519.PublicKey{"server-root": publicKey}
	if err := value.Verify(trusted); err != nil {
		t.Fatal(err)
	}
	value.DeploymentID = "mutated"
	if err := value.Verify(trusted); err == nil {
		t.Fatal("mutated profile unexpectedly verified")
	}
}

func TestProfileDigestIncludesSignature(t *testing.T) {
	value, _ := signedProfile(t)
	digest, err := value.Digest()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(digest, "sha256:") {
		t.Fatalf("unexpected digest %q", digest)
	}
}

func TestProfileRejectsMalformedEndpoints(t *testing.T) {
	value, _ := signedProfile(t)
	value.Endpoints = map[string]string{"api": "https://user:secret@agent.example/api"}
	if err := value.Validate(); err == nil || !strings.Contains(err.Error(), "without credentials") {
		t.Fatalf("unexpected credentialed-endpoint result: %v", err)
	}

	value, _ = signedProfile(t)
	value.Endpoints = map[string]string{"api": "ftp://agent.example/api"}
	if err := value.Validate(); err == nil || !strings.Contains(err.Error(), "HTTP(S) URL") {
		t.Fatalf("unexpected scheme result: %v", err)
	}
}

func TestProfileRequiresDeploymentFacts(t *testing.T) {
	value, _ := signedProfile(t)
	value.DeploymentID = ""
	if err := value.Validate(); err == nil || !strings.Contains(err.Error(), "deploymentId") {
		t.Fatalf("expected a deploymentId rejection, got %v", err)
	}

	value, _ = signedProfile(t)
	value.ServerRelease.Commit = ""
	if err := value.Validate(); err == nil || !strings.Contains(err.Error(), "serverRelease") {
		t.Fatalf("expected a server release rejection, got %v", err)
	}

	value, _ = signedProfile(t)
	value.Sequence = 0
	if err := value.Validate(); err == nil || !strings.Contains(err.Error(), "sequence") {
		t.Fatalf("expected a sequence rejection, got %v", err)
	}

	value, _ = signedProfile(t)
	value.SecretRefs = map[string]string{"token": ""}
	if err := value.Validate(); err == nil || !strings.Contains(err.Error(), "secretRefs") {
		t.Fatalf("expected a secret reference rejection, got %v", err)
	}
}
