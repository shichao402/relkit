package wrapper

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/shichao402/relkit/internal/vnext/document"
	"github.com/shichao402/relkit/internal/vnext/project"
)

func TestGeneratePinsTrustRootAndUsesResolution(t *testing.T) {
	root := t.TempDir()
	fingerprint := "sha256:" + strings.Repeat("a", 64)
	value := project.Document{
		Schema:           project.Schema,
		Product:          "demo",
		OperatorRef:      "prod",
		TrustFingerprint: fingerprint,
		Roles:            []project.Role{project.RoleUpdaterHost},
		CapabilityFingerprint: project.CapabilityFingerprint{
			OSArch:   []string{"darwin-arm64"},
			Facades:  []string{"rust"},
			Topology: "self-update",
		},
	}
	projectPath := filepath.Join(root, "relkit.project.json")
	if err := document.Write(projectPath, value); err != nil {
		t.Fatal(err)
	}
	paths, err := Generate(Options{ProjectPath: projectPath, OutputDir: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 2 {
		t.Fatalf("got %d paths, want 2", len(paths))
	}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		text := string(data)
		for _, required := range []string{fingerprint, "resolution.json", "materialize", "doctor", "RELKIT_PROJECT", "candidate-root-fingerprint"} {
			if !strings.Contains(text, required) {
				t.Fatalf("%s does not contain %q", filepath.Base(path), required)
			}
		}
		if strings.Contains(text, "latest") {
			t.Fatalf("%s must not resolve an unscoped latest", filepath.Base(path))
		}
	}
	info, err := os.Stat(filepath.Join(root, "relkitw"))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode()&0o111 == 0 {
		t.Fatal("relkitw is not executable")
	}
}

func TestGeneratedShellWrapperRunsOneImmutableResolution(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell wrapper execution is covered on Unix runners")
	}
	root := t.TempDir()
	fingerprint := "sha256:" + strings.Repeat("b", 64)
	value := project.Document{
		Schema:           project.Schema,
		Product:          "demo",
		OperatorRef:      "prod",
		TrustFingerprint: fingerprint,
		Roles:            []project.Role{project.RoleUpdaterProjection},
		CapabilityFingerprint: project.CapabilityFingerprint{
			OSArch:      []string{"darwin-arm64"},
			Projections: []string{"typescript"},
			Topology:    "producer-only",
		},
	}
	projectPath := filepath.Join(root, "relkit.project.json")
	if err := document.Write(projectPath, value); err != nil {
		t.Fatal(err)
	}
	if _, err := Generate(Options{ProjectPath: projectPath, OutputDir: root}); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(root, "calls.log")
	bootstrap := filepath.Join(root, "fake-bootstrap")
	script := `#!/bin/sh
set -eu
printf '%s|%s|%s|%s|%s\n' "$1" "${RELKIT_EXPECTED_ROOT_FINGERPRINT:-}" "${RELKIT_CANDIDATE_ROOT_FINGERPRINT:-}" "${RELKIT_PROJECT:-}" "$*" >> "$WRAPPER_LOG"
command_name="$1"
shift
case "$command_name" in
  resolve)
    out=
    while [ "$#" -gt 0 ]; do
      if [ "$1" = "--out" ]; then out="$2"; shift 2; else shift; fi
    done
    mkdir -p "$(dirname "$out")"
    printf '{}\n' > "$out"
    ;;
  doctor) ;;
  materialize)
    mkdir -p "$WRAPPER_ROOT/.relkit/components/bin"
    if [ "${SKIP_SELECTED:-0}" = "1" ]; then
      rm -f "$WRAPPER_ROOT/.relkit/components/bin/relkit"
    else
      cat > "$WRAPPER_ROOT/.relkit/components/bin/relkit" <<'SELECTED'
#!/bin/sh
printf 'selected|%s|%s|%s\n' "$PWD" "${RELKIT_PROJECT:-}" "$*" >> "$WRAPPER_LOG"
SELECTED
      chmod +x "$WRAPPER_ROOT/.relkit/components/bin/relkit"
    fi
    ;;
  probe) printf 'bootstrap|%s|%s|%s\n' "$PWD" "${RELKIT_PROJECT:-}" "$*" >> "$WRAPPER_LOG" ;;
  *) exit 91 ;;
esac
`
	if err := os.WriteFile(bootstrap, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	candidateRoot := "sha256:" + strings.Repeat("d", 64)
	if err := os.MkdirAll(filepath.Join(root, ".relkit"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(root, ".relkit", "candidate-root-fingerprint"),
		[]byte(candidateRoot+"\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	run := func(extra ...string) {
		t.Helper()
		command := exec.Command(filepath.Join(root, "relkitw"), "probe", "two")
		command.Env = append(
			os.Environ(),
			"RELKIT_BOOTSTRAP_CLI="+bootstrap,
			"WRAPPER_LOG="+logPath,
			"WRAPPER_ROOT="+root,
			"RELKIT_CANDIDATE_ROOT_FINGERPRINT=",
		)
		command.Env = append(command.Env, extra...)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("wrapper failed: %v\n%s", err, output)
		}
	}
	run()
	run()
	run("RELKIT_REFRESH_RESOLUTION=1")
	candidate := "sha256:" + strings.Repeat("c", 64)
	run("RELKIT_CANDIDATE_DIGEST=" + candidate)
	run("SKIP_SELECTED=1")
	logData, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	logText := string(logData)
	if got := strings.Count(logText, "resolve|"+fingerprint); got != 3 {
		t.Fatalf("resolve calls = %d, want initial + explicit refresh + candidate\n%s", got, logText)
	}
	if got := strings.Count(logText, "doctor|"+fingerprint); got != 5 {
		t.Fatalf("doctor calls = %d, want 5\n%s", got, logText)
	}
	if !strings.Contains(logText, "doctor|"+fingerprint+"|"+candidateRoot) {
		t.Fatalf("wrapper did not load the installed Candidate Root:\n%s", logText)
	}
	if !strings.Contains(logText, ".relkit/candidate/resolution.json --candidate "+candidate) {
		t.Fatalf("candidate did not use an isolated immutable resolution:\n%s", logText)
	}
	if !strings.Contains(logText, "selected|"+root+"|"+projectPath+"|probe two") {
		t.Fatalf("selected CLI did not preserve project context, project root, and original args:\n%s", logText)
	}
	if !strings.Contains(logText, "bootstrap|"+root+"|"+projectPath+"|two") {
		t.Fatalf("bootstrap CLI did not preserve project context outside the product targets:\n%s", logText)
	}
}

func TestGeneratedWindowsWrapperUsesSafeRootAndHardFailures(t *testing.T) {
	script := cmdScript("relkit.project.json", "sha256:"+strings.Repeat("d", 64), "operator")
	if !strings.Contains(script, `set "PROJECT_ROOT=%ROOT%."`) {
		t.Fatal("Windows wrapper does not remove the trailing separator from executable arguments")
	}
	if !strings.Contains(script, `--project-root "%PROJECT_ROOT%"`) {
		t.Fatal("Windows wrapper does not pass the safe project root")
	}
	if !strings.Contains(script, `candidate-root-fingerprint`) {
		t.Fatal("Windows wrapper does not load the installed Candidate Root")
	}
	if !strings.Contains(script, `set "RELKIT_PROJECT=%PROJECT%"`) {
		t.Fatal("Windows wrapper does not preserve project context for selected CLIs")
	}
	if !strings.Contains(script, `set "SELECTED=%BOOTSTRAP%"`) {
		t.Fatal("Windows wrapper does not support an operator host outside the product targets")
	}
	if strings.Contains(script, `|| exit /b %errorlevel%`) {
		t.Fatal("Windows wrapper expands a stale errorlevel on the same command line")
	}
}

func TestGeneratedWindowsWrapperRunsAndPropagatesMaterializeFailure(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows command-line parsing is covered on Windows runners")
	}
	root := filepath.Join(t.TempDir(), "project with spaces")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	value := project.Document{
		Schema:           project.Schema,
		Product:          "demo",
		OperatorRef:      "prod",
		TrustFingerprint: "sha256:" + strings.Repeat("e", 64),
		Roles:            []project.Role{project.RoleUpdaterHost},
		CapabilityFingerprint: project.CapabilityFingerprint{
			OSArch:   []string{"windows-amd64"},
			Facades:  []string{"go"},
			Topology: "self-update",
		},
	}
	projectPath := filepath.Join(root, "relkit.project.json")
	if err := document.Write(projectPath, value); err != nil {
		t.Fatal(err)
	}
	if _, err := Generate(Options{ProjectPath: projectPath, OutputDir: root}); err != nil {
		t.Fatal(err)
	}

	helperSource := filepath.Join(t.TempDir(), "main.go")
	helperBinary := filepath.Join(filepath.Dir(helperSource), "fake-bootstrap.exe")
	const helper = `package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func value(name string) string {
	for index := 1; index+1 < len(os.Args); index++ {
		if os.Args[index] == name {
			return os.Args[index+1]
		}
	}
	return ""
}

func appendLog(line string) {
	file, err := os.OpenFile(os.Getenv("WRAPPER_LOG"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		panic(err)
	}
	defer file.Close()
	fmt.Fprintln(file, line)
}

func copySelf(destination string) {
	sourcePath, err := os.Executable()
	if err != nil {
		panic(err)
	}
	source, err := os.Open(sourcePath)
	if err != nil {
		panic(err)
	}
	defer source.Close()
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		panic(err)
	}
	output, err := os.Create(destination)
	if err != nil {
		panic(err)
	}
	if _, err := io.Copy(output, source); err != nil {
		output.Close()
		panic(err)
	}
	if err := output.Close(); err != nil {
		panic(err)
	}
}

func main() {
	if len(os.Args) < 2 {
		os.Exit(90)
	}
	switch os.Args[1] {
	case "resolve":
		output := value("--out")
		if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
			panic(err)
		}
		if err := os.WriteFile(output, []byte("{}\n"), 0o644); err != nil {
			panic(err)
		}
	case "doctor":
	case "materialize":
		root := value("--project-root")
		appendLog("materialize-root=" + root)
		if os.Getenv("FAIL_MATERIALIZE") == "1" {
			os.Exit(37)
		}
		copySelf(filepath.Join(root, ".relkit", "components", "bin", "relkit.exe"))
	case "probe":
		appendLog("selected=" + fmt.Sprint(os.Args[1:]))
		appendLog("selected-project=" + os.Getenv("RELKIT_PROJECT"))
	default:
		os.Exit(91)
	}
}
`
	if err := os.WriteFile(helperSource, []byte(helper), 0o644); err != nil {
		t.Fatal(err)
	}
	build := exec.Command("go", "build", "-o", helperBinary, helperSource)
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build helper: %v\n%s", err, output)
	}

	logPath := filepath.Join(root, "calls.log")
	run := func(fail bool) error {
		command := exec.Command("cmd.exe", "/d", "/c", `call .\relkitw.cmd probe two`)
		command.Dir = root
		command.Env = append(os.Environ(),
			"RELKIT_BOOTSTRAP_CLI="+helperBinary,
			"WRAPPER_LOG="+logPath,
		)
		if fail {
			command.Env = append(command.Env, "FAIL_MATERIALIZE=1")
		}
		output, err := command.CombinedOutput()
		if err != nil && !fail {
			t.Fatalf("wrapper failed: %v\n%s", err, output)
		}
		return err
	}
	if err := run(true); err == nil {
		t.Fatal("materialize failure was reported as success")
	}
	if err := run(false); err != nil {
		t.Fatal(err)
	}
	logData, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	var materializedRoot string
	for _, line := range strings.Split(string(logData), "\n") {
		if strings.HasPrefix(line, "materialize-root=") {
			materializedRoot = strings.TrimPrefix(line, "materialize-root=")
		}
	}
	if strings.Contains(materializedRoot, `"`) {
		t.Fatalf("project root contains a stray quote: %q", materializedRoot)
	}
	if filepath.Clean(materializedRoot) != filepath.Clean(root) {
		t.Fatalf("project root = %q, want %q", materializedRoot, root)
	}
	if !strings.Contains(string(logData), "selected=[probe two]") {
		t.Fatalf("selected CLI did not receive original arguments:\n%s", logData)
	}
	if !strings.Contains(string(logData), "selected-project="+projectPath) {
		t.Fatalf("selected CLI lost project context %q:\n%s", projectPath, logData)
	}
}
