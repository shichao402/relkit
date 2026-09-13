package simulate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"go.firoyang.com/relkit/internal/config"
)

func TestLoadIndexSkipsMissingS3Credentials(t *testing.T) {
	root := t.TempDir()
	payload, err := json.Marshal(map[string]any{
		"product":        "demo",
		"defaultChannel": "dev",
		"channels":       []string{"dev"},
		"backends": map[string]any{
			"cos": map[string]any{
				"type":         "s3-compatible",
				"endpoint":     "https://cos.example.invalid",
				"bucket":       "demo",
				"region":       "ap-guangzhou",
				"prefix":       "rup/",
				"baseUrl":      "https://example.invalid/rup/",
				"accessKeyEnv": "COS_SECRET_ID",
				"secretKeyEnv": "COS_SECRET_KEY",
			},
		},
		"publishTo": []string{"cos"},
	})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "relkit.json")
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("COS_SECRET_ID", "")
	t.Setenv("COS_SECRET_KEY", "")

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	index, err := LoadIndex(cfg, "", "", "", "")
	if err != nil {
		t.Fatalf("missing publish credentials should not fail simulate: %v", err)
	}
	if index == nil || len(index.Versions) != 0 {
		t.Fatalf("expected an empty index, got %#v", index)
	}
}
