package ops

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shichao402/relkit/internal/vnext/catalog"
	"github.com/shichao402/relkit/internal/vnext/document"
	vnextregistry "github.com/shichao402/relkit/internal/vnext/registry"
)

func TestBuildDistributionCreatesCompleteDeterministicIdentity(t *testing.T) {
	options := distributionFixture(t, "")
	value, err := BuildDistribution(options)
	if err != nil {
		t.Fatal(err)
	}
	if value.State != catalog.StateCandidate {
		t.Fatalf("state=%s, want candidate", value.State)
	}
	expected, err := value.IdentityDigest()
	if err != nil {
		t.Fatal(err)
	}
	if value.Digest != expected {
		t.Fatalf("digest=%s, want %s", value.Digest, expected)
	}
	foundUniversal := false
	foundAgent := false
	for _, component := range value.Components {
		if component.Name == "updater" && component.Platform == "darwin-universal" {
			foundUniversal = true
		}
		if component.Name == "agent" && component.Platform == "linux-amd64" {
			foundAgent = true
		}
		if !strings.HasPrefix(component.Source.URL, options.SourceBase+"/") {
			t.Fatalf("component URL is not release scoped: %s", component.Source.URL)
		}
	}
	if !foundUniversal || !foundAgent {
		t.Fatalf("complete distribution is missing universal updater or agent: %+v", value.Components)
	}
	if _, exists := value.ReleaseAssets["SHA256SUMS"]; exists {
		t.Fatal("distribution identity must not include its enclosing checksum manifest")
	}
	if _, err := os.Stat(options.Output); err != nil {
		t.Fatal(err)
	}
}

func TestBuildDistributionRejectsMissingRegistryAsset(t *testing.T) {
	options := distributionFixture(t, "relkit-updater-darwin-universal")
	if _, err := BuildDistribution(options); err == nil ||
		!strings.Contains(err.Error(), "relkit-updater-darwin-universal") {
		t.Fatalf("error=%v, want missing universal updater", err)
	}
}

func distributionFixture(t *testing.T, omit string) BuildDistributionOptions {
	t.Helper()
	root := t.TempDir()
	assets := filepath.Join(root, "assets")
	if err := os.MkdirAll(assets, 0o755); err != nil {
		t.Fatal(err)
	}
	registry, err := vnextregistry.Default()
	if err != nil {
		t.Fatal(err)
	}
	manifest := releaseManifest{
		Schema:        "relkit.release/1",
		Version:       "0.6.1+cccccccccccc",
		Commit:        strings.Repeat("c", 40),
		MinProtocol:   2,
		MaxProtocol:   2,
		MinUpdaterIPC: 3,
		MaxUpdaterIPC: 3,
		BuiltAt:       "2026-10-16T00:00:00Z",
	}
	written := map[string]bool{}
	for _, definition := range registry.Components {
		for _, target := range definition.Targets {
			name, _, err := definition.ReleaseAssetName(target)
			if err != nil {
				t.Fatal(err)
			}
			if name != omit && !written[name] {
				content := []byte(definition.Name + "/" + target + "\n")
				if err := os.WriteFile(filepath.Join(assets, name), content, 0o755); err != nil {
					t.Fatal(err)
				}
				written[name] = true
			}
			if target == "darwin-universal" {
				continue
			}
			osName, arch := "any", "any"
			if target != "all" {
				osName, arch, _ = strings.Cut(target, "-")
			}
			content, readErr := os.ReadFile(filepath.Join(assets, name))
			if readErr != nil {
				t.Fatal(readErr)
			}
			sum := sha256.Sum256(content)
			manifest.Artifacts = append(manifest.Artifacts, releaseManifestArtifact{
				Component: definition.Name,
				OS:        osName,
				Arch:      arch,
				Path:      name,
				SHA256:    fmt.Sprintf("%x", sum[:]),
			})
		}
	}
	manifestPath := filepath.Join(assets, "manifest.json")
	if err := document.Write(manifestPath, manifest); err != nil {
		t.Fatal(err)
	}
	var sums strings.Builder
	entries, err := os.ReadDir(assets)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		data, readErr := os.ReadFile(filepath.Join(assets, entry.Name()))
		if readErr != nil {
			t.Fatal(readErr)
		}
		sum := sha256.Sum256(data)
		fmt.Fprintf(&sums, "%x  %s\n", sum[:], entry.Name())
	}
	if err := os.WriteFile(filepath.Join(assets, "SHA256SUMS"), []byte(sums.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return BuildDistributionOptions{
		Version:      "v0.6.1",
		ManifestPath: manifestPath,
		Assets:       assets,
		SourceBase:   "https://github.com/example/relkit/releases/download/v0.6.1",
		Output:       filepath.Join(root, "distribution.json"),
	}
}
