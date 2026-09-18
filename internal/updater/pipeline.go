package updater

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	updaterv1 "go.firoyang.com/relkit/api/updater/v1"
	"go.firoyang.com/relkit/internal/model"
	"go.firoyang.com/relkit/internal/payload"
	"google.golang.org/protobuf/proto"
)

const (
	baselineFileName = "baseline.pb"
	baselineSchema   = "relkit.baseline/1"
)

type applyContent struct {
	table       *updaterv1.FileTable
	filesRoot   string
	scriptsRoot string
}

func planRequiresHostExit(inst *updaterv1.InstallSpec, plan *updaterv1.UpdatePlan) (bool, error) {
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
	for _, file := range table.Files {
		if file == nil {
			continue
		}
		if inst.Placement == updaterv1.Placement_PLACEMENT_IN_PLACE &&
			sameRelpath(file.Relpath, inst.ExecutableRelpath) {
			return true, nil
		}
		if sameRelpath(file.Relpath, inst.SidecarRelpath) {
			return true, nil
		}
	}
	return false, nil
}

func applyPlan(dataDir string, sess *updaterv1.ApplySessionRecord, plan *updaterv1.UpdatePlan) error {
	content, err := prepareContent(sess, plan)
	if err != nil {
		return err
	}
	switch sess.Placement {
	case updaterv1.Placement_PLACEMENT_IN_PLACE:
		return applyContentTable(dataDir, sess, plan, content, sess.InstallRoot, true)
	case updaterv1.Placement_PLACEMENT_LIBRARY:
		return applyLibraryContent(dataDir, sess, plan, content)
	default:
		return fmt.Errorf("unsupported placement")
	}
}

