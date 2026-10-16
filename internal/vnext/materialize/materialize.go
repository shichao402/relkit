// Package materialize installs exactly the component rows recorded in an
// immutable resolution. It never re-reads a catalog or resolves "latest".
package materialize

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/shichao402/relkit/internal/fsx"
	"github.com/shichao402/relkit/internal/vnext/canonical"
	"github.com/shichao402/relkit/internal/vnext/document"
	vnextregistry "github.com/shichao402/relkit/internal/vnext/registry"
	"github.com/shichao402/relkit/internal/vnext/resolution"
)

type Options struct {
	ProjectRoot string
	Target      string
	Client      *http.Client
}

type Result struct {
	ResolutionDigest string            `json:"resolutionDigest"`
	Target           string            `json:"target"`
	Paths            map[string]string `json:"paths"`
	GoModFile        string            `json:"goModFile,omitempty"`
}

func Run(ctx context.Context, resolved resolution.Document, options Options) (*Result, error) {
	if err := resolved.Validate(); err != nil {
		return nil, err
	}
	root := options.ProjectRoot
	if root == "" {
		root = "."
	}
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	target := options.Target
	if target == "" || target == "host" {
		target, err = HostTarget()
		if err != nil {
			return nil, err
		}
		target = selectHostTarget(resolved, target)
	}
	registry, err := vnextregistry.Default()
	if err != nil {
		return nil, err
	}
	client := options.Client
	if client == nil {
		client = &http.Client{Timeout: 2 * time.Minute}
	}
	result := &Result{
		ResolutionDigest: resolved.Digest,
		Target:           target,
		Paths:            map[string]string{},
	}
	for _, component := range resolved.Components {
		if component.Platform != target && component.Platform != "all" && component.Platform != "any" {
			continue
		}
		definition, ok := registry.ByName(component.Name)
		if !ok {
			return nil, fmt.Errorf("resolution contains unregistered component %q", component.Name)
		}
		destination := filepath.Join(absoluteRoot, filepath.FromSlash(definition.Destination))
		switch definition.Role {
		case vnextregistry.RoleProductBinary:
			name := component.Name
			if component.Name == "cli" {
				name = "relkit"
			} else if component.Name == "updater" {
				name = "relkit-updater"
			}
			if strings.HasPrefix(target, "windows-") {
				name += ".exe"
			}
			destination = filepath.Join(destination, name)
			if component.Source == string(vnextregistry.TransportModule) {
				cached := cachePath(absoluteRoot, component.SHA256)
				if fileDigest(cached) == component.SHA256 {
					if err := installBinary(cached, destination, component.SHA256); err != nil {
						return nil, fmt.Errorf("materialize cached %s: %w", component.Name, err)
					}
				} else if len(component.Mirrors) > 0 {
					artifactPath, mirrorErr := fetchLocators(ctx, client, absoluteRoot, component, component.Mirrors)
					if mirrorErr == nil {
						if err := installBinary(artifactPath, destination, component.SHA256); err != nil {
							return nil, fmt.Errorf("materialize mirrored %s: %w", component.Name, err)
						}
					} else if err := installModuleBinary(
						ctx,
						component,
						destination,
						resolved.SelectedDistribution.ModulePath,
						resolved.SelectedDistribution.ModuleH1,
					); err != nil {
						return nil, fmt.Errorf("materialize %s: mirrors failed (%v); module fallback failed: %w", component.Name, mirrorErr, err)
					}
				} else {
					if err := installModuleBinary(
						ctx,
						component,
						destination,
						resolved.SelectedDistribution.ModulePath,
						resolved.SelectedDistribution.ModuleH1,
					); err != nil {
						return nil, fmt.Errorf("materialize %s: %w", component.Name, err)
					}
				}
			} else {
				artifactPath, err := fetch(ctx, client, absoluteRoot, component)
				if err != nil {
					return nil, err
				}
				if err := installBinary(artifactPath, destination, component.SHA256); err != nil {
					return nil, fmt.Errorf("materialize %s: %w", component.Name, err)
				}
			}
		case vnextregistry.RoleFacade, vnextregistry.RoleProjection:
			if component.Source == string(vnextregistry.TransportModule) {
				if err := installModuleReference(component, destination, resolved.SelectedDistribution.ModuleH1); err != nil {
					return nil, fmt.Errorf("materialize %s: %w", component.Name, err)
				}
				if component.Name == "sdk-go" {
					modFile, err := installGoOverlay(
						ctx,
						absoluteRoot,
						component,
						resolved.SelectedDistribution.ModulePath,
						resolved.SelectedDistribution.ModuleH1,
					)
					if err != nil {
						return nil, fmt.Errorf("materialize %s: %w", component.Name, err)
					}
					result.GoModFile = modFile
				}
			} else {
				artifactPath, err := fetch(ctx, client, absoluteRoot, component)
				if err != nil {
					return nil, err
				}
				if err := installArchive(artifactPath, destination, component.SHA256); err != nil {
					return nil, fmt.Errorf("materialize %s: %w", component.Name, err)
				}
				if component.Name == "sdk-go" {
					modFile, err := installGoArchiveOverlay(ctx, absoluteRoot, destination)
					if err != nil {
						return nil, fmt.Errorf("materialize %s: %w", component.Name, err)
					}
					result.GoModFile = modFile
				}
			}
		default:
			return nil, fmt.Errorf("component %s role %s is not materialized into product repositories", component.Name, definition.Role)
		}
		result.Paths[component.Name] = destination
	}
	if len(result.Paths) == 0 {
		return nil, fmt.Errorf("resolution has no components for target %s", target)
	}
	marker := filepath.Join(absoluteRoot, ".relkit", "components", ".resolution-digest")
	if err := os.MkdirAll(filepath.Dir(marker), 0o755); err != nil {
		return nil, err
	}
	if err := atomicWrite(marker, []byte(resolved.Digest+"\n"), 0o644); err != nil {
		return nil, err
	}
	return result, nil
}

