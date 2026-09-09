package updater

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	updaterv1 "firoyang.com/relkit/api/updater/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (e *Engine) handleApply(_ context.Context, req *updaterv1.UpdaterRequest, op *updaterv1.ApplyOp, st store) error {
	if op == nil || op.PlanId == "" {
		return e.emitApplyFail(updaterv1.ErrorCode_ERROR_CODE_PLAN_UNKNOWN, "planId required")
	}
	plan, err := st.loadPlan(op.PlanId)
	if err != nil {
		return e.emitApplyFail(updaterv1.ErrorCode_ERROR_CODE_PLAN_UNKNOWN, "unknown planId")
	}
	key, err := st.planKey()
	if err != nil || !verifyPlan(key, plan) {
		return e.emitApplyFail(updaterv1.ErrorCode_ERROR_CODE_PLAN_TAMPERED, "plan hmac mismatch")
	}
	if plan.ExpiresAt != nil && time.Now().After(plan.ExpiresAt.AsTime()) {
		return e.emitApplyFail(updaterv1.ErrorCode_ERROR_CODE_PLAN_EXPIRED, "plan expired")
	}
	for _, f := range plan.Files {
		if !f.Downloaded || f.LocalPath == "" {
			return e.emitApplyFail(updaterv1.ErrorCode_ERROR_CODE_PLAN_NOT_DOWNLOADED, "download the plan before apply")
		}
		if fi, err := os.Stat(f.LocalPath); err != nil || fi.Size() != f.Size {
			return e.emitApplyFail(updaterv1.ErrorCode_ERROR_CODE_PLAN_TAMPERED, "downloaded file size mismatch")
		}
	}
	inst := req.GetRuntime().GetInstall()
	if inst == nil {
		return e.emitApplyFail(updaterv1.ErrorCode_ERROR_CODE_LAYOUT_UNSUPPORTED, "install spec required")
	}
	if inst.InstallRoot == "" {
		return e.emitApplyFail(updaterv1.ErrorCode_ERROR_CODE_PROFILE_INVALID, "installRoot required")
	}
	if err := assertWritable(inst.InstallRoot); err != nil {
		return e.emitApplyFail(updaterv1.ErrorCode_ERROR_CODE_PERMISSION_DENIED, err.Error())
	}

	sessionID := newID("sess")
	now := timestamppb.Now()
	sess := &updaterv1.ApplySessionRecord{
		SessionId:         sessionID,
		PlanId:            plan.PlanId,
		Phase:             updaterv1.SessionPhase_SESSION_PHASE_WAITING_FOR_EXIT,
		StartedAt:         now,
		HeartbeatAt:       now,
		InstallRoot:       inst.InstallRoot,
		StagedRoot:        filepath.Join(req.GetRuntime().GetDataDir(), "staging", plan.PlanId),
		TargetCode:        plan.Code,
		TargetVersion:     plan.Version,
		Pid:               int32(os.Getpid()),
		Layout:            inst.Layout,
		Relaunch:          inst.Relaunch,
		ExecutableRelpath: inst.ExecutableRelpath,
		Preserve:          inst.Preserve,
		Retain:            inst.Retain,
		FileSet:           inst.FileSet,
		SidecarRelpath:    inst.SidecarRelpath,
	}
	if err := st.saveSession(sess); err != nil {
		return e.emitApplyFail(updaterv1.ErrorCode_ERROR_CODE_DISK, err.Error())
	}
	if inst.Layout == updaterv1.Layout_LAYOUT_VERSIONED_DIR {
		_ = writeLegacyJSONSession(inst.InstallRoot, sess)
	}
	return e.emit(&updaterv1.UpdaterEvent{
		Kind: &updaterv1.UpdaterEvent_Apply{Apply: &updaterv1.ApplyResult{
			Kind: &updaterv1.ApplyResult_Accepted{Accepted: &updaterv1.ApplyAccepted{
				SessionId:        sessionID,
				PlanId:           plan.PlanId,
				RequiresHostExit: layoutRequiresHostExit(inst.Layout),
			}},
		}},
	})
}

func (e *Engine) emitApplyFail(code updaterv1.ErrorCode, message string) error {
	return e.emit(&updaterv1.UpdaterEvent{
		Kind: &updaterv1.UpdaterEvent_Apply{Apply: &updaterv1.ApplyResult{
			Kind: &updaterv1.ApplyResult_Failed{Failed: &updaterv1.Failed{
				Error: newError(code, false, message, nil),
			}},
		}},
	})
}

