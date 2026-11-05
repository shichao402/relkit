package updater

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	enginestate "github.com/shichao402/relkit/api/updater/enginestate/v1"
	updaterv2 "github.com/shichao402/relkit/api/updater/v2"
	"github.com/shichao402/relkit/internal/model"
	"github.com/shichao402/relkit/internal/payload"
	"google.golang.org/protobuf/proto"
)

const (
	// legacyBaselineFileName is the pre-ADR 0027 baseline, shared by every
	// product in a data directory. It is read once and migrated.
	legacyBaselineFileName = "baseline.pb"
	baselineSchema         = "relkit.baseline/1"
	partialSuffix          = ".relkit-partial"
)

type applyContent struct {
	table       *enginestate.FileTable
	filesRoot   string
	scriptsRoot string
}

// applyFailure carries the error code a failed apply reports. Any other error
// is reported as DISK.
type applyFailure struct {
	code updaterv2.ErrorCode
	err  error
}

func (f *applyFailure) Error() string { return f.err.Error() }
func (f *applyFailure) Unwrap() error { return f.err }

func failWith(code updaterv2.ErrorCode, format string, args ...any) error {
	return &applyFailure{code: code, err: fmt.Errorf(format, args...)}
}

func applyErrorCode(err error) updaterv2.ErrorCode {
	var f *applyFailure
	if errors.As(err, &f) {
		return f.code
	}
	return updaterv2.ErrorCode_ERROR_CODE_DISK
}

// writeMode is how content reaches disk (ADR 0027 §4).
type writeMode int

const (
	// writeReconcile updates an open directory file by file against the
	// baseline. Only TREE content placed IN_PLACE uses it.
	writeReconcile writeMode = iota + 1
	// writeFresh builds the target from nothing beside it and switches to it.
	writeFresh
)

// writeModeFor is the single rule that picks the write path. The target is
// owned by the content everywhere except IN_PLACE, whose install root also
// holds launchers, pointers, user files and preserved entries.
func writeModeFor(placement updaterv2.Placement) (writeMode, error) {
	switch placement {
	case updaterv2.Placement_PLACEMENT_IN_PLACE:
		return writeReconcile, nil
	case updaterv2.Placement_PLACEMENT_LIBRARY:
		return writeFresh, nil
	default:
		return 0, fmt.Errorf("unsupported placement")
	}
}

// planRequiresHostExit answers whether the worker must wait for the host.
// A pointer switch never touches running files; a reconcile write does when
// the table replaces the host executable or the engine beside it.
func planRequiresHostExit(inst *updaterv2.InstallSpec, plan *enginestate.UpdatePlan) (bool, error) {
	if inst == nil || plan == nil || len(plan.Files) == 0 {
		return false, fmt.Errorf("plan has no files")
	}
	first := plan.Files[0]
	if first.Kind != "payload" {
		return false, fmt.Errorf("artifact requires the full installation flow")
	}
	table, err := payload.Validate(first.LocalPath)
	if err != nil {
		return false, err
	}
	mode, err := writeModeFor(inst.Placement)
	if err != nil {
		return false, err
	}
	if mode != writeReconcile {
		return false, nil
	}
	for _, file := range table.Files {
		if file == nil {
			continue
		}
		if sameRelpath(file.Relpath, inst.ExecutableRelpath) || sameRelpath(file.Relpath, inst.SidecarRelpath) {
			return true, nil
		}
	}
	return false, nil
}

func applyPlan(dataDir string, sess *enginestate.ApplySessionRecord, plan *enginestate.UpdatePlan) error {
	mode, err := writeModeFor(sess.Placement)
	if err != nil {
		return err
	}
	content, err := prepareContent(sess, plan)
	if err != nil {
		return err
	}
	switch mode {
	case writeReconcile:
		return applyReconcile(dataDir, sess, plan, content, sess.InstallRoot)
	default:
		return applyLibrary(sess, plan, content)
	}
}

