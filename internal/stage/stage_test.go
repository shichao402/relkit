package stage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	rupv2 "go.firoyang.com/relkit/api/rup/v2"
	"go.firoyang.com/relkit/internal/config"
	"go.firoyang.com/relkit/internal/payload"
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
		Backends:  map[string]map[string]any{"prod": {"type": "relkit-compatible", "secretEnv": "BACKEND_SECRET"}},
		PublishTo: []string{"prod"},
		Site:      config.SiteConfig{Title: "Demo"},
		Changelog: config.ChangelogConfig{
			File:        "CHANGELOG.md",
			URLTemplate: "https://example.com/notes/{version}",
		},
		Directory: &config.DirectoryConfig{
			PublishTo: []string{"prod"},
			EntryURLs: []string{"https://updates.example/directory/demo.pb"},
		},
	}

	if _, err := Run(cfg, "1.0.0", 1, 0, []AddSpec{{Path: artifactPath, Track: "install"}}, "", "", "", "", false, nil); err != nil {
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
	if policy.Site.Title != "Demo" {
		t.Fatalf("loaded site = %+v", policy.Site)
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

func TestRunBuildsPayloadAndRequiresInstallCounterpart(t *testing.T) {
	root := t.TempDir()
	tree := filepath.Join(root, "tree")
	if err := os.MkdirAll(tree, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tree, "app.exe"), []byte("app"), 0o755); err != nil {
		t.Fatal(err)
	}
	installer := filepath.Join(root, "setup.exe")
	if err := os.WriteFile(installer, []byte("setup"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{
		Root: root, Product: "demo", DefaultChannel: "stable", Channels: []string{"stable"},
		CodeStrategy: "explicit",
	}
	if _, err := Run(cfg, "1.0.0", 1, 0, []AddSpec{{
		Path: tree, Track: "payload", PairsText: "os=windows",
	}}, "", "", "", "", false, nil); err == nil {
		t.Fatal("payload without full installer must fail")
	}
	staged, err := Run(cfg, "1.0.0", 1, 0, []AddSpec{
		{Path: tree, Track: "payload", PairsText: "os=windows"},
		{Path: installer, Track: "install", PairsText: "kind=installer,os=windows"},
	}, "", "", "", "", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	var payloadArtifact *rupv2.StagedArtifact
	for _, artifact := range staged.Artifacts {
		if artifact.Kind == rupv2.ArtifactKind_ARTIFACT_KIND_PAYLOAD {
			payloadArtifact = artifact
		}
	}
	if payloadArtifact == nil {
		t.Fatal("payload artifact missing")
	}
	if _, err := payload.Validate(filepath.Join(ArtifactsDir(root, "1.0.0"), payloadArtifact.Filename)); err != nil {
		t.Fatal(err)
	}
}

func TestRunDerivesDistinctPayloadFilenamesAndRejectsCollisions(t *testing.T) {
	root := t.TempDir()
	cfg := &config.Config{
		Root: root, Product: "demo", DefaultChannel: "stable", Channels: []string{"stable"},
		CodeStrategy: "explicit",
	}
	var adds []AddSpec
	for _, target := range []struct {
		os, arch string
	}{
		{os: "linux", arch: "amd64"},
		{os: "windows", arch: "amd64"},
	} {
		tree := filepath.Join(root, target.os+"-tree")
		if err := os.MkdirAll(tree, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(tree, "app.bin"), []byte(target.os), 0o755); err != nil {
			t.Fatal(err)
		}
		installer := filepath.Join(root, target.os+"-setup.zip")
		if err := os.WriteFile(installer, []byte("install-"+target.os), 0o644); err != nil {
			t.Fatal(err)
		}
		selectors := "os=" + target.os + ",arch=" + target.arch
		adds = append(adds,
			AddSpec{Path: installer, Track: "install", PairsText: selectors},
			AddSpec{Path: tree, Track: "payload", PairsText: selectors},
		)
	}

	staged, err := Run(cfg, "1.0.0", 1, 0, adds, "", "", "", "", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	payloadFilenames := map[string]bool{}
	for _, artifact := range staged.Artifacts {
		if artifact.Kind == rupv2.ArtifactKind_ARTIFACT_KIND_PAYLOAD {
			payloadFilenames[artifact.Filename] = true
		}
	}
	if len(payloadFilenames) != 2 {
		t.Fatalf("payload filenames=%v", payloadFilenames)
	}
	for filename := range payloadFilenames {
		if _, err := os.Stat(filepath.Join(ArtifactsDir(root, "1.0.0"), filename)); err != nil {
			t.Fatalf("payload %q was not materialized: %v", filename, err)
		}
	}

	colliding := append([]AddSpec(nil), adds...)
	colliding[1].PairsText += ",filename=shared-payload.zip"
	colliding[3].PairsText += ",filename=shared-payload.zip"
	if _, err := Run(cfg, "1.0.1", 2, 0, colliding, "", "", "", "", false, nil); err == nil ||
		!strings.Contains(err.Error(), "artifact filenames must be unique") {
		t.Fatalf("duplicate filename err=%v", err)
	}
}
