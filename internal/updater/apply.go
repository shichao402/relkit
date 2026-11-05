package updater

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	enginestate "github.com/shichao402/relkit/api/updater/enginestate/v1"
	updaterv2 "github.com/shichao402/relkit/api/updater/v2"
	"github.com/shichao402/relkit/internal/sdk"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (e *Engine) handleApply(_ context.Context, req *updaterv2.UpdaterRequest, op *updaterv2.ApplyOp, st store) error {
	if op == nil || op.PlanId == "" {
		return e.emitApplyFail(updaterv2.ErrorCode_ERROR_CODE_PLAN_UNKNOWN, "planId required")
	}
	plan, err := st.loadPlan(op.PlanId)
	if err != nil {
		return e.emitApplyFail(updaterv2.ErrorCode_ERROR_CODE_PLAN_UNKNOWN, "unknown planId")
	}
	key, err := st.planKey()
	if err != nil || !verifyPlan(key, plan) {
		return e.emitApplyFail(updaterv2.ErrorCode_ERROR_CODE_PLAN_TAMPERED, "plan hmac mismatch")
	}
	if plan.ExpiresAt != nil && time.Now().After(plan.ExpiresAt.AsTime()) {
		return e.emitApplyFail(updaterv2.ErrorCode_ERROR_CODE_PLAN_EXPIRED, "plan expired")
	}
	// A plan need not be downloaded first: the worker fetches it after the
	// host exits (ADR 0026 §6). Files marked downloaded must still be intact.
	for _, f := range plan.Files {
		if f.Downloaded && !planFileReady(f) {
			return e.emitApplyFail(updaterv2.ErrorCode_ERROR_CODE_PLAN_TAMPERED, "downloaded file size mismatch")
		}
	}
	downloaded := planDownloaded(plan)
	if len(plan.Files) == 0 {
		return e.emitApplyFail(updaterv2.ErrorCode_ERROR_CODE_LAYOUT_UNSUPPORTED, "plan has no files")
	}
	inst := req.GetRuntime().GetInstall()
	if inst == nil {
		return e.emitApplyFail(updaterv2.ErrorCode_ERROR_CODE_LAYOUT_UNSUPPORTED, "install spec required")
	}
	disposition, installerRel, derr := classifyApply(plan)
	if derr != nil {
		return e.emitApplyFail(updaterv2.ErrorCode_ERROR_CODE_LAYOUT_UNSUPPORTED, derr.Error())
	}
	if disposition == updaterv2.ApplyDisposition_APPLY_DISPOSITION_FULL_INSTALL && fullInstallRefused(runtime.GOOS) {
		// macOS products ship the whole .app as a payload; there is no
		// installer handoff (requirement 3.11).
		return e.emitApplyFail(updaterv2.ErrorCode_ERROR_CODE_LAYOUT_UNSUPPORTED,
			"full-install disposition is not supported on macOS; ship the .app as a payload")
	}
	if inst.InstallRoot == "" {
		return e.emitApplyFail(updaterv2.ErrorCode_ERROR_CODE_PROFILE_INVALID, "installRoot required")
	}
	// The unit at install_root decides where control files go before the
	// payload is known (ADR 0027 §4.3); the table is cross-checked against it.
	onDisk := enginestate.ContentUnit_CONTENT_UNIT_TREE
	switch inst.Placement {
	case updaterv2.Placement_PLACEMENT_IN_PLACE:
		unit, uerr := detectInPlaceUnit(inst.InstallRoot)
		if uerr != nil {
			return e.emitApplyFail(applyErrorCode(uerr), uerr.Error())
		}
		onDisk = unit
		if onDisk == enginestate.ContentUnit_CONTENT_UNIT_BUNDLE {
			if err := checkUnitSupported(onDisk, false, hostGOOS); err != nil {
				return e.emitApplyFail(applyErrorCode(err), err.Error())
			}
			if len(inst.Preserve) > 0 {
				return e.emitApplyFail(updaterv2.ErrorCode_ERROR_CODE_LAYOUT_UNSUPPORTED,
					"a bundle replaced in place cannot preserve entries")
			}
		}
	case updaterv2.Placement_PLACEMENT_LIBRARY:
		if err := checkLibraryRoot(inst.InstallRoot, req.GetRuntime().GetDataDir(), hostGOOS); err != nil {
			return e.emitApplyFail(applyErrorCode(err), err.Error())
		}
	}
	if err := assertWritable(controlDirFor(inst.InstallRoot, inst.Placement, onDisk)); err != nil {
		return e.emitApplyFail(updaterv2.ErrorCode_ERROR_CODE_PERMISSION_DENIED, err.Error())
	}

	sessionID := newID("sess")
	now := timestamppb.Now()
	lockPath := applyLockPathFor(inst.InstallRoot, inst.Placement, onDisk)
	if err := acquireApplyLockAt(lockPath, sessionID); err != nil {
		return e.emitApplyFail(updaterv2.ErrorCode_ERROR_CODE_OCCUPIED, err.Error())
	}
	requiresHostExit := true
	// Without the payload on disk the engine cannot tell whether it touches
	// the running executable, so an undownloaded plan always asks the host
	// to exit.
	if disposition == updaterv2.ApplyDisposition_APPLY_DISPOSITION_INTERNAL && downloaded {
		ok, perr := planRequiresHostExit(inst, plan, onDisk)
		if perr != nil {
			releaseApplyLockAt(lockPath)
			code := updaterv2.ErrorCode_ERROR_CODE_PLAN_TAMPERED
			var f *applyFailure
			if errors.As(perr, &f) {
				code = f.code
			}
			return e.emitApplyFail(code, perr.Error())
		}
		requiresHostExit = ok
	}
	sess := &enginestate.ApplySessionRecord{
		SessionId:         sessionID,
		PlanId:            plan.PlanId,
		Phase:             updaterv2.SessionPhase_SESSION_PHASE_WAITING_FOR_EXIT,
		StartedAt:         now,
		HeartbeatAt:       now,
		InstallRoot:       inst.InstallRoot,
		StagedRoot:        st.planStagingDir(plan.PlanId),
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
		Disposition:       disposition,
		InstallerRelpath:  installerRel,
		HideProgress:      op.HideProgress,
		DisplayName:       displayNameFor(req),
		Recovery:          req.GetProfile().GetRecovery(),
		Product:           req.GetProfile().GetExpectedProduct(),
		Unit:              onDisk,
	}
	if err := st.saveSession(sess); err != nil {
		releaseApplyLockAt(lockPath)
		return e.emitApplyFail(updaterv2.ErrorCode_ERROR_CODE_DISK, err.Error())
	}
	launch := e.LaunchWorker
	if launch == nil {
		launch = launchApplyWorker
	}
	if err := launch(req.GetRuntime().GetDataDir(), sessionID, sess.StagedRoot); err != nil {
		sess.Phase = updaterv2.SessionPhase_SESSION_PHASE_NEEDS_ATTENTION
		sess.Error = newError(updaterv2.ErrorCode_ERROR_CODE_DISK, true, err.Error(), nil)
		sess.HeartbeatAt = timestamppb.Now()
		_ = st.saveSession(sess)
		releaseApplyLockAt(lockPath)
		return e.emitApplyFail(updaterv2.ErrorCode_ERROR_CODE_DISK, "failed to start apply worker: "+err.Error())
	}
	return e.emit(&updaterv2.UpdaterEvent{
		Kind: &updaterv2.UpdaterEvent_Apply{Apply: &updaterv2.ApplyResult{
			Kind: &updaterv2.ApplyResult_Accepted{Accepted: &updaterv2.ApplyAccepted{
				SessionId:        sessionID,
				PlanId:           plan.PlanId,
				RequiresHostExit: requiresHostExit,
			}},
		}},
	})
}

