package updater

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	enginestate "github.com/shichao402/relkit/api/updater/enginestate/v1"
	updaterv2 "github.com/shichao402/relkit/api/updater/v2"
	"github.com/shichao402/relkit/internal/model"
	"github.com/shichao402/relkit/internal/payload"
)

// ADR 0027 step 0: fresh write for LIBRARY, prune by code with in-use
// protection, and the managed-library coexistence duties of reconcile write.

func stubRunning(t *testing.T, exes ...string) {
	t.Helper()
	prev := listRunningExecutables
	listRunningExecutables = func() []string { return exes }
	t.Cleanup(func() { listRunningExecutables = prev })
}

func librarySession(root, stage string) *enginestate.ApplySessionRecord {
	return &enginestate.ApplySessionRecord{
		InstallRoot:       root,
		StagedRoot:        stage,
		Placement:         updaterv2.Placement_PLACEMENT_LIBRARY,
		ExecutableRelpath: "bin/app.exe",
		Library:           &updaterv2.LibraryPolicy{Retain: 2},
	}
}

func TestWriteModeFor(t *testing.T) {
	if m, _ := writeModeFor(updaterv2.Placement_PLACEMENT_IN_PLACE); m != writeReconcile {
		t.Fatalf("IN_PLACE = %v", m)
	}
	if m, _ := writeModeFor(updaterv2.Placement_PLACEMENT_LIBRARY); m != writeFresh {
		t.Fatalf("LIBRARY = %v", m)
	}
	if _, err := writeModeFor(updaterv2.Placement_PLACEMENT_UNSPECIFIED); err == nil {
		t.Fatal("unspecified placement accepted")
	}
}

func TestLibraryNeverRequiresHostExit(t *testing.T) {
	stage := t.TempDir()
	plan := mustPayloadPlan(t, stage, "1.0.0+1", 1, map[string][]byte{
		"bin/app.exe":        []byte("a"),
		"relkit-updater.exe": []byte("e"),
	})
	inst := &updaterv2.InstallSpec{
		Placement:         updaterv2.Placement_PLACEMENT_LIBRARY,
		ExecutableRelpath: "bin/app.exe",
		SidecarRelpath:    "relkit-updater.exe",
	}
	if exit, err := planRequiresHostExit(inst, plan); err != nil || exit {
		t.Fatalf("library exit=%v err=%v", exit, err)
	}
	inst.Placement = updaterv2.Placement_PLACEMENT_IN_PLACE
	if exit, err := planRequiresHostExit(inst, plan); err != nil || !exit {
		t.Fatalf("in-place exit=%v err=%v", exit, err)
	}
}

func TestLibraryFreshWriteLeavesNoPartialOrBackup(t *testing.T) {
	root, stage := t.TempDir(), t.TempDir()
	stubRunning(t)
	plan := mustPayloadPlan(t, stage, "1.0.0+1", 1, map[string][]byte{"bin/app.exe": []byte("a")})
	if err := applyPlan(t.TempDir(), librarySession(root, stage), plan); err != nil {
		t.Fatal(err)
	}
	ents, _ := os.ReadDir(filepath.Join(root, VersionsDirName))
	if len(ents) != 1 || ents[0].Name() != "1.0.0+1" {
		t.Fatalf("versions/ = %v", ents)
	}
	if _, err := os.Stat(filepath.Join(root, VersionsDirName, "1.0.0+1", ".relkit-backup")); !os.IsNotExist(err) {
		t.Fatal("fresh write left a backup directory")
	}
	meta := readVersionMeta(filepath.Join(root, VersionsDirName, "1.0.0+1"))
	if meta == nil || meta.Code != 1 || meta.Executable != "versions/1.0.0+1/bin/app.exe" {
		t.Fatalf("meta %+v", meta)
	}
}

func TestLibrarySameCodeReinstallIsIdempotent(t *testing.T) {
	root, stage := t.TempDir(), t.TempDir()
	stubRunning(t)
	sess := librarySession(root, stage)
	plan := mustPayloadPlan(t, stage, "1.0.0+1", 1, map[string][]byte{"bin/app.exe": []byte("a")})
	if err := applyPlan(t.TempDir(), sess, plan); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(root, VersionsDirName, "1.0.0+1", "bin", "runtime-marker")
	if err := os.WriteFile(marker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := applyPlan(t.TempDir(), sess, plan); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatal("same-code reinstall rewrote the installed version")
	}
	if ptr := readActive(root); ptr == nil || ptr.Code != 1 || ptr.Previous != "" {
		t.Fatalf("active %+v", ptr)
	}
}

