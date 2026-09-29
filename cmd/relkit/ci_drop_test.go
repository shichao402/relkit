package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// dropFixture is an in-memory agent drop: it records PUT bodies and serves
// them back on GET, mirroring the agent's /v1/drop/{product}/{version}/
// {filename} endpoints (PUT/GET/DELETE, Bearer auth) closely enough to
// exercise the client without a real agent.
type dropFixture struct {
	mu     chan struct{}
	files  map[string][]byte
	server *httptest.Server
	tokens map[string]bool
}

func newDropFixture(t *testing.T) *dropFixture {
	t.Helper()
	fixture := &dropFixture{
		mu:     make(chan struct{}, 1),
		files:  map[string][]byte{},
		tokens: map[string]bool{"token-ok": true},
	}
	fixture.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/v1/drop/") {
			http.Error(w, "unexpected path", http.StatusBadRequest)
			return
		}
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/v1/drop/"), "/")
		if len(parts) != 3 {
			http.Error(w, "expected /v1/drop/{product}/{version}/{filename}", http.StatusBadRequest)
			return
		}
		if r.Header.Get("Authorization") != "Bearer token-ok" {
			http.Error(w, "bad token", http.StatusUnauthorized)
			return
		}
		key := strings.Join(parts, "/")
		fixture.mu <- struct{}{}
		defer func() { <-fixture.mu }()
		switch r.Method {
		case http.MethodPut:
			body, _ := io.ReadAll(r.Body)
			fixture.files[key] = body
			w.WriteHeader(http.StatusCreated)
			fmt.Fprintf(w, `{"file":%q}`, parts[2])
		case http.MethodGet:
			data, ok := fixture.files[key]
			if !ok {
				http.Error(w, "missing", http.StatusNotFound)
				return
			}
			_, _ = w.Write(data)
		case http.MethodDelete:
			delete(fixture.files, key)
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	t.Cleanup(fixture.server.Close)
	return fixture
}

