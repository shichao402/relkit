package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// ConfigName is looked for next to the binary and in /etc when -config is
// omitted. Which one was used is always logged at startup, so an unexpected
// config never goes unnoticed.
const ConfigName = "relkit-serve.json"

var searchPaths = []string{ConfigName, "/etc/" + ConfigName}

// FileConfig mirrors the config file. Every field is optional; anything left
// out keeps the built-in default, and anything given on the command line wins
// over the file.
//
// Durations and sizes are strings ("30s", "4GiB") because a config file is read
// by people, and 4294967296 tells the reader nothing.
type FileConfig struct {
	Addr string `json:"addr"`
	Dir  string `json:"dir"`

	// Exactly one of these, or neither for a read-only server.
	//
	// UploadToken is omitempty so that the skeleton does not show an empty
	// slot inviting someone to paste a secret into a mode-0644 file.
	UploadToken     string `json:"uploadToken,omitempty"`
	UploadTokenFile string `json:"uploadTokenFile"`

	MaxUpload string `json:"maxUpload"`

	Cache *CacheConfig `json:"cache"`
	GC    *GCConfig    `json:"gc"`

	ShutdownTimeout string `json:"shutdownTimeout"`
	LogRequests     *bool  `json:"logRequests"`
}

type CacheConfig struct {
	NoCache       []string `json:"noCache"`
	Immutable     []string `json:"immutable"`
	DefaultMaxAge *int     `json:"defaultMaxAge"`
}

// GCConfig controls orphan cleanup of unreferenced manifest/ and artifact/
// objects. The server never rewrites signed indexes; publish should push a
// single-node index when only the latest release should be retained.
type GCConfig struct {
	Enabled  *bool  `json:"enabled"`
	Interval string `json:"interval"`
}

// LoadFileConfig reads a config file, or returns nil if there is none.
//
// Unknown fields are an error rather than being ignored. A misspelled key that
// silently keeps a default is the worst kind of configuration bug: the server
// starts, reports success, and behaves differently than the file says. This is
// especially true of the cache prefixes, where the only symptom would be
// releases appearing late.
func LoadFileConfig(explicit string) (*FileConfig, string, error) {
	path := explicit
	if path == "" {
		for _, candidate := range searchPaths {
			if _, err := os.Stat(candidate); err == nil {
				path = candidate
				break
			}
		}
		if path == "" {
			return nil, "", nil
		}
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, path, err
	}

	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()

	var cfg FileConfig
	if err := decoder.Decode(&cfg); err != nil {
		return nil, path, fmt.Errorf("%s: %w", path, err)
	}
	return &cfg, path, nil
}

// TokenFromFileConfig resolves the upload token declared by the config file.
//
// A token written directly into the config makes the file as sensitive as a
// private key, so its permissions are checked. Anyone who can read it can
// publish, and on this server publishing means replacing the binaries that
// clients will install and run.
func (c *FileConfig) TokenFromFileConfig(configPath string) ([]byte, []string, error) {
	if c == nil {
		return nil, nil, nil
	}
	if c.UploadToken != "" && c.UploadTokenFile != "" {
		return nil, nil, fmt.Errorf(
			"set either uploadToken or uploadTokenFile, not both")
	}

	var warnings []string

	if c.UploadTokenFile != "" {
		path := c.UploadTokenFile
		if !filepath.IsAbs(path) && configPath != "" {
			path = filepath.Join(filepath.Dir(configPath), path)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, nil, fmt.Errorf("uploadTokenFile: %w", err)
		}
		token := strings.TrimSpace(string(raw))
		if token == "" {
			return nil, nil, fmt.Errorf("uploadTokenFile %s is empty", path)
		}
		warnings = append(warnings, checkPermissions(path)...)
		return hashToken(token), warnings, nil
	}

	if c.UploadToken != "" {
		warnings = append(warnings, checkPermissions(configPath)...)
		return hashToken(c.UploadToken), warnings, nil
	}
	return nil, nil, nil
}

// checkPermissions warns when a secret-bearing file is readable beyond its
// owner.
//
// Skipped on Windows, where access is governed by ACLs that os.Stat does not
// reflect: it reports 0666 for any writable file, so the check would fire on
// every deployment and teach the reader to ignore it. Warnings that are usually
// wrong are worse than no warning.
func checkPermissions(path string) []string {
	if path == "" || runtime.GOOS == "windows" {
		return nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil
	}
	mode := info.Mode().Perm()
	if mode&0o077 != 0 {
		return []string{fmt.Sprintf(
			"%s holds an upload token but is mode %04o; "+
				"run: chmod 600 %s", path, mode, path)}
	}
	return nil
}

// ParseSize accepts a plain byte count or a suffixed one ("512MiB", "4GiB").
func ParseSize(text string) (int64, error) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return 0, fmt.Errorf("empty size")
	}

	units := []struct {
		suffix string
		factor int64
	}{
		{"KiB", 1 << 10}, {"MiB", 1 << 20}, {"GiB", 1 << 30}, {"TiB", 1 << 40},
		{"KB", 1000}, {"MB", 1000 * 1000}, {"GB", 1000 * 1000 * 1000},
		{"K", 1 << 10}, {"M", 1 << 20}, {"G", 1 << 30}, {"B", 1},
	}
	upper := strings.ToUpper(trimmed)
	for _, unit := range units {
		if strings.HasSuffix(upper, strings.ToUpper(unit.suffix)) {
			number := strings.TrimSpace(upper[:len(upper)-len(unit.suffix)])
			value, err := parseFloat(number)
			if err != nil {
				return 0, fmt.Errorf("invalid size %q", text)
			}
			return int64(value * float64(unit.factor)), nil
		}
	}

	value, err := parseFloat(upper)
	if err != nil {
		return 0, fmt.Errorf("invalid size %q", text)
	}
	return int64(value), nil
}

// parseFloat requires the whole string to be a number.
//
// fmt.Sscanf would not: it happily reads "12" out of "12XiB" and reports
// success, so a typo in a unit would silently become a much smaller limit.
func parseFloat(text string) (float64, error) {
	value, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
	if err != nil {
		return 0, err
	}
	if value < 0 {
		return 0, fmt.Errorf("negative")
	}
	return value, nil
}

func ParseDuration(text string) (time.Duration, error) {
	return time.ParseDuration(strings.TrimSpace(text))
}

// Skeleton is what `relkit-serve init` writes.
//
// Ships with the RUP cache prefixes already filled in rather than empty,
// because those are the values that make a release visible immediately, and a
// reader who does not yet know why they matter would otherwise leave them out.
func Skeleton(dir string) []byte {
	maxAge := 60
	logRequests := true
	gcEnabled := true
	cfg := FileConfig{
		Addr:            ":8080",
		Dir:             dir,
		UploadTokenFile: "relkit-serve.token",
		MaxUpload:       "4GiB",
		Cache: &CacheConfig{
			NoCache:       []string{"index/", "fallback/"},
			Immutable:     []string{"manifest/", "artifact/"},
			DefaultMaxAge: &maxAge,
		},
		GC: &GCConfig{
			Enabled:  &gcEnabled,
			Interval: "1h",
		},
		ShutdownTimeout: "30s",
		LogRequests:     &logRequests,
	}
	out, _ := json.MarshalIndent(cfg, "", "  ")
	return append(out, '\n')
}