func TestLibraryForeignTargetIsReplacedWhenIdle(t *testing.T) {
	root, stage := t.TempDir(), t.TempDir()
	stubRunning(t)
	foreign := filepath.Join(root, VersionsDirName, "1.0.0+1")
	_ = os.MkdirAll(foreign, 0o755)
	_ = os.WriteFile(filepath.Join(foreign, "junk"), []byte("j"), 0o644)
	plan := mustPayloadPlan(t, stage, "1.0.0+1", 1, map[string][]byte{"bin/app.exe": []byte("a")})
	if err := applyPlan(t.TempDir(), librarySession(root, stage), plan); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(foreign, "junk")); !os.IsNotExist(err) {
		t.Fatal("foreign content survived")
	}
	if got, _ := os.ReadFile(filepath.Join(foreign, "bin", "app.exe")); string(got) != "a" {
		t.Fatalf("app.exe = %q", got)
	}
}

func TestLibraryForeignTargetInUseIsOccupied(t *testing.T) {
	root, stage := t.TempDir(), t.TempDir()
	foreign := filepath.Join(root, VersionsDirName, "1.0.0+1")
	_ = os.MkdirAll(filepath.Join(foreign, "bin"), 0o755)
	_ = os.WriteFile(filepath.Join(foreign, "bin", "app.exe"), []byte("old"), 0o755)
	stubRunning(t, filepath.Join(foreign, "bin", "app.exe"))
	plan := mustPayloadPlan(t, stage, "1.0.0+1", 1, map[string][]byte{"bin/app.exe": []byte("a")})
	err := applyPlan(t.TempDir(), librarySession(root, stage), plan)
	if applyErrorCode(err) != updaterv2.ErrorCode_ERROR_CODE_OCCUPIED {
		t.Fatalf("err = %v", err)
	}
	if got, _ := os.ReadFile(filepath.Join(foreign, "bin", "app.exe")); string(got) != "old" {
		t.Fatal("running version was touched")
	}
}

func TestLibraryPostScriptFailureLeavesNothing(t *testing.T) {
	root, stage := t.TempDir(), t.TempDir()
	content := &applyContent{
		table: &enginestate.FileTable{Schema: payload.Schema, Scripts: []*enginestate.ScriptEntry{{
			Phase:       enginestate.ScriptPhase_SCRIPT_PHASE_POST_APPLY,
			Relpath:     "missing.sh",
			Interpreter: enginestate.ScriptInterpreter_SCRIPT_INTERPRETER_UNSPECIFIED,
		}}},
		filesRoot:   t.TempDir(),
		scriptsRoot: t.TempDir(),
	}
	err := applyLibrary(librarySession(root, stage), &enginestate.UpdatePlan{Version: "1.0.0+1", Code: 1}, content)
	if err == nil {
		t.Fatal("post script failure ignored")
	}
	ents, _ := os.ReadDir(filepath.Join(root, VersionsDirName))
	if len(ents) != 0 {
		t.Fatalf("versions/ = %v", ents)
	}
	if readActive(root) != nil {
		t.Fatal("active written after failure")
	}
}