func selectHostTarget(resolved resolution.Document, hostTarget string) string {
	if !strings.HasPrefix(hostTarget, "darwin-") {
		return hostTarget
	}
	hasHostCLI := false
	hasUniversalCLI := false
	for _, component := range resolved.Components {
		if component.Name != "cli" {
			continue
		}
		switch component.Platform {
		case hostTarget:
			hasHostCLI = true
		case "darwin-universal":
			hasUniversalCLI = true
		}
	}
	if !hasHostCLI && hasUniversalCLI {
		return "darwin-universal"
	}
	return hostTarget
}

func installModuleBinary(
	ctx context.Context,
	component resolution.Component,
	destination string,
	modulePath string,
	moduleH1 string,
) error {
	if component.Locator == "" {
		return fmt.Errorf("module component has no package locator")
	}
	if modulePath == "" || moduleH1 == "" {
		return fmt.Errorf("module component has no pinned modulePath/moduleH1")
	}
	if fileDigest(destination) == component.SHA256 {
		return nil
	}
	if err := verifyModule(ctx, modulePath, component.Version, moduleH1, ""); err != nil {
		return err
	}
	binDir, err := os.MkdirTemp("", "relkit-module-bin-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(binDir)
	command := exec.CommandContext(ctx, "go", "install", component.Locator+"@"+component.Version)
	command.Env = append(os.Environ(), "GOBIN="+binDir)
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("go install %s@%s failed: %w: %s", component.Locator, component.Version, err, strings.TrimSpace(string(output)))
	}
	name := filepath.Base(component.Locator)
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	built := filepath.Join(binDir, name)
	if got := fileDigest(built); got != component.SHA256 {
		return fmt.Errorf("module-built binary digest is %s, expected %s", got, component.SHA256)
	}
	return installBinary(built, destination, component.SHA256)
}

