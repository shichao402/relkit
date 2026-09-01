package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeAgentCfg(t *testing.T, dir string, cfg FileConfig) string {
	t.Helper()
	path := filepath.Join(dir, "relkit-agent.json")
	if err := writeFileConfig(path, &cfg); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestInitListProducts(t *testing.T) {
	dir := t.TempDir()
	path := writeAgentCfg(t, dir, FileConfig{
		Addr:            "127.0.0.1:8787",
		UploadTokenFile: "/etc/relkit-agent/token",
		MaxUpload:       "8GiB",
		Products: map[string]ProductConfig{
			"dec": {Root: "/srv/relkit/dec"},
			"app": {Root: "/srv/relkit/app"},
		},
	})

	var buf bytes.Buffer
	if err := runInit(&buf, []string{"-config", path, "-list-products"}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if strings.Contains(out, "secret") || strings.Contains(out, "RELKIT_AGENT_TOKEN=") {
		t.Fatalf("list printed a secret:\n%s", out)
	}
	if !strings.Contains(out, "config "+path) {
		t.Fatalf("missing config path:\n%s", out)
	}
	if !strings.Contains(out, "/etc/relkit-agent/token") {
		t.Fatalf("missing token file path:\n%s", out)
	}
	if !strings.Contains(out, "dec") || !strings.Contains(out, "app") {
		t.Fatalf("missing products:\n%s", out)
	}
}

func TestInitListMissingConfigDoesNotCreateDir(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "no-such", "relkit-agent.json")
	if err := runInit(bytes.NewBuffer(nil), []string{"-config", missing, "-list-products"}); err == nil {
		t.Fatal("expected error")
	}
	if _, err := os.Stat(filepath.Dir(missing)); !os.IsNotExist(err) {
		t.Fatalf("list created config dir: %v", err)
	}
}

func TestInitAddAndRemoveProduct(t *testing.T) {
	dir := t.TempDir()
	path := writeAgentCfg(t, dir, FileConfig{
		Addr:            "127.0.0.1:9",
		UploadTokenFile: "/etc/relkit-agent/token",
		MaxUpload:       "8GiB",
		MaxFiles:        10000,
		StateDir:        "/var/lib/relkit-agent",
		Products: map[string]ProductConfig{
			"dec": {Root: "/srv/relkit/dec"},
		},
	})
	root := filepath.Join(dir, "svn-auto-merge")

	var buf bytes.Buffer
	if err := runInit(&buf, []string{"-config", path, "-product", "svn-auto-merge", "-root", root}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if strings.Contains(out, "export RELKIT") {
		t.Fatalf("add printed a new secret:\n%s", out)
	}
	if !strings.Contains(out, "No new secret") {
		t.Fatalf("missing reuse notice:\n%s", out)
	}
	if st, err := os.Stat(root); err != nil || !st.IsDir() {
		t.Fatalf("root not created: %v", err)
	}

	raw, err := loadFileConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if raw.Addr != "127.0.0.1:9" || raw.UploadTokenFile != "/etc/relkit-agent/token" || raw.MaxUpload != "8GiB" {
		t.Fatalf("hand-edited fields were rewritten: %+v", raw)
	}
	if raw.Products["svn-auto-merge"].Root != root {
		t.Fatalf("product not added: %+v", raw.Products)
	}
	if raw.Products["dec"].Root != "/srv/relkit/dec" {
		t.Fatalf("existing product lost: %+v", raw.Products)
	}

	if err := runInit(bytes.NewBuffer(nil), []string{"-config", path, "-product", "svn-auto-merge", "-root", root}); err == nil {
		t.Fatal("duplicate add should fail")
	}

	if err := runInit(&buf, []string{"-config", path, "-product", "svn-auto-merge", "-remove"}); err != nil {
		t.Fatal(err)
	}
	raw, err = loadFileConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := raw.Products["svn-auto-merge"]; ok {
		t.Fatal("product still listed")
	}
	if _, err := os.Stat(root); err != nil {
		t.Fatal("remove deleted the product root")
	}
	if raw.UploadTokenFile != "/etc/relkit-agent/token" {
		t.Fatal("token file path rewritten on remove")
	}
}

func TestInitRemoveLastProduct(t *testing.T) {
	dir := t.TempDir()
	path := writeAgentCfg(t, dir, FileConfig{
		UploadTokenFile: "token",
		Products:        map[string]ProductConfig{"dec": {Root: "/srv/relkit/dec"}},
	})
	if err := runInit(bytes.NewBuffer(nil), []string{"-config", path, "-product", "dec", "-remove"}); err != nil {
		t.Fatal(err)
	}
	raw, err := loadFileConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw.Products) != 0 {
		t.Fatalf("expected empty products, got %+v", raw.Products)
	}
	data, _ := os.ReadFile(path)
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if _, ok := decoded["products"]; !ok {
		t.Fatal("products key dropped")
	}
}

func TestParseStagedRoute(t *testing.T) {
	cases := []struct {
		in      string
		product string
		version string
		ok      bool
	}{
		{"/v1/staged/dec/1.13.38", "dec", "1.13.38", true},
		{"//v1/staged/dec/1.13.38", "dec", "1.13.38", true},
		{"/v1/staged/dec/1.13.38/", "dec", "1.13.38", true},
		{"/v1/staged/dec", "", "", false},
		{"/v1/publish", "", "", false},
	}
	for _, tc := range cases {
		p, v, ok := parseStagedRoute(tc.in)
		if ok != tc.ok || p != tc.product || v != tc.version {
			t.Fatalf("%q: got (%q,%q,%v) want (%q,%q,%v)", tc.in, p, v, ok, tc.product, tc.version, tc.ok)
		}
	}
}

func TestInitFlagCombos(t *testing.T) {
	dir := t.TempDir()
	path := writeAgentCfg(t, dir, FileConfig{
		Products: map[string]ProductConfig{"dec": {Root: "/x"}},
	})
	cases := [][]string{
		{"-config", path, "-list-products", "-product", "dec"},
		{"-config", path, "-list-products", "-remove"},
		{"-config", path, "-remove"},
		{"-config", path, "-product", "dec", "-remove", "-root", dir},
		{"-config", path, "-list-products", "-migrate-profile"},
		{"-config", path, "-product", "dec", "-remove", "-migrate-profile"},
		{"-config", path, "-product", "dec", "-migrate-profile", "-root", dir},
		{"-config", path, "-migrate-profile"},
		{"-config", path},
		{"-config", path, "-product", "bad id"},
		{"-config", path, "-product", "missing", "-remove"},
	}
	for _, args := range cases {
		if err := runInit(bytes.NewBuffer(nil), args); err == nil {
			t.Fatalf("expected error for %v", args)
		}
	}
}

func TestInitMigrateProfileFromLegacy(t *testing.T) {
	dir := t.TempDir()
	productRoot := filepath.Join(dir, "product")
	if err := os.MkdirAll(productRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	legacy := map[string]any{
		"product":        "demo",
		"defaultChannel": "stable",
		"channels":       []any{"stable", "beta"},
		"codeStrategy":   "explicit",
		"retainVersions": 4,
		"signing": map[string]any{
			"keyId":          "k1",
			"privateKeyPath": "private.pb",
			"privateKeyEnv":  "DEMO_SEED",
			"publicKeys": []any{map[string]any{
				"keyId":           "k1",
				"publicKeyBase64": "public-material",
			}},
		},
		"backends": map[string]any{
			"prod": map[string]any{"type": "local", "outputDir": "dist", "secretIdEnv": "COS_SECRET"},
		},
		"publishTo": []any{"prod"},
		"changelog": map[string]any{
			"file":        "docs/CHANGELOG.md",
			"urlTemplate": "https://example.com/notes/{version}",
		},
		"directory": map[string]any{
			"publishTo": []any{"prod"},
			"entryUrls": []any{"https://updates.example/directory/demo.pb"},
		},
		"site": map[string]any{
			"title": "Demo Portal",
			"makers": map[string]any{
				"projectId": "makers-demo",
				"region":    "global",
				"tokenEnv":  "MAKERS_TOKEN",
			},
		},
	}
	legacyBytes, _ := json.MarshalIndent(legacy, "", "  ")
	if err := os.WriteFile(filepath.Join(productRoot, "relkit.json"), legacyBytes, 0o644); err != nil {
		t.Fatal(err)
	}

	path := writeAgentCfg(t, dir, FileConfig{
		Addr:     "127.0.0.1:9",
		StateDir: filepath.Join(dir, "state"),
		Products: map[string]ProductConfig{"demo": {Root: productRoot}},
	})

	var buf bytes.Buffer
	if err := runInit(&buf, []string{"-config", path, "-product", "demo", "-migrate-profile"}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "profile") {
		t.Fatalf("missing profile notice:\n%s", out)
	}

	raw, err := loadFileConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	gotProfile := raw.Products["demo"].Profile
	if gotProfile != filepath.ToSlash(filepath.Join("products", "demo.json")) {
		t.Fatalf("agent profile path = %q", gotProfile)
	}

	profilePath := filepath.Join(dir, "products", "demo.json")
	data, err := os.ReadFile(profilePath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, forbidden := range []string{
		"publicKeys", "public-material", "defaultChannel", "channels",
		"retainVersions", "changelog", "CHANGELOG.md", "urlTemplate",
		"entryUrls", "projectId", "makers-demo", "Demo Portal", "title",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("profile leaked public policy field %q: %s", forbidden, text)
		}
	}
	for _, required := range []string{`"product": "demo"`, `"keyId": "k1"`, `"privateKeyPath"`, `"privateKeyEnv"`, `"backends"`, `"publishTo"`, `"tokenEnv"`} {
		if !strings.Contains(text, required) {
			t.Fatalf("profile lacks machine field %s: %s", required, text)
		}
	}

	if err := runInit(bytes.NewBuffer(nil), []string{"-config", path, "-product", "demo", "-migrate-profile"}); err == nil {
		t.Fatal("repeat migrate should be refused")
	} else if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("repeat migrate error = %v", err)
	}
}
