package wrapper

import (
	"os"
	"os/exec"
	"path/filepath"
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
		for _, required := range []string{fingerprint, "resolution.json", "materialize", "doctor"} {
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
	if info.Mode()&0o111 == 0 {
		t.Fatal("relkitw is not executable")
	}
}

func TestGeneratedShellWrapperRunsOneImmutableResolution(t *testing.T) {
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
printf '%s|%s|%s\n' "$1" "${RELKIT_EXPECTED_ROOT_FINGERPRINT:-}" "$*" >> "$WRAPPER_LOG"
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
    cat > "$WRAPPER_ROOT/.relkit/components/bin/relkit" <<'SELECTED'
#!/bin/sh
printf 'selected|%s|%s\n' "$PWD" "$*" >> "$WRAPPER_LOG"
SELECTED
    chmod +x "$WRAPPER_ROOT/.relkit/components/bin/relkit"
    ;;
  *) exit 91 ;;
esac
`
	if err := os.WriteFile(bootstrap, []byte(script), 0o755); err != nil {
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
	logData, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	logText := string(logData)
	if got := strings.Count(logText, "resolve|"+fingerprint); got != 3 {
		t.Fatalf("resolve calls = %d, want initial + explicit refresh + candidate\n%s", got, logText)
	}
	if got := strings.Count(logText, "doctor|"+fingerprint); got != 4 {
		t.Fatalf("doctor calls = %d, want 4\n%s", got, logText)
	}
	if !strings.Contains(logText, ".relkit/candidate/resolution.json --candidate "+candidate) {
		t.Fatalf("candidate did not use an isolated immutable resolution:\n%s", logText)
	}
	if !strings.Contains(logText, "selected|"+root+"|probe two") {
		t.Fatalf("selected CLI did not run from project root with original args:\n%s", logText)
	}
}
