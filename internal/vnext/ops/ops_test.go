package ops

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	rupv2 "github.com/shichao402/relkit/api/rup/v2"
	"github.com/shichao402/relkit/internal/keys"
	"github.com/shichao402/relkit/internal/vnext/bootstrap"
	"github.com/shichao402/relkit/internal/vnext/catalog"
	"github.com/shichao402/relkit/internal/vnext/contracts"
	"github.com/shichao402/relkit/internal/vnext/extension"
	"github.com/shichao402/relkit/internal/vnext/operator"
	"github.com/shichao402/relkit/internal/vnext/project"
	"github.com/shichao402/relkit/internal/vnext/trust"
)

func TestOpsAdapterHelper(t *testing.T) {
	if os.Getenv("RELKIT_OPS_TEST_HELPER") != "1" {
		return
	}
	data, _ := io.ReadAll(os.Stdin)
	var input map[string]any
	if err := json.Unmarshal(data, &input); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	phase, _ := input["phase"].(string)
	if logPath := os.Getenv("RELKIT_OPS_TEST_LOG"); logPath != "" {
		file, _ := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if file != nil {
			_, _ = fmt.Fprintln(file, phase)
			_ = file.Close()
		}
	}
	if failed := os.Getenv("RELKIT_OPS_TEST_FAIL"); failed != "" && phase == failed {
		fmt.Fprintln(os.Stderr, "injected "+phase+" failure")
		os.Exit(2)
	}
	output := StepResult{
		Schema:   StepResultSchema,
		OK:       true,
		Evidence: map[string]any{"phase": phase},
	}
	if phase == "mint" {
		output.SecretRef = "secret://new"
	}
	_ = json.NewEncoder(os.Stdout).Encode(output)
	os.Exit(0)
}

func TestUpgradePlanAndApply(t *testing.T) {
	fixture := newOpsFixture(t)
	plan, err := PlanUpgrade(fixture.inventory, "latest")
	if err != nil {
		t.Fatal(err)
	}
	if plan.Target.Version != "2.0.0" {
		t.Fatalf("target=%s", plan.Target.Version)
	}
	profile, err := ApplyUpgrade(context.Background(), fixture.inventory, *plan)
	if err != nil {
		t.Fatal(err)
	}
	if profile.ServerRelease.Version != "2.0.0" || profile.Sequence != 2 {
		t.Fatalf("profile=%+v", profile)
	}
	if len(profile.Sources.Mirrors) != 1 || profile.Sources.Mirrors[0] != fixture.inventory.Mirrors[0] {
		t.Fatalf("profile mirrors=%v, want %v", profile.Sources.Mirrors, fixture.inventory.Mirrors)
	}
	active, err := operator.Load(fixture.inventory.ActiveProfilePath)
	if err != nil {
		t.Fatal(err)
	}
	if active.ServerRelease.DistributionDigest != fixture.newDistribution.Digest {
		t.Fatalf("active digest=%s", active.ServerRelease.DistributionDigest)
	}
	assertPhases(t, fixture.logPath, "candidate", "sync", "deploy", "verify", "activate")
	status, err := ReadStatus(fixture.inventory)
	if err != nil {
		t.Fatal(err)
	}
	if !status.OK || status.UpgradeAvailable {
		t.Fatalf("status=%+v", status)
	}
}

func TestUpgradeFailureRollsBackWithoutPublishingProfile(t *testing.T) {
	fixture := newOpsFixture(t)
	before, err := os.ReadFile(fixture.inventory.ActiveProfilePath)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("RELKIT_OPS_TEST_FAIL", "verify")
	plan, err := PlanUpgrade(fixture.inventory, "latest")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyUpgrade(context.Background(), fixture.inventory, *plan); err == nil ||
		!strings.Contains(err.Error(), "did not become active") {
		t.Fatalf("unexpected error: %v", err)
	}
	after, err := os.ReadFile(fixture.inventory.ActiveProfilePath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("active profile changed after failed upgrade")
	}
	assertPhases(t, fixture.logPath, "candidate", "sync", "deploy", "verify", "rollback")
	status, err := ReadStatus(fixture.inventory)
	if err != nil {
		t.Fatal(err)
	}
	if status.OK || len(status.IncompleteOperations) != 1 {
		t.Fatalf("failed operation not visible: %+v", status)
	}
}

