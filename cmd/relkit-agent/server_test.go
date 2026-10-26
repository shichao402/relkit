package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	rupv2 "github.com/shichao402/relkit/api/rup/v2"
	"github.com/shichao402/relkit/internal/agentadmin"
	"github.com/shichao402/relkit/internal/config"
	"github.com/shichao402/relkit/internal/custody"
	"github.com/shichao402/relkit/internal/keys"
	"github.com/shichao402/relkit/internal/makers"
	"github.com/shichao402/relkit/internal/publishproto"
	"github.com/shichao402/relkit/internal/stage"
	"github.com/shichao402/relkit/internal/vnext/artifactset"
	"github.com/shichao402/relkit/internal/vnext/document"
	"github.com/shichao402/relkit/internal/vnext/project"
	vnextrelease "github.com/shichao402/relkit/internal/vnext/release"
	vnexttrust "github.com/shichao402/relkit/internal/vnext/trust"
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
	// The agent accepts exactly one staged shape: a vNext staged tree carrying
	// staged.pb, the project declaration, the ArtifactSet and the artifacts.
	// There is no resolution sidecar to prove and no candidate lane to pick, so
	// the fixture uploads that tree as a tarball and publishes it as a dry run.
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

// writeAdminDoc seeds the management document the way the server persists it:
// product registrations plus sha256 hashes, never plaintext. Tests write it
// directly because the store has no "create a token with this exact value"
// entry point on purpose (requirement 11.3).
func writeAdminDoc(t *testing.T, path string, roots map[string]string, plaintext, product string) {
	t.Helper()
	writeAdminDocWithProfile(t, path, roots, "", true, plaintext, product)
}

