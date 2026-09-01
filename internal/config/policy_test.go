package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractProductPolicyOmitsMachineFields(t *testing.T) {
	cfg := &Config{
		Product:        "demo",
		DefaultChannel: "stable",
		Channels:       []string{"stable", "beta"},
		CodeStrategy:   "explicit",
		Signing: map[string]any{
			"keyId":          "k1",
			"privateKeyEnv":  "SECRET_SEED",
			"privateKeyPath": "keys/private.pb",
			"publicKeys": []any{
				map[string]any{"keyId": "k1", "publicKeyBase64": "public"},
			},
		},
		Backends:  map[string]map[string]any{"prod": {"type": "cos", "secretIdEnv": "SECRET_ID"}},
		PublishTo: []string{"prod"},
		Directory: &DirectoryConfig{
			PublishTo: []string{"prod"},
			EntryURLs: []string{"https://updates.example/directory/demo.pb"},
		},
		Changelog: ChangelogConfig{
			File:        "docs/CHANGELOG.md",
			URLTemplate: "https://example.com/notes/{version}",
		},
		Site: SiteConfig{Makers: &MakersConfig{
			ProjectID: "makers-demo",
			Region:    "global",
			TokenEnv:  "MAKERS_SECRET",
		}},
	}

	policy, err := ExtractProductPolicy(cfg)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(policy)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, forbidden := range []string{"privateKeyEnv", "privateKeyPath", "backends", "publishTo", "tokenEnv", "SECRET", "docs/CHANGELOG.md", `"file"`} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("policy contains machine field/value %q: %s", forbidden, text)
		}
	}
	for _, required := range []string{`"projectId":"makers-demo"`, `"entryUrls"`, `"publicKeys"`, `"urlTemplate":"https://example.com/notes/{version}"`} {
		if !strings.Contains(text, required) {
			t.Fatalf("policy lacks %s: %s", required, text)
		}
	}
}

func TestLoadProductPolicyStrictlyRejectsMachineAndUnknownFields(t *testing.T) {
	for name, extra := range map[string]string{
		"top-level backend": `,"backends":{"prod":{"type":"local"}}`,
		"private key":       `,"privateKeyEnv":"SECRET"`,
		"makers token":      `,"tokenEnv":"SECRET"`,
		"directory target":  `,"publishTo":["prod"]`,
		"changelog file":    `,"changelog":{"file":"CHANGELOG.md","urlTemplate":"https://example.com/{version}"}`,
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "policy.json")
			var raw string
			switch name {
			case "private key":
				raw = `{"product":"demo","defaultChannel":"stable","channels":["stable"],"codeStrategy":"explicit","signing":{"keyId":"k1"` + extra + `}}`
			case "makers token":
				raw = `{"product":"demo","defaultChannel":"stable","channels":["stable"],"codeStrategy":"explicit","signing":{"keyId":"k1"},"site":{"makers":{"projectId":"m1"` + extra + `}}}`
			case "directory target":
				raw = `{"product":"demo","defaultChannel":"stable","channels":["stable"],"codeStrategy":"explicit","signing":{"keyId":"k1"},"directory":{"entryUrls":[]` + extra + `}}`
			case "changelog file":
				raw = `{"product":"demo","defaultChannel":"stable","channels":["stable"],"codeStrategy":"explicit","signing":{"keyId":"k1"}` + extra + `}`
			default:
				raw = `{"product":"demo","defaultChannel":"stable","channels":["stable"],"codeStrategy":"explicit","signing":{"keyId":"k1"}` + extra + `}`
			}
			if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadProductPolicy(path); err == nil {
				t.Fatalf("LoadProductPolicy accepted forbidden field: %s", raw)
			}
		})
	}
}

func TestMergeProductPolicy(t *testing.T) {
	policy := &ProductPolicy{
		Product:        "demo",
		DefaultChannel: "stable",
		Channels:       []string{"stable", "beta"},
		CodeStrategy:   "explicit",
		RetainVersions: 3,
		Signing: ProductSigningPolicy{
			KeyID:      "k1",
			PublicKeys: []PublicKeyConfig{{KeyID: "k1", PublicKeyBase64: "public"}},
		},
		Changelog: ProductChangelogPolicy{URLTemplate: "https://example.com/notes/{version}"},
		Directory: &ProductDirectoryPolicy{EntryURLs: []string{"https://updates.example/directory/demo.pb"}},
		Site: ProductSitePolicy{Makers: &ProductMakersPolicy{
			ProjectID: "makers-demo",
			Region:    "global",
		}},
	}
	profile := &PublishProfile{
		Product:   "demo",
		Signing:   PublishSigningProfile{KeyID: "k1", PrivateKeyEnv: "DEMO_SEED"},
		Backends:  map[string]map[string]any{"prod": {"type": "local", "outputDir": "out"}},
		PublishTo: []string{"prod"},
		Directory: &PublishDirectoryProfile{PublishTo: []string{"prod"}},
		Site:      PublishSiteProfile{Makers: &PublishMakersProfile{TokenEnv: "MAKERS_TOKEN"}},
	}
	root := filepath.Join(t.TempDir(), "product")

	cfg, err := MergeProductPolicy(policy, profile, root)
	if err != nil {
		t.Fatal(err)
	}
	absRoot, _ := filepath.Abs(root)
	if cfg.Root != absRoot {
		t.Fatalf("Root = %q, want %q", cfg.Root, absRoot)
	}
	if cfg.Product != "demo" || cfg.RetainVersions != 3 || cfg.Signing["privateKeyEnv"] != "DEMO_SEED" {
		t.Fatalf("merged config = %+v", cfg)
	}
	if cfg.Directory == nil || len(cfg.Directory.EntryURLs) != 1 || len(cfg.Directory.PublishTo) != 1 {
		t.Fatalf("merged directory = %+v", cfg.Directory)
	}
	if cfg.Site.Makers == nil || cfg.Site.Makers.TokenEnv != "MAKERS_TOKEN" {
		t.Fatalf("merged makers = %+v", cfg.Site.Makers)
	}
	if cfg.Changelog.URLTemplate != "https://example.com/notes/{version}" || cfg.Changelog.File != "" {
		t.Fatalf("merged changelog = %+v", cfg.Changelog)
	}
	if _, ok := cfg.Raw["product"]; !ok {
		t.Fatalf("merged Raw was not rebuilt: %#v", cfg.Raw)
	}
}

func TestMergeProductPolicyRejectsMismatchesAndUnknownTargets(t *testing.T) {
	policy := &ProductPolicy{
		Product:        "demo",
		DefaultChannel: "stable",
		Channels:       []string{"stable"},
		CodeStrategy:   "explicit",
		Signing:        ProductSigningPolicy{KeyID: "k1"},
	}
	base := &PublishProfile{
		Product:   "other",
		Signing:   PublishSigningProfile{KeyID: "k1"},
		Backends:  map[string]map[string]any{"prod": {"type": "local"}},
		PublishTo: []string{"prod"},
	}
	if _, err := MergeProductPolicy(policy, base, t.TempDir()); err == nil {
		t.Fatal("expected product mismatch")
	}
	base.Product = "demo"
	base.Signing.KeyID = "k2"
	if _, err := MergeProductPolicy(policy, base, t.TempDir()); err == nil {
		t.Fatal("expected keyId mismatch")
	}
	base.Signing.KeyID = "k1"
	base.PublishTo = []string{"missing"}
	if _, err := MergeProductPolicy(policy, base, t.TempDir()); err == nil {
		t.Fatal("expected unknown publishTo backend")
	}
}
