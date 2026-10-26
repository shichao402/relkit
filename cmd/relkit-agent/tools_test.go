package main

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shichao402/relkit/internal/vnext/catalog"
	"github.com/shichao402/relkit/internal/vnext/tools"
)

// toolFixture publishes one tool set through the real endpoint, so the tests
// exercise the same code the deploy pipeline hits.
type toolFixture struct {
	fx      *agentFixture
	version string
	name    string
	payload []byte
}

// toolToken is the deploy pipeline credential every tool fixture writes.
const toolToken = "deploy-token"

func (f *agentFixture) toolsGet(t *testing.T, path string) *http.Response {
	t.Helper()
	response, err := http.Get(f.ts.URL + path)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func (f *agentFixture) publishTools(t *testing.T, body []byte, token string) (int, []byte) {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, f.ts.URL+"/v1/tools", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	out, _ := io.ReadAll(response.Body)
	return response.StatusCode, out
}

func (f *agentFixture) uploadTool(t *testing.T, version, name string, payload []byte, token string) (int, []byte) {
	t.Helper()
	request, err := http.NewRequest(http.MethodPut, f.ts.URL+"/v1/tools/"+version+"/"+name, bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	out, _ := io.ReadAll(response.Body)
	return response.StatusCode, out
}

func newToolFixture(t *testing.T) *toolFixture {
	t.Helper()
	fx := newAgentFixture(t, agentFixtureOpts{deployToken: true})
	version := "0.7.0_20261022abc"
	name := "relkit-updater-windows-amd64.exe"
	payload := []byte("MZ fake updater payload")
	// The deploy pipeline PUTs the bytes, then publishes the manifest that
	// names their digests. Doing the same here exercises the whole chain.
	if status, out := fx.uploadTool(t, version, name, payload, toolToken); status != http.StatusCreated {
		t.Fatalf("tool upload status=%d body=%s", status, out)
	}
	body, err := json.Marshal(map[string]any{
		"version": version,
		"commit":  "abcdef0123456789",
		"files": []any{map[string]any{
			"component": "updater",
			"name":      name, "platform": "windows-amd64", "os": "windows", "arch": "amd64",
			"size": len(payload), "sha256": tools.Sum256(payload),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	status, out := fx.publishTools(t, body, toolToken)
	if status != http.StatusOK {
		t.Fatalf("tool publish status=%d body=%s", status, out)
	}
	return &toolFixture{fx: fx, version: version, name: name, payload: payload}
}

// TestAgentServesToolWithVerifiableSSHSignature walks the host CI path: read the
// signed manifest, check the digest the manifest declares, then verify the
// sidecar signature with the key the manifest itself carries.
// TestAgentDerivesPlatformFromOSAndArch covers the field the deploy pipeline
// leaves out. The manifest cannot be signed without a platform, so a publish
// that states only os and arch must still succeed by deriving "os-arch"; hosts
// select tools by that value, and an empty one makes every lookup miss.
func TestAgentDerivesPlatformFromOSAndArch(t *testing.T) {
	fx := newAgentFixture(t, agentFixtureOpts{deployToken: true})
	version := "0.7.0_derivedplatform"
	name := "relkit-cli-linux-amd64"
	payload := []byte("ELF fake cli payload")
	if status, out := fx.uploadTool(t, version, name, payload, toolToken); status != http.StatusCreated {
		t.Fatalf("tool upload status=%d body=%s", status, out)
	}
	body, err := json.Marshal(map[string]any{
		"version": version,
		"commit":  "abcdef0123456789",
		"files": []any{map[string]any{
			"component": "cli",
			"name":      name,
			"os":        "linux",
			"arch":      "amd64",
			"size":      len(payload), "sha256": tools.Sum256(payload),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	status, out := fx.publishTools(t, body, toolToken)
	if status != http.StatusOK {
		t.Fatalf("tool publish status=%d body=%s", status, out)
	}

	manifestResponse := fx.toolsGet(t, "/v1/tools/"+version+"/manifest.json")
	defer manifestResponse.Body.Close()
	var manifest tools.Document
	if err := json.NewDecoder(manifestResponse.Body).Decode(&manifest); err != nil {
		t.Fatal(err)
	}
	row, ok := manifest.Tool(name, "linux-amd64")
	if !ok {
		platforms := make([]string, 0, len(manifest.Tools))
		for _, tool := range manifest.Tools {
			platforms = append(platforms, tool.Name+"@"+tool.Platform)
		}
		t.Fatalf("manifest has no row for %s@linux-amd64, has %v", name, platforms)
	}
	if row.OS != "linux" || row.Arch != "amd64" {
		t.Fatalf("row os/arch = %s/%s", row.OS, row.Arch)
	}
}

func TestAgentServesToolWithVerifiableSSHSignature(t *testing.T) {
	fixture := newToolFixture(t)

	manifestResponse := fixture.fx.toolsGet(t, "/v1/tools/"+fixture.version+"/manifest.json")
	defer manifestResponse.Body.Close()
	if manifestResponse.StatusCode != http.StatusOK {
		t.Fatalf("manifest status=%d", manifestResponse.StatusCode)
	}
	var manifest tools.Document
	if err := json.NewDecoder(manifestResponse.Body).Decode(&manifest); err != nil {
		t.Fatal(err)
	}
	if err := manifest.Verify(map[string]ed25519.PublicKey{fixture.fx.rootKeyID: fixture.fx.rootPublic()}); err != nil {
		t.Fatalf("manifest verify: %v", err)
	}

	row, ok := manifest.Tool(fixture.name, "windows-amd64")
	if !ok {
		t.Fatalf("manifest has no row for %s", fixture.name)
	}
	fileResponse := fixture.fx.toolsGet(t, "/v1/tools/"+fixture.version+"/"+fixture.name)
	defer fileResponse.Body.Close()
	body, _ := io.ReadAll(fileResponse.Body)
	if got := tools.Sum256(body); got != row.SHA256 {
		t.Fatalf("downloaded digest %s, manifest says %s", got, row.SHA256)
	}

	signatureResponse := fixture.fx.toolsGet(t, "/v1/tools/"+fixture.version+"/"+fixture.name+".sig")
	defer signatureResponse.Body.Close()
	armored, _ := io.ReadAll(signatureResponse.Body)
	publicKey, err := manifest.PublicKey()
	if err != nil {
		t.Fatal(err)
	}
	if err := tools.VerifySSHSignature(string(armored), publicKey, body); err != nil {
		t.Fatalf("SSH signature verify: %v", err)
	}
}

// TestAgentToolSignatureVerifiesWithOpenSSH is the end-to-end proof for
// requirement 12.3: the exact bytes a host CI downloads, checked by the
// building machine's own ssh-keygen rather than by relkit code.
func TestAgentToolSignatureVerifiesWithOpenSSH(t *testing.T) {
	sshKeygen, err := exec.LookPath("ssh-keygen")
	if err != nil {
		t.Skip("ssh-keygen is not installed on this machine")
	}
	fixture := newToolFixture(t)

	manifestResponse := fixture.fx.toolsGet(t, "/v1/tools/"+fixture.version+"/manifest.json")
	defer manifestResponse.Body.Close()
	var manifest tools.Document
	if err := json.NewDecoder(manifestResponse.Body).Decode(&manifest); err != nil {
		t.Fatal(err)
	}
	fileResponse := fixture.fx.toolsGet(t, "/v1/tools/"+fixture.version+"/"+fixture.name)
	defer fileResponse.Body.Close()
	payload, _ := io.ReadAll(fileResponse.Body)
	signatureResponse := fixture.fx.toolsGet(t, "/v1/tools/"+fixture.version+"/"+fixture.name+".sig")
	defer signatureResponse.Body.Close()
	armored, _ := io.ReadAll(signatureResponse.Body)

	publicKey, err := manifest.PublicKey()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	payloadPath := filepath.Join(dir, fixture.name)
	signaturePath := filepath.Join(dir, fixture.name+".sig")
	allowlistPath := filepath.Join(dir, tools.AllowlistName)
	files := map[string][]byte{
		payloadPath:   payload,
		signaturePath: armored,
		allowlistPath: []byte(tools.AllowedSignerLine(manifest.RootKeyID, publicKey) + "\n"),
	}
	for path, data := range files {
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	payloadFile, err := os.Open(payloadPath)
	if err != nil {
		t.Fatal(err)
	}
	defer payloadFile.Close()
	verify := exec.Command(sshKeygen, "-Y", "verify",
		"-f", allowlistPath, "-I", manifest.RootKeyID, "-n", tools.SignatureNamespace, "-s", signaturePath)
	verify.Stdin = payloadFile
	if output, err := verify.CombinedOutput(); err != nil {
		t.Fatalf("ssh-keygen rejected the served signature: %v\n%s", err, output)
	}
}

func TestAgentToolPublishRejectsDigestMismatch(t *testing.T) {
	fx := newAgentFixture(t, agentFixtureOpts{deployToken: true})
	version := "0.7.0_1"
	if status, out := fx.uploadTool(t, version, "relkit.exe", []byte("actual"), toolToken); status != http.StatusCreated {
		t.Fatalf("upload status=%d body=%s", status, out)
	}
	body, _ := json.Marshal(map[string]any{
		"version": version,
		"files": []any{map[string]any{
			"component": "cli",
			"name":      "relkit.exe", "platform": "windows-amd64", "os": "windows", "arch": "amd64",
			"size": 6, "sha256": tools.Sum256([]byte("declared")),
		}},
	})
	status, out := fx.publishTools(t, body, toolToken)
	if status != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", status, out)
	}
	if !strings.Contains(string(out), "digest mismatch") {
		t.Fatalf("body=%s", out)
	}
	if _, err := os.Stat(filepath.Join(fx.cfg.ToolsDir, version, "manifest.json")); !os.IsNotExist(err) {
		t.Fatal("a mismatched upload must not produce a manifest")
	}
}

// TestAgentToolUploadKeepsSignedNamesWriteOnly pins that an upload can never
// overwrite what signing produced, and that a version is immutable.
func TestAgentToolUploadKeepsSignedNamesWriteOnly(t *testing.T) {
	fixture := newToolFixture(t)

	for _, name := range []string{"manifest.json", fixture.name + toolSignatureSuffix} {
		status, out := fixture.fx.uploadTool(t, fixture.version, name, []byte("forged"), toolToken)
		if status != http.StatusBadRequest {
			t.Fatalf("%s upload status=%d body=%s", name, status, out)
		}
	}
	// Same version, different bytes: a signed version must not change under a
	// host that already verified it.
	status, out := fixture.fx.uploadTool(t, fixture.version, fixture.name, []byte("different bytes"), toolToken)
	if status != http.StatusConflict {
		t.Fatalf("different bytes status=%d body=%s", status, out)
	}
	// A retried pipeline re-uploads identical bytes and must succeed.
	if status, out := fixture.fx.uploadTool(t, fixture.version, fixture.name, fixture.payload, toolToken); status != http.StatusOK {
		t.Fatalf("identical re-upload status=%d body=%s", status, out)
	}
}

func TestAgentToolUploadRequiresDeployToken(t *testing.T) {
	fx := newAgentFixture(t, agentFixtureOpts{deployToken: true})
	if status, _ := fx.uploadTool(t, "0.1.0_1", "relkit.exe", []byte("x"), ""); status != http.StatusUnauthorized {
		t.Fatalf("unauthenticated upload status=%d", status)
	}
	if status, _ := fx.uploadTool(t, "0.1.0_1", "relkit.exe", []byte("x"), fx.token); status != http.StatusUnauthorized {
		t.Fatalf("product token must not upload tools, status=%d", status)
	}
}

func TestAgentToolPublishRequiresDeployToken(t *testing.T) {
	fx := newAgentFixture(t, agentFixtureOpts{deployToken: true})
	body, _ := json.Marshal(map[string]any{"version": "1.0.0", "files": []any{}})
	if status, _ := fx.publishTools(t, body, ""); status != http.StatusUnauthorized {
		t.Fatalf("missing token status=%d", status)
	}
	// A product upload token authorizes products, not the shared toolchain.
	if status, _ := fx.publishTools(t, body, fx.token); status != http.StatusUnauthorized {
		t.Fatalf("product token must not publish tools, status=%d", status)
	}
}

// TestAgentToolPublishDisabledWithoutDeployToken pins the safe default: a
// server without toolsTokenFile serves tools it already has but accepts no
// uploads.
func TestAgentToolPublishDisabledWithoutDeployToken(t *testing.T) {
	fx := newAgentFixture(t, agentFixtureOpts{})
	body, _ := json.Marshal(map[string]any{"version": "1.0.0", "files": []any{}})
	status, out := fx.publishTools(t, body, fx.token)
	if status != http.StatusMethodNotAllowed {
		t.Fatalf("status=%d body=%s", status, out)
	}
}

// TestAgentSignsCatalogWhenPublishingTools pins requirement 12.6: the upload
// that signs tool files also signs the Catalog and marks that version current,
// so "the files of this version" and "the version served" cannot disagree.
func TestAgentSignsCatalogWhenPublishingTools(t *testing.T) {
	fixture := newToolFixture(t)

	response := fixture.fx.toolsGet(t, "/v1/catalog")
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("catalog status=%d", response.StatusCode)
	}
	var document catalog.Document
	if err := json.NewDecoder(response.Body).Decode(&document); err != nil {
		t.Fatal(err)
	}
	if err := document.Verify(map[string]ed25519.PublicKey{fixture.fx.rootKeyID: fixture.fx.rootPublic()}); err != nil {
		t.Fatalf("catalog verify: %v", err)
	}
	// Tool versions carry a build suffix; the Catalog keeps the exact string.
	if document.Current != fixture.version {
		t.Fatalf("catalog current=%q want %q", document.Current, fixture.version)
	}
	release, ok := document.ReleaseByVersion(fixture.version)
	if !ok {
		t.Fatalf("catalog has no release %s", fixture.version)
	}
	var found bool
	for _, row := range release.Components {
		if row.Platform == "windows-amd64" {
			found = true
			// The fixture names its file the registry way, so falling back to
			// the file name has to recover "updater".
			if row.Name != "updater" {
				t.Fatalf("catalog component name=%q, want updater", row.Name)
			}
			if row.Source.Kind != "server" || row.Source.URL == "" {
				t.Fatalf("catalog component source=%+v", row.Source)
			}
			// Tools are signed onto the CI face hosts already know, never onto
			// whatever origin the deploy pipeline happened to call from.
			if !strings.HasPrefix(row.Source.URL, "https://publish.example/v1/tools/") {
				t.Fatalf("catalog component url=%q, want the agent's publicURL", row.Source.URL)
			}
			if row.SHA256 == "" {
				t.Fatal("catalog component has no digest")
			}
		}
	}
	if !found {
		t.Fatal("catalog release has no windows-amd64 component")
	}
}

// TestAgentRefusesToolPublishWithoutPublicURL covers a server that has no
// publicURL and is called on loopback, which is exactly how the deploy
// pipeline publishes. Signing 127.0.0.1 URLs would report success while every
// build machine fails to download.
func TestAgentRefusesToolPublishWithoutPublicURL(t *testing.T) {
	fx := newAgentFixture(t, agentFixtureOpts{deployToken: true, noPublicURL: true})
	version := "0.7.0_nopublicurl"
	name := "relkit-cli-linux-amd64"
	payload := []byte("ELF fake cli payload")
	if status, out := fx.uploadTool(t, version, name, payload, toolToken); status != http.StatusCreated {
		t.Fatalf("tool upload status=%d body=%s", status, out)
	}
	body, _ := json.Marshal(map[string]any{
		"version": version,
		"files": []any{map[string]any{
			"component": "cli", "name": name, "os": "linux", "arch": "amd64",
			"size": len(payload), "sha256": tools.Sum256(payload),
		}},
	})
	status, out := fx.publishTools(t, body, toolToken)
	if status != http.StatusConflict || !strings.Contains(string(out), "publicURL") {
		t.Fatalf("publish without publicURL status=%d body=%s, want 409 naming publicURL", status, out)
	}
}

// TestAgentIgnoresCallerChosenToolBase pins that the download prefix is not a
// caller decision any more: an old pipeline that still sends baseUrl must not
// be able to sign tools onto another domain.
func TestAgentIgnoresCallerChosenToolBase(t *testing.T) {
	fx := newAgentFixture(t, agentFixtureOpts{deployToken: true})
	version := "0.7.0_callerbase"
	name := "relkit-cli-linux-amd64"
	payload := []byte("ELF fake cli payload")
	if status, out := fx.uploadTool(t, version, name, payload, toolToken); status != http.StatusCreated {
		t.Fatalf("tool upload status=%d body=%s", status, out)
	}
	body, _ := json.Marshal(map[string]any{
		"version": version,
		"commit":  "abcdef0123456789",
		"baseUrl": "https://update-internal.example",
		"files": []any{map[string]any{
			"component": "cli", "name": name, "os": "linux", "arch": "amd64",
			"size": len(payload), "sha256": tools.Sum256(payload),
		}},
	})
	if status, out := fx.publishTools(t, body, toolToken); status != http.StatusOK {
		t.Fatalf("publish status=%d body=%s", status, out)
	}
	response := fx.toolsGet(t, "/v1/tools/"+version+"/manifest.json")
	defer response.Body.Close()
	raw, _ := io.ReadAll(response.Body)
	if strings.Contains(string(raw), "update-internal.example") {
		t.Fatalf("manifest signed the caller's base URL: %s", raw)
	}
	if !strings.Contains(string(raw), "https://publish.example/v1/tools/"+version+"/") {
		t.Fatalf("manifest does not use the agent publicURL: %s", raw)
	}
}

func TestIsPublicOrigin(t *testing.T) {
	cases := map[string]bool{
		"http://publish.devcloud.woa.com": true,
		"https://publish.firoyang.com":    true,
		"http://127.0.0.1:8787":           false,
		"https://127.0.0.1:8787":          false,
		"http://localhost:8787":           false,
		"http://[::1]:8787":               false,
		"http://0.0.0.0:8787":             false,
		"":                                false,
		"publish.firoyang.com":            false,
	}
	for origin, want := range cases {
		if got := isPublicOrigin(origin); got != want {
			t.Errorf("isPublicOrigin(%q)=%v, want %v", origin, got, want)
		}
	}
}

func TestAgentCatalogRejectsUnknownVersion(t *testing.T) {
	fixture := newToolFixture(t)
	response := fixture.fx.toolsGet(t, "/v1/catalog")
	defer response.Body.Close()
	var document catalog.Document
	if err := json.NewDecoder(response.Body).Decode(&document); err != nil {
		t.Fatal(err)
	}
	if _, ok := document.ReleaseByVersion("9.9.9"); ok {
		t.Fatal("catalog invented a release that was never published")
	}
}

func TestAgentServesPublishedRootPublicKey(t *testing.T) {
	fixture := newToolFixture(t)
	response := fixture.fx.toolsGet(t, "/v1/tools/"+toolRootKeyFile)
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("root.pub status=%d", response.StatusCode)
	}
	body, _ := io.ReadAll(response.Body)
	// root.pub is published in the exact form a host pastes into
	// relkit.project.json and CI copies into allowed_signers, so what is
	// served here is directly usable by ssh-keygen.
	want, err := tools.EncodePublicKey(fixture.fx.rootPublic())
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(body)); got != want {
		t.Fatalf("root.pub=%q want %q", got, want)
	}
	if !strings.HasPrefix(want, "ssh-ed25519 ") {
		t.Fatalf("root.pub is not an ssh-ed25519 line: %q", want)
	}
}

func TestAgentRejectsToolPathTraversal(t *testing.T) {
	fixture := newToolFixture(t)
	for _, path := range []string{
		"/v1/tools/" + fixture.version + "/..%2F..%2Fmanifest.json",
		"/v1/tools/..%2F" + fixture.version + "/" + fixture.name,
	} {
		response := fixture.fx.toolsGet(t, path)
		status := response.StatusCode
		response.Body.Close()
		if status == http.StatusOK {
			t.Fatalf("%s must not serve a file outside the tool store", path)
		}
	}
}

// TestAgentServesCurrentAlias pins requirement 12.5: the CI template never
// names a relkit version, so the version segment has to be resolvable from the
// Catalog the server signed. Serving it as an alias keeps "which version is
// current" in exactly one place.
func TestAgentServesCurrentAlias(t *testing.T) {
	fixture := newToolFixture(t)

	response := fixture.fx.toolsGet(t, "/v1/tools/"+toolCurrentAlias+"/manifest.json")
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("current alias status=%d", response.StatusCode)
	}
	// A moving pointer must not be cached: CI would keep an old toolchain.
	if cache := response.Header.Get("Cache-Control"); cache != "no-cache" {
		t.Fatalf("current alias cache-control=%q, want no-cache", cache)
	}
	var manifest tools.Document
	if err := json.NewDecoder(response.Body).Decode(&manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Version != fixture.version {
		t.Fatalf("current alias served version %q, want %q", manifest.Version, fixture.version)
	}

	fileResponse := fixture.fx.toolsGet(t, "/v1/tools/"+toolCurrentAlias+"/"+fixture.name)
	defer fileResponse.Body.Close()
	body, _ := io.ReadAll(fileResponse.Body)
	if !bytes.Equal(body, fixture.payload) {
		t.Fatal("current alias served the wrong tool bytes")
	}
}

// TestAgentCurrentAliasIsReadOnly keeps a deploy pipeline from repointing every
// host at once by uploading under the alias.
func TestAgentCurrentAliasIsReadOnly(t *testing.T) {
	fixture := newToolFixture(t)

	if status, out := fixture.fx.uploadTool(t, toolCurrentAlias, fixture.name, []byte("x"), toolToken); status != http.StatusBadRequest {
		t.Fatalf("upload under current alias status=%d body=%s", status, out)
	}
	body, _ := json.Marshal(map[string]any{
		"version": toolCurrentAlias,
		"files": []any{map[string]any{
			"component": "updater", "name": fixture.name, "platform": "windows-amd64",
			"os": "windows", "arch": "amd64", "size": len(fixture.payload),
			"sha256": tools.Sum256(fixture.payload),
		}},
	})
	if status, out := fixture.fx.publishTools(t, body, toolToken); status != http.StatusBadRequest {
		t.Fatalf("publish under current alias status=%d body=%s", status, out)
	}
	// The alias must still resolve to the real version afterwards.
	response := fixture.fx.toolsGet(t, "/v1/tools/"+toolCurrentAlias+"/manifest.json")
	defer response.Body.Close()
	var manifest tools.Document
	if err := json.NewDecoder(response.Body).Decode(&manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Version != fixture.version {
		t.Fatalf("current alias moved to %q after a rejected upload", manifest.Version)
	}
}

// TestAgentServesCurrentAliasNeedsPublishedCatalog pins the failure mode when a
// server has nothing published yet: a clear 404, not an empty download.
func TestAgentCurrentAliasNeedsPublishedCatalog(t *testing.T) {
	fx := newAgentFixture(t, agentFixtureOpts{deployToken: true})
	response := fx.toolsGet(t, "/v1/tools/"+toolCurrentAlias+"/manifest.json")
	defer response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("status=%d, want 404 with no published catalog", response.StatusCode)
	}
}

// TestAgentResolvesToolByComponent pins the addressing host CI uses: a host
// names "updater for windows-amd64" and never a file name, so a release can
// rename a binary without editing a frozen yaml.
func TestAgentResolvesToolByComponent(t *testing.T) {
	fixture := newToolFixture(t)

	response := fixture.fx.toolsGet(t, "/v1/tools/"+toolCurrentAlias+"/updater/windows-amd64")
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("component download status=%d", response.StatusCode)
	}
	body, _ := io.ReadAll(response.Body)
	if !bytes.Equal(body, fixture.payload) {
		t.Fatalf("component download returned %q", body)
	}
	// The served names and digest let a template save and check the file
	// without parsing the manifest.
	if got := response.Header.Get("X-Relkit-Tool-Name"); got != fixture.name {
		t.Fatalf("X-Relkit-Tool-Name=%q want %q", got, fixture.name)
	}
	if got := response.Header.Get("X-Relkit-Tool-SHA256"); got != tools.Sum256(fixture.payload) {
		t.Fatalf("X-Relkit-Tool-SHA256=%q want %q", got, tools.Sum256(fixture.payload))
	}

	signatureResponse := fixture.fx.toolsGet(t, "/v1/tools/"+toolCurrentAlias+"/updater/windows-amd64/signature")
	defer signatureResponse.Body.Close()
	if signatureResponse.StatusCode != http.StatusOK {
		t.Fatalf("component signature status=%d", signatureResponse.StatusCode)
	}
	armored, _ := io.ReadAll(signatureResponse.Body)
	if !strings.Contains(string(armored), "BEGIN SSH SIGNATURE") {
		t.Fatalf("component signature is not an SSHSIG blob: %q", armored)
	}
}

// TestAgentComponentAddressingIsReadOnly keeps the resolving form from becoming
// an upload path: a file is uploaded by name, never by component.
func TestAgentComponentAddressingIsReadOnly(t *testing.T) {
	fixture := newToolFixture(t)
	for _, method := range []string{http.MethodPut, http.MethodPost} {
		request, err := http.NewRequest(method, fixture.fx.ts.URL+"/v1/tools/"+fixture.version+"/updater/windows-amd64", strings.NewReader("x"))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Authorization", "Bearer "+toolToken)
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		status := response.StatusCode
		response.Body.Close()
		if status != http.StatusMethodNotAllowed {
			t.Fatalf("%s by component status=%d, want 405", method, status)
		}
	}
}

// TestAgentRejectsUnknownComponent pins a legible failure when a host asks for
// something this version does not carry.
func TestAgentRejectsUnknownComponent(t *testing.T) {
	fixture := newToolFixture(t)
	response := fixture.fx.toolsGet(t, "/v1/tools/"+toolCurrentAlias+"/cli/windows-amd64")
	defer response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown component status=%d, want 404", response.StatusCode)
	}
}