func prepareContent(sess *enginestate.ApplySessionRecord, plan *enginestate.UpdatePlan) (*applyContent, error) {
	if len(plan.Files) == 0 {
		return nil, fmt.Errorf("plan has no files")
	}
	first := plan.Files[0]
	if first.Kind != "payload" {
		return nil, fmt.Errorf("artifact requires the full installation flow")
	}
	pkg, err := payload.Extract(first.LocalPath, filepath.Join(sess.StagedRoot, "payload"))
	if err != nil {
		return nil, err
	}
	return &applyContent{table: pkg.Table, filesRoot: pkg.FilesRoot, scriptsRoot: pkg.ScriptsRoot}, nil
}

// libraryVersionID names the version directory for a plan.
func libraryVersionID(plan *enginestate.UpdatePlan) string {
	id := strings.ReplaceAll(plan.Version, "/", "_")
	if id == "" {
		id = fmt.Sprintf("code-%d", plan.Code)
	}
	return id
}

// applyLibrary installs one version with a fresh write and a pointer switch
// (ADR 0027 §4.2, §4.3). Nothing already installed is modified until the
// atomic rename, so a failure leaves every version as it was.
func applyLibrary(sess *enginestate.ApplySessionRecord, plan *enginestate.UpdatePlan, content *applyContent) error {
	root := sess.InstallRoot
	id := libraryVersionID(plan)
	versionsDir := filepath.Join(root, VersionsDirName)
	if err := os.MkdirAll(versionsDir, 0o755); err != nil {
		return err
	}
	target := filepath.Join(versionsDir, id)
	versionRel := path.Join(VersionsDirName, id)
	executableRel := versionRel
	if sess.ExecutableRelpath != "" {
		executableRel = path.Join(versionRel, filepath.ToSlash(sess.ExecutableRelpath))
	}

	reuse, err := prepareLibraryTarget(root, target, plan.Code)
	if err != nil {
		return err
	}
	if !reuse {
		partial, err := materializeFresh(content, target)
		if err != nil {
			return err
		}
		meta := versionMeta{Code: plan.Code, Version: plan.Version, Path: versionRel, Executable: executableRel}
		if err := writeVersionMeta(partial, meta); err != nil {
			_ = os.RemoveAll(partial)
			return err
		}
		if err := renameWithRetry(partial, target); err != nil {
			_ = os.RemoveAll(partial)
			return fmt.Errorf("could not move the new version into place: %w", err)
		}
	}

	previous := readActive(root)
	pointer := activePointer{Code: plan.Code, Version: plan.Version, Path: versionRel, Executable: executableRel}
	if previous != nil {
		if filepath.ToSlash(previous.Path) == versionRel {
			pointer.Previous = previous.Previous
		} else {
			pointer.Previous = previous.Path
		}
	}
	if !sess.InstallOnly {
		if err := writeActive(root, pointer); err != nil {
			return err
		}
	} else if previous != nil {
		pointer = *previous
	}
	pruneVersions(root, pointer, int(sess.GetLibrary().GetRetain()), sess.GetLibrary().GetReservedCodes(), versionRel)
	return nil
}

// prepareLibraryTarget decides what to do when versions/<id> already exists.
// A directory whose metadata names the planned code can only have come from a
// completed rename, so it is reused as is. Anything else is foreign or
// damaged: set aside when idle, refused when a process runs from it.
func prepareLibraryTarget(root, target string, code int64) (reuse bool, err error) {
	if _, err := os.Lstat(target); errors.Is(err, os.ErrNotExist) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	if meta := readVersionMeta(target); meta != nil && meta.Code == code {
		return true, nil
	}
	if _, busy := versionsInUse(root)[filepath.Base(target)]; busy {
		return false, failWith(updaterv2.ErrorCode_ERROR_CODE_OCCUPIED,
			"%s exists without matching version metadata and a running process uses it", filepath.Base(target))
	}
	if err := moveToTrash(root, target); err != nil {
		return false, err
	}
	return false, nil
}