// displayNameFor is the product name the progress window shows: the host's
// InstallSpec.display_name, else the product id the host declared.
func displayNameFor(req *updaterv2.UpdaterRequest) string {
	if n := strings.TrimSpace(req.GetRuntime().GetInstall().GetDisplayName()); n != "" {
		return n
	}
	return req.GetProfile().GetExpectedProduct()
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

func (e *Engine) emitApplyFail(code updaterv2.ErrorCode, message string) error {
	return e.emit(&updaterv2.UpdaterEvent{
		Kind: &updaterv2.UpdaterEvent_Apply{Apply: &updaterv2.ApplyResult{
			Kind: &updaterv2.ApplyResult_Failed{Failed: &updaterv2.Failed{
				Error: newError(code, false, message, nil),
			}},
		}},
	})
}

// fullInstallRefused is the platform rule for installer handoff, kept as a pure
// function so the rule itself is testable on any OS (requirement 3.11).
//
// macOS products update by replacing the whole .app bundle in place, so an
// unsigned installer is never the right answer there. Every other desktop OS
// accepts the handoff.
func fullInstallRefused(goos string) bool {
	return goos == "darwin"
}

// classifyApply decides which of the two install styles a plan needs.
//
// payload  -> the engine unpacks the signed payload itself (IN_PLACE/LIBRARY)
// anything else -> the engine hands over to the product's own installer
//
// The engine never guesses: a plan whose single artifact is neither a payload
// nor a runnable installer is rejected by the caller.
func classifyApply(plan *enginestate.UpdatePlan) (upderv1Disposition updaterv2.ApplyDisposition, installerRel string, err error) {
	if plan == nil || len(plan.Files) == 0 {
		return updaterv2.ApplyDisposition_APPLY_DISPOSITION_UNSPECIFIED, "", fmt.Errorf("plan has no files")
	}
	first := plan.Files[0]
	if first.Kind == "payload" {
		return updaterv2.ApplyDisposition_APPLY_DISPOSITION_INTERNAL, "", nil
	}
	if first.Kind != "installer" {
		return updaterv2.ApplyDisposition_APPLY_DISPOSITION_UNSPECIFIED, "",
			fmt.Errorf("artifact kind %q is neither a payload nor an installer", first.Kind)
	}
	rel := first.DestRelpath
	if rel == "" {
		rel = first.Name
	}
	if err := ValidateArtifactFilename(rel); err != nil {
		return updaterv2.ApplyDisposition_APPLY_DISPOSITION_UNSPECIFIED, "", err
	}
	return updaterv2.ApplyDisposition_APPLY_DISPOSITION_FULL_INSTALL, rel, nil
}

// installerCommand builds the process that runs the verified installer.
//
// It is a package variable only so tests can substitute a stub process and
// exercise the real exit-code path; production always uses the installer file
// itself, interactively, with no silent flags and no elevation.
var installerCommand = func(installer string) *exec.Cmd {
	cmd := exec.Command(installer)
	cmd.Dir = filepath.Dir(installer)
	// Interactive: the installer draws its own UI and needs a console.
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd
}

// runFullInstall performs the handoff the hosts used to implement themselves
// (requirement 3.10): wait for the host to exit, run the verified installer
// interactively (no silent flags, no elevation — the OS raises UAC if the
// installer needs it), then judge the result by exit code and relaunch.
func runFullInstall(sess *enginestate.ApplySessionRecord, installer string) error {
	if _, err := os.Stat(installer); err != nil {
		return fmt.Errorf("installer missing from staging: %w", err)
	}
	err := installerCommand(installer).Run()
	if err == nil {
		return nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		code := exitErr.ExitCode()
		if isUserCancelledExit(code) {
			return fmt.Errorf("%w (exit code %d)", errInstallerCancelled, code)
		}
		return fmt.Errorf("%w (exit code %d)", errInstallerFailed, code)
	}
	return err
}

var (
	errInstallerCancelled = errors.New("installer was cancelled by the user")
	errInstallerFailed    = errors.New("installer exited with a failure code")
)

// isUserCancelledExit maps the exit codes the supported installers use for
// "the user closed the wizard". Inno Setup: 1 = user cancelled before install.
// NSIS: 2 = user aborted. Anything else is a real failure.
func isUserCancelledExit(code int) bool {
	return code == 1 || code == 2
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

// RunWorker applies a previously accepted session. It must be started from staging.
//
// While the host is gone it drives the progress window (ADR 0026 §6): wait
// for the host to exit, download the plan if apply was accepted before
// download, then install and relaunch. Waiting and downloading can be
// cancelled; once copying starts they cannot. When the session ends with the
// old install intact (cancelled, download failed, rolled back) the worker
// relaunches the old host so the user is not left with nothing running.
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
	lockPath := sessionLockPath(sess)
	refreshApplyLockAt(lockPath, sess.SessionId, os.Getpid())

	ui := e.openProgressUI(sess)
	defer ui.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		select {
		case <-ui.Cancelled():
			cancel()
		case <-ctx.Done():
		}
	}()

	setPhase := func(p updaterv2.SessionPhase, errp *updaterv2.Error) {
		if errp != nil && errp.Recovery == nil {
			errp.Recovery = sess.Recovery
		}
		sess.Phase = p
		sess.HeartbeatAt = timestamppb.Now()
		sess.Error = errp
		_ = st.saveSession(sess)
	}
	enter := func(p updaterv2.SessionPhase) {
		setPhase(p, nil)
		ui.SetPhase(p, phaseCancelable(p))
	}
	// finish records a terminal failure, shows it, and relaunches the old
	// host when the install is untouched and the host is gone.
	finish := func(p updaterv2.SessionPhase, errp *updaterv2.Error, cause error, relaunchOld bool) error {
		setPhase(p, errp)
		releaseApplyLockAt(lockPath)
		if errp.GetCode() == updaterv2.ErrorCode_ERROR_CODE_CANCELED {
			ui.Close()
		} else {
			ui.Fail(errp)
		}
		if relaunchOld && sess.RequiresHostExit {
			relaunchHost(sess)
		}
		return cause
	}

	if sess.RequiresHostExit {
		ui.SetPhase(updaterv2.SessionPhase_SESSION_PHASE_WAITING_FOR_EXIT, true)
		if err := waitForHostExit(ctx, int(sess.Pid), 5*time.Minute, func() {
			setPhase(updaterv2.SessionPhase_SESSION_PHASE_WAITING_FOR_EXIT, nil)
		}); err != nil {
			if ctx.Err() != nil {
				// The host may still be up; nothing to relaunch.
				return finish(updaterv2.SessionPhase_SESSION_PHASE_NEEDS_ATTENTION,
					newError(updaterv2.ErrorCode_ERROR_CODE_CANCELED, true, "cancelled while waiting for the host to exit", nil),
					err, false)
			}
			return finish(updaterv2.SessionPhase_SESSION_PHASE_NEEDS_ATTENTION,
				newError(updaterv2.ErrorCode_ERROR_CODE_OCCUPIED, true, err.Error(), nil), err, false)
		}
	}
	if !planDownloaded(plan) {
		enter(updaterv2.SessionPhase_SESSION_PHASE_DOWNLOADING)
		var totalSize int64
		received := make([]int64, len(plan.Files))
		for i, f := range plan.Files {
			totalSize += f.Size
			if planFileReady(f) {
				received[i] = f.Size
			}
		}
		if _, derr := e.downloadPlanFiles(ctx, st, plan, sess.StagedRoot, func(i int, p sdk.Progress) {
			received[i] = p.Received
			var sum int64
			for _, r := range received {
				sum += r
			}
			ui.SetDownload(sum, totalSize, p.BytesPerSecond)
		}); derr != nil {
			return finish(updaterv2.SessionPhase_SESSION_PHASE_NEEDS_ATTENTION, derr, errors.New(derr.GetMessage()), true)
		}
	}
	if ctx.Err() != nil {
		return finish(updaterv2.SessionPhase_SESSION_PHASE_NEEDS_ATTENTION,
			newError(updaterv2.ErrorCode_ERROR_CODE_CANCELED, true, "cancelled before install", nil), ctx.Err(), true)
	}
	if sess.Disposition == updaterv2.ApplyDisposition_APPLY_DISPOSITION_FULL_INSTALL {
		// The installer draws its own UI from here on.
		ui.Close()
		return runFullInstallPhase(sess, st, setPhase)
	}
	enter(updaterv2.SessionPhase_SESSION_PHASE_COPYING)

	err = applyPlan(dataDir, sess, plan)
	if err != nil {
		attn := strings.Contains(err.Error(), "INCOMPLETE")
		phase := updaterv2.SessionPhase_SESSION_PHASE_ROLLED_BACK
		if attn {
			phase = updaterv2.SessionPhase_SESSION_PHASE_NEEDS_ATTENTION
		}
		return finish(phase, newError(applyErrorCode(err), false, err.Error(), nil), err, !attn)
	}
	if sess.Relaunch && sess.ExecutableRelpath != "" && !sess.InstallOnly {
		enter(updaterv2.SessionPhase_SESSION_PHASE_RELAUNCHING)
		relaunchHost(sess)
	}
	setPhase(updaterv2.SessionPhase_SESSION_PHASE_COMPLETED, nil)
	releaseApplyLockAt(lockPath)
	return nil
}