func assertWritable(root string) error {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	probe := filepath.Join(root, ".relkit-write-probe")
	if err := os.WriteFile(probe, []byte("ok"), 0o644); err != nil {
		return fmt.Errorf("install root not writable")
	}
	_ = os.Remove(probe)
	return nil
}

func writeLegacyJSONSession(installRoot string, sess *updaterv1.ApplySessionRecord) error {
	state := "running"
	switch sess.Phase {
	case updaterv1.SessionPhase_SESSION_PHASE_COMPLETED:
		state = "succeeded"
	case updaterv1.SessionPhase_SESSION_PHASE_ROLLED_BACK, updaterv1.SessionPhase_SESSION_PHASE_NEEDS_ATTENTION:
		state = "failed"
	}
	payload := map[string]any{
		"state":          state,
		"pid":            sess.Pid,
		"startedAt":      sess.GetStartedAt().AsTime().UTC().Format(time.RFC3339Nano),
		"updatedAt":      sess.GetHeartbeatAt().AsTime().UTC().Format(time.RFC3339Nano),
		"installDir":     sess.InstallRoot,
		"stagedRoot":     sess.StagedRoot,
		"targetCode":     sess.TargetCode,
		"targetVersion":  sess.TargetVersion,
		"needsAttention": sess.Phase == updaterv1.SessionPhase_SESSION_PHASE_NEEDS_ATTENTION,
	}
	raw, _ := json.MarshalIndent(payload, "", "  ")
	return atomicWrite(filepath.Join(installRoot, JSONSessionName), append(raw, '\n'))
}

// RunWorker applies a previously accepted session. It must be started from staging.
func (e *Engine) RunWorker(dataDir, sessionID string) error {
	st := store{dataDir: dataDir}
	sess, err := st.loadSession(sessionID)
	if err != nil {
		return err
	}
	plan, err := st.loadPlan(sess.PlanId)
	if err != nil {
		return err
	}
	setPhase := func(p updaterv1.SessionPhase, errp *updaterv1.Error) {
		sess.Phase = p
		sess.HeartbeatAt = timestamppb.Now()
		sess.Error = errp
		_ = st.saveSession(sess)
		if sess.Layout == updaterv1.Layout_LAYOUT_VERSIONED_DIR {
			_ = writeLegacyJSONSession(sess.InstallRoot, sess)
		}
	}
	setPhase(updaterv1.SessionPhase_SESSION_PHASE_COPYING, nil)

	switch sess.Layout {
	case updaterv1.Layout_LAYOUT_VERSIONED_DIR:
		err = applyVersionedDir(sess, plan)
	case updaterv1.Layout_LAYOUT_WHOLE_ROOT:
		err = applyWholeRoot(sess, plan)
	case updaterv1.Layout_LAYOUT_FILE_SET:
		err = applyFileSet(sess, plan)
	default:
		err = fmt.Errorf("unsupported layout")
	}
	if err != nil {
		attn := strings.Contains(err.Error(), "INCOMPLETE")
		phase := updaterv1.SessionPhase_SESSION_PHASE_ROLLED_BACK
		if attn {
			phase = updaterv1.SessionPhase_SESSION_PHASE_NEEDS_ATTENTION
		}
		setPhase(phase, newError(updaterv1.ErrorCode_ERROR_CODE_DISK, false, err.Error(), nil))
		return err
	}
	if sess.Relaunch && sess.ExecutableRelpath != "" {
		setPhase(updaterv1.SessionPhase_SESSION_PHASE_RELAUNCHING, nil)
		exe := filepath.Join(sess.InstallRoot, filepath.FromSlash(sess.ExecutableRelpath))
		if sess.Layout == updaterv1.Layout_LAYOUT_VERSIONED_DIR {
			if ptr := readActive(sess.InstallRoot); ptr != nil && ptr.Executable != "" {
				exe = filepath.Join(sess.InstallRoot, filepath.FromSlash(ptr.Executable))
			} else if ptr != nil {
				exe = filepath.Join(sess.InstallRoot, filepath.FromSlash(ptr.Path), filepath.Base(sess.ExecutableRelpath))
			}
		}
		cmd := exec.Command(exe)
		cmd.Dir = sess.InstallRoot
		_ = cmd.Start()
	}
	setPhase(updaterv1.SessionPhase_SESSION_PHASE_COMPLETED, nil)
	return nil
}