// moveToTrash renames dir under install_root/.trash and then removes it. The
// rename takes it out of view at once; removal is best effort beyond that.
func moveToTrash(root, dir string) error {
	trash := filepath.Join(root, TrashDirName)
	if err := os.MkdirAll(trash, 0o755); err != nil {
		return err
	}
	quarantined := filepath.Join(trash, fmt.Sprintf("%s.%d", filepath.Base(dir), time.Now().UnixNano()))
	if err := os.Rename(dir, quarantined); err != nil {
		return fmt.Errorf("could not move %s aside: %w", filepath.Base(dir), err)
	}
	if err := os.RemoveAll(quarantined); err != nil {
		return fmt.Errorf("moved to %s but could not remove it: %w", quarantined, err)
	}
	return nil
}

// partialPathFor is the hidden sibling a fresh write builds in. It lives on
// the target's volume so the final rename is atomic.
func partialPathFor(target string) string {
	return filepath.Join(filepath.Dir(target), "."+filepath.Base(target)+partialSuffix)
}

// materializeFresh builds the content in the partial directory and runs the
// scripts there. No journal and no backup: the partial is new, so failing
// means deleting it. The caller renames the returned path into place.
func materializeFresh(content *applyContent, target string) (string, error) {
	partial := partialPathFor(target)
	if err := os.RemoveAll(partial); err != nil {
		return "", err
	}
	if err := os.MkdirAll(partial, 0o755); err != nil {
		return "", err
	}
	fail := func(cause error) (string, error) {
		_ = os.RemoveAll(partial)
		return "", cause
	}
	if err := runScripts(content, enginestate.ScriptPhase_SCRIPT_PHASE_PRE_APPLY, partial); err != nil {
		return fail(fmt.Errorf("pre-apply script: %w", err))
	}
	for _, file := range content.table.Files {
		if file == nil {
			continue
		}
		src := filepath.Join(content.filesRoot, filepath.FromSlash(file.Relpath))
		dest := filepath.Join(partial, filepath.FromSlash(file.Relpath))
		if err := copyFile(src, dest); err != nil {
			return fail(err)
		}
		if err := os.Chmod(dest, fileMode(file)); err != nil {
			return fail(err)
		}
	}
	if err := runScripts(content, enginestate.ScriptPhase_SCRIPT_PHASE_POST_APPLY, partial); err != nil {
		return fail(fmt.Errorf("post-apply script: %w", err))
	}
	return partial, nil
}

func fileMode(file *enginestate.FileEntry) os.FileMode {
	if mode := os.FileMode(file.Mode); mode != 0 {
		return mode
	}
	return 0o644
}

// renameWithRetry renames a freshly written directory. On Windows antivirus
// and indexing briefly hold handles on new files, which makes a directory
// rename fail; retry with backoff for about five seconds. Elsewhere a failed
// rename is not transient.
func renameWithRetry(from, to string) error {
	err := os.Rename(from, to)
	if err == nil || runtime.GOOS != "windows" {
		return err
	}
	delay := 50 * time.Millisecond
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(delay)
		if err = os.Rename(from, to); err == nil {
			return nil
		}
		if delay < time.Second {
			delay *= 2
		}
	}
	return err
}

