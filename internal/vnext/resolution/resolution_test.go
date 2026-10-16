package resolution

import (
	"crypto/ed25519"
	"strings"
	"testing"

	"github.com/shichao402/relkit/internal/vnext/catalog"
	"github.com/shichao402/relkit/internal/vnext/contracts"
	"github.com/shichao402/relkit/internal/vnext/operator"
	"github.com/shichao402/relkit/internal/vnext/project"
	"github.com/shichao402/relkit/internal/vnext/runtimeprofile"
	"github.com/shichao402/relkit/internal/vnext/trust"
)

const testDigest = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func resolverFixture(t *testing.T, policy operator.SelectionPolicy) Request {
	t.Helper()
	seed := make([]byte, ed25519.SeedSize)
	for i := range seed {
		seed[i] = byte(i + 11)
	}
	publicKey := ed25519.NewKeyFromSeed(seed).Public().(ed25519.PublicKey)
	fingerprint, err := trust.Fingerprint(publicKey)
	if err != nil {
		t.Fatal(err)
	}
	required := []contracts.Capability{
		{Domain: contracts.DomainPublishProtocol, Peer: "agent", Min: 3, Max: 3},
	}
	product := project.Document{
		Schema:           project.Schema,
		Product:          "demo",
		Channels:         []string{"dev", "stable"},
		OperatorRef:      "prod",
		TrustFingerprint: fingerprint,
		Roles:            []project.Role{project.RoleUpdaterHost},
		CapabilityFingerprint: project.CapabilityFingerprint{
			OSArch:            []string{"windows-amd64"},
			Facades:           []string{"rust"},
			Topology:          "self-update",
			RequiredContracts: required,
		},
	}
	makeComponents := func(version string) []catalog.Component {
		return []catalog.Component{
			{Name: "cli", Version: version, Platform: "windows-amd64", SHA256: testDigest, Source: catalog.Source{Kind: "release-asset", URL: "https://example.invalid/cli"}},
			{Name: "updater", Version: version, Platform: "windows-amd64", SHA256: testDigest, Source: catalog.Source{Kind: "release-asset", URL: "https://example.invalid/updater"}},
			{Name: "sdk-rust", Version: version, Platform: "all", SHA256: testDigest, Source: catalog.Source{Kind: "release-asset", URL: "https://example.invalid/sdk-rust.zip"}},
			{Name: "store", Version: version, Platform: "windows-amd64", SHA256: testDigest, Source: catalog.Source{Kind: "release-asset", URL: "https://example.invalid/store"}},
		}
	}
	makeDistribution := func(version, commit, digest string, state catalog.State, certification int64) catalog.Distribution {
		return catalog.Distribution{
			Version:               version,
			Commit:                commit,
			Digest:                digest,
			State:                 state,
			CertificationSequence: certification,
			Components:            makeComponents(version),
			Provides: []contracts.Capability{
				{Domain: contracts.DomainPublishProtocol, Peer: "agent", Min: 2, Max: 3},
			},
		}
	}
	oldDigest := "sha256:" + strings.Repeat("b", 64)
	newDigest := "sha256:" + strings.Repeat("c", 64)
	candidateDigest := "sha256:" + strings.Repeat("d", 64)
	catalogDocument := catalog.Document{
		Schema:   catalog.Schema,
		Sequence: 3,
		Distributions: []catalog.Distribution{
			makeDistribution("v1.0.0", strings.Repeat("1", 40), oldDigest, catalog.StateCertified, 1),
			makeDistribution("v1.1.0", strings.Repeat("2", 40), newDigest, catalog.StateCertified, 2),
			makeDistribution("v1.2.0", strings.Repeat("3", 40), candidateDigest, catalog.StateCandidate, 0),
		},
	}
	if err := catalogDocument.Sign("root", seed); err != nil {
		t.Fatal(err)
	}
	operatorDocument := operator.Document{
		Schema:       operator.Schema,
		DeploymentID: "prod",
		ServerRelease: operator.ServerRelease{
			Version:            "v1.0.0",
			Commit:             strings.Repeat("1", 40),
			DistributionDigest: oldDigest,
		},
		SelectionPolicy: policy,
		Provides: []contracts.Capability{
			{Domain: contracts.DomainPublishProtocol, Peer: "agent", Min: 2, Max: 3},
		},
		Sequence: 2,
	}
	if err := operatorDocument.Sign("root", seed); err != nil {
		t.Fatal(err)
	}
	return Request{
		Project:                 product,
		Operator:                operatorDocument,
		Catalog:                 catalogDocument,
		TrustedDistributionKeys: map[string]ed25519.PublicKey{"root": publicKey},
	}
}

