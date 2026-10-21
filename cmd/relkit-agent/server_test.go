package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	rupv2 "github.com/shichao402/relkit/api/rup/v2"
	"github.com/shichao402/relkit/internal/casput"
	"github.com/shichao402/relkit/internal/config"
	"github.com/shichao402/relkit/internal/inprocess"
	"github.com/shichao402/relkit/internal/keys"
	"github.com/shichao402/relkit/internal/makers"
	"github.com/shichao402/relkit/internal/payload"
	"github.com/shichao402/relkit/internal/publishproto"
	"github.com/shichao402/relkit/internal/stage"
	"github.com/shichao402/relkit/internal/verify"
	"github.com/shichao402/relkit/internal/vnext/artifactset"
	"github.com/shichao402/relkit/internal/vnext/catalog"
	"github.com/shichao402/relkit/internal/vnext/document"
	"github.com/shichao402/relkit/internal/vnext/operator"
	"github.com/shichao402/relkit/internal/vnext/project"
	vnextrelease "github.com/shichao402/relkit/internal/vnext/release"
	"github.com/shichao402/relkit/internal/vnext/resolution"
	"github.com/shichao402/relkit/internal/vnext/runtimeprofile"
	"github.com/shichao402/relkit/internal/vnext/trust"
	"github.com/shichao402/relkit/sdk"
)