type activePointer struct {
	Code       int64  `json:"code"`
	Version    string `json:"version"`
	Path       string `json:"path"`
	Executable string `json:"executable,omitempty"`
	Previous   string `json:"previous,omitempty"`
}

func readActive(root string) *activePointer {
	raw, err := os.ReadFile(filepath.Join(root, ActivePointer))
	if err != nil {
		return nil
	}
	var p activePointer
	if json.Unmarshal(raw, &p) != nil {
		return nil
	}
	return &p
}

func writeActive(root string, p activePointer) error {
	raw, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(filepath.Join(root, ActivePointer), append(raw, '\n'))
}

func applyVersionedDir(sess *updaterv1.ApplySessionRecord, plan *updaterv1.UpdatePlan) error {
	id := strings.ReplaceAll(plan.Version, "/", "_")
	if id == "" {
		id = fmt.Sprintf("code-%d", plan.Code)
	}
	dest := filepath.Join(sess.InstallRoot, "versions", id)
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	payload, err := unpackFirst(sess, plan)
	if err != nil {
		return err
	}
	if err := copyTree(payload, dest); err != nil {
		_ = os.RemoveAll(dest)
		return err
	}
	prev := readActive(sess.InstallRoot)
	ptr := activePointer{
		Code:       plan.Code,
		Version:    plan.Version,
		Path:       filepath.ToSlash(filepath.Join("versions", id)),
		Executable: filepath.ToSlash(filepath.Join("versions", id, sess.ExecutableRelpath)),
	}
	if prev != nil {
		ptr.Previous = prev.Path
	}
	if err := writeActive(sess.InstallRoot, ptr); err != nil {
		return err
	}
	retain := int(sess.Retain)
	if retain <= 0 {
		retain = 2
	}
	pruneVersions(sess.InstallRoot, ptr, retain)
	refreshSidecar(sess, payload)
	return nil
}

func pruneVersions(root string, current activePointer, retain int) {
	keep := map[string]struct{}{filepath.FromSlash(current.Path): {}}
	if current.Previous != "" && retain >= 2 {
		keep[filepath.FromSlash(current.Previous)] = struct{}{}
	}
	base := filepath.Join(root, "versions")
	ents, err := os.ReadDir(base)
	if err != nil {
		return
	}
	for _, e := range ents {
		p := filepath.Join("versions", e.Name())
		if _, ok := keep[p]; ok {
			continue
		}
		if retain <= 2 {
			_ = os.RemoveAll(filepath.Join(root, p))
		}
	}
}

func applyWholeRoot(sess *updaterv1.ApplySessionRecord, plan *updaterv1.UpdatePlan) error {
	payload, err := unpackFirst(sess, plan)
	if err != nil {
		return err
	}
	install := sess.InstallRoot
	backup := install + ".relkit-old"
	_ = os.RemoveAll(backup)
	if err := waitRename(install, backup, 60*time.Second); err != nil {
		return fmt.Errorf("occupied: %w", err)
	}
	if err := copyTree(payload, install); err != nil {
		_ = os.RemoveAll(install)
		if rerr := os.Rename(backup, install); rerr != nil {
			return fmt.Errorf("INCOMPLETE: restore failed: %v (original at %s)", rerr, backup)
		}
		return err
	}
	for _, rel := range sess.Preserve {
		src := filepath.Join(backup, filepath.FromSlash(rel))
		dst := filepath.Join(install, filepath.FromSlash(rel))
		if _, err := os.Stat(src); err == nil {
			_ = copyTree(src, dst)
		}
	}
	_ = os.RemoveAll(backup)
	return nil
}

