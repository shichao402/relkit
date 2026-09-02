package config

import (
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	rupv2 "cnb.cool/shichao402/relkit/api/rup/v2"
	"cnb.cool/shichao402/relkit/internal/envelope"
	"cnb.cool/shichao402/relkit/internal/jsonio"
	"cnb.cool/shichao402/relkit/internal/keys"
	"cnb.cool/shichao402/relkit/internal/model"
)

const ConfigName = "relkit.json"

var envStrategyPattern = regexp.MustCompile(`^env:([A-Za-z_][A-Za-z0-9_]*)(?:\+(\d+))?$`)

type Error struct {
	Message string
}

func (e Error) Error() string {
	return e.Message
}

type Config struct {
	Path           string
	Root           string
	Raw            map[string]any
	Product        string
	DefaultChannel string
	Channels       []string
	CodeStrategy   string
	// RetainVersions caps how many version nodes remain in a published index.
	// 0 (default) keeps the full history. N >= 1 keeps only the N highest-code
	// nodes after merge, so relkit-serve orphan GC can drop older artifacts.
	RetainVersions int
	Signing        map[string]any
	Backends       map[string]map[string]any
	PublishTo      []string
	// Directory is optional bootstrap-document publish settings (SPEC §16).
	Directory *DirectoryConfig
	// Changelog is optional. When set, stage can auto-fill notes from File and
	// notesUrl from URLTemplate; publish compacts older index nodes to links.
	Changelog ChangelogConfig
	// Site is optional copy for the human-facing release portal. Publish writes
	// it to site/<product>.json; protocol clients never read it.
	Site SiteConfig
}

// DirectoryConfig mirrors the optional relkit.json "directory" object.
type DirectoryConfig struct {
	PublishTo []string `json:"publishTo,omitempty"`
	// EntryURLs documents the App-embedded entry list (primary → backups).
	// Not uploaded by the CLI; publish writes directory/<product>.pb to PublishTo.
	EntryURLs []string                 `json:"entryUrls,omitempty"`
	Services  []DirectoryServiceConfig `json:"services,omitempty"`
}

// DirectoryServiceConfig is one UpdateDirectory.services entry template.
type DirectoryServiceConfig struct {
	ID          string `json:"id,omitempty"`
	Priority    int32  `json:"priority,omitempty"`
	IndexURL    string `json:"indexUrl,omitempty"`
	FallbackURL string `json:"fallbackUrl,omitempty"`
	Channel     string `json:"channel,omitempty"`
}

// ChangelogConfig mirrors the optional relkit.json "changelog" object.
type ChangelogConfig struct {
	File        string `json:"file,omitempty"`
	URLTemplate string `json:"urlTemplate,omitempty"`
}

type SiteConfig struct {
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	Homepage    string `json:"homepage,omitempty"`
	// Makers deploys the human index to EdgeOne Pages after a public publish.
	// Intranet local/http-put backends skip this even when it is set.
	Makers *MakersConfig `json:"makers,omitempty"`
}

const DefaultMakersTokenEnv = "EDGEONE_PAGES_API_TOKEN"

// MakersConfig is the optional relkit.json "site.makers" object.
type MakersConfig struct {
	ProjectID string `json:"projectId"`
	TokenEnv  string `json:"tokenEnv,omitempty"`
	// Region is "china" (default) or "global".
	Region string `json:"region,omitempty"`
}

func FindConfig(start string) (string, error) {
	current := start
	if current == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		current = cwd
	}
	resolved, err := filepath.Abs(current)
	if err != nil {
		return "", err
	}

	for {
		candidate := filepath.Join(resolved, ConfigName)
		info, err := os.Stat(candidate)
		if err == nil && !info.IsDir() {
			return candidate, nil
		}
		parent := filepath.Dir(resolved)
		if parent == resolved {
			return "", nil
		}
		resolved = parent
	}
}

