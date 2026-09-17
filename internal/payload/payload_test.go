package payload

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuildValidateExtract(t *testing.T) {
	tree := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tree, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tree, "bin", "app.exe"), []byte("app"), 0o755); err != nil {
		t.Fatal(err)
	}
	configDir := filepath.Join(tree, ConfigDir)
	if err := os.MkdirAll(filepath.Join(configDir, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "scripts", "migrate.ps1"), []byte("exit 0"), 0o644); err != nil {
		t.Fatal(err)
	}
	config := `{
  "preserve": ["config/user.json"],
  "scripts": [{
    "phase": "pre",
    "path": "migrate.ps1",
    "interpreter": "powershell",
    "timeout": "10s"
  }]
}`
	if err := os.WriteFile(filepath.Join(configDir, ConfigName), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}

	archive := filepath.Join(t.TempDir(), "update.zip")
	table, err := Build(archive, BuildOptions{Tree: tree})
	if err != nil {
		t.Fatal(err)
	}
	if len(table.Files) != 1 || len(table.Scripts) != 1 || len(table.Preserve) != 1 {
		t.Fatalf("unexpected table: %+v", table)
	}
	extracted, err := Extract(archive, filepath.Join(t.TempDir(), "out"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(extracted.FilesRoot, "bin", "app.exe"))
	if err != nil || string(got) != "app" {
		t.Fatalf("extracted file: %q %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(extracted.FilesRoot, ConfigDir)); !os.IsNotExist(err) {
		t.Fatal("payload build metadata leaked into install files")
	}
}

func TestCheckRelpathRejectsEscapes(t *testing.T) {
	for _, value := range []string{"", "../x", "a/../b", "/root", `a\b`, "CON/file"} {
		if err := CheckRelpath(value); err == nil {
			t.Errorf("expected %q to fail", value)
		}
	}
}