func (f *dropFixture) setEnv(t *testing.T, root, product, version string) {
	t.Helper()
	t.Setenv("RELKIT_UPLOAD_TOKEN", "token-ok")
	t.Setenv("RELKIT_AGENT_URL", f.server.URL)
	relkitJSON := map[string]any{"product": product, "agent": map[string]any{"url": f.server.URL}}
	data, _ := json.Marshal(relkitJSON)
	if err := os.WriteFile(filepath.Join(root, "relkit.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func writePayloadTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for relPath, content := range files {
		abs := filepath.Join(root, filepath.FromSlash(relPath))
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestValidateDropScope(t *testing.T) {
	for _, scope := range []string{"bk-123", "build.42", "A_b-c"} {
		if err := validateDropScope(scope); err != nil {
			t.Fatalf("expected %q to pass, got %v", scope, err)
		}
	}
	for _, scope := range []string{"", "bad scope", "with/slash", "..", "a b"} {
		if err := validateDropScope(scope); err == nil {
			t.Fatalf("expected %q to be rejected", scope)
		}
	}
}

func TestValidateDropPlatform(t *testing.T) {
	if err := validateDropPlatform("windows"); err != nil {
		t.Fatal(err)
	}
	if err := validateDropPlatform("macos"); err != nil {
		t.Fatal(err)
	}
	for _, platform := range []string{"", "Windows", "win/dows", "-leading"} {
		if err := validateDropPlatform(platform); err == nil {
			t.Fatalf("expected %q to be rejected", platform)
		}
	}
}

func TestDropManifestName(t *testing.T) {
	if got, want := dropManifestName("bk-1", "windows"), "bk-1--release-artifacts-windows.json"; got != want {
		t.Fatalf("dropManifestName = %q, want %q", got, want)
	}
	if got, want := scopedDropName("bk-1", "app.zip"), "bk-1--app.zip"; got != want {
		t.Fatalf("scopedDropName = %q, want %q", got, want)
	}
}

func TestZipDirRoundTrip(t *testing.T) {
	root := t.TempDir()
	writePayloadTree(t, root, map[string]string{
		"versions/0.2.0+165/app.exe": "binary-ish",
		"versions/0.2.0+165/data/config.ini": "config",
	})
	zipPath := filepath.Join(t.TempDir(), "payload.zip")
	count, err := zipDir(filepath.Join(root), zipPath)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("expected 2 entries, got %d", count)
	}
	dest := filepath.Join(t.TempDir(), "out")
	written, err := unpackPayloadZip(zipPath, dest)
	if err != nil {
		t.Fatal(err)
	}
	if written != 2 {
		t.Fatalf("expected 2 written, got %d", written)
	}
	for relPath, content := range map[string]string{
		"versions/0.2.0+165/app.exe":       "binary-ish",
		"versions/0.2.0+165/data/config.ini": "config",
	} {
		data, err := os.ReadFile(filepath.Join(dest, filepath.FromSlash(relPath)))
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != content {
			t.Fatalf("%s: got %q, want %q", relPath, string(data), content)
		}
	}
}

func TestUnpackPayloadZipRejectsTraversal(t *testing.T) {
	root := t.TempDir()
	zipPath := filepath.Join(root, "evil.zip")
	file, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	entry, err := writer.Create("../escape.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte("bad")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := unpackPayloadZip(zipPath, filepath.Join(root, "dest")); err == nil {
		t.Fatal("expected traversal to be rejected")
	}
}

func TestUnpackPayloadZipRejectsAbsolute(t *testing.T) {
	root := t.TempDir()
	zipPath := filepath.Join(root, "evil2.zip")
	file, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	entry, err := writer.Create("/etc/passwd")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte("bad")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := unpackPayloadZip(zipPath, filepath.Join(root, "dest")); err == nil {
		t.Fatal("expected absolute path to be rejected")
	}
}

func TestSelectorMapsEqualAndKey(t *testing.T) {
	a := map[string]string{"os": "windows", "arch": "x64"}
	b := map[string]string{"arch": "x64", "os": "windows"}
	if !selectorMapsEqual(a, b) {
		t.Fatal("expected equal maps")
	}
	if selectorMapsEqual(a, map[string]string{"os": "macos"}) {
		t.Fatal("expected unequal maps")
	}
	if got, want := selectorsKey(a), "arch=x64,os=windows"; got != want {
		t.Fatalf("selectorsKey = %q, want %q", got, want)
	}
}

// partialManifest builds a /2 manifest doc for one platform.
func partialManifest(t *testing.T, version, platform string, withPayload bool) map[string]any {
	t.Helper()
	selectors := map[string]string{"os": platform}
	if platform == "windows" {
		selectors["arch"] = "x64"
	}
	doc := map[string]any{
		"schema":  "relkit.release-artifacts/2",
		"version": version,
		"selectorGroups": []map[string]any{
			{
				"selectors": selectors,
				"install": map[string]any{
					"path":     "dist/app-" + platform + "-setup" + map[bool]string{true: ".exe", false: ".dmg"}[platform == "windows"],
					"kind":     "installer",
					"filename": "app-" + platform + "-setup" + map[bool]string{true: ".exe", false: ".dmg"}[platform == "windows"],
				},
			},
		},
	}
	if withPayload {
		doc["selectorGroups"].([]map[string]any)[0]["payloads"] = []map[string]any{
			{
				"path":      ".release/versioned/versions/" + version,
				"filename":  "app-" + platform + "-payload.zip",
				"selectors": selectors,
			},
		}
	}
	return doc
}

func TestParseReleaseArtifactsDoc(t *testing.T) {
	doc := partialManifest(t, "0.2.0+165", "windows", true)
	data, _ := json.Marshal(doc)
	artifacts, err := parseReleaseArtifactsDoc(data, "0.2.0+165")
	if err != nil {
		t.Fatal(err)
	}
	if len(artifacts.Groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(artifacts.Groups))
	}
	group := artifacts.Groups[0]
	if group.Install.Kind != "installer" || group.Install.Filename == "" {
		t.Fatalf("bad install ref: %+v", group.Install)
	}
	if len(group.Payloads) != 1 || group.Payloads[0].Filename != "app-windows-payload.zip" {
		t.Fatalf("bad payloads: %+v", group.Payloads)
	}
	if _, err := parseReleaseArtifactsDoc(data, "9.9.9+1"); err == nil {
		t.Fatal("expected version mismatch to be rejected")
	}
	badSchema := map[string]any{"schema": "relkit.release-artifacts/1"}
	badData, _ := json.Marshal(badSchema)
	if _, err := parseReleaseArtifactsDoc(badData, "0.2.0+165"); err == nil {
		t.Fatal("expected /1 schema to be rejected")
	}
}

func TestCheckGroupOverlap(t *testing.T) {
	windowsGroup := ciSelectorGroup{Selectors: map[string]string{"os": "windows", "arch": "x64"}}
	macosGroup := ciSelectorGroup{Selectors: map[string]string{"os": "macos"}}
	if err := checkGroupOverlap([]ciSelectorGroup{windowsGroup}, macosGroup); err != nil {
		t.Fatalf("expected distinct profiles to union: %v", err)
	}
	if err := checkGroupOverlap([]ciSelectorGroup{windowsGroup}, windowsGroup); err == nil {
		t.Fatal("expected duplicate profile to be rejected")
	}
}

func TestDropClientRoundTrip(t *testing.T) {
	fixture := newDropFixture(t)
	root := t.TempDir()
	fixture.setEnv(t, root, "svn-auto-merge", "0.2.0+165")
	client, err := newDropClient(root, "svn-auto-merge", "0.2.0+165", "bk-42", "token-ok")
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "artifact.zip")
	if err := os.WriteFile(source, []byte("zip-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := client.put("bk-42--artifact.zip", source); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(t.TempDir(), "back.zip")
	if err := client.get("bk-42--artifact.zip", dest); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(dest)
	if string(data) != "zip-bytes" {
		t.Fatalf("round trip corrupted data: %q", string(data))
	}
	if err := client.delete("bk-42--artifact.zip"); err != nil {
		t.Fatal(err)
	}
	if err := client.get("bk-42--artifact.zip", dest); err == nil {
		t.Fatal("expected 404 after delete")
	}
}

func TestDropClientRejectsBadScope(t *testing.T) {
	root := t.TempDir()
	if _, err := newDropClient(root, "p", "1.0.0", "bad scope", "token"); err == nil {
		t.Fatal("expected bad scope to be rejected")
	}
	if _, err := newDropClient(root, "p", "1.0.0", "ok-scope", ""); err == nil {
		t.Fatal("expected missing token to be rejected")
	}
}

func TestDropClientUnauthorized(t *testing.T) {
	fixture := newDropFixture(t)
	root := t.TempDir()
	fixture.setEnv(t, root, "svn-auto-merge", "0.2.0+165")
	client, err := newDropClient(root, "svn-auto-merge", "0.2.0+165", "bk-42", "token-wrong")
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "f.zip")
	if err := os.WriteFile(source, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := client.put("bk-42--f.zip", source); err == nil {
		t.Fatal("expected unauthorized put to fail")
	}
}

func TestDropPlatformsConfig(t *testing.T) {
	root := t.TempDir()
	relkitJSON := map[string]any{
		"product": "p",
		"release": map[string]any{"dropPlatforms": []any{"windows", "macos"}},
	}
	data, _ := json.Marshal(relkitJSON)
	if err := os.WriteFile(filepath.Join(root, "relkit.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	platforms, err := dropPlatforms(root, "")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(platforms, []string{"windows", "macos"}) {
		t.Fatalf("platforms = %v", platforms)
	}
	if _, err := dropPlatforms(root, "linux"); err != nil {
		t.Fatal(err)
	}
	empty := t.TempDir()
	if _, err := dropPlatforms(empty, ""); err == nil {
		t.Fatal("expected missing platform list to be rejected")
	}
}

func TestAssembleFromDropEndToEnd(t *testing.T) {
	fixture := newDropFixture(t)
	root := t.TempDir()
	fixture.setEnv(t, root, "svn-auto-merge", "0.2.0+165")
	version := "0.2.0+165"
	scope := "bk-77"

	// Windows platform job: install + payload tree, pushed via uploadArtifacts.
	writePayloadTree(t, root, map[string]string{
		"dist/SvnAutoMerge_windows_0.2.0+165_setup.exe": "setup-bytes",
		".release/versioned/versions/" + version + "/app.exe": "app-bytes",
		".release/versioned/versions/" + version + "/data/config.ini": "config",
	})
	windowsManifest := partialManifest(t, version, "windows", true)
	windowsManifest["selectorGroups"].([]map[string]any)[0]["install"].(map[string]any)["path"] = "dist/SvnAutoMerge_windows_0.2.0+165_setup.exe"
	windowsManifest["selectorGroups"].([]map[string]any)[0]["install"].(map[string]any)["filename"] = "SvnAutoMerge_windows_0.2.0+165_setup.exe"
	windowsManifest["selectorGroups"].([]map[string]any)[0]["payloads"].([]map[string]any)[0]["filename"] = "SvnAutoMerge_windows_0.2.0+165_payload.zip"
	manifestData, _ := json.Marshal(windowsManifest)
	manifestPath := filepath.Join(root, "dist", "release-artifacts.json")
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, manifestData, 0o644); err != nil {
		t.Fatal(err)
	}
	artifacts, err := parseReleaseArtifactsDoc(manifestData, version)
	if err != nil {
		t.Fatal(err)
	}
	client, err := newDropClient(root, "svn-auto-merge", version, scope, "token-ok")
	if err != nil {
		t.Fatal(err)
	}
	if err := uploadArtifacts(client, root, artifacts, "dist/release-artifacts.json", scope, "windows"); err != nil {
		t.Fatal(err)
	}

	// macOS platform job: install only.
	writePayloadTree(t, root, map[string]string{
		"dist/SvnAutoMerge_macos_0.2.0+165.dmg": "dmg-bytes",
	})
	macosManifest := partialManifest(t, version, "macos", false)
	macosManifest["selectorGroups"].([]map[string]any)[0]["install"].(map[string]any)["path"] = "dist/SvnAutoMerge_macos_0.2.0+165.dmg"
	macosManifest["selectorGroups"].([]map[string]any)[0]["install"].(map[string]any)["filename"] = "SvnAutoMerge_macos_0.2.0+165.dmg"
	macosData, _ := json.Marshal(macosManifest)
	macosPath := filepath.Join(root, "dist", "release-artifacts.json")
	if err := os.WriteFile(macosPath, macosData, 0o644); err != nil {
		t.Fatal(err)
	}
	macosArtifacts, err := parseReleaseArtifactsDoc(macosData, version)
	if err != nil {
		t.Fatal(err)
	}
	if err := uploadArtifacts(client, root, macosArtifacts, "dist/release-artifacts.json", scope, "macos"); err != nil {
		t.Fatal(err)
	}

	// Fresh workspace for the aggregate job (drop files are re-downloaded).
	aggRoot := t.TempDir()
	aggJSON := map[string]any{
		"product": "svn-auto-merge",
		"agent":   map[string]any{"url": fixture.server.URL},
		"release": map[string]any{"dropPlatforms": []any{"windows", "macos"}},
	}
	aggData, _ := json.Marshal(aggJSON)
	if err := os.WriteFile(filepath.Join(aggRoot, "relkit.json"), aggData, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("RELKIT_UPLOAD_TOKEN", "token-ok")
	t.Setenv("RELKIT_AGENT_URL", fixture.server.URL)

	groups, cleanup, err := assembleFromDrop(aggRoot, version, scope, []string{"windows", "macos"})
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}
	// The install artifacts and payload tree are materialized at their
	// declared paths.
	if data, err := os.ReadFile(filepath.Join(aggRoot, "dist", "SvnAutoMerge_windows_0.2.0+165_setup.exe")); err != nil || string(data) != "setup-bytes" {
		t.Fatalf("windows install not materialized: %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(aggRoot, "dist", "SvnAutoMerge_macos_0.2.0+165.dmg")); err != nil || string(data) != "dmg-bytes" {
		t.Fatalf("macos install not materialized: %v", err)
	}
	payloadExe := filepath.Join(aggRoot, ".release", "versioned", "versions", version, "app.exe")
	if data, err := os.ReadFile(payloadExe); err != nil || string(data) != "app-bytes" {
		t.Fatalf("payload tree not materialized: %v", err)
	}
	expectedCleanup := map[string]bool{
		"bk-77--release-artifacts-windows.json":     true,
		"bk-77--release-artifacts-macos.json":       true,
		"bk-77--SvnAutoMerge_windows_0.2.0+165_setup.exe": true,
		"bk-77--SvnAutoMerge_windows_0.2.0+165_payload.zip": true,
		"bk-77--SvnAutoMerge_macos_0.2.0+165.dmg":   true,
	}
	for _, name := range cleanup {
		if !expectedCleanup[name] {
			t.Fatalf("unexpected cleanup name %q", name)
		}
		delete(expectedCleanup, name)
	}
	if len(expectedCleanup) != 0 {
		t.Fatalf("missing cleanup names: %v", expectedCleanup)
	}

	// Cleanup empties the scope's drop files.
	cleanupDropFiles(aggRoot, version, scope, cleanup)
	for _, pm := range []string{"windows", "macos"} {
		remaining := false
		fixture.mu <- struct{}{}
		for key := range fixture.files {
			if strings.Contains(key, "bk-77--") {
				remaining = true
			}
		}
		<-fixture.mu
		if remaining {
			t.Fatalf("drop files remain after cleanup (%s)", pm)
		}
	}
}

func TestAssembleFromDropRejectsDuplicateProfile(t *testing.T) {
	fixture := newDropFixture(t)
	root := t.TempDir()
	fixture.setEnv(t, root, "svn-auto-merge", "0.2.0+165")
	version := "0.2.0+165"
	scope := "bk-78"

	for _, platform := range []string{"windows", "macos"} {
		writePayloadTree(t, root, map[string]string{
			"dist/app-" + platform + "-setup.exe": "setup",
		})
		doc := partialManifest(t, version, platform, false)
		doc["selectorGroups"].([]map[string]any)[0]["selectors"] = map[string]string{"os": "windows", "arch": "x64"}
		doc["selectorGroups"].([]map[string]any)[0]["install"].(map[string]any)["path"] = "dist/app-" + platform + "-setup.exe"
		doc["selectorGroups"].([]map[string]any)[0]["install"].(map[string]any)["filename"] = "app-" + platform + "-setup.exe"
		data, _ := json.Marshal(doc)
		manifestPath := filepath.Join(root, "dist", "release-artifacts.json")
		if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(manifestPath, data, 0o644); err != nil {
			t.Fatal(err)
		}
		artifacts, err := parseReleaseArtifactsDoc(data, version)
		if err != nil {
			t.Fatal(err)
		}
		client, err := newDropClient(root, "svn-auto-merge", version, scope, "token-ok")
		if err != nil {
			t.Fatal(err)
		}
		if err := uploadArtifacts(client, root, artifacts, "dist/release-artifacts.json", scope, platform); err != nil {
			t.Fatal(err)
		}
	}
	aggRoot := t.TempDir()
	aggJSON := map[string]any{
		"product": "svn-auto-merge",
		"agent":   map[string]any{"url": fixture.server.URL},
	}
	aggData, _ := json.Marshal(aggJSON)
	if err := os.WriteFile(filepath.Join(aggRoot, "relkit.json"), aggData, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("RELKIT_UPLOAD_TOKEN", "token-ok")
	if _, _, err := assembleFromDrop(aggRoot, version, scope, []string{"windows", "macos"}); err == nil {
		t.Fatal("expected duplicate selector profile to be rejected")
	}
}

func TestAssembleFromDropMissingPlatform(t *testing.T) {
	fixture := newDropFixture(t)
	root := t.TempDir()
	fixture.setEnv(t, root, "svn-auto-merge", "0.2.0+165")
	aggRoot := t.TempDir()
	aggJSON := map[string]any{
		"product": "svn-auto-merge",
		"agent":   map[string]any{"url": fixture.server.URL},
	}
	aggData, _ := json.Marshal(aggJSON)
	if err := os.WriteFile(filepath.Join(aggRoot, "relkit.json"), aggData, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("RELKIT_UPLOAD_TOKEN", "token-ok")
	if _, _, err := assembleFromDrop(aggRoot, "0.2.0+165", "bk-99", []string{"windows"}); err == nil {
		t.Fatal("expected missing platform manifest to fail")
	}
}

func TestAssembleFromDropVersionMismatch(t *testing.T) {
	fixture := newDropFixture(t)
	root := t.TempDir()
	fixture.setEnv(t, root, "svn-auto-merge", "0.2.0+165")
	version := "0.2.0+165"
	scope := "bk-100"
	doc := partialManifest(t, version, "windows", false)
	writePayloadTree(t, root, map[string]string{
		"dist/app-windows-setup.exe": "setup",
	})
	doc["selectorGroups"].([]map[string]any)[0]["install"].(map[string]any)["path"] = "dist/app-windows-setup.exe"
	data, _ := json.Marshal(doc)
	manifestPath := filepath.Join(root, "dist", "release-artifacts.json")
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	artifacts, err := parseReleaseArtifactsDoc(data, version)
	if err != nil {
		t.Fatal(err)
	}
	client, err := newDropClient(root, "svn-auto-merge", version, scope, "token-ok")
	if err != nil {
		t.Fatal(err)
	}
	if err := uploadArtifacts(client, root, artifacts, "dist/release-artifacts.json", scope, "windows"); err != nil {
		t.Fatal(err)
	}
	aggRoot := t.TempDir()
	aggJSON := map[string]any{
		"product": "svn-auto-merge",
		"agent":   map[string]any{"url": fixture.server.URL},
	}
	aggData, _ := json.Marshal(aggJSON)
	if err := os.WriteFile(filepath.Join(aggRoot, "relkit.json"), aggData, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("RELKIT_UPLOAD_TOKEN", "token-ok")
	if _, _, err := assembleFromDrop(aggRoot, "0.2.0+166", scope, []string{"windows"}); err == nil {
		t.Fatal("expected version mismatch to be rejected")
	}
}

func TestMergeManifestsEquivalent(t *testing.T) {
	windows := partialManifest(t, "1.0.0", "windows", false)
	macos := partialManifest(t, "1.0.0", "macos", false)
	windowsData, _ := json.Marshal(windows)
	macosData, _ := json.Marshal(macos)
	windowsArtifacts, err := parseReleaseArtifactsDoc(windowsData, "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	macosArtifacts, err := parseReleaseArtifactsDoc(macosData, "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	groups := append([]ciSelectorGroup{}, windowsArtifacts.Groups...)
	for _, group := range macosArtifacts.Groups {
		if err := checkGroupOverlap(groups, group); err != nil {
			t.Fatal(err)
		}
		groups = append(groups, group)
	}
	if len(groups) != 2 {
		t.Fatalf("expected 2 merged groups, got %d", len(groups))
	}
}

func TestInstallDropNameRequiresSafeName(t *testing.T) {
	if _, err := installDropName(ciArtifactRef{Path: "dist/app.exe", Kind: "installer", Filename: "sub/dir/app.exe"}); err == nil {
		t.Fatal("expected path separator in filename to be rejected")
	}
	if _, err := payloadDropName(ciPayloadRef{Path: "tree/"}); err == nil {
		t.Fatal("expected payload without filename to be rejected")
	}
	name, err := installDropName(ciArtifactRef{Path: "dist/app.exe", Kind: "installer"})
	if err != nil || name != "app.exe" {
		t.Fatalf("installDropName fallback = %q, %v", name, err)
	}
}

func TestUploadArtifactsPushesManifestLast(t *testing.T) {
	fixture := newDropFixture(t)
	root := t.TempDir()
	fixture.setEnv(t, root, "p", "1.0.0")
	version := "1.0.0"
	scope := "bk-101"

	writePayloadTree(t, root, map[string]string{
		"dist/app-setup.exe": "setup",
		"tree/app.exe":       "app",
	})
	doc := map[string]any{
		"schema":  "relkit.release-artifacts/2",
		"version": version,
		"selectorGroups": []map[string]any{
			{
				"selectors": map[string]string{"os": "windows"},
				"install": map[string]any{
					"path":     "dist/app-setup.exe",
					"kind":     "installer",
					"filename": "app-setup.exe",
				},
				"payloads": []map[string]any{
					{
						"path":      "tree/",
						"filename":  "app-payload.zip",
						"selectors": map[string]string{"os": "windows"},
					},
				},
			},
		},
	}
	data, _ := json.Marshal(doc)
	manifestPath := filepath.Join(root, "dist", "release-artifacts.json")
	if err := os.WriteFile(manifestPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	artifacts, err := parseReleaseArtifactsDoc(data, version)
	if err != nil {
		t.Fatal(err)
	}
	client, err := newDropClient(root, "p", version, scope, "token-ok")
	if err != nil {
		t.Fatal(err)
	}
	if err := uploadArtifacts(client, root, artifacts, "dist/release-artifacts.json", scope, "windows"); err != nil {
		t.Fatal(err)
	}
	fixture.mu <- struct{}{}
	names := make([]string, 0, len(fixture.files))
	for key := range fixture.files {
		names = append(names, key)
	}
	<-fixture.mu
	joined := strings.Join(names, ";")
	for _, expected := range []string{"app-setup.exe", "app-payload.zip", "release-artifacts-windows.json"} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("drop missing %q: %v", expected, names)
		}
	}
}

func TestPayloadZipEmptyRejected(t *testing.T) {
	root := t.TempDir()
	zipPath := filepath.Join(root, "empty.zip")
	if _, err := zipDir(filepath.Join(root, "no-such-dir"), zipPath); err == nil {
		t.Fatal("expected missing dir to fail")
	}
}

func TestZipDirRejectsEmptyTree(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := zipDir(filepath.Join(root, "empty"), filepath.Join(root, "e.zip")); err == nil {
		t.Fatal("expected empty tree to be rejected")
	}
}

// compile-time guards for unused-import hygiene in this test file.
var _ = bytes.MinRead
var _ = http.MethodGet
