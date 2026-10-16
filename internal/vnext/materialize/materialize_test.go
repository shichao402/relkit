package materialize

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/shichao402/relkit/internal/vnext/contracts"
	"github.com/shichao402/relkit/internal/vnext/operator"
	"github.com/shichao402/relkit/internal/vnext/resolution"
)

func TestRunMaterializesPinnedBinary(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "fixture-relkit")
	content := []byte("fake relkit binary\n")
	if err := os.WriteFile(source, content, 0o755); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(content)
	artifactDigest := "sha256:" + fmt.Sprintf("%x", sum[:])
	controlDigest := "sha256:" + strings.Repeat("a", 64)
	value := resolution.Document{
		Schema:               resolution.Schema,
		Identity:             resolution.IdentityProduction,
		AuthorityFingerprint: controlDigest,
		AllowedChannels:      []string{"dev"},
		ProjectDigest:        controlDigest,
		Operator: resolution.OperatorProof{
			DeploymentID:    "test",
			ProfileDigest:   controlDigest,
			Sequence:        1,
			SelectionPolicy: operator.SelectionExact,
		},
		CatalogSnapshotDigest: controlDigest,
		SelectedDistribution: resolution.SelectedDistribution{
			Version: "v1.0.0",
			Commit:  strings.Repeat("b", 40),
			Digest:  controlDigest,
		},
		Components: []resolution.Component{
			{
				Name:     "cli",
				Version:  "v1.0.0",
				Platform: "darwin-arm64",
				SHA256:   artifactDigest,
				Source:   "release-asset",
				Locator:  "https://public.invalid/fixture-relkit",
				Mirrors:  []string{source},
			},
		},
		Contracts: []contracts.Capability{
			{Domain: contracts.DomainPublishProtocol, Peer: "agent", Min: 2, Max: 3},
		},
	}
	if err := value.Finalize(); err != nil {
		t.Fatal(err)
	}
	result, err := Run(context.Background(), value, Options{ProjectRoot: root, Target: "darwin-arm64"})
	if err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(root, ".relkit", "components", "bin", "relkit")
	if result.Paths["cli"] != destination {
		t.Fatalf("cli path = %q, want %q", result.Paths["cli"], destination)
	}
	if got := fileDigest(destination); got != artifactDigest {
		t.Fatalf("installed digest = %s, want %s", got, artifactDigest)
	}
	marker, err := os.ReadFile(filepath.Join(root, ".relkit", "components", ".resolution-digest"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(marker)) != value.Digest {
		t.Fatalf("marker = %q, want %q", marker, value.Digest)
	}
}

func TestSelectHostTargetUsesUniversalDarwinCLI(t *testing.T) {
	resolved := resolution.Document{
		Components: []resolution.Component{
			{Name: "cli", Platform: "darwin-universal"},
			{Name: "sdk-dart", Platform: "all"},
		},
	}
	if got := selectHostTarget(resolved, "darwin-amd64"); got != "darwin-universal" {
		t.Fatalf("target=%s, want darwin-universal", got)
	}
	resolved.Components = append(resolved.Components, resolution.Component{Name: "cli", Platform: "darwin-amd64"})
	if got := selectHostTarget(resolved, "darwin-amd64"); got != "darwin-amd64" {
		t.Fatalf("target=%s, want exact host target", got)
	}
}

