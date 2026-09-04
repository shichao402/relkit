package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAgentOnboardCheckJSONStableAndNoSecrets(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "relkit-agent.json")
	if err := os.WriteFile(cfg, []byte(`{"addr":"127.0.0.1:9","products":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := runAgentOnboard(&buf, []string{"check", "-config", cfg, "-product", "missing", "-json"}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if strings.Contains(out, "secret") || strings.Contains(out, "RELKIT_UPLOAD_TOKEN") {
		t.Fatalf("leaked secret:\n%s", out)
	}
	if !strings.Contains(out, `"checklist": "agent-machine"`) {
		t.Fatalf("missing checklist:\n%s", out)
	}
}

func TestAgentOnboardRequiresProduct(t *testing.T) {
	if err := runAgentOnboard(bytes.NewBuffer(nil), []string{"check"}); err == nil {
		t.Fatal("expected error")
	}
}
