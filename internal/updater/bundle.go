package updater

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	enginestate "github.com/shichao402/relkit/api/updater/enginestate/v1"
	updaterv2 "github.com/shichao402/relkit/api/updater/v2"
	"github.com/shichao402/relkit/internal/payload"
)

// hostGOOS is the platform the layout rules are judged for. It is a variable
// only so tests can exercise the darwin rules on any OS.
var hostGOOS = runtime.GOOS

const oldSuffix = ".relkit-old"

// switchMode is how a fresh write becomes the installed content (ADR 0027 §4.3).
type switchMode int

const (
	// switchNone: the reconcile write changes the target in place.
	switchNone switchMode = iota
	// switchPointer renames the partial to versions/<id> and rewrites active.json.
	switchPointer
	// switchRename exchanges the partial with the bundle at install_root.
	switchRename
)

// normUnit maps UNSPECIFIED (a /1 table, or a session from an older engine)
// to TREE.
func normUnit(unit enginestate.ContentUnit) enginestate.ContentUnit {
	if unit == enginestate.ContentUnit_CONTENT_UNIT_BUNDLE {
		return unit
	}
	return enginestate.ContentUnit_CONTENT_UNIT_TREE
}

// layoutFor is the single rule of the four-cell table (ADR 0027 §3): only
// TREE placed IN_PLACE reconciles; everything else writes fresh, and the
// placement picks the switch.
func layoutFor(unit enginestate.ContentUnit, placement updaterv2.Placement) (writeMode, switchMode, error) {
	switch placement {
	case updaterv2.Placement_PLACEMENT_IN_PLACE:
		if normUnit(unit) == enginestate.ContentUnit_CONTENT_UNIT_BUNDLE {
			return writeFresh, switchRename, nil
		}
		return writeReconcile, switchNone, nil
	case updaterv2.Placement_PLACEMENT_LIBRARY:
		return writeFresh, switchPointer, nil
	default:
		return 0, switchNone, fmt.Errorf("unsupported placement")
	}
}

// detectInPlaceUnit tells from disk what an IN_PLACE root holds: a root with
// a regular Contents/Info.plist is a bundle. It only decides where control
// files go before the payload is known; the table's unit is cross-checked
// against it before anything is written.
func detectInPlaceUnit(root string) (enginestate.ContentUnit, error) {
	root = filepath.Clean(root)
	info, err := os.Lstat(filepath.Join(root, filepath.FromSlash(payload.BundleInfoPlist)))
	if err != nil || !info.Mode().IsRegular() {
		return enginestate.ContentUnit_CONTENT_UNIT_TREE, nil
	}
	if li, err := os.Lstat(root); err == nil && li.Mode()&os.ModeSymlink != 0 {
		return 0, failWith(updaterv2.ErrorCode_ERROR_CODE_LAYOUT_UNSUPPORTED,
			"install root %s is a symlink to a bundle; installRoot must name the bundle itself", root)
	}
	return enginestate.ContentUnit_CONTENT_UNIT_BUNDLE, nil
}

// controlDirFor is where the apply lock and the write probe live. A bundle
// replaced by a rename switch must not hold them: writing into it breaks its
// seal and the swap would carry them away. Every other layout keeps them in
// install_root.
func controlDirFor(root string, placement updaterv2.Placement, unit enginestate.ContentUnit) string {
	root = filepath.Clean(root)
	if _, sw, _ := layoutFor(unit, placement); sw == switchRename {
		return filepath.Dir(root)
	}
	return root
}

func applyLockPathFor(root string, placement updaterv2.Placement, unit enginestate.ContentUnit) string {
	root = filepath.Clean(root)
	if _, sw, _ := layoutFor(unit, placement); sw == switchRename {
		return filepath.Join(filepath.Dir(root), "."+filepath.Base(root)+".relkit-apply.lock")
	}
	return applyLockPath(root)
}

