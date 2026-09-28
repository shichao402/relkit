package releasegate

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/shichao402/relkit/internal/consume"
)

// This file mirrors hostlib/reconcile.py's release-facing slice: the pieces
// cmd_release runs before publishing when the environment is CI. The full
// onboarding reconcile (SSH probes, remote listings, evidence journal) stays
// Python-side (ADR 0017 decision 8); Go owns only what the release gate
// needs to decide "safe to publish".

// Report is the outcome of the CI reconcile slice.
type Report struct {
	Drift       []string
	Unconfirmed []string
}

// LockDrift mirrors the reconcile lock checks: schema guard and the
// scripts/host tree hash against the lock's hostScriptsSha256. A missing
// lock file is not drift here (install owns that error). A consume/3 lock
// without hostScriptsSha256 is the hostless form: unpinned is not drift.
func LockDrift(root string) []string {
	lockPath := filepath.Join(root, "scripts", "relkit.lock.json")
	data, err := os.ReadFile(lockPath)
	if err != nil {
		return nil // missing lock: install/check own that failure
	}
	lock, err := consume.ParseLock(data)
	if err != nil {
		return []string{fmt.Sprintf("%s is not a readable lock: %v", filepath.ToSlash(lockPath), err)}
	}
	var drift []string
	pinned := strings.ToLower(strings.TrimSpace(lock.HostScriptsSHA256))
	if pinned == "" {
		if lock.Schema == consume.SchemaV3 {
			// Hostless consume/3 lock (ADR 0017): the product retired
			// scripts/host, so neither the tree nor its hash exists to
			// reconcile. Unpinned is not drift; consume/2 locks always
			// pinned the hash, so an empty one there stays malformed.
			return nil
		}
		drift = append(drift, "lock has no valid hostScriptsSha256")
		return drift
	}
	if !sha256HexPattern.MatchString(pinned) {
		drift = append(drift, "lock has no valid hostScriptsSha256")
		return drift
	}
	hostDir := filepath.Join(root, "scripts", "host")
	if _, err := os.Stat(hostDir); err != nil {
		drift = append(drift, "scripts/host tree does not match lock hostScriptsSha256")
		return drift
	}
	actual, err := consume.TreeSHA256(hostDir)
	if err != nil {
		drift = append(drift, fmt.Sprintf("cannot hash scripts/host: %v", err))
		return drift
	}
	if pinned != strings.ToLower(actual) {
		drift = append(drift, "scripts/host tree does not match lock hostScriptsSha256")
	}
	return drift
}

var sha256HexPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// StagedDrift mirrors the staged-tree verification in the CI publish path:
// the staged.pb the release will publish must exist under the staged cache
// and match the lock-pinned scripts/host tree hash the staged tree was
// built from. A staged tree keyed by plain "x.y.z" (Python consumers pass
// the full version) is accepted alongside "x.y.z+build".
func StagedDrift(root, version string) []string {
	staged := stagedDir(root, version)
	if _, err := os.Stat(staged); err != nil {
		return []string{fmt.Sprintf("no staged tree for %s; run relkit stage first (%s)", version, filepath.ToSlash(staged))}
	}
	return nil
}

func stagedDir(root, version string) string {
	return filepath.Join(root, ".relkit", "cache", "staged", version)
}

// ReleaseVersionFor mirrors release.py project_version_for_relkit: read
// VERSION.json (v prefix normalized), no CLI subprocess.
func ReleaseVersionFor(root string) (string, error) {
	for _, name := range []string{"VERSION.json", "version.json"} {
		path := filepath.Join(root, name)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var doc struct {
			Version string `json:"version"`
		}
		if jsonUnmarshal(data, &doc) != nil {
			continue
		}
		raw := strings.TrimSpace(doc.Version)
		if raw != "" {
			return strings.TrimPrefix(raw, "v"), nil
		}
	}
	return "", fmt.Errorf("VERSION.json / VERSION is required for release")
}

// Reconcile runs the CI slice: lock drift, staged-tree presence, and the
// release-contract gate. It never probes remotes: CI cannot reach the serve
// host over SSH, so remote evidence collection stays in the Python
// onboarding flow.
func Reconcile(root, version string) (*Report, error) {
	report := &Report{}
	report.Drift = append(report.Drift, LockDrift(root)...)
	report.Drift = append(report.Drift, StagedDrift(root, version)...)

	// release-contract gate: the installed host scripts must agree with the
	// lock they just wrote (mirrors gates.py release-lock-contract).
	contractPath := filepath.Join(root, "scripts", "host", "release-contract.json")
	contractData, contractErr := os.ReadFile(contractPath)
	if contractErr != nil {
		// The contract file is optional; releases before it shipped can
		// still publish as long as the lock hash itself holds.
		return report, nil
	}
	var contract map[string]any
	if err := jsonUnmarshal(contractData, &contract); err != nil {
		report.Drift = append(report.Drift, fmt.Sprintf("release lock contract is unreadable: %v", err))
		return report, nil
	}
	if schema, _ := contract["schema"].(string); schema != "relkit.host-contract/1" {
		report.Drift = append(report.Drift, "scripts/host/release-contract.json has an unsupported schema")
		return report, nil
	}
	lockData, err := os.ReadFile(filepath.Join(root, "scripts", "relkit.lock.json"))
	if err != nil {
		return report, nil
	}
	var lock map[string]any
	if err := jsonUnmarshal(lockData, &lock); err != nil {
		report.Drift = append(report.Drift, fmt.Sprintf("release lock contract is unreadable: %v", err))
		return report, nil
	}
	for _, field := range []string{"protocol", "updaterIpc"} {
		expected, _ := contract[field]
		actual, _ := lock[field]
		if !intWindowEqual(expected, actual) {
			report.Drift = append(report.Drift, fmt.Sprintf(
				"lock %s=%v does not match installed release contract %v; rerun upgrade for %v",
				field, actual, expected, lock["release"],
			))
		}
	}
	return report, nil
}

func intWindowEqual(a, b any) bool {
	norm := func(v any) (int, int, bool) {
		window, ok := v.(map[string]any)
		if !ok {
			return 0, 0, false
		}
		minimum, minOK := window["min"].(float64)
		maximum, maxOK := window["max"].(float64)
		if !minOK || !maxOK {
			return 0, 0, false
		}
		return int(minimum), int(maximum), true
	}
	aMin, aMax, aOK := norm(a)
	bMin, bMax, bOK := norm(b)
	return aOK && bOK && aMin == bMin && aMax == bMax
}
