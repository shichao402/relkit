package stage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cnb.cool/shichao402/relkit/internal/config"
)

func TestRunWritesNormalizedReleasePolicy(t *testing.T) {
	root := t.TempDir()
	artifactPath := filepath.Join(root, "demo.zip")
	if err := os.WriteFile(artifactPath, []byte("artifact"), 0o644); err != nil {
		t.Fatal(err)
	}
	changelogPath := filepath.Join(root, "CHANGELOG.md")
	if err := os.WriteFile(changelogPath, []byte("## 1.0.0\n\n- staged notes\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{
		Root:           root,
		Product:        "demo",
		DefaultChannel: "stable",
		Channels:       []string{"stable"},
		CodeStrategy:   "explicit",
		Signing: map[string]any{
			"keyId":          "k1",
			"privateKeyEnv":  "PRIVATE_SEED",
			"privateKeyPath": "private.pb",
			"publicKeys": []any{
				map[string]any{"keyId": "k1", "publicKeyBase64": "public"},
			},
		},
		Backends:  map[string]map[string]any{"prod": {"type": "local", "secretEnv": "BACKEND_SECRET"}},
		PublishTo: []string{"prod"},
		Site: config.SiteConfig{Makers: &config.MakersConfig{
			ProjectID: "makers-demo",
			Region:    "global",
			TokenEnv:  "MAKERS_TOKEN",
		}},
		Changelog: config.ChangelogConfig{
			File:        "CHANGELOG.md",
			URLTemplate: "https://example.com/notes/{version}",
		},
		Directory: &config.DirectoryConfig{
			PublishTo: []string{"prod"},
			EntryURLs: []string{"https://updates.example/directory/demo.pb"},
		},
	}

	if _, err := Run(cfg, "1.0.0", 1, 0, []AddSpec{{Path: artifactPath}}, "", "", "", "", false, nil); err != nil {
		t.Fatal(err)
	}
	policyPath := ReleasePolicyPath(root, "1.0.0")
	data, err := os.ReadFile(policyPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 || data[len(data)-1] != '\n' {
		t.Fatalf("policy is not normalized pretty JSON: %q", data)
	}
	text := string(data)
	for _, forbidden := range []string{"privateKey", "backends", "publishTo", "tokenEnv", "SECRET", "CHANGELOG.md", `"file"`} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("release policy contains forbidden %q: %s", forbidden, text)
		}
	}
	if !strings.Contains(text, `"urlTemplate"`) {
		t.Fatalf("release policy dropped changelog urlTemplate: %s", text)
	}
	policy, err := LoadReleasePolicy(root, "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if policy.Product != "demo" || policy.Signing.KeyID != "k1" {
		t.Fatalf("loaded policy = %+v", policy)
	}
	if policy.Site.Makers == nil || policy.Site.Makers.ProjectID != "makers-demo" {
		t.Fatalf("loaded makers = %+v", policy.Site.Makers)
	}
	if policy.Directory == nil || len(policy.Directory.EntryURLs) != 1 {
		t.Fatalf("loaded directory = %+v", policy.Directory)
	}
}

func TestLoadReleasePolicyReportsMissingFile(t *testing.T) {
	if _, err := LoadReleasePolicy(t.TempDir(), "missing"); err == nil {
		t.Fatal("expected missing release policy error")
	}
}
