package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteOperatorPreview(t *testing.T) {
	dir := t.TempDir()
	if err := writeOperatorPreview(dir); err != nil {
		t.Fatal(err)
	}
	portal, err := os.ReadFile(filepath.Join(dir, "portal.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(portal), "SVN Auto Merge") {
		t.Fatalf("portal missing product\n%s", portal)
	}
	product, err := os.ReadFile(filepath.Join(dir, "product.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(product), "class=\"release-card\"") {
		t.Fatalf("product missing release card\n%s", product)
	}
	if out := os.Getenv("RELKIT_PREVIEW_OUT"); out != "" {
		if err := writeOperatorPreview(out); err != nil {
			t.Fatal(err)
		}
	}
}
