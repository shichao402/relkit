package ops

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/shichao402/relkit/internal/vnext/document"
)

func TestAgentPolicyPlansThenAtomicallyPreservesExistingSecrets(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "relkit-agent.json")
	original := []byte(`{
  "listen": "127.0.0.1:8080",
  "products": {
    "loom": {
      "root": "/srv/loom",
      "uploadToken": "do-not-print-or-rewrite-away"
    }
  }
}
`)
	if err := os.WriteFile(configPath, original, 0o600); err != nil {
		t.Fatal(err)
	}
	policyPath := filepath.Join(root, "policy.json")
	policy := AgentPolicyDocument{
		Schema: AgentProductPolicySchema,
		Products: map[string]AgentProductPolicy{
			"loom": {
				DistributionRootFingerprint: "sha256:" + strings.Repeat("a", 64),
				OperatorDeploymentID:        "tencent-internal",
				Lane:                        "production",
			},
		},
	}
	if err := document.Write(policyPath, policy); err != nil {
		t.Fatal(err)
	}

	plan, err := ApplyAgentPolicy(AgentPolicyOptions{Config: configPath, Policy: policyPath})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Changed || plan.Executed || len(plan.ChangedProducts) != 1 {
		t.Fatalf("plan=%+v", plan)
	}
	planJSON, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(planJSON), "do-not-print-or-rewrite-away") {
		t.Fatal("policy plan exposed an existing agent secret")
	}
	afterPlan, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(afterPlan) != string(original) {
		t.Fatal("plan mutated the agent config")
	}

	backupPath := filepath.Join(root, "backups", "before-vnext.json")
	applied, err := ApplyAgentPolicy(AgentPolicyOptions{
		Config: configPath, Policy: policyPath, Backup: backupPath, Execute: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !applied.Changed || !applied.Executed || applied.Backup != backupPath {
		t.Fatalf("applied=%+v", applied)
	}
	backup, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(backup) != string(original) {
		t.Fatal("rollback backup does not contain the exact original config")
	}
	var config struct {
		Products map[string]map[string]any `json:"products"`
	}
	if err := json.Unmarshal(mustRead(t, configPath), &config); err != nil {
		t.Fatal(err)
	}
	product := config.Products["loom"]
	if product["uploadToken"] != "do-not-print-or-rewrite-away" ||
		product["distributionRootFingerprint"] != policy.Products["loom"].DistributionRootFingerprint ||
		product["operatorDeploymentId"] != "tencent-internal" ||
		product["lane"] != "production" {
		t.Fatalf("updated product=%+v", product)
	}
	info, err := os.Stat(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("config mode=%o, want 600", info.Mode().Perm())
	}

	idempotent, err := ApplyAgentPolicy(AgentPolicyOptions{
		Config: configPath, Policy: policyPath, Execute: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if idempotent.Changed {
		t.Fatalf("idempotent application changed config: %+v", idempotent)
	}
}

func TestAgentPolicyRefusesUnprovisionedProduct(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "agent.json")
	if err := os.WriteFile(configPath, []byte(`{"products":{"loom":{"root":"/srv/loom"}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	policyPath := filepath.Join(root, "policy.json")
	if err := document.Write(policyPath, AgentPolicyDocument{
		Schema: AgentProductPolicySchema,
		Products: map[string]AgentProductPolicy{
			"other": {
				DistributionRootFingerprint: "sha256:" + strings.Repeat("b", 64),
				OperatorDeploymentID:        "operator",
				Lane:                        "production",
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyAgentPolicy(AgentPolicyOptions{
		Config: configPath, Policy: policyPath, Execute: true,
	}); err == nil || !strings.Contains(err.Error(), "does not provision") {
		t.Fatalf("error=%v, want unprovisioned product rejection", err)
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
