package release

import (
	"crypto/ed25519"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shichao402/relkit/internal/stage"
	"github.com/shichao402/relkit/internal/vnext/artifactset"
	"github.com/shichao402/relkit/internal/vnext/catalog"
	"github.com/shichao402/relkit/internal/vnext/contracts"
	"github.com/shichao402/relkit/internal/vnext/document"
	"github.com/shichao402/relkit/internal/vnext/operator"
	"github.com/shichao402/relkit/internal/vnext/project"
	"github.com/shichao402/relkit/internal/vnext/resolution"
	"github.com/shichao402/relkit/internal/vnext/trust"
)

func TestStageCarriesImmutableResolutionProvenance(t *testing.T) {
	root := t.TempDir()
	artifactRoot := filepath.Join(root, "candidate")
	if err := os.MkdirAll(artifactRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	content := []byte("installer")
	if err := os.WriteFile(filepath.Join(artifactRoot, "demo.dmg"), content, 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(content)
	controlDigest := "sha256:" + strings.Repeat("a", 64)
	seed := make([]byte, ed25519.SeedSize)
	for index := range seed {
		seed[index] = byte(index + 20)
	}
	publicKey := ed25519.NewKeyFromSeed(seed).Public().(ed25519.PublicKey)
	rootFingerprint, err := trust.Fingerprint(publicKey)
	if err != nil {
		t.Fatal(err)
	}
	productDocument := project.Document{
		Schema:           project.Schema,
		Product:          "demo",
		Channels:         []string{"dev", "stable"},
		OperatorRef:      "prod",
		TrustFingerprint: rootFingerprint,
		Roles:            []project.Role{project.RolePublisher},
		ArtifactProducer: &project.Command{Command: []string{"fixture"}},
		CapabilityFingerprint: project.CapabilityFingerprint{
			OSArch:   []string{"darwin-arm64"},
			Topology: "producer-only",
		},
	}
	operatorDocument := operator.Document{
		Schema:       operator.Schema,
		DeploymentID: "prod",
		ServerRelease: operator.ServerRelease{
			Version:            "v1.0.0",
			Commit:             strings.Repeat("b", 40),
			DistributionDigest: controlDigest,
		},
		SelectionPolicy: operator.SelectionExact,
		Provides: []contracts.Capability{
			{Domain: contracts.DomainPublishProtocol, Peer: "agent", Min: 3, Max: 3},
		},
		Sequence: 1,
	}
	if err := operatorDocument.Sign("root", seed); err != nil {
		t.Fatal(err)
	}
	catalogDocument := catalog.Document{
		Schema:   catalog.Schema,
		Sequence: 1,
		Distributions: []catalog.Distribution{
			{
				Version:               "v1.0.0",
				Commit:                strings.Repeat("b", 40),
				Digest:                controlDigest,
				State:                 catalog.StateCertified,
				CertificationSequence: 1,
				Components: []catalog.Component{
					{Name: "cli", Version: "v1.0.0", Platform: "darwin-arm64", SHA256: controlDigest, Source: catalog.Source{Kind: "release-asset", URL: "fixture"}},
				},
				Provides: []contracts.Capability{
					{Domain: contracts.DomainArtifactSet, Peer: "producer", ID: artifactset.Schema},
				},
			},
		},
	}
	if err := catalogDocument.Sign("root", seed); err != nil {
		t.Fatal(err)
	}
	resolved, err := resolution.Resolve(resolution.Request{
		Project:                 productDocument,
		Operator:                operatorDocument,
		Catalog:                 catalogDocument,
		TrustedDistributionKeys: map[string]ed25519.PublicKey{"root": publicKey},
	})
	if err != nil {
		t.Fatal(err)
	}
	artifacts := artifactset.Document{
		Schema:           artifactset.Schema,
		ResolutionDigest: resolved.Digest,
		Product:          "demo",
		Version:          "1.2.3",
		ReleaseCode:      12,
		Channel:          "dev",
		Artifacts: []artifactset.Artifact{
			{
				ID:       "macos-installer",
				Path:     "demo.dmg",
				Kind:     artifactset.KindInstaller,
				Platform: "darwin-arm64",
				SHA256:   "sha256:" + fmt.Sprintf("%x", sum[:]),
				Size:     int64(len(content)),
			},
		},
	}
	artifactSetPath := filepath.Join(artifactRoot, ArtifactSetFile)
	bundlePath := filepath.Join(root, ResolutionBundleFile)
	bundle := resolution.Bundle{
		Schema:            resolution.BundleSchema,
		Project:           productDocument,
		Operator:          operatorDocument,
		Catalog:           catalogDocument,
		DistributionRoots: resolution.PublicKeys(map[string]ed25519.PublicKey{"root": publicKey}),
		Resolution:        *resolved,
	}
	if err := document.Write(bundlePath, bundle); err != nil {
		t.Fatal(err)
	}
	staged, err := Stage(productDocument, *resolved, artifacts, StageOptions{
		ProjectRoot:          root,
		ArtifactSetPath:      artifactSetPath,
		ResolutionBundlePath: bundlePath,
		Code:                 12,
	})
	if err != nil {
		t.Fatal(err)
	}
	if staged.ResolutionDigest != resolved.Digest ||
		staged.OperatorDeploymentId != "prod" ||
		staged.ResolutionIdentity != string(resolution.IdentityProduction) {
		t.Fatalf("staged provenance is incomplete: %#v", staged)
	}
	for _, name := range []string{"staged.pb", ResolutionFile, ResolutionBundleFile, ProjectFile, ArtifactSetFile, "artifacts/demo.dmg"} {
		if _, err := os.Stat(filepath.Join(stage.StagingDir(root, "1.2.3"), name)); err != nil {
			t.Fatalf("missing staged file %s: %v", name, err)
		}
	}
}
