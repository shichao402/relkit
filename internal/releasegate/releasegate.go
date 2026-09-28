// Package releasegate is the Go home of the release-side gate the CI entry
// owns: reading the onboarding state's step statuses, deciding which steps
// still block a release, and the agent publish path (cas-put + POST
// /publish). The onboarding state file itself is still written by the
// Python onboarding flow during the migration (ADR 0017 decision 8: ops
// surface stays Python); this package only reads it.
package releasegate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// StepIDs mirrors const.py STEP_IDS: every onboarding step, in order.
var StepIDs = []string{
	"repo.root",
	"env.inspect",
	"product.id",
	"updater.process",
	"channel.ssot",
	"backend.kind",
	"ssh.host",
	"ssh.config_dir",
	"token.isolation",
	"serve.register",
	"agent.register",
	"signing.keys",
	"consume.lock",
	"sidecar.layout",
	"fake.release",
	"pack.ci",
	"ops.retrospect",
}

// DecisionSteps mirrors const.py DECISION_STEPS: steps a human answers
// rather than a tool runs; "confirmed" is acceptable evidence for them.
var decisionSteps = map[string]bool{
	"repo.root":       true,
	"env.inspect":     true,
	"product.id":      true,
	"updater.process": true,
	"channel.ssot":    true,
	"backend.kind":    true,
	"ssh.host":        true,
	"ssh.config_dir":  true,
	"token.isolation": true,
}

// State is the subset of relkit.onboarding/1 the gate reads.
type State struct {
	Product string
	Steps   map[string]StepStatus
}

// StepStatus is one step's recorded evidence.
type StepStatus struct {
	Status string
	Value  any
	Note   string
}

// LoadState reads .relkit/onboarding.json (or reports it absent).
func LoadState(root string) (*State, error) {
	path := filepath.Join(root, ".relkit", "onboarding.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf(".relkit/onboarding.json is required for release; run relkit_host.py onboard first (%s)", path)
	}
	var doc struct {
		Schema  string                `json:"schema"`
		Product string                `json:"product"`
		Steps   map[string]StepStatus `json:"steps"`
	}
	if err := jsonUnmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("%s is not readable JSON: %v", path, err)
	}
	if doc.Schema != "relkit.onboarding/1" {
		return nil, fmt.Errorf("%s must use schema relkit.onboarding/1", path)
	}
	steps := map[string]StepStatus{}
	for _, id := range StepIDs {
		if step, ok := doc.Steps[id]; ok {
			steps[id] = step
		} else {
			steps[id] = StepStatus{Status: "unanswered"}
		}
	}
	return &State{Product: strings.TrimSpace(doc.Product), Steps: steps}, nil
}

// ProductID returns the product identity the release publishes under.
func (s *State) ProductID() (string, error) {
	if s.Product != "" {
		return s.Product, nil
	}
	if step, ok := s.Steps["product.id"]; ok {
		if value, ok := step.Value.(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value), nil
		}
	}
	return "", fmt.Errorf("product.id is unanswered; run onboard set product.id <id>")
}

// TopologyMode mirrors remote.py publish_topology's mode for the direct-mode
// irrelevant-step set. Only "direct" matters to the gate.
func TopologyMode(root string) string {
	config, err := readJSONMap(filepath.Join(root, "relkit.json"))
	if err != nil {
		return "unknown"
	}
	agent, _ := config["agent"].(map[string]any)
	if url, _ := agent["url"].(string); strings.TrimSpace(url) != "" {
		return "agent"
	}
	backends, _ := config["backends"].(map[string]any)
	if len(backends) == 0 {
		return "unknown"
	}
	gatewayTypes := map[string]bool{"relkit-compatible": true, "intranet-relkit-compatible": true}
	direct := 0
	for _, raw := range backends {
		backend, _ := raw.(map[string]any)
		kind, _ := backend["type"].(string)
		if gatewayTypes[kind] {
			return "gateway"
		}
		if kind == "direct" {
			direct++
		}
	}
	if direct == len(backends) {
		return "direct"
	}
	return "mixed"
}

// IncompleteSteps mirrors release.py release_incomplete_steps: which steps
// still block a release. via_ci relaxes ops.retrospect (CI cannot answer an
// ops question) and accepts "confirmed" for pack.ci (CI itself is the pack
// evidence).
func IncompleteSteps(s *State, viaCI bool, root string) []string {
	irrelevant := map[string]bool{}
	if TopologyMode(root) == "direct" {
		for _, id := range []string{"ssh.host", "ssh.config_dir", "token.isolation", "serve.register", "agent.register"} {
			irrelevant[id] = true
		}
	}
	var missing []string
	for _, step := range StepIDs {
		if irrelevant[step] {
			continue
		}
		if step == "ops.retrospect" && viaCI {
			continue
		}
		status := s.Steps[step].Status
		allowed := []string{"verified", "skipped"}
		if decisionSteps[step] {
			allowed = []string{"confirmed", "verified", "skipped"}
		}
		if step == "pack.ci" && viaCI {
			allowed = []string{"confirmed", "verified", "skipped"}
		}
		ok := false
		for _, a := range allowed {
			if status == a {
				ok = true
				break
			}
		}
		if !ok {
			missing = append(missing, step)
		}
	}
	return missing
}

// ClearStaleStagedTrees mirrors reconcile.py clear_stale_staged_trees:
// delete disposable staged caches that are not the release being published.
func ClearStaleStagedTrees(root, currentVersion string) ([]string, error) {
	staged := filepath.Join(root, ".relkit", "cache", "staged")
	entries, err := os.ReadDir(staged)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var removed []string
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == currentVersion {
			continue
		}
		if err := os.RemoveAll(filepath.Join(staged, entry.Name())); err != nil {
			return nil, err
		}
		removed = append(removed, entry.Name())
	}
	return removed, nil
}

// PublishProtocolWindow mirrors release.py publish_protocol_window: the
// lock pins the CLI↔agent handshake window. Default when no lock exists.
func PublishProtocolWindow(root string, fallback int) (int, int) {
	lock, err := readJSONMap(filepath.Join(root, "scripts", "relkit.lock.json"))
	if err != nil {
		return fallback, fallback
	}
	protocol, _ := lock["protocol"].(map[string]any)
	minimum := intOf(protocol["min"], fallback)
	maximum := intOf(protocol["max"], minimum)
	return minimum, maximum
}

func readJSONMap(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var value map[string]any
	if err := jsonUnmarshal(data, &value); err != nil {
		return nil, err
	}
	return value, nil
}

func intOf(v any, fallback int) int {
	switch value := v.(type) {
	case float64:
		return int(value)
	case int:
		return value
	}
	return fallback
}

func jsonUnmarshal(data []byte, into any) error {
	return json.Unmarshal(data, into)
}
