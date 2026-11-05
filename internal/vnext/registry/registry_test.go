package registry

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/shichao402/relkit/internal/vnext/document"
)

func TestEmbeddedRegistryIsValidAndHostless(t *testing.T) {
	value, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := value.ByName("host-scripts"); ok {
		t.Fatal("retired host-scripts is present")
	}
	for _, component := range value.Components {
		if component.Role != RoleInfrastructureBinary && component.Role != RoleProductBinary {
			t.Fatalf("%s has non-binary role %q; product trees were retired by ADR 0026", component.Name, component.Role)
		}
	}
	for _, retired := range []string{"sdk-dart", "sdk-rust", "bindings-ts"} {
		if _, ok := value.ByName(retired); ok {
			t.Fatalf("retired component %s is present", retired)
		}
	}
}

func TestRegistryRejectsRetiredProductTreeFields(t *testing.T) {
	raw := []byte(`{"schema":"relkit.component-registry/1","buildTargets":["linux-amd64"],"components":[{"name":"x","role":"facade","transport":"release-asset","targets":["all"],"destination":"d","build":{"flag":"x","archive":"x.zip"}}]}`)
	var value Document
	if err := document.Decode(raw, &value); err == nil {
		if err := value.Validate(); err == nil {
			t.Fatal("registry accepted a retired facade component")
		}
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