// phaseCancelable is the cancel rule: only the stretches that change nothing
// on disk may be cancelled.
func phaseCancelable(p updaterv2.SessionPhase) bool {
	return p == updaterv2.SessionPhase_SESSION_PHASE_WAITING_FOR_EXIT ||
		p == updaterv2.SessionPhase_SESSION_PHASE_DOWNLOADING
}

// relaunchHost starts the host the session names, resolving the active
// version for LIBRARY installs. On macOS a bundle is opened through
// LaunchServices (`open -n`), so the updater does not become the responsible
// process of the new host (ADR 0027 §5). It is a no-op unless the session
// asked for relaunch.
var relaunchHost = func(sess *enginestate.ApplySessionRecord) {
	if !sess.Relaunch || sess.ExecutableRelpath == "" || sess.InstallOnly {
		return
	}
	exe, bundle := relaunchTarget(sess)
	var cmd *exec.Cmd
	if bundle != "" && runtime.GOOS == "darwin" {
		cmd = exec.Command("open", "-n", bundle)
	} else {
		cmd = exec.Command(exe)
	}
	cmd.Dir = sess.InstallRoot
	_ = cmd.Start()
}

// relaunchTarget is the executable to start and, when the content is a
// bundle, the bundle that holds it: install_root for IN_PLACE, the active
// version's bundle for LIBRARY.
func relaunchTarget(sess *enginestate.ApplySessionRecord) (exe, bundle string) {
	root := sess.InstallRoot
	exe = filepath.Join(root, filepath.FromSlash(sess.ExecutableRelpath))
	if sess.Placement == updaterv2.Placement_PLACEMENT_LIBRARY {
		ptr := readActive(root)
		if ptr == nil {
			return exe, ""
		}
		if ptr.Executable != "" {
			exe = filepath.Join(root, filepath.FromSlash(ptr.Executable))
		} else {
			exe = filepath.Join(root, filepath.FromSlash(ptr.Path), filepath.Base(sess.ExecutableRelpath))
		}
		if ptr.Bundle != "" {
			bundle = filepath.Join(root, filepath.FromSlash(ptr.Path), ptr.Bundle)
		}
		return exe, bundle
	}
	if sess.Unit == enginestate.ContentUnit_CONTENT_UNIT_BUNDLE {
		bundle = filepath.Clean(root)
	}
	return exe, bundle
}

