package consume

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/shichao402/relkit/internal/registry"
)

// StackSkipDirs are never scanned for stack detection signals.
var StackSkipDirs = map[string]bool{
	"node_modules": true,
	"third_party":  true,
	"target":       true,
	"dist":         true,
	".git":         true,
	".relkit":      true,
}

// Stack is the detected product stack: which updater processes own the host
// integration and therefore which SDK trees install consumes.
type Stack struct {
	Languages []string `json:"languages"`
	Updater   string   `json:"updater"`
}

// DetectStack mirrors hostlib/onboard.detect_stack: walk the product tree
// (skipping vendor-ish dirs), collect relative file signals, and map them to
// product-tree components via registry detect patterns.
func DetectStack(root string) (Stack, error) {
	var signals []string
	seen := map[string]bool{}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			// Windows CI may race-delete junctions under node_modules; treat
			// as "no match" instead of aborting install (same as Python).
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return nil
		}
		if rel == "." {
			return nil
		}
		parts := strings.Split(filepath.ToSlash(rel), "/")
		for _, part := range parts[:len(parts)-1] {
			if StackSkipDirs[part] {
				return filepath.SkipDir
			}
		}
		if d.IsDir() || !d.Type().IsRegular() {
			return nil
		}
		posix := filepath.ToSlash(rel)
		if !seen[posix] {
			seen[posix] = true
			signals = append(signals, posix)
		}
		return nil
	})
	if err != nil {
		return Stack{}, err
	}
	sort.Strings(signals)

	names := registry.DetectedComponents(signals)
	kinds := map[string]bool{}
	var languages []string
	for _, name := range names {
		row := registry.ByName[name]
		if row.UpdaterProcess != "" && !kinds[row.UpdaterProcess] {
			kinds[row.UpdaterProcess] = true
			languages = append(languages, row.UpdaterProcess)
		}
	}
	// Languages keep registry order (Python dedupes with dict.fromkeys over
	// detected rows in registry order); sorting would drift the status
	// output away from the host scripts during the dual-implementation
	// window.
	stack := Stack{Languages: languages}
	if len(languages) > 1 {
		stack.Updater = "choose rust or node (which process calls Updater.open)"
	} else if len(languages) == 1 {
		stack.Updater = languages[0]
	}
	return stack, nil
}

// ConsumeComponents mirrors hostlib/release.consume_components: detected SDK
// trees plus cli and updater, deduplicated in registry order.
func ConsumeComponents(root string) ([]string, error) {
	stack, err := DetectStack(root)
	if err != nil {
		return nil, err
	}
	languages := map[string]bool{}
	for _, lang := range stack.Languages {
		languages[lang] = true
	}
	var names []string
	for _, row := range registry.ProductComponents() {
		if row.Role != registry.RoleProductTree {
			continue
		}
		if row.UpdaterProcess != "" && languages[row.UpdaterProcess] {
			names = append(names, row.Name)
			continue
		}
		if row.WebviewProjection && hasWebview(root) {
			names = append(names, row.Name)
		}
	}
	names = append(names, "cli", "updater")
	return dedupe(names), nil
}

// hasWebview mirrors hostlib/gates.has_webview: any package.json outside
// excluded and dot directories.
func hasWebview(root string) bool {
	found := false
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || found {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil || rel == "." {
			return nil
		}
		parts := strings.Split(filepath.ToSlash(rel), "/")
		dirParts := parts[:len(parts)-1]
		for _, part := range dirParts {
			if StackSkipDirs[part] || strings.HasPrefix(part, ".") {
				return filepath.SkipDir
			}
		}
		if d.IsDir() || !d.Type().IsRegular() || d.Name() != "package.json" {
			return nil
		}
		found = true
		return nil
	})
	return found
}

// CheckResult is the per-component verification outcome.
type CheckResult struct {
	Component string `json:"component"`
	OK        bool   `json:"ok"`
	Detail    string `json:"detail,omitempty"`
}

