package updater

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	enginestate "github.com/shichao402/relkit/api/updater/enginestate/v1"
)

type applyLockFile struct {
	SessionID string `json:"sessionId"`
	Pid       int    `json:"pid"`
}

// applyLockPath is the lock of an install root that holds its own control
// files: every layout except BUNDLE/IN_PLACE (see applyLockPathFor).
func applyLockPath(root string) string {
	return filepath.Join(root, ApplyLockName)
}

// sessionLockPath is the lock a session took when it was accepted. The unit
// recorded at acceptance decides it, so worker and status agree on one file.
func sessionLockPath(sess *enginestate.ApplySessionRecord) string {
	return applyLockPathFor(sess.GetInstallRoot(), sess.GetPlacement(), sess.GetUnit())
}

func acquireApplyLock(root, sessionID string) error {
	return acquireApplyLockAt(applyLockPath(root), sessionID)
}

func releaseApplyLock(root string) {
	releaseApplyLockAt(applyLockPath(root))
}

func acquireApplyLockAt(path, sessionID string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	payload, err := json.Marshal(applyLockFile{SessionID: sessionID, Pid: os.Getpid()})
	if err != nil {
		return err
	}
	payload = append(payload, '\n')
	for i := 0; i < 12; i++ {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			_, werr := f.Write(payload)
			_ = f.Close()
			return werr
		}
		raw, rerr := os.ReadFile(path)
		if rerr != nil {
			time.Sleep(40 * time.Millisecond)
			continue
		}
		var existing applyLockFile
		if json.Unmarshal(raw, &existing) == nil && existing.Pid > 0 && processAlive(existing.Pid) {
			return fmt.Errorf("apply already running (pid %d)", existing.Pid)
		}
		_ = os.Remove(path)
	}
	return fmt.Errorf("could not acquire apply lock")
}

func refreshApplyLockAt(path, sessionID string, pid int) {
	payload, err := json.Marshal(applyLockFile{SessionID: sessionID, Pid: pid})
	if err != nil {
		return
	}
	_ = os.WriteFile(path, append(payload, '\n'), 0o644)
}

func releaseApplyLockAt(path string) {
	_ = os.Remove(path)
}