func writeVersion(t *testing.T, root, name string, code int64) string {
	t.Helper()
	dir := filepath.Join(root, VersionsDirName, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeVersionMeta(dir, versionMeta{Code: code, Path: "versions/" + name}); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestPruneOrdersByCodeNotName(t *testing.T) {
	root := t.TempDir()
	stubRunning(t)
	writeVersion(t, root, "0.2.0+99", 99)
	writeVersion(t, root, "0.2.0+221", 221)
	writeVersion(t, root, "0.2.0+222", 222)
	pruneVersions(root, activePointer{Code: 222, Path: "versions/0.2.0+222"}, 2, nil, "versions/0.2.0+222")
	if _, err := os.Stat(filepath.Join(root, VersionsDirName, "0.2.0+221")); err != nil {
		t.Fatal("newer code 221 pruned in favour of 99")
	}
	if _, err := os.Stat(filepath.Join(root, VersionsDirName, "0.2.0+99")); !os.IsNotExist(err) {
		t.Fatal("oldest code 99 kept")
	}
}

func TestPruneSkipsVersionsInUse(t *testing.T) {
	root := t.TempDir()
	old := writeVersion(t, root, "0.1.0+1", 1)
	writeVersion(t, root, "0.1.1+2", 2)
	writeVersion(t, root, "0.1.2+3", 3)
	stubRunning(t, filepath.Join(old, "bin", "app.exe"))
	pruneVersions(root, activePointer{Code: 3, Path: "versions/0.1.2+3", Previous: "versions/0.1.1+2"}, 2, nil, "versions/0.1.2+3")
	if _, err := os.Stat(old); err != nil {
		t.Fatal("running version pruned")
	}
}

func TestPruneRemovesPartialsAndListSkipsThem(t *testing.T) {
	root := t.TempDir()
	stubRunning(t)
	writeVersion(t, root, "0.1.0+1", 1)
	partial := partialPathFor(filepath.Join(root, VersionsDirName, "0.1.1+2"))
	_ = os.MkdirAll(partial, 0o755)
	if got := listInstalled(root).GetVersions(); len(got) != 1 {
		t.Fatalf("listed %v", got)
	}
	pruneVersions(root, activePointer{Code: 1, Path: "versions/0.1.0+1"}, 2, nil, "")
	if _, err := os.Stat(partial); !os.IsNotExist(err) {
		t.Fatal("stale partial kept")
	}
}

func TestReconcileLeavesUnrelatedEmptyDirectories(t *testing.T) {
	root, dataDir, stage := t.TempDir(), t.TempDir(), t.TempDir()
	// A managed product's version with an intentionally empty directory.
	managed := filepath.Join(root, VersionsDirName, "7", "plugins", "empty")
	_ = os.MkdirAll(managed, 0o755)
	_ = os.MkdirAll(filepath.Join(root, "lib", "old"), 0o755)
	_ = os.WriteFile(filepath.Join(root, "lib", "old", "gone.dll"), []byte("old"), 0o644)
	if err := saveBaseline(baselinePathFor(dataDir, "launcher"), &enginestate.Baseline{
		Schema: baselineSchema,
		Files:  []*enginestate.BaselineEntry{{Relpath: "lib/old/gone.dll", Sha256: model.Sha256Bytes([]byte("old"))}},
	}); err != nil {
		t.Fatal(err)
	}
	sess := &enginestate.ApplySessionRecord{InstallRoot: root, StagedRoot: stage, Product: "launcher"}
	content := &applyContent{table: &enginestate.FileTable{Schema: payload.Schema}, filesRoot: t.TempDir()}
	if err := applyReconcile(dataDir, sess, &enginestate.UpdatePlan{Code: 2}, content, root); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(managed); err != nil {
		t.Fatal("managed product's empty directory removed")
	}
	if _, err := os.Stat(filepath.Join(root, "lib")); !os.IsNotExist(err) {
		t.Fatal("emptied ancestors of a deleted file kept")
	}
	if _, err := os.Stat(filepath.Join(root, ".relkit-backup")); !os.IsNotExist(err) {
		t.Fatal("backup root kept")
	}
}

func TestBaselineIsPerProductAndMigratesLegacy(t *testing.T) {
	root, dataDir, stage := t.TempDir(), t.TempDir(), t.TempDir()
	legacy := filepath.Join(dataDir, legacyBaselineFileName)
	_ = os.WriteFile(filepath.Join(root, "gone.txt"), []byte("old"), 0o644)
	if err := saveBaseline(legacy, &enginestate.Baseline{
		Schema: baselineSchema,
		Files:  []*enginestate.BaselineEntry{{Relpath: "gone.txt", Sha256: model.Sha256Bytes([]byte("old"))}},
	}); err != nil {
		t.Fatal(err)
	}
	sess := &enginestate.ApplySessionRecord{InstallRoot: root, StagedRoot: stage, Product: "loom-launcher"}
	content := &applyContent{table: &enginestate.FileTable{Schema: payload.Schema}, filesRoot: t.TempDir()}
	if err := applyReconcile(dataDir, sess, &enginestate.UpdatePlan{Code: 2}, content, root); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "gone.txt")); !os.IsNotExist(err) {
		t.Fatal("legacy baseline not honoured")
	}
	if _, err := os.Stat(legacy); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("legacy baseline not migrated away")
	}
	if b := loadBaseline(filepath.Join(dataDir, "baseline-loom-launcher.pb")); b == nil || b.Code != 2 {
		t.Fatalf("product baseline %+v", b)
	}
	if got := baselinePathFor(dataDir, "../x y"); filepath.Dir(got) != dataDir {
		t.Fatalf("product name escaped data dir: %s", got)
	}
}

func TestFirstComponentUnder(t *testing.T) {
	base := filepath.Join(string(filepath.Separator)+"r", "versions")
	cases := map[string]string{
		filepath.Join(base, "1.0.0+1", "bin", "app"): "1.0.0+1",
		filepath.Join(base, "x"):                     "x",
	}
	for p, want := range cases {
		if got, ok := firstComponentUnder(base, p); !ok || got != want {
			t.Errorf("%s -> %q %v", p, got, ok)
		}
	}
	for _, p := range []string{base, filepath.Dir(base), filepath.Join(filepath.Dir(base), "versions2", "a")} {
		if got, ok := firstComponentUnder(base, p); ok {
			t.Errorf("%s -> %q", p, got)
		}
	}
}

func TestRunningExecutablesSeesThisProcess(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Skip(err)
	}
	want, _ := filepath.EvalSymlinks(self)
	all := runningExecutables()
	if all == nil {
		t.Skip("process enumeration unsupported on this platform")
	}
	for _, exe := range all {
		if got, err := filepath.EvalSymlinks(exe); err == nil && got == want {
			return
		}
	}
	t.Fatalf("%s not among %d running executables", want, len(all))
}
