package offlinebundle

import (
	"archive/zip"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shichao402/relkit/internal/vnext/bootstrap"
	"github.com/shichao402/relkit/internal/vnext/catalog"
	"github.com/shichao402/relkit/internal/vnext/document"
	"github.com/shichao402/relkit/internal/vnext/materialize"
	"github.com/shichao402/relkit/internal/vnext/operator"
	"github.com/shichao402/relkit/internal/vnext/project"
	"github.com/shichao402/relkit/internal/vnext/resolution"
	"github.com/shichao402/relkit/internal/vnext/trust"
)

func TestCompleteBundleVerifiesInstallsAndMaterializesOffline(t *testing.T) {
	fixture := newBundleFixture(t)
	output := filepath.Join(fixture.root, "bundle")
	manifest, err := Create(CreateOptions{
		ResolutionBundle: fixture.bundle,
		Policy: resolution.BundlePolicy{
			DistributionRootFingerprint: fixture.project.TrustFingerprint,
			OperatorDeploymentID:        fixture.operator.DeploymentID,
			Identity:                    resolution.IdentityProduction,
		},
		Assets:          fixture.assets,
		OutputDirectory: output,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Files) != 4 {
		t.Fatalf("files=%d", len(manifest.Files))
	}
	projectRoot := filepath.Join(fixture.root, "checkout")
	if err := os.MkdirAll(projectRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	projectPath := filepath.Join(projectRoot, "relkit.project.json")
	if err := document.Write(projectPath, fixture.project); err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(fixture.root, "home")
	resolved, err := Install(InstallOptions{
		Directory: output, ProjectPath: projectPath, ProjectRoot: projectRoot,
		RelkitHome: home, Target: "linux-amd64",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Digest != fixture.bundle.Resolution.Digest {
		t.Fatalf("installed resolution=%s", resolved.Digest)
	}
	bootstrapDocument, err := bootstrap.Load(bootstrap.Path(home, fixture.project.OperatorRef))
	if err != nil {
		t.Fatal(err)
	}
	if len(bootstrapDocument.AssetRoots) != 1 || bootstrapDocument.AssetRoots[0] != output {
		t.Fatalf("bootstrap=%+v", bootstrapDocument)
	}
	if _, err := materialize.Run(context.Background(), *resolved, materialize.Options{
		ProjectRoot: projectRoot,
		Target:      "linux-amd64",
	}); err != nil {
		t.Fatalf("offline materialize: %v", err)
	}
	if _, err := os.Stat(filepath.Join(projectRoot, ".relkit", "components", "bin", "relkit")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(projectRoot, ".relkit", "bootstrap", "relkit-ops")); err != nil {
		t.Fatal(err)
	}
}

func TestCandidateBundleInstallRequiresExplicitCandidateRoot(t *testing.T) {
	fixture := newBundleFixture(t)
	publicKey := ed25519.NewKeyFromSeed(fixture.seed).Public().(ed25519.PublicKey)
	candidateFingerprint := fixture.project.TrustFingerprint

	fixture.bundle.Catalog.Distributions[0].State = catalog.StateCandidate
	fixture.bundle.Catalog.Signatures = nil
	if err := fixture.bundle.Catalog.Sign("candidate", fixture.seed); err != nil {
		t.Fatal(err)
	}
	fixture.bundle.Operator.Signatures = nil
	if err := fixture.bundle.Operator.Sign("candidate", fixture.seed); err != nil {
		t.Fatal(err)
	}
	resolved, err := resolution.Resolve(resolution.Request{
		Project:                  fixture.project,
		Operator:                 fixture.bundle.Operator,
		Catalog:                  fixture.bundle.Catalog,
		TrustedCandidateKeys:     map[string]ed25519.PublicKey{"candidate": publicKey},
		CandidateRootFingerprint: candidateFingerprint,
		Identity:                 resolution.IdentityCandidate,
		CandidateDigest:          fixture.bundle.Catalog.Distributions[0].Digest,
	})
	if err != nil {
		t.Fatal(err)
	}
	fixture.bundle.CandidateRoots = resolution.PublicKeys(map[string]ed25519.PublicKey{"candidate": publicKey})
	fixture.bundle.Resolution = *resolved

	output := filepath.Join(fixture.root, "candidate-bundle")
	if _, err := Create(CreateOptions{
		ResolutionBundle: fixture.bundle,
		Policy: resolution.BundlePolicy{
			DistributionRootFingerprint: fixture.project.TrustFingerprint,
			CandidateRootFingerprint:    candidateFingerprint,
			OperatorDeploymentID:        fixture.operator.DeploymentID,
			Identity:                    resolution.IdentityCandidate,
		},
		Assets: fixture.assets, OutputDirectory: output,
	}); err != nil {
		t.Fatal(err)
	}
	projectRoot := filepath.Join(fixture.root, "candidate-checkout")
	if err := os.MkdirAll(projectRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	projectPath := filepath.Join(projectRoot, "relkit.project.json")
	if err := document.Write(projectPath, fixture.project); err != nil {
		t.Fatal(err)
	}
	options := InstallOptions{
		Directory: output, ProjectPath: projectPath, ProjectRoot: projectRoot,
		RelkitHome: filepath.Join(fixture.root, "candidate-home"), Target: "linux-amd64",
	}
	if _, err := Install(options); err == nil || !strings.Contains(err.Error(), "Candidate Root fingerprint") {
		t.Fatalf("candidate install without explicit root = %v", err)
	}
	options.CandidateRootFingerprint = candidateFingerprint
	if _, err := Install(options); err != nil {
		t.Fatalf("candidate install: %v", err)
	}
	pinnedRoot, err := os.ReadFile(filepath.Join(projectRoot, ".relkit", "candidate-root-fingerprint"))
	if err != nil {
		t.Fatal(err)
	}
	if string(pinnedRoot) != candidateFingerprint+"\n" {
		t.Fatalf("pinned Candidate Root = %q", pinnedRoot)
	}
}

func TestBundleRejectsMissingAndTamperedAssets(t *testing.T) {
	fixture := newBundleFixture(t)
	missing := map[string]string{}
	for digest, path := range fixture.assets {
		missing[digest] = path
		delete(missing, digest)
		break
	}
	if _, err := Create(CreateOptions{
		ResolutionBundle: fixture.bundle,
		Policy: resolution.BundlePolicy{
			DistributionRootFingerprint: fixture.project.TrustFingerprint,
			Identity:                    resolution.IdentityProduction,
		},
		Assets: missing, OutputDirectory: filepath.Join(fixture.root, "missing"),
	}); err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("unexpected missing result: %v", err)
	}
	output := filepath.Join(fixture.root, "complete")
	manifest, err := Create(CreateOptions{
		ResolutionBundle: fixture.bundle,
		Policy: resolution.BundlePolicy{
			DistributionRootFingerprint: fixture.project.TrustFingerprint,
			Identity:                    resolution.IdentityProduction,
		},
		Assets: fixture.assets, OutputDirectory: output,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(BlobPath(output, manifest.Files[0].Digest), []byte("tampered"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Verify(output, resolution.BundlePolicy{
		DistributionRootFingerprint: fixture.project.TrustFingerprint,
		Identity:                    resolution.IdentityProduction,
	}); err == nil || !strings.Contains(err.Error(), "size") {
		t.Fatalf("unexpected tamper result: %v", err)
	}
}

type bundleFixture struct {
	root     string
	seed     []byte
	project  project.Document
	operator operator.Document
	bundle   resolution.Bundle
	assets   map[string]string
}

func newBundleFixture(t *testing.T) bundleFixture {
	t.Helper()
	root := t.TempDir()
	seed := make([]byte, ed25519.SeedSize)
	for index := range seed {
		seed[index] = byte(index + 41)
	}
	publicKey := ed25519.NewKeyFromSeed(seed).Public().(ed25519.PublicKey)
	fingerprint, err := trust.Fingerprint(publicKey)
	if err != nil {
		t.Fatal(err)
	}
	cliPath := filepath.Join(root, "cli")
	opsPath := filepath.Join(root, "ops")
	updaterPath := filepath.Join(root, "updater")
	archivePath := filepath.Join(root, "bindings.zip")
	if err := os.WriteFile(cliPath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(updaterPath, []byte("updater"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(opsPath, []byte("#!/bin/sh\necho ops\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	makeZip(t, archivePath)
	cliDigest := digestFile(t, cliPath)
	opsDigest := digestFile(t, opsPath)
	updaterDigest := digestFile(t, updaterPath)
	archiveDigest := digestFile(t, archivePath)
	distributionDigest := "sha256:" + strings.Repeat("d", 64)
	distribution := catalog.Distribution{
		Version:               "v1.0.0",
		Commit:                strings.Repeat("1", 40),
		Digest:                distributionDigest,
		State:                 catalog.StateCertified,
		CertificationSequence: 1,
		Components: []catalog.Component{
			{Name: "cli", Version: "v1.0.0", Platform: "linux-amd64", SHA256: cliDigest, Source: catalog.Source{Kind: "release-asset", URL: "https://offline.invalid/relkit"}},
			{Name: "ops", Version: "v1.0.0", Platform: "linux-amd64", SHA256: opsDigest, Source: catalog.Source{Kind: "release-asset", URL: "https://offline.invalid/relkit-ops"}},
			{Name: "updater", Version: "v1.0.0", Platform: "linux-amd64", SHA256: updaterDigest, Source: catalog.Source{Kind: "release-asset", URL: "https://offline.invalid/relkit-updater"}},
			{Name: "bindings-ts", Version: "v1.0.0", Platform: "all", SHA256: archiveDigest, Source: catalog.Source{Kind: "release-asset", URL: "https://offline.invalid/bindings.zip"}},
		},
	}
	catalogDocument := catalog.Document{
		Schema: catalog.Schema, Sequence: 1,
		Distributions: []catalog.Distribution{distribution},
	}
	if err := catalogDocument.Sign("root", seed); err != nil {
		t.Fatal(err)
	}
	operatorDocument := operator.Document{
		Schema: operator.Schema, DeploymentID: "offline", SelectionPolicy: operator.SelectionExact,
		ServerRelease: operator.ServerRelease{
			Version: "v1.0.0", Commit: distribution.Commit, DistributionDigest: distributionDigest,
		},
		Sequence: 1,
	}
	if err := operatorDocument.Sign("root", seed); err != nil {
		t.Fatal(err)
	}
	product := project.Document{
		Schema: project.Schema, Product: "demo", Channels: []string{"dev"},
		OperatorRef: "offline", TrustFingerprint: fingerprint,
		Roles: []project.Role{project.RoleUpdaterProjection},
		CapabilityFingerprint: project.CapabilityFingerprint{
			OSArch: []string{"linux-amd64"}, Projections: []string{"typescript"}, Topology: "projection",
		},
	}
	resolved, err := resolution.Resolve(resolution.Request{
		Project: product, Operator: operatorDocument, Catalog: catalogDocument,
		TrustedDistributionKeys: map[string]ed25519.PublicKey{"root": publicKey},
		Identity:                resolution.IdentityProduction,
	})
	if err != nil {
		t.Fatal(err)
	}
	bundle := resolution.Bundle{
		Schema: resolution.BundleSchema, Project: product, Operator: operatorDocument,
		Catalog: catalogDocument, DistributionRoots: resolution.PublicKeys(map[string]ed25519.PublicKey{"root": publicKey}),
		Resolution: *resolved,
	}
	return bundleFixture{
		root: root, seed: seed, project: product, operator: operatorDocument, bundle: bundle,
		assets: map[string]string{
			cliDigest: cliPath, opsDigest: opsPath, updaterDigest: updaterPath, archiveDigest: archivePath,
		},
	}
}

func makeZip(t *testing.T, path string) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	entry, err := writer.Create("index.js")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte("export {};\n")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func digestFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	return "sha256:" + fmt.Sprintf("%x", sum[:])
}
