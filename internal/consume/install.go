package consume

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/shichao402/relkit/internal/registry"
)

// DownloadAttempts mirrors the Python consumer's retry count.
const DownloadAttempts = 3

// Downloader fetches bytes for a URL. It is an interface so tests can inject
// fixtures; production uses the stdlib HTTP client with the same trust model
// as the Python consumer (strict TLS, no insecure fallback unless opted in).
type Downloader interface {
	Fetch(url string) ([]byte, error)
}

// HTTPDownloader is the default strict-TLS downloader.
type HTTPDownloader struct {
	Timeout time.Duration
}

// Fetch retrieves the URL body with redirects enabled.
func (d *HTTPDownloader) Fetch(url string) ([]byte, error) {
	timeout := d.Timeout
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	client := &http.Client{Timeout: timeout}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s returned %d", url, resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// CacheDir returns the artifact cache directory under root.
func CacheDir(root string) string {
	return filepath.Join(root, ".relkit", "cache", "artifacts")
}

// DownloadArtifact fetches the spec's bytes into the content-addressed cache
// and returns the cached path. Cache hits skip the network entirely.
func DownloadArtifact(root, component string, spec *ArtifactSpec, dl Downloader) (string, error) {
	cache := CacheDir(root)
	if err := os.MkdirAll(cache, 0o755); err != nil {
		return "", err
	}
	destination := filepath.Join(cache, spec.SHA256)
	if data, err := os.ReadFile(destination); err == nil {
		if fmt.Sprintf("%x", sha256.Sum256(data)) == spec.SHA256 {
			return destination, nil
		}
	}
	_ = os.Remove(destination)

	var errs []string
	for attempt := 1; attempt <= DownloadAttempts; attempt++ {
		for _, url := range spec.URLs {
			body, err := dl.Fetch(url)
			if err != nil {
				errs = append(errs, fmt.Sprintf("attempt %d %s: %v", attempt, url, err))
				continue
			}
			if got := fmt.Sprintf("%x", sha256.Sum256(body)); got != spec.SHA256 {
				errs = append(errs, fmt.Sprintf("attempt %d %s: sha256 mismatch", attempt, url))
				continue
			}
			tmp := destination + ".tmp"
			if err := os.WriteFile(tmp, body, 0o644); err != nil {
				return "", err
			}
			if err := os.Rename(tmp, destination); err != nil {
				return "", err
			}
			return destination, nil
		}
		if attempt < DownloadAttempts {
			time.Sleep(time.Duration(1<<attempt) * time.Second)
		}
	}
	return "", fmt.Errorf("cannot download %s: %s", component, strings.Join(errs, "; "))
}

// HostTarget resolves the current host target string.
func HostTarget() (string, error) {
	var osName, arch string
	switch runtime.GOOS {
	case "windows":
		osName = "windows"
	case "linux":
		osName = "linux"
	case "darwin":
		osName = "darwin"
	default:
		return "", fmt.Errorf("unsupported host OS: %s", runtime.GOOS)
	}
	switch runtime.GOARCH {
	case "amd64":
		arch = "amd64"
	case "arm64":
		arch = "arm64"
	default:
		return "", fmt.Errorf("unsupported host arch: %s", runtime.GOARCH)
	}
	target := osName + "-" + arch
	for _, t := range registry.Targets {
		if t == target {
			return target, nil
		}
	}
	return "", fmt.Errorf("unsupported host target: %s", target)
}

// InstallBinary places a product-binary at its destination with an atomic
// rename, then verifies the installed hash.
func InstallBinary(root, component, target, artifactPath, digest string) (string, error) {
	row, ok := registry.ByName[component]
	if !ok || row.Role != registry.RoleProductBinary {
		return "", fmt.Errorf("%s is not a binary component", component)
	}
	name, err := row.InstallName(target)
	if err != nil {
		return "", err
	}
	destination := filepath.Join(root, row.Destination, name)
	if existing, err := FileSHA256(destination); err == nil && existing == digest {
		return destination, nil
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return "", err
	}
	data, err := os.ReadFile(artifactPath)
	if err != nil {
		return "", err
	}
	tmp := destination + ".tmp"
	if err := os.WriteFile(tmp, data, 0o755); err != nil {
		return "", err
	}
	if err := replaceLockedBinary(tmp, destination); err != nil {
		return "", err
	}
	got, err := FileSHA256(destination)
	if err != nil {
		return "", err
	}
	if got != digest {
		return "", fmt.Errorf("installed binary hash drifted: %s", destination)
	}
	return destination, nil
}

// replaceLockedBinary moves tmp onto destination, tolerating a Windows
// destination that is still mapped by a running process or a scanner: the
// locked inode is sidestepped to "<name>.old-<ms>" first (renaming a locked
// exe is allowed; overwriting it is not), the new file lands under the
// original name, and stale sidesteps are swept best-effort. Mirrors the
// Python consumer's fs_utils.place_binary semantics (build #26 fix).
func replaceLockedBinary(tmp, destination string) error {
	firstErr := os.Rename(tmp, destination)
	if firstErr == nil {
		return nil
	}
	if runtime.GOOS != "windows" {
		return firstErr
	}
	sidestep := fmt.Sprintf("%s.old-%d", destination, time.Now().UnixMilli())
	if err := os.Rename(destination, sidestep); err != nil {
		return err
	}
	if err := os.Rename(tmp, destination); err != nil {
		// The sidestep never got superseded; put the old inode back so the
		// tree keeps its previous state instead of a half-landed swap.
		_ = os.Rename(sidestep, destination)
		return err
	}
	sweepSidestepped(destination)
	return nil
}

// sweepSidestepped removes leftover "<name>.old-<ms>" siblings best-effort;
// entries still held open by a lingering process are skipped, not fatal.
func sweepSidestepped(destination string) {
	prefix := filepath.Base(destination) + ".old-"
	entries, err := os.ReadDir(filepath.Dir(destination))
	if err != nil {
		return
	}
	for _, ent := range entries {
		if ent.IsDir() || !strings.HasPrefix(ent.Name(), prefix) {
			continue
		}
		_ = os.Remove(filepath.Join(filepath.Dir(destination), ent.Name()))
	}
}

// ExtractTree unpacks an SDK zip into destination with safe path checking,
// preserving sibling registry trees, and writes the provenance marker.
func ExtractTree(artifactPath, destination, digest, component string) error {
	row, ok := registry.ByName[component]
	if !ok || row.Role != registry.RoleProductTree {
		return fmt.Errorf("%s is not a tree component", component)
	}
	marker := filepath.Join(destination, ".relkit-artifact.json")
	if state, err := os.ReadFile(marker); err == nil {
		var m struct {
			SHA256 string `json:"sha256"`
		}
		if jsonErr := unmarshalJSON(state, &m); jsonErr == nil && m.SHA256 == digest {
			if treeComplete(destination, row) {
				return nil
			}
		}
	}

	parent := filepath.Dir(destination)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	staging, err := os.MkdirTemp(parent, "."+component+"-stage-")
	if err != nil {
		return err
	}
	backup := destination + ".relkit-old"
	defer os.RemoveAll(staging)

	if err := unzipInto(artifactPath, staging); err != nil {
		return err
	}
	if !treeComplete(staging, row) {
		return fmt.Errorf("%s artifact is incomplete", component)
	}
	if err := carryPreservedSubtrees(destination, staging, component); err != nil {
		return err
	}
	markerBytes := []byte(fmt.Sprintf(`{"schema": %q, "sha256": %q}`+"\n", SchemaV3, digest))
	if err := os.WriteFile(filepath.Join(staging, ".relkit-artifact.json"), markerBytes, 0o644); err != nil {
		return err
	}

	if err := os.RemoveAll(backup); err != nil && !os.IsNotExist(err) {
		return err
	}
	if _, err := os.Stat(destination); err == nil {
		if err := os.Rename(destination, backup); err != nil {
			return err
		}
	}
	if err := os.Rename(staging, destination); err != nil {
		// Roll back to the backup if the new tree never landed.
		if _, statErr := os.Stat(destination); os.IsNotExist(statErr) {
			if _, backupErr := os.Stat(backup); backupErr == nil {
				_ = os.Rename(backup, destination)
			}
		}
		return err
	}
	_ = os.RemoveAll(backup)
	return nil
}

// InstallHostScripts unpacks the host-scripts zip into scripts/host with
// tree-hash verification and a backup/rollback swap, mirroring the Python
// consumer's safe_extract_host_scripts.
func InstallHostScripts(artifactPath, destination, expectedTreeHash string) error {
	required := make(map[string]bool)
	for _, rel := range registry.ByName["host-scripts"].RequiredPaths {
		required[rel] = true
	}
	staging, err := os.MkdirTemp(filepath.Dir(destination), ".host-scripts-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)
	backup := destination + ".relkit-old"

	if err := unzipInto(artifactPath, staging); err != nil {
		return err
	}
	names, err := listFiles(staging)
	if err != nil {
		return err
	}
	for _, name := range names {
		parts := strings.Split(filepath.ToSlash(name), "/")
		unsafe := filepath.IsAbs(name)
		for _, part := range parts {
			if part == ".." || part == "__pycache__" {
				unsafe = true
			}
		}
		if unsafe {
			return fmt.Errorf("host scripts artifact has unsafe or incomplete tree")
		}
	}
	for _, rel := range registry.ByName["host-scripts"].RequiredPaths {
		found := false
		for _, name := range names {
			if name == rel {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("host scripts artifact has unsafe or incomplete tree")
		}
	}
	actual, err := TreeSHA256(staging)
	if err != nil || actual != strings.ToLower(expectedTreeHash) {
		return fmt.Errorf(
			"host scripts tree hash mismatch: expected %s, got %s",
			strings.ToLower(expectedTreeHash), actual,
		)
	}

	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	if err := os.RemoveAll(backup); err != nil && !os.IsNotExist(err) {
		return err
	}
	if _, err := os.Stat(destination); err == nil {
		if err := os.Rename(destination, backup); err != nil {
			return err
		}
	}
	if err := os.Rename(staging, destination); err != nil {
		// Roll back: remove the half-landed destination, restore the backup.
		_ = os.RemoveAll(destination)
		if _, backupErr := os.Stat(backup); backupErr == nil {
			_ = os.Rename(backup, destination)
		}
		return err
	}
	verified, err := TreeSHA256(destination)
	if err != nil || verified != strings.ToLower(expectedTreeHash) {
		// The new tree drifted after landing; restore the backup.
		_ = os.RemoveAll(destination)
		if _, backupErr := os.Stat(backup); backupErr == nil {
			_ = os.Rename(backup, destination)
		}
		return fmt.Errorf("installed host scripts hash drifted")
	}
	_ = os.RemoveAll(backup)
	return nil
}

// listFiles walks dir and returns every regular file path relative to it,
// slash-separated.
func listFiles(dir string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !d.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	return out, err
}

// ClearOrphanRelkitProto removes a leftover third_party/relkit/proto tree that
// shadows the packaged Rust IDL, mirroring clear_orphan_relkit_proto.
func ClearOrphanRelkitProto(root string) {
	orphan := filepath.Join(root, "third_party", "relkit", "proto")
	info, err := os.Stat(orphan)
	if err != nil || !info.IsDir() {
		return
	}
	relative := "third_party/relkit/proto"
	for _, row := range registry.ProductComponents() {
		if row.Role == registry.RoleProductTree && row.Name != "host-scripts" &&
			strings.TrimSuffix(row.Destination, "/") == relative {
			return
		}
	}
	if err := os.RemoveAll(orphan); err == nil {
		fmt.Printf("relkit consume: removed orphan %s (shadows packaged sdk-rust IDL)\n", relative)
	}
}

// unzipInto extracts a zip with path-safety checks into dir.
func unzipInto(archive, dir string) error {
	reader, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer reader.Close()
	root, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	for _, member := range reader.File {
		if member.FileInfo().IsDir() {
			continue
		}
		clean := filepath.Clean(member.Name)
		if filepath.IsAbs(clean) || strings.HasPrefix(clean, "..") {
			return fmt.Errorf("unsafe archive path: %s", member.Name)
		}
		target := filepath.Join(root, filepath.FromSlash(clean))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		src, err := member.Open()
		if err != nil {
			return err
		}
		dst, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, member.Mode())
		if err != nil {
			src.Close()
			return err
		}
		if _, err := io.Copy(dst, src); err != nil {
			src.Close()
			dst.Close()
			return err
		}
		src.Close()
		dst.Close()
	}
	return nil
}

// treeComplete checks required paths exist under dir.
func treeComplete(dir string, row registry.Component) bool {
	for _, rel := range row.RequiredPaths {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(rel))); err != nil {
			return false
		}
	}
	return true
}

// carryPreservedSubtrees copies sibling registry trees nested under the tree
// being replaced into staging before the swap.
func carryPreservedSubtrees(destination, staging, component string) error {
	owner := strings.TrimSuffix(registry.ByName[component].Destination, "/")
	prefix := owner + "/"
	var nested []string
	for _, row := range registry.ProductComponents() {
		if row.Role != registry.RoleProductTree || row.Name == component || row.Name == "host-scripts" {
			continue
		}
		if strings.HasPrefix(row.Destination, prefix) {
			nested = append(nested, strings.TrimPrefix(row.Destination, prefix))
		}
	}
	sort.Strings(nested)
	for _, rel := range nested {
		src := filepath.Join(destination, filepath.FromSlash(rel))
		if info, err := os.Stat(src); err != nil || !info.IsDir() {
			continue
		}
		dst := filepath.Join(staging, filepath.FromSlash(rel))
		if err := copyTree(src, dst); err != nil {
			return err
		}
	}
	return nil
}

// copyTree recursively copies src to dst.
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}

// unmarshalJSON is a tiny indirection so the marker parse above stays terse.
func unmarshalJSON(data []byte, v any) error {
	return json.Unmarshal(data, v)
}
