package e2e_test

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shichao402/relkit/internal/testutil"
	"github.com/shichao402/relkit/internal/vnext/artifactset"
	"github.com/shichao402/relkit/internal/vnext/document"
	"github.com/shichao402/relkit/internal/vnext/project"
)

// TestCLIEndToEndVNextRelease drives the surviving command surface from the
// outside: artifacts produce -> stage -> inspect. There is no host-side
// resolution any more: the project declaration and the ArtifactSet the build
// produced are the only control-plane inputs a host repository holds
// (requirement 14.1).
func TestCLIEndToEndVNextRelease(t *testing.T) {
	exe := buildRelkit(t)
	root := t.TempDir()
	fixture := newVNextCLIFixture(t, root)

	artifactSetDir := filepath.Join(root, ".relkit", "artifact-sets", "1.0.0", "windows-amd64")
	artifactSetPath := filepath.Join(artifactSetDir, "artifact-set.json")
	produceOut := runRelkit(t, exe, root, map[string]string{"RELKIT_E2E_PRODUCER_HELPER": "1"}, 0,
		"artifacts", "produce",
		"--project", fixture.projectPath,
		"--version", "1.0.0",
		"--channel", "dev",
		"--target", "windows-amd64",
		"--out-dir", artifactSetDir,
		"--out", artifactSetPath,
	)
	assertContains(t, produceOut, "produced 1 artifact(s)")

	stageOut := runRelkit(t, exe, root, nil, 0,
		"stage",
		"--project", fixture.projectPath,
		"--artifact-set", artifactSetPath,
		"--code", "1",
	)
	assertContains(t, stageOut, "staged 1.0.0")

	stagedRoot := filepath.Join(root, ".relkit", "cache", "staged", "1.0.0")
	stagedOut := runRelkit(t, exe, root, nil, 0, "inspect", "--file", filepath.Join(stagedRoot, "staged.pb"))
	assertContains(t, stagedOut, "1.0.0")

	// The staged tree carries exactly what the build produced: the staged
	// document, the project declaration, the ArtifactSet, and the artifacts.
	for _, name := range []string{"staged.pb", "relkit.project.json", "artifact-set.json", "artifacts/demo-1.0.0-windows-amd64.zip"} {
		if _, err := os.Stat(filepath.Join(stagedRoot, name)); err != nil {
			t.Fatalf("staged tree is missing %s: %v", name, err)
		}
	}
	// The retired host-side picking artifacts must not come back.
	for _, gone := range []string{"resolution.json", "resolution-bundle.json", "release-policy.json"} {
		if _, err := os.Stat(filepath.Join(stagedRoot, gone)); !os.IsNotExist(err) {
			t.Fatalf("staged tree still carries the retired %s", gone)
		}
	}
}

// TestCLIRetiredCommandsAreGone pins the control-plane deletion: the console
// must not silently accept a verb that no longer has a code path behind it.
func TestCLIRetiredCommandsAreGone(t *testing.T) {
	exe := buildRelkit(t)
	root := t.TempDir()
	for _, command := range []string{
		"init", "keygen", "publish", "simulate", "backends", "check", "install",
		"upgrade", "status", "ci", "consume", "onboard",
		"resolve", "doctor", "materialize", "wrapper", "verify", "version", "trust",
	} {
		out := runRelkit(t, exe, root, nil, 2, command)
		assertContains(t, out, "unknown command")
	}
	usage := runRelkit(t, exe, root, nil, 2)
	for _, command := range []string{"artifacts", "payload", "stage", "inspect", "release", "build", "custody"} {
		assertContains(t, usage, command)
	}
}

