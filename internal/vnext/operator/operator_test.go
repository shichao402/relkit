package operator

import (
	"crypto/ed25519"
	"strings"
	"testing"

	"github.com/shichao402/relkit/internal/vnext/contracts"
)

func signedProfile(t *testing.T, policy SelectionPolicy) (Document, ed25519.PublicKey) {
	t.Helper()
	seed := make([]byte, ed25519.SeedSize)
	for i := range seed {
		seed[i] = byte(100 + i)
	}
	value := Document{
		Schema:       Schema,
		DeploymentID: "prod-public",
		ServerRelease: ServerRelease{
			Version:            "v1.2.3",
			Commit:             strings.Repeat("a", 40),
			DistributionDigest: "sha256:" + strings.Repeat("b", 64),
		},
		SelectionPolicy: policy,
		Provides: []contracts.Capability{
			{Domain: contracts.DomainPublishProtocol, Peer: "cli", Min: 2, Max: 3},
			{Domain: contracts.DomainRUPSchemaSet, Peer: "updater", ID: "rup.v2"},
		},
		Sequence: 4,
	}
	if err := value.Sign("distribution-2026", seed); err != nil {
		t.Fatal(err)
	}
	return value, ed25519.NewKeyFromSeed(seed).Public().(ed25519.PublicKey)
}

func TestSignedProfileDetectsMutation(t *testing.T) {
	value, publicKey := signedProfile(t, SelectionExact)
	trusted := map[string]ed25519.PublicKey{"distribution-2026": publicKey}
	if err := value.Verify(trusted); err != nil {
		t.Fatal(err)
	}
	value.DeploymentID = "mutated"
	if err := value.Verify(trusted); err == nil {
		t.Fatal("mutated profile unexpectedly verified")
	}
}

func TestProfileDigestIncludesSignature(t *testing.T) {
	value, _ := signedProfile(t, SelectionLatestCompatible)
	digest, err := value.Digest()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(digest, "sha256:") {
		t.Fatalf("unexpected digest %q", digest)
	}
}
