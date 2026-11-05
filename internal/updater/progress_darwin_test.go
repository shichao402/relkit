//go:build darwin && cgo

package updater

import (
	"os"
	"testing"
	"time"

	updaterv2 "github.com/shichao402/relkit/api/updater/v2"
	"github.com/shichao402/relkit/internal/mainthread"
)

// The test binary's main goroutine serves the main thread the same way
// cmd/relkit-updater's worker does, so the AppKit window can be exercised.
func TestMain(m *testing.M) {
	var code int
	_ = mainthread.Run(func() error { code = m.Run(); return nil })
	os.Exit(code)
}

// TestMacProgressWindowSmoke drives a real window through every state. It
// needs a logged-in GUI session, so it only runs on request:
//
//	RELKIT_PROGRESS_SMOKE=1 go test ./internal/updater -run MacProgressWindowSmoke -v
//
// Set RELKIT_PROGRESS_SMOKE=interactive to leave the failure screen up until
// the Close button is pressed.
func TestMacProgressWindowSmoke(t *testing.T) {
	mode := os.Getenv("RELKIT_PROGRESS_SMOKE")
	if mode == "" {
		t.Skip("set RELKIT_PROGRESS_SMOKE=1 to open a real AppKit window")
	}
	ui, err := platformProgressUI(progressSpec{Product: "Smoke", Version: "9.9.9+1", Lang: uiLanguage()})
	if err != nil {
		t.Fatalf("platformProgressUI: %v", err)
	}
	ui.SetPhase(updaterv2.SessionPhase_SESSION_PHASE_WAITING_FOR_EXIT, true)
	time.Sleep(700 * time.Millisecond)
	ui.SetPhase(updaterv2.SessionPhase_SESSION_PHASE_DOWNLOADING, true)
	for i := int64(0); i <= 10; i++ {
		ui.SetDownload(i*1<<20, 10<<20, 2<<20)
		time.Sleep(120 * time.Millisecond)
	}
	ui.SetPhase(updaterv2.SessionPhase_SESSION_PHASE_COPYING, false)
	time.Sleep(700 * time.Millisecond)

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
		go func() { time.Sleep(1500 * time.Millisecond); ui.Close() }()
		ui.Fail(failure)
	}
	ui.Close()
	if currentMac() != nil {
		t.Fatal("window still registered after Close")
	}
}
