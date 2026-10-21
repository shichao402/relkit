package build

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
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

// FileSHA256 hashes a file on disk.
