package updater

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	updaterv1 "go.firoyang.com/relkit/api/updater/v1"
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
	if len(plan.Files) == 0 || plan.Files[0].Kind != "payload" {
		return e.emitApplyFail(updaterv1.ErrorCode_ERROR_CODE_LAYOUT_UNSUPPORTED, "artifact requires the full installation flow")
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
	if err := acquireApplyLock(inst.InstallRoot, sessionID); err != nil {
		return e.emitApplyFail(updaterv1.ErrorCode_ERROR_CODE_OCCUPIED, err.Error())
	}
	requiresHostExit, err := planRequiresHostExit(inst, plan)
	if err != nil {
		releaseApplyLock(inst.InstallRoot)
		return e.emitApplyFail(updaterv1.ErrorCode_ERROR_CODE_PLAN_TAMPERED, err.Error())
	}
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
		Pid:               int32(os.Getppid()),
		Placement:         inst.Placement,
		Relaunch:          inst.Relaunch && !op.InstallOnly,
		ExecutableRelpath: inst.ExecutableRelpath,
		Preserve:          inst.Preserve,
		SidecarRelpath:    inst.SidecarRelpath,
		InstallOnly:       op.InstallOnly,
		Library:           inst.Library,
		RequiresHostExit:  requiresHostExit,
	}
	if err := st.saveSession(sess); err != nil {
		releaseApplyLock(inst.InstallRoot)
		return e.emitApplyFail(updaterv1.ErrorCode_ERROR_CODE_DISK, err.Error())
	}
	_ = writeCompatibilitySession(req.GetRuntime().GetDataDir(), sess)
	launch := e.LaunchWorker
	if launch == nil {
		launch = launchApplyWorker
	}
	if err := launch(req.GetRuntime().GetDataDir(), sessionID, sess.StagedRoot); err != nil {
		sess.Phase = updaterv1.SessionPhase_SESSION_PHASE_NEEDS_ATTENTION
		sess.Error = newError(updaterv1.ErrorCode_ERROR_CODE_DISK, true, err.Error(), nil)
		sess.HeartbeatAt = timestamppb.Now()
		_ = st.saveSession(sess)
		_ = writeCompatibilitySession(req.GetRuntime().GetDataDir(), sess)
		releaseApplyLock(inst.InstallRoot)
		return e.emitApplyFail(updaterv1.ErrorCode_ERROR_CODE_DISK, "failed to start apply worker: "+err.Error())
	}
	return e.emit(&updaterv1.UpdaterEvent{
		Kind: &updaterv1.UpdaterEvent_Apply{Apply: &updaterv1.ApplyResult{
			Kind: &updaterv1.ApplyResult_Accepted{Accepted: &updaterv1.ApplyAccepted{
				SessionId:        sessionID,
				PlanId:           plan.PlanId,
				RequiresHostExit: requiresHostExit,
			}},
		}},
	})
}

