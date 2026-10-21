package e2e_test

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	rupv2 "github.com/shichao402/relkit/api/rup/v2"
	"github.com/shichao402/relkit/internal/keys"
	"github.com/shichao402/relkit/internal/testutil"
	"github.com/shichao402/relkit/internal/vnext/artifactset"
	"github.com/shichao402/relkit/internal/vnext/catalog"
	"github.com/shichao402/relkit/internal/vnext/document"
	"github.com/shichao402/relkit/internal/vnext/operator"
	"github.com/shichao402/relkit/internal/vnext/project"
	"github.com/shichao402/relkit/internal/vnext/trust"
)

// TestCLIEndToEndVNextRelease drives the entire surviving command surface from
// the outside: resolve -> artifacts produce -> stage -> inspect -> doctor ->
// wrapper generate. ADR 0019 §1 retired the legacy command set (init, keygen,
// publish, simulate, backends, ...), so this is what "end to end" now means for
// the CLI.
func TestCLIEndToEndVNextRelease(t *testing.T) {
	exe := buildRelkit(t)
	root := t.TempDir()
	fixture := newVNextCLIFixture(t, root)

	outputPath := filepath.Join(root, ".relkit", "resolution.json")
	resolveOut := runRelkit(t, exe, root, nil, 0,
		"resolve",
		"--project", fixture.projectPath,
		"--operator", fixture.operatorPath,
		"--catalog", fixture.catalogPath,
		"--trust-key", fixture.publicKeyPath,
		"--out", outputPath,
	)
	assertContains(t, resolveOut, "resolution")

	inspected := runRelkit(t, exe, root, nil, 0, "inspect", "--resolution", outputPath)
	assertContains(t, inspected, "\"distribution\"")
	assertContains(t, inspected, "\"identity\"")

	doctorOut := runRelkit(t, exe, root, nil, 0, "doctor",
		"--project", fixture.projectPath,
		"--resolution", outputPath,
	)
	assertContains(t, doctorOut, "ok")

	artifactSetPath := filepath.Join(root, ".relkit", "artifact-sets", "1.0.0", "windows-amd64", "artifact-set.json")
	produceOut := runRelkit(t, exe, root, map[string]string{"RELKIT_E2E_PRODUCER_HELPER": "1"}, 0,
		"artifacts", "produce",
		"--project", fixture.projectPath,
		"--resolution", outputPath,
		"--version", "1.0.0",
		"--channel", "dev",
		"--target", "windows-amd64",
		"--out", artifactSetPath,
	)
	assertContains(t, produceOut, "produced 1 artifact(s)")

	stageOut := runRelkit(t, exe, root, nil, 0,
		"stage",
		"--project", fixture.projectPath,
		"--resolution", outputPath,
		"--artifact-set", artifactSetPath,
		"--code", "1",
	)
	assertContains(t, stageOut, "staged 1.0.0")

	stagedPath := filepath.Join(root, ".relkit", "cache", "staged", "1.0.0", "staged.pb")
	stagedOut := runRelkit(t, exe, root, nil, 0, "inspect", "--file", stagedPath)
	assertContains(t, stagedOut, "1.0.0")

	// The staged tree must carry vNext provenance, not a retired policy sidecar.
	for _, name := range []string{"resolution.json", "resolution-bundle.json", "relkit.project.json", "artifact-set.json"} {
		path := filepath.Join(root, ".relkit", "cache", "staged", "1.0.0", name)
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("staged tree is missing %s: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, ".relkit", "cache", "staged", "1.0.0", "release-policy.json")); !os.IsNotExist(err) {
		t.Fatalf("staged tree still carries the retired release-policy.json: %v", err)
	}

	wrapperOut := runRelkit(t, exe, root, nil, 0,
		"wrapper", "generate",
		"--project", fixture.projectPath,
		"--out-dir", root,
	)
	assertContains(t, wrapperOut, "relkitw")
	for _, name := range []string{"relkitw", "relkitw.cmd"} {
		if _, err := os.Stat(filepath.Join(root, name)); err != nil {
			t.Fatalf("wrapper generate did not write %s: %v", name, err)
		}
	}
}

// TestCLIRetiredCommandsAreGone pins the ADR 0019 §1 control-plane deletion:
// the console must not silently accept a legacy verb that no longer has a code
// path behind it.
func TestCLIRetiredCommandsAreGone(t *testing.T) {
	exe := buildRelkit(t)
	root := t.TempDir()
	for _, command := range []string{
		"init", "keygen", "publish", "simulate", "backends", "check", "install",
		"upgrade", "status", "ci", "consume", "onboard",
	} {
		out := runRelkit(t, exe, root, nil, 2, command)
		assertContains(t, out, "unknown command")
	}
	usage := runRelkit(t, exe, root, nil, 2)
	assertContains(t, usage, "resolve")
	assertContains(t, usage, "stage")
	assertContains(t, usage, "release")
	if strings.Contains(usage, "publish") || strings.Contains(usage, "consume") {
		t.Fatalf("usage still advertises a retired command:\n%s", usage)
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
		ResolutionDigest string `json:"resolutionDigest"`
		Product          string `json:"product"`
		Version          string `json:"version"`
		Channel          string `json:"channel"`
		Target           string `json:"target"`
		OutputDirectory  string `json:"outputDirectory"`
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
		Schema:           artifactset.Schema,
		ResolutionDigest: input.ResolutionDigest,
		Product:          input.Product,
		Version:          input.Version,
		ReleaseCode:      1,
		Channel:          input.Channel,
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
	projectPath    string
	operatorPath   string
	catalogPath    string
	publicKeyPath  string
	distributionID string
}

func newVNextCLIFixture(t *testing.T, root string) vNextCLIFixture {
	t.Helper()
	seed := make([]byte, ed25519.SeedSize)
	for i := range seed {
		seed[i] = byte(i + 23)
	}
	publicKey := ed25519.NewKeyFromSeed(seed).Public().(ed25519.PublicKey)
	fingerprint, err := trust.Fingerprint(publicKey)
	if err != nil {
		t.Fatal(err)
	}
	publicDocument := keys.PublicKeyDocument("distribution-root", seed)
	publicBytes, err := rupv2.MarshalPublicKey(&publicDocument)
	if err != nil {
		t.Fatal(err)
	}
	publicKeyPath := filepath.Join(root, "distribution.public.pb")
	if err := os.WriteFile(publicKeyPath, publicBytes, 0o644); err != nil {
		t.Fatal(err)
	}

	digest := "sha256:" + strings.Repeat("a", 64)
	productDocument := project.Document{
		Schema:           project.Schema,
		Product:          "demo",
		Channels:         []string{"dev", "stable"},
		OperatorRef:      "prod",
		TrustFingerprint: fingerprint,
		Roles:            []project.Role{project.RolePublisher},
		ArtifactProducer: &project.Command{
			Command:         []string{os.Args[0], "-test.run=TestArtifactProducerHelperProcess"},
			ExtensionSchema: "relkit.extension/1",
			EnvAllowlist:    []string{"RELKIT_E2E_PRODUCER_HELPER"},
			Permissions:     []string{"read-workspace", "write-out-dir", "exec-self"},
		},
		CapabilityFingerprint: project.CapabilityFingerprint{
			OSArch:   []string{"windows-amd64"},
			Topology: "producer-only",
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
		Sequence:        1,
	}
	if err := operatorDocument.Sign("distribution-root", seed); err != nil {
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
					{
						Name: "cli", Version: "v1.0.0", Platform: "windows-amd64",
						SHA256: digest,
						Source: catalog.Source{Kind: "release-asset", URL: "https://example.invalid/cli"},
					},
				},
			},
		},
	}
	if err := catalogDocument.Sign("distribution-root", seed); err != nil {
		t.Fatal(err)
	}
	catalogPath := filepath.Join(root, "catalog.json")
	if err := document.Write(catalogPath, catalogDocument); err != nil {
		t.Fatal(err)
	}

	return vNextCLIFixture{
		projectPath:    projectPath,
		operatorPath:   operatorPath,
		catalogPath:    catalogPath,
		publicKeyPath:  publicKeyPath,
		distributionID: "v1.0.0",
	}
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