// CheckInstalled verifies an installed component against the lock, mirroring
// relkit_consume.check_installed.
func CheckInstalled(root string, lock *Lock, component, target string) CheckResult {
	spec, err := lock.ArtifactSpecFor(component, target)
	if err != nil {
		moduleChannel := component == "updater" && lock.Schema == SchemaV3 &&
			(errors.Is(err, ErrNoArtifact) || strings.Contains(err.Error(), "lock has no updater artifact"))
		if moduleChannel {
			// ADR 0017 decision 7: the updater exits the artifacts block on
			// consume/3; install placed it through the module channel, so
			// verification falls to the on-disk binary + smoke probe rather
			// than a pinned artifact hash.
			return checkModuleChannelUpdater(root, component, target)
		}
		return CheckResult{Component: component, OK: false, Detail: err.Error()}
	}
	row, ok := registry.ByName[component]
	if !ok {
		return CheckResult{Component: component, OK: false, Detail: "unknown component"}
	}

	switch row.Role {
	case registry.RoleProductTree:
		destination := filepath.Join(root, filepath.FromSlash(row.Destination))
		if component == "host-scripts" {
			pinned := strings.ToLower(lock.HostScriptsSHA256)
			if !isSHA256(pinned) {
				return CheckResult{Component: component, OK: false, Detail: "lock has no valid hostScriptsSha256"}
			}
			actual, err := TreeSHA256(destination)
			if err != nil || actual != pinned {
				return CheckResult{Component: component, OK: false, Detail: "installed scripts/host does not match lock"}
			}
			return CheckResult{Component: component, OK: true}
		}
		markerPath := filepath.Join(destination, ".relkit-artifact.json")
		state, err := os.ReadFile(markerPath)
		if err != nil {
			return CheckResult{Component: component, OK: false, Detail: fmt.Sprintf("no install marker: %v", err)}
		}
		var marker struct {
			SHA256 string `json:"sha256"`
		}
		if err := json.Unmarshal(state, &marker); err != nil || marker.SHA256 != spec.SHA256 {
			return CheckResult{Component: component, OK: false, Detail: fmt.Sprintf("installed %s does not match lock", component)}
		}
		if !treeComplete(destination, row) {
			return CheckResult{Component: component, OK: false, Detail: fmt.Sprintf("installed %s is incomplete", component)}
		}
		return CheckResult{Component: component, OK: true}

	case registry.RoleProductBinary:
		name, err := row.InstallName(target)
		if err != nil {
			return CheckResult{Component: component, OK: false, Detail: err.Error()}
		}
		destination := filepath.Join(root, row.Destination, name)
		digest, err := FileSHA256(destination)
		if err != nil || digest != spec.SHA256 {
			return CheckResult{Component: component, OK: false, Detail: fmt.Sprintf("installed %s does not match lock: %s", component, destination)}
		}
		if err := SmokeTest(destination); err != nil {
			return CheckResult{Component: component, OK: false, Detail: err.Error()}
		}
		return CheckResult{Component: component, OK: true}
	}
	return CheckResult{Component: component, OK: false, Detail: "unsupported role"}
}

// checkModuleChannelUpdater verifies the updater installed through the
// module channel (consume/3): the binary exists under the registry install
// name and answers --version. The lock pins the source version, not the
// binary bytes (release CI patch drift makes byte pinning impossible), so
// the probe is the verification.
func checkModuleChannelUpdater(root, component, target string) CheckResult {
	row, ok := registry.ByName[component]
	if !ok {
		return CheckResult{Component: component, OK: false, Detail: "unknown component"}
	}
	name, err := row.InstallName(target)
	if err != nil {
		return CheckResult{Component: component, OK: false, Detail: err.Error()}
	}
	destination := filepath.Join(root, row.Destination, name)
	if _, err := os.Stat(destination); err != nil {
		return CheckResult{Component: component, OK: false, Detail: fmt.Sprintf("module-channel updater missing: %s", destination)}
	}
	if err := SmokeTest(destination); err != nil {
		return CheckResult{Component: component, OK: false, Detail: err.Error()}
	}
	return CheckResult{Component: component, OK: true}
}

// SmokeTest runs --version as the install/verify probe. Exported so the
// CLI install surface reuses the exact probe checkModuleChannelUpdater
// gates on (updater already-installed shortcut, ADR 0017).
func SmokeTest(binary string) error {
	ctx := timeoutAfter(30 * time.Second)
	cmd := exec.CommandContext(ctx, binary, "--version")
	cmd.SysProcAttr = detachForProbe()
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("smoke test failed (%v): %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// ResolvedArtifacts is the install report written via --resolved-out and the
// status display: component -> sha256.
type ResolvedArtifacts struct {
	Schema    string            `json:"schema"`
	Release   string            `json:"release"`
	Commit    string            `json:"commit"`
	Target    string            `json:"target"`
	Artifacts map[string]string `json:"artifacts"`
}

// isSHA256 validates a lowercase hex digest.
func isSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, ch := range value {
		if !((ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f')) {
			return false
		}
	}
	return true
}

// dedupe keeps first occurrences in order.
func dedupe(items []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, item := range items {
		if !seen[item] {
			seen[item] = true
			out = append(out, item)
		}
	}
	return out
}
