package updater

import (
	"os"
	"path/filepath"
	"testing"

	enginestate "github.com/shichao402/relkit/api/updater/enginestate/v1"
)

func writeTestFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// A macOS host keeps its sidecar at Contents/Resources/runtime/relkit-updater.
// The refresh must replace that file, and must not drop a stray copy at the
// bundle root (outside Contents, which would break the code signature).
func TestRefreshSidecarHonoursNestedRelpath(t *testing.T) {
	const rel = "Contents/Resources/runtime/relkit-updater"
	installRoot := filepath.Join(t.TempDir(), "SvnAutoMerge.app")
	payload := t.TempDir()
	writeTestFile(t, filepath.Join(installRoot, filepath.FromSlash(rel)), "old engine")
	writeTestFile(t, filepath.Join(payload, filepath.FromSlash(rel)), "new engine")

	sess := &enginestate.ApplySessionRecord{
		InstallRoot:    installRoot,
		StagedRoot:     t.TempDir(),
		SidecarRelpath: rel,
	}
	refreshSidecar(sess, payload)

	if got := readTestFile(t, filepath.Join(installRoot, filepath.FromSlash(rel))); got != "new engine" {
		t.Fatalf("nested sidecar = %q, want new engine", got)
	}
	for _, stray := range []string{"relkit-updater", "relkit-updater.exe"} {
		if _, err := os.Stat(filepath.Join(installRoot, stray)); !os.IsNotExist(err) {
			t.Fatalf("stray %s written at install root (err=%v)", stray, err)
		}
	}
	if _, err := os.Stat(filepath.Join(installRoot, filepath.FromSlash(rel)) + ".old"); !os.IsNotExist(err) {
		t.Fatalf("aside copy left behind (err=%v)", err)
	}
}

// When the payload itself carried the sidecar, the content table has already
// written the new bytes; the refresh must leave that file alone.
func TestRefreshSidecarSkipsIdenticalDestination(t *testing.T) {
	const rel = "relkit-updater.exe"
	installRoot := t.TempDir()
	payload := t.TempDir()
	dest := filepath.Join(installRoot, rel)
	writeTestFile(t, dest, "same engine")
	writeTestFile(t, filepath.Join(payload, rel), "same engine")
	if err := os.Chmod(dest, 0o700); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(dest)
	if err != nil {
		t.Fatal(err)
	}

	refreshSidecar(&enginestate.ApplySessionRecord{
		InstallRoot:    installRoot,
		StagedRoot:     t.TempDir(),
		SidecarRelpath: rel,
	}, payload)

	after, err := os.Stat(dest)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(before, after) {
		t.Fatal("identical sidecar was replaced")
	}
	if after.Mode().Perm() != before.Mode().Perm() {
		t.Fatalf("identical sidecar was touched: mode %v -> %v", before.Mode().Perm(), after.Mode().Perm())
	}
}

func TestRefreshSidecarRefusesEscapingRelpath(t *testing.T) {
	base := t.TempDir()
	installRoot := filepath.Join(base, "app")
	payload := filepath.Join(base, "payload")
	if err := os.MkdirAll(installRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(payload, "relkit-updater"), "new engine")
	writeTestFile(t, filepath.Join(base, "relkit-updater"), "outside")

	for _, rel := range []string{"../relkit-updater", "/relkit-updater", "."} {
		refreshSidecar(&enginestate.ApplySessionRecord{
			InstallRoot:    installRoot,
			StagedRoot:     payload,
			SidecarRelpath: rel,
		}, payload)
	}
	if got := readTestFile(t, filepath.Join(base, "relkit-updater")); got != "outside" {
		t.Fatalf("file outside install root overwritten: %q", got)
	}
}