func prepareContent(sess *updaterv1.ApplySessionRecord, plan *updaterv1.UpdatePlan) (*applyContent, error) {
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

func applyLibraryContent(dataDir string, sess *updaterv1.ApplySessionRecord, plan *updaterv1.UpdatePlan, content *applyContent) error {
	id := strings.ReplaceAll(plan.Version, "/", "_")
	if id == "" {
		id = fmt.Sprintf("code-%d", plan.Code)
	}
	versionRoot := filepath.Join(sess.InstallRoot, "versions", id)
	_ = os.RemoveAll(versionRoot)
	targetRoot := versionRoot
	bundleName := ""
	if strings.HasSuffix(strings.ToLower(content.filesRoot), ".app") {
		bundleName = filepath.Base(content.filesRoot)
		targetRoot = filepath.Join(versionRoot, bundleName)
	}
	if err := applyContentTable(dataDir, sess, plan, content, targetRoot, false); err != nil {
		_ = os.RemoveAll(versionRoot)
		return err
	}
	versionRel := filepath.ToSlash(filepath.Join("versions", id))
	executableRel := filepath.ToSlash(filepath.Join(versionRel, bundleName, sess.ExecutableRelpath))
	meta := versionMeta{Code: plan.Code, Version: plan.Version, Path: versionRel, Executable: executableRel}
	if err := writeVersionMeta(versionRoot, meta); err != nil {
		return err
	}
	previous := readActive(sess.InstallRoot)
	pointer := activePointer{Code: plan.Code, Version: plan.Version, Path: versionRel, Executable: executableRel}
	if previous != nil {
		pointer.Previous = previous.Path
	}
	if !sess.InstallOnly {
		if err := writeActive(sess.InstallRoot, pointer); err != nil {
			return err
		}
	} else if previous != nil {
		pointer = *previous
	}
	retain := int(sess.GetLibrary().GetRetain())
	if retain <= 0 {
		retain = 2
	}
	pruneVersions(sess.InstallRoot, pointer, retain, sess.GetLibrary().GetReservedCodes(), versionRel)
	refreshSidecar(sess, content.filesRoot)
	return nil
}

func applyContentTable(dataDir string, sess *updaterv1.ApplySessionRecord, plan *updaterv1.UpdatePlan, content *applyContent, targetRoot string, useBaseline bool) error {
	if err := runScripts(content, updaterv1.ScriptPhase_SCRIPT_PHASE_PRE_APPLY, sess.InstallRoot); err != nil {
		return fmt.Errorf("pre-apply script: %w", err)
	}
	if err := os.MkdirAll(targetRoot, 0o755); err != nil {
		return err
	}
	journalPath := filepath.Join(sess.StagedRoot, "journal.pb")
	backupRoot := filepath.Join(sess.StagedRoot, "backup")
	_ = os.RemoveAll(backupRoot)
	journal := &updaterv1.ApplyJournal{SessionId: sess.SessionId}
	rollback := func(cause error) error {
		rollbackJournal(journal)
		return cause
	}
	preserved := append([]string(nil), sess.Preserve...)
	preserved = append(preserved, content.table.Preserve...)
	current := make(map[string]*updaterv1.FileEntry, len(content.table.Files))
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
		entry := &updaterv1.JournalEntry{DestPath: dest, BackupPath: backup, SourcePath: src}
		journal.Entries = append(journal.Entries, entry)
		if err := writeJournal(journalPath, journal); err != nil {
			return rollback(err)
		}
		if err := copyFile(src, dest); err != nil {
			return rollback(err)
		}
		mode := os.FileMode(file.Mode)
		if mode == 0 {
			mode = 0o644
		}
		if err := os.Chmod(dest, mode); err != nil {
			return rollback(err)
		}
	}
	if useBaseline {
		baseline := loadBaseline(filepath.Join(dataDir, baselineFileName))
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
			journal.Entries = append(journal.Entries, &updaterv1.JournalEntry{DestPath: dest, BackupPath: backup})
			if err := writeJournal(journalPath, journal); err != nil {
				return rollback(err)
			}
		}
	}
	if err := runScripts(content, updaterv1.ScriptPhase_SCRIPT_PHASE_POST_APPLY, sess.InstallRoot); err != nil {
		return rollback(fmt.Errorf("post-apply script: %w", err))
	}
	if useBaseline {
		next := &updaterv1.Baseline{Schema: baselineSchema, Code: plan.Code, Version: plan.Version}
		for _, file := range content.table.Files {
			if file != nil {
				next.Files = append(next.Files, &updaterv1.BaselineEntry{Relpath: file.Relpath, Sha256: file.Sha256})
			}
		}
		if err := saveBaseline(filepath.Join(dataDir, baselineFileName), next); err != nil {
			return rollback(err)
		}
	}
	journal.Committed = true
	if err := writeJournal(journalPath, journal); err != nil {
		return rollback(err)
	}
	_ = os.RemoveAll(backupRoot)
	removeEmptyDirectories(targetRoot)
	refreshSidecar(sess, content.filesRoot)
	return nil
}

func runScripts(content *applyContent, phase updaterv1.ScriptPhase, workDir string) error {
	for _, script := range content.table.Scripts {
		if script == nil || script.Phase != phase {
			continue
		}
		path := filepath.Join(content.scriptsRoot, filepath.FromSlash(script.Relpath))
		timeout := script.GetTimeout().AsDuration()
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		var command *exec.Cmd
		switch script.Interpreter {
		case updaterv1.ScriptInterpreter_SCRIPT_INTERPRETER_DIRECT:
			command = exec.CommandContext(ctx, path, script.Args...)
		case updaterv1.ScriptInterpreter_SCRIPT_INTERPRETER_SH:
			command = exec.CommandContext(ctx, "sh", append([]string{path}, script.Args...)...)
		case updaterv1.ScriptInterpreter_SCRIPT_INTERPRETER_POWERSHELL:
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

func loadBaseline(path string) *updaterv1.Baseline {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	baseline := &updaterv1.Baseline{}
	if proto.Unmarshal(raw, baseline) != nil || baseline.Schema != baselineSchema {
		return nil
	}
	return baseline
}

func saveBaseline(path string, baseline *updaterv1.Baseline) error {
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

func removeEmptyDirectories(root string) {
	var dirs []string
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err == nil && info != nil && info.IsDir() && path != root {
			dirs = append(dirs, path)
		}
		return nil
	})
	sort.Slice(dirs, func(i, j int) bool { return len(dirs[i]) > len(dirs[j]) })
	for _, dir := range dirs {
		_ = os.Remove(dir)
	}
}