func TestUpgradeRejectsStalePlan(t *testing.T) {
	fixture := newOpsFixture(t)
	plan, err := PlanUpgrade(fixture.inventory, "latest")
	if err != nil {
		t.Fatal(err)
	}
	active, err := operator.Load(fixture.inventory.ActiveProfilePath)
	if err != nil {
		t.Fatal(err)
	}
	active.Sequence++
	if err := active.Sign(fixture.keyID, fixture.seed); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(fixture.inventory.ActiveProfilePath, active); err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyUpgrade(context.Background(), fixture.inventory, *plan); err == nil ||
		!strings.Contains(err.Error(), "changed after the plan") {
		t.Fatalf("unexpected stale-plan result: %v", err)
	}
}

func TestUpgradeRejectsTamperedMirrorPlan(t *testing.T) {
	fixture := newOpsFixture(t)
	plan, err := PlanUpgrade(fixture.inventory, "latest")
	if err != nil {
		t.Fatal(err)
	}
	plan.Mirrors = []string{"https://attacker.invalid/relkit"}
	if _, err := ApplyUpgrade(context.Background(), fixture.inventory, *plan); err == nil ||
		!strings.Contains(err.Error(), "mirrors changed") {
		t.Fatalf("unexpected mirror-plan result: %v", err)
	}
}

func TestInventoryRejectsDuplicateMirrors(t *testing.T) {
	fixture := newOpsFixture(t)
	fixture.inventory.Mirrors = append(fixture.inventory.Mirrors, fixture.inventory.Mirrors[0])
	if err := fixture.inventory.Validate(); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("unexpected duplicate-mirror result: %v", err)
	}
	fixture.inventory.Mirrors = []string{"https://writer:secret@mirror.invalid/relkit"}
	if err := fixture.inventory.Validate(); err == nil || !strings.Contains(err.Error(), "without credentials") {
		t.Fatalf("unexpected credentialed-mirror result: %v", err)
	}
}

func TestRotateRequiresEveryPostconditionBeforeProfileActivation(t *testing.T) {
	phases := []string{"mint", "update-secret-store", "check-service-read", "reload", "health-and-write", "revoke-old"}
	for _, failedPhase := range phases {
		t.Run(failedPhase, func(t *testing.T) {
			fixture := newOpsFixture(t)
			before, _ := os.ReadFile(fixture.inventory.ActiveProfilePath)
			t.Setenv("RELKIT_OPS_TEST_FAIL", failedPhase)
			if _, err := RotateCredential(context.Background(), fixture.inventory, "upload"); err == nil {
				t.Fatalf("phase %s unexpectedly succeeded", failedPhase)
			}
			after, _ := os.ReadFile(fixture.inventory.ActiveProfilePath)
			if !bytes.Equal(before, after) {
				t.Fatalf("profile activated after %s failed", failedPhase)
			}
		})
	}
}

func TestRotateActivatesOnlyAfterRevoke(t *testing.T) {
	fixture := newOpsFixture(t)
	profile, err := RotateCredential(context.Background(), fixture.inventory, "upload")
	if err != nil {
		t.Fatal(err)
	}
	if profile.SecretRefs["upload"] != "secret://new" || profile.Sequence != 2 {
		t.Fatalf("profile=%+v", profile)
	}
	assertPhases(t, fixture.logPath, "mint", "update-secret-store", "check-service-read", "reload", "health-and-write", "revoke-old")
}

