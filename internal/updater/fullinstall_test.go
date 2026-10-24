package updater

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	updaterv1 "github.com/shichao402/relkit/api/updater/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// exitCommand returns a process that exits with the given status, using
// whatever shell the current OS provides. The installer itself is never
// executed by these tests; only the exit-code path is under test.
func exitCommand(code int) *exec.Cmd {
	if runtime.GOOS == "windows" {
		return exec.Command("cmd", "/c", "exit", itoa(int64(code)))
	}
	return exec.Command("sh", "-c", "exit "+itoa(int64(code)))
}

// stubInstaller swaps the installer process for the duration of one test.
func stubInstaller(t *testing.T, cmd *exec.Cmd) {
	t.Helper()
	original := installerCommand
	installerCommand = func(string) *exec.Cmd { return cmd }
	t.Cleanup(func() { installerCommand = original })
}

// fullInstallFixture stages a fake installer and returns the session plus a
// store that persists phase changes, mirroring what the worker does.
func fullInstallFixture(t *testing.T, installerName string) (*updaterv1.ApplySessionRecord, store) {
	t.Helper()
	dataDir := t.TempDir()
	root := t.TempDir()
	stagedRoot := filepath.Join(dataDir, "staging", "plan-full")
	if err := os.MkdirAll(stagedRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	installer := filepath.Join(stagedRoot, installerName)
	if err := os.WriteFile(installer, []byte("stub installer"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := acquireApplyLock(root, "sess-full"); err != nil {
		t.Fatal(err)
	}
	sess := &updaterv1.ApplySessionRecord{
		SessionId:        "sess-full",
		PlanId:           "plan-full",
		Phase:            updaterv1.SessionPhase_SESSION_PHASE_WAITING_FOR_EXIT,
		StartedAt:        timestamppb.Now(),
		InstallRoot:      root,
		StagedRoot:       stagedRoot,
		Disposition:      updaterv1.ApplyDisposition_APPLY_DISPOSITION_FULL_INSTALL,
		InstallerRelpath: installerName,
	}
	return sess, store{dataDir: dataDir}
}

// phaseRecorder returns the setPhase callback the worker uses, plus a pointer to
// the persisted session so the assertion reads what the next status call reads.
func phaseRecorder(t *testing.T, sess *updaterv1.ApplySessionRecord, st store) func(updaterv1.SessionPhase, *updaterv1.Error) {
	t.Helper()
	return func(p updaterv1.SessionPhase, errp *updaterv1.Error) {
		sess.Phase = p
		sess.HeartbeatAt = timestamppb.Now()
		sess.Error = errp
		if err := st.saveSession(sess); err != nil {
			t.Fatal(err)
		}
	}
}

func assertLockReleased(t *testing.T, root string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(root, ApplyLockName)); !os.IsNotExist(err) {
		t.Fatal("apply lock survived the full-install phase")
	}
}

func TestRunFullInstallPhaseCompletesOnZeroExit(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("macOS refuses FULL_INSTALL (requirement 3.11)")
	}
	sess, st := fullInstallFixture(t, "setup.exe")
	stubInstaller(t, exitCommand(0))
	if err := runFullInstallPhase(sess, st, phaseRecorder(t, sess, st)); err != nil {
		t.Fatalf("runFullInstallPhase: %v", err)
	}
	if sess.Phase != updaterv1.SessionPhase_SESSION_PHASE_COMPLETED {
		t.Fatalf("phase = %v, want COMPLETED", sess.Phase)
	}
	if sess.Error != nil {
		t.Fatalf("successful handoff recorded an error: %v", sess.Error)
	}
	persisted, err := st.loadSession(sess.SessionId)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Phase != updaterv1.SessionPhase_SESSION_PHASE_COMPLETED {
		t.Fatalf("persisted phase = %v", persisted.Phase)
	}
	assertLockReleased(t, sess.InstallRoot)
}

func TestRunFullInstallPhaseReportsFailureExit(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("macOS refuses FULL_INSTALL (requirement 3.11)")
	}
	sess, st := fullInstallFixture(t, "setup.exe")
	stubInstaller(t, exitCommand(5))
	err := runFullInstallPhase(sess, st, phaseRecorder(t, sess, st))
	if err == nil {
		t.Fatal("non-zero installer exit must surface as an error")
	}
	if sess.Phase != updaterv1.SessionPhase_SESSION_PHASE_NEEDS_ATTENTION {
		t.Fatalf("phase = %v, want NEEDS_ATTENTION", sess.Phase)
	}
	if sess.Error.GetCode() != updaterv1.ErrorCode_ERROR_CODE_INSTALLER_FAILED {
		t.Fatalf("error code = %v, want INSTALLER_FAILED", sess.Error.GetCode())
	}
	if sess.Error.GetRetryable() {
		t.Fatal("a failed installer is not retryable automatically")
	}
	persisted, err := st.loadSession(sess.SessionId)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Error.GetCode() != updaterv1.ErrorCode_ERROR_CODE_INSTALLER_FAILED {
		t.Fatalf("persisted error code = %v", persisted.Error.GetCode())
	}
	assertLockReleased(t, sess.InstallRoot)
}

func TestRunFullInstallPhaseTreatsWizardCancelAsCanceled(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("macOS refuses FULL_INSTALL (requirement 3.11)")
	}
	for _, code := range []int{1, 2} {
		sess, st := fullInstallFixture(t, "setup.exe")
		stubInstaller(t, exitCommand(code))
		err := runFullInstallPhase(sess, st, phaseRecorder(t, sess, st))
		if err == nil {
			t.Fatalf("exit %d must surface as an error", code)
		}
		if sess.Phase != updaterv1.SessionPhase_SESSION_PHASE_NEEDS_ATTENTION {
			t.Fatalf("exit %d phase = %v", code, sess.Phase)
		}
		if sess.Error.GetCode() != updaterv1.ErrorCode_ERROR_CODE_CANCELED {
			t.Fatalf("exit %d error code = %v, want CANCELED", code, sess.Error.GetCode())
		}
		if !sess.Error.GetRetryable() {
			t.Fatalf("exit %d: a user cancel is retryable", code)
		}
		assertLockReleased(t, sess.InstallRoot)
	}
}

// The default command must be the installer itself, run from its own directory
// so relative resources inside the staging folder resolve (requirement 3.10).
func TestInstallerCommandRunsInstallerInPlace(t *testing.T) {
	cmd := installerCommand(filepath.Join("staging", "plan", "setup.exe"))
	if cmd == nil || len(cmd.Args) == 0 {
		t.Fatal("no command built")
	}
	if cmd.Args[0] != filepath.Join("staging", "plan", "setup.exe") {
		t.Fatalf("argv[0] = %q, want the installer path", cmd.Args[0])
	}
	if len(cmd.Args) != 1 {
		t.Fatalf("installer must be launched without flags, got %v", cmd.Args[1:])
	}
	if cmd.Dir != filepath.Join("staging", "plan") {
		t.Fatalf("working directory = %q, want the installer directory", cmd.Dir)
	}
}