func installModuleReference(component resolution.Component, destination, moduleH1 string) error {
	if component.Locator == "" {
		return fmt.Errorf("module component has no module locator")
	}
	value := map[string]any{
		"schema":  "relkit.module-reference/1",
		"module":  component.Locator,
		"version": component.Version,
		"h1":      moduleH1,
	}
	if moduleH1 == "" {
		return fmt.Errorf("module component %s has no distribution moduleH1", component.Name)
	}
	return document.Write(filepath.Join(destination, "module.json"), value)
}

func installGoOverlay(
	ctx context.Context,
	projectRoot string,
	component resolution.Component,
	modulePath string,
	moduleH1 string,
) (string, error) {
	if modulePath == "" || moduleH1 == "" {
		return "", fmt.Errorf("Go facade has no pinned modulePath/moduleH1")
	}
	if component.Locator != modulePath {
		return "", fmt.Errorf("Go facade module %q does not match distribution module %q", component.Locator, modulePath)
	}
	projectMod := filepath.Join(projectRoot, "go.mod")
	data, err := os.ReadFile(projectMod)
	if err != nil {
		return "", fmt.Errorf("Go facade requires host go.mod: %w", err)
	}
	overlay := filepath.Join(projectRoot, ".relkit", "components", "sdk", "go", "relkit.mod")
	if err := atomicWrite(overlay, data, 0o644); err != nil {
		return "", err
	}
	if err := installGoOverlaySum(overlay, filepath.Join(projectRoot, "go.sum")); err != nil {
		return "", err
	}
	command := exec.CommandContext(ctx, "go", "mod", "edit", "-modfile="+overlay, "-require="+modulePath+"@"+component.Version)
	if output, err := command.CombinedOutput(); err != nil {
		return "", fmt.Errorf("generate temporary modfile: %w: %s", err, strings.TrimSpace(string(output)))
	}
	if err := verifyModule(ctx, modulePath, component.Version, moduleH1, overlay); err != nil {
		return "", err
	}
	return overlay, nil
}

