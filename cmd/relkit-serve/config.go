package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"cnb.cool/shichao402/relkit/internal/model"
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

	// Operator credential: exactly one of uploadToken / uploadTokenFile, or
	// neither. Product-scoped credentials live in uploadTokens and may coexist
	// with the operator token. No credentials at all means a read-only server.
	//
	// UploadToken is omitempty so that the skeleton does not show an empty
	// slot inviting someone to paste a secret into a mode-0644 file.
	UploadToken     string             `json:"uploadToken,omitempty"`
	UploadTokenFile string             `json:"uploadTokenFile"`
	UploadTokens    []UploadTokenEntry `json:"uploadTokens,omitempty"`

	MaxUpload string `json:"maxUpload"`

	Cache   *CacheConfig   `json:"cache"`
	GC      *GCConfig      `json:"gc"`
	Site    *SiteConfig    `json:"site,omitempty"`
	Publish *PublishConfig `json:"publish,omitempty"`

	// StatsFile is the download-counter JSON. Empty means
	// <dir>/.relkit-serve-stats.json, which is writable under the default
	// systemd unit. Point it outside the tree if you add another ReadWritePaths.
	StatsFile string `json:"statsFile,omitempty"`

	ShutdownTimeout string `json:"shutdownTimeout"`
	LogRequests     *bool  `json:"logRequests"`
}

// UploadTokenEntry is a product-scoped publisher credential. File is the
// plaintext token path (0600); Products is the allow-list of product ids.
type UploadTokenEntry struct {
	File     string   `json:"file"`
	Products []string `json:"products"`
}

// credential is an in-memory upload secret. products == nil is the operator
// token (any key). A non-nil list is scoped to those product ids.
type credential struct {
	hash     []byte
	products []string
}

// SiteConfig is the operator's copy for the human-facing pages. None of it
// reaches protocol clients.
//
// It lives here rather than in the release tree because a product blurb is not
// part of a release: it changes on its own schedule, and whoever holds the
// upload token should not be able to rewrite what the site says about other
// products.
type SiteConfig struct {
	Title string `json:"title"`
	// Keyed by the directory name under index/, which is the product id used
	// everywhere else in RUP.
	Products map[string]*ProductConfig `json:"products"`
}

type ProductConfig struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Homepage    string `json:"homepage"`
}

// PublishConfig gates authenticated writers without affecting update clients.
// Zero keeps compatibility with generic PUT clients; a positive value requires
// every PUT to advertise at least that publish protocol.
type PublishConfig struct {
	MinProtocol int `json:"minProtocol"`
}

func (s *SiteConfig) product(id string) *ProductConfig {
	if s == nil || s.Products == nil {
		return nil
	}
	return s.Products[id]
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
	if cfg.Publish != nil && cfg.Publish.MinProtocol < 0 {
		return nil, path, fmt.Errorf("%s: publish.minProtocol must not be negative", path)
	}
	for i, entry := range cfg.UploadTokens {
		if err := entry.validate(); err != nil {
			return nil, path, fmt.Errorf("%s: uploadTokens[%d]: %w", path, i, err)
		}
	}
	return &cfg, path, nil
}

func (e UploadTokenEntry) validate() error {
	if strings.TrimSpace(e.File) == "" {
		return fmt.Errorf("file is required")
	}
	if len(e.Products) == 0 {
		return fmt.Errorf("products must be a non-empty list")
	}
	seen := map[string]bool{}
	for _, product := range e.Products {
		if err := model.CheckIdentifier(product, "product"); err != nil {
			return err
		}
		if seen[product] {
			return fmt.Errorf("duplicate product %q", product)
		}
		seen[product] = true
	}
	return nil
}

// TokenFromFileConfig resolves the upload token declared by the config file.
//
// A token written directly into the config makes the file as sensitive as a
// private key, so its permissions are checked. Anyone who can read it can
// publish, and on this server publishing means replacing the binaries that
// clients will install and run.
func (c *FileConfig) TokenFromFileConfig(configPath string) ([]byte, []string, error) {
	creds, warnings, err := c.CredentialsFromFileConfig(configPath)
	if err != nil {
		return nil, nil, err
	}
	for _, cred := range creds {
		if cred.products == nil {
			return cred.hash, warnings, nil
		}
	}
	return nil, warnings, nil
}

// CredentialsFromFileConfig loads the operator token (if any) and every
// product-scoped token. Duplicate hashes are rejected so a leaked product
// token cannot silently inherit operator rights, or vice versa.
func (c *FileConfig) CredentialsFromFileConfig(configPath string) ([]credential, []string, error) {
	if c == nil {
		return nil, nil, nil
	}
	if c.UploadToken != "" && c.UploadTokenFile != "" {
		return nil, nil, fmt.Errorf(
			"set either uploadToken or uploadTokenFile, not both")
	}

	var (
		creds    []credential
		warnings []string
		hashes   = map[string]bool{}
	)
	add := func(hash []byte, products []string, source string) error {
		key := hex.EncodeToString(hash)
		if hashes[key] {
			return fmt.Errorf("duplicate upload token hash from %s", source)
		}
		hashes[key] = true
		creds = append(creds, credential{hash: hash, products: products})
		return nil
	}

	if c.UploadTokenFile != "" {
		path := resolveRelative(c.UploadTokenFile, configPath)
		hash, extra, err := hashedTokenFromFile(path)
		if err != nil {
			return nil, nil, fmt.Errorf("uploadTokenFile: %w", err)
		}
		warnings = append(warnings, extra...)
		if err := add(hash, nil, path); err != nil {
			return nil, nil, err
		}
	} else if c.UploadToken != "" {
		warnings = append(warnings, checkPermissions(configPath)...)
		if err := add(hashToken(c.UploadToken), nil, configPath); err != nil {
			return nil, nil, err
		}
	}

	for i, entry := range c.UploadTokens {
		path := resolveRelative(entry.File, configPath)
		hash, extra, err := hashedTokenFromFile(path)
		if err != nil {
			return nil, nil, fmt.Errorf("uploadTokens[%d]: %w", i, err)
		}
		warnings = append(warnings, extra...)
		products := append([]string(nil), entry.Products...)
		if err := add(hash, products, path); err != nil {
			return nil, nil, err
		}
	}
	return creds, warnings, nil
}

