package releasegate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestIncompleteStepsMatrix pins the gate's step-status matrix: which
// statuses release each step, what via_ci relaxes, and what direct mode
// makes irrelevant.
func TestIncompleteStepsMatrix(t *testing.T) {
	state := &State{
		Product: "demo",
		Steps:   map[string]StepStatus{},
	}
	for _, id := range StepIDs {
		state.Steps[id] = StepStatus{Status: "verified"}
	}

	// All verified: nothing missing, with or without CI.
	if missing := IncompleteSteps(state, false, t.TempDir()); len(missing) != 0 {
		t.Errorf("all-verified state must release; got %v", missing)
	}
	if missing := IncompleteSteps(state, true, t.TempDir()); len(missing) != 0 {
		t.Errorf("all-verified via CI must release; got %v", missing)
	}

	// ops.retrospect unanswered blocks a local release but not a CI one.
	state.Steps["ops.retrospect"] = StepStatus{Status: "unanswered"}
	if missing := IncompleteSteps(state, false, t.TempDir()); len(missing) != 1 || missing[0] != "ops.retrospect" {
		t.Errorf("unanswered ops.retrospect must block local release; got %v", missing)
	}
	if missing := IncompleteSteps(state, true, t.TempDir()); len(missing) != 0 {
		t.Errorf("via CI must not require ops.retrospect; got %v", missing)
	}

	// pack.ci confirmed is CI evidence, not local evidence.
	state.Steps["ops.retrospect"] = StepStatus{Status: "verified"}
	state.Steps["pack.ci"] = StepStatus{Status: "confirmed"}
	if missing := IncompleteSteps(state, true, t.TempDir()); len(missing) != 0 {
		t.Errorf("confirmed pack.ci must release via CI; got %v", missing)
	}
	if missing := IncompleteSteps(state, false, t.TempDir()); len(missing) != 1 || missing[0] != "pack.ci" {
		t.Errorf("confirmed pack.ci must not release locally; got %v", missing)
	}

	// Decision steps accept confirmed; action steps do not.
	state.Steps["pack.ci"] = StepStatus{Status: "verified"}
	state.Steps["backend.kind"] = StepStatus{Status: "confirmed"}
	if missing := IncompleteSteps(state, false, t.TempDir()); len(missing) != 0 {
		t.Errorf("confirmed decision step must release; got %v", missing)
	}
	state.Steps["signing.keys"] = StepStatus{Status: "confirmed"}
	if missing := IncompleteSteps(state, false, t.TempDir()); len(missing) != 1 || missing[0] != "signing.keys" {
		t.Errorf("confirmed action step must not release; got %v", missing)
	}
	state.Steps["signing.keys"] = StepStatus{Status: "skipped"}
	if missing := IncompleteSteps(state, false, t.TempDir()); len(missing) != 0 {
		t.Errorf("skipped action step must release; got %v", missing)
	}
}