func TestRunRejectsModuleFacadeWithoutPinnedH1(t *testing.T) {
	controlDigest := "sha256:" + strings.Repeat("a", 64)
	value := resolution.Document{
		Schema:               resolution.Schema,
		Identity:             resolution.IdentityProduction,
		AuthorityFingerprint: controlDigest,
		ProjectDigest:        controlDigest,
		Operator: resolution.OperatorProof{
			DeploymentID:    "test",
			ProfileDigest:   controlDigest,
			Sequence:        1,
			SelectionPolicy: operator.SelectionExact,
		},
		CatalogSnapshotDigest: controlDigest,
		SelectedDistribution: resolution.SelectedDistribution{
			Version: "v1.0.0",
			Commit:  strings.Repeat("b", 40),
			Digest:  controlDigest,
		},
		Components: []resolution.Component{
			{Name: "sdk-go", Version: "v1.0.0", Platform: "all", SHA256: controlDigest, Source: "module", Locator: "github.com/example/tool"},
		},
		Contracts: []contracts.Capability{
			{Domain: contracts.DomainPublishProtocol, Peer: "agent", Min: 2, Max: 3},
		},
	}
	if err := value.Finalize(); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(context.Background(), value, Options{ProjectRoot: t.TempDir(), Target: "darwin-arm64"}); err == nil || !strings.Contains(err.Error(), "moduleH1") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunCreatesVerifiedTemporaryGoModOverlay(t *testing.T) {
	root := t.TempDir()
	t.Chdir(t.TempDir())
	originalMod := "module example.com/host\n\ngo 1.24\n"
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte(originalMod), 0o644); err != nil {
		t.Fatal(err)
	}
	proxy := filepath.Join(t.TempDir(), "proxy")
	modulePath := "example.com/relkit"
	version := "v1.0.0"
	versionDir := filepath.Join(proxy, "example.com", "relkit", "@v")
	if err := os.MkdirAll(versionDir, 0o755); err != nil {
		t.Fatal(err)
	}
	moduleFile := "module " + modulePath + "\n\ngo 1.24\n"
	if err := os.WriteFile(filepath.Join(versionDir, version+".mod"), []byte(moduleFile), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(versionDir, version+".info"), []byte(`{"Version":"v1.0.0","Time":"2026-01-01T00:00:00Z"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	zipPath := filepath.Join(versionDir, version+".zip")
	zipFile, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	zipWriter := zip.NewWriter(zipFile)
	entry, err := zipWriter.Create(modulePath + "@" + version + "/go.mod")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte(moduleFile)); err != nil {
		t.Fatal(err)
	}
	if err := zipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := zipFile.Close(); err != nil {
		t.Fatal(err)
	}
	proxyPath := filepath.ToSlash(proxy)
	if runtime.GOOS == "windows" && !strings.HasPrefix(proxyPath, "/") {
		proxyPath = "/" + proxyPath
	}
	t.Setenv("GOPROXY", (&url.URL{Scheme: "file", Path: proxyPath}).String())
	t.Setenv("GOSUMDB", "off")
	moduleCacheRoot := t.TempDir()
	moduleCache := filepath.Join(moduleCacheRoot, "gomodcache")
	t.Setenv("GOMODCACHE", moduleCache)
	t.Cleanup(func() {
		_ = filepath.Walk(moduleCacheRoot, func(path string, info os.FileInfo, err error) error {
			if err == nil {
				_ = os.Chmod(path, info.Mode()|0o700)
			}
			return nil
		})
	})
	output, err := exec.Command("go", "mod", "download", "-json", modulePath+"@"+version).CombinedOutput()
	if err != nil {
		t.Fatalf("seed module download: %v\n%s", err, output)
	}
	var downloaded struct {
		Sum string `json:"Sum"`
	}
	if err := json.Unmarshal(output, &downloaded); err != nil {
		t.Fatal(err)
	}
	if downloaded.Sum == "" {
		t.Fatalf("module download did not return h1: %s", output)
	}

	controlDigest := "sha256:" + strings.Repeat("c", 64)
	value := resolution.Document{
		Schema:               resolution.Schema,
		Identity:             resolution.IdentityProduction,
		AuthorityFingerprint: controlDigest,
		ProjectDigest:        controlDigest,
		Operator: resolution.OperatorProof{
			DeploymentID:    "test",
			ProfileDigest:   controlDigest,
			Sequence:        1,
			SelectionPolicy: operator.SelectionExact,
		},
		CatalogSnapshotDigest: controlDigest,
		SelectedDistribution: resolution.SelectedDistribution{
			Version:    version,
			Commit:     strings.Repeat("d", 40),
			Digest:     controlDigest,
			ModulePath: modulePath,
			ModuleH1:   downloaded.Sum,
		},
		Components: []resolution.Component{
			{Name: "sdk-go", Version: version, Platform: "all", SHA256: controlDigest, Source: "module", Locator: modulePath},
		},
		Contracts: []contracts.Capability{
			{Domain: contracts.DomainUpdaterIPC, Peer: "updater", Min: 3, Max: 3},
		},
	}
	if err := value.Finalize(); err != nil {
		t.Fatal(err)
	}
	result, err := Run(context.Background(), value, Options{ProjectRoot: root, Target: "darwin-arm64"})
	if err != nil {
		t.Fatal(err)
	}
	if result.GoModFile == "" {
		t.Fatal("temporary Go modfile was not reported")
	}
	overlay, err := os.ReadFile(result.GoModFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(overlay), "require "+modulePath+" "+version) {
		t.Fatalf("overlay does not pin selected module:\n%s", overlay)
	}
	unchanged, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	if string(unchanged) != originalMod {
		t.Fatalf("host go.mod was modified:\n%s", unchanged)
	}
}

func TestInstallGoArchiveOverlayUsesMaterializedSDK(t *testing.T) {
	root := t.TempDir()
	t.Chdir(t.TempDir())
	originalMod := "module example.com/host\n\ngo 1.24\n"
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte(originalMod), 0o644); err != nil {
		t.Fatal(err)
	}
	hostSum := "example.com/host-dependency v1.0.0 h1:host\n"
	if err := os.WriteFile(filepath.Join(root, "go.sum"), []byte(hostSum), 0o644); err != nil {
		t.Fatal(err)
	}
	sdkRoot := filepath.Join(root, ".relkit", "components", "sdk", "go")
	if err := os.MkdirAll(sdkRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(sdkRoot, "go.mod"),
		[]byte("module github.com/shichao402/relkit\n\ngo 1.24\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	sdkSum := "example.com/sdk-dependency v2.0.0 h1:sdk\n"
	if err := os.WriteFile(filepath.Join(sdkRoot, "go.sum"), []byte(sdkSum), 0o644); err != nil {
		t.Fatal(err)
	}

	modFile, err := installGoArchiveOverlay(context.Background(), root, sdkRoot)
	if err != nil {
		t.Fatal(err)
	}
	overlay, err := os.ReadFile(modFile)
	if err != nil {
		t.Fatal(err)
	}
	text := string(overlay)
	if !strings.Contains(text, "require github.com/shichao402/relkit v0.0.0") {
		t.Fatalf("overlay does not require materialized SDK:\n%s", text)
	}
	relativeSDK, err := filepath.Rel(root, sdkRoot)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "replace github.com/shichao402/relkit => ./"+filepath.ToSlash(relativeSDK)) {
		t.Fatalf("overlay does not replace materialized SDK:\n%s", text)
	}
	overlaySum, err := os.ReadFile(filepath.Join(sdkRoot, "relkit.sum"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(overlaySum), strings.TrimSpace(hostSum)) ||
		!strings.Contains(string(overlaySum), strings.TrimSpace(sdkSum)) {
		t.Fatalf("overlay sum does not contain host and SDK checksums:\n%s", overlaySum)
	}
	unchanged, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	if string(unchanged) != originalMod {
		t.Fatalf("host go.mod was modified:\n%s", unchanged)
	}
}
