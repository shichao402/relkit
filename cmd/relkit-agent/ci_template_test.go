package main

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/shichao402/relkit/internal/vnext/tools"
)

// The CI template is the only relkit code a host build machine runs before it
// has any relkit code (requirement 12.5). These tests drive the template
// against a real agent server instead of re-implementing its steps in Go, so a
// change that breaks the frozen three-command shape fails here rather than in a
// host repository.
const ciScriptPath = "../../scripts/ci/relkit-tool-verify.ps1"

// ciFixture publishes one tool the way the deploy pipeline does and supplies
// exactly the environment a host project would hold.
type ciFixture struct {
	fixture *toolFixture
	dir     string
}

func newCIFixture(t *testing.T) *ciFixture {
	t.Helper()
	return &ciFixture{fixture: newToolFixture(t), dir: t.TempDir()}
}

// env is the template's input: the server address and the root public key in
// the exact textual form the server publishes at /v1/tools/root.pub.
func (f *ciFixture) env() map[string]string {
	encoded, err := tools.EncodePublicKey(f.fixture.fx.rootPublic())
	if err != nil {
		panic(err)
	}
	return map[string]string{
		"RELKIT_SERVER":          f.fixture.fx.ts.URL,
		"RELKIT_ROOT_PUBLIC_KEY": encoded,
		"RELKIT_TOOL":            "updater",
		"RELKIT_PLATFORM":        "windows-amd64",
		"RELKIT_TOOL_DIR":        f.dir,
	}
}

// run executes the template with a minimal environment plus exactly the
// template inputs, so a stray RELKIT_* variable in the developer's shell cannot
// make a broken template look green.
func (f *ciFixture) run(t *testing.T, overrides map[string]string) (string, string, error) {
	t.Helper()
	if runtime.GOOS != "windows" {
		t.Skip("the PowerShell template only runs on Windows build machines")
	}
	pwsh, err := exec.LookPath("powershell")
	if err != nil {
		t.Skip("powershell is not installed")
	}
	script, err := filepath.Abs(ciScriptPath)
	if err != nil {
		t.Fatal(err)
	}
	environment := f.env()
	for key, value := range overrides {
		environment[key] = value
	}
	command := exec.Command(pwsh, "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", script)
	command.Dir = f.dir
	// Inherit the machine environment (a real build agent has PATH, PATHEXT and
	// the OpenSSH directory) but drop every RELKIT_* variable first, so a stray
	// value in the developer's shell cannot make a broken template look green.
	for _, entry := range os.Environ() {
		if strings.HasPrefix(strings.ToUpper(entry), "RELKIT_") {
			continue
		}
		command.Env = append(command.Env, entry)
	}
	for key, value := range environment {
		command.Env = append(command.Env, key+"="+value)
	}
	var stdout, stderr strings.Builder
	command.Stdout = &stdout
	command.Stderr = &stderr
	err = command.Run()
	return stdout.String(), stderr.String(), err
}