func installGoArchiveOverlay(ctx context.Context, projectRoot, sdkRoot string) (string, error) {
	projectMod := filepath.Join(projectRoot, "go.mod")
	data, err := os.ReadFile(projectMod)
	if err != nil {
		return "", fmt.Errorf("Go facade requires host go.mod: %w", err)
	}
	sdkMod := filepath.Join(sdkRoot, "go.mod")
	moduleData, err := os.ReadFile(sdkMod)
	if err != nil {
		return "", fmt.Errorf("Go facade archive has no go.mod: %w", err)
	}
	modulePath := strings.TrimSpace(strings.TrimPrefix(strings.SplitN(string(moduleData), "\n", 2)[0], "module "))
	if modulePath != "github.com/shichao402/relkit" {
		return "", fmt.Errorf("Go facade archive declares unexpected module %q", modulePath)
	}
	overlay := filepath.Join(sdkRoot, "relkit.mod")
	if err := atomicWrite(overlay, data, 0o644); err != nil {
		return "", err
	}
	if err := installGoOverlaySum(
		overlay,
		filepath.Join(projectRoot, "go.sum"),
		filepath.Join(sdkRoot, "go.sum"),
	); err != nil {
		return "", err
	}
	relativeSDK, err := filepath.Rel(projectRoot, sdkRoot)
	if err != nil {
		return "", err
	}
	command := exec.CommandContext(
		ctx,
		"go",
		"mod",
		"edit",
		"-modfile="+overlay,
		"-require="+modulePath+"@v0.0.0",
		"-replace="+modulePath+"=./"+filepath.ToSlash(relativeSDK),
	)
	if output, err := command.CombinedOutput(); err != nil {
		return "", fmt.Errorf("generate Go facade overlay: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return overlay, nil
}

// installGoOverlaySum gives an alternate -modfile its matching checksum set.
// Go ignores a replaced module's go.sum, so a local SDK facade needs the union
// of the host and SDK sums beside relkit.mod as relkit.sum.
func installGoOverlaySum(overlay string, inputs ...string) error {
	lines := map[string]struct{}{}
	for _, input := range inputs {
		data, err := os.ReadFile(input)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("read Go checksum input %s: %w", input, err)
		}
		for _, line := range strings.Split(string(data), "\n") {
			if line = strings.TrimSpace(line); line != "" {
				lines[line] = struct{}{}
			}
		}
	}
	ordered := make([]string, 0, len(lines))
	for line := range lines {
		ordered = append(ordered, line)
	}
	sort.Strings(ordered)
	content := []byte(strings.Join(ordered, "\n"))
	if len(content) > 0 {
		content = append(content, '\n')
	}
	sumFile := strings.TrimSuffix(overlay, filepath.Ext(overlay)) + ".sum"
	if err := atomicWrite(sumFile, content, 0o644); err != nil {
		return fmt.Errorf("write Go overlay checksum file: %w", err)
	}
	return nil
}

func verifyModule(ctx context.Context, modulePath, version, expectedH1, modFile string) error {
	if modulePath == "" || version == "" || expectedH1 == "" {
		return fmt.Errorf("module identity is incomplete")
	}
	args := []string{"mod", "download"}
	if modFile != "" {
		args = append(args, "-modfile="+modFile)
	}
	args = append(args, "-json", modulePath+"@"+version)
	command := exec.CommandContext(ctx, "go", args...)
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("verify module %s@%s: %w: %s", modulePath, version, err, strings.TrimSpace(string(output)))
	}
	var result struct {
		Path    string `json:"Path"`
		Version string `json:"Version"`
		Sum     string `json:"Sum"`
		Error   *struct {
			Err string `json:"Err"`
		} `json:"Error"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		return fmt.Errorf("verify module %s@%s returned invalid JSON: %w", modulePath, version, err)
	}
	if result.Error != nil {
		return fmt.Errorf("verify module %s@%s: %s", modulePath, version, result.Error.Err)
	}
	if result.Path != modulePath || result.Version != version {
		return fmt.Errorf("module resolver returned %s@%s, expected %s@%s", result.Path, result.Version, modulePath, version)
	}
	if result.Sum != expectedH1 {
		return fmt.Errorf("module %s@%s h1 is %q, expected %q", modulePath, version, result.Sum, expectedH1)
	}
	return nil
}

func HostTarget() (string, error) {
	switch runtime.GOOS + "/" + runtime.GOARCH {
	case "windows/amd64":
		return "windows-amd64", nil
	case "linux/amd64":
		return "linux-amd64", nil
	case "linux/arm64":
		return "linux-arm64", nil
	case "darwin/amd64":
		return "darwin-amd64", nil
	case "darwin/arm64":
		return "darwin-arm64", nil
	default:
		return "", fmt.Errorf("unsupported host target %s/%s", runtime.GOOS, runtime.GOARCH)
	}
}

func fetch(ctx context.Context, client *http.Client, root string, component resolution.Component) (string, error) {
	locators := append([]string(nil), component.Mirrors...)
	if component.Locator != "" {
		locators = append(locators, component.Locator)
	}
	return fetchLocators(ctx, client, root, component, locators)
}

func fetchLocators(ctx context.Context, client *http.Client, root string, component resolution.Component, locators []string) (string, error) {
	if len(locators) == 0 {
		return "", fmt.Errorf("component %s has no asset locator", component.Name)
	}
	cache := filepath.Join(root, ".relkit", "cache", "vnext")
	if err := os.MkdirAll(cache, 0o755); err != nil {
		return "", err
	}
	destination := cachePath(root, component.SHA256)
	if fileDigest(destination) == component.SHA256 {
		return destination, nil
	}
	var failures []string
	for _, locator := range locators {
		data, err := readLocator(ctx, client, root, locator)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", locator, err))
			continue
		}
		sum := sha256.Sum256(data)
		got := canonical.DigestPrefix + fmt.Sprintf("%x", sum[:])
		if got != component.SHA256 {
			failures = append(failures, fmt.Sprintf("%s: digest %s", locator, got))
			continue
		}
		if err := atomicWrite(destination, data, 0o644); err != nil {
			return "", err
		}
		return destination, nil
	}
	return "", fmt.Errorf("component %s could not be fetched from signed source order: %s", component.Name, strings.Join(failures, "; "))
}

func readLocator(ctx context.Context, client *http.Client, root, locator string) ([]byte, error) {
	parsed, err := url.Parse(locator)
	if err != nil {
		return nil, err
	}
	switch parsed.Scheme {
	case "http", "https":
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, locator, nil)
		if err != nil {
			return nil, err
		}
		response, err := client.Do(request)
		if err != nil {
			return nil, err
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("GET returned %d", response.StatusCode)
		}
		return io.ReadAll(response.Body)
	case "file":
		return os.ReadFile(parsed.Path)
	case "":
		path := locator
		if !filepath.IsAbs(path) {
			path = filepath.Join(root, path)
		}
		return os.ReadFile(path)
	default:
		return nil, fmt.Errorf("unsupported locator scheme %q", parsed.Scheme)
	}
}

func cachePath(root, digest string) string {
	return filepath.Join(root, ".relkit", "cache", "vnext", strings.TrimPrefix(digest, canonical.DigestPrefix))
}

func installBinary(source, destination, expected string) error {
	if fileDigest(destination) == expected {
		return nil
	}
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	if err := atomicWrite(destination, data, 0o755); err != nil {
		return err
	}
	if got := fileDigest(destination); got != expected {
		return fmt.Errorf("installed binary digest is %s, expected %s", got, expected)
	}
	return nil
}

func installArchive(source, destination, expected string) error {
	marker := filepath.Join(destination, ".relkit-artifact-digest")
	if data, err := os.ReadFile(marker); err == nil && strings.TrimSpace(string(data)) == expected {
		return nil
	}
	parent := filepath.Dir(destination)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	staging, err := os.MkdirTemp(parent, "."+filepath.Base(destination)+"-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)
	reader, err := zip.OpenReader(source)
	if err != nil {
		return err
	}
	defer reader.Close()
	for _, entry := range reader.File {
		clean := filepath.Clean(filepath.FromSlash(entry.Name))
		if clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return fmt.Errorf("archive contains unsafe path %q", entry.Name)
		}
		if entry.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("archive contains unsupported symlink %q", entry.Name)
		}
		target := filepath.Join(staging, clean)
		if entry.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		sourceFile, err := entry.Open()
		if err != nil {
			return err
		}
		targetFile, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, entry.Mode().Perm())
		if err != nil {
			sourceFile.Close()
			return err
		}
		_, copyErr := io.Copy(targetFile, sourceFile)
		closeErr := targetFile.Close()
		sourceFile.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	if err := os.WriteFile(filepath.Join(staging, ".relkit-artifact-digest"), []byte(expected+"\n"), 0o644); err != nil {
		return err
	}
	backup := destination + ".old"
	_ = os.RemoveAll(backup)
	if _, err := os.Stat(destination); err == nil {
		if err := os.Rename(destination, backup); err != nil {
			return err
		}
	}
	if err := os.Rename(staging, destination); err != nil {
		_ = os.Rename(backup, destination)
		return err
	}
	_ = os.RemoveAll(backup)
	return nil
}

func fileDigest(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return canonical.DigestPrefix + fmt.Sprintf("%x", sum[:])
}

func atomicWrite(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(mode); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return fsx.Replace(temporaryPath, path)
}