func TestCandidateAuthorityCannotSignCertifiedCatalog(t *testing.T) {
	fixture := newOpsFixture(t)
	unsigned := fixture.catalog
	unsigned.Signatures = nil
	input := filepath.Join(fixture.root, "unsigned-catalog.json")
	if err := writeJSON(input, unsigned); err != nil {
		t.Fatal(err)
	}
	_, err := SignCatalog(SignCatalogOptions{
		Input: input, Output: filepath.Join(fixture.root, "candidate.json"),
		KeyPath: fixture.privateKeyPath, Authority: AuthorityCandidate,
	})
	if err == nil || !strings.Contains(err.Error(), "candidate authority cannot sign") {
		t.Fatalf("unexpected result: %v", err)
	}
}

func TestProvisionBootstrapKeepsPrivateInputsOut(t *testing.T) {
	fixture := newOpsFixture(t)
	binary := filepath.Join(fixture.root, "relkit")
	if err := os.WriteFile(binary, []byte("bootstrap"), 0o755); err != nil {
		t.Fatal(err)
	}
	fixture.inventory.BootstrapBinaries = map[string]string{"darwin-arm64": binary}
	home := filepath.Join(fixture.root, "home")
	path, err := ProvisionBootstrap(fixture.inventory, home)
	if err != nil {
		t.Fatal(err)
	}
	value, err := bootstrap.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if value.OperatorRef != fixture.inventory.Operator {
		t.Fatalf("operatorRef=%s", value.OperatorRef)
	}
	if value.BootstrapBinaries["darwin-arm64"].SHA256 == "" {
		t.Fatal("bootstrap digest is missing")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte(fixture.inventory.SigningKeyPath)) ||
		bytes.Contains(data, fixture.seed) {
		t.Fatal("bootstrap document leaked a private signing input")
	}
}