// runFullInstallPhase runs the verified installer and reports the outcome in
// the session, which is what the next status call reads (requirement 3.10).
func runFullInstallPhase(
	sess *enginestate.ApplySessionRecord,
	st store,
	setPhase func(updaterv2.SessionPhase, *updaterv2.Error),
) error {
	defer releaseApplyLockAt(sessionLockPath(sess))
	installer := filepath.Join(sess.StagedRoot, filepath.FromSlash(sess.InstallerRelpath))
	setPhase(updaterv2.SessionPhase_SESSION_PHASE_COMMITTING, nil)
	if err := runFullInstall(sess, installer); err != nil {
		code := updaterv2.ErrorCode_ERROR_CODE_INSTALLER_FAILED
		retry := false
		if errors.Is(err, errInstallerCancelled) {
			code = updaterv2.ErrorCode_ERROR_CODE_CANCELED
			retry = true
		}
		setPhase(updaterv2.SessionPhase_SESSION_PHASE_NEEDS_ATTENTION, newError(code, retry, err.Error(), nil))
		return err
	}
	if sess.Relaunch && sess.ExecutableRelpath != "" && !sess.InstallOnly {
		setPhase(updaterv2.SessionPhase_SESSION_PHASE_RELAUNCHING, nil)
		relaunchHost(sess)
	}
	setPhase(updaterv2.SessionPhase_SESSION_PHASE_COMPLETED, nil)
	return nil
}

