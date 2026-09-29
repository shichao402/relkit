package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeManifest writes a manifest doc to dist/release-artifacts.json under a
// temporary root and creates the referenced install/payload/archive files.
func writeManifest(t *testing.T, root string, doc map[string]any, files map[string]string, dirs []string) {
	t.Helper()
	for name, content := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(root, "dist", "release-artifacts.json")
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadReleaseArtifactsManifestSchema2(t *testing.T) {
	root := t.TempDir()
	doc := map[string]any{
		"schema":  "relkit.release-artifacts/2",
		"version": "1.13.78",
		"selectorGroups": []map[string]any{
			{
				"selectors": map[string]string{"os": "windows", "arch": "amd64", "component": "dec", "audience": "runtime"},
				"install": map[string]any{
					"path":     "dist/dec-windows-amd64.exe",
					"kind":     "binary",
					"filename": "dec-windows-amd64.exe",
				},
				"payloads": []map[string]any{
					{
						"path":      "dist/payload/dec-windows-amd64/",
						"filename":  "dec-1.13.78-dec-windows-amd64-payload.zip",
						"selectors": map[string]string{"os": "windows", "arch": "amd64", "component": "dec", "audience": "runtime"},
					},
				},
			},
			{
				"selectors": map[string]string{"os": "windows", "arch": "amd64", "component": "console", "audience": "user"},
				"install": map[string]any{
					"path": "dist/dec-console-windows-amd64.exe",
					"kind": "installer",
				},
				"payloads": []map[string]any{
					{
						"path":      "dist/payload/dec-console-windows-amd64/",
						"selectors": map[string]string{"os": "windows", "arch": "amd64", "component": "console", "audience": "user"},
					},
				},
			},
		},
		"archives": []map[string]string{{"path": "dist/dec-1.13.78-source.zip", "role": "ci-only"}},
	}
	writeManifest(t, root, doc,
		map[string]string{
			"dist/dec-windows-amd64.exe":         "runtime binary",
			"dist/dec-console-windows-amd64.exe": "console installer",
			"dist/dec-1.13.78-source.zip":        "source archive",
		},
		[]string{"dist/payload/dec-windows-amd64/", "dist/payload/dec-console-windows-amd64/"})

	artifacts, err := loadReleaseArtifactsManifest(root, "dist/release-artifacts.json", "1.13.78")
	if err != nil {
		t.Fatalf("expected /2 manifest to load: %v", err)
	}
	if len(artifacts.Groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(artifacts.Groups))
	}
	first := artifacts.Groups[0]
	if first.Install.Kind != "binary" {
		t.Fatalf("expected kind binary, got %q", first.Install.Kind)
	}
	if first.Install.Filename != "dec-windows-amd64.exe" {
		t.Fatalf("expected explicit filename, got %q", first.Install.Filename)
	}
	if len(first.Payloads) != 1 || first.Payloads[0].Filename == "" {
		t.Fatalf("expected payload filename to survive loading: %+v", first.Payloads)
	}
}

func TestLoadReleaseArtifactsManifestRejectsSchema1(t *testing.T) {
	root := t.TempDir()
	// The /1 wire form (comma-joined selector strings) was removed with the
	// "fully structured, no special cases" decision: /2 is the only accepted
	// release manifest schema, and /1 manifests fail fast with a schema hint.
	doc := map[string]any{
		"schema":  "relkit.release-artifacts/1",
		"version": "1.0.0",
		"install": map[string]string{"path": "dist/app-setup.exe", "kind": "installer", "selectors": "os=windows,arch=amd64"},
		"payload": map[string]string{"path": "dist/payload/app/", "selectors": "os=windows,arch=amd64"},
		"archives": []map[string]string{},
	}
	writeManifest(t, root, doc,
		map[string]string{"dist/app-setup.exe": "installer bytes"},
		[]string{"dist/payload/app/"})

	_, err := loadReleaseArtifactsManifest(root, "dist/release-artifacts.json", "1.0.0")
	if err == nil {
		t.Fatal("expected /1 manifest to be rejected")
	}
	if !strings.Contains(err.Error(), "relkit.release-artifacts/2") {
		t.Fatalf("expected schema hint in error, got %q", err.Error())
	}
}

