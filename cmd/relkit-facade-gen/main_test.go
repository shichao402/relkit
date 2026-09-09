package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFacadeTokensPresent(t *testing.T) {
	root := findRoot(t)
	raw, err := os.ReadFile(filepath.Join(root, "conformance", "updater", "facade-signatures.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "updateAvailable") {
		t.Fatal(raw)
	}
}

func findRoot(t *testing.T) string {
	t.Helper()
	wd, _ := os.Getwd()
	for i := 0; i < 6; i++ {
		if _, err := os.Stat(filepath.Join(wd, "go.mod")); err == nil {
			return wd
		}
		wd = filepath.Dir(wd)
	}
	t.Fatal("go.mod not found")
	return ""
}
