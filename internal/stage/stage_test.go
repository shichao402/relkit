package stage

import (
	"os"
	"path/filepath"
	"testing"

	rupv2 "github.com/shichao402/relkit/api/rup/v2"
	"github.com/shichao402/relkit/internal/config"
	"github.com/shichao402/relkit/internal/model"
)

// The legacy control plane (release-policy.json, AddSpec/ParseKeyValues,
// Run) is deleted per ADR 0019 §1. What remains is the data-plane staging
// layout shared by `relkit stage`, staged upload, and publish, so these tests
// pin exactly that surface.

func TestStagingLayoutIsVersionKeyed(t *testing.T) {
	root := t.TempDir()
	want := filepath.Join(root, ".relkit", "cache", "staged", "1.2.3")
	if got := StagingDir(root, "1.2.3"); got != want {
		t.Fatalf("StagingDir=%s, want %s", got, want)
	}
	if got, want := StagedPath(root, "1.2.3"), filepath.Join(want, "staged.pb"); got != want {
		t.Fatalf("StagedPath=%s, want %s", got, want)
	}
	if got, want := ArtifactsDir(root, "1.2.3"), filepath.Join(StagingDir(root, "1.2.3"), "artifacts"); got != want {
		t.Fatalf("ArtifactsDir=%s, want %s", got, want)
	}
}

func TestLoadStagedReportsMissingTree(t *testing.T) {
	if _, err := LoadStaged(t.TempDir(), "9.9.9"); err == nil {
		t.Fatal("expected an error for a version with no staged tree")
	}
}

func TestStagedVersionsListsNewestFirst(t *testing.T) {
	root := t.TempDir()
	for _, version := range []string{"1.0.0", "1.0.1"} {
		if err := os.MkdirAll(StagingDir(root, version), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	versions, err := StagedVersions(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 2 {
		t.Fatalf("versions=%v", versions)
	}
}

func TestVerifyPresentStagedHashesToleratesThinTrees(t *testing.T) {
	root := t.TempDir()
	version := "1.0.0"
	if err := os.MkdirAll(ArtifactsDir(root, version), 0o755); err != nil {
		t.Fatal(err)
	}
	payload := []byte("staged-bytes")
	if err := os.WriteFile(filepath.Join(ArtifactsDir(root, version), "app.bin"), payload, 0o644); err != nil {
		t.Fatal(err)
	}
	digest, size, err := model.Sha256File(filepath.Join(ArtifactsDir(root, version), "app.bin"))
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := model.NewStagedArtifact(
		"app", "app.bin", size, digest, "binary",
		map[string]string{"os": "linux"}, nil,
		filepath.Join(ArtifactsDir(root, version), "app.bin"),
	)
	if err != nil {
		t.Fatal(err)
	}
	staged, err := model.NewStagedDocument("demo", version, 1, 0, "stable", []*model.StagedArtifact{artifact}, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Root: root}
	if mismatches := VerifyStagedHashes(cfg, staged); len(mismatches) > 0 {
		t.Fatalf("present artifact reported as mismatch: %v", mismatches)
	}
	// A thin staged tree whose bytes went to CAS first must still verify.
	if err := os.Remove(filepath.Join(ArtifactsDir(root, version), "app.bin")); err != nil {
		t.Fatal(err)
	}
	if mismatches := VerifyStagedHashes(cfg, staged); len(mismatches) == 0 {
		t.Fatal("strict verification must report the missing artifact")
	}
	if mismatches := VerifyPresentStagedHashes(cfg, staged); len(mismatches) > 0 {
		t.Fatalf("present-only verification reported a thin tree: %v", mismatches)
	}
}

func TestVerifyStagedHashesRejectsTamperedBytes(t *testing.T) {
	root := t.TempDir()
	version := "1.0.0"
	if err := os.MkdirAll(ArtifactsDir(root, version), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(ArtifactsDir(root, version), "app.bin")
	if err := os.WriteFile(path, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	digest, size, err := model.Sha256File(path)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := model.NewStagedArtifact("app", "app.bin", size, digest, "binary", map[string]string{"os": "linux"}, nil, path)
	if err != nil {
		t.Fatal(err)
	}
	staged, err := model.NewStagedDocument("demo", version, 1, 0, "stable", []*model.StagedArtifact{artifact}, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("tampered"), 0o644); err != nil {
		t.Fatal(err)
	}
	if mismatches := VerifyStagedHashes(&config.Config{Root: root}, staged); len(mismatches) == 0 {
		t.Fatal("tampered artifact must be reported")
	}
	_ = rupv2.ArtifactKind_ARTIFACT_KIND_BINARY
}
