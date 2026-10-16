package main

import (
	"crypto/ed25519"
	"os"
	"path/filepath"
	"strings"
	"testing"

	rupv2 "github.com/shichao402/relkit/api/rup/v2"
	"github.com/shichao402/relkit/internal/keys"
	"github.com/shichao402/relkit/internal/vnext/bootstrap"
	"github.com/shichao402/relkit/internal/vnext/catalog"
	"github.com/shichao402/relkit/internal/vnext/contracts"
	"github.com/shichao402/relkit/internal/vnext/document"
	"github.com/shichao402/relkit/internal/vnext/operator"
	"github.com/shichao402/relkit/internal/vnext/project"
	"github.com/shichao402/relkit/internal/vnext/resolution"
	"github.com/shichao402/relkit/internal/vnext/runtimeprofile"
	"github.com/shichao402/relkit/internal/vnext/trust"
)

func TestResolveCommandWritesImmutableResolution(t *testing.T) {
	t.Setenv("RELKIT_OPERATOR_PROFILE", "")
	t.Setenv("RELKIT_DISTRIBUTION_CATALOG", "")
	t.Setenv("RELKIT_DISTRIBUTION_ROOT", "")
	t.Setenv("RELKIT_OPERATOR_BOOTSTRAP", "")
	t.Setenv("RELKIT_PROJECT", "")
	root := t.TempDir()
	seed := make([]byte, ed25519.SeedSize)
	for i := range seed {
		seed[i] = byte(i + 19)
	}
	keyID := "distribution-root"
	publicKey := ed25519.NewKeyFromSeed(seed).Public().(ed25519.PublicKey)
	fingerprint, err := trust.Fingerprint(publicKey)
	if err != nil {
		t.Fatal(err)
	}
	publicDocument := keys.PublicKeyDocument(keyID, seed)
	publicBytes, err := rupv2.MarshalPublicKey(&publicDocument)
	if err != nil {
		t.Fatal(err)
	}
	publicPath := filepath.Join(root, "distribution.public.pb")
	if err := os.WriteFile(publicPath, publicBytes, 0o644); err != nil {
		t.Fatal(err)
	}

	digest := "sha256:" + strings.Repeat("a", 64)
	productDocument := project.Document{
		Schema:           project.Schema,
		Product:          "demo",
		Channels:         []string{"dev", "stable"},
		OperatorRef:      "prod",
		TrustFingerprint: fingerprint,
		Roles:            []project.Role{project.RoleUpdaterHost, project.RoleSubscriber},
		RuntimeProfileSubscriptions: []project.Subscription{
			{ProductRef: "demo", RootFingerprint: fingerprint},
		},
		CapabilityFingerprint: project.CapabilityFingerprint{
			OSArch:        []string{"darwin-arm64"},
			Facades:       []string{"rust"},
			Subscriptions: []string{"demo"},
			Topology:      "self-update",
			RequiredContracts: []contracts.Capability{
				{Domain: contracts.DomainPublishProtocol, Peer: "agent", Min: 3, Max: 3},
			},
		},
	}
	projectPath := filepath.Join(root, "relkit.project.json")
	if err := document.Write(projectPath, productDocument); err != nil {
		t.Fatal(err)
	}

	operatorDocument := operator.Document{
		Schema:       operator.Schema,
		DeploymentID: "prod",
		ServerRelease: operator.ServerRelease{
			Version:            "v1.0.0",
			Commit:             strings.Repeat("b", 40),
			DistributionDigest: digest,
		},
		SelectionPolicy: operator.SelectionExact,
		Provides: []contracts.Capability{
			{Domain: contracts.DomainPublishProtocol, Peer: "agent", Min: 2, Max: 3},
		},
		Sequence: 1,
	}
	if err := operatorDocument.Sign(keyID, seed); err != nil {
		t.Fatal(err)
	}
	operatorPath := filepath.Join(root, "operator.json")
	if err := document.Write(operatorPath, operatorDocument); err != nil {
		t.Fatal(err)
	}

	catalogDocument := catalog.Document{
		Schema:   catalog.Schema,
		Sequence: 1,
		Distributions: []catalog.Distribution{
			{
				Version:               "v1.0.0",
				Commit:                strings.Repeat("b", 40),
				Digest:                digest,
				State:                 catalog.StateCertified,
				CertificationSequence: 1,
				Components: []catalog.Component{
					{Name: "cli", Version: "v1.0.0", Platform: "darwin-arm64", SHA256: digest, Source: catalog.Source{Kind: "release-asset", URL: "https://example.invalid/cli"}},
					{Name: "updater", Version: "v1.0.0", Platform: "darwin-arm64", SHA256: digest, Source: catalog.Source{Kind: "release-asset", URL: "https://example.invalid/updater"}},
					{Name: "sdk-rust", Version: "v1.0.0", Platform: "all", SHA256: digest, Source: catalog.Source{Kind: "release-asset", URL: "https://example.invalid/sdk-rust.zip"}},
				},
				Provides: []contracts.Capability{
					{Domain: contracts.DomainPublishProtocol, Peer: "agent", Min: 2, Max: 3},
				},
			},
		},
	}
	if err := catalogDocument.Sign(keyID, seed); err != nil {
		t.Fatal(err)
	}
	catalogPath := filepath.Join(root, "catalog.json")
	if err := document.Write(catalogPath, catalogDocument); err != nil {
		t.Fatal(err)
	}
	runtimeDocument := runtimeprofile.Document{
		Schema:          runtimeprofile.Schema,
		Product:         "demo",
		ProfileVersion:  1,
		AllowedChannels: []string{"dev", "stable"},
		DirectoryBootstrap: runtimeprofile.DirectoryBootstrap{
			EntryURLs: []string{"https://updates.example.invalid/directory/demo.pb"},
		},
		RUPSchemaSet: "rup.v2",
	}
	if err := runtimeDocument.Sign(keyID, seed); err != nil {
		t.Fatal(err)
	}
	runtimePath := filepath.Join(root, "runtime-profile.json")
	if err := document.Write(runtimePath, runtimeDocument); err != nil {
		t.Fatal(err)
	}

	outputPath := filepath.Join(root, ".relkit", "resolution.json")
	if err := cmdResolve([]string{
		"--project", projectPath,
		"--operator", operatorPath,
		"--catalog", catalogPath,
		"--trust-key", publicPath,
		"--runtime-profile", "demo=" + runtimePath,
		"--runtime-profile-root", "demo=" + publicPath,
		"--out", outputPath,
	}); err != nil {
		t.Fatal(err)
	}
	value, err := resolution.Load(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	if value.SelectedDistribution.Version != "v1.0.0" {
		t.Fatalf("selected %q", value.SelectedDistribution.Version)
	}
	if err := cmdDoctor([]string{"--resolution", outputPath}); err != nil {
		t.Fatal(err)
	}

	home := filepath.Join(root, "user-config")
	if _, err := bootstrap.Write(home, bootstrap.Document{
		Schema:            bootstrap.Schema,
		OperatorRef:       "prod",
		OperatorProfile:   operatorPath,
		Catalog:           catalogPath,
		DistributionRoots: []string{publicPath},
		RuntimeProfiles: map[string]bootstrap.RuntimeProfile{
			"demo": {Profile: runtimePath, Roots: []string{publicPath}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("RELKIT_HOME", home)
	autoOutput := filepath.Join(root, "automatic", "resolution.json")
	if err := cmdResolve([]string{
		"--project", projectPath,
		"--out", autoOutput,
	}); err != nil {
		t.Fatalf("resolve through logical Operator bootstrap: %v", err)
	}
	automatic, err := resolution.Load(autoOutput)
	if err != nil {
		t.Fatal(err)
	}
	if automatic.Digest != value.Digest {
		t.Fatalf("bootstrap resolution %s differs from explicit resolution %s", automatic.Digest, value.Digest)
	}
	if err := os.RemoveAll(filepath.Join(filepath.Dir(autoOutput), "control-plane")); err != nil {
		t.Fatal(err)
	}
	if err := cmdDoctor([]string{"--project", projectPath, "--resolution", autoOutput}); err != nil {
		t.Fatalf("doctor through logical Operator bootstrap: %v", err)
	}
	t.Setenv("RELKIT_PROJECT", projectPath)
	if err := cmdDoctor([]string{"--resolution", autoOutput}); err != nil {
		t.Fatalf("doctor through wrapper-provided project context: %v", err)
	}
}