// applyReconcile updates an open install root in place (ADR 0027 §4.1): the
// old file goes to a same-volume backup under a journal, the new file is
// written, files the baseline owned and the table dropped are removed, and
// any failure rolls the journal back.
func applyReconcile(dataDir string, sess *enginestate.ApplySessionRecord, plan *enginestate.UpdatePlan, content *applyContent, targetRoot string) error {
	if err := runScripts(content, enginestate.ScriptPhase_SCRIPT_PHASE_PRE_APPLY, sess.InstallRoot); err != nil {
		return fmt.Errorf("pre-apply script: %w", err)
	}
	if err := os.MkdirAll(targetRoot, 0o755); err != nil {
		return err
	}
	journalPath := filepath.Join(sess.StagedRoot, "journal.pb")
	// backup 必须与 targetRoot 同卷：os.Rename 不能跨盘符（Windows EXDEV）。
	backupRoot := filepath.Join(targetRoot, ".relkit-backup", sess.SessionId)
	_ = os.RemoveAll(backupRoot)
	journal := &enginestate.ApplyJournal{SessionId: sess.SessionId}
	rollback := func(cause error) error {
		rollbackJournal(journal)
		return cause
	}
	preserved := append([]string(nil), sess.Preserve...)
	preserved = append(preserved, content.table.Preserve...)
	current := make(map[string]*enginestate.FileEntry, len(content.table.Files))
	for _, file := range content.table.Files {
		if file == nil {
			continue
		}
		current[file.Relpath] = file
		dest := filepath.Join(targetRoot, filepath.FromSlash(file.Relpath))
		if matchesPreserve(file.Relpath, preserved) {
			if _, err := os.Stat(dest); err == nil {
				continue
			}
		}
		src := filepath.Join(content.filesRoot, filepath.FromSlash(file.Relpath))
		backup := ""
		if _, err := os.Stat(dest); err == nil {
			backup = filepath.Join(backupRoot, filepath.FromSlash(file.Relpath))
			if err := os.MkdirAll(filepath.Dir(backup), 0o755); err != nil {
				return rollback(err)
			}
			if err := os.Rename(dest, backup); err != nil {
				return rollback(err)
			}
		}
		entry := &enginestate.JournalEntry{DestPath: dest, BackupPath: backup, SourcePath: src}
		journal.Entries = append(journal.Entries, entry)
		if err := writeJournal(journalPath, journal); err != nil {
			return rollback(err)
		}
		if err := copyFile(src, dest); err != nil {
			return rollback(err)
		}
		if err := os.Chmod(dest, fileMode(file)); err != nil {
			return rollback(err)
		}
	}
	baselinePath := baselinePathFor(dataDir, sess.Product)
	baseline, legacy := loadProductBaseline(dataDir, sess.Product)
	var removedDirs []string
	for _, old := range baseline.GetFiles() {
		if old == nil {
			continue
		}
		if _, exists := current[old.Relpath]; exists || matchesPreserve(old.Relpath, preserved) {
			continue
		}
		dest := filepath.Join(targetRoot, filepath.FromSlash(old.Relpath))
		digest, _, err := model.Sha256File(dest)
		if err != nil || digest != old.Sha256 {
			continue
		}
		backup := filepath.Join(backupRoot, filepath.FromSlash(old.Relpath))
		if err := os.MkdirAll(filepath.Dir(backup), 0o755); err != nil {
			return rollback(err)
		}
		if err := os.Rename(dest, backup); err != nil {
			return rollback(err)
		}
		journal.Entries = append(journal.Entries, &enginestate.JournalEntry{DestPath: dest, BackupPath: backup})
		if err := writeJournal(journalPath, journal); err != nil {
			return rollback(err)
		}
		removedDirs = append(removedDirs, filepath.Dir(dest))
	}
	if err := runScripts(content, enginestate.ScriptPhase_SCRIPT_PHASE_POST_APPLY, sess.InstallRoot); err != nil {
		return rollback(fmt.Errorf("post-apply script: %w", err))
	}
	next := &enginestate.Baseline{Schema: baselineSchema, Code: plan.Code, Version: plan.Version}
	for _, file := range content.table.Files {
		if file != nil {
			next.Files = append(next.Files, &enginestate.BaselineEntry{Relpath: file.Relpath, Sha256: file.Sha256})
		}
	}
	if err := saveBaseline(baselinePath, next); err != nil {
		return rollback(err)
	}
	if legacy != "" && legacy != baselinePath {
		_ = os.Remove(legacy)
	}
	journal.Committed = true
	if err := writeJournal(journalPath, journal); err != nil {
		return rollback(err)
	}
	_ = os.RemoveAll(backupRoot)
	_ = os.Remove(filepath.Dir(backupRoot))
	removeEmptyAncestors(targetRoot, removedDirs)
	return nil
}

