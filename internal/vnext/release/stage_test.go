package release

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shichao402/relkit/internal/stage"
	"github.com/shichao402/relkit/internal/vnext/artifactset"
	"github.com/shichao402/relkit/internal/vnext/document"
	"github.com/shichao402/relkit/internal/vnext/project"
)

func stageFixture(t *testing.T) (string, string, artifactset.Document) {
	t.Helper()
	root := t.TempDir()
	artifactRoot := filepath.Join(root, "candidate")
	if err := os.MkdirAll(artifactRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	content := []byte("installer")
	if err := os.WriteFile(filepath.Join(artifactRoot, "demo.dmg"), content, 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(content)

	// The project root public key is opaque to staging: a staged tree records
	// the declaration verbatim and the agent compares it against its own root.
	rootPublicKey := base64.StdEncoding.EncodeToString(make([]byte, 32))
	productDocument := project.Document{
		Schema:             project.Schema,
		Product:            "demo",
		Server:             "https://updates.example",
		RootPublicKey:      rootPublicKey,
		Channels:           []string{"dev", "stable"},
		SelectorDimensions: []string{"os-arch"},
		ArtifactProducer:   &project.Command{Command: []string{"fixture"}},
	}
	artifacts := artifactset.Document{
		Schema:      artifactset.Schema,
		Product:     "demo",
		Version:     "1.2.3",
		ReleaseCode: 12,
		Channel:     "dev",
		Artifacts: []artifactset.Artifact{
			{
				ID:       "macos-installer",
				Path:     "demo.dmg",
				Kind:     artifactset.KindInstaller,
				Platform: "darwin-arm64",
				SHA256:   "sha256:" + fmt.Sprintf("%x", sum[:]),
				Size:     int64(len(content)),
			},
		},
	}
	projectPath := filepath.Join(root, ProjectFile)
	if err := document.Write(projectPath, productDocument); err != nil {
		t.Fatal(err)
	}
	artifactSetPath := filepath.Join(artifactRoot, ArtifactSetFile)
	if err := document.Write(artifactSetPath, artifacts); err != nil {
		t.Fatal(err)
	}
	return root, artifactSetPath, artifacts
}

func TestStageWritesProjectAndArtifactSet(t *testing.T) {
	root, artifactSetPath, artifacts := stageFixture(t)
	productDocument, err := project.Load(filepath.Join(root, ProjectFile))
	if err != nil {
		t.Fatal(err)
	}
	staged, err := Stage(*productDocument, artifacts, StageOptions{
		ProjectRoot:     root,
		ArtifactSetPath: artifactSetPath,
		Code:            12,
	})
	if err != nil {
		t.Fatal(err)
	}
	if staged.Product != "demo" || staged.Version != "1.2.3" || staged.Code != 12 || staged.Channel != "dev" {
		t.Fatalf("unexpected staged document: %#v", staged)
	}
	stagedRoot := stage.StagingDir(root, "1.2.3")
	for _, name := range []string{"staged.pb", ProjectFile, ArtifactSetFile, "artifacts/demo.dmg"} {
		if _, err := os.Stat(filepath.Join(stagedRoot, name)); err != nil {
			t.Fatalf("missing staged file %s: %v", name, err)
		}
	}
	for _, gone := range []string{"resolution.json", "resolution-bundle.json", "release-policy.json"} {
		if _, err := os.Stat(filepath.Join(stagedRoot, gone)); !os.IsNotExist(err) {
			t.Fatalf("staged tree still carries the retired %s", gone)
		}
	}
}

func TestStageRefusesToReplaceAnExistingTree(t *testing.T) {
	root, artifactSetPath, artifacts := stageFixture(t)
	productDocument, err := project.Load(filepath.Join(root, ProjectFile))
	if err != nil {
		t.Fatal(err)
	}
	options := StageOptions{ProjectRoot: root, ArtifactSetPath: artifactSetPath, Code: 12}
	if _, err := Stage(*productDocument, artifacts, options); err != nil {
		t.Fatal(err)
	}
	if _, err := Stage(*productDocument, artifacts, options); err == nil ||
		!strings.Contains(err.Error(), "immutable") {
		t.Fatalf("expected a second stage to be refused, got %v", err)
	}
}

func TestStageRejectsUndeclaredChannel(t *testing.T) {
	root, artifactSetPath, artifacts := stageFixture(t)
	productDocument, err := project.Load(filepath.Join(root, ProjectFile))
	if err != nil {
		t.Fatal(err)
	}
	productDocument.Channels = []string{"stable"}
	if _, err := Stage(*productDocument, artifacts, StageOptions{
		ProjectRoot:     root,
		ArtifactSetPath: artifactSetPath,
		Code:            12,
	}); err == nil || !strings.Contains(err.Error(), "does not declare channel") {
		t.Fatalf("expected an undeclared channel rejection, got %v", err)
	}
}