func applyFileSet(sess *updaterv1.ApplySessionRecord, plan *updaterv1.UpdatePlan) error {
	journalPath := filepath.Join(sess.StagedRoot, "journal.pb")
	j := &updaterv1.ApplyJournal{SessionId: sess.SessionId}
	byName := map[string]*updaterv1.PlannedFile{}
	for _, f := range plan.Files {
		byName[f.Name] = f
	}
	for _, ent := range sess.FileSet {
		srcFile := byName[ent.ArtifactName]
		if srcFile == nil || srcFile.LocalPath == "" {
			rollbackJournal(j)
			return fmt.Errorf("missing artifact %s", ent.ArtifactName)
		}
		dest := filepath.Join(sess.InstallRoot, filepath.FromSlash(ent.DestRelpath))
		backup := dest + ".relkit-bak"
		_ = os.MkdirAll(filepath.Dir(dest), 0o755)
		_ = os.Remove(backup)
		if _, err := os.Stat(dest); err == nil {
			if err := os.Rename(dest, backup); err != nil {
				rollbackJournal(j)
				return err
			}
		}
		if err := copyFile(srcFile.LocalPath, dest); err != nil {
			rollbackJournal(j)
			if _, e2 := os.Stat(backup); e2 == nil {
				_ = os.Rename(backup, dest)
			}
			return err
		}
		_ = os.Chmod(dest, 0o755)
		j.Entries = append(j.Entries, &updaterv1.JournalEntry{
			DestPath:   dest,
			BackupPath: backup,
			SourcePath: srcFile.LocalPath,
		})
		_ = writeJournal(journalPath, j)
	}
	j.Committed = true
	_ = writeJournal(journalPath, j)
	for _, ent := range j.Entries {
		_ = os.Remove(ent.BackupPath)
	}
	return nil
}

func rollbackJournal(j *updaterv1.ApplyJournal) {
	for i := len(j.Entries) - 1; i >= 0; i-- {
		ent := j.Entries[i]
		_ = os.Remove(ent.DestPath)
		if ent.BackupPath != "" {
			_ = os.Rename(ent.BackupPath, ent.DestPath)
		}
	}
}

func unpackFirst(sess *updaterv1.ApplySessionRecord, plan *updaterv1.UpdatePlan) (string, error) {
	if len(plan.Files) == 0 {
		return "", fmt.Errorf("plan has no files")
	}
	src := plan.Files[0].LocalPath
	if strings.HasSuffix(strings.ToLower(src), ".zip") {
		out := filepath.Join(sess.StagedRoot, "unpacked")
		if err := unzip(src, out); err != nil {
			return "", err
		}
		if runtime.GOOS == "darwin" {
			if app := findAppBundle(out); app != "" {
				return app, nil
			}
		}
		return out, nil
	}
	return filepath.Dir(src), nil
}

func findAppBundle(root string) string {
	var found string
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil {
			return nil
		}
		if strings.HasSuffix(info.Name(), ".app") && info.IsDir() {
			found = path
			return io.EOF
		}
		return nil
	})
	return found
}

func unzip(src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	for _, f := range r.File {
		if err := ValidateArtifactFilename(f.Name); err != nil {
			if strings.HasSuffix(f.Name, "/") {
				continue
			}
			return err
		}
		path := filepath.Join(dest, filepath.FromSlash(f.Name))
		if !strings.HasPrefix(path, filepath.Clean(dest)+string(os.PathSeparator)) && path != filepath.Clean(dest) {
			return fmt.Errorf("zip path escapes")
		}
		if f.FileInfo().IsDir() {
			_ = os.MkdirAll(path, 0o755)
			continue
		}
		_ = os.MkdirAll(filepath.Dir(path), 0o755)
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, f.Mode())
		if err != nil {
			rc.Close()
			return err
		}
		_, err = io.Copy(out, rc)
		out.Close()
		rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func copyTree(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		return copyFile(path, target)
	})
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	_ = os.MkdirAll(filepath.Dir(dst), 0o755)
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	cerr := out.Close()
	if err != nil {
		return err
	}
	return cerr
}

func waitRename(from, to string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var last error
	for time.Now().Before(deadline) {
		last = os.Rename(from, to)
		if last == nil {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	if last == nil {
		last = fmt.Errorf("timeout")
	}
	return last
}

func refreshSidecar(sess *updaterv1.ApplySessionRecord, payload string) {
	if sess.SidecarRelpath == "" {
		return
	}
	src := filepath.Join(payload, filepath.FromSlash(sess.SidecarRelpath))
	if _, err := os.Stat(src); err != nil {
		src = filepath.Join(sess.StagedRoot, filepath.FromSlash(sess.SidecarRelpath))
	}
	if _, err := os.Stat(src); err != nil {
		return
	}
	dest := filepath.Join(sess.InstallRoot, "relkit-updater")
	if runtime.GOOS == "windows" {
		dest += ".exe"
	}
	aside := dest + ".old"
	_ = os.Rename(dest, aside)
	_ = copyFile(src, dest)
	_ = os.Chmod(dest, 0o755)
	_ = os.Remove(aside)
}

func writeJournal(path string, j *updaterv1.ApplyJournal) error {
	raw, err := proto.Marshal(j)
	if err != nil {
		return err
	}
	return atomicWrite(path, raw)
}
