//go:build windows

package updater

import (
	"os"
	"strconv"
	"testing"
	"time"

	updaterv2 "github.com/shichao402/relkit/api/updater/v2"
)

// TestWinProgressWindowSmoke drives a real window through every state. It
// needs an interactive desktop, so it only runs on request:
//
//	RELKIT_PROGRESS_SMOKE=1 go test ./internal/updater -run WinProgressWindowSmoke -v
//
// Set RELKIT_PROGRESS_SMOKE=interactive to leave the failure screen up until
// Close is pressed. RELKIT_PROGRESS_SMOKE_HOLD=<seconds> holds each screen
// that long, for looking at it.
func TestWinProgressWindowSmoke(t *testing.T) {
	mode := os.Getenv("RELKIT_PROGRESS_SMOKE")
	if mode == "" {
		t.Skip("set RELKIT_PROGRESS_SMOKE=1 to open a real Win32 window")
	}
	hold := 700 * time.Millisecond
	if s, err := strconv.Atoi(os.Getenv("RELKIT_PROGRESS_SMOKE_HOLD")); err == nil && s > 0 {
		hold = time.Duration(s) * time.Second
	}
	ui, err := platformProgressUI(progressSpec{Product: "Novoard Agent", Version: "0.3.9+51", Lang: uiLanguage()})
	if err != nil {
		t.Fatalf("platformProgressUI: %v", err)
	}
	ui.SetPhase(updaterv2.SessionPhase_SESSION_PHASE_WAITING_FOR_EXIT, true)
	time.Sleep(hold)
	ui.SetPhase(updaterv2.SessionPhase_SESSION_PHASE_DOWNLOADING, true)
	for i := int64(0); i <= 10; i++ {
		ui.SetDownload(i*3<<20, 30<<20, 2<<20)
		time.Sleep(hold / 5)
	}
	ui.SetPhase(updaterv2.SessionPhase_SESSION_PHASE_COPYING, false)
	time.Sleep(hold)

	failure := &updaterv2.Error{
		Message: "smoke failure",
		Recovery: &updaterv2.RecoveryHelp{
			Message: "The current version still works.",
			Links:   []*updaterv2.RecoveryLink{{Label: "Help", Url: "https://example.com"}},
		},
	}
	if mode == "interactive" {
		ui.Fail(failure)
	} else {
		go func() { time.Sleep(hold + 800*time.Millisecond); ui.Close() }()
		ui.Fail(failure)
	}
	ui.Close()
	activeWinMu.Lock()
	defer activeWinMu.Unlock()
	if activeWin != nil {
		t.Fatal("window still registered after Close")
	}
}
