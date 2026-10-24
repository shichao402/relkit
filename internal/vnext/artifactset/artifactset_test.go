package artifactset

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const artifactDigest = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func platformSet(platform string, artifacts []Artifact) Document {
	return Document{
		Schema:      Schema,
		Product:     "demo",
		Version:     "1.2.3",
		ReleaseCode: 12,
		Channel:     "stable",
		Artifacts:   artifacts,
	}
}

func TestMergeAllowsPlatformAsymmetry(t *testing.T) {
	windows := platformSet("windows-amd64", []Artifact{
		{ID: "windows-installer", Path: "demo.exe", Kind: KindInstaller, Platform: "windows-amd64", SHA256: artifactDigest, Size: 10},
		{ID: "windows-payload", Path: "payload.zip", Kind: KindPayload, Platform: "windows-amd64", SHA256: artifactDigest, Size: 20},
	})
	macos := platformSet("darwin-arm64", []Artifact{
		{ID: "macos-installer", Path: "demo.dmg", Kind: KindInstaller, Platform: "darwin-arm64", SHA256: artifactDigest, Size: 30},
	})
	merged, err := Merge(windows, macos)
	if err != nil {
		t.Fatal(err)
	}
	if len(merged.Artifacts) != 3 {
		t.Fatalf("got %d artifacts, want 3", len(merged.Artifacts))
	}
}

func TestMergeRejectsIdentityDrift(t *testing.T) {
	left := platformSet("windows-amd64", []Artifact{
		{ID: "windows-installer", Path: "a", Kind: KindInstaller, Platform: "windows-amd64", SHA256: artifactDigest, Size: 1},
	})
	right := platformSet("darwin-arm64", []Artifact{
		{ID: "macos-installer", Path: "b", Kind: KindInstaller, Platform: "darwin-arm64", SHA256: artifactDigest, Size: 1},
	})
	right.Version = "1.2.4"
	if _, err := Merge(left, right); err == nil || !strings.Contains(err.Error(), "different") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMergeRejectsMetadataDrift(t *testing.T) {
	left := platformSet("windows-amd64", []Artifact{
		{ID: "windows-installer", Path: "a.exe", Kind: KindInstaller, Platform: "windows-amd64", SHA256: artifactDigest, Size: 1},
	})
	right := platformSet("darwin-arm64", []Artifact{
		{ID: "macos-installer", Path: "b.dmg", Kind: KindInstaller, Platform: "darwin-arm64", SHA256: artifactDigest, Size: 1},
	})
	left.Display = map[string]any{"title": "one"}
	right.Display = map[string]any{"title": "two"}
	if _, err := Merge(left, right); err == nil || !strings.Contains(err.Error(), "display metadata") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestVerifyFilesChecksSelectorAndBytes(t *testing.T) {
	root := t.TempDir()
	content := []byte("artifact bytes")
	if err := os.WriteFile(filepath.Join(root, "demo.zip"), content, 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(content)
	value := platformSet("darwin-arm64", []Artifact{
		{
			ID:        "macos-payload",
			Path:      "demo.zip",
			Kind:      KindPayload,
			Platform:  "darwin-arm64",
			Selectors: map[string]string{"audience": "desktop"},
			SHA256:    "sha256:" + fmt.Sprintf("%x", sum[:]),
			Size:      int64(len(content)),
		},
	})
	if err := value.VerifyFiles(root, []string{"audience"}); err != nil {
		t.Fatal(err)
	}
	if err := value.VerifyFiles(root, []string{"component"}); err == nil || !strings.Contains(err.Error(), "undeclared selector") {
		t.Fatalf("unexpected selector result: %v", err)
	}
	value.Artifacts[0].Size++
	if err := value.VerifyFiles(root, []string{"audience"}); err == nil || !strings.Contains(err.Error(), "size") {
		t.Fatalf("unexpected size result: %v", err)
	}
}

func TestValidateRejectsUnsafePathAndMissingRelation(t *testing.T) {
	value := platformSet("windows-amd64", []Artifact{
		{ID: "windows-payload", Path: "../escape", Kind: KindPayload, Platform: "windows-amd64", SHA256: artifactDigest, Size: 1},
	})
	if err := value.Validate(); err == nil || !strings.Contains(err.Error(), "normalized relative") {
		t.Fatalf("unexpected unsafe path result: %v", err)
	}
	value.Artifacts[0].Path = "payload.zip"
	value.Artifacts[0].Relations.PayloadOf = "missing-installer"
	if err := value.Validate(); err == nil || !strings.Contains(err.Error(), "missing artifact") {
		t.Fatalf("unexpected relation result: %v", err)
	}
}
