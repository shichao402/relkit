package uploadtoken

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsProductNamed(t *testing.T) {
	products := []string{"loom", "svn-auto-merge"}
	if !IsProductNamed("tokens/loom.token", products) {
		t.Fatal("loom.token is product-named")
	}
	if !IsProductNamed("tokens/svn-auto-merge.token", products) {
		t.Fatal("svn-auto-merge.token is product-named")
	}
	if IsProductNamed("tokens/shared.token", products) {
		t.Fatal("shared.token must not be treated as a product id")
	}
	if IsProductNamed("tokens/shared-2.token", products) {
		t.Fatal("shared-2.token must not be treated as a product id")
	}
}

func TestNextFamilyRelSkipsTaken(t *testing.T) {
	if got := NextFamilyRel(nil); got != FamilyRelPath {
		t.Fatalf("got %s", got)
	}
	if got := NextFamilyRel([]string{FamilyRelPath, "tokens/other.token"}); got != "tokens/shared-2.token" {
		t.Fatalf("got %s", got)
	}
	if got := NextFamilyRel([]string{FamilyRelPath, "tokens/shared-2.token"}); got != "tokens/shared-3.token" {
		t.Fatalf("got %s", got)
	}
}

func TestPromoteFileRenamesProductNamed(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "relkit-agent.json")
	src := filepath.Join(dir, "tokens", "loom.token")
	if err := os.MkdirAll(filepath.Dir(src), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, []byte("secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := PromoteFile(configPath, "tokens/loom.token", []string{"loom", "svn-auto-merge"}, []string{"tokens/loom.token"})
	if err != nil {
		t.Fatal(err)
	}
	if got != FamilyRelPath {
		t.Fatalf("got %s", got)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatalf("old product-named file should be gone: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(dir, "tokens", "shared.token"))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "secret\n" {
		t.Fatalf("body %q", body)
	}
}

func TestPromoteFileKeepsFamilyName(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "relkit-agent.json")
	got, err := PromoteFile(configPath, FamilyRelPath, []string{"loom", "svn-auto-merge"}, []string{FamilyRelPath})
	if err != nil {
		t.Fatal(err)
	}
	if got != FamilyRelPath {
		t.Fatalf("got %s", got)
	}
}
