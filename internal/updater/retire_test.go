package updater

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFrozenSDKBanner(t *testing.T) {
	root := findModuleRoot(t)
	raw, err := os.ReadFile(filepath.Join(root, "sdk", "updater.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "Frozen") {
		t.Fatal("sdk.Updater must stay frozen (ADR 0010)")
	}
}

func findModuleRoot(t *testing.T) string {
	t.Helper()
	wd, _ := os.Getwd()
	for i := 0; i < 6; i++ {
		if _, err := os.Stat(filepath.Join(wd, "go.mod")); err == nil {
			return wd
		}
		wd = filepath.Dir(wd)
	}
	t.Fatal("no go.mod")
	return ""
}