func runScripts(content *applyContent, phase enginestate.ScriptPhase, workDir string) error {
	for _, script := range content.table.Scripts {
		if script == nil || script.Phase != phase {
			continue
		}
		path := filepath.Join(content.scriptsRoot, filepath.FromSlash(script.Relpath))
		timeout := script.GetTimeout().AsDuration()
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		var command *exec.Cmd
		switch script.Interpreter {
		case enginestate.ScriptInterpreter_SCRIPT_INTERPRETER_DIRECT:
			command = exec.CommandContext(ctx, path, script.Args...)
		case enginestate.ScriptInterpreter_SCRIPT_INTERPRETER_SH:
			command = exec.CommandContext(ctx, "sh", append([]string{path}, script.Args...)...)
		case enginestate.ScriptInterpreter_SCRIPT_INTERPRETER_POWERSHELL:
			args := []string{"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", path}
			command = exec.CommandContext(ctx, "powershell", append(args, script.Args...)...)
		default:
			cancel()
			return fmt.Errorf("unsupported interpreter for %q", script.Relpath)
		}
		command.Dir = workDir
		output, err := command.CombinedOutput()
		cancel()
		if err != nil {
			return fmt.Errorf("%s failed: %w: %s", script.Relpath, err, strings.TrimSpace(string(output)))
		}
	}
	return nil
}

// baselinePathFor scopes the baseline to one product: products sharing a
// data directory (a managed library) must not read each other's baseline
// (ADR 0027 §6.2).
func baselinePathFor(dataDir, product string) string {
	name := sanitizeFileComponent(product)
	if name == "" {
		return filepath.Join(dataDir, legacyBaselineFileName)
	}
	return filepath.Join(dataDir, "baseline-"+name+".pb")
}

func sanitizeFileComponent(s string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	return strings.Trim(b.String(), ".")
}

// loadProductBaseline reads the product's baseline, falling back to the
// legacy shared file once. The second result names the legacy file to delete
// after the new baseline is saved, or "".
func loadProductBaseline(dataDir, product string) (*enginestate.Baseline, string) {
	path := baselinePathFor(dataDir, product)
	if b := loadBaseline(path); b != nil {
		return b, ""
	}
	legacy := filepath.Join(dataDir, legacyBaselineFileName)
	if legacy == path {
		return nil, ""
	}
	if b := loadBaseline(legacy); b != nil {
		return b, legacy
	}
	return nil, ""
}

func loadBaseline(path string) *enginestate.Baseline {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	baseline := &enginestate.Baseline{}
	if proto.Unmarshal(raw, baseline) != nil || baseline.Schema != baselineSchema {
		return nil
	}
	return baseline
}

func saveBaseline(path string, baseline *enginestate.Baseline) error {
	raw, err := proto.Marshal(baseline)
	if err != nil {
		return err
	}
	return atomicWrite(path, raw)
}

func matchesPreserve(rel string, preserved []string) bool {
	rel = filepath.ToSlash(filepath.Clean(filepath.FromSlash(rel)))
	if runtime.GOOS == "windows" {
		rel = strings.ToLower(rel)
	}
	for _, candidate := range preserved {
		candidate = strings.TrimSuffix(filepath.ToSlash(filepath.Clean(filepath.FromSlash(candidate))), "/")
		if runtime.GOOS == "windows" {
			candidate = strings.ToLower(candidate)
		}
		if candidate != "." && (rel == candidate || strings.HasPrefix(rel, candidate+"/")) {
			return true
		}
	}
	return false
}

func sameRelpath(left, right string) bool {
	if left == "" || right == "" {
		return false
	}
	left = filepath.ToSlash(filepath.Clean(filepath.FromSlash(left)))
	right = filepath.ToSlash(filepath.Clean(filepath.FromSlash(right)))
	if runtime.GOOS == "windows" {
		return strings.EqualFold(left, right)
	}
	return left == right
}

// removeEmptyAncestors removes directories a reconcile write emptied, walking
// up from each until a non-empty one or the root. It never scans the root:
// an open install root may hold another product's tree whose empty
// directories are meant to be there (ADR 0027 §6.2).
func removeEmptyAncestors(root string, dirs []string) {
	root = filepath.Clean(root)
	for _, dir := range dirs {
		for dir = filepath.Clean(dir); dir != root; dir = filepath.Dir(dir) {
			if _, ok := firstComponentUnder(root, dir); !ok {
				break
			}
			if os.Remove(dir) != nil {
				break
			}
		}
	}
}