func TestCatalogTransitionIncrementsCertificationSequence(t *testing.T) {
	fixture := newOpsFixture(t)
	candidate := fixture.catalog
	candidate.Sequence = 3
	candidate.Distributions[1].State = catalog.StateCandidate
	candidate.Distributions[1].CertificationSequence = 0
	if err := candidate.Sign(fixture.keyID, fixture.seed); err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(fixture.root, "candidate-catalog.json")
	output := filepath.Join(fixture.root, "certified-catalog.json")
	if err := writeJSON(input, candidate); err != nil {
		t.Fatal(err)
	}
	reportPath := filepath.Join(fixture.root, "certification.json")
	if err := writeJSON(reportPath, CertificationReport{
		Schema: CertificationReportSchema, CandidateDigest: fixture.newDistribution.Digest,
		ImpactDigest: "sha256:" + strings.Repeat("9", 64), Passed: true,
		Results: []CertificationResult{{HostID: "canary", CI: "gha", Status: "passed"}},
	}); err != nil {
		t.Fatal(err)
	}
	value, err := TransitionCatalog(TransitionCatalogOptions{
		Input: input, Output: output, Distribution: fixture.newDistribution.Digest,
		State: catalog.StateCertified, KeyPath: fixture.privateKeyPath,
		TrustKeyPaths: []string{fixture.publicKeyPath}, Authority: AuthorityProduction,
		CertificationReportPath: reportPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	if value.Sequence != 4 || value.Distributions[1].CertificationSequence != 4 {
		t.Fatalf("transition=%+v", value)
	}
}

func TestGenerateImpactUsesIndependentCandidateAuthorityAndDiffsContracts(t *testing.T) {
	root := t.TempDir()
	productionSeed := bytes.Repeat([]byte{11}, 32)
	candidateSeed := bytes.Repeat([]byte{29}, 32)
	productionKeyID := "production-root"
	candidateKeyID := "candidate-root"
	productionPublic := keys.PublicKeyDocument(productionKeyID, productionSeed)
	candidatePublic := keys.PublicKeyDocument(candidateKeyID, candidateSeed)
	productionPublicPath := filepath.Join(root, "production.public.pb")
	candidatePublicPath := filepath.Join(root, "candidate.public.pb")
	for path, value := range map[string]rupv2.PublicKeyDocument{
		productionPublicPath: productionPublic,
		candidatePublicPath:  candidatePublic,
	} {
		data, err := rupv2.MarshalPublicKey(&value)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	candidateFingerprint, err := trust.Fingerprint(candidatePublic.PublicKey)
	if err != nil {
		t.Fatal(err)
	}

	productionDigest := "sha256:" + strings.Repeat("1", 64)
	candidateDigest := "sha256:" + strings.Repeat("2", 64)
	componentDigest := "sha256:" + strings.Repeat("3", 64)
	changedComponentDigest := "sha256:" + strings.Repeat("4", 64)
	productionDistribution := catalog.Distribution{
		Version: "1.0.0", Commit: strings.Repeat("1", 40), Digest: productionDigest,
		State: catalog.StateCertified, CertificationSequence: 1,
		Components: []catalog.Component{{
			Name: "cli", Version: "1.0.0", Platform: "linux-amd64", SHA256: componentDigest,
			Source: catalog.Source{Kind: "release-asset", URL: "https://example.invalid/cli"},
		}},
		Provides: []contracts.Capability{{
			Domain: contracts.DomainPublishProtocol, Peer: "agent", Min: 2, Max: 3,
		}},
	}
	candidateDistribution := catalog.Distribution{
		Version: "2.0.0", Commit: strings.Repeat("2", 40), Digest: candidateDigest,
		State: catalog.StateCandidate,
		Components: []catalog.Component{
			{
				Name: "cli", Version: "2.0.0", Platform: "linux-amd64", SHA256: changedComponentDigest,
				Source: catalog.Source{Kind: "release-asset", URL: "https://example.invalid/cli"},
			},
			{
				Name: "updater", Version: "2.0.0", Platform: "linux-amd64", SHA256: componentDigest,
				Source: catalog.Source{Kind: "release-asset", URL: "https://example.invalid/updater"},
			},
		},
		Provides: []contracts.Capability{
			{Domain: contracts.DomainPublishProtocol, Peer: "agent", Min: 3, Max: 4},
			{Domain: contracts.DomainArtifactSet, Peer: "producer", ID: "relkit.artifact-set/1"},
		},
	}
	productionCatalog := catalog.Document{
		Schema: catalog.Schema, Sequence: 1, Distributions: []catalog.Distribution{productionDistribution},
	}
	if err := productionCatalog.Sign(productionKeyID, productionSeed); err != nil {
		t.Fatal(err)
	}
	candidateCatalog := catalog.Document{
		Schema: catalog.Schema, Sequence: 1, Distributions: []catalog.Distribution{candidateDistribution},
	}
	if err := candidateCatalog.Sign(candidateKeyID, candidateSeed); err != nil {
		t.Fatal(err)
	}
	profile := operator.Document{
		Schema: operator.Schema, DeploymentID: "candidate-operator",
		ServerRelease: operator.ServerRelease{
			Version: "2.0.0", Commit: candidateDistribution.Commit, DistributionDigest: candidateDigest,
		},
		SelectionPolicy: operator.SelectionExact,
		Provides:        candidateDistribution.Provides,
		Sequence:        1,
	}
	if err := profile.Sign(candidateKeyID, candidateSeed); err != nil {
		t.Fatal(err)
	}
	productionCatalogPath := filepath.Join(root, "production-catalog.json")
	candidateCatalogPath := filepath.Join(root, "candidate-catalog.json")
	candidateProfilePath := filepath.Join(root, "candidate-profile.json")
	if err := writeJSON(productionCatalogPath, productionCatalog); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(candidateCatalogPath, candidateCatalog); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(candidateProfilePath, profile); err != nil {
		t.Fatal(err)
	}

	options := GenerateImpactOptions{
		ProductionCatalogPath:    productionCatalogPath,
		CandidateCatalogPath:     candidateCatalogPath,
		ProductionTrustKeyPaths:  []string{productionPublicPath},
		CandidateTrustKeyPaths:   []string{candidatePublicPath},
		CandidateDigest:          candidateDigest,
		CandidateProfilePath:     candidateProfilePath,
		CandidateRootFingerprint: candidateFingerprint,
		Risk:                     RiskContract,
	}
	impact, err := GenerateImpact(options)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(impact.AffectedComponents, ",") != "cli,updater" {
		t.Fatalf("affected components=%v", impact.AffectedComponents)
	}
	if len(impact.AffectedCapabilities) != 2 || len(impact.NewCapabilities) != 1 ||
		impact.NewCapabilities[0].Domain != contracts.DomainArtifactSet {
		t.Fatalf("affected=%+v new=%+v", impact.AffectedCapabilities, impact.NewCapabilities)
	}

	profile.ServerRelease.DistributionDigest = productionDigest
	if err := profile.Sign(candidateKeyID, candidateSeed); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(candidateProfilePath, profile); err != nil {
		t.Fatal(err)
	}
	if _, err := GenerateImpact(options); err == nil || !strings.Contains(err.Error(), "impact target") {
		t.Fatalf("unexpected profile mismatch result: %v", err)
	}
}

func TestFleetSelectionUsesCohortCanariesAndCrossCuttingFull(t *testing.T) {
	root := t.TempDir()
	capability := contracts.Capability{Domain: contracts.DomainPublishProtocol, Peer: "agent", Min: 3, Max: 3}
	projectA := writeFleetProject(t, root, "a", []contracts.Capability{capability})
	projectB := writeFleetProject(t, root, "b", []contracts.Capability{capability})
	projectC := writeFleetProject(t, root, "c", []contracts.Capability{{
		Domain: contracts.DomainUpdaterIPC, Peer: "updater", Min: 3, Max: 3,
	}})
	adapter := adapterSpec(extension.KindCIAdapter, CertificationInputSchema)
	fleet := Fleet{
		Schema: FleetSchema,
		Hosts: []FleetHost{
			{ID: "fixed", Active: true, ProjectPath: projectA, Canary: CanaryFixed, RequiredCI: []string{"gha"}, CIAdapters: map[string]extension.Spec{"gha": adapter}},
			{ID: "rotate", Active: true, ProjectPath: projectB, Canary: CanaryRotating, RequiredCI: []string{"gha"}, CIAdapters: map[string]extension.Spec{"gha": adapter}},
			{ID: "other", Active: true, ProjectPath: projectC, Canary: CanaryFixed, RequiredCI: []string{"gha"}, CIAdapters: map[string]extension.Spec{"gha": adapter}},
		},
	}
	impact := testImpact(RiskSingleCapability, capability)
	selected, err := SelectFleet(fleet, impact, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(selected) != 2 || selected[0].Host.ID != "fixed" || selected[1].Host.ID != "rotate" {
		t.Fatalf("selected=%v", selectedIDs(selected))
	}
	impact.Risk = RiskCrossCutting
	selected, err = SelectFleet(fleet, impact, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(selected) != 3 {
		t.Fatalf("cross-cutting selected=%v", selectedIDs(selected))
	}
}

func TestCertificationRequiresEveryDeclaredCIPath(t *testing.T) {
	root := t.TempDir()
	t.Setenv("RELKIT_OPS_TEST_HELPER", "1")
	t.Setenv("RELKIT_OPS_TEST_FAIL", "")
	t.Setenv("RELKIT_OPS_TEST_LOG", filepath.Join(root, "cert.log"))
	capability := contracts.Capability{Domain: contracts.DomainPublishProtocol, Peer: "agent", Min: 3, Max: 3}
	projectPath := writeFleetProject(t, root, "host", []contracts.Capability{capability})
	adapter := adapterSpec(extension.KindCIAdapter, CertificationInputSchema)
	fleet := Fleet{
		Schema: FleetSchema,
		Hosts: []FleetHost{{
			ID: "host", Active: true, ProjectPath: projectPath, Canary: CanaryFixed,
			RequiredCI: []string{"gha", "blue"},
			CIAdapters: map[string]extension.Spec{"gha": adapter},
		}},
	}
	report, err := RunCertification(context.Background(), fleet, testImpact(RiskContract, capability), false)
	if err != nil {
		t.Fatal(err)
	}
	if report.Passed {
		t.Fatal("certification passed with missing required CI path")
	}
	if len(report.Results) != 2 || report.Results[1].Status != "skipped-unverified" {
		t.Fatalf("results=%+v", report.Results)
	}
	fleet.Hosts[0].CIAdapters["blue"] = adapter
	report, err = RunCertification(context.Background(), fleet, testImpact(RiskContract, capability), false)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Passed || len(report.Results) != 2 {
		t.Fatalf("report=%+v", report)
	}
}

type opsFixture struct {
	root            string
	logPath         string
	privateKeyPath  string
	publicKeyPath   string
	keyID           string
	seed            []byte
	catalog         catalog.Document
	newDistribution catalog.Distribution
	inventory       Inventory
}

func newOpsFixture(t *testing.T) opsFixture {
	t.Helper()
	root := t.TempDir()
	t.Setenv("RELKIT_OPS_TEST_HELPER", "1")
	t.Setenv("RELKIT_OPS_TEST_FAIL", "")
	logPath := filepath.Join(root, "steps.log")
	t.Setenv("RELKIT_OPS_TEST_LOG", logPath)

	seed, err := keys.GenerateSeed()
	if err != nil {
		t.Fatal(err)
	}
	keyID := "root"
	privateKeyPath := filepath.Join(root, "root.private.pb")
	publicKeyPath := filepath.Join(root, "root.public.pb")
	privateDoc := keys.PrivateKeyDocument(keyID, seed)
	privateData, _ := rupv2.MarshalPrivateKey(&privateDoc)
	if err := os.WriteFile(privateKeyPath, privateData, 0o600); err != nil {
		t.Fatal(err)
	}
	publicDoc := keys.PublicKeyDocument(keyID, seed)
	publicData, _ := rupv2.MarshalPublicKey(&publicDoc)
	if err := os.WriteFile(publicKeyPath, publicData, 0o644); err != nil {
		t.Fatal(err)
	}

	oldDistribution := testDistribution("1.0.0", "1", 1)
	newDistribution := testDistribution("2.0.0", "2", 2)
	catalogDocument := catalog.Document{
		Schema:        catalog.Schema,
		Sequence:      2,
		Distributions: []catalog.Distribution{oldDistribution, newDistribution},
	}
	if err := catalogDocument.Sign(keyID, seed); err != nil {
		t.Fatal(err)
	}
	catalogPath := filepath.Join(root, "catalog.json")
	if err := writeJSON(catalogPath, catalogDocument); err != nil {
		t.Fatal(err)
	}
	profile := operator.Document{
		Schema:       operator.Schema,
		DeploymentID: "demo-operator",
		ServerRelease: operator.ServerRelease{
			Version:            oldDistribution.Version,
			Commit:             oldDistribution.Commit,
			DistributionDigest: oldDistribution.Digest,
		},
		SelectionPolicy: operator.SelectionExact,
		Provides:        oldDistribution.Provides,
		SecretRefs:      map[string]string{"upload": "secret://old"},
		Sequence:        1,
	}
	if err := profile.Sign(keyID, seed); err != nil {
		t.Fatal(err)
	}
	profilePath := filepath.Join(root, "operator.json")
	if err := writeJSON(profilePath, profile); err != nil {
		t.Fatal(err)
	}
	upgrade := UpgradeAdapters{
		Candidate: adapterSpec(extension.KindCIAdapter, UpgradeInputSchema),
		Sync:      adapterSpec(extension.KindMirrorProducer, UpgradeInputSchema),
		Deploy:    adapterSpec(extension.KindDeployAdapter, UpgradeInputSchema),
		Verify:    adapterSpec(extension.KindDeployAdapter, UpgradeInputSchema),
		Activate:  adapterSpec(extension.KindDeployAdapter, UpgradeInputSchema),
		Rollback:  adapterSpec(extension.KindDeployAdapter, UpgradeInputSchema),
	}
	rotate := RotateAdapters{
		Mint:      adapterSpec(extension.KindRotateAdapter, RotateInputSchema),
		Update:    adapterSpec(extension.KindRotateAdapter, RotateInputSchema),
		CheckRead: adapterSpec(extension.KindRotateAdapter, RotateInputSchema),
		Reload:    adapterSpec(extension.KindRotateAdapter, RotateInputSchema),
		Health:    adapterSpec(extension.KindRotateAdapter, RotateInputSchema),
		Revoke:    adapterSpec(extension.KindRotateAdapter, RotateInputSchema),
	}
	inventory := Inventory{
		Schema:            InventorySchema,
		Operator:          "demo-operator",
		ActiveProfilePath: profilePath,
		CatalogPath:       catalogPath,
		TrustKeyPaths:     []string{publicKeyPath},
		SigningKeyPath:    privateKeyPath,
		StateDirectory:    filepath.Join(root, "state"),
		Services:          []string{"agent", "store"},
		Mirrors:           []string{"https://mirror.invalid/relkit"},
		Upgrade:           upgrade,
		Rotate:            map[string]RotateAdapters{"upload": rotate},
	}
	if err := inventory.Validate(); err != nil {
		t.Fatal(err)
	}
	return opsFixture{
		root: root, logPath: logPath, privateKeyPath: privateKeyPath, publicKeyPath: publicKeyPath,
		keyID: keyID, seed: seed, catalog: catalogDocument,
		newDistribution: newDistribution, inventory: inventory,
	}
}

func testDistribution(version, fill string, sequence int64) catalog.Distribution {
	digest := "sha256:" + strings.Repeat(fill, 64)
	return catalog.Distribution{
		Version:               version,
		Commit:                strings.Repeat(fill, 40),
		Digest:                digest,
		State:                 catalog.StateCertified,
		CertificationSequence: sequence,
		Components: []catalog.Component{{
			Name: "cli", Version: version, Platform: "linux-amd64",
			SHA256: digest, Source: catalog.Source{Kind: "release-asset", URL: "file:///tmp/relkit"},
		}},
	}
}

func adapterSpec(kind extension.Kind, inputSchema string) extension.Spec {
	return extension.Spec{
		Schema:         extension.Schema,
		Kind:           kind,
		Command:        []string{os.Args[0], "-test.run=TestOpsAdapterHelper", "--"},
		StdinSchema:    inputSchema,
		StdoutSchema:   StepResultSchema,
		TimeoutSeconds: 10,
		EnvAllowlist:   []string{"RELKIT_OPS_TEST_HELPER", "RELKIT_OPS_TEST_FAIL", "RELKIT_OPS_TEST_LOG"},
	}
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func assertPhases(t *testing.T, path string, expected ...string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Fields(string(data))
	if strings.Join(got, ",") != strings.Join(expected, ",") {
		t.Fatalf("phases=%v want=%v", got, expected)
	}
}

func writeFleetProject(t *testing.T, root, name string, required []contracts.Capability) string {
	t.Helper()
	value := project.Document{
		Schema: project.Schema, Product: name, OperatorRef: "operator",
		TrustFingerprint: "sha256:" + strings.Repeat("a", 64),
		Roles:            []project.Role{project.RoleUpdaterHost},
		CapabilityFingerprint: project.CapabilityFingerprint{
			OSArch: []string{"linux-amd64"}, Facades: []string{"go"},
			Topology: "self-update", RequiredContracts: required,
		},
	}
	path := filepath.Join(root, name+".project.json")
	if err := writeJSON(path, value); err != nil {
		t.Fatal(err)
	}
	return path
}

func testImpact(risk Risk, capability contracts.Capability) Impact {
	return Impact{
		Schema: ImpactSchema, CandidateDigest: "sha256:" + strings.Repeat("c", 64),
		Risk: risk, AffectedCapabilities: []contracts.Capability{capability},
		CandidateProfilePath:     "candidate-operator.json",
		CandidateCatalogPath:     "candidate-catalog.json",
		CandidateRootFingerprint: "sha256:" + strings.Repeat("b", 64),
	}
}

func selectedIDs(values []SelectedHost) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, value.Host.ID)
	}
	return result
}