// checkUnitSupported is the platform rule: symlinks never reach Windows and
// bundles only exist on macOS. The publish gate enforces the same rule on
// selectors; this is the engine's own check.
func checkUnitSupported(unit enginestate.ContentUnit, symlinks bool, goos string) error {
	if symlinks && goos == "windows" {
		return failWith(updaterv2.ErrorCode_ERROR_CODE_LAYOUT_UNSUPPORTED, "payload with symlinks is not supported on windows")
	}
	if normUnit(unit) == enginestate.ContentUnit_CONTENT_UNIT_BUNDLE && goos != "darwin" {
		return failWith(updaterv2.ErrorCode_ERROR_CODE_LAYOUT_UNSUPPORTED, "bundle payload is only supported on macOS")
	}
	return nil
}

func checkTableSupported(table *enginestate.FileTable, goos string) error {
	return checkUnitSupported(payload.UnitOf(table), payload.HasSymlinks(table), goos)
}

// checkInPlaceUnit refuses a table whose unit differs from what install_root
// holds: a bundle never reconciles into an open directory, and a tree never
// replaces a bundle.
func checkInPlaceUnit(table *enginestate.FileTable, onDisk enginestate.ContentUnit) error {
	if got := payload.UnitOf(table); got != normUnit(onDisk) {
		return failWith(updaterv2.ErrorCode_ERROR_CODE_LAYOUT_UNSUPPORTED,
			"payload unit %s does not match the install root (%s)", got, normUnit(onDisk))
	}
	return nil
}

