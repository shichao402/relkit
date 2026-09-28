package releasegate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shichao402/relkit/internal/consume"
)

// writeTree writes a deterministic file tree and returns the sha256 the
// digest would compute (callers compare against consume.TreeSHA256).
func writeTree(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		path := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// TestLockDrift pins the lock reconcile slice: schema guard, hash format,
// and the scripts/host tree hash agreement. Missing lock is not drift here.
func TestLockDrift(t *testing.T) {
	// No lock at all: install owns that failure, not the release gate.
	if drift := LockDrift(t.TempDir()); len(drift) != 0 {
		t.Errorf("missing lock must not be drift; got %v", drift)
	}

	// consume/3 lock with a matching tree: clean.
	root := t.TempDir()
	writeTree(t, filepath.Join(root, "scripts", "host"), map[string]string{
		"relkit_host.py": "print('ok')\n",
	})
	actual, err := consumeTreeSHA(t, root)
	if err != nil {
		t.Fatal(err)
	}
	writeLock(t, root, consume3Lock(actual))
	if drift := LockDrift(root); len(drift) != 0 {
		t.Errorf("matching consume/3 lock must be clean; got %v", drift)
	}

	// Wrong tree hash: drift.
	writeLock(t, root, consume3Lock(strings64("a")))
	if drift := LockDrift(root); len(drift) != 1 || drift[0] != "scripts/host tree does not match lock hostScriptsSha256" {
		t.Errorf("hash mismatch drift = %v", drift)
	}

	// Non-hex hash: refused before hashing.
	writeLock(t, root, consume3Lock("not-a-hash"))
	if drift := LockDrift(root); len(drift) != 1 || drift[0] != "lock has no valid hostScriptsSha256" {
		t.Errorf("non-hex hash drift = %v", drift)
	}

	// Hostless consume/3 lock (ADR 0017 phase-3 target): no hash pinned,
	// no scripts/host tree, no drift either.
	hostlessRoot := t.TempDir()
	writeLock(t, hostlessRoot, consume3HostlessLock())
	if drift := LockDrift(hostlessRoot); len(drift) != 0 {
		t.Errorf("hostless consume/3 lock must be clean; got %v", drift)
	}
}

// consume3HostlessLock builds the hostless form: consume/3 without
// hostScriptsSha256, the shape products get after retiring scripts/host.
func consume3HostlessLock() string {
	return `{
	"schema": "relkit.consume/3",
	"release": "v0.5.4",
	"commit": "7777777777777777777777777777777777777777",
	"source": {"module": "github.com/shichao402/relkit", "version": "v0.5.4", "h1": "", "commit": "7777777777777777777777777777777777777777"},
	"protocol": {"min": 2, "max": 2},
	"updaterIpc": {"min": 3, "max": 3},
	"artifacts": {"cli": {"urls": ["https://example.invalid/relkit.exe"], "sha256": "` + strings64("b") + `"}}
}`
}

// TestStagedDrift pins the staged-presence check keyed by the release
// version, including the plain "x.y.z" shape Python consumers stage with.
func TestStagedDrift(t *testing.T) {
	root := t.TempDir()
	if drift := StagedDrift(root, "0.2.0+160"); len(drift) != 1 {
		t.Errorf("missing staged tree must drift; got %v", drift)
	}
	staged := filepath.Join(root, ".relkit", "cache", "staged", "0.2.0+160")
	if err := os.MkdirAll(staged, 0o755); err != nil {
		t.Fatal(err)
	}
	if drift := StagedDrift(root, "0.2.0+160"); len(drift) != 0 {
		t.Errorf("present staged tree must be clean; got %v", drift)
	}
}

// TestReleaseVersionFor pins the VERSION.json read: v-prefix normalized,
// version.json fallback, error when nothing exists.
func TestReleaseVersionFor(t *testing.T) {
	root := t.TempDir()
	if _, err := ReleaseVersionFor(root); err == nil {
		t.Error("no VERSION.json must be an error")
	}
	if err := os.WriteFile(filepath.Join(root, "VERSION.json"), []byte(`{"schema": "relkit.version/1", "version": "v0.2.0+160"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	version, err := ReleaseVersionFor(root)
	if err != nil || version != "0.2.0+160" {
		t.Errorf("ReleaseVersionFor = %q, %v; want 0.2.0+160", version, err)
	}
}

// TestReconcileContractGate pins the release-contract cross-check: protocol
// and updaterIpc windows must agree between lock and installed contract.
func TestReconcileContractGate(t *testing.T) {
	root := t.TempDir()
	writeTree(t, filepath.Join(root, "scripts", "host"), map[string]string{
		"relkit_host.py": "print('ok')\n",
	})
	actual, err := consumeTreeSHA(t, root)
	if err != nil {
		t.Fatal(err)
	}
	writeLock(t, root, consume3Lock(actual))
	writeTree(t, filepath.Join(root, "scripts", "host"), map[string]string{
		"relkit_host.py":     "print('ok')\n",
		"release-contract.json": `{"schema": "relkit.host-contract/1", "protocol": {"min": 2, "max": 2}, "updaterIpc": {"min": 3, "max": 3}}`,
	})
	// The contract file changed the tree hash; re-pin the lock to it.
	actual2, err := consumeTreeSHA(t, root)
	if err != nil {
		t.Fatal(err)
	}
	writeLock(t, root, consume3Lock(actual2))

	staged := filepath.Join(root, ".relkit", "cache", "staged", "0.2.0+160")
	if err := os.MkdirAll(staged, 0o755); err != nil {
		t.Fatal(err)
	}
	report, err := Reconcile(root, "0.2.0+160")
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Drift) != 0 {
		t.Errorf("agreeing contract must be clean; got %v", report.Drift)
	}

	// updaterIpc disagreement: drift names the field.
	writeLock(t, root, consume3LockOverride(actual2, `{"min": 9, "max": 9}`))
	report, err = Reconcile(root, "0.2.0+160")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range report.Drift {
		if strings.Contains(item, "updaterIpc") {
			found = true
		}
	}
	if !found {
		t.Errorf("updaterIpc disagreement must drift; got %v", report.Drift)
	}
}

// helpers

func writeLock(t *testing.T, root, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "scripts", "relkit.lock.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func consume3Lock(hostTreeSHA string) string {
	return `{
	"schema": "relkit.consume/3",
	"release": "v0.5.1",
	"commit": "6a50e778820f9bdb1c8060a8f1e036c04bf25051",
	"source": {"module": "github.com/shichao402/relkit", "version": "v0.5.1", "h1": "", "commit": "6a50e778820f9bdb1c8060a8f1e036c04bf25051"},
	"hostScriptsSha256": "` + hostTreeSHA + `",
	"protocol": {"min": 2, "max": 2},
	"updaterIpc": {"min": 3, "max": 3},
	"artifacts": {"cli": {"urls": ["https://example.invalid/relkit.exe"], "sha256": "` + strings64("b") + `"}}
}`
}

func consume3LockOverride(hostTreeSHA, updaterIPC string) string {
	return `{
	"schema": "relkit.consume/3",
	"release": "v0.5.1",
	"commit": "6a50e778820f9bdb1c8060a8f1e036c04bf25051",
	"source": {"module": "github.com/shichao402/relkit", "version": "v0.5.1", "h1": "", "commit": "6a50e778820f9bdb1c8060a8f1e036c04bf25051"},
	"hostScriptsSha256": "` + hostTreeSHA + `",
	"protocol": {"min": 2, "max": 2},
	"updaterIpc": ` + updaterIPC + `,
	"artifacts": {"cli": {"urls": ["https://example.invalid/relkit.exe"], "sha256": "` + strings64("b") + `"}}
}`
}

func consumeTreeSHA(t *testing.T, root string) (string, error) {
	return consume.TreeSHA256(filepath.Join(root, "scripts", "host"))
}

func strings64(char string) string {
	out := ""
	for i := 0; i < 64; i++ {
		out += char
	}
	return out
}