func launchApplyWorker(dataDir, sessionID, stagedRoot string) error {
	current, err := os.Executable()
	if err != nil {
		return err
	}
	workerDir := filepath.Join(stagedRoot, ".worker")
	name := "relkit-updater"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	worker := filepath.Join(workerDir, name)
	if err := copyFile(current, worker); err != nil {
		return err
	}
	if err := os.Chmod(worker, 0o755); err != nil {
		return err
	}
	cmd := exec.Command(worker, "--worker", sessionID, "--data-dir", dataDir)
	cmd.Dir = workerDir
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
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

func writeCompatibilitySession(dataDir string, sess *updaterv1.ApplySessionRecord) error {
	root := dataDir
	if sess.Placement == updaterv1.Placement_PLACEMENT_LIBRARY {
		root = sess.InstallRoot
	}
	return writeLegacyJSONSession(root, sess)
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
	refreshApplyLock(sess.InstallRoot, sess.SessionId, os.Getpid())
	setPhase := func(p updaterv1.SessionPhase, errp *updaterv1.Error) {
		sess.Phase = p
		sess.HeartbeatAt = timestamppb.Now()
		sess.Error = errp
		_ = st.saveSession(sess)
		_ = writeCompatibilitySession(dataDir, sess)
	}
	if sess.RequiresHostExit {
		if err := waitForHostExit(int(sess.Pid), 5*time.Minute, func() {
			setPhase(updaterv1.SessionPhase_SESSION_PHASE_WAITING_FOR_EXIT, nil)
		}); err != nil {
			setPhase(
				updaterv1.SessionPhase_SESSION_PHASE_NEEDS_ATTENTION,
				newError(updaterv1.ErrorCode_ERROR_CODE_OCCUPIED, true, err.Error(), nil),
			)
			releaseApplyLock(sess.InstallRoot)
			return err
		}
	}
	setPhase(updaterv1.SessionPhase_SESSION_PHASE_COPYING, nil)

	err = applyPlan(dataDir, sess, plan)
	if err != nil {
		attn := strings.Contains(err.Error(), "INCOMPLETE")
		phase := updaterv1.SessionPhase_SESSION_PHASE_ROLLED_BACK
		if attn {
			phase = updaterv1.SessionPhase_SESSION_PHASE_NEEDS_ATTENTION
		}
		setPhase(phase, newError(updaterv1.ErrorCode_ERROR_CODE_DISK, false, err.Error(), nil))
		releaseApplyLock(sess.InstallRoot)
		return err
	}
	if sess.Relaunch && sess.ExecutableRelpath != "" && !sess.InstallOnly {
		setPhase(updaterv1.SessionPhase_SESSION_PHASE_RELAUNCHING, nil)
		exe := filepath.Join(sess.InstallRoot, filepath.FromSlash(sess.ExecutableRelpath))
		if sess.Placement == updaterv1.Placement_PLACEMENT_LIBRARY {
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
	releaseApplyLock(sess.InstallRoot)
	return nil
}

func waitForHostExit(pid int, timeout time.Duration, heartbeat func()) error {
	if pid <= 0 {
		return nil
	}
	deadline := time.Now().Add(timeout)
	for processAlive(pid) {
		if time.Now().After(deadline) {
			return fmt.Errorf("host process %d did not exit within %s", pid, timeout)
		}
		if heartbeat != nil {
			heartbeat()
		}
		time.Sleep(500 * time.Millisecond)
	}
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

func writeVersionMeta(versionDir string, meta versionMeta) error {
	raw, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(filepath.Join(versionDir, VersionMetaName), append(raw, '\n'))
}

func readVersionMeta(versionDir string) *versionMeta {
	raw, err := os.ReadFile(filepath.Join(versionDir, VersionMetaName))
	if err != nil {
		return nil
	}
	var m versionMeta
	if json.Unmarshal(raw, &m) != nil {
		return nil
	}
	return &m
}

type versionMeta struct {
	Code       int64  `json:"code"`
	Version    string `json:"version"`
	Path       string `json:"path"`
	Executable string `json:"executable,omitempty"`
}

func pruneVersions(root string, current activePointer, retain int, reserved []int64, extraKeep string) {
	if retain <= 0 {
		retain = 2
	}
	keep := map[string]struct{}{}
	if current.Path != "" {
		keep[filepath.ToSlash(current.Path)] = struct{}{}
	}
	if extraKeep != "" {
		keep[filepath.ToSlash(extraKeep)] = struct{}{}
	}
	if current.Previous != "" && retain >= 2 {
		keep[filepath.ToSlash(current.Previous)] = struct{}{}
	}
	reservedSet := map[int64]struct{}{}
	for _, c := range reserved {
		if c != 0 {
			reservedSet[c] = struct{}{}
		}
	}
	base := filepath.Join(root, "versions")
	ents, err := os.ReadDir(base)
	if err != nil {
		return
	}
	type item struct {
		rel  string
		name string
		code int64
	}
	var all []item
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		rel := filepath.ToSlash(filepath.Join("versions", e.Name()))
		code := int64(0)
		if m := readVersionMeta(filepath.Join(root, filepath.FromSlash(rel))); m != nil {
			code = m.Code
		}
		all = append(all, item{rel: rel, name: e.Name(), code: code})
		if _, ok := reservedSet[code]; ok {
			keep[rel] = struct{}{}
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].name > all[j].name })
	for _, it := range all {
		if len(keep) >= retain {
			break
		}
		keep[it.rel] = struct{}{}
	}
	for _, it := range all {
		if _, ok := keep[it.rel]; ok {
			continue
		}
		_ = os.RemoveAll(filepath.Join(root, filepath.FromSlash(it.rel)))
	}
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

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	info, _ := in.Stat()
	mode := os.FileMode(0o644)
	if info != nil {
		if perm := info.Mode() & os.ModePerm; perm != 0 {
			mode = perm
		}
	}
	_ = os.MkdirAll(filepath.Dir(dst), 0o755)
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
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
