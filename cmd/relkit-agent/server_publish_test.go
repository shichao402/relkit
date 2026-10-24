package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	rupv2 "github.com/shichao402/relkit/api/rup/v2"
	"github.com/shichao402/relkit/internal/casput"
	"github.com/shichao402/relkit/internal/config"
	"github.com/shichao402/relkit/internal/inprocess"
	"github.com/shichao402/relkit/internal/payload"
	"github.com/shichao402/relkit/internal/publishproto"
	"github.com/shichao402/relkit/internal/stage"
	"github.com/shichao402/relkit/internal/verify"
	"github.com/shichao402/relkit/internal/vnext/artifactset"
	"github.com/shichao402/relkit/internal/vnext/document"
	"github.com/shichao402/relkit/internal/vnext/project"
	"github.com/shichao402/relkit/internal/vnext/runtimeprofile"
	vnexttrust "github.com/shichao402/relkit/internal/vnext/trust"
	"github.com/shichao402/relkit/internal/sdk"
)

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

// fixtureRoot is the resolved server root a merge test hands to the publisher.
func fixtureRoot(t *testing.T) *config.RootKey {
	t.Helper()
	seed := testRootSeed()
	publicKey := ed25519.NewKeyFromSeed(seed).Public().(ed25519.PublicKey)
	fingerprint, err := vnexttrust.Fingerprint(publicKey)
	if err != nil {
		t.Fatal(err)
	}
	return &config.RootKey{KeyID: "server-root", Seed: seed, PublicKey: publicKey, Fingerprint: fingerprint}
}

// fixtureProject is a project declaration that trusts the given root, which is
// the only thing the publisher needs to know about signing (requirement 10.5).
func fixtureProject(t *testing.T, root *config.RootKey) project.Document {
	t.Helper()
	return project.Document{
		Schema:        project.Schema,
		Product:       "demo",
		Server:        "https://updates.example.invalid",
		RootPublicKey: base64.StdEncoding.EncodeToString(root.PublicKey),
		Channels:      []string{"dev", "stable"},
	}
}

