package updater

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type applyLockFile struct {
	SessionID string `json:"sessionId"`
	Pid       int    `json:"pid"`
}

func applyLockPath(root string) string {
	return filepath.Join(root, ApplyLockName)
}

func acquireApplyLock(root, sessionID string) error {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	path := applyLockPath(root)
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

func refreshApplyLock(root, sessionID string, pid int) {
	payload, err := json.Marshal(applyLockFile{SessionID: sessionID, Pid: pid})
	if err != nil {
		return
	}
	_ = os.WriteFile(applyLockPath(root), append(payload, '\n'), 0o644)
}

func releaseApplyLock(root string) {
	_ = os.Remove(applyLockPath(root))
}