func waitForHostExit(ctx context.Context, pid int, timeout time.Duration, heartbeat func()) error {
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
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	return nil
}

type activePointer struct {
	Code       int64  `json:"code"`
	Version    string `json:"version"`
	Path       string `json:"path"`
	Executable string `json:"executable,omitempty"`
	// Bundle names the .app inside Path when the content is a bundle.
	Bundle   string `json:"bundle,omitempty"`
	Previous string `json:"previous,omitempty"`
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
	Bundle     string `json:"bundle,omitempty"`
}

// pruneVersions trims versions/ after an install (ADR 0027 §4.4). It keeps
// the active and previous versions, the version just installed, reserved
// codes, then the newest by code up to retain. A version a running process
// executes from is never removed, whatever the budget says. Leftover partial
// directories from interrupted fresh writes are removed; the caller holds the
// apply lock, so none of them belongs to a live session.
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
	base := filepath.Join(root, VersionsDirName)
	ents, err := os.ReadDir(base)
	if err != nil {
		return
	}
	type item struct {
		rel  string
		code int64
	}
	var all []item
	for _, e := range ents {
		if strings.HasPrefix(e.Name(), ".") {
			if strings.HasSuffix(e.Name(), partialSuffix) {
				_ = os.RemoveAll(filepath.Join(base, e.Name()))
			}
			continue
		}
		if !e.IsDir() {
			continue
		}
		rel := path.Join(VersionsDirName, e.Name())
		code := installedCode(root, e.Name())
		all = append(all, item{rel: rel, code: code})
		if _, ok := reservedSet[code]; ok && code != 0 {
			keep[rel] = struct{}{}
		}
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].code > all[j].code })
	for _, it := range all {
		if len(keep) >= retain {
			break
		}
		keep[it.rel] = struct{}{}
	}
	var inUse map[string]struct{}
	for _, it := range all {
		if _, ok := keep[it.rel]; ok {
			continue
		}
		if inUse == nil {
			inUse = versionsInUse(root)
		}
		if _, busy := inUse[path.Base(it.rel)]; busy {
			continue
		}
		_ = moveToTrash(root, filepath.Join(root, filepath.FromSlash(it.rel)))
	}
}