func TestMergeVNextPublishConfigDerivesStandardDirectoryURLs(t *testing.T) {
	root := t.TempDir()
	serverRoot := fixtureRoot(t)
	productDocument := fixtureProject(t, serverRoot)
	profile := &config.PublishProfile{
		Product: "demo",
		Backends: map[string]map[string]any{
			"primary": {"type": "relkit-compatible", "baseUrl": "https://updates.example.invalid/root"},
		},
		PublishTo: []string{"primary"},
		Directory: &config.PublishDirectoryProfile{
			PublishTo: []string{"primary"},
		},
	}
	cfg, err := mergeVNextPublishConfig(&productDocument, profile, root, serverRoot)
	if err != nil {
		t.Fatal(err)
	}
	trusted, err := cfg.TrustedPublicKeys()
	if err != nil {
		t.Fatal(err)
	}
	// The publisher signs as the server's one root: there is no per-product key
	// to derive any more.
	if len(trusted) != 1 {
		t.Fatalf("publish config trusts %d keys, want exactly the server root", len(trusted))
	}
	if !slices.Equal(trusted[serverRoot.KeyID], serverRoot.PublicKey) {
		t.Fatalf("trusted keys=%v, want only %q", trusted, serverRoot.KeyID)
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
	serverRoot := fixtureRoot(t)
	profile := &config.PublishProfile{
		Product: "demo",
		Backends: map[string]map[string]any{
			"primary": {"type": "relkit-compatible", "baseUrl": "https://updates.example.invalid/root"},
		},
		PublishTo: []string{"primary"},
		Directory: &config.PublishDirectoryProfile{PublishTo: []string{"primary"}},
	}

	declared := fixtureProject(t, serverRoot)
	declared.Site = &project.Site{Title: "Demo", Description: "A demo product.", Homepage: "https://example.com/demo"}
	cfg, err := mergeVNextPublishConfig(&declared, profile, root, serverRoot)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Site.Title != "Demo" || cfg.Site.Description != "A demo product." || cfg.Site.Homepage != "https://example.com/demo" {
		t.Fatalf("site did not reach the publish config: %+v", cfg.Site)
	}

	// A product that does not declare site must stay opted out, not inherit
	// an empty block that publish would then have to second-guess.
	optedOut := fixtureProject(t, serverRoot)
	cfg, err = mergeVNextPublishConfig(&optedOut, profile, root, serverRoot)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Site.Title != "" || cfg.Site.Description != "" || cfg.Site.Homepage != "" {
		t.Fatalf("undeclared site must stay empty: %+v", cfg.Site)
	}
}

// TestMergeVNextPublishConfigRequiresTheServerRoot: a server without a root
// cannot publish at all. There is no fallback to a product key, so the merge
// must refuse instead of producing a config that would seal unverifiable
// documents (requirements 10.5, 14.9).
func TestMergeVNextPublishConfigRequiresTheServerRoot(t *testing.T) {
	root := t.TempDir()
	serverRoot := fixtureRoot(t)
	productDocument := fixtureProject(t, serverRoot)
	profile := &config.PublishProfile{
		Product: "demo",
		Backends: map[string]map[string]any{
			"primary": {"type": "relkit-compatible", "baseUrl": "https://updates.example.invalid/root"},
		},
		PublishTo: []string{"primary"},
		Directory: &config.PublishDirectoryProfile{PublishTo: []string{"primary"}},
	}
	_, err := mergeVNextPublishConfig(&productDocument, profile, root, nil)
	if err == nil || !strings.Contains(err.Error(), "server root is required") {
		t.Fatalf("merge without a server root err=%v", err)
	}
}

func TestAgentBootstrapsRuntimeProfileWithoutExposingPrivateKey(t *testing.T) {
	fx := makeVNextAgentFixture(t, vNextFixtureOptions{})
	payloadBytes, err := json.Marshal(runtimeProfileRequest{
		Product: "demo", Project: fx.vNextProject,
	})
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest(http.MethodPost, fx.ts.URL+"/v1/runtime-profile", bytes.NewReader(payloadBytes))
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
	// The profile is signed by the server root named in the custody manifest,
	// not by a per-product key.
	if err := result.Profile.Verify(map[string]ed25519.PublicKey{fx.rootKeyID: fx.rootPublic()}); err != nil {
		t.Fatalf("profile signature: %v", err)
	}
	if !fx.ingest.hasPrefix("runtime-profile/demo/") {
		t.Fatal("Runtime Profile immutable object was not published")
	}
	if rawDirectory := fx.ingest.get("directory/demo.pb"); len(rawDirectory) == 0 {
		t.Fatal("directory pointer was not published")
	}
	if bytes.Contains(raw, fx.rootSeed) || bytes.Contains(raw, []byte("custody")) {
		t.Fatal("Runtime Profile response exposed private signing material")
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
	// The directory target list is derived from the publish profile's backends,
	// so a failing directory PUT is the only knob left.
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
	fx := makeVNextAgentFixture(t, vNextFixtureOptions{})
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
	fx := makeVNextAgentFixture(t, vNextFixtureOptions{})
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
	fx := makeVNextAgentFixture(t, vNextFixtureOptions{})
	status, body := fx.putStaged(t, "demo", "1.0.0")
	if status != http.StatusCreated {
		t.Fatalf("staged status=%d body=%s", status, body)
	}
	status, body = fx.publish(t, `{"product":"demo","version":"1.0.0"}`)
	if status != http.StatusOK {
		t.Fatalf("publish status=%d body=%s", status, body)
	}
	fieldUpdater := &inprocess.Updater{
		Product:     "demo",
		Channel:     "dev",
		CurrentCode: 0,
		EntryURLs:   []string{strings.TrimRight(fx.ingest.URL, "/") + "/directory/demo.pb"},
		TrustedKeys: sdk.TrustedKeys{fx.rootKeyID: fx.rootPublic()},
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
	fx := makeVNextAgentFixture(t, vNextFixtureOptions{})
	publishVNextFixture(t, fx)
	// Released verification carries no resolution digest or Operator identity:
	// the documents are trusted because the server's one root signed them
	// (requirements 10.6, 10.7).
	findings, err := verify.RunReleasedHTTP(t.Context(), verify.ReleasedHTTPOptions{
		Product: "demo", Channel: "dev", Version: "1.0.0",
		ConsumerBaseURL: fx.ingest.URL,
		DirectoryURL:    fx.ingest.URL,
		TrustedRootKeys: map[string]ed25519.PublicKey{
			fx.rootKeyID: fx.rootPublic(),
		},
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

// TestScenarioS2ManagedProfilePinsTheServerRoot is the successor of the
// resolution-based "manager" scenario. A subscriber no longer resolves a
// manager document against an Operator and a Catalog: it reads the Runtime
// Profile the directory pins and verifies it with the one server root it was
// built with.
func TestScenarioS2ManagedProfilePinsTheServerRoot(t *testing.T) {
	fx := makeVNextAgentFixture(t, vNextFixtureOptions{})
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
	trusted := map[string]ed25519.PublicKey{fx.rootKeyID: fx.rootPublic()}
	if err := profile.Verify(trusted); err != nil {
		t.Fatalf("pinned profile is not signed by the server root: %v", err)
	}
	if profile.Product != "demo" || profile.Digest == "" {
		t.Fatalf("profile=%+v", profile)
	}
	if !slices.Contains(profile.AllowedChannels, "dev") {
		t.Fatalf("profile allowed channels=%v, want the published channel", profile.AllowedChannels)
	}
	if len(profile.DirectoryBootstrap.EntryURLs) == 0 {
		t.Fatal("profile carries no directory entry URLs")
	}
}

func TestScenarioS3ComponentAudienceSelection(t *testing.T) {
	fx := makeVNextAgentFixture(t, vNextFixtureOptions{
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
	fieldUpdater := &inprocess.Updater{
		Product:         "demo",
		Channel:         "dev",
		CurrentCode:     0,
		EntryURLs:       []string{strings.TrimRight(fx.ingest.URL, "/") + "/directory/demo.pb"},
		TrustedKeys:     sdk.TrustedKeys{fx.rootKeyID: fx.rootPublic()},
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
	fx := makeVNextAgentFixture(t, vNextFixtureOptions{})
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

// TestAgentRejectsChannelTheProjectDoesNotDeclare replaces the retired
// candidate-lane test. There is one lane per project now, so the only thing
// left to prove is that a staged release cannot claim a channel its own project
// declaration never listed.
func TestAgentRejectsChannelTheProjectDoesNotDeclare(t *testing.T) {
	fx := newAgentFixture(t, agentFixtureOpts{
		tamperStable: true,
		patchProject: func(document *project.Document) {
			document.Channels = []string{"dev"}
		},
	})
	status, body := fx.putStaged(t, "demo", "1.0.0")
	if status != http.StatusBadRequest {
		t.Fatalf("staged status=%d body=%s", status, body)
	}
	if !strings.Contains(string(body), "does not declare channel") {
		t.Fatalf("body=%s", body)
	}
}

func TestAgentRejectsArtifactSetReleaseCodeTampering(t *testing.T) {
	fx := makeVNextAgentFixture(t, vNextFixtureOptions{tamperReleaseCode: true})
	status, body := fx.putStaged(t, "demo", "1.0.0")
	if status != http.StatusBadRequest || !strings.Contains(string(body), "releaseCode") {
		t.Fatalf("releaseCode tamper status=%d body=%s", status, body)
	}
}

// TestAgentPublishWritesOnlyTheRequestedChannel replaces the candidate-lane
// publish test: publishing dev must not touch the stable index, because the
// agent no longer has a certification step that could promote the tree.
func TestAgentPublishWritesOnlyTheRequestedChannel(t *testing.T) {
	fx := makeVNextAgentFixture(t, vNextFixtureOptions{})
	status, body := fx.putStaged(t, "demo", "1.0.0")
	if status != http.StatusCreated {
		t.Fatalf("staged status=%d body=%s", status, body)
	}
	status, body = fx.publish(t, `{"product":"demo","version":"1.0.0"}`)
	if status != http.StatusOK {
		t.Fatalf("publish status=%d body=%s", status, body)
	}
	if len(fx.ingest.get("index/demo/dev.pb")) == 0 {
		t.Fatal("dev index was not written")
	}
	if len(fx.ingest.get("index/demo/stable.pb")) != 0 {
		t.Fatal("publish wrote an index for a channel it did not release")
	}
}

func TestAgentPublishesRuntimeProfileBeforeDirectoryCommit(t *testing.T) {
	fx := makeVNextAgentFixture(t, vNextFixtureOptions{})
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
	fx := makeVNextAgentFixture(t, vNextFixtureOptions{})
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

func TestAgentPublishRequiresProfile(t *testing.T) {
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

// TestAgentPublishRejectsProfileProductMismatch: the publish profile is the
// only per-product file left, and its product must agree with the staged
// project declaration the server is about to sign.
func TestAgentPublishRejectsProfileProductMismatch(t *testing.T) {
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
}

// TestAgentPublishRejectsProfileWithoutDirectory: a release must be reachable
// by consumers. A profile that names no directory plane cannot produce a
// verified commit, so publish refuses instead of skipping the step.
func TestAgentPublishRejectsProfileWithoutDirectory(t *testing.T) {
	fx := newAgentFixture(t, agentFixtureOpts{
		patchProfile: func(profile *config.PublishProfile) {
			profile.Directory = nil
		},
	})
	status, body := fx.putStaged(t, "demo", "1.0.0")
	if status != http.StatusCreated {
		t.Fatalf("staged status=%d body=%s", status, body)
	}
	status, body = fx.publish(t, `{"product":"demo","version":"1.0.0","dryRun":true}`)
	if status == http.StatusOK {
		t.Fatalf("expected missing directory error, body=%s", body)
	}
	if !bytes.Contains(body, []byte("directory")) {
		t.Fatalf("body=%s", body)
	}
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
	req, _ := http.NewRequest(http.MethodPut, fx.ts.URL+"/v1/staged/svn-auto-merge/1.0.0", bytes.NewReader(fx.tarball))
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
	// Requirement 11.1 asks for an error that says what happened, so an operator
	// does not have to guess which of two tokens was pasted.
	if !bytes.Contains(body, []byte(`"demo"`)) || !bytes.Contains(body, []byte(`"svn-auto-merge"`)) {
		t.Fatalf("refusal does not name both products: %s", body)
	}
}
