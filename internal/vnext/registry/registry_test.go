package registry

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestEmbeddedRegistryIsValidAndHostless(t *testing.T) {
	value, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := value.ByName("host-scripts"); ok {
		t.Fatal("retired host-scripts is present")
	}
	if component, ok := value.ByName("bindings-ts"); !ok || component.Role != RoleProjection {
		t.Fatalf("bindings-ts must be a projection, got %+v", component)
	}
}

func TestRequiredSeparatesFacadeAndProjection(t *testing.T) {
	value, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	required, err := value.Required("windows-amd64", []string{"rust"}, []string{"ts"}, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"cli", "updater", "sdk-rust", "bindings-ts"} {
		if !slices.Contains(required, name) {
			t.Fatalf("required components %v do not contain %s", required, name)
		}
	}
	if slices.Contains(required, "sdk-node") {
		t.Fatal("TypeScript projection was misclassified as a Node facade")
	}
}

func TestVNextCoreContainsNoHostRepositorySpecialCases(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate vNext source root")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), ".."))
	forbidden := []string{"loom-launcher", "loom", "svnmergetool", "svn merge tool", `"dec"`}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		lower := strings.ToLower(string(data))
		for _, value := range forbidden {
			if strings.Contains(lower, value) {
				relative, _ := filepath.Rel(root, path)
				t.Errorf("%s contains forbidden host identity %q", relative, value)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
