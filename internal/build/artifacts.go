package build

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/shichao402/relkit/internal/registry"
)

// WriteDeterministicZip writes entries to destination as a deterministic
// zip: fixed 1980-01-01 timestamps, sorted by archive name, deflate level 9,
// 0644 file mode.
func WriteDeterministicZip(destination string, entries []Entry) error {
	sorted := make([]Entry, len(entries))
	copy(sorted, entries)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ArchiveName < sorted[j].ArchiveName })
	out, err := os.Create(destination)
	if err != nil {
		return err
	}
	defer out.Close()
	writer := zip.NewWriter(out)
	defer writer.Close()
	for _, entry := range sorted {
		data, err := os.ReadFile(entry.Source)
		if err != nil {
			return fmt.Errorf("read %s: %w", entry.Source, err)
		}
		header := zip.FileHeader{
			Name:          entry.ArchiveName,
			Method:        zip.Deflate,
			Modified:      time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC),
			ExternalAttrs: 0o100644 << 16,
		}
		file, err := writer.CreateHeader(&header)
		if err != nil {
			return err
		}
		if _, err := file.Write(data); err != nil {
			return err
		}
	}
	return writer.Close()
}

// TreeSHA256 computes the host-scripts tree hash the lock pins: sorted by
// archive name, normalized line endings, name\0content\0.
func TreeSHA256(entries []Entry) (string, error) {
	sorted := make([]Entry, len(entries))
	copy(sorted, entries)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ArchiveName < sorted[j].ArchiveName })
	digest := sha256.New()
	for _, entry := range sorted {
		data, err := os.ReadFile(entry.Source)
		if err != nil {
			return "", fmt.Errorf("read %s: %w", entry.Source, err)
		}
		digest.Write([]byte(entry.ArchiveName))
		digest.Write([]byte{0})
		digest.Write(normalizeNewlines(data))
		digest.Write([]byte{0})
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

func normalizeNewlines(data []byte) []byte {
	normalized := bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	return bytes.ReplaceAll(normalized, []byte("\r"), []byte("\n"))
}

func relWithin(sourceRoot, full string) string {
	rel, err := filepath.Rel(sourceRoot, full)
	if err != nil {
		return filepath.ToSlash(full)
	}
	return filepath.ToSlash(rel)
}

func filterExcludedPaths(entries []Entry, prefixes []string) []Entry {
	var kept []Entry
	for _, entry := range entries {
		if !pathExcluded(entry.ArchiveName, prefixes) {
			kept = append(kept, entry)
		}
	}
	return kept
}

func pathExcluded(archiveName string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasSuffix(prefix, "/") {
			if archiveName == prefix[:len(prefix)-1] || strings.HasPrefix(archiveName, prefix) {
				return true
			}
		} else if archiveName == prefix || strings.HasPrefix(archiveName, prefix+"/") {
			return true
		}
	}
	return false
}

// FileSHA256 hashes a file on disk.
func FileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	digest := sha256.New()
	if _, err := io.Copy(digest, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

// HostReleaseContract loads scripts/host/release-contract.json and asserts
// the schema tag.
func HostReleaseContract(root string) (map[string]any, error) {
	raw, err := os.ReadFile(filepath.Join(root, "scripts", "host", "release-contract.json"))
	if err != nil {
		return nil, fmt.Errorf("cannot read release-contract.json: %w", err)
	}
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("release-contract.json is not JSON: %w", err)
	}
	if value["schema"] != "relkit.host-contract/1" {
		return nil, fmt.Errorf("release-contract.json must use relkit.host-contract/1")
	}
	return value, nil
}

// UpdateIPCWindow reads IPCMin/IPCMax from internal/updater/const.go.
func UpdateIPCWindow(root string) (int, int, error) {
	raw, err := os.ReadFile(filepath.Join(root, "internal", "updater", "const.go"))
	if err != nil {
		return 0, 0, err
	}
	text := string(raw)
	foundMin := regexp.MustCompile(`IPCMin\s+uint32\s*=\s*(\d+)`).FindStringSubmatch(text)
	foundMax := regexp.MustCompile(`IPCMax\s+uint32\s*=\s*(\d+)`).FindStringSubmatch(text)
	if foundMin == nil || foundMax == nil {
		return 0, 0, fmt.Errorf("cannot read IPCMin/IPCMax from internal/updater/const.go")
	}
	min, err1 := strconv.Atoi(foundMin[1])
	max, err2 := strconv.Atoi(foundMax[1])
	if err1 != nil || err2 != nil {
		return 0, 0, fmt.Errorf("cannot read IPCMin/IPCMax from internal/updater/const.go")
	}
	return min, max, nil
}

// fileSHA256 is FileSHA256's internal alias for parity with the Python
// helper name; kept so manifest code reads the same either side.
func fileSHA256(path string) (string, error) {
	return FileSHA256(path)
}

// inprocessInHostZip mirrors INPROCESS_IN_HOST_ZIP: the in-process engine
// marker host zips must never pack.
var inprocessInHostZip = regexp.MustCompile(`\bRupUpdater\b|\bpackage inprocess\b`)

// hostSourceSuffixSet mirrors HOST_SOURCE_SUFFIXES.
var hostSourceSuffixSet = map[string]bool{
	".go": true, ".dart": true, ".ts": true, ".tsx": true, ".js": true, ".rs": true,
}

// packedHostSurfaceErrors mirrors packed_host_surface_errors in
// hostlib/facets.py: host zips may ship protocol helpers plus a facade,
// never an in-process engine.
func packedHostSurfaceErrors(row *registry.Component, names []string, texts map[string]string) []string {
	if row.UpdaterProcess == "" {
		return nil
	}
	nameSet := map[string]bool{}
	for _, name := range names {
		nameSet[name] = true
	}
	var problems []string
	for _, facade := range row.FacadePaths {
		if !nameSet[facade] {
			problems = append(problems, fmt.Sprintf("%s facade %s is not packed", row.Name, facade))
		}
	}
	for name, text := range texts {
		ext := strings.ToLower(filepath.Ext(name))
		if hostSourceSuffixSet[ext] && inprocessInHostZip.MatchString(text) {
			problems = append(problems, fmt.Sprintf("%s packs in-process updater in %s", row.Name, name))
		}
	}
	return problems
}

func indexOf(values []string, want string) (int, bool) {
	for i, value := range values {
		if value == want {
			return i, true
		}
	}
	return -1, false
}
