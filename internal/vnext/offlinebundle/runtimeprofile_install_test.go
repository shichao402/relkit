package offlinebundle

import (
	"crypto/ed25519"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shichao402/relkit/internal/vnext/catalog"
	"github.com/shichao402/relkit/internal/vnext/document"
	"github.com/shichao402/relkit/internal/vnext/operator"
	"github.com/shichao402/relkit/internal/vnext/project"
	"github.com/shichao402/relkit/internal/vnext/resolution"
	"github.com/shichao402/relkit/internal/vnext/runtimebootstrap"
	"github.com/shichao402/relkit/internal/vnext/runtimeprofile"
	"github.com/shichao402/relkit/internal/vnext/trust"
)

// TestBundleInstallMaterializesRuntimeProfileCache pins down a regression the
// rotation work surfaced: Install wrote resolution.json (which carries
// runtimeProfileDigests) but never produced the runtime-profiles cache that
// `relkit doctor` reads back. Every product with runtimeProfileSubscriptions
// therefore installed cleanly and then failed on the very next doctor call with
// "resolved Runtime Profile is unavailable".
func TestBundleInstallMaterializesRuntimeProfileCache(t *testing.T) {
	fixture := newSubscriberBundleFixture(t)
	output := filepath.Join(fixture.root, "bundle")
	if _, err := Create(CreateOptions{
		ResolutionBundle: fixture.bundle,
		Policy: resolution.BundlePolicy{
			DistributionRootFingerprint: fixture.project.TrustFingerprint,
			OperatorDeploymentID:        fixture.operator.DeploymentID,
			Identity:                    resolution.IdentityProduction,
		},
		Assets:          fixture.assets,
		OutputDirectory: output,
	}); err != nil {
		t.Fatal(err)
	}

	projectRoot := filepath.Join(fixture.root, "checkout")
	if err := os.MkdirAll(projectRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	projectPath := filepath.Join(projectRoot, "relkit.project.json")
	if err := document.Write(projectPath, fixture.project); err != nil {
		t.Fatal(err)
	}
	if _, err := Install(InstallOptions{
		Directory: output, ProjectPath: projectPath, ProjectRoot: projectRoot,
		RelkitHome: filepath.Join(fixture.root, "home"), Target: "linux-amd64",
	}); err != nil {
		t.Fatal(err)
	}

	directory := filepath.Join(projectRoot, ".relkit", "control-plane", "runtime-profiles")
	for productRef, expectedDigest := range fixture.bundle.Resolution.RuntimeProfileDigests {
		profilePath := filepath.Join(directory, productRef+".json")
		profile, err := runtimeprofile.Load(profilePath)
		if err != nil {
			t.Fatalf("runtime profile %q was not materialized: %v", productRef, err)
		}
		if profile.Digest != expectedDigest {
			t.Fatalf("runtime profile %q digest=%s, want %s", productRef, profile.Digest, expectedDigest)
		}
		entries, err := os.ReadDir(filepath.Join(directory, productRef+".roots"))
		if err != nil {
			t.Fatalf("runtime roots for %q were not materialized: %v", productRef, err)
		}
		if len(entries) == 0 {
			t.Fatalf("runtime roots for %q are empty", productRef)
		}
	}
}

type subscriberFixture struct {
	root     string
	seed     []byte
	project  project.Document
	operator operator.Document
	bundle   resolution.Bundle
	assets   map[string]string
}

// newSubscriberBundleFixture builds the smallest bundle that exercises a
// declared runtime profile subscription: product declares subscriber role and
// one subscription, the bundle carries the signed profile and its root.
func newSubscriberBundleFixture(t *testing.T) subscriberFixture {
	t.Helper()
	root := t.TempDir()

	rootSeed := make([]byte, ed25519.SeedSize)
	for index := range rootSeed {
		rootSeed[index] = byte(index + 41)
	}
	rootKey := ed25519.NewKeyFromSeed(rootSeed).Public().(ed25519.PublicKey)
	rootFingerprint, err := trust.Fingerprint(rootKey)
	if err != nil {
		t.Fatal(err)
	}

	profileSeed := make([]byte, ed25519.SeedSize)
	for index := range profileSeed {
		profileSeed[index] = byte(index + 97)
	}
	profileKey := ed25519.NewKeyFromSeed(profileSeed).Public().(ed25519.PublicKey)
	profileFingerprint, err := trust.Fingerprint(profileKey)
	if err != nil {
		t.Fatal(err)
	}

	sdkPath := filepath.Join(root, "sdk-rust.zip")
	makeZip(t, sdkPath)
	sdkDigest := digestFile(t, sdkPath)

	cliPath := filepath.Join(root, "cli")
	opsPath := filepath.Join(root, "ops")
	updaterPath := filepath.Join(root, "updater")
	for path, content := range map[string]string{
		cliPath: "#!/bin/sh\nexit 0\n", opsPath: "#!/bin/sh\necho ops\n", updaterPath: "updater",
	} {
		if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	cliDigest := digestFile(t, cliPath)
	opsDigest := digestFile(t, opsPath)
	updaterDigest := digestFile(t, updaterPath)

	distributionDigest := "sha256:" + strings.Repeat("e", 64)
	distribution := catalog.Distribution{
		Version:               "v1.0.0",
		Commit:                strings.Repeat("2", 40),
		Digest:                distributionDigest,
		State:                 catalog.StateCertified,
		CertificationSequence: 1,
		Components: []catalog.Component{
			{Name: "cli", Version: "v1.0.0", Platform: "linux-amd64", SHA256: cliDigest, Source: catalog.Source{Kind: "release-asset", URL: "https://offline.invalid/relkit"}},
			{Name: "ops", Version: "v1.0.0", Platform: "linux-amd64", SHA256: opsDigest, Source: catalog.Source{Kind: "release-asset", URL: "https://offline.invalid/relkit-ops"}},
			{Name: "updater", Version: "v1.0.0", Platform: "linux-amd64", SHA256: updaterDigest, Source: catalog.Source{Kind: "release-asset", URL: "https://offline.invalid/relkit-updater"}},
			{Name: "sdk-rust", Version: "v1.0.0", Platform: "all", SHA256: sdkDigest, Source: catalog.Source{Kind: "release-asset", URL: "https://offline.invalid/sdk-rust.zip"}},
		},
	}
	catalogDocument := catalog.Document{
		Schema: catalog.Schema, Sequence: 1,
		Distributions: []catalog.Distribution{distribution},
	}
	if err := catalogDocument.Sign("root", rootSeed); err != nil {
		t.Fatal(err)
	}
	operatorDocument := operator.Document{
		Schema: operator.Schema, DeploymentID: "offline", SelectionPolicy: operator.SelectionExact,
		ServerRelease: operator.ServerRelease{
			Version: "v1.0.0", Commit: distribution.Commit, DistributionDigest: distributionDigest,
		},
		Sequence: 1,
	}
	if err := operatorDocument.Sign("root", rootSeed); err != nil {
		t.Fatal(err)
	}

	profile := runtimeprofile.Document{
		Schema:          runtimeprofile.Schema,
		Product:         "managed",
		ProfileVersion:  1,
		AllowedChannels: []string{"dev"},
		DirectoryBootstrap: runtimeprofile.DirectoryBootstrap{
			EntryURLs: []string{"https://offline.invalid/directory/managed.pb"},
		},
		RUPSchemaSet: "rup.v2",
	}
	if err := profile.Sign("managed-root", profileSeed); err != nil {
		t.Fatal(err)
	}

	product := project.Document{
		Schema: project.Schema, Product: "demo", Channels: []string{"dev"},
		OperatorRef: "offline", TrustFingerprint: rootFingerprint,
		Roles: []project.Role{project.RoleUpdaterHost, project.RoleSubscriber},
		RuntimeProfileSubscriptions: []project.Subscription{
			{ProductRef: "managed", RootFingerprint: profileFingerprint},
		},
		CapabilityFingerprint: project.CapabilityFingerprint{
			OSArch:        []string{"linux-amd64"},
			Facades:       []string{"rust"},
			Topology:      "manager-managed-products",
			Subscriptions: []string{"managed"},
		},
	}
	resolved, err := resolution.Resolve(resolution.Request{
		Project:                 product,
		Operator:                operatorDocument,
		Catalog:                 catalogDocument,
		TrustedDistributionKeys: map[string]ed25519.PublicKey{"root": rootKey},
		RuntimeProfiles:         map[string]runtimeprofile.Document{"managed": profile},
		TrustedRuntimeRoots: map[string]map[string]ed25519.PublicKey{
			"managed": {"managed-root": profileKey},
		},
		Identity: resolution.IdentityProduction,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved.RuntimeProfileDigests) != 1 {
		t.Fatalf("runtimeProfileDigests=%v", resolved.RuntimeProfileDigests)
	}
	bundle := resolution.Bundle{
		Schema:            resolution.BundleSchema,
		Project:           product,
		Operator:          operatorDocument,
		Catalog:           catalogDocument,
		RuntimeProfiles:   map[string]runtimeprofile.Document{"managed": profile},
		RuntimeRoots:      map[string][]resolution.PublicKey{"managed": resolution.PublicKeys(map[string]ed25519.PublicKey{"managed-root": profileKey})},
		DistributionRoots: resolution.PublicKeys(map[string]ed25519.PublicKey{"root": rootKey}),
		Resolution:        *resolved,
	}
	return subscriberFixture{
		root: root, seed: rootSeed, project: product, operator: operatorDocument,
		bundle: bundle,
		assets: map[string]string{
			cliDigest: cliPath, opsDigest: opsPath, updaterDigest: updaterPath,
			sdkDigest: sdkPath,
		},
	}
}

var _ = runtimebootstrap.Schema