// TestArtifactProducerHelperProcess is the child process the e2e fixture points
// its artifactProducer at. It speaks the documented stdin/stdout contract so the
// CLI exercises the same extension boundary a host repository would.
func TestArtifactProducerHelperProcess(t *testing.T) {
	if os.Getenv("RELKIT_E2E_PRODUCER_HELPER") != "1" {
		return
	}
	var input struct {
		Product         string `json:"product"`
		Version         string `json:"version"`
		Channel         string `json:"channel"`
		Target          string `json:"target"`
		OutputDirectory string `json:"outputDirectory"`
	}
	if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
		os.Exit(81)
	}
	content := []byte("e2e artifact " + input.Version + " " + input.Target)
	name := "demo-" + input.Version + "-" + input.Target + ".zip"
	if err := os.WriteFile(filepath.Join(input.OutputDirectory, name), content, 0o644); err != nil {
		os.Exit(82)
	}
	sum := sha256.Sum256(content)
	output := artifactset.Document{
		Schema:      artifactset.Schema,
		Product:     input.Product,
		Version:     input.Version,
		ReleaseCode: 1,
		Channel:     input.Channel,
		Artifacts: []artifactset.Artifact{
			{
				ID:       "desktop",
				Path:     name,
				Kind:     artifactset.KindBlob,
				Platform: input.Target,
				SHA256:   "sha256:" + fmt.Sprintf("%x", sum[:]),
				Size:     int64(len(content)),
			},
		},
	}
	if err := json.NewEncoder(os.Stdout).Encode(output); err != nil {
		os.Exit(83)
	}
	os.Exit(0)
}

type vNextCLIFixture struct {
	projectPath string
}

// newVNextCLIFixture writes the only control-plane declaration a host now
// commits: the server address, that server's root public key, and the product.
func newVNextCLIFixture(t *testing.T, root string) vNextCLIFixture {
	t.Helper()
	seed := make([]byte, ed25519.SeedSize)
	for i := range seed {
		seed[i] = byte(i + 23)
	}
	publicKey := ed25519.NewKeyFromSeed(seed).Public().(ed25519.PublicKey)

	productDocument := project.Document{
		Schema:             project.Schema,
		Product:            "demo",
		Server:             "https://updates.example",
		RootPublicKey:      base64.StdEncoding.EncodeToString(publicKey),
		Channels:           []string{"dev", "stable"},
		SelectorDimensions: []string{"os-arch"},
		ArtifactProducer: &project.Command{
			Command:         []string{os.Args[0], "-test.run=TestArtifactProducerHelperProcess"},
			ExtensionSchema: "relkit.extension/1",
			EnvAllowlist:    []string{"RELKIT_E2E_PRODUCER_HELPER"},
			Permissions:     []string{"read-workspace", "write-out-dir", "exec-self"},
		},
	}
	projectPath := filepath.Join(root, "relkit.project.json")
	if err := document.Write(projectPath, productDocument); err != nil {
		t.Fatal(err)
	}
	return vNextCLIFixture{projectPath: projectPath}
}

func buildRelkit(t *testing.T) string {
	t.Helper()
	exe := filepath.Join(t.TempDir(), "relkit.exe")
	cmd := exec.Command("go", "build", "-o", exe, "./cmd/relkit")
	cmd.Dir = testutil.RepoRoot(t)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go build failed: %v\n%s", err, output)
	}
	return exe
}

func runRelkit(t *testing.T, exe string, cwd string, extraEnv map[string]string, expectCode int, args ...string) string {
	t.Helper()
	cmd := exec.Command(exe, args...)
	cmd.Dir = cwd
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, "GODEBUG=http2client=0")
	for key, value := range extraEnv {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	output, err := cmd.CombinedOutput()
	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if ok := asExitError(err, &exitErr); ok {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("command failed to start: %v", err)
		}
	}
	if exitCode != expectCode {
		t.Fatalf("relkit %s exit mismatch: got %d want %d\n%s", strings.Join(args, " "), exitCode, expectCode, output)
	}
	return string(output)
}

func asExitError(err error, target **exec.ExitError) bool {
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		return false
	}
	*target = exitErr
	return true
}

func assertContains(t *testing.T, text string, want string) {
	t.Helper()
	if !strings.Contains(text, want) {
		t.Fatalf("expected output to contain %q\n%s", want, text)
	}
}
