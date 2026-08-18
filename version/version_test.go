package version_test

import (
	"os"
	"path/filepath"
	"testing"

	"cnb.cool/shichao402/relkit/version"
)

func TestParseAndBump(t *testing.T) {
	parts, err := version.Parse("1.2.3+9")
	if err != nil {
		t.Fatal(err)
	}
	if parts.String() != "1.2.3+9" || parts.Number() != "1.2.3" {
		t.Fatalf("unexpected parts: %+v", parts)
	}

	doc, err := version.Skeleton("1.2.3+9")
	if err != nil {
		t.Fatal(err)
	}
	got, err := doc.Bump("patch")
	if err != nil {
		t.Fatal(err)
	}
	if got.String() != "1.2.4+9" {
		t.Fatalf("patch bump: got %s", got)
	}
	got, err = doc.Bump("build")
	if err != nil {
		t.Fatal(err)
	}
	if got.String() != "1.2.4+10" {
		t.Fatalf("build bump: got %s", got)
	}
}

func TestLoadLegacyAndRewriteOfficial(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, version.FileName)
	legacy := []byte(`{
  "app": {
    "version": "0.0.22+18"
  },
  "compatibility": {
    "min_app_version": "1.0.0"
  }
}
`)
	if err := os.WriteFile(path, legacy, 0o644); err != nil {
		t.Fatal(err)
	}
	doc, err := version.LoadPath(path)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Version != "0.0.22+18" {
		t.Fatalf("version: got %q", doc.Version)
	}
	if err := doc.Write(); err != nil {
		t.Fatal(err)
	}
	rewritten, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(rewritten)
	if !containsAll(text, `"schema": "rup.version/1"`, `"version": "0.0.22+18"`, `"min_app_version"`) {
		t.Fatalf("rewrite missing expected fields:\n%s", text)
	}
	if containsAll(text, `"app"`) {
		t.Fatalf("legacy app object should be dropped on write:\n%s", text)
	}
}

func TestFindWalksParents(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatal(err)
	}
	doc, err := version.Skeleton("0.1.0+1")
	if err != nil {
		t.Fatal(err)
	}
	doc.Path = filepath.Join(root, version.FileName)
	if err := doc.Write(); err != nil {
		t.Fatal(err)
	}
	found, err := version.Find(child)
	if err != nil {
		t.Fatal(err)
	}
	if found != doc.Path {
		t.Fatalf("find: got %q want %q", found, doc.Path)
	}
}

func containsAll(text string, parts ...string) bool {
	for _, part := range parts {
		if !contains(text, part) {
			return false
		}
	}
	return true
}

func contains(text, part string) bool {
	return len(text) >= len(part) && (text == part || len(part) == 0 ||
		(func() bool {
			for i := 0; i+len(part) <= len(text); i++ {
				if text[i:i+len(part)] == part {
					return true
				}
			}
			return false
		})())
}
