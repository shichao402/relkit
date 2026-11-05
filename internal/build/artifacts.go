package build

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
)

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

var updaterProtocolPattern = regexp.MustCompile(`(?m)^\s*Protocol\s+uint32\s*=\s*(\d+)`)

// UpdaterProtocol reads the single updater protocol number (ADR 0026) from
// internal/updater/const.go under root. Reading the file rather than importing
// the constant keeps --project-root builds honest about the checkout they pack.
func UpdaterProtocol(root string) (int, error) {
	raw, err := os.ReadFile(filepath.Join(root, "internal", "updater", "const.go"))
	if err != nil {
		return 0, err
	}
	found := updaterProtocolPattern.FindSubmatch(raw)
	if found == nil {
		return 0, fmt.Errorf("cannot read Protocol from internal/updater/const.go")
	}
	value, err := strconv.Atoi(string(found[1]))
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("internal/updater/const.go declares an invalid Protocol %q", found[1])
	}
	return value, nil
}
