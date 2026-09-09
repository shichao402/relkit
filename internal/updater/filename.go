package updater

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"unicode"
)

var windowsReserved = map[string]struct{}{
	"CON": {}, "PRN": {}, "AUX": {}, "NUL": {},
	"COM1": {}, "COM2": {}, "COM3": {}, "COM4": {}, "COM5": {}, "COM6": {}, "COM7": {}, "COM8": {}, "COM9": {},
	"LPT1": {}, "LPT2": {}, "LPT3": {}, "LPT4": {}, "LPT5": {}, "LPT6": {}, "LPT7": {}, "LPT8": {}, "LPT9": {},
}

// ValidateArtifactFilename rejects path escape, absolute paths, and reserved names.
func ValidateArtifactFilename(name string) error {
	if name == "" {
		return fmt.Errorf("empty artifact filename")
	}
	if filepath.IsAbs(name) {
		return fmt.Errorf("absolute artifact filename")
	}
	if strings.ContainsRune(name, 0) {
		return fmt.Errorf("NUL in artifact filename")
	}
	normalized := strings.ReplaceAll(name, "\\", "/")
	if strings.HasPrefix(normalized, "/") || (len(normalized) >= 2 && normalized[1] == ':') {
		return fmt.Errorf("absolute artifact filename")
	}
	parts := strings.Split(normalized, "/")
	for _, p := range parts {
		if p == "" || p == "." || p == ".." {
			return fmt.Errorf("illegal path segment %q", p)
		}
		base := p
		if i := strings.IndexByte(base, '.'); i >= 0 {
			base = base[:i]
		}
		upper := strings.Map(unicode.ToUpper, base)
		if _, ok := windowsReserved[upper]; ok {
			return fmt.Errorf("reserved artifact filename %q", p)
		}
	}
	if runtime.GOOS == "windows" && strings.ContainsAny(name, `<>:"|?*`) {
		return fmt.Errorf("illegal windows filename characters")
	}
	clean := filepath.Clean(filepath.FromSlash(normalized))
	if strings.HasPrefix(clean, "..") {
		return fmt.Errorf("artifact filename escapes directory")
	}
	return nil
}

func joinUnder(root, rel string) (string, error) {
	if err := ValidateArtifactFilename(rel); err != nil {
		return "", err
	}
	full := filepath.Join(root, filepath.FromSlash(strings.ReplaceAll(rel, "\\", "/")))
	relOut, err := filepath.Rel(root, full)
	if err != nil || strings.HasPrefix(relOut, "..") {
		return "", fmt.Errorf("path escapes root")
	}
	return full, nil
}
