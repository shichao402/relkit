package version_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.firoyang.com/relkit/version"
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

func TestLoadOfficialSchema(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, version.FileName)
	body := []byte(`{
  "schema": "relkit.version/1",
  "version": "0.0.22+18",
  "compatibility": {
    "min_app_version": "1.0.0"
  }
}
`)
	if err := os.WriteFile(path, body, 0o644); err != nil {
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
	if !strings.Contains(text, `"schema": "relkit.version/1"`) ||
		!strings.Contains(text, `"version": "0.0.22+18"`) ||
		!strings.Contains(text, `"min_app_version"`) {
		t.Fatalf("rewrite missing expected fields:\n%s", text)
	}
}

func TestRejectRetiredSchemas(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name string
		body string
		want string
	}{
		{
			name: "rup.version/1",
			body: `{"schema":"rup.version/1","version":"1.0.0+1"}`,
			want: "retired",
		},
		{
			name: "legacy app.version",
			body: `{"app":{"version":"1.0.0+1"}}`,
			want: "legacy app.version",
		},
		{
			name: "missing schema with bare version",
			body: `{"version":"1.0.0+1"}`,
			want: "missing required field",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(dir, strings.ReplaceAll(tc.name, "/", "_")+".json")
			if err := os.WriteFile(path, []byte(tc.body), 0o644); err != nil {
				t.Fatal(err)
			}
			_, err := version.LoadPath(path)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q does not contain %q", err, tc.want)
			}
		})
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
