package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shichao402/relkit/internal/custody"
	"github.com/shichao402/relkit/internal/vnext/artifactset"
	"github.com/shichao402/relkit/internal/vnext/document"
	"github.com/shichao402/relkit/internal/vnext/project"
)

const testProjectFile = "relkit.project.json"

func testProjectDocument(t *testing.T) project.Document {
	t.Helper()
	seed := make([]byte, ed25519.SeedSize)
	for i := range seed {
		seed[i] = byte(i + 7)
	}
	publicKey := ed25519.NewKeyFromSeed(seed).Public().(ed25519.PublicKey)
	return project.Document{
		Schema:             project.Schema,
		Product:            "demo",
		Server:             "https://updates.example.invalid",
		RootPublicKey:      base64.StdEncoding.EncodeToString(publicKey),
		Channels:           []string{"dev", "stable"},
		SelectorDimensions: []string{"os-arch"},
	}
}

// stageFixture stages one version under root from an ArtifactSet plus the
// project declaration. Both staging and release read the same
// .relkit/cache/staged/<version> tree, so the shape lives in one helper.
func stageFixture(t *testing.T, root, version string) {
	t.Helper()
	productDocument := testProjectDocument(t)
	projectPath := filepath.Join(root, testProjectFile)
	if err := document.Write(projectPath, productDocument); err != nil {
		t.Fatal(err)
	}

	artifactRoot := filepath.Join(root, "artifacts")
	if err := os.MkdirAll(artifactRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	payload := []byte("payload-bytes")
	if err := os.WriteFile(filepath.Join(artifactRoot, "payload.zip"), payload, 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(payload)
	value := artifactset.Document{
		Schema:      artifactset.Schema,
		Product:     productDocument.Product,
		Version:     version,
		ReleaseCode: 7,
		Channel:     "dev",
		Artifacts: []artifactset.Artifact{
			{
				ID:        "payload",
				Kind:      artifactset.KindPayload,
				Path:      "payload.zip",
				Size:      int64(len(payload)),
				SHA256:    "sha256:" + hex.EncodeToString(sum[:]),
				Selectors: map[string]string{"os-arch": "windows-amd64"},
			},
		},
	}
	artifactSetPath := filepath.Join(artifactRoot, "artifact-set.json")
	if err := document.Write(artifactSetPath, value); err != nil {
		t.Fatal(err)
	}

	if err := cmdStage([]string{
		"--project", projectPath,
		"--artifact-set", artifactSetPath,
	}); err != nil {
		t.Fatal(err)
	}
}

// TestStageFromArtifactSet covers the remaining staging input shape: an
// ArtifactSet plus the project declaration. There is no resolution, no
// Operator Profile, and no bundle in the path any more (requirements 12.1,
// 14.9).
func TestStageFromArtifactSet(t *testing.T) {
	root := t.TempDir()
	stageFixture(t, root, "1.2.3")

	// A second stage of the same version must fail: staged releases are
	// immutable.
	projectPath := filepath.Join(root, testProjectFile)
	artifactSetPath := filepath.Join(root, "artifacts", "artifact-set.json")
	if err := cmdStage([]string{
		"--project", projectPath,
		"--artifact-set", artifactSetPath,
	}); err == nil || !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("err=%v", err)
	}
}

// TestReleaseTokenFromCustodyManifest covers the token half of `relkit release`:
// the token comes from the credential manifest by keyId, never from a
// hard-coded variable name in a flag or in the project file (requirement 15).
func TestReleaseTokenFromCustodyManifest(t *testing.T) {
	root := t.TempDir()
	tokenPath := filepath.Join(root, "token")
	if err := os.WriteFile(tokenPath, []byte("ci-token-value\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(root, "custody-manifest.json")
	manifest := custody.Manifest{
		Schema: custody.ManifestSchema,
		Scope:  custody.ScopeHostCI,
		Keys: []custody.Entry{
			{
				KeyID:   "demo-ci-token",
				Kind:    custody.KindTokenRef,
				Purpose: "publish demo from CI",
				Carrier: custody.CarrierFile,
				Path:    tokenPath,
			},
		},
	}
	if err := document.Write(manifestPath, manifest); err != nil {
		t.Fatal(err)
	}

	value, err := releaseToken(manifestPath, "", true)
	if err != nil {
		t.Fatal(err)
	}
	if value != "ci-token-value" {
		t.Fatalf("token=%q", value)
	}
	if _, err := releaseToken(manifestPath, "missing-key", true); err == nil {
		t.Fatal("expected an unknown keyId to fail")
	}

	// A dry run tolerates a machine with no CI secrets so a plan can be printed.
	if value, err := releaseToken(filepath.Join(root, "absent.json"), "", false); err != nil || value != "" {
		t.Fatalf("dry run token=%q err=%v", value, err)
	}
}

// TestReleaseRequiresCIToken covers the execute gate: publishing needs an
// explicit CI marker and a declared server, and a plan alone does not.
func TestReleaseRequiresCIToken(t *testing.T) {
	root := t.TempDir()
	stageFixture(t, root, "1.2.3")
	projectPath := filepath.Join(root, testProjectFile)
	t.Setenv("RELKIT_AGENT_TRANSPORT_URL", "")
	t.Setenv("RELKIT_CUSTODY_MANIFEST", "")
	if err := cmdRelease([]string{"--project", projectPath, "--version", "1.2.3"}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("RELKIT_RELEASE_VIA_CI", "")
	if err := cmdRelease([]string{"--project", projectPath, "--version", "1.2.3", "--execute"}); err == nil ||
		!strings.Contains(err.Error(), "RELKIT_RELEASE_VIA_CI=1") {
		t.Fatalf("err=%v", err)
	}
}