// checkLibraryRoot keeps a macOS library out of bundles and out of the data
// directory (ADR 0027 §4.4): writing versions/ inside a launcher .app breaks
// its signature.
func checkLibraryRoot(root, dataDir, goos string) error {
	if goos != "darwin" {
		return nil
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	candidates := []string{abs}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil && resolved != abs {
		candidates = append(candidates, resolved)
	}
	if dataDir != "" {
		if d, err := filepath.Abs(dataDir); err == nil {
			for _, c := range candidates {
				if c == d {
					return failWith(updaterv2.ErrorCode_ERROR_CODE_LAYOUT_UNSUPPORTED,
						"library install root must not be the data directory")
				}
			}
		}
	}
	for _, c := range candidates {
		for dir := c; ; dir = filepath.Dir(dir) {
			if strings.HasSuffix(strings.ToLower(filepath.Base(dir)), ".app") {
				return failWith(updaterv2.ErrorCode_ERROR_CODE_LAYOUT_UNSUPPORTED,
					"library install root %s is inside the bundle %s", root, dir)
			}
			if parent := filepath.Dir(dir); parent == dir {
				break
			}
		}
	}
	return nil
}

// verifyBundleSignature checks the bundle is intact as signed. Ad-hoc
// signatures pass: this guards completeness, not identity. A variable so tests
// can run without codesign.
var verifyBundleSignature = func(bundle string) error {
	out, err := exec.Command("codesign", "--verify", "--deep", "--strict", bundle).CombinedOutput()
	if err != nil {
		return failWith(updaterv2.ErrorCode_ERROR_CODE_SIGNATURE,
			"codesign rejected %s: %v: %s", filepath.Base(bundle), err, strings.TrimSpace(string(out)))
	}
	return nil
}

var errSwapUnsupported = errors.New("atomic directory swap is not supported here")

// swapDirs is a variable so tests can force the two-rename fallback.
var swapDirs = swapDirsNative

func oldPathFor(target string) string {
	return filepath.Join(filepath.Dir(target), "."+filepath.Base(target)+oldSuffix)
}

// recoverSwap settles a rename switch an earlier worker left half done. The
// state is read from the two siblings, so no journal is needed:
//   - T missing and O present: the fallback stopped between its two renames;
//     O is the old bundle, put it back.
//   - T present: O, if any, is an old bundle awaiting deletion.
//   - P is never installed content: either an unfinished fresh write, or the
//     old bundle an atomic swap left behind.
func recoverSwap(target string) error {
	partial, old := partialPathFor(target), oldPathFor(target)
	if _, err := os.Lstat(target); errors.Is(err, os.ErrNotExist) {
		if _, oerr := os.Lstat(old); oerr == nil {
			if err := os.Rename(old, target); err != nil {
				return fmt.Errorf("INCOMPLETE: could not restore %s: %w", filepath.Base(target), err)
			}
		}
	} else if err != nil {
		return err
	}
	if _, err := os.Lstat(target); err == nil {
		if err := os.RemoveAll(old); err != nil {
			return err
		}
	}
	return os.RemoveAll(partial)
}

// applySwap is BUNDLE/IN_PLACE: a fresh write beside the bundle, verified,
// then exchanged with it (ADR 0027 §4.2, §4.3). Until the exchange the
// installed bundle is untouched; after it the old bundle is deleted, since
// rollback belongs to LIBRARY.
func applySwap(dataDir string, sess *enginestate.ApplySessionRecord, content *applyContent) error {
	target := filepath.Clean(sess.InstallRoot)
	if err := recoverSwap(target); err != nil {
		return err
	}
	if len(sess.Preserve) > 0 {
		return failWith(updaterv2.ErrorCode_ERROR_CODE_LAYOUT_UNSUPPORTED, "a bundle replaced in place cannot preserve entries")
	}
	if info, err := os.Lstat(target); err != nil || !info.IsDir() {
		return failWith(updaterv2.ErrorCode_ERROR_CODE_LAYOUT_UNSUPPORTED, "bundle %s is missing", target)
	}
	partial, err := materializeFresh(content, target, "")
	if err != nil {
		return err
	}
	if err := exchange(partial, target); err != nil {
		_ = os.RemoveAll(partial)
		return err
	}
	// Commit: P (atomic swap) or O (fallback) now holds the old bundle.
	_ = os.RemoveAll(partial)
	_ = os.RemoveAll(oldPathFor(target))
	_ = os.Remove(baselinePathFor(dataDir, sess.Product))
	return nil
}

// exchange puts partial at target. renamex_np(RENAME_SWAP) does it in one
// step on APFS; elsewhere two renames go through O, and a failure in between
// puts the old bundle back.
func exchange(partial, target string) error {
	if err := swapDirs(partial, target); err == nil {
		return nil
	}
	old := oldPathFor(target)
	if err := os.RemoveAll(old); err != nil {
		return err
	}
	if err := os.Rename(target, old); err != nil {
		return fmt.Errorf("could not move the old bundle aside: %w", err)
	}
	if err := os.Rename(partial, target); err != nil {
		if rerr := os.Rename(old, target); rerr != nil {
			return fmt.Errorf("INCOMPLETE: new bundle not moved in (%v) and old bundle not restored: %w", err, rerr)
		}
		return fmt.Errorf("could not move the new bundle into place: %w", err)
	}
	return nil
}

// checkNoSymlinkParents refuses to write rel when a directory on its way
// under root is a symlink: following it could write outside root. It returns
// the relpath of the offending directory. verified caches directories known
// to be real.
func checkNoSymlinkParents(root, rel string, verified map[string]struct{}) (string, error) {
	parts := strings.Split(rel, "/")
	cur := ""
	for _, part := range parts[:len(parts)-1] {
		if cur == "" {
			cur = part
		} else {
			cur += "/" + part
		}
		if _, ok := verified[cur]; ok {
			continue
		}
		info, err := os.Lstat(filepath.Join(root, filepath.FromSlash(cur)))
		if errors.Is(err, os.ErrNotExist) {
			// Nothing deeper exists either; MkdirAll creates real directories.
			return "", nil
		}
		if err != nil {
			return "", err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return cur, fmt.Errorf("%s: parent %s is a symlink", rel, cur)
		}
		if !info.IsDir() {
			return cur, fmt.Errorf("%s: parent %s is not a directory", rel, cur)
		}
		verified[cur] = struct{}{}
	}
	return "", nil
}

// writeEntry puts one table entry at dest: a symlink, or a file copied from
// the extracted payload with its mode.
func writeEntry(content *applyContent, file *enginestate.FileEntry, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	if payload.IsSymlink(file) {
		return os.Symlink(file.LinkTarget, dest)
	}
	src := filepath.Join(content.filesRoot, filepath.FromSlash(file.Relpath))
	if err := copyFile(src, dest); err != nil {
		return err
	}
	return os.Chmod(dest, fileMode(file))
}