func resolveRelative(path, configPath string) string {
	if filepath.IsAbs(path) || configPath == "" {
		return path
	}
	return filepath.Join(filepath.Dir(configPath), path)
}

func hashedTokenFromFile(path string) ([]byte, []string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	token := strings.TrimSpace(string(raw))
	if token == "" {
		return nil, nil, fmt.Errorf("%s is empty", path)
	}
	return hashToken(token), checkPermissions(path), nil
}

func withOperatorToken(creds []credential, hash []byte) ([]credential, error) {
	key := hex.EncodeToString(hash)
	out := []credential{{hash: hash}}
	for _, cred := range creds {
		if cred.products == nil {
			continue
		}
		if hex.EncodeToString(cred.hash) == key {
			return nil, fmt.Errorf("operator token matches a product-scoped token")
		}
		out = append(out, cred)
	}
	return out, nil
}

func (c credential) allowsKey(name string) bool {
	if c.products == nil {
		return true
	}
	for _, product := range c.products {
		if productAllowsKey(product, name) {
			return true
		}
	}
	return false
}

func productAllowsKey(product, name string) bool {
	for _, prefix := range []string{
		"index/" + product + "/",
		"manifest/" + product + "/",
		"artifact/" + product + "/",
		"latest/" + product + "/",
	} {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return name == "directory/"+product+".pb" ||
		name == "fallback/"+product+".pb" ||
		name == "site/"+product+".json"
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
			NoCache:       []string{"index/", "fallback/", "directory/", "site/", "latest/"},
			Immutable:     []string{"manifest/", "artifact/"},
			DefaultMaxAge: &maxAge,
		},
		GC: &GCConfig{
			Enabled:  &gcEnabled,
			Interval: "1h",
		},
		Publish:         &PublishConfig{MinProtocol: 0},
		ShutdownTimeout: "30s",
		LogRequests:     &logRequests,
	}
	out, _ := json.MarshalIndent(cfg, "", "  ")
	return append(out, '\n')
}

func writeFileConfig(path string, cfg *FileConfig) error {
	out, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(out, '\n'), 0o644)
}

func productTokenRelPath(product string) string {
	return "tokens/" + product + ".token"
}

func (c *FileConfig) upsertProductToken(product, relFile string) {
	for i, entry := range c.UploadTokens {
		if len(entry.Products) == 1 && entry.Products[0] == product {
			c.UploadTokens[i].File = relFile
			return
		}
	}
	c.UploadTokens = append(c.UploadTokens, UploadTokenEntry{
		File:     relFile,
		Products: []string{product},
	})
}

// shareProductToken adds product to the uploadTokens entry that already lists
// with. It does not create or rotate a token file.
func (c *FileConfig) shareProductToken(product, with string) (string, error) {
	if c == nil {
		return "", fmt.Errorf("config is empty")
	}
	if _, listed := c.productTokenFile(product); listed {
		return "", fmt.Errorf("%s is already listed in uploadTokens", product)
	}
	rel, ok := c.productTokenFile(with)
	if !ok {
		return "", fmt.Errorf("%s is not listed in uploadTokens", with)
	}
	for i, entry := range c.UploadTokens {
		if entry.File != rel {
			continue
		}
		c.UploadTokens[i].Products = append(append([]string(nil), entry.Products...), product)
		return rel, nil
	}
	return "", fmt.Errorf("%s is not listed in uploadTokens", with)
}

// removeProductToken drops product from uploadTokens and reports the token
// file that is now unreferenced, which is empty when the entry listed other
// products too: revoking one product must not lock out the others sharing that
// file.
func (c *FileConfig) removeProductToken(product string) (string, bool) {
	if c == nil {
		return "", false
	}
	var (
		kept   []UploadTokenEntry
		orphan string
		found  bool
	)
	for _, entry := range c.UploadTokens {
		var products []string
		hit := false
		for _, id := range entry.Products {
			if id == product {
				hit = true
				continue
			}
			products = append(products, id)
		}
		if !hit {
			kept = append(kept, entry)
			continue
		}
		found = true
		if len(products) == 0 {
			orphan = entry.File
			continue
		}
		entry.Products = products
		kept = append(kept, entry)
	}
	if !found {
		return "", false
	}
	c.UploadTokens = kept
	return orphan, true
}

func (c *FileConfig) productTokenFile(product string) (string, bool) {
	if c == nil {
		return "", false
	}
	found := ""
	n := 0
	for _, entry := range c.UploadTokens {
		for _, id := range entry.Products {
			if id == product {
				found = entry.File
				n++
			}
		}
	}
	if n != 1 || found == "" {
		return "", false
	}
	return found, true
}