func TestLoadReleaseArtifactsManifestSchema2RejectsBadKind(t *testing.T) {
	root := t.TempDir()
	doc := map[string]any{
		"schema":  "relkit.release-artifacts/2",
		"version": "1.13.78",
		"selectorGroups": []map[string]any{
			{
				"selectors": map[string]string{"os": "windows"},
				"install":   map[string]string{"path": "dist/app.exe", "kind": "archive"},
				"payloads":  []map[string]any{{"path": "dist/payload/app/", "selectors": map[string]string{"os": "windows"}}},
			},
		},
		"archives": []map[string]string{},
	}
	writeManifest(t, root, doc,
		map[string]string{"dist/app.exe": "bytes"},
		[]string{"dist/payload/app/"})

	_, err := loadReleaseArtifactsManifest(root, "dist/release-artifacts.json", "1.13.78")
	if err == nil {
		t.Fatal("expected kind=archive to be rejected")
	}
	if !strings.Contains(err.Error(), "installer, binary, or blob") {
		t.Fatalf("expected kind rejection message, got %q", err.Error())
	}
}

func TestLoadReleaseArtifactsManifestSchema2RejectsMissingInstall(t *testing.T) {
	root := t.TempDir()
	doc := map[string]any{
		"schema":  "relkit.release-artifacts/2",
		"version": "1.13.78",
		"selectorGroups": []map[string]any{
			{
				"selectors": map[string]string{"os": "windows"},
				"payloads":  []map[string]any{{"path": "dist/payload/app/", "selectors": map[string]string{"os": "windows"}}},
			},
		},
		"archives": []map[string]string{},
	}
	writeManifest(t, root, doc,
		map[string]string{},
		[]string{"dist/payload/app/"})

	_, err := loadReleaseArtifactsManifest(root, "dist/release-artifacts.json", "1.13.78")
	if err == nil {
		t.Fatal("expected missing install to be rejected")
	}
}

func TestLoadReleaseArtifactsManifestSchema2InstallOnlyGroup(t *testing.T) {
	root := t.TempDir()
	// dec's publish face: console installers and the runtime manifest blob
	// stage no payload — install-only groups are the /2 expression of that.
	doc := map[string]any{
		"schema":  "relkit.release-artifacts/2",
		"version": "1.13.78",
		"selectorGroups": []map[string]any{
			{
				"selectors": map[string]string{"os": "windows", "arch": "amd64", "component": "console", "audience": "user"},
				"install": map[string]any{
					"path": "dist/dec-console-windows-amd64.exe",
					"kind": "installer",
				},
			},
			{
				"selectors": map[string]string{"component": "manifest", "audience": "runtime"},
				"install": map[string]any{
					"path":     "dist/dec-runtime-manifest.json",
					"kind":     "blob",
					"filename": "dec-runtime-manifest.json",
				},
				"payloads": []map[string]any{},
			},
		},
		"archives": []map[string]string{},
	}
	writeManifest(t, root, doc,
		map[string]string{
			"dist/dec-console-windows-amd64.exe": "console installer",
			"dist/dec-runtime-manifest.json":     "{}",
		},
		nil)

	artifacts, err := loadReleaseArtifactsManifest(root, "dist/release-artifacts.json", "1.13.78")
	if err != nil {
		t.Fatalf("expected install-only groups to load: %v", err)
	}
	if len(artifacts.Groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(artifacts.Groups))
	}
	if len(artifacts.Groups[0].Payloads) != 0 || len(artifacts.Groups[1].Payloads) != 0 {
		t.Fatalf("expected no payloads in install-only groups, got %+v", artifacts.Groups)
	}
}

