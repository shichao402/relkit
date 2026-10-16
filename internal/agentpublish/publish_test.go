package agentpublish

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDryRunRequiresStagedTreeButNotCredential(t *testing.T) {
	root := t.TempDir()
	staged := filepath.Join(root, ".relkit", "cache", "staged", "1.2.3")
	if err := os.MkdirAll(staged, 0o755); err != nil {
		t.Fatal(err)
	}
	result, err := Run(t.Context(), Options{
		Root: root, Product: "demo", Version: "1.2.3",
		AgentURL: "https://publish.example.invalid", Execute: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Execute || result.AgentURL != "https://publish.example.invalid" {
		t.Fatalf("result=%+v", result)
	}
}

func TestNormalizeEndpointAcceptsRootAndV1(t *testing.T) {
	for input, want := range map[string]string{
		"https://example.invalid":     "https://example.invalid/v1/publish",
		"https://example.invalid/":    "https://example.invalid/v1/publish",
		"https://example.invalid/v1":  "https://example.invalid/v1/publish",
		"https://example.invalid/v1/": "https://example.invalid/v1/publish",
	} {
		if got := normalizeEndpoint(input, "publish"); got != want {
			t.Fatalf("normalizeEndpoint(%q)=%q, want %q", input, got, want)
		}
	}
}