// TestCITemplateDownloadsVerifiesAndReturnsTheTool is the end-to-end proof for
// requirement 12.2: download from the server, verify with the build machine's
// ssh-keygen, hand the tool back. Nothing in the template names a version.
func TestCITemplateDownloadsVerifiesAndReturnsTheTool(t *testing.T) {
	f := newCIFixture(t)
	stdout, stderr, err := f.run(t, nil)
	if err != nil {
		t.Fatalf("template failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	lines := strings.Fields(strings.TrimSpace(stdout))
	if len(lines) != 1 {
		t.Fatalf("template printed %d lines, want the tool path only: %q", len(lines), stdout)
	}
	toolPath := lines[0]
	if filepath.Base(toolPath) != f.fixture.name {
		t.Fatalf("template returned %q, want %q", toolPath, f.fixture.name)
	}
	downloaded, err := os.ReadFile(toolPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(downloaded) != string(f.fixture.payload) {
		t.Fatalf("downloaded %q, want %q", downloaded, f.fixture.payload)
	}
}

// TestCITemplateFailsOnForeignRoot pins the no-fallback rule: a project pinning
// the wrong root must fail loudly instead of installing an unverified tool.
func TestCITemplateFailsOnForeignRoot(t *testing.T) {
	f := newCIFixture(t)
	other := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	_, stderr, runErr := f.run(t, map[string]string{"RELKIT_ROOT_PUBLIC_KEY": other})
	if runErr == nil {
		t.Fatal("the template accepted a tool signed by a key the project does not pin")
	}
	if !strings.Contains(stderr, "ssh-keygen rejected") {
		t.Fatalf("failure did not name the signature check: %s", stderr)
	}
}

// TestCITemplateOverwritesAnUnverifiedFile proves the signature covers the bytes
// that end up on disk: a file planted at the target path must not survive.
func TestCITemplateOverwritesAnUnverifiedFile(t *testing.T) {
	f := newCIFixture(t)
	target := filepath.Join(f.dir, f.fixture.name)
	if err := os.WriteFile(target, []byte("forged"), 0o644); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, err := f.run(t, nil)
	if err != nil {
		t.Fatalf("template failed: %v\n%s", err, stderr)
	}
	downloaded, err := os.ReadFile(strings.TrimSpace(stdout))
	if err != nil {
		t.Fatal(err)
	}
	if string(downloaded) != string(f.fixture.payload) {
		t.Fatalf("downloaded %q, want the published payload", downloaded)
	}
}

// TestCITemplateRequiresServerAndKey keeps the template from quietly running
// against an empty configuration.
func TestCITemplateRequiresServerAndKey(t *testing.T) {
	f := newCIFixture(t)
	for _, missing := range []string{"RELKIT_SERVER", "RELKIT_ROOT_PUBLIC_KEY", "RELKIT_TOOL", "RELKIT_PLATFORM"} {
		_, stderr, err := f.run(t, map[string]string{missing: " "})
		if err == nil {
			t.Fatalf("template ran without %s", missing)
		}
		if !strings.Contains(stderr, missing) {
			t.Fatalf("failure did not name %s: %s", missing, stderr)
		}
	}
}

// TestCICurrentAliasServesThePublishedVersion keeps the alias honest: the
// version CI resolves is the version the server marked current.
func TestCICurrentAliasServesThePublishedVersion(t *testing.T) {
	f := newCIFixture(t)
	response := f.fixture.fx.toolsGet(t, "/v1/tools/"+toolCurrentAlias+"/manifest.json")
	defer response.Body.Close()
	var document tools.Document
	if err := json.NewDecoder(response.Body).Decode(&document); err != nil {
		t.Fatal(err)
	}
	if document.Version != f.fixture.version {
		t.Fatalf("current alias served %q, want %q", document.Version, f.fixture.version)
	}
	if _, ok := document.Component("updater", "windows-amd64"); !ok {
		t.Fatal("current manifest has no updater row for windows-amd64")
	}
}

// TestCIAllowedSignerLineIsWhatSSHKeygenAccepts pins the format that cost us a
// debugging round: principal first. An authorized_keys-shaped line (key first,
// keyId trailing as a comment) is rejected by ssh-keygen with a message about
// the signature, not about the file.
func TestCIAllowedSignerLineIsWhatSSHKeygenAccepts(t *testing.T) {
	sshKeygen, err := exec.LookPath("ssh-keygen")
	if err != nil {
		t.Skip("ssh-keygen is not installed on this machine")
	}
	f := newCIFixture(t)
	publicKey := f.fixture.fx.rootPublic()
	dir := t.TempDir()

	good := filepath.Join(dir, "good_signers")
	if err := os.WriteFile(good, []byte(tools.AllowedSignerLine("relkit-root", publicKey)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Same key, wrong shape: key first, keyId trailing as a comment.
	bad := filepath.Join(dir, "bad_signers")
	wire := strings.TrimPrefix(tools.AllowedSignerLine("relkit-root", publicKey), "relkit-root ")
	if err := os.WriteFile(bad, []byte(wire+" relkit-root\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Use the bytes and the signature the server actually serves, so this
	// asserts what CI consumes rather than a locally generated signature.
	payloadPath := filepath.Join(dir, f.fixture.name)
	signaturePath := payloadPath + ".sig"
	payloadResponse := f.fixture.fx.toolsGet(t, "/v1/tools/"+toolCurrentAlias+"/"+f.fixture.name)
	payload, _ := io.ReadAll(payloadResponse.Body)
	payloadResponse.Body.Close()
	if err := os.WriteFile(payloadPath, payload, 0o644); err != nil {
		t.Fatal(err)
	}
	signatureResponse := f.fixture.fx.toolsGet(t, "/v1/tools/"+toolCurrentAlias+"/"+f.fixture.name+".sig")
	armored, _ := io.ReadAll(signatureResponse.Body)
	signatureResponse.Body.Close()
	if err := os.WriteFile(signaturePath, armored, 0o644); err != nil {
		t.Fatal(err)
	}

	run := func(allowlist string) error {
		command := exec.Command(sshKeygen, "-Y", "verify",
			"-f", allowlist, "-I", "relkit-root", "-n", tools.SignatureNamespace, "-s", signaturePath)
		file, err := os.Open(payloadPath)
		if err != nil {
			return err
		}
		defer file.Close()
		command.Stdin = file
		return command.Run()
	}
	if err := run(good); err != nil {
		t.Fatalf("allowed_signers line was rejected: %v", err)
	}
	if err := run(bad); err == nil {
		t.Fatal("an authorized_keys-shaped line was accepted; the format distinction is not enforced")
	}
}

// TestCIUnknownComponentIsAHonest404 keeps the template's failure legible when
// a host asks for a component this version does not carry.
func TestCIUnknownComponentIsAHonest404(t *testing.T) {
	f := newCIFixture(t)
	response := f.fixture.fx.toolsGet(t, "/v1/tools/"+toolCurrentAlias+"/relkit-cli-windows-amd64.exe")
	defer response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("missing tool status=%d, want 404", response.StatusCode)
	}
}