// installedCode is the code of versions/<name>: its metadata, else the active
// pointer naming it, else a code-N directory name, else 0.
func installedCode(root, name string) int64 {
	dir := filepath.Join(root, VersionsDirName, name)
	if m := readVersionMeta(dir); m != nil {
		return m.Code
	}
	if a := readActive(root); a != nil && filepath.ToSlash(a.Path) == path.Join(VersionsDirName, name) {
		return a.Code
	}
	if strings.HasPrefix(name, "code-") {
		if n, err := strconv.ParseInt(strings.TrimPrefix(name, "code-"), 10, 64); err == nil {
			return n
		}
	}
	return 0
}

func rollbackJournal(j *enginestate.ApplyJournal) {
	for i := len(j.Entries) - 1; i >= 0; i-- {
		ent := j.Entries[i]
		// A real directory at dest was created by this apply where a
		// displaced symlink or file stood; everything in it is new.
		if info, err := os.Lstat(ent.DestPath); err == nil && info.IsDir() && ent.BackupPath != "" {
			_ = os.RemoveAll(ent.DestPath)
		} else {
			_ = os.Remove(ent.DestPath)
		}
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

func writeJournal(path string, j *enginestate.ApplyJournal) error {
	raw, err := proto.Marshal(j)
	if err != nil {
		return err
	}
	return atomicWrite(path, raw)
}
