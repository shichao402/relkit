package updater

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	enginestate "github.com/shichao402/relkit/api/updater/enginestate/v1"
	updaterv2 "github.com/shichao402/relkit/api/updater/v2"
	"github.com/shichao402/relkit/internal/payload"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ADR 0027 step 3: BUNDLE content (fresh write, rename and pointer switch),
// symlinks in both write paths, control files outside the bundle.

const testPlist = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict><key>CFBundleExecutable</key><string>%s</string><key>CFBundleIdentifier</key><string>%s</string><key>CFBundlePackageType</key><string>%s</string></dict></plist>
`

func asDarwin(t *testing.T) {
	t.Helper()
	prev := hostGOOS
	hostGOOS = "darwin"
	t.Cleanup(func() { hostGOOS = prev })
}

// stubSignature replaces codesign; fail makes it reject every bundle.
func stubSignature(t *testing.T, fail bool) *[]string {
	t.Helper()
	var seen []string
	prev := verifyBundleSignature
	verifyBundleSignature = func(bundle string) error {
		seen = append(seen, bundle)
		if fail {
			return failWith(updaterv2.ErrorCode_ERROR_CODE_SIGNATURE, "stub rejects %s", bundle)
		}
		return nil
	}
	t.Cleanup(func() { verifyBundleSignature = prev })
	return &seen
}

func writeTestFile(t *testing.T, path string, body []byte, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, mode); err != nil {
		t.Fatal(err)
	}
}

func symlinkOrFatal(t *testing.T, target, link string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
}

// makeBundle builds dir/Demo.app with a versioned framework, the structure
// whose symlinks codesign requires. exe is the Mach-O to use for both
// binaries, or nil for plain bytes (signature stubbed).
func makeBundle(t *testing.T, dir, marker string, exe []byte) string {
	t.Helper()
	app := filepath.Join(dir, "Demo.app")
	if exe == nil {
		exe = []byte("demo-" + marker)
	}
	writeTestFile(t, filepath.Join(app, "Contents", "Info.plist"),
		[]byte(fmt.Sprintf(testPlist, "Demo", "com.example.demo", "APPL")), 0o644)
	writeTestFile(t, filepath.Join(app, "Contents", "MacOS", "Demo"), exe, 0o755)
	writeTestFile(t, filepath.Join(app, "Contents", "Resources", "marker.txt"), []byte(marker), 0o644)
	fw := filepath.Join(app, "Contents", "Frameworks", "F.framework")
	writeTestFile(t, filepath.Join(fw, "Versions", "A", "F"), exe, 0o755)
	writeTestFile(t, filepath.Join(fw, "Versions", "A", "Resources", "Info.plist"),
		[]byte(fmt.Sprintf(testPlist, "F", "com.example.f", "FMWK")), 0o644)
	symlinkOrFatal(t, "A", filepath.Join(fw, "Versions", "Current"))
	symlinkOrFatal(t, "Versions/Current/F", filepath.Join(fw, "F"))
	symlinkOrFatal(t, "Versions/Current/Resources", filepath.Join(fw, "Resources"))
	return app
}

func bundlePlan(t *testing.T, stage, version string, code int64, app string) *enginestate.UpdatePlan {
	t.Helper()
	zipPath := filepath.Join(stage, version+"-bundle.zip")
	table, err := payload.Build(zipPath, payload.BuildOptions{Tree: app, Bundle: true})
	if err != nil {
		t.Fatal(err)
	}
	if table.Schema != payload.SchemaV2 || table.Unit != enginestate.ContentUnit_CONTENT_UNIT_BUNDLE {
		t.Fatalf("table %s %s", table.Schema, table.Unit)
	}
	info, err := os.Stat(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	return &enginestate.UpdatePlan{Version: version, Code: code, Files: []*enginestate.PlannedFile{{
		Name: filepath.Base(zipPath), Kind: "payload", Size: info.Size(), LocalPath: zipPath,
	}}}
}

func swapSession(root, stage string) *enginestate.ApplySessionRecord {
	return &enginestate.ApplySessionRecord{
		InstallRoot:       root,
		StagedRoot:        stage,
		Placement:         updaterv2.Placement_PLACEMENT_IN_PLACE,
		Unit:              enginestate.ContentUnit_CONTENT_UNIT_BUNDLE,
		ExecutableRelpath: "Contents/MacOS/Demo",
		Product:           "demo",
	}
}

func assertBundle(t *testing.T, app, marker string) {
	t.Helper()
	if got, err := os.ReadFile(filepath.Join(app, "Contents", "Resources", "marker.txt")); err != nil || string(got) != marker {
		t.Fatalf("marker = %q %v", got, err)
	}
	fw := filepath.Join(app, "Contents", "Frameworks", "F.framework")
	for link, want := range map[string]string{
		"Versions/Current": "A",
		"F":                "Versions/Current/F",
		"Resources":        "Versions/Current/Resources",
	} {
		if got, err := os.Readlink(filepath.Join(fw, filepath.FromSlash(link))); err != nil || got != want {
			t.Fatalf("%s -> %q %v", link, got, err)
		}
	}
	if info, err := os.Stat(filepath.Join(app, "Contents", "MacOS", "Demo")); err != nil || info.Mode().Perm()&0o100 == 0 {
		t.Fatalf("executable mode %v %v", info, err)
	}
}

func assertNoSiblings(t *testing.T, target string) {
	t.Helper()
	for _, p := range []string{partialPathFor(target), oldPathFor(target)} {
		if _, err := os.Lstat(p); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s left behind", filepath.Base(p))
		}
	}
}

func TestBundleControlFilesLiveBesideTheBundle(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Demo.app")
	bundle := enginestate.ContentUnit_CONTENT_UNIT_BUNDLE
	inPlace := updaterv2.Placement_PLACEMENT_IN_PLACE
	if got := controlDirFor(root, inPlace, bundle); got != filepath.Dir(root) {
		t.Fatalf("control dir %s", got)
	}
	if got := applyLockPathFor(root, inPlace, bundle); got != filepath.Join(filepath.Dir(root), ".Demo.app.relkit-apply.lock") {
		t.Fatalf("lock %s", got)
	}
	for _, c := range []struct {
		unit      enginestate.ContentUnit
		placement updaterv2.Placement
	}{{enginestate.ContentUnit_CONTENT_UNIT_TREE, inPlace}, {bundle, updaterv2.Placement_PLACEMENT_LIBRARY}} {
		if got := applyLockPathFor(root, c.placement, c.unit); got != applyLockPath(root) {
			t.Fatalf("%s/%s lock %s", c.unit, c.placement, got)
		}
		if got := controlDirFor(root, c.placement, c.unit); got != filepath.Clean(root) {
			t.Fatalf("%s/%s control dir %s", c.unit, c.placement, got)
		}
	}
	sess := &enginestate.ApplySessionRecord{InstallRoot: root, Placement: inPlace, Unit: bundle}
	if sessionLockPath(sess) != applyLockPathFor(root, inPlace, bundle) {
		t.Fatal("session lock path disagrees")
	}
}

func TestDetectInPlaceUnit(t *testing.T) {
	dir := t.TempDir()
	app := makeBundle(t, dir, "1", nil)
	if u, err := detectInPlaceUnit(app); err != nil || u != enginestate.ContentUnit_CONTENT_UNIT_BUNDLE {
		t.Fatalf("bundle detected as %s %v", u, err)
	}
	if u, err := detectInPlaceUnit(filepath.Join(dir, "missing")); err != nil || u != enginestate.ContentUnit_CONTENT_UNIT_TREE {
		t.Fatalf("missing root = %s %v", u, err)
	}
	link := filepath.Join(dir, "Link.app")
	symlinkOrFatal(t, app, link)
	if _, err := detectInPlaceUnit(link); applyErrorCode(err) != updaterv2.ErrorCode_ERROR_CODE_LAYOUT_UNSUPPORTED {
		t.Fatalf("symlinked bundle root: %v", err)
	}
}

func TestPlatformRules(t *testing.T) {
	bundle := enginestate.ContentUnit_CONTENT_UNIT_BUNDLE
	tree := enginestate.ContentUnit_CONTENT_UNIT_TREE
	layout := updaterv2.ErrorCode_ERROR_CODE_LAYOUT_UNSUPPORTED
	if err := checkUnitSupported(bundle, true, "darwin"); err != nil {
		t.Fatal(err)
	}
	if err := checkUnitSupported(tree, true, "linux"); err != nil {
		t.Fatal(err)
	}
	if applyErrorCode(checkUnitSupported(bundle, false, "linux")) != layout {
		t.Fatal("bundle accepted off macOS")
	}
	if applyErrorCode(checkUnitSupported(tree, true, "windows")) != layout {
		t.Fatal("symlinks accepted on windows")
	}

	dir := t.TempDir()
	if err := checkLibraryRoot(filepath.Join(dir, "lib"), filepath.Join(dir, "data"), "darwin"); err != nil {
		t.Fatal(err)
	}
	if applyErrorCode(checkLibraryRoot(filepath.Join(dir, "Launcher.app", "Contents", "lib"), "", "darwin")) != layout {
		t.Fatal("library inside a bundle accepted")
	}
	if applyErrorCode(checkLibraryRoot(filepath.Join(dir, "data"), filepath.Join(dir, "data"), "darwin")) != layout {
		t.Fatal("library equal to data dir accepted")
	}
	if err := checkLibraryRoot(filepath.Join(dir, "X.app", "lib"), "", "windows"); err != nil {
		t.Fatal("darwin rule applied on windows")
	}
}

func TestBundleRequiresHostExitAndMatchingUnit(t *testing.T) {
	asDarwin(t)
	stage := t.TempDir()
	plan := bundlePlan(t, stage, "2", 2, makeBundle(t, t.TempDir(), "2", nil))
	inst := &updaterv2.InstallSpec{Placement: updaterv2.Placement_PLACEMENT_IN_PLACE, ExecutableRelpath: "Contents/MacOS/Demo"}
	if exit, err := planRequiresHostExit(inst, plan, enginestate.ContentUnit_CONTENT_UNIT_BUNDLE); err != nil || !exit {
		t.Fatalf("swap exit=%v err=%v", exit, err)
	}
	if _, err := planRequiresHostExit(inst, plan, enginestate.ContentUnit_CONTENT_UNIT_TREE); applyErrorCode(err) != updaterv2.ErrorCode_ERROR_CODE_LAYOUT_UNSUPPORTED {
		t.Fatalf("bundle onto a tree: %v", err)
	}
	inst.Placement = updaterv2.Placement_PLACEMENT_LIBRARY
	if exit, err := planRequiresHostExit(inst, plan, enginestate.ContentUnit_CONTENT_UNIT_TREE); err != nil || exit {
		t.Fatalf("library exit=%v err=%v", exit, err)
	}
	hostGOOS = "linux"
	if _, err := planRequiresHostExit(inst, plan, enginestate.ContentUnit_CONTENT_UNIT_TREE); applyErrorCode(err) != updaterv2.ErrorCode_ERROR_CODE_LAYOUT_UNSUPPORTED {
		t.Fatalf("bundle off macOS: %v", err)
	}
}

func TestBundleSwapReplacesTheBundle(t *testing.T) {
	for _, fallback := range []bool{false, true} {
		name := "native"
		if fallback {
			name = "fallback"
		}
		t.Run(name, func(t *testing.T) {
			asDarwin(t)
			seen := stubSignature(t, false)
			if fallback {
				prev := swapDirs
				swapDirs = func(string, string) error { return errSwapUnsupported }
				t.Cleanup(func() { swapDirs = prev })
			}
			dir, dataDir, stage := t.TempDir(), t.TempDir(), t.TempDir()
			root := makeBundle(t, dir, "1", nil)
			baseline := baselinePathFor(dataDir, "demo")
			if err := saveBaseline(baseline, &enginestate.Baseline{Schema: baselineSchema}); err != nil {
				t.Fatal(err)
			}
			plan := bundlePlan(t, stage, "2", 2, makeBundle(t, t.TempDir(), "2", nil))
			sess := swapSession(root, stage)
			if err := applyPlan(dataDir, sess, plan); err != nil {
				t.Fatal(err)
			}
			assertBundle(t, root, "2")
			assertNoSiblings(t, root)
			if len(*seen) != 1 || (*seen)[0] != partialPathFor(root) {
				t.Fatalf("signature checked on %v", *seen)
			}
			if _, err := os.Stat(baseline); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("stale baseline kept")
			}
			if sess.BundleName != "Demo.app" || sess.Unit != enginestate.ContentUnit_CONTENT_UNIT_BUNDLE {
				t.Fatalf("session unit %s %q", sess.Unit, sess.BundleName)
			}
			ents, _ := os.ReadDir(dir)
			if len(ents) != 1 {
				t.Fatalf("beside the bundle: %v", ents)
			}
		})
	}
}

func TestBundleSwapFailuresLeaveTheOldBundle(t *testing.T) {
	asDarwin(t)
	t.Run("signature", func(t *testing.T) {
		stubSignature(t, true)
		dir, stage := t.TempDir(), t.TempDir()
		root := makeBundle(t, dir, "1", nil)
		plan := bundlePlan(t, stage, "2", 2, makeBundle(t, t.TempDir(), "2", nil))
		err := applyPlan(t.TempDir(), swapSession(root, stage), plan)
		if applyErrorCode(err) != updaterv2.ErrorCode_ERROR_CODE_SIGNATURE {
			t.Fatalf("err = %v", err)
		}
		assertBundle(t, root, "1")
		assertNoSiblings(t, root)
	})
	t.Run("preserve", func(t *testing.T) {
		stubSignature(t, false)
		dir, stage := t.TempDir(), t.TempDir()
		root := makeBundle(t, dir, "1", nil)
		plan := bundlePlan(t, stage, "2", 2, makeBundle(t, t.TempDir(), "2", nil))
		sess := swapSession(root, stage)
		sess.Preserve = []string{"Contents/Resources/user.cfg"}
		if err := applyPlan(t.TempDir(), sess, plan); applyErrorCode(err) != updaterv2.ErrorCode_ERROR_CODE_LAYOUT_UNSUPPORTED {
			t.Fatalf("err = %v", err)
		}
		assertBundle(t, root, "1")
	})
	t.Run("tree payload onto a bundle", func(t *testing.T) {
		stubSignature(t, false)
		dir, stage := t.TempDir(), t.TempDir()
		root := makeBundle(t, dir, "1", nil)
		plan := mustPayloadPlan(t, stage, "2", 2, map[string][]byte{"Contents/MacOS/Demo": []byte("tree")})
		if err := applyPlan(t.TempDir(), swapSession(root, stage), plan); applyErrorCode(err) != updaterv2.ErrorCode_ERROR_CODE_LAYOUT_UNSUPPORTED {
			t.Fatalf("err = %v", err)
		}
		assertBundle(t, root, "1")
	})
}

func TestRecoverSwap(t *testing.T) {
	mk := func(t *testing.T, p, marker string) {
		writeTestFile(t, filepath.Join(p, "m"), []byte(marker), 0o644)
	}
	read := func(p string) string {
		b, _ := os.ReadFile(filepath.Join(p, "m"))
		return string(b)
	}
	t.Run("between the fallback renames", func(t *testing.T) {
		target := filepath.Join(t.TempDir(), "Demo.app")
		mk(t, oldPathFor(target), "old")
		mk(t, partialPathFor(target), "new")
		if err := recoverSwap(target); err != nil {
			t.Fatal(err)
		}
		if read(target) != "old" {
			t.Fatal("old bundle not restored")
		}
		assertNoSiblings(t, target)
	})
	t.Run("after the switch", func(t *testing.T) {
		target := filepath.Join(t.TempDir(), "Demo.app")
		mk(t, target, "new")
		mk(t, oldPathFor(target), "old")
		mk(t, partialPathFor(target), "old-swapped")
		if err := recoverSwap(target); err != nil {
			t.Fatal(err)
		}
		if read(target) != "new" {
			t.Fatal("installed bundle touched")
		}
		assertNoSiblings(t, target)
	})
}

func TestBundleLibraryInstallsIntoVersion(t *testing.T) {
	asDarwin(t)
	seen := stubSignature(t, false)
	stubRunning(t)
	root, stage := t.TempDir(), t.TempDir()
	plan := bundlePlan(t, stage, "2.0.0+2", 2, makeBundle(t, t.TempDir(), "2", nil))
	sess := &enginestate.ApplySessionRecord{
		InstallRoot:       root,
		StagedRoot:        stage,
		Placement:         updaterv2.Placement_PLACEMENT_LIBRARY,
		ExecutableRelpath: "Contents/MacOS/Demo",
		Relaunch:          true,
		Library:           &updaterv2.LibraryPolicy{Retain: 2},
	}
	if err := applyPlan(t.TempDir(), sess, plan); err != nil {
		t.Fatal(err)
	}
	version := filepath.Join(root, VersionsDirName, "2.0.0+2")
	assertBundle(t, filepath.Join(version, "Demo.app"), "2")
	if len(*seen) != 1 || (*seen)[0] != filepath.Join(partialPathFor(version), "Demo.app") {
		t.Fatalf("signature checked on %v", *seen)
	}
	wantExe := "versions/2.0.0+2/Demo.app/Contents/MacOS/Demo"
	meta := readVersionMeta(version)
	if meta == nil || meta.Bundle != "Demo.app" || meta.Executable != wantExe {
		t.Fatalf("meta %+v", meta)
	}
	ptr := readActive(root)
	if ptr == nil || ptr.Bundle != "Demo.app" || ptr.Executable != wantExe {
		t.Fatalf("active %+v", ptr)
	}
	exe, bundle := relaunchTarget(sess)
	if bundle != filepath.Join(version, "Demo.app") || exe != filepath.Join(root, filepath.FromSlash(wantExe)) {
		t.Fatalf("relaunch %s %s", exe, bundle)
	}
	// Switching away and back keeps the bundle in the pointer.
	writeVersion(t, root, "1.0.0+1", 1)
	if err := switchActive(root, 1); err != nil {
		t.Fatal(err)
	}
	if err := rollbackActive(root); err != nil {
		t.Fatal(err)
	}
	if ptr := readActive(root); ptr == nil || ptr.Code != 2 || ptr.Bundle != "Demo.app" {
		t.Fatalf("after rollback %+v", ptr)
	}
}

func TestRelaunchTargetInPlace(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Demo.app")
	sess := swapSession(root, "")
	exe, bundle := relaunchTarget(sess)
	if bundle != root || exe != filepath.Join(root, "Contents", "MacOS", "Demo") {
		t.Fatalf("bundle %s exe %s", bundle, exe)
	}
	sess.Unit = enginestate.ContentUnit_CONTENT_UNIT_TREE
	if _, bundle := relaunchTarget(sess); bundle != "" {
		t.Fatal("tree relaunched as a bundle")
	}
}

func TestBundleLibraryRootInsideBundleIsRefused(t *testing.T) {
	asDarwin(t)
	if applyErrorCode(checkLibraryRoot(filepath.Join(t.TempDir(), "Launcher.app", "Contents", "Library"), "", hostGOOS)) !=
		updaterv2.ErrorCode_ERROR_CODE_LAYOUT_UNSUPPORTED {
		t.Fatal("accepted")
	}
}

// symlinkTreePlan builds a TREE payload with a shared-library style link.
func symlinkTreePlan(t *testing.T, stage, version string, code int64, build func(tree string)) *enginestate.UpdatePlan {
	t.Helper()
	tree := filepath.Join(stage, "tree-"+version)
	if err := os.MkdirAll(tree, 0o755); err != nil {
		t.Fatal(err)
	}
	build(tree)
	zipPath := filepath.Join(stage, version+"-payload.zip")
	if _, err := payload.Build(zipPath, payload.BuildOptions{Tree: tree}); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(zipPath)
	return &enginestate.UpdatePlan{Version: version, Code: code, Files: []*enginestate.PlannedFile{{
		Name: filepath.Base(zipPath), Kind: "payload", Size: info.Size(), LocalPath: zipPath,
	}}}
}

func inPlaceTreeSession(root, stage string) *enginestate.ApplySessionRecord {
	return &enginestate.ApplySessionRecord{
		InstallRoot: root,
		StagedRoot:  stage,
		Placement:   updaterv2.Placement_PLACEMENT_IN_PLACE,
		Unit:        enginestate.ContentUnit_CONTENT_UNIT_TREE,
		Product:     "tool",
	}
}

func TestReconcileUpdatesSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks are refused on windows")
	}
	root, dataDir := t.TempDir(), t.TempDir()
	v1 := symlinkTreePlan(t, t.TempDir(), "1", 1, func(tree string) {
		writeTestFile(t, filepath.Join(tree, "lib", "libfoo.so.1"), []byte("one"), 0o644)
		symlinkOrFatal(t, "libfoo.so.1", filepath.Join(tree, "lib", "libfoo.so"))
	})
	if err := applyPlan(dataDir, inPlaceTreeSession(root, t.TempDir()), v1); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.Readlink(filepath.Join(root, "lib", "libfoo.so")); got != "libfoo.so.1" {
		t.Fatalf("v1 link %q", got)
	}
	v2 := symlinkTreePlan(t, t.TempDir(), "2", 2, func(tree string) {
		writeTestFile(t, filepath.Join(tree, "lib", "libfoo.so.2"), []byte("two"), 0o644)
		symlinkOrFatal(t, "libfoo.so.2", filepath.Join(tree, "lib", "libfoo.so"))
	})
	if err := applyPlan(dataDir, inPlaceTreeSession(root, t.TempDir()), v2); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.Readlink(filepath.Join(root, "lib", "libfoo.so")); got != "libfoo.so.2" {
		t.Fatalf("v2 link %q", got)
	}
	if _, err := os.Lstat(filepath.Join(root, "lib", "libfoo.so.1")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("dropped file kept")
	}
	b := loadBaseline(baselinePathFor(dataDir, "tool"))
	found := false
	for _, e := range b.GetFiles() {
		if e.Relpath == "lib/libfoo.so" {
			found = e.Kind == enginestate.EntryKind_ENTRY_KIND_SYMLINK && e.LinkTarget == "libfoo.so.2"
		}
	}
	if !found {
		t.Fatalf("baseline %v", b.GetFiles())
	}
}

func TestReconcileReplacesOwnedSymlinkParent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks are refused on windows")
	}
	root, dataDir := t.TempDir(), t.TempDir()
	v1 := symlinkTreePlan(t, t.TempDir(), "1", 1, func(tree string) {
		writeTestFile(t, filepath.Join(tree, "plugins.real", "a"), []byte("a1"), 0o644)
		symlinkOrFatal(t, "plugins.real", filepath.Join(tree, "plugins"))
	})
	if err := applyPlan(dataDir, inPlaceTreeSession(root, t.TempDir()), v1); err != nil {
		t.Fatal(err)
	}
	v2 := symlinkTreePlan(t, t.TempDir(), "2", 2, func(tree string) {
		writeTestFile(t, filepath.Join(tree, "plugins", "a"), []byte("a2"), 0o644)
	})
	if err := applyPlan(dataDir, inPlaceTreeSession(root, t.TempDir()), v2); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(filepath.Join(root, "plugins"))
	if err != nil || !info.IsDir() {
		t.Fatalf("plugins is not a real directory: %v %v", info, err)
	}
	if got, _ := os.ReadFile(filepath.Join(root, "plugins", "a")); string(got) != "a2" {
		t.Fatalf("plugins/a = %q", got)
	}
	if _, err := os.Lstat(filepath.Join(root, "plugins.real")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("dropped directory kept")
	}
}

func TestReconcileNeverWritesThroughForeignSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks are refused on windows")
	}
	root, dataDir, outside := t.TempDir(), t.TempDir(), t.TempDir()
	writeTestFile(t, filepath.Join(root, "keep.txt"), []byte("old"), 0o644)
	symlinkOrFatal(t, outside, filepath.Join(root, "data"))
	plan := mustPayloadPlan(t, t.TempDir(), "2", 2, map[string][]byte{
		"keep.txt": []byte("new"),
		"data/x":   []byte("x"),
	})
	err := applyPlan(dataDir, inPlaceTreeSession(root, t.TempDir()), plan)
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("err = %v", err)
	}
	if ents, _ := os.ReadDir(outside); len(ents) != 0 {
		t.Fatalf("wrote outside the root: %v", ents)
	}
	if got, _ := os.ReadFile(filepath.Join(root, "keep.txt")); string(got) != "old" {
		t.Fatal("failed apply not rolled back")
	}
}

func TestReconcileLeavesModifiedSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks are refused on windows")
	}
	root, dataDir := t.TempDir(), t.TempDir()
	symlinkOrFatal(t, "elsewhere", filepath.Join(root, "link"))
	if err := saveBaseline(baselinePathFor(dataDir, "tool"), &enginestate.Baseline{
		Schema: baselineSchema,
		Files: []*enginestate.BaselineEntry{{
			Relpath: "link", Kind: enginestate.EntryKind_ENTRY_KIND_SYMLINK, LinkTarget: "original",
		}},
	}); err != nil {
		t.Fatal(err)
	}
	plan := mustPayloadPlan(t, t.TempDir(), "2", 2, map[string][]byte{"a": []byte("a")})
	if err := applyPlan(dataDir, inPlaceTreeSession(root, t.TempDir()), plan); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.Readlink(filepath.Join(root, "link")); got != "elsewhere" {
		t.Fatal("user-changed symlink removed")
	}
}

// TestBundleSwapWithCodesign runs the real signature check on an ad-hoc
// signed bundle with framework symlinks, then a tampered one.
func TestBundleSwapWithCodesign(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("codesign is macOS only")
	}
	if _, err := exec.LookPath("codesign"); err != nil {
		t.Skip("codesign not available")
	}
	exe, err := os.ReadFile("/usr/bin/true")
	if err != nil {
		t.Skip(err)
	}
	sign := func(app string) {
		if out, err := exec.Command("codesign", "--force", "--deep", "--sign", "-", app).CombinedOutput(); err != nil {
			t.Fatalf("sign: %v %s", err, out)
		}
	}
	dir := t.TempDir()
	root := makeBundle(t, dir, "1", exe)
	sign(root)

	stage := t.TempDir()
	next := makeBundle(t, t.TempDir(), "2", exe)
	sign(next)
	if err := applyPlan(t.TempDir(), swapSession(root, stage), bundlePlan(t, stage, "2", 2, next)); err != nil {
		t.Fatal(err)
	}
	assertBundle(t, root, "2")
	assertNoSiblings(t, root)

	stage = t.TempDir()
	tampered := makeBundle(t, t.TempDir(), "3", exe)
	sign(tampered)
	writeTestFile(t, filepath.Join(tampered, "Contents", "Resources", "marker.txt"), []byte("3-tampered"), 0o644)
	err = applyPlan(t.TempDir(), swapSession(root, stage), bundlePlan(t, stage, "3", 3, tampered))
	if applyErrorCode(err) != updaterv2.ErrorCode_ERROR_CODE_SIGNATURE {
		t.Fatalf("tampered bundle: %v", err)
	}
	assertBundle(t, root, "2")
	assertNoSiblings(t, root)
}

// Acceptance of an IN_PLACE bundle: unit is read from disk, the lock and the
// write probe live beside the bundle, nothing is added inside it, and the
// rename switch makes the host exit.
func TestHandleApplyAcceptsInPlaceBundle(t *testing.T) {
	asDarwin(t)
	stubSignature(t, false)
	dataDir := t.TempDir()
	dir := t.TempDir()
	root := makeBundle(t, dir, "1", nil)
	plan := bundlePlan(t, t.TempDir(), "2", 2, makeBundle(t, t.TempDir(), "2", nil))
	st := store{dataDir: dataDir}
	key, err := st.planKey()
	if err != nil {
		t.Fatal(err)
	}
	plan.PlanId = "plan-bundle"
	plan.ExpiresAt = timestamppb.New(time.Now().Add(time.Hour))
	plan.Files[0].Downloaded = true
	plan.PlanHmac = hmacPlan(key, plan)
	if err := st.savePlan(plan); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var sessionID string
	eng := &Engine{Stdout: &stdout, LaunchWorker: func(_, id, _ string) error { sessionID = id; return nil }}
	req := &updaterv2.UpdaterRequest{
		Profile: &updaterv2.ClientProfile{ExpectedProduct: "demo"},
		Runtime: &updaterv2.Runtime{
			DataDir: dataDir,
			Install: &updaterv2.InstallSpec{Placement: updaterv2.Placement_PLACEMENT_IN_PLACE, InstallRoot: root},
		},
	}
	if err := eng.handleApply(context.Background(), req, &updaterv2.ApplyOp{PlanId: plan.PlanId}, st); err != nil {
		t.Fatal(err)
	}
	events := decodeEvents(t, &stdout)
	acc := events[0].GetApply().GetAccepted()
	if acc == nil {
		t.Fatalf("bundle apply not accepted: %v", events)
	}
	if !acc.RequiresHostExit {
		t.Fatal("a rename switch replaces the running bundle, the host must exit")
	}
	sess, err := st.loadSession(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if sess.Unit != enginestate.ContentUnit_CONTENT_UNIT_BUNDLE {
		t.Fatalf("session unit = %v", sess.Unit)
	}
	lock := filepath.Join(dir, ".Demo.app.relkit-apply.lock")
	if sessionLockPath(sess) != lock {
		t.Fatalf("session lock = %s", sessionLockPath(sess))
	}
	if _, err := os.Stat(lock); err != nil {
		t.Fatalf("lock beside the bundle: %v", err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "Contents" {
		t.Fatalf("bundle root gained entries: %v", entries)
	}
	releaseApplyLockAt(lock)
}