func TestLoadReleaseArtifactsManifestSchema2CronkitShape(t *testing.T) {
	root := t.TempDir()
	// cronkit's publish face after the /2 unification: a single group for the
	// win-x64 product — installer install + versionedDir payload, explicit
	// filenames, structured selectors. One group is the /2 expression of a
	// single-target product, not a schema special case.
	doc := map[string]any{
		"schema":  "relkit.release-artifacts/2",
		"version": "0.1.0+15",
		"selectorGroups": []map[string]any{
			{
				"selectors": map[string]string{"os": "windows", "arch": "x64"},
				"install": map[string]any{
					"path":     "dist/cronkit-0.1.0+15-win-x64-setup.exe",
					"kind":     "installer",
					"filename": "cronkit-0.1.0+15-win-x64-setup.exe",
				},
				"payloads": []map[string]any{
					{
						"path":      ".release/versioned/versions/0.1.0+15",
						"filename":  "cronkit-0.1.0+15-win-x64-payload.zip",
						"selectors": map[string]string{"os": "windows", "arch": "x64"},
					},
				},
			},
		},
		"archives": []map[string]string{{"path": "dist/cronkit-0.1.0+15-win-x64.zip", "role": "ci-only"}},
	}
	writeManifest(t, root, doc,
		map[string]string{
			"dist/cronkit-0.1.0+15-win-x64-setup.exe": "installer bytes",
			"dist/cronkit-0.1.0+15-win-x64.zip":       "archive bytes",
		},
		[]string{".release/versioned/versions/0.1.0+15"})

	artifacts, err := loadReleaseArtifactsManifest(root, "dist/release-artifacts.json", "0.1.0+15")
	if err != nil {
		t.Fatalf("expected cronkit /2 shape to load: %v", err)
	}
	if len(artifacts.Groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(artifacts.Groups))
	}
	group := artifacts.Groups[0]
	if group.Selectors["os"] != "windows" || group.Selectors["arch"] != "x64" {
		t.Fatalf("expected structured selectors, got %v", group.Selectors)
	}
	if group.Install.Kind != "installer" {
		t.Fatalf("expected kind installer, got %q", group.Install.Kind)
	}
	if len(group.Payloads) != 1 || group.Payloads[0].Filename != "cronkit-0.1.0+15-win-x64-payload.zip" {
		t.Fatalf("expected explicit payload filename, got %+v", group.Payloads)
	}
	if len(artifacts.Archives) != 1 || artifacts.Archives[0].Role != "ci-only" {
		t.Fatalf("expected ci-only archive, got %+v", artifacts.Archives)
	}
}

func TestLoadReleaseArtifactsManifestRejectsUnknownSchema(t *testing.T) {
	root := t.TempDir()
	doc := map[string]any{
		"schema":  "relkit.release-artifacts/3",
		"version": "1.0.0",
		"archives": []map[string]string{},
	}
	writeManifest(t, root, doc, map[string]string{}, nil)

	_, err := loadReleaseArtifactsManifest(root, "dist/release-artifacts.json", "1.0.0")
	if err == nil {
		t.Fatal("expected unknown schema to be rejected")
	}
	if !strings.Contains(err.Error(), "relkit.release-artifacts/2") {
		t.Fatalf("expected schema hint in error, got %q", err.Error())
	}
}

func TestInstallPairsTextDeterministic(t *testing.T) {
	group := ciSelectorGroup{
		Selectors: map[string]string{"os": "windows", "arch": "amd64"},
		Install:   ciArtifactRef{Path: "dist/app.exe", Kind: "binary", Filename: "app.exe"},
	}
	pairs := installPairsText(group)
	if len(pairs) != 1 {
		t.Fatalf("expected single joined pair string, got %v", pairs)
	}
	if pairs[0] != "kind=binary,filename=app.exe,arch=amd64,os=windows" {
		t.Fatalf("unexpected pairs text: %q", pairs[0])
	}
}