func TestSiteStatusIsRedacted(t *testing.T) {
	t.Setenv("PAGES_SECRET", "must-not-leak")
	srv := NewServer(&Config{Site: SiteConfig{Makers: &makers.Config{
		ProjectID: "makers-demo", TokenEnv: "PAGES_SECRET", Region: "china",
	}}})
	req := httptest.NewRequest(http.MethodGet, "/-/site", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	body := rec.Body.String()
	var status map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK || status["tokenPresent"] != true {
		t.Fatalf("status=%d body=%s", rec.Code, body)
	}
	if strings.Contains(body, "must-not-leak") {
		t.Fatalf("site status leaked token: %s", body)
	}
}

func TestAgentStagedAndPublishDryRun(t *testing.T) {
	// The agent accepts exactly one staged shape since ADR 0019 §1: a vNext
	// staged tree with a signed resolution digest. The fixture builds that tree,
	// uploads it as a tarball and publishes it as a dry run.
	fx := newAgentFixture(t, agentFixtureOpts{channel: "stable"})
	status, body := fx.putStaged(t, "demo", "1.0.0")
	if status != http.StatusCreated {
		t.Fatalf("staged status=%d body=%s", status, body)
	}
	status, body = fx.publish(t, `{"product":"demo","version":"1.0.0","dryRun":true}`)
	if status != http.StatusOK {
		t.Fatalf("publish status=%d body=%s", status, body)
	}
}

func TestAgentDropPutGetHead(t *testing.T) {
	root := t.TempDir()
	productRoot := filepath.Join(root, "product")
	if err := os.MkdirAll(productRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	agentCfgPath := filepath.Join(root, "agent.json")
	tokenPath := filepath.Join(root, "tokens", "demo.token")
	if err := os.MkdirAll(filepath.Dir(tokenPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tokenPath, []byte("test-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	agentDoc := map[string]any{
		"stateDir": filepath.Join(root, "state"),
		"uploadTokens": []any{
			map[string]any{"file": "tokens/demo.token", "products": []any{"demo"}},
		},
		"products": map[string]any{
			"demo": map[string]any{"root": productRoot},
		},
	}
	agentBytes, _ := json.MarshalIndent(agentDoc, "", "  ")
	if err := os.WriteFile(agentCfgPath, agentBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(agentCfgPath)
	if err != nil {
		t.Fatal(err)
	}
	srv := NewServer(cfg)
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/drop/", srv.handleDrop)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	payload := []byte("macos-zip-bytes")
	url := ts.URL + "/v1/drop/demo/0.2.0+105/SvnAutoMerge_macos_0.2.0+105.zip"
	req, _ := http.NewRequest(http.MethodPut, url, bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer test-token")
	publishproto.Apply(req.Header)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("drop put status=%d body=%s", resp.StatusCode, body)
	}

	head, err := http.NewRequest(http.MethodHead, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	head.Header.Set("Authorization", "Bearer test-token")
	publishproto.Apply(head.Header)
	respH, err := http.DefaultClient.Do(head)
	if err != nil {
		t.Fatal(err)
	}
	respH.Body.Close()
	if respH.StatusCode != http.StatusOK {
		t.Fatalf("drop head status=%d", respH.StatusCode)
	}

	get, _ := http.NewRequest(http.MethodGet, url, nil)
	get.Header.Set("Authorization", "Bearer test-token")
	publishproto.Apply(get.Header)
	respG, err := http.DefaultClient.Do(get)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(respG.Body)
	respG.Body.Close()
	if respG.StatusCode != http.StatusOK {
		t.Fatalf("drop get status=%d", respG.StatusCode)
	}
	if string(got) != string(payload) {
		t.Fatalf("drop get body=%q", got)
	}

	del, _ := http.NewRequest(http.MethodDelete, url, nil)
	del.Header.Set("Authorization", "Bearer test-token")
	publishproto.Apply(del.Header)
	respD, err := http.DefaultClient.Do(del)
	if err != nil {
		t.Fatal(err)
	}
	respD.Body.Close()
	if respD.StatusCode != http.StatusNoContent {
		t.Fatalf("drop delete status=%d", respD.StatusCode)
	}

	after, _ := http.NewRequest(http.MethodHead, url, nil)
	after.Header.Set("Authorization", "Bearer test-token")
	publishproto.Apply(after.Header)
	respAfter, err := http.DefaultClient.Do(after)
	if err != nil {
		t.Fatal(err)
	}
	respAfter.Body.Close()
	if respAfter.StatusCode != http.StatusNotFound {
		t.Fatalf("drop after delete status=%d", respAfter.StatusCode)
	}
}

func TestExtractRefusesTraversal(t *testing.T) {
	dir := t.TempDir()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	hdr := &tar.Header{Name: "../evil.txt", Mode: 0o644, Size: 4}
	_ = tw.WriteHeader(hdr)
	_, _ = tw.Write([]byte("evil"))
	_ = tw.Close()
	_ = gz.Close()
	arch := filepath.Join(dir, "bad.tgz")
	_ = os.WriteFile(arch, buf.Bytes(), 0o644)
	dest := filepath.Join(dir, "out")
	_ = os.MkdirAll(dest, 0o755)
	if err := extractTarGz(arch, dest, 100); err == nil {
		t.Fatal("expected traversal refusal")
	}
}

func TestExtractAllowsDotSlashRoot(t *testing.T) {
	dir := t.TempDir()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	_ = tw.WriteHeader(&tar.Header{Name: "./", Typeflag: tar.TypeDir, Mode: 0o755})
	payload := []byte("ok")
	_ = tw.WriteHeader(&tar.Header{Name: "./staged.pb", Mode: 0o644, Size: int64(len(payload))})
	_, _ = tw.Write(payload)
	_ = tw.Close()
	_ = gz.Close()
	arch := filepath.Join(dir, "ok.tgz")
	_ = os.WriteFile(arch, buf.Bytes(), 0o644)
	dest := filepath.Join(dir, "out")
	_ = os.MkdirAll(dest, 0o755)
	if err := extractTarGz(arch, dest, 100); err != nil {
		t.Fatalf("extract: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dest, "staged.pb"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "ok" {
		t.Fatalf("got %q", got)
	}
}

type agentFixture struct {
	productRoot string
	profilePath string
	cfg         *Config
	ts          *httptest.Server
	token       string
	tarball     []byte
	ingest      *fakeRelkitServe
	signingSeed []byte

	vNextProject          project.Document
	vNextOperator         operator.Document
	vNextCatalog          catalog.Document
	vNextResolution       resolution.Document
	vNextDistributionKeys map[string]ed25519.PublicKey
}

// newAgentFixture builds the only staged-release shape the agent accepts since
// ADR 0019 §1: a vNext staged tree carrying a signed resolution digest plus its
// four sidecar documents. The release-policy.json control plane and the
// stage.Run pipeline that produced it are gone, so the fixture writes the vNext
// provenance directly instead of staging through a retired path.
func newAgentFixture(t *testing.T, opts agentFixtureOpts) *agentFixture {
	t.Helper()
	if opts.product == "" {
		opts.product = "demo"
	}
	if opts.version == "" {
		opts.version = "1.0.0"
	}
	if opts.channel == "" {
		opts.channel = "dev"
	}
	if opts.identity == "" {
		opts.identity = resolution.IdentityProduction
	}
	root := t.TempDir()
	stateDir := filepath.Join(root, "state")
	productRoot := filepath.Join(root, "product")
	if err := os.MkdirAll(productRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	ingest := startFakeRelkitServe(t, "serve-token")

	seed, err := keys.GenerateSeed()
	if err != nil {
		t.Fatal(err)
	}
	privateDocument := keys.PrivateKeyDocument("k1", seed)
	privateBytes, err := rupv2.MarshalPrivateKey(&privateDocument)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(productRoot, "private.pb"), privateBytes, 0o600); err != nil {
		t.Fatal(err)
	}

	productionSeed := make([]byte, ed25519.SeedSize)
	candidateSeed := make([]byte, ed25519.SeedSize)
	for index := range productionSeed {
		productionSeed[index] = byte(index + 31)
		candidateSeed[index] = byte(index + 131)
	}
	productionPublic := ed25519.NewKeyFromSeed(productionSeed).Public().(ed25519.PublicKey)
	candidatePublic := ed25519.NewKeyFromSeed(candidateSeed).Public().(ed25519.PublicKey)
	productionFingerprint, err := trust.Fingerprint(productionPublic)
	if err != nil {
		t.Fatal(err)
	}
	candidateFingerprint, err := trust.Fingerprint(candidatePublic)
	if err != nil {
		t.Fatal(err)
	}
	releasePublic := ed25519.NewKeyFromSeed(seed).Public().(ed25519.PublicKey)
	releaseFingerprint, err := trust.Fingerprint(releasePublic)
	if err != nil {
		t.Fatal(err)
	}

	productDocument := project.Document{
		Schema: project.Schema, Product: opts.product, Channels: []string{"dev", "stable"},
		OperatorRef: "demo-operator", TrustFingerprint: productionFingerprint,
		Roles:              []project.Role{project.RolePublisher},
		SelectorDimensions: append([]string(nil), opts.selectorDimensions...),
		ArtifactProducer: &project.Command{
			Command: []string{"fixture-producer"}, ExtensionSchema: "relkit.extension/1",
		},
		CapabilityFingerprint: project.CapabilityFingerprint{
			OSArch: []string{"linux-amd64"}, Topology: "producer-only",
		},
	}
	if len(opts.osArch) > 0 {
		productDocument.CapabilityFingerprint.OSArch = append([]string(nil), opts.osArch...)
	}
	if opts.runtimeProfile {
		productDocument.RuntimeProfilePublication = &project.RuntimeProfilePublication{
			RootFingerprint: releaseFingerprint,
			RUPSchemaSet:    "rup.v2",
			Recovery:        map[string]any{"message": "manual install"},
		}
	}
	if opts.site != nil {
		productDocument.Site = opts.site
	}
	if opts.patchProject != nil {
		opts.patchProject(&productDocument)
	}

	distributionDigest := "sha256:" + strings.Repeat("d", 64)
	distributionState := catalog.StateCertified
	certificationSequence := int64(1)
	if opts.identity == resolution.IdentityCandidate {
		distributionState = catalog.StateCandidate
		certificationSequence = 0
	}
	distribution := catalog.Distribution{
		Version: "1.0.0", Commit: strings.Repeat("c", 40), Digest: distributionDigest,
		State: distributionState, CertificationSequence: certificationSequence,
		Components: []catalog.Component{
			{
				Name: "cli", Version: "1.0.0", Platform: "linux-amd64",
				SHA256: distributionDigest,
				Source: catalog.Source{Kind: "release-asset", URL: "https://example.invalid/relkit"},
			},
			{
				Name: "updater", Version: "1.0.0", Platform: "linux-amd64",
				SHA256: distributionDigest,
				Source: catalog.Source{Kind: "release-asset", URL: "https://example.invalid/relkit-updater"},
			},
			{
				Name: "sdk-rust", Version: "1.0.0", Platform: "all",
				SHA256: distributionDigest,
				Source: catalog.Source{Kind: "release-asset", URL: "https://example.invalid/sdk-rust.zip"},
			},
		},
	}
	for _, platform := range opts.osArch {
		if platform == "linux-amd64" {
			continue
		}
		distribution.Components = append(distribution.Components, catalog.Component{
			Name: "cli", Version: "1.0.0", Platform: platform,
			SHA256: distributionDigest,
			Source: catalog.Source{Kind: "release-asset", URL: "https://example.invalid/relkit-" + platform},
		})
	}
	catalogDocument := catalog.Document{
		Schema: catalog.Schema, Sequence: 1,
		Distributions: []catalog.Distribution{distribution},
	}
	operatorDocument := operator.Document{
		Schema: operator.Schema, DeploymentID: "demo-operator",
		ServerRelease: operator.ServerRelease{
			Version: "1.0.0", Commit: distribution.Commit, DistributionDigest: distributionDigest,
		},
		SelectionPolicy: operator.SelectionExact,
		Sequence:        1,
	}
	signingSeed := productionSeed
	signingKeyID := "distribution-root"
	if opts.identity == resolution.IdentityCandidate {
		signingSeed = candidateSeed
		signingKeyID = "candidate-root"
	}
	if err := catalogDocument.Sign(signingKeyID, signingSeed); err != nil {
		t.Fatal(err)
	}
	if err := operatorDocument.Sign(signingKeyID, signingSeed); err != nil {
		t.Fatal(err)
	}
	request := resolution.Request{
		Project: productDocument, Operator: operatorDocument, Catalog: catalogDocument,
		TrustedDistributionKeys: map[string]ed25519.PublicKey{"distribution-root": productionPublic},
		Identity:                opts.identity,
	}
	if opts.identity == resolution.IdentityCandidate {
		request.CandidateDigest = distributionDigest
		request.CandidateRootFingerprint = candidateFingerprint
		request.TrustedCandidateKeys = map[string]ed25519.PublicKey{"candidate-root": candidatePublic}
	}
	resolved, err := resolution.Resolve(request)
	if err != nil {
		t.Fatal(err)
	}
	bundle := resolution.Bundle{
		Schema: resolution.BundleSchema, Project: productDocument,
		Operator: operatorDocument, Catalog: catalogDocument,
		DistributionRoots: resolution.PublicKeys(map[string]ed25519.PublicKey{
			"distribution-root": productionPublic,
		}),
		Resolution: *resolved,
	}
	if opts.identity == resolution.IdentityCandidate {
		bundle.CandidateRoots = resolution.PublicKeys(map[string]ed25519.PublicKey{"candidate-root": candidatePublic})
	}

	inputRoot := filepath.Join(root, "vnext-input")
	if err := os.MkdirAll(inputRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	specs := opts.artifacts
	if len(specs) == 0 {
		specs = []vNextFixtureArtifact{{
			ID: "app", Path: "app.bin", Kind: artifactset.KindBlob,
			Platform: "linux-amd64", Content: []byte("hello"),
		}}
	}
	artifacts := artifactset.Document{
		Schema: artifactset.Schema, ResolutionDigest: resolved.Digest,
		Product: productDocument.Product, Version: opts.version, ReleaseCode: 1, Channel: opts.channel,
	}
	for _, spec := range specs {
		artifactPath := filepath.Join(inputRoot, filepath.FromSlash(spec.Path))
		if err := os.MkdirAll(filepath.Dir(artifactPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(artifactPath, spec.Content, 0o644); err != nil {
			t.Fatal(err)
		}
		artifactHash := sha256.Sum256(spec.Content)
		artifacts.Artifacts = append(artifacts.Artifacts, artifactset.Artifact{
			ID: spec.ID, Path: spec.Path, Kind: spec.Kind, Platform: spec.Platform,
			Selectors: spec.Selectors, Relations: spec.Relations,
			SHA256: "sha256:" + fmt.Sprintf("%x", artifactHash[:]),
			Size:   int64(len(spec.Content)),
		})
	}
	artifactSetPath := filepath.Join(inputRoot, "artifact-set.json")
	bundlePath := filepath.Join(inputRoot, "resolution-bundle.json")
	if err := document.Write(artifactSetPath, artifacts); err != nil {
		t.Fatal(err)
	}
	if err := document.Write(bundlePath, bundle); err != nil {
		t.Fatal(err)
	}
	staged, err := vnextrelease.Stage(productDocument, *resolved, artifacts, vnextrelease.StageOptions{
		ProjectRoot: productRoot, ArtifactSetPath: artifactSetPath,
		ResolutionBundlePath: bundlePath, CandidateRootFingerprint: request.CandidateRootFingerprint,
		Code: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	stagingRoot := stage.StagingDir(productRoot, opts.version)
	if opts.tamperStable {
		stagedBytes, err := os.ReadFile(stage.StagedPath(productRoot, opts.version))
		if err != nil {
			t.Fatal(err)
		}
		tampered, err := rupv2.UnmarshalStaged(stagedBytes)
		if err != nil {
			t.Fatal(err)
		}
		tampered.Channel = "stable"
		if stagedBytes, err = rupv2.MarshalStaged(tampered); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(stage.StagedPath(productRoot, opts.version), stagedBytes, 0o644); err != nil {
			t.Fatal(err)
		}
		artifacts.Channel = "stable"
		if err := document.Write(filepath.Join(stagingRoot, vnextrelease.ArtifactSetFile), artifacts); err != nil {
			t.Fatal(err)
		}
	}
	if opts.tamperReleaseCode {
		artifacts.ReleaseCode++
		if err := document.Write(filepath.Join(stagingRoot, vnextrelease.ArtifactSetFile), artifacts); err != nil {
			t.Fatal(err)
		}
	}
	if opts.omitProvenance {
		stagedBytes, err := os.ReadFile(stage.StagedPath(productRoot, opts.version))
		if err != nil {
			t.Fatal(err)
		}
		stripped, err := rupv2.UnmarshalStaged(stagedBytes)
		if err != nil {
			t.Fatal(err)
		}
		stripped.ResolutionDigest = ""
		stripped.OperatorDeploymentId = ""
		stripped.ResolutionIdentity = ""
		if stagedBytes, err = rupv2.MarshalStaged(stripped); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(stage.StagedPath(productRoot, opts.version), stagedBytes, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if staged == nil {
		t.Fatal("vNext stage returned no staged document")
	}

	tarball, err := tarStaging(stagingRoot)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(stagingRoot); err != nil {
		t.Fatal(err)
	}

	profilePath := filepath.Join(root, "products", opts.product+".json")
	if !opts.omitProfile {
		profile := &config.PublishProfile{
			Product: opts.product,
			Signing: config.PublishSigningProfile{
				KeyID: "k1", PrivateKeyPath: "private.pb",
			},
			Backends:  map[string]map[string]any{"serve": ingest.backendConfig()},
			PublishTo: []string{"serve"},
			Directory: &config.PublishDirectoryProfile{PublishTo: []string{"serve"}},
		}
		if opts.patchProfile != nil {
			opts.patchProfile(profile)
		}
		if err := os.MkdirAll(filepath.Dir(profilePath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := document.Write(profilePath, profile); err != nil {
			t.Fatal(err)
		}
	}

	productCfg := map[string]any{
		"root":                        productRoot,
		"distributionRootFingerprint": productionFingerprint,
		"operatorDeploymentId":        "demo-operator",
		"lane":                        string(opts.identity),
	}
	if opts.identity == resolution.IdentityCandidate {
		productCfg["candidateRootFingerprint"] = candidateFingerprint
	}
	if !opts.omitProfile {
		productCfg["profile"] = profilePath
	}
	agentCfgPath := filepath.Join(root, "agent.json")
	tokenPath := filepath.Join(root, "tokens", opts.product+".token")
	if err := os.MkdirAll(filepath.Dir(tokenPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tokenPath, []byte("test-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	agentDoc := map[string]any{
		"stateDir": stateDir,
		"uploadTokens": []any{
			map[string]any{
				"file":     filepath.ToSlash(filepath.Join("tokens", opts.product+".token")),
				"products": []any{opts.product},
			},
		},
		"products": map[string]any{
			opts.product: productCfg,
		},
	}
	agentBytes, _ := json.MarshalIndent(agentDoc, "", "  ")
	if err := os.WriteFile(agentCfgPath, agentBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(agentCfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if opts.tinyParts {
		cfg.MinPartSize = 1
		cfg.PartSize = 32
		cfg.MaxPartSize = 256
		cfg.MaxPartConcurrency = 4
	}
	ts := httptest.NewServer(NewServer(cfg).Handler())
	t.Cleanup(ts.Close)

	return &agentFixture{
		productRoot:           productRoot,
		profilePath:           profilePath,
		cfg:                   cfg,
		ts:                    ts,
		token:                 "test-token",
		tarball:               tarball,
		ingest:                ingest,
		signingSeed:           seed,
		vNextProject:          productDocument,
		vNextOperator:         operatorDocument,
		vNextCatalog:          catalogDocument,
		vNextResolution:       *resolved,
		vNextDistributionKeys: map[string]ed25519.PublicKey{"distribution-root": productionPublic},
	}
}

type agentFixtureOpts struct {
	product            string
	version            string
	channel            string
	identity           resolution.Identity
	omitProfile        bool
	patchProfile       func(*config.PublishProfile)
	site               *project.Site
	patchProject       func(*project.Document)
	tamperStable       bool
	tamperReleaseCode  bool
	omitProvenance     bool
	runtimeProfile     bool
	osArch             []string
	selectorDimensions []string
	artifacts          []vNextFixtureArtifact
	tinyParts          bool
}

type vNextFixtureOptions struct {
	identity           resolution.Identity
	tamperStable       bool
	tamperReleaseCode  bool
	runtimeProfile     bool
	osArch             []string
	selectorDimensions []string
	artifacts          []vNextFixtureArtifact
}

type vNextFixtureArtifact struct {
	ID        string
	Path      string
	Kind      artifactset.Kind
	Platform  string
	Selectors map[string]string
	Relations artifactset.Relations
	Content   []byte
}

// makeVNextAgentFixture is the historical spelling of the vNext fixture. It now
// forwards to newAgentFixture, which builds a vNext staged tree by default, so
// both entry points produce the same agent and the call sites did not have to
// be rewritten when ADR 0019 retired the legacy staged shape.
func makeVNextAgentFixture(t *testing.T, options vNextFixtureOptions) *agentFixture {
	t.Helper()
	identity := options.identity
	if identity == "" {
		identity = resolution.IdentityProduction
	}
	return newAgentFixture(t, agentFixtureOpts{
		identity:           identity,
		tamperStable:       options.tamperStable,
		tamperReleaseCode:  options.tamperReleaseCode,
		runtimeProfile:     options.runtimeProfile,
		osArch:             options.osArch,
		selectorDimensions: options.selectorDimensions,
		artifacts:          options.artifacts,
	})
}

func tarStaging(stagedRoot string) ([]byte, error) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	err := filepath.Walk(stagedRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(stagedRoot, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		hdr, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		hdr.Name = filepath.ToSlash(rel)
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if info.Mode().IsRegular() {
			f, err := os.Open(path)
			if err != nil {
				return err
			}
			defer f.Close()
			_, err = io.Copy(tw, f)
			return err
		}
		return nil
	})
	_ = tw.Close()
	_ = gz.Close()
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (f *agentFixture) putStaged(t *testing.T, product, version string) (int, []byte) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPut, f.ts.URL+"/v1/staged/"+product+"/"+version, bytes.NewReader(f.tarball))
	req.Header.Set("Authorization", "Bearer "+f.token)
	publishproto.Apply(req.Header)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, body
}

func (f *agentFixture) publish(t *testing.T, body string) (int, []byte) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, f.ts.URL+"/v1/publish", bytes.NewReader([]byte(body)))
	req.Header.Set("Authorization", "Bearer "+f.token)
	publishproto.Apply(req.Header)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, out
}

func TestMergeVNextPublishConfigDerivesStandardDirectoryURLs(t *testing.T) {
	root := t.TempDir()
	seed, err := keys.GenerateSeed()
	if err != nil {
		t.Fatal(err)
	}
	privateDocument := keys.PrivateKeyDocument("k1", seed)
	privateBytes, err := rupv2.MarshalPrivateKey(&privateDocument)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "private.pb"), privateBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	productDocument := project.Document{
		Schema: project.Schema, Product: "demo", Channels: []string{"dev", "stable"},
	}
	profile := &config.PublishProfile{
		Product: "demo",
		Signing: config.PublishSigningProfile{
			KeyID: "k1", PrivateKeyPath: "private.pb",
		},
		Backends: map[string]map[string]any{
			"primary": {"type": "relkit-compatible", "baseUrl": "https://updates.example.invalid/root"},
		},
		PublishTo: []string{"primary"},
		Directory: &config.PublishDirectoryProfile{
			PublishTo: []string{"primary"},
		},
	}
	cfg, err := mergeVNextPublishConfig(&productDocument, profile, root)
	if err != nil {
		t.Fatal(err)
	}
	trusted, err := cfg.TrustedPublicKeys()
	if err != nil {
		t.Fatal(err)
	}
	wantPublic := ed25519.NewKeyFromSeed(seed).Public().(ed25519.PublicKey)
	if !slices.Equal(trusted["k1"], wantPublic) {
		t.Fatal("derived public key does not match the configured private signer")
	}
	if got, want := cfg.Directory.EntryURLs, []string{"https://updates.example.invalid/root/directory/demo.pb"}; !slices.Equal(got, want) {
		t.Fatalf("entry URLs=%v, want %v", got, want)
	}
	if len(cfg.Directory.Services) != 2 {
		t.Fatalf("services=%+v", cfg.Directory.Services)
	}
	if cfg.Directory.Services[0].IndexURL != "https://updates.example.invalid/root/index/demo/dev.pb" ||
		cfg.Directory.Services[1].IndexURL != "https://updates.example.invalid/root/index/demo/stable.pb" {
		t.Fatalf("services=%+v", cfg.Directory.Services)
	}
}

// TestMergeVNextPublishConfigCarriesSite: the regression that made every
// migrated product vanish from the rendered catalog. A vNext product has no
// relkit.json, so this merge is the only place its site declaration can reach
// the publisher. When it is dropped, publish.hasSiteCopy is false and
// site/<product>.json stops being written while the protocol documents keep
// flowing, which is invisible until someone opens the update page.
func TestMergeVNextPublishConfigCarriesSite(t *testing.T) {
	root := t.TempDir()
	seed, err := keys.GenerateSeed()
	if err != nil {
		t.Fatal(err)
	}
	privateDocument := keys.PrivateKeyDocument("k1", seed)
	privateBytes, err := rupv2.MarshalPrivateKey(&privateDocument)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "private.pb"), privateBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	profile := &config.PublishProfile{
		Product: "demo",
		Signing: config.PublishSigningProfile{KeyID: "k1", PrivateKeyPath: "private.pb"},
		Backends: map[string]map[string]any{
			"primary": {"type": "relkit-compatible", "baseUrl": "https://updates.example.invalid/root"},
		},
		PublishTo: []string{"primary"},
		Directory: &config.PublishDirectoryProfile{
			PublishTo: []string{"primary"},
		},
	}

	declared := project.Document{
		Schema: project.Schema, Product: "demo", Channels: []string{"dev", "stable"},
		Site: &project.Site{Title: "Demo", Description: "A demo product.", Homepage: "https://example.com/demo"},
	}
	cfg, err := mergeVNextPublishConfig(&declared, profile, root)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Site.Title != "Demo" || cfg.Site.Description != "A demo product." || cfg.Site.Homepage != "https://example.com/demo" {
		t.Fatalf("site did not reach the publish config: %+v", cfg.Site)
	}

	// A product that does not declare site must stay opted out, not inherit
	// an empty block that publish would then have to second-guess.
	optedOut := project.Document{
		Schema: project.Schema, Product: "demo", Channels: []string{"dev", "stable"},
	}
	cfg, err = mergeVNextPublishConfig(&optedOut, profile, root)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Site.Title != "" || cfg.Site.Description != "" || cfg.Site.Homepage != "" {
		t.Fatalf("undeclared site must stay empty: %+v", cfg.Site)
	}
}

func TestAgentBootstrapsRuntimeProfileWithoutExposingPrivateKey(t *testing.T) {
	fx := makeVNextAgentFixture(t, vNextFixtureOptions{runtimeProfile: true})
	payload, err := json.Marshal(runtimeProfileRequest{
		Product: "demo", Project: fx.vNextProject,
	})
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest(http.MethodPost, fx.ts.URL+"/v1/runtime-profile", bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+fx.token)
	req.Header.Set("Content-Type", "application/json")
	publishproto.Apply(req.Header)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.StatusCode, raw)
	}
	var result struct {
		OK                bool                    `json:"ok"`
		Profile           runtimeprofile.Document `json:"profile"`
		DirectorySequence int64                   `json:"directorySequence"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	if !result.OK || result.Profile.Product != "demo" || result.DirectorySequence < 1 {
		t.Fatalf("result=%+v", result)
	}
	publicKey := ed25519.NewKeyFromSeed(fx.signingSeed).Public().(ed25519.PublicKey)
	if err := result.Profile.Verify(map[string]ed25519.PublicKey{"k1": publicKey}); err != nil {
		t.Fatalf("profile signature: %v", err)
	}
	if !fx.ingest.hasPrefix("runtime-profile/demo/") {
		t.Fatal("Runtime Profile immutable object was not published")
	}
	if rawDirectory := fx.ingest.get("directory/demo.pb"); len(rawDirectory) == 0 {
		t.Fatal("directory pointer was not published")
	}
	if bytes.Contains(raw, fx.signingSeed) || bytes.Contains(raw, []byte("private.pb")) {
		t.Fatal("Runtime Profile response exposed private signing material")
	}
}

// TestAgentRejectsProjectProductMismatch covers the vNext replacement for the
// retired release-policy.json check: the staged project document, not a policy
// sidecar, must name the product in the route.
func TestAgentRejectsProjectProductMismatch(t *testing.T) {
	fx := newAgentFixture(t, agentFixtureOpts{
		patchProject: func(document *project.Document) {
			document.Product = "other"
		},
	})
	status, body := fx.putStaged(t, "demo", "1.0.0")
	if status != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", status, body)
	}
	if !bytes.Contains(body, []byte("does not match")) {
		t.Fatalf("body=%s", body)
	}
}

func TestAgentPublishStagedSHA256MatchAndMismatch(t *testing.T) {
	fx := newAgentFixture(t, agentFixtureOpts{})
	status, body := fx.putStaged(t, "demo", "1.0.0")
	if status != http.StatusCreated {
		t.Fatalf("staged status=%d body=%s", status, body)
	}
	var stagedResp map[string]any
	if err := json.Unmarshal(body, &stagedResp); err != nil {
		t.Fatal(err)
	}
	sum, _ := stagedResp["sha256"].(string)
	if sum == "" {
		t.Fatalf("missing sha256: %s", body)
	}

	status, body = fx.publish(t, `{"product":"demo","version":"1.0.0","dryRun":true,"stagedSha256":"`+sum+`"}`)
	if status != http.StatusOK {
		t.Fatalf("matching sha status=%d body=%s", status, body)
	}

	status, body = fx.publish(t, `{"product":"demo","version":"1.0.0","dryRun":true,"stagedSha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`)
	if status != http.StatusConflict {
		t.Fatalf("mismatch status=%d body=%s", status, body)
	}
	if !bytes.Contains(body, []byte("staged sha256 mismatch")) {
		t.Fatalf("body=%s", body)
	}

	status, body = fx.publish(t, `{"product":"demo","version":"1.0.0","dryRun":true,"stagedSha256":"short"}`)
	if status != http.StatusConflict {
		t.Fatalf("short sha status=%d body=%s", status, body)
	}
}

func TestAgentPublishDirectoryFailureIsNotSuccess(t *testing.T) {
	// ADR 0019 §1: the directory target list is derived from the publish
	// profile's backends, so a failing directory PUT is the only knob left.
	fx := newAgentFixture(t, agentFixtureOpts{})
	status, body := fx.putStaged(t, "demo", "1.0.0")
	if status != http.StatusCreated {
		t.Fatalf("staged status=%d body=%s", status, body)
	}
	fx.ingest.failPutsWithPrefix("directory/")
	status, body = fx.publish(t, `{"product":"demo","version":"1.0.0","idempotencyKey":"directory-failure"}`)
	if status != http.StatusConflict {
		t.Fatalf("publish status=%d body=%s", status, body)
	}
	var response map[string]any
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatal(err)
	}
	if response["ok"] != false || response["code"] != "release.directory_partial" {
		t.Fatalf("unexpected response: %s", body)
	}
	if len(fx.ingest.get("index/demo/dev.pb")) == 0 {
		t.Fatal("test did not reach the index commit point")
	}
	if _, ok := fx.cfg.Products["demo"]; !ok {
		t.Fatal("fixture lost product config")
	}
}

func TestAgentRejectsAllowPartial(t *testing.T) {
	fx := newAgentFixture(t, agentFixtureOpts{})
	status, body := fx.publish(t, `{"product":"demo","version":"1.0.0","allowPartial":true}`)
	if status != http.StatusBadRequest || !bytes.Contains(body, []byte("release.allow_partial_retired")) {
		t.Fatalf("status=%d body=%s", status, body)
	}
}

func TestAgentReportsPartialCommitWhenLaterBackendFails(t *testing.T) {
	failing := startFakeRelkitServe(t, "serve-token")
	failing.failPutsWithPrefix("index/")
	fx := newAgentFixture(t, agentFixtureOpts{
		patchProfile: func(profile *config.PublishProfile) {
			profile.Backends["failing"] = failing.backendConfig()
			profile.PublishTo = []string{"serve", "failing"}
		},
	})
	status, body := fx.putStaged(t, "demo", "1.0.0")
	if status != http.StatusCreated {
		t.Fatalf("staged status=%d body=%s", status, body)
	}
	status, body = fx.publish(t, `{"product":"demo","version":"1.0.0"}`)
	if status != http.StatusConflict {
		t.Fatalf("publish status=%d body=%s", status, body)
	}
	if !strings.Contains(string(body), "release.partial_commit") {
		t.Fatalf("missing partial-commit code: %s", body)
	}
	if len(fx.ingest.get("index/demo/dev.pb")) == 0 {
		t.Fatalf("first backend did not commit before later backend failed")
	}
	if len(failing.get("index/demo/dev.pb")) != 0 {
		t.Fatalf("failing backend unexpectedly committed")
	}
}

func TestAgentTreatsWebProjectionFailureAsRebuildable(t *testing.T) {
	// A vNext product declares its human-facing site block in
	// relkit.project.json; there is no release-policy.json to patch anymore.
	fx := newAgentFixture(t, agentFixtureOpts{
		site: &project.Site{Title: "Demo"},
	})
	status, body := fx.putStaged(t, "demo", "1.0.0")
	if status != http.StatusCreated {
		t.Fatalf("staged status=%d body=%s", status, body)
	}
	fx.ingest.failPutsWithPrefix("site/")
	status, body = fx.publish(t, `{"product":"demo","version":"1.0.0"}`)
	if status != http.StatusOK {
		t.Fatalf("publish status=%d body=%s", status, body)
	}
	var response map[string]any
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatal(err)
	}
	if response["ok"] != true {
		t.Fatalf("web projection failure changed core result: %s", body)
	}
	logs, _ := response["log"].([]any)
	foundWarning := false
	for _, line := range logs {
		if strings.Contains(fmt.Sprint(line), "web projection failed") {
			foundWarning = true
		}
	}
	if !foundWarning {
		t.Fatalf("publish did not report rebuildable projection warning: %s", body)
	}
}

func TestAgentPublishesAndVerifiesVNextRelease(t *testing.T) {
	fx := makeVNextAgentFixture(t, vNextFixtureOptions{identity: resolution.IdentityProduction})
	status, body := fx.putStaged(t, "demo", "1.0.0")
	if status != http.StatusCreated {
		t.Fatalf("staged status=%d body=%s", status, body)
	}
	status, body = fx.publish(t, `{"product":"demo","version":"1.0.0"}`)
	if status != http.StatusOK {
		t.Fatalf("publish status=%d body=%s", status, body)
	}
	if len(fx.ingest.get("directory/demo.pb")) == 0 || len(fx.ingest.get("index/demo/dev.pb")) == 0 {
		t.Fatalf("consumer-visible vNext documents were not committed")
	}
}

func TestScenarioCASPutPreservesVNextControlPlane(t *testing.T) {
	fx := makeVNextAgentFixture(t, vNextFixtureOptions{identity: resolution.IdentityProduction})
	clientRoot := t.TempDir()
	archivePath := filepath.Join(clientRoot, "staged.tar.gz")
	if err := os.WriteFile(archivePath, fx.tarball, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := extractTarGz(archivePath, stage.StagingDir(clientRoot, "1.0.0"), 100); err != nil {
		t.Fatal(err)
	}

	result, err := casput.Put(t.Context(), casput.Options{
		Root:       clientRoot,
		Product:    "demo",
		Version:    "1.0.0",
		URL:        fx.ts.URL,
		Token:      fx.token,
		HTTPClient: fx.ts.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Uploaded != 1 || result.StagedSHA256 == "" {
		t.Fatalf("CAS put result=%+v", result)
	}

	status, body := fx.publish(t, `{"product":"demo","version":"1.0.0","dryRun":true}`)
	if status != http.StatusOK {
		t.Fatalf("publish status=%d body=%s", status, body)
	}
}

func TestFrozenFieldUpdaterReadsVNextPublishedRUP(t *testing.T) {
	fx := makeVNextAgentFixture(t, vNextFixtureOptions{identity: resolution.IdentityProduction})
	status, body := fx.putStaged(t, "demo", "1.0.0")
	if status != http.StatusCreated {
		t.Fatalf("staged status=%d body=%s", status, body)
	}
	status, body = fx.publish(t, `{"product":"demo","version":"1.0.0"}`)
	if status != http.StatusOK {
		t.Fatalf("publish status=%d body=%s", status, body)
	}
	publicKey := ed25519.NewKeyFromSeed(fx.signingSeed).Public().(ed25519.PublicKey)
	fieldUpdater := &inprocess.Updater{
		Product:     "demo",
		Channel:     "dev",
		CurrentCode: 0,
		EntryURLs:   []string{strings.TrimRight(fx.ingest.URL, "/") + "/directory/demo.pb"},
		TrustedKeys: sdk.TrustedKeys{"k1": publicKey},
		StateStore:  sdk.NewMemoryStateStore(&sdk.UpdateState{}),
	}
	result := fieldUpdater.CheckForce(t.Context(), true)
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	if result.Available == nil || result.Available.Target.GetVersion() != "1.0.0" ||
		result.Available.Artifact.GetId() != "app" {
		t.Fatalf("field updater result=%+v", result)
	}
}

func TestVNextVerifyReleasedReadsConsumerVisibleDocuments(t *testing.T) {
	fx := makeVNextAgentFixture(t, vNextFixtureOptions{identity: resolution.IdentityProduction})
	publishVNextFixture(t, fx)
	publicKey := ed25519.NewKeyFromSeed(fx.signingSeed).Public().(ed25519.PublicKey)
	findings, err := verify.RunReleasedHTTP(t.Context(), verify.ReleasedHTTPOptions{
		Product: "demo", Channel: "dev", Version: "1.0.0",
		ConsumerBaseURL: fx.ingest.URL,
		DirectoryURL:    fx.ingest.URL,
		TrustedProductRoots: map[string]ed25519.PublicKey{
			"k1": publicKey,
		},
		ResolutionDigest:     fx.vNextResolution.Digest,
		OperatorDeploymentID: fx.vNextResolution.Operator.DeploymentID,
		ResolutionIdentity:   string(fx.vNextResolution.Identity),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !findings.OK() {
		t.Fatalf("findings=%+v", findings)
	}
}

func TestScenarioS1MultiPlatformAsymmetryAndApplySelection(t *testing.T) {
	fx := makeVNextAgentFixture(t, vNextFixtureOptions{
		identity:           resolution.IdentityProduction,
		osArch:             []string{"windows-amd64", "darwin-arm64"},
		selectorDimensions: []string{"os", "arch"},
		artifacts: []vNextFixtureArtifact{
			{
				ID: "windows-installer", Path: "demo-setup.exe", Kind: artifactset.KindInstaller,
				Platform: "windows-amd64", Selectors: map[string]string{"os": "windows", "arch": "amd64"},
				Content: []byte("windows installer"),
			},
			{
				ID: "windows-payload", Path: "demo-payload.zip", Kind: artifactset.KindPayload,
				Platform: "windows-amd64", Selectors: map[string]string{"os": "windows", "arch": "amd64"},
				Relations: artifactset.Relations{PayloadOf: "windows-installer"},
				Content:   payloadFixture(t, "demo.exe", "windows payload"),
			},
			{
				ID: "macos-installer", Path: "demo.dmg", Kind: artifactset.KindInstaller,
				Platform: "darwin-arm64", Selectors: map[string]string{"os": "darwin", "arch": "arm64"},
				Content: []byte("macos installer"),
			},
		},
	})
	publishVNextFixture(t, fx)
	assertPublishedArtifact(t, fx, map[string]string{
		"os": "windows", "arch": "amd64", "apply": "relkit-payload",
	}, "windows-payload")
	assertPublishedArtifact(t, fx, map[string]string{
		"os": "windows", "arch": "amd64",
	}, "windows-installer")
	assertPublishedArtifact(t, fx, map[string]string{
		"os": "darwin", "arch": "arm64", "apply": "relkit-payload",
	}, "macos-installer")
}

func TestScenarioS2ManagedProfileFeedsManagerResolution(t *testing.T) {
	fx := makeVNextAgentFixture(t, vNextFixtureOptions{
		identity: resolution.IdentityProduction, runtimeProfile: true,
	})
	publishVNextFixture(t, fx)
	directoryEnvelope, err := rupv2.UnmarshalEnvelope(fx.ingest.get("directory/demo.pb"))
	if err != nil {
		t.Fatal(err)
	}
	directoryDocument, err := rupv2.UnmarshalDirectory(directoryEnvelope.Payload)
	if err != nil {
		t.Fatal(err)
	}
	reference := directoryDocument.GetRuntimeProfile()
	if reference == nil || reference.GetSha256() == "" {
		t.Fatal("managed product directory did not pin a Runtime Profile")
	}
	profileBytes := fx.ingest.get("runtime-profile/demo/" + reference.GetSha256() + ".json")
	var profile runtimeprofile.Document
	if err := document.Decode(profileBytes, &profile); err != nil {
		t.Fatal(err)
	}
	runtimePublic := ed25519.NewKeyFromSeed(fx.signingSeed).Public().(ed25519.PublicKey)
	runtimeFingerprint, err := trust.Fingerprint(runtimePublic)
	if err != nil {
		t.Fatal(err)
	}
	manager := project.Document{
		Schema: project.Schema, Product: "manager", Channels: []string{"dev", "stable"},
		OperatorRef: fx.vNextProject.OperatorRef, TrustFingerprint: fx.vNextProject.TrustFingerprint,
		Roles: []project.Role{project.RoleUpdaterHost, project.RoleSubscriber},
		RuntimeProfileSubscriptions: []project.Subscription{{
			ProductRef: "demo", RootFingerprint: runtimeFingerprint,
		}},
		CapabilityFingerprint: project.CapabilityFingerprint{
			OSArch: []string{"linux-amd64"}, Facades: []string{"rust"}, Topology: "manager",
			Subscriptions: []string{"demo"},
		},
	}
	resolved, err := resolution.Resolve(resolution.Request{
		Project: manager, Operator: fx.vNextOperator, Catalog: fx.vNextCatalog,
		TrustedDistributionKeys: fx.vNextDistributionKeys,
		RuntimeProfiles:         map[string]runtimeprofile.Document{"demo": profile},
		TrustedRuntimeRoots: map[string]map[string]ed25519.PublicKey{
			"demo": {"k1": runtimePublic},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resolved.RuntimeProfileDigests["demo"] != profile.Digest {
		t.Fatalf("manager resolved profile %q, want %q", resolved.RuntimeProfileDigests["demo"], profile.Digest)
	}
}

func TestScenarioS3ComponentAudienceSelection(t *testing.T) {
	fx := makeVNextAgentFixture(t, vNextFixtureOptions{
		identity:           resolution.IdentityProduction,
		selectorDimensions: []string{"component", "audience"},
		artifacts: []vNextFixtureArtifact{
			{
				ID: "server-runtime", Path: "server-runtime.zip", Kind: artifactset.KindBlob,
				Platform: "linux-amd64", Selectors: map[string]string{"component": "server", "audience": "runtime"},
				Content: []byte("server runtime"),
			},
			{
				ID: "server-user", Path: "server-user.zip", Kind: artifactset.KindBlob,
				Platform: "linux-amd64", Selectors: map[string]string{"component": "server", "audience": "user"},
				Content: []byte("server user"),
			},
			{
				ID: "console-runtime", Path: "console-runtime.zip", Kind: artifactset.KindBlob,
				Platform: "linux-amd64", Selectors: map[string]string{"component": "console", "audience": "runtime"},
				Content: []byte("console runtime"),
			},
			{
				ID: "console-user", Path: "console-user.zip", Kind: artifactset.KindBlob,
				Platform: "linux-amd64", Selectors: map[string]string{"component": "console", "audience": "user"},
				Content: []byte("console user"),
			},
		},
	})
	publishVNextFixture(t, fx)
	for _, test := range []struct {
		component string
		audience  string
		want      string
	}{
		{component: "server", audience: "runtime", want: "server-runtime"},
		{component: "server", audience: "user", want: "server-user"},
		{component: "console", audience: "runtime", want: "console-runtime"},
		{component: "console", audience: "user", want: "console-user"},
	} {
		assertPublishedArtifact(t, fx, map[string]string{
			"component": test.component, "audience": test.audience, "apply": "relkit-payload",
		}, test.want)
	}
}

func publishVNextFixture(t *testing.T, fx *agentFixture) {
	t.Helper()
	status, body := fx.putStaged(t, "demo", "1.0.0")
	if status != http.StatusCreated {
		t.Fatalf("staged status=%d body=%s", status, body)
	}
	status, body = fx.publish(t, `{"product":"demo","version":"1.0.0"}`)
	if status != http.StatusOK {
		t.Fatalf("publish status=%d body=%s", status, body)
	}
}

func assertPublishedArtifact(t *testing.T, fx *agentFixture, clientSelectors map[string]string, want string) {
	t.Helper()
	publicKey := ed25519.NewKeyFromSeed(fx.signingSeed).Public().(ed25519.PublicKey)
	fieldUpdater := &inprocess.Updater{
		Product:         "demo",
		Channel:         "dev",
		CurrentCode:     0,
		EntryURLs:       []string{strings.TrimRight(fx.ingest.URL, "/") + "/directory/demo.pb"},
		TrustedKeys:     sdk.TrustedKeys{"k1": publicKey},
		ClientSelectors: clientSelectors,
		StateStore:      sdk.NewMemoryStateStore(&sdk.UpdateState{}),
	}
	result := fieldUpdater.CheckForce(t.Context(), true)
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	if result.Available == nil || result.Available.Artifact.GetId() != want {
		t.Fatalf("selectors=%v selected %+v, want %s", clientSelectors, result.Available, want)
	}
}

func payloadFixture(t *testing.T, name, content string) []byte {
	t.Helper()
	tree := t.TempDir()
	path := filepath.Join(tree, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "payload.zip")
	if _, err := payload.Build(out, payload.BuildOptions{Tree: tree}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestAgentDoesNotReturnSuccessWhenReleasedVerificationFails(t *testing.T) {
	fx := makeVNextAgentFixture(t, vNextFixtureOptions{identity: resolution.IdentityProduction})
	status, body := fx.putStaged(t, "demo", "1.0.0")
	if status != http.StatusCreated {
		t.Fatalf("staged status=%d body=%s", status, body)
	}
	fx.ingest.corruptGetsWithPrefix("index/")
	status, body = fx.publish(t, `{"product":"demo","version":"1.0.0"}`)
	if status != http.StatusConflict || !strings.Contains(string(body), "release.verify_failed") {
		t.Fatalf("publish status=%d body=%s", status, body)
	}
}

func TestAgentCandidateUsesIndependentRootAndCannotStageStable(t *testing.T) {
	fx := makeVNextAgentFixture(t, vNextFixtureOptions{
		identity: resolution.IdentityCandidate, tamperStable: true,
	})
	status, body := fx.putStaged(t, "demo", "1.0.0")
	if status != http.StatusBadRequest || !strings.Contains(string(body), "does not allow channel") {
		t.Fatalf("candidate stable upload status=%d body=%s", status, body)
	}
}

func TestAgentRejectsArtifactSetReleaseCodeTampering(t *testing.T) {
	fx := makeVNextAgentFixture(t, vNextFixtureOptions{
		identity: resolution.IdentityProduction, tamperReleaseCode: true,
	})
	status, body := fx.putStaged(t, "demo", "1.0.0")
	if status != http.StatusBadRequest || !strings.Contains(string(body), "releaseCode") {
		t.Fatalf("releaseCode tamper status=%d body=%s", status, body)
	}
}

func TestAgentAcceptsAuthorizedCandidateOnDevLane(t *testing.T) {
	fx := makeVNextAgentFixture(t, vNextFixtureOptions{identity: resolution.IdentityCandidate})
	status, body := fx.putStaged(t, "demo", "1.0.0")
	if status != http.StatusCreated {
		t.Fatalf("candidate staged status=%d body=%s", status, body)
	}
	status, body = fx.publish(t, `{"product":"demo","version":"1.0.0"}`)
	if status != http.StatusOK {
		t.Fatalf("candidate publish status=%d body=%s", status, body)
	}
	if len(fx.ingest.get("index/demo/stable.pb")) != 0 {
		t.Fatal("candidate lane wrote stable index")
	}
}

func TestAgentPublishesRuntimeProfileBeforeDirectoryCommit(t *testing.T) {
	fx := makeVNextAgentFixture(t, vNextFixtureOptions{
		identity: resolution.IdentityProduction, runtimeProfile: true,
	})
	status, body := fx.putStaged(t, "demo", "1.0.0")
	if status != http.StatusCreated {
		t.Fatalf("staged status=%d body=%s", status, body)
	}
	status, body = fx.publish(t, `{"product":"demo","version":"1.0.0"}`)
	if status != http.StatusOK {
		t.Fatalf("publish status=%d body=%s", status, body)
	}
	if !fx.ingest.hasPrefix("runtime-profile/demo/") {
		t.Fatal("Runtime Profile was not published")
	}
	directoryBytes := fx.ingest.get("directory/demo.pb")
	directoryEnvelope, err := rupv2.UnmarshalEnvelope(directoryBytes)
	if err != nil {
		t.Fatal(err)
	}
	directoryDocument, err := rupv2.UnmarshalDirectory(directoryEnvelope.Payload)
	if err != nil {
		t.Fatal(err)
	}
	if directoryDocument.RuntimeProfile == nil || directoryDocument.RuntimeProfile.Sha256 == "" {
		t.Fatalf("directory does not pin Runtime Profile: %+v", directoryDocument)
	}
}

func TestAgentRuntimeProfileFailurePreventsIndexCommit(t *testing.T) {
	fx := makeVNextAgentFixture(t, vNextFixtureOptions{
		identity: resolution.IdentityProduction, runtimeProfile: true,
	})
	status, body := fx.putStaged(t, "demo", "1.0.0")
	if status != http.StatusCreated {
		t.Fatalf("staged status=%d body=%s", status, body)
	}
	fx.ingest.failPutsWithPrefix("runtime-profile/")
	status, body = fx.publish(t, `{"product":"demo","version":"1.0.0"}`)
	if status != http.StatusConflict || !strings.Contains(string(body), "release.runtime_profile_failed") {
		t.Fatalf("publish status=%d body=%s", status, body)
	}
	if len(fx.ingest.get("index/demo/dev.pb")) != 0 {
		t.Fatal("index committed after Runtime Profile failure")
	}
}

func TestAgentPublishRequiresProfileWhenPolicyPresent(t *testing.T) {
	fx := newAgentFixture(t, agentFixtureOpts{omitProfile: true})
	status, body := fx.putStaged(t, "demo", "1.0.0")
	if status != http.StatusCreated {
		t.Fatalf("staged status=%d body=%s", status, body)
	}
	status, body = fx.publish(t, `{"product":"demo","version":"1.0.0","dryRun":true}`)
	if status == http.StatusOK {
		t.Fatalf("expected missing profile error, body=%s", body)
	}
	if !bytes.Contains(body, []byte("profile")) {
		t.Fatalf("body=%s", body)
	}
}

// TestAgentStagedRequiresResolutionDigest is the vNext successor of the retired
// "staged tree requires release-policy.json" test. Without the signed
// resolution digest there is no proof of what the tree contains, so the upload
// is refused and the extracted tree is cleaned up.
func TestAgentStagedRequiresResolutionDigest(t *testing.T) {
	fx := newAgentFixture(t, agentFixtureOpts{omitProvenance: true})
	status, body := fx.putStaged(t, "demo", "1.0.0")
	if status != http.StatusBadRequest {
		t.Fatalf("staged status=%d body=%s", status, body)
	}
	if !bytes.Contains(body, []byte("resolution digest")) {
		t.Fatalf("body=%s", body)
	}
	if _, err := os.Stat(stage.StagingDir(fx.productRoot, "1.0.0")); !os.IsNotExist(err) {
		t.Fatalf("staged tree should be rejected and cleaned: %v", err)
	}
}

func TestAgentPublishRejectsPolicyProfileConflicts(t *testing.T) {
	t.Run("product", func(t *testing.T) {
		fx := newAgentFixture(t, agentFixtureOpts{
			patchProfile: func(profile *config.PublishProfile) {
				profile.Product = "other"
			},
		})
		status, body := fx.putStaged(t, "demo", "1.0.0")
		if status != http.StatusCreated {
			t.Fatalf("staged status=%d body=%s", status, body)
		}
		status, body = fx.publish(t, `{"product":"demo","version":"1.0.0","dryRun":true}`)
		if status == http.StatusOK {
			t.Fatalf("expected product mismatch, body=%s", body)
		}
		if !bytes.Contains(body, []byte("product")) {
			t.Fatalf("body=%s", body)
		}
	})
	t.Run("keyId", func(t *testing.T) {
		fx := newAgentFixture(t, agentFixtureOpts{
			patchProfile: func(profile *config.PublishProfile) {
				profile.Signing.KeyID = "k2"
			},
		})
		status, body := fx.putStaged(t, "demo", "1.0.0")
		if status != http.StatusCreated {
			t.Fatalf("staged status=%d body=%s", status, body)
		}
		status, body = fx.publish(t, `{"product":"demo","version":"1.0.0","dryRun":true}`)
		if status == http.StatusOK {
			t.Fatalf("expected keyId mismatch, body=%s", body)
		}
		if !bytes.Contains(body, []byte("keyId")) {
			t.Fatalf("body=%s", body)
		}
	})
}

func TestHealthJSONOmitsUploadToken(t *testing.T) {
	fx := newAgentFixture(t, agentFixtureOpts{})
	resp, err := http.Get(fx.ts.URL + "/-/health")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.StatusCode, body)
	}
	if bytes.Contains(body, []byte(fx.token)) || bytes.Contains(body, []byte("test-token")) {
		t.Fatalf("health leaked token: %s", body)
	}
	if !bytes.Contains(body, []byte(`"ok":true`)) && !bytes.Contains(body, []byte(`"ok": true`)) {
		t.Fatalf("health body=%s", body)
	}
}

func TestProductTokenCannotWriteOtherProduct(t *testing.T) {
	fx := newAgentFixture(t, agentFixtureOpts{})
	req, _ := http.NewRequest(http.MethodPut, fx.ts.URL+"/v1/staged/cronkit/1.0.0", bytes.NewReader(fx.tarball))
	req.Header.Set("Authorization", "Bearer "+fx.token)
	publishproto.Apply(req.Header)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", resp.StatusCode, body)
	}
}