// TestIncompleteStepsDirectModeIrrelevant pins the direct-topology
// exemption: a direct-publish product owes no SSH/token/registration steps.
func TestIncompleteStepsDirectModeIrrelevant(t *testing.T) {
	root := t.TempDir()
	config := `{"backends": {"primary": {"type": "direct"}}, "agent": {}}`
	if err := os.WriteFile(filepath.Join(root, "relkit.json"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	state := &State{Product: "demo", Steps: map[string]StepStatus{}}
	for _, id := range StepIDs {
		state.Steps[id] = StepStatus{Status: "unanswered"}
	}
	// Direct mode leaves exactly the SSH/token/registration steps out.
	irrelevant := map[string]bool{
		"ssh.host": true, "ssh.config_dir": true, "token.isolation": true,
		"serve.register": true, "agent.register": true,
	}
	missing := IncompleteSteps(state, true, root)
	for _, id := range missing {
		if irrelevant[id] {
			t.Errorf("direct mode must not require %s", id)
		}
	}
	if TopologyMode(root) != "direct" {
		t.Errorf("TopologyMode = %s, want direct", TopologyMode(root))
	}
}

// TestLoadStateShape pins the state-file read: schema guard, step fill-in,
// product fallback to product.id step value.
func TestLoadStateShape(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".relkit"), 0o755); err != nil {
		t.Fatal(err)
	}
	stateJSON := `{
		"schema": "relkit.onboarding/1",
		"steps": {
			"product.id": {"status": "confirmed", "value": "demo", "note": "set by hand"},
			"pack.ci": {"status": "verified", "value": ".github/workflows/release.yml", "note": ""}
		}
	}`
	if err := os.WriteFile(filepath.Join(root, ".relkit", "onboarding.json"), []byte(stateJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	state, err := LoadState(root)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if product, err := state.ProductID(); err != nil || product != "demo" {
		t.Errorf("ProductID = %q, %v; want demo", product, err)
	}
	if state.Steps["pack.ci"].Status != "verified" {
		t.Errorf("pack.ci status = %s", state.Steps["pack.ci"].Status)
	}
	if state.Steps["repo.root"].Status != "unanswered" {
		t.Errorf("missing steps must default to unanswered; repo.root = %s", state.Steps["repo.root"].Status)
	}

	// Wrong schema is refused, not guessed around.
	badRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(badRoot, ".relkit"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(badRoot, ".relkit", "onboarding.json"), []byte(`{"schema": "other"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadState(badRoot); err == nil {
		t.Error("wrong schema must be refused")
	}
}

// TestClearStaleStagedTrees pins the disposable-cache sweep: only versions
// other than the one being published go, non-directories stay.
func TestClearStaleStagedTrees(t *testing.T) {
	root := t.TempDir()
	staged := filepath.Join(root, ".relkit", "cache", "staged")
	for _, name := range []string{"0.4.9", "0.5.0", "0.5.1"} {
		if err := os.MkdirAll(filepath.Join(staged, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(staged, "staged.pb"), []byte("stray"), 0o644); err != nil {
		t.Fatal(err)
	}
	removed, err := ClearStaleStagedTrees(root, "0.5.0")
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 2 || removed[0] != "0.4.9" || removed[1] != "0.5.1" {
		t.Errorf("removed = %v; want [0.4.9 0.5.1]", removed)
	}
	if _, err := os.Stat(filepath.Join(staged, "0.5.0")); err != nil {
		t.Error("current version must survive")
	}
	if _, err := os.Stat(filepath.Join(staged, "staged.pb")); err != nil {
		t.Error("stray file must survive")
	}
}

// TestUploadTokenFaces pins the token intake: sanitized env, unsubstituted
// pipeline placeholder refused, local note fallback.
func TestUploadTokenFaces(t *testing.T) {
	t.Setenv(TokenEnv, "  'tok-abc123'  \r\n")
	token, err := UploadToken(t.TempDir())
	if err != nil || token != "tok-abc123" {
		t.Errorf("env token = %q, %v; want tok-abc123", token, err)
	}

	t.Setenv(TokenEnv, "${{ secrets.agentToken }}")
	if _, err := UploadToken(t.TempDir()); err == nil {
		t.Error("unsubstituted pipeline token must be refused")
	}

	t.Setenv(TokenEnv, "")
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".relkit", "secrets"), 0o755); err != nil {
		t.Fatal(err)
	}
	note := "export RELKIT_UPLOAD_TOKEN='file-token-1'\n"
	if err := os.WriteFile(filepath.Join(root, ".relkit", "secrets", "agent.env"), []byte(note), 0o600); err != nil {
		t.Fatal(err)
	}
	token, err = UploadToken(root)
	if err != nil || token != "file-token-1" {
		t.Errorf("note token = %q, %v; want file-token-1", token, err)
	}
}

// TestPublishProtocolWindow pins the lock-pinned window and its fallback.
func TestPublishProtocolWindow(t *testing.T) {
	root := t.TempDir()
	if min, max := PublishProtocolWindow(root, 2); min != 2 || max != 2 {
		t.Errorf("no lock: window = (%d, %d); want (2, 2)", min, max)
	}
	if err := os.MkdirAll(filepath.Join(root, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	lock := `{"schema": "relkit.consume/3", "protocol": {"min": 3, "max": 5}}`
	if err := os.WriteFile(filepath.Join(root, "scripts", "relkit.lock.json"), []byte(lock), 0o644); err != nil {
		t.Fatal(err)
	}
	if min, max := PublishProtocolWindow(root, 2); min != 3 || max != 5 {
		t.Errorf("locked window = (%d, %d); want (3, 5)", min, max)
	}
}

// publishEndpointFor pins the agent-URL normalization the Python tail and
// casput.normalizeBase already had: a site-root form (no /v1) and an explicit
// /v1 form must both reach /v1/publish. A root-form base POSTing to /publish
// gets 405 from the agent (dec dev/v1.13.104 drill caught this).
func publishEndpointFor(url string) string {
	base := strings.TrimRight(url, "/")
	if !strings.HasSuffix(base, "/v1") {
		base += "/v1"
	}
	return base + "/publish"
}

func TestPublishEndpointNormalization(t *testing.T) {
	cases := map[string]string{
		"https://publish.firoyang.com":        "https://publish.firoyang.com/v1/publish",
		"https://publish.firoyang.com/":       "https://publish.firoyang.com/v1/publish",
		"https://publish.firoyang.com/v1":     "https://publish.firoyang.com/v1/publish",
		"https://publish.firoyang.com/v1/":    "https://publish.firoyang.com/v1/publish",
		"http://127.0.0.1:8080":               "http://127.0.0.1:8080/v1/publish",
		"https://example.com/base/":           "https://example.com/base/v1/publish",
		"https://example.com/base/v1":         "https://example.com/base/v1/publish",
		"https://example.com/v11":             "https://example.com/v11/v1/publish",
	}
	for url, want := range cases {
		if got := publishEndpointFor(url); got != want {
			t.Errorf("publishEndpointFor(%q) = %q; want %q", url, got, want)
		}
	}
}
