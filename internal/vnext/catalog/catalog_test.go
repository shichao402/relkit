package catalog

import (
	"crypto/ed25519"
	"strings"
	"testing"

	"github.com/shichao402/relkit/internal/vnext/canonical"
)

const digest = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func testCatalog(t *testing.T) (Document, ed25519.PublicKey) {
	t.Helper()
	seed := make([]byte, ed25519.SeedSize)
	for i := range seed {
		seed[i] = byte(i + 7)
	}
	makeRelease := func(version string) Release {
		return Release{
			Version: version,
			Commit:  strings.Repeat("c", 40),
			Components: []Component{
				{
					Name:     "cli",
					Version:  version,
					Platform: "darwin-arm64",
					SHA256:   digest,
					Source:   Source{Kind: "server", URL: "https://updates.example/tools/" + version + "/cli"},
				},
			},
		}
	}
	value := Document{
		Schema:   Schema,
		Sequence: 9,
		Current:  "v1.2.0",
		Releases: []Release{
			makeRelease("v1.3.0-rc.1"),
			makeRelease("v1.1.9"),
			makeRelease("v1.2.0"),
		},
	}
	if err := value.Sign("server-root", seed); err != nil {
		t.Fatal(err)
	}
	return value, ed25519.NewKeyFromSeed(seed).Public().(ed25519.PublicKey)
}

func TestCatalogSignatureDetectsMutation(t *testing.T) {
	value, publicKey := testCatalog(t)
	trusted := map[string]ed25519.PublicKey{"server-root": publicKey}
	if err := value.Verify(trusted); err != nil {
		t.Fatal(err)
	}
	value.Sequence++
	if err := value.Verify(trusted); err == nil {
		t.Fatal("mutated catalog unexpectedly verified")
	}
}

func TestCurrentReleaseIsTheServedVersion(t *testing.T) {
	value, _ := testCatalog(t)
	release, err := value.CurrentRelease()
	if err != nil {
		t.Fatal(err)
	}
	if release.Version != "v1.2.0" {
		t.Fatalf("current release = %q", release.Version)
	}
	if len(release.Components) != 1 || release.Components[0].Source.Kind != "server" {
		t.Fatalf("components must be served by the server: %#v", release.Components)
	}
}

func TestValidateRejectsUnknownCurrentAndForeignSource(t *testing.T) {
	value, _ := testCatalog(t)
	value.Current = "v9.9.9"
	if err := value.Validate(); err == nil || !strings.Contains(err.Error(), "not one of the releases") {
		t.Fatalf("expected an unknown current rejection, got %v", err)
	}

	value, _ = testCatalog(t)
	value.Releases[0].Components[0].Source.Kind = "release-asset"
	if err := value.Validate(); err == nil || !strings.Contains(err.Error(), "served by the server") {
		t.Fatalf("expected a foreign source rejection, got %v", err)
	}
}

func TestValidateRejectsDuplicateReleaseAndComponent(t *testing.T) {
	value, _ := testCatalog(t)
	value.Releases = append(value.Releases, value.Releases[0])
	if err := value.Validate(); err == nil || !strings.Contains(err.Error(), "duplicate release version") {
		t.Fatalf("expected a duplicate release rejection, got %v", err)
	}

	value, _ = testCatalog(t)
	value.Releases[0].Components = append(value.Releases[0].Components, value.Releases[0].Components[0])
	if err := value.Validate(); err == nil || !strings.Contains(err.Error(), "duplicate component") {
		t.Fatalf("expected a duplicate component rejection, got %v", err)
	}
}

// TestCatalogHasNoLifecycleState pins the epoch: a release either is the
// version the server marks current or a historical entry, with no candidate /
// certified split and no certification sequence.
func TestCatalogHasNoLifecycleState(t *testing.T) {
	value, _ := testCatalog(t)
	unsigned, err := canonical.Bytes(value)
	if err != nil {
		t.Fatal(err)
	}
	text := string(unsigned)
	for _, gone := range []string{"state", "certificationSequence", "lifecycle", "provides", "requires"} {
		if strings.Contains(text, gone) {
			t.Fatalf("catalog still serializes %q:\n%s", gone, text)
		}
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
