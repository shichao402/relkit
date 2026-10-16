package catalog

import (
	"crypto/ed25519"
	"strings"
	"testing"

	"github.com/shichao402/relkit/internal/vnext/contracts"
)

const digest = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func testCatalog(t *testing.T) (Document, ed25519.PublicKey) {
	t.Helper()
	seed := make([]byte, ed25519.SeedSize)
	for i := range seed {
		seed[i] = byte(i + 7)
	}
	makeDistribution := func(version string, state State, certification int64) Distribution {
		return Distribution{
			Version:               version,
			Commit:                strings.Repeat("c", 40),
			Digest:                digest,
			State:                 state,
			CertificationSequence: certification,
			Components: []Component{
				{
					Name:     "cli",
					Version:  version,
					Platform: "darwin-arm64",
					SHA256:   digest,
					Source:   Source{Kind: "release-asset", URL: "https://example.invalid/relkit"},
				},
			},
			Provides: []contracts.Capability{
				{Domain: contracts.DomainPublishProtocol, Peer: "agent", Min: 2, Max: 3},
			},
		}
	}
	value := Document{
		Schema:   Schema,
		Sequence: 9,
		Distributions: []Distribution{
			makeDistribution("v1.3.0-rc.1", StateCertified, 7),
			makeDistribution("v1.1.9", StateCertified, 5),
			makeDistribution("v1.2.0", StateCertified, 6),
			makeDistribution("v2.0.0", StateCandidate, 0),
		},
	}
	if err := value.Sign("distribution-root", seed); err != nil {
		t.Fatal(err)
	}
	return value, ed25519.NewKeyFromSeed(seed).Public().(ed25519.PublicKey)
}

func TestProductionUsesSemVerAndExcludesPrerelease(t *testing.T) {
	value, _ := testCatalog(t)
	production, err := value.Production()
	if err != nil {
		t.Fatal(err)
	}
	if len(production) != 2 {
		t.Fatalf("got %d production distributions, want 2", len(production))
	}
	if production[0].Version != "v1.2.0" || production[1].Version != "v1.1.9" {
		t.Fatalf("unexpected production order: %v, %v", production[0].Version, production[1].Version)
	}
}

func TestCatalogSignatureDetectsMutation(t *testing.T) {
	value, publicKey := testCatalog(t)
	trusted := map[string]ed25519.PublicKey{"distribution-root": publicKey}
	if err := value.Verify(trusted); err != nil {
		t.Fatal(err)
	}
	value.Sequence++
	if err := value.Verify(trusted); err == nil {
		t.Fatal("mutated catalog unexpectedly verified")
	}
}

func TestCatalogSupportsDualSignatureRotation(t *testing.T) {
	value, publicKey := testCatalog(t)
	nextSeed := make([]byte, ed25519.SeedSize)
	for index := range nextSeed {
		nextSeed[index] = byte(index + 90)
	}
	nextPublic := ed25519.NewKeyFromSeed(nextSeed).Public().(ed25519.PublicKey)
	if err := value.AddSignature("next-root", nextSeed); err != nil {
		t.Fatal(err)
	}
	if err := value.Verify(map[string]ed25519.PublicKey{"distribution-root": publicKey}); err != nil {
		t.Fatalf("old root could not verify overlap: %v", err)
	}
	if err := value.Verify(map[string]ed25519.PublicKey{"next-root": nextPublic}); err != nil {
		t.Fatalf("new root could not verify overlap: %v", err)
	}
}

func TestSemVerPrecedence(t *testing.T) {
	cases := [][2]string{
		{"v1.0.0-alpha", "v1.0.0-alpha.1"},
		{"v1.0.0-alpha.1", "v1.0.0-alpha.beta"},
		{"v1.0.0-beta.11", "v1.0.0-rc.1"},
		{"v1.0.0-rc.1", "v1.0.0"},
		{"v1.9.9", "v2.0.0"},
	}
	for _, pair := range cases {
		got, err := CompareVersions(pair[0], pair[1])
		if err != nil {
			t.Fatal(err)
		}
		if got >= 0 {
			t.Fatalf("CompareVersions(%q, %q) = %d, want < 0", pair[0], pair[1], got)
		}
	}
}

func TestStateTransitions(t *testing.T) {
	for _, transition := range [][2]State{
		{StateCandidate, StateCertified},
		{StateCertified, StateSuperseded},
		{StateCertified, StateRevoked},
		{StateSuperseded, StateRevoked},
	} {
		if err := ValidateTransition(transition[0], transition[1]); err != nil {
			t.Fatalf("%v: %v", transition, err)
		}
	}
	if err := ValidateTransition(StateRevoked, StateCertified); err == nil {
		t.Fatal("revoked distribution unexpectedly returned to certified")
	}
}