func writeAdminDocWithProfile(t *testing.T, path string, roots map[string]string, profile string, omitProfile bool, plaintext, product string) {
	t.Helper()
	products := make([]any, 0, len(roots))
	for id, root := range roots {
		entry := map[string]any{"id": id, "root": root}
		if !omitProfile && profile != "" && id == product {
			entry["profile"] = profile
		}
		products = append(products, entry)
	}
	sum := sha256.Sum256([]byte(plaintext))
	doc := map[string]any{
		"schema":   agentadmin.Schema,
		"products": products,
		"tokens": []any{
			map[string]any{
				"id":      "tok_fixture",
				"product": product,
				"label":   "fixture",
				"hash":    hex.EncodeToString(sum[:]),
			},
		},
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(raw, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestAgentDropPutGetHead(t *testing.T) {
	root := t.TempDir()
	productRoot := filepath.Join(root, "product")
	if err := os.MkdirAll(productRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	agentCfgPath := filepath.Join(root, "agent.json")
	stateDir := filepath.Join(root, "state")
	agentDoc := map[string]any{
		"stateDir": stateDir,
	}
	agentBytes, _ := json.MarshalIndent(agentDoc, "", "  ")
	if err := os.WriteFile(agentCfgPath, agentBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	writeAdminDoc(t, filepath.Join(stateDir, "admin.json"), map[string]string{"demo": productRoot}, "test-token", "demo")
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
	payloadBytes := []byte("ok")
	_ = tw.WriteHeader(&tar.Header{Name: "./staged.pb", Mode: 0o644, Size: int64(len(payloadBytes))})
	_, _ = tw.Write(payloadBytes)
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

// agentFixture is one agent server plus the staged tarball it accepts. The
// fixture mirrors the post-epoch model exactly:
//
//   - the server holds one root, resolved from a custody manifest it reads by
//     keyId (requirements 10.1, 15.3);
//   - a staged tree is staged.pb + relkit.project.json + artifact-set.json +
//     artifacts/, with no resolution bundle and no per-product signing key;
//   - the project declaration names the same rootPublicKey the server signs
//     with, which is the only provenance the server has left.
type agentFixture struct {
	productRoot string
	profilePath string
	cfg         *Config
	ts          *httptest.Server
	token       string
	tarball     []byte
	ingest      *fakeRelkitServe

	rootKeyID string
	rootSeed  []byte

	vNextProject project.Document
}

// rootPublic is the public half of the fixture server's one root.
func (f *agentFixture) rootPublic() ed25519.PublicKey {
	return ed25519.NewKeyFromSeed(f.rootSeed).Public().(ed25519.PublicKey)
}

// testRootSeed is the fixed seed every fixture server signs with, so a failing
// assertion is reproducible without ever printing key material.
func testRootSeed() []byte {
	seed := make([]byte, ed25519.SeedSize)
	for index := range seed {
		seed[index] = byte(index + 41)
	}
	return seed
}

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
	root := t.TempDir()
	stateDir := filepath.Join(root, "state")
	productRoot := filepath.Join(root, "product")
	if err := os.MkdirAll(productRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	ingest := startFakeRelkitServe(t, "serve-token")

	seed := testRootSeed()
	publicKey := ed25519.NewKeyFromSeed(seed).Public().(ed25519.PublicKey)
	fingerprint, err := vnexttrust.Fingerprint(publicKey)
	if err != nil {
		t.Fatal(err)
	}
	rootKeyID := "server-root"
	custodyDir := filepath.Join(root, "custody")
	if err := os.MkdirAll(custodyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	rootDocument := keys.PrivateKeyDocument(rootKeyID, seed)
	rootBytes, err := rupv2.MarshalPrivateKey(&rootDocument)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(custodyDir, rootKeyID+".pb"), rootBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	manifest := map[string]any{
		"schema": custody.ManifestSchema,
		"scope":  custody.ScopeServer,
		"keys": []any{
			map[string]any{
				"keyId":       rootKeyID,
				"kind":        custody.KindRoot,
				"purpose":     "the agent server root every document is signed with",
				"carrier":     custody.CarrierFile,
				"path":        rootKeyID + ".pb",
				"fingerprint": fingerprint,
			},
		},
	}
	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(custodyDir, "manifest.json"), append(manifestBytes, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}

	productDocument := project.Document{
		Schema:             project.Schema,
		Product:            opts.product,
		Server:             "https://updates.example",
		RootPublicKey:      vnexttrust.SSHPublicKeyLine(publicKey),
		Channels:           []string{"dev", "stable"},
		SelectorDimensions: append([]string(nil), opts.selectorDimensions...),
		ArtifactProducer: &project.Command{
			Command: []string{"fixture-producer"}, ExtensionSchema: "relkit.extension/1",
		},
	}
	if opts.site != nil {
		productDocument.Site = opts.site
	}
	if opts.patchProject != nil {
		opts.patchProject(&productDocument)
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
		Schema: artifactset.Schema, Product: productDocument.Product,
		Version: opts.version, ReleaseCode: 1, Channel: opts.channel,
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
	if err := document.Write(artifactSetPath, artifacts); err != nil {
		t.Fatal(err)
	}
	if _, err := vnextrelease.Stage(productDocument, artifacts, vnextrelease.StageOptions{
		ProjectRoot: productRoot, ArtifactSetPath: artifactSetPath, Code: 1,
	}); err != nil {
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
	if opts.omitProject {
		// The server proves where a tree came from by reading the project
		// declaration inside it. Without that file there is nothing to prove, so
		// the upload is refused and the extracted tree is cleaned up.
		if err := os.Remove(filepath.Join(stagingRoot, vnextrelease.ProjectFile)); err != nil {
			t.Fatal(err)
		}
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
			Product:   opts.product,
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

	agentCfgPath := filepath.Join(root, "agent.json")
	agentDoc := map[string]any{
		"stateDir":        stateDir,
		"rootKeyId":       rootKeyID,
		"custodyManifest": filepath.ToSlash(filepath.Join("custody", "manifest.json")),
		"toolsDir":        filepath.ToSlash(filepath.Join(stateDir, "tools")),
	}
	if opts.deployToken {
		deployTokenPath := filepath.Join(root, "tokens", "deploy.token")
		if err := os.MkdirAll(filepath.Dir(deployTokenPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(deployTokenPath, []byte(toolToken+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		agentDoc["toolsTokenFile"] = filepath.ToSlash(filepath.Join("tokens", "deploy.token"))
		// Tool URLs are signed with the agent's own public address, so a
		// server that publishes tools always has one (see isPublicOrigin).
		if !opts.noPublicURL {
			agentDoc["publicURL"] = "https://publish.example"
		}
	}
	agentBytes, _ := json.MarshalIndent(agentDoc, "", "  ")
	if err := os.WriteFile(agentCfgPath, agentBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	// The admin document is agent state, so it is seeded before the config is
	// loaded: LoadConfig reads it to build the product map and credential list.
	productRoots := map[string]string{opts.product: productRoot}
	writeAdminDocWithProfile(t, filepath.Join(stateDir, "admin.json"), productRoots, profilePath, opts.omitProfile, "test-token", opts.product)
	cfg, err := LoadConfig(agentCfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Root == nil {
		t.Fatal("agent fixture did not resolve a server root from the custody manifest")
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
		productRoot:  productRoot,
		profilePath:  profilePath,
		cfg:          cfg,
		ts:           ts,
		token:        "test-token",
		tarball:      tarball,
		ingest:       ingest,
		rootKeyID:    rootKeyID,
		rootSeed:     seed,
		vNextProject: productDocument,
	}
}

type agentFixtureOpts struct {
	product            string
	version            string
	channel            string
	omitProfile        bool
	patchProfile       func(*config.PublishProfile)
	site               *project.Site
	patchProject       func(*project.Document)
	tamperStable       bool
	tamperReleaseCode  bool
	omitProject        bool
	selectorDimensions []string
	artifacts          []vNextFixtureArtifact
	tinyParts          bool
	deployToken        bool
	noPublicURL        bool
}

type vNextFixtureOptions struct {
	tamperStable       bool
	tamperReleaseCode  bool
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

// makeVNextAgentFixture is the historical spelling of the vNext fixture. It
// forwards to newAgentFixture, so both entry points produce the same single-root
// agent and the call sites did not have to be rewritten when the candidate lane
// and the resolution bundle went away.
func makeVNextAgentFixture(t *testing.T, options vNextFixtureOptions) *agentFixture {
	t.Helper()
	return newAgentFixture(t, agentFixtureOpts{
		tamperStable:       options.tamperStable,
		tamperReleaseCode:  options.tamperReleaseCode,
		selectorDimensions: options.selectorDimensions,
		artifacts:          options.artifacts,
	})
}