func TestExactUsesDeployedDistribution(t *testing.T) {
	request := resolverFixture(t, operator.SelectionExact)
	value, err := Resolve(request)
	if err != nil {
		t.Fatal(err)
	}
	if value.SelectedDistribution.Version != "v1.0.0" {
		t.Fatalf("selected %s, want v1.0.0", value.SelectedDistribution.Version)
	}
	if err := value.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestLatestCompatibleUsesNewestCertifiedDistribution(t *testing.T) {
	request := resolverFixture(t, operator.SelectionLatestCompatible)
	request.Operator.Sources.Mirrors = []string{"https://mirror.example.invalid/relkit/"}
	seed := make([]byte, ed25519.SeedSize)
	for i := range seed {
		seed[i] = byte(i + 11)
	}
	if err := request.Operator.Sign("root", seed); err != nil {
		t.Fatal(err)
	}
	value, err := Resolve(request)
	if err != nil {
		t.Fatal(err)
	}
	if value.SelectedDistribution.Version != "v1.1.0" {
		t.Fatalf("selected %s, want v1.1.0", value.SelectedDistribution.Version)
	}
	for _, component := range value.Components {
		if component.Name == "store" {
			t.Fatal("resolver selected an undeclared infrastructure component")
		}
		if len(component.Mirrors) != 1 || !strings.Contains(component.Mirrors[0], "/blobs/") {
			t.Fatalf("component mirrors=%v", component.Mirrors)
		}
	}
}

func TestResolveRequiresEveryDeclaredPlatformComponent(t *testing.T) {
	request := resolverFixture(t, operator.SelectionExact)
	request.Project.CapabilityFingerprint.OSArch = append(
		request.Project.CapabilityFingerprint.OSArch,
		"darwin-arm64",
	)
	_, err := Resolve(request)
	if err == nil || !strings.Contains(err.Error(), "cli/darwin-arm64") ||
		!strings.Contains(err.Error(), "updater/darwin-arm64") {
		t.Fatalf("unexpected result: %v", err)
	}
}

func TestResolveRejectsRegistryTransportMismatch(t *testing.T) {
	request := resolverFixture(t, operator.SelectionExact)
	request.Catalog.Distributions[0].Components[0].Source = catalog.Source{
		Kind:   "module",
		Module: "github.com/example/relkit",
	}
	seed := make([]byte, ed25519.SeedSize)
	for index := range seed {
		seed[index] = byte(index + 11)
	}
	if err := request.Catalog.Sign("root", seed); err != nil {
		t.Fatal(err)
	}
	if _, err := Resolve(request); err == nil || !strings.Contains(err.Error(), "registry requires") {
		t.Fatalf("unexpected result: %v", err)
	}
}

func TestCandidateFiltersStableChannel(t *testing.T) {
	request := resolverFixture(t, operator.SelectionExact)
	candidateSeed := make([]byte, ed25519.SeedSize)
	for index := range candidateSeed {
		candidateSeed[index] = byte(index + 120)
	}
	candidatePublic := ed25519.NewKeyFromSeed(candidateSeed).Public().(ed25519.PublicKey)
	candidateFingerprint, err := trust.Fingerprint(candidatePublic)
	if err != nil {
		t.Fatal(err)
	}
	if err := request.Operator.Sign("candidate", candidateSeed); err != nil {
		t.Fatal(err)
	}
	if err := request.Catalog.Sign("candidate", candidateSeed); err != nil {
		t.Fatal(err)
	}
	request.Identity = IdentityCandidate
	request.CandidateDigest = "sha256:" + strings.Repeat("d", 64)
	request.CandidateRootFingerprint = candidateFingerprint
	request.TrustedCandidateKeys = map[string]ed25519.PublicKey{"candidate": candidatePublic}
	value, err := Resolve(request)
	if err != nil {
		t.Fatal(err)
	}
	if value.SelectedDistribution.Version != "v1.2.0" {
		t.Fatalf("selected %s, want v1.2.0", value.SelectedDistribution.Version)
	}
	for _, channel := range value.AllowedChannels {
		if channel == "stable" {
			t.Fatal("candidate resolution allows stable")
		}
	}
}

func TestCandidateRequiresIndependentAuthority(t *testing.T) {
	request := resolverFixture(t, operator.SelectionExact)
	request.Identity = IdentityCandidate
	request.CandidateDigest = "sha256:" + strings.Repeat("d", 64)
	if _, err := Resolve(request); err == nil || !strings.Contains(err.Error(), "independently pinned") {
		t.Fatalf("unexpected result: %v", err)
	}
}

func TestResolutionDigestDetectsMutation(t *testing.T) {
	request := resolverFixture(t, operator.SelectionExact)
	value, err := Resolve(request)
	if err != nil {
		t.Fatal(err)
	}
	value.Operator.DeploymentID = "other"
	if err := value.Validate(); err == nil || !strings.Contains(err.Error(), "digest mismatch") {
		t.Fatalf("unexpected validation result: %v", err)
	}
}

func TestProjectFingerprintCannotBeBypassedByAdditionalTrustedKey(t *testing.T) {
	request := resolverFixture(t, operator.SelectionExact)
	attackerSeed := make([]byte, ed25519.SeedSize)
	for index := range attackerSeed {
		attackerSeed[index] = byte(200 - index)
	}
	attackerPublic := ed25519.NewKeyFromSeed(attackerSeed).Public().(ed25519.PublicKey)
	request.TrustedDistributionKeys["attacker"] = attackerPublic
	if err := request.Operator.Sign("attacker", attackerSeed); err != nil {
		t.Fatal(err)
	}
	if err := request.Catalog.Sign("attacker", attackerSeed); err != nil {
		t.Fatal(err)
	}
	if _, err := Resolve(request); err == nil || !strings.Contains(err.Error(), "trusted") {
		t.Fatalf("unexpected result: %v", err)
	}
}

func TestRuntimeProfileSubscriptionIsVerifiedAndRecorded(t *testing.T) {
	request := resolverFixture(t, operator.SelectionExact)
	runtimeSeed := make([]byte, ed25519.SeedSize)
	for index := range runtimeSeed {
		runtimeSeed[index] = byte(index + 80)
	}
	runtimePublic := ed25519.NewKeyFromSeed(runtimeSeed).Public().(ed25519.PublicKey)
	fingerprint, err := trust.Fingerprint(runtimePublic)
	if err != nil {
		t.Fatal(err)
	}
	request.Project.Roles = append(request.Project.Roles, project.RoleSubscriber)
	request.Project.RuntimeProfileSubscriptions = []project.Subscription{
		{ProductRef: "editor", RootFingerprint: fingerprint},
	}
	request.Project.CapabilityFingerprint.Subscriptions = []string{"editor"}
	profile := runtimeprofile.Document{
		Schema:          runtimeprofile.Schema,
		Product:         "editor",
		ProfileVersion:  1,
		AllowedChannels: []string{"dev", "stable"},
		DirectoryBootstrap: runtimeprofile.DirectoryBootstrap{
			EntryURLs: []string{"https://updates.example.invalid/directory/editor.pb"},
		},
		RUPSchemaSet: "rup.v2",
	}
	if err := profile.Sign("editor-root", runtimeSeed); err != nil {
		t.Fatal(err)
	}
	request.RuntimeProfiles = map[string]runtimeprofile.Document{"editor": profile}
	request.TrustedRuntimeRoots = map[string]map[string]ed25519.PublicKey{
		"editor": {"editor-root": runtimePublic},
	}
	value, err := Resolve(request)
	if err != nil {
		t.Fatal(err)
	}
	if value.RuntimeProfileDigests["editor"] != profile.Digest {
		t.Fatalf("runtime profile digest = %q, want %q", value.RuntimeProfileDigests["editor"], profile.Digest)
	}

	delete(request.RuntimeProfiles, "editor")
	if _, err := Resolve(request); err == nil || !strings.Contains(err.Error(), "required but was not injected") {
		t.Fatalf("unexpected missing-profile result: %v", err)
	}
}

func TestManagerResolvesMultipleRuntimeProfileSubscriptions(t *testing.T) {
	request := resolverFixture(t, operator.SelectionExact)
	request.Project.Roles = append(request.Project.Roles, project.RoleSubscriber)
	request.Project.CapabilityFingerprint.Topology = "manager"
	request.RuntimeProfiles = map[string]runtimeprofile.Document{}
	request.TrustedRuntimeRoots = map[string]map[string]ed25519.PublicKey{}

	for offset, productRef := range []string{"managed-editor", "managed-tools"} {
		seed := make([]byte, ed25519.SeedSize)
		for index := range seed {
			seed[index] = byte(index + 80 + offset*40)
		}
		publicKey := ed25519.NewKeyFromSeed(seed).Public().(ed25519.PublicKey)
		fingerprint, err := trust.Fingerprint(publicKey)
		if err != nil {
			t.Fatal(err)
		}
		request.Project.RuntimeProfileSubscriptions = append(
			request.Project.RuntimeProfileSubscriptions,
			project.Subscription{ProductRef: productRef, RootFingerprint: fingerprint},
		)
		request.Project.CapabilityFingerprint.Subscriptions = append(
			request.Project.CapabilityFingerprint.Subscriptions, productRef,
		)
		profile := runtimeprofile.Document{
			Schema:          runtimeprofile.Schema,
			Product:         productRef,
			ProfileVersion:  int64(offset + 1),
			AllowedChannels: []string{"dev", "stable"},
			DirectoryBootstrap: runtimeprofile.DirectoryBootstrap{
				EntryURLs: []string{"https://updates.example.invalid/directory/" + productRef + ".pb"},
			},
			RUPSchemaSet: "rup.v2",
		}
		keyID := productRef + "-root"
		if err := profile.Sign(keyID, seed); err != nil {
			t.Fatal(err)
		}
		request.RuntimeProfiles[productRef] = profile
		request.TrustedRuntimeRoots[productRef] = map[string]ed25519.PublicKey{keyID: publicKey}
	}

	value, err := Resolve(request)
	if err != nil {
		t.Fatal(err)
	}
	for productRef, profile := range request.RuntimeProfiles {
		if value.RuntimeProfileDigests[productRef] != profile.Digest {
			t.Fatalf("%s profile digest = %q, want %q", productRef, value.RuntimeProfileDigests[productRef], profile.Digest)
		}
	}
	if len(value.RuntimeProfileDigests) != 2 {
		t.Fatalf("runtime profile digests=%v", value.RuntimeProfileDigests)
	}
}

func TestResolutionBundleReplaysSignedInputs(t *testing.T) {
	request := resolverFixture(t, operator.SelectionExact)
	value, err := Resolve(request)
	if err != nil {
		t.Fatal(err)
	}
	bundle := Bundle{
		Schema:            BundleSchema,
		Project:           request.Project,
		Operator:          request.Operator,
		Catalog:           request.Catalog,
		DistributionRoots: PublicKeys(request.TrustedDistributionKeys),
		Resolution:        *value,
	}
	verified, err := bundle.Verify(BundlePolicy{
		DistributionRootFingerprint: request.Project.TrustFingerprint,
		OperatorDeploymentID:        request.Operator.DeploymentID,
		Identity:                    IdentityProduction,
	})
	if err != nil {
		t.Fatal(err)
	}
	if verified.Digest != value.Digest {
		t.Fatalf("verified digest = %s, want %s", verified.Digest, value.Digest)
	}
	bundle.Resolution.SelectedDistribution.Commit = strings.Repeat("f", 40)
	if _, err := bundle.Verify(BundlePolicy{Identity: IdentityProduction}); err == nil {
		t.Fatal("mutated bundled resolution unexpectedly verified")
	}
}