func Load(path string) (*Config, error) {
	resolved := path
	var err error
	if resolved == "" {
		resolved, err = FindConfig("")
		if err != nil {
			return nil, err
		}
		if resolved == "" {
			return nil, Error{Message: fmt.Sprintf("no %s found in the current directory or any parent; run 'relkit init' to create one", ConfigName)}
		}
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(resolved)
	if err != nil || info.IsDir() {
		return nil, Error{Message: fmt.Sprintf("config not found: %s", resolved)}
	}

	var raw map[string]any
	if err := jsonio.LoadPathLenient(resolved, &raw); err != nil {
		return nil, Error{Message: fmt.Sprintf("%s is not valid JSON: %v", resolved, err)}
	}

	cfg := &Config{
		Path: resolved,
		Root: filepath.Dir(resolved),
		Raw:  raw,
	}

	product, ok := raw["product"].(string)
	if !ok {
		return nil, Error{Message: fmt.Sprintf("%s is missing required field %q", resolved, "product")}
	}
	if err := model.CheckIdentifier(product, "product"); err != nil {
		return nil, err
	}
	cfg.Product = product

	cfg.DefaultChannel = "stable"
	if value, ok := raw["defaultChannel"]; ok {
		asString, ok := value.(string)
		if !ok {
			return nil, Error{Message: "defaultChannel must be a string"}
		}
		if err := model.CheckIdentifier(asString, "defaultChannel"); err != nil {
			return nil, err
		}
		cfg.DefaultChannel = asString
	}

	cfg.Channels = []string{cfg.DefaultChannel}
	if value, ok := raw["channels"]; ok {
		list, err := stringList(value)
		if err != nil {
			return nil, err
		}
		cfg.Channels = list
	}
	if !contains(cfg.Channels, cfg.DefaultChannel) {
		return nil, Error{Message: fmt.Sprintf("defaultChannel %q is not listed in channels %q", cfg.DefaultChannel, cfg.Channels)}
	}
	for _, channel := range cfg.Channels {
		if err := model.CheckIdentifier(channel, "channel"); err != nil {
			return nil, err
		}
	}

	cfg.CodeStrategy = "explicit"
	if value, ok := raw["codeStrategy"]; ok {
		asString, ok := value.(string)
		if !ok {
			return nil, Error{Message: "codeStrategy must be a string"}
		}
		cfg.CodeStrategy = asString
	}
	if err := checkCodeStrategy(cfg.CodeStrategy); err != nil {
		return nil, err
	}

	cfg.RetainVersions = 0
	if value, ok := raw["retainVersions"]; ok {
		n, err := asNonNegativeInt(value, "retainVersions")
		if err != nil {
			return nil, err
		}
		cfg.RetainVersions = n
	}

	cfg.Signing = map[string]any{}
	if value, ok := raw["signing"]; ok {
		obj, ok := value.(map[string]any)
		if !ok {
			return nil, Error{Message: "signing must be an object"}
		}
		cfg.Signing = obj
	}

	backendsValue, ok := raw["backends"]
	if !ok {
		return nil, Error{Message: "at least one backend must be configured"}
	}
	backendObjects, ok := backendsValue.(map[string]any)
	if !ok || len(backendObjects) == 0 {
		return nil, Error{Message: "at least one backend must be configured"}
	}
	cfg.Backends = make(map[string]map[string]any, len(backendObjects))
	for name, entry := range backendObjects {
		obj, ok := entry.(map[string]any)
		if !ok {
			return nil, Error{Message: fmt.Sprintf("backend %q must be an object", name)}
		}
		cfg.Backends[name] = obj
	}

	if value, ok := raw["publishTo"]; ok {
		list, err := stringList(value)
		if err != nil {
			return nil, err
		}
		cfg.PublishTo = list
	} else {
		for name := range cfg.Backends {
			cfg.PublishTo = append(cfg.PublishTo, name)
		}
	}
	for _, name := range cfg.PublishTo {
		if _, ok := cfg.Backends[name]; !ok {
			return nil, Error{Message: fmt.Sprintf("publishTo names unknown backend %q (configured: %s)", name, strings.Join(sortedKeys(cfg.Backends), ", "))}
		}
	}

	if value, ok := raw["directory"]; ok {
		obj, ok := value.(map[string]any)
		if !ok {
			return nil, Error{Message: "directory must be an object"}
		}
		dir := &DirectoryConfig{}
		if publishTo, exists := obj["publishTo"]; exists {
			list, err := stringList(publishTo)
			if err != nil {
				return nil, Error{Message: "directory.publishTo must be a string array"}
			}
			dir.PublishTo = list
			for _, name := range dir.PublishTo {
				if _, ok := cfg.Backends[name]; !ok {
					return nil, Error{Message: fmt.Sprintf("directory.publishTo names unknown backend %q", name)}
				}
			}
		}
		if entryURLs, exists := obj["entryUrls"]; exists {
			list, err := stringList(entryURLs)
			if err != nil {
				return nil, Error{Message: "directory.entryUrls must be a string array"}
			}
			dir.EntryURLs = list
		}
		if services, exists := obj["services"]; exists {
			list, ok := services.([]any)
			if !ok {
				return nil, Error{Message: "directory.services must be an array"}
			}
			for i, rawService := range list {
				serviceObj, ok := rawService.(map[string]any)
				if !ok {
					return nil, Error{Message: fmt.Sprintf("directory.services[%d] must be an object", i)}
				}
				service := DirectoryServiceConfig{}
				id, _ := serviceObj["id"].(string)
				service.ID = id
				if priority, exists := serviceObj["priority"]; exists {
					n, err := asNonNegativeInt(priority, fmt.Sprintf("directory.services[%d].priority", i))
					if err != nil {
						return nil, err
					}
					service.Priority = int32(n)
				}
				service.IndexURL, _ = serviceObj["indexUrl"].(string)
				service.FallbackURL, _ = serviceObj["fallbackUrl"].(string)
				service.Channel, _ = serviceObj["channel"].(string)
				dir.Services = append(dir.Services, service)
			}
		}
		cfg.Directory = dir
	}

	if value, ok := raw["changelog"]; ok {
		obj, ok := value.(map[string]any)
		if !ok {
			return nil, Error{Message: "changelog must be an object"}
		}
		if file, ok := obj["file"].(string); ok {
			cfg.Changelog.File = file
		}
		if tmpl, ok := obj["urlTemplate"].(string); ok {
			cfg.Changelog.URLTemplate = tmpl
		}
		if cfg.Changelog.File == "" && cfg.Changelog.URLTemplate == "" {
			return nil, Error{Message: "changelog object needs at least one of file / urlTemplate"}
		}
	}

	if value, ok := raw["site"]; ok {
		obj, ok := value.(map[string]any)
		if !ok {
			return nil, Error{Message: "site must be an object"}
		}
		var err error
		if cfg.Site.Title, err = optionalString(obj, "title", "site.title"); err != nil {
			return nil, err
		}
		if cfg.Site.Description, err = optionalString(obj, "description", "site.description"); err != nil {
			return nil, err
		}
		if cfg.Site.Homepage, err = optionalString(obj, "homepage", "site.homepage"); err != nil {
			return nil, err
		}
		if makersRaw, ok := obj["makers"]; ok {
			mobj, ok := makersRaw.(map[string]any)
			if !ok {
				return nil, Error{Message: "site.makers must be an object"}
			}
			makers := &MakersConfig{}
			if makers.ProjectID, err = optionalString(mobj, "projectId", "site.makers.projectId"); err != nil {
				return nil, err
			}
			if makers.ProjectID == "" {
				return nil, Error{Message: "site.makers.projectId is required"}
			}
			if makers.TokenEnv, err = optionalString(mobj, "tokenEnv", "site.makers.tokenEnv"); err != nil {
				return nil, err
			}
			if makers.TokenEnv == "" {
				makers.TokenEnv = DefaultMakersTokenEnv
			}
			if makers.Region, err = optionalString(mobj, "region", "site.makers.region"); err != nil {
				return nil, err
			}
			if makers.Region == "" {
				makers.Region = "china"
			}
			switch makers.Region {
			case "china", "global":
			default:
				return nil, Error{Message: `site.makers.region must be "china" or "global"`}
			}
			cfg.Site.Makers = makers
		}
		if cfg.Site.Title == "" && cfg.Site.Description == "" && cfg.Site.Homepage == "" && cfg.Site.Makers == nil {
			return nil, Error{Message: "site needs at least one of title / description / homepage / makers"}
		}
	}

	return cfg, nil
}

func (c *Config) ResolveCode(version string, explicit *int) (int, error) {
	if c.CodeStrategy == "explicit" {
		if explicit == nil {
			return 0, Error{Message: "codeStrategy is 'explicit', so --code is required"}
		}
		return *explicit, nil
	}

	derived, err := c.deriveCode(version, c.CodeStrategy)
	if err != nil {
		return 0, err
	}
	if explicit != nil && *explicit != derived {
		return 0, Error{Message: fmt.Sprintf("--code %d conflicts with codeStrategy %q which yields %d for version %q; change the version, the strategy, or drop --code", *explicit, c.CodeStrategy, derived, version)}
	}
	return derived, nil
}

func (c *Config) BackendConfig(name string) (map[string]any, error) {
	entry, ok := c.Backends[name]
	if !ok {
		return nil, Error{Message: fmt.Sprintf("unknown backend %q (configured: %s)", name, strings.Join(sortedKeys(c.Backends), ", "))}
	}
	if _, ok := entry["type"]; !ok {
		return nil, Error{Message: fmt.Sprintf("backend %q has no 'type'", name)}
	}
	cloned := make(map[string]any, len(entry))
	for key, value := range entry {
		cloned[key] = value
	}
	return cloned, nil
}

func (c *Config) LoadSigners() ([]envelope.Signer, error) {
	keyID, _ := c.Signing["keyId"].(string)
	if keyID == "" {
		return nil, Error{Message: "signing.keyId is required in order to publish"}
	}

	if rawPath, ok := c.Signing["privateKeyPath"]; ok && rawPath != nil {
		keyPath, ok := rawPath.(string)
		if !ok {
			return nil, Error{Message: "signing.privateKeyPath must be a string or null"}
		}
		resolved := keyPath
		if !filepath.IsAbs(resolved) {
			resolved = filepath.Join(c.Root, keyPath)
		}
		info, err := os.Stat(resolved)
		if err != nil || info.IsDir() {
			return nil, Error{Message: fmt.Sprintf("signing.privateKeyPath not found: %s", resolved)}
		}
		data, err := os.ReadFile(resolved)
		if err != nil {
			return nil, err
		}
		document, err := rupv2.UnmarshalPrivateKey(data)
		if err != nil {
			return nil, err
		}
		seed, err := keys.LoadPrivateSeed(*document, resolved)
		if err != nil {
			return nil, err
		}
		return []envelope.Signer{{KeyID: keyID, Seed: seed}}, nil
	}

	return nil, Error{Message: "no private key available: set signing.privateKeyPath to this product's key file"}
}

func (c *Config) TrustedPublicKeys() (map[string]ed25519.PublicKey, error) {
	value, ok := c.Signing["publicKeys"]
	if !ok {
		return nil, Error{Message: "signing.publicKeys is empty; add the public key(s) clients embed so that verification is possible without the private key"}
	}
	list, ok := value.([]any)
	if !ok || len(list) == 0 {
		return nil, Error{Message: "signing.publicKeys is empty; add the public key(s) clients embed so that verification is possible without the private key"}
	}

	trusted := make(map[string]ed25519.PublicKey, len(list))
	for _, entry := range list {
		obj, ok := entry.(map[string]any)
		if !ok {
			return nil, Error{Message: "every signing.publicKeys entry must be an object"}
		}
		keyID, _ := obj["keyId"].(string)
		if keyID == "" {
			return nil, Error{Message: "every signing.publicKeys entry needs a keyId"}
		}
		publicKeyBase64, _ := obj["publicKeyBase64"].(string)
		publicKey, err := keys.DecodePublicKey(publicKeyBase64, fmt.Sprintf("signing.publicKeys[%s]", keyID))
		if err != nil {
			return nil, err
		}
		trusted[keyID] = publicKey
	}
	return trusted, nil
}

func (c *Config) ChannelOrDefault(channel string) (string, error) {
	if channel == "" {
		return c.DefaultChannel, nil
	}
	if !contains(c.Channels, channel) {
		return "", Error{Message: fmt.Sprintf("channel %q is not listed in channels %q", channel, c.Channels)}
	}
	return channel, nil
}

func Skeleton(product string) map[string]any {
	return map[string]any{
		"product":        product,
		"defaultChannel": "stable",
		"channels":       []string{"stable", "beta"},
		"codeStrategy":   "version-build",
		"signing": map[string]any{
			"keyId":          "k1",
			"privateKeyPath": ".relkit-keys/k1.private.pb",
			"publicKeys":     []any{},
		},
		"backends": map[string]any{
			"local": map[string]any{
				"type":      "local",
				"outputDir": "dist/publish",
				"baseUrl":   "https://example.invalid/rup/",
			},
		},
		"publishTo": []string{"local"},
		"site": map[string]any{
			"title":       product,
			"description": "",
			"homepage":    "",
		},
	}
}

func ParseJSONDocument(path string) (map[string]any, error) {
	var raw map[string]any
	if err := jsonio.LoadPathLenient(path, &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

func PrettyDocument(doc map[string]any) ([]byte, error) {
	return jsonio.MarshalPretty(doc)
}

func checkCodeStrategy(strategy string) error {
	switch strategy {
	case "explicit", "semver", "version-build":
		return nil
	}
	if envStrategyPattern.MatchString(strategy) {
		return nil
	}
	return Error{Message: fmt.Sprintf("unknown codeStrategy %q; expected one of explicit, semver, version-build, or env:VAR, or env:VAR+N", strategy)}
}

func (c *Config) deriveCode(version, strategy string) (int, error) {
	switch strategy {
	case "semver":
		return semverCode(version)
	case "version-build":
		return versionBuildCode(version)
	}

	matched := envStrategyPattern.FindStringSubmatch(strategy)
	name := matched[1]
	offset := 0
	if matched[2] != "" {
		parsed, err := strconv.Atoi(matched[2])
		if err != nil {
			return 0, err
		}
		offset = parsed
	}
	raw := os.Getenv(name)
	if raw == "" {
		return 0, Error{Message: fmt.Sprintf("codeStrategy %q requires environment variable %s to be set", strategy, name)}
	}
	if !digitsOnly(strings.TrimSpace(raw)) {
		return 0, Error{Message: fmt.Sprintf("environment variable %s must hold a non-negative integer, got %q", name, raw)}
	}
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return 0, err
	}
	return value + offset, nil
}

func semverCode(version string) (int, error) {
	core := strings.Split(strings.Split(version, "+")[0], "-")[0]
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return 0, Error{Message: fmt.Sprintf("codeStrategy 'semver' needs a major.minor.patch version, got %q", version)}
	}
	values := make([]int, 3)
	for i, part := range parts {
		value, err := strconv.Atoi(part)
		if err != nil {
			return 0, Error{Message: fmt.Sprintf("version %q has non-numeric components", version)}
		}
		values[i] = value
	}
	if values[1] > 999 || values[2] > 999 {
		return 0, Error{Message: fmt.Sprintf("version %q would overflow the semver code encoding (minor and patch must each stay under 1000); use an explicit code instead", version)}
	}
	return values[0]*1_000_000 + values[1]*1_000 + values[2], nil
}

func versionBuildCode(version string) (int, error) {
	if !strings.Contains(version, "+") {
		return 0, Error{Message: fmt.Sprintf("codeStrategy 'version-build' needs a '+build' segment, got %q", version)}
	}
	build := version[strings.LastIndex(version, "+")+1:]
	if !digitsOnly(build) {
		return 0, Error{Message: fmt.Sprintf("build segment of %q is not a non-negative integer", version)}
	}
	value, err := strconv.Atoi(build)
	if err != nil {
		return 0, err
	}
	return value, nil
}

func stringList(value any) ([]string, error) {
	items, ok := value.([]any)
	if !ok {
		return nil, Error{Message: "expected an array of strings"}
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		asString, ok := item.(string)
		if !ok {
			return nil, Error{Message: "expected an array of strings"}
		}
		result = append(result, asString)
	}
	return result, nil
}

func optionalString(obj map[string]any, key, field string) (string, error) {
	value, exists := obj[key]
	if !exists {
		return "", nil
	}
	text, ok := value.(string)
	if !ok {
		return "", Error{Message: field + " must be a string"}
	}
	return strings.TrimSpace(text), nil
}

func asNonNegativeInt(value any, field string) (int, error) {
	switch n := value.(type) {
	case float64:
		if n != float64(int(n)) || n < 0 {
			return 0, Error{Message: fmt.Sprintf("%s must be a non-negative integer", field)}
		}
		return int(n), nil
	case int:
		if n < 0 {
			return 0, Error{Message: fmt.Sprintf("%s must be a non-negative integer", field)}
		}
		return n, nil
	case json.Number:
		parsed, err := n.Int64()
		if err != nil || parsed < 0 {
			return 0, Error{Message: fmt.Sprintf("%s must be a non-negative integer", field)}
		}
		return int(parsed), nil
	default:
		return 0, Error{Message: fmt.Sprintf("%s must be a non-negative integer", field)}
	}
}

func sortedKeys[V any](input map[string]V) []string {
	keys := make([]string, 0, len(input))
	for key := range input {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func contains(list []string, item string) bool {
	for _, candidate := range list {
		if candidate == item {
			return true
		}
	}
	return false
}

func digitsOnly(value string) bool {
	if value == "" {
		return false
	}
	for _, ch := range value {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}

func ParseRawJSON(data []byte) (map[string]any, error) {
	var value map[string]any
	if err := json.Unmarshal(data, &value); err != nil {
		return nil, err
	}
	return value, nil
}
