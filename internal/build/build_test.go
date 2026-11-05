package build

import (
	"testing"

	"github.com/shichao402/relkit/internal/testutil"
	"github.com/shichao402/relkit/internal/updater"
)

// TestUpdaterProtocolMatchesEngine pins the manifest's updaterProtocol to the
// constant the engine actually negotiates with.
func TestUpdaterProtocolMatchesEngine(t *testing.T) {
	got, err := UpdaterProtocol(testutil.RepoRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	if got != int(updater.Protocol) {
		t.Fatalf("UpdaterProtocol = %d, engine Protocol = %d", got, updater.Protocol)
	}
}
