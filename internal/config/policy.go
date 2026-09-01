package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"cnb.cool/shichao402/relkit/internal/jsonio"
	"cnb.cool/shichao402/relkit/internal/model"
)

// ProductPolicy is the repository-owned, portable part of relkit
// configuration. It intentionally has no publishing credentials or targets.
type ProductPolicy struct {
	Product        string                  `json:"product"`
	DefaultChannel string                  `json:"defaultChannel"`
	Channels       []string                `json:"channels"`
	CodeStrategy   string                  `json:"codeStrategy"`
	RetainVersions int                     `json:"retainVersions,omitempty"`
	Signing        ProductSigningPolicy    `json:"signing,omitempty"`
	Changelog      ProductChangelogPolicy  `json:"changelog,omitempty"`
	Site           ProductSitePolicy       `json:"site,omitempty"`
	Directory      *ProductDirectoryPolicy `json:"directory,omitempty"`
}

// ProductChangelogPolicy is the portable changelog subset needed at publish
// time (compacting older notes to links). Local markdown paths stay in the
// repository relkit.json and must not travel with a staged release.
type ProductChangelogPolicy struct {
	URLTemplate string `json:"urlTemplate,omitempty"`
}

type ProductSigningPolicy struct {
	KeyID      string            `json:"keyId,omitempty"`
	PublicKeys []PublicKeyConfig `json:"publicKeys,omitempty"`
}

type PublicKeyConfig struct {
	KeyID           string `json:"keyId"`
	PublicKeyBase64 string `json:"publicKeyBase64,omitempty"`
	// Key preserves the legacy public-key spelling accepted by older configs.
	Key string `json:"key,omitempty"`
}

type ProductSitePolicy struct {
	Title       string               `json:"title,omitempty"`
	Description string               `json:"description,omitempty"`
	Homepage    string               `json:"homepage,omitempty"`
	Makers      *ProductMakersPolicy `json:"makers,omitempty"`
}

type ProductMakersPolicy struct {
	ProjectID string `json:"projectId"`
	Region    string `json:"region,omitempty"`
}

type ProductDirectoryPolicy struct {
	EntryURLs []string                 `json:"entryUrls,omitempty"`
	Services  []DirectoryServiceConfig `json:"services,omitempty"`
}

// PublishProfile is the machine-owned publishing part of relkit
// configuration. Product and signing.keyId bind it to one ProductPolicy.
type PublishProfile struct {
	Product   string                    `json:"product"`
	Signing   PublishSigningProfile     `json:"signing"`
	Backends  map[string]map[string]any `json:"backends"`
	PublishTo []string                  `json:"publishTo,omitempty"`
	Directory *PublishDirectoryProfile  `json:"directory,omitempty"`
	Site      PublishSiteProfile        `json:"site,omitempty"`
}

type PublishSigningProfile struct {
	KeyID          string `json:"keyId"`
	PrivateKeyEnv  string `json:"privateKeyEnv,omitempty"`
	PrivateKeyPath string `json:"privateKeyPath,omitempty"`
}

type PublishDirectoryProfile struct {
	PublishTo []string `json:"publishTo,omitempty"`
}

type PublishSiteProfile struct {
	Makers *PublishMakersProfile `json:"makers,omitempty"`
}

type PublishMakersProfile struct {
	TokenEnv string `json:"tokenEnv,omitempty"`
}

func LoadProductPolicy(path string) (*ProductPolicy, error) {
	var policy ProductPolicy
	if err := loadStrict(path, &policy); err != nil {
		return nil, Error{Message: fmt.Sprintf("%s is not a valid product policy: %v", path, err)}
	}
	if err := validateProductPolicy(&policy); err != nil {
		return nil, err
	}
	return &policy, nil
}

func LoadPublishProfile(path string) (*PublishProfile, error) {
	var profile PublishProfile
	if err := loadStrict(path, &profile); err != nil {
		return nil, Error{Message: fmt.Sprintf("%s is not a valid publish profile: %v", path, err)}
	}
	if err := validatePublishProfile(&profile); err != nil {
		return nil, err
	}
	return &profile, nil
}

// ExtractProductPolicy returns a deep, typed projection that cannot contain
// private key references, backend credentials, publish targets, or Makers token
// environment names.
func ExtractProductPolicy(cfg *Config) (*ProductPolicy, error) {
	if cfg == nil {
		return nil, Error{Message: "config is nil"}
	}
	policy := &ProductPolicy{
		Product:        cfg.Product,
		DefaultChannel: cfg.DefaultChannel,
		Channels:       append([]string(nil), cfg.Channels...),
		CodeStrategy:   cfg.CodeStrategy,
		RetainVersions: cfg.RetainVersions,
		Changelog:      ProductChangelogPolicy{URLTemplate: strings.TrimSpace(cfg.Changelog.URLTemplate)},
		Site: ProductSitePolicy{
			Title:       cfg.Site.Title,
			Description: cfg.Site.Description,
			Homepage:    cfg.Site.Homepage,
		},
	}
	policy.Signing.KeyID, _ = cfg.Signing["keyId"].(string)
	if rawKeys, ok := cfg.Signing["publicKeys"].([]any); ok {
		for i, rawKey := range rawKeys {
			entry, ok := rawKey.(map[string]any)
			if !ok {
				return nil, Error{Message: fmt.Sprintf("signing.publicKeys[%d] must be an object", i)}
			}
			keyID, keyIDOK := entry["keyId"].(string)
			publicKey, _ := entry["publicKeyBase64"].(string)
			legacyKey, _ := entry["key"].(string)
			if !keyIDOK || (publicKey == "" && legacyKey == "") {
				return nil, Error{Message: fmt.Sprintf("signing.publicKeys[%d] needs string keyId and publicKeyBase64 (or legacy key)", i)}
			}
			policy.Signing.PublicKeys = append(policy.Signing.PublicKeys, PublicKeyConfig{KeyID: keyID, PublicKeyBase64: publicKey, Key: legacyKey})
		}
	} else if _, exists := cfg.Signing["publicKeys"]; exists {
		return nil, Error{Message: "signing.publicKeys must be an array"}
	}
	if cfg.Site.Makers != nil {
		policy.Site.Makers = &ProductMakersPolicy{
			ProjectID: cfg.Site.Makers.ProjectID,
			Region:    cfg.Site.Makers.Region,
		}
	}
	if cfg.Directory != nil {
		policy.Directory = &ProductDirectoryPolicy{
			EntryURLs: append([]string(nil), cfg.Directory.EntryURLs...),
			Services:  append([]DirectoryServiceConfig(nil), cfg.Directory.Services...),
		}
	}
	if err := validateProductPolicy(policy); err != nil {
		return nil, err
	}
	return policy, nil
}

// ExtractPublishProfile returns the machine-owned projection of an existing
// Config. Backend values are cloned through JSON to avoid sharing nested data.
func ExtractPublishProfile(cfg *Config) (*PublishProfile, error) {
	if cfg == nil {
		return nil, Error{Message: "config is nil"}
	}
	profile := &PublishProfile{
		Product:   cfg.Product,
		Backends:  cloneBackends(cfg.Backends),
		PublishTo: append([]string(nil), cfg.PublishTo...),
	}
	profile.Signing.KeyID, _ = cfg.Signing["keyId"].(string)
	profile.Signing.PrivateKeyEnv, _ = cfg.Signing["privateKeyEnv"].(string)
	profile.Signing.PrivateKeyPath, _ = cfg.Signing["privateKeyPath"].(string)
	if cfg.Directory != nil && len(cfg.Directory.PublishTo) > 0 {
		profile.Directory = &PublishDirectoryProfile{PublishTo: append([]string(nil), cfg.Directory.PublishTo...)}
	}
	if cfg.Site.Makers != nil && cfg.Site.Makers.TokenEnv != "" {
		profile.Site.Makers = &PublishMakersProfile{TokenEnv: cfg.Site.Makers.TokenEnv}
	}
	if err := validatePublishProfile(profile); err != nil {
		return nil, err
	}
	return profile, nil
}

// MergeProductPolicy combines repository and machine configuration. productRoot
// is required and always becomes Config.Root; document source paths are never
// allowed to determine the product root.
func MergeProductPolicy(policy *ProductPolicy, profile *PublishProfile, productRoot string) (*Config, error) {
	if err := validateProductPolicy(policy); err != nil {
		return nil, err
	}
	if err := validatePublishProfile(profile); err != nil {
		return nil, err
	}
	if strings.TrimSpace(productRoot) == "" {
		return nil, Error{Message: "product root is required"}
	}
	root, err := filepath.Abs(productRoot)
	if err != nil {
		return nil, err
	}
	if policy.Product != profile.Product {
		return nil, Error{Message: fmt.Sprintf("publish profile product %q does not match policy product %q", profile.Product, policy.Product)}
	}
	if policy.Signing.KeyID != profile.Signing.KeyID {
		return nil, Error{Message: fmt.Sprintf("publish profile signing.keyId %q does not match policy signing.keyId %q", profile.Signing.KeyID, policy.Signing.KeyID)}
	}

	cfg := &Config{
		Root:           root,
		Product:        policy.Product,
		DefaultChannel: policy.DefaultChannel,
		Channels:       append([]string(nil), policy.Channels...),
		CodeStrategy:   policy.CodeStrategy,
		RetainVersions: policy.RetainVersions,
		Signing:        map[string]any{"keyId": policy.Signing.KeyID},
		Backends:       cloneBackends(profile.Backends),
		PublishTo:      append([]string(nil), profile.PublishTo...),
		Changelog:      ChangelogConfig{URLTemplate: policy.Changelog.URLTemplate},
		Site: SiteConfig{
			Title:       policy.Site.Title,
			Description: policy.Site.Description,
			Homepage:    policy.Site.Homepage,
		},
	}
	if len(policy.Signing.PublicKeys) > 0 {
		keys := make([]any, 0, len(policy.Signing.PublicKeys))
		for _, key := range policy.Signing.PublicKeys {
			entry := map[string]any{"keyId": key.KeyID}
			if key.PublicKeyBase64 != "" {
				entry["publicKeyBase64"] = key.PublicKeyBase64
			}
			if key.Key != "" {
				entry["key"] = key.Key
			}
			keys = append(keys, entry)
		}
		cfg.Signing["publicKeys"] = keys
	}
	if profile.Signing.PrivateKeyEnv != "" {
		cfg.Signing["privateKeyEnv"] = profile.Signing.PrivateKeyEnv
	}
	if profile.Signing.PrivateKeyPath != "" {
		cfg.Signing["privateKeyPath"] = profile.Signing.PrivateKeyPath
	}
	if policy.Directory != nil || profile.Directory != nil {
		cfg.Directory = &DirectoryConfig{}
		if policy.Directory != nil {
			cfg.Directory.EntryURLs = append([]string(nil), policy.Directory.EntryURLs...)
			cfg.Directory.Services = append([]DirectoryServiceConfig(nil), policy.Directory.Services...)
		}
		if profile.Directory != nil {
			cfg.Directory.PublishTo = append([]string(nil), profile.Directory.PublishTo...)
		}
	}
	if policy.Site.Makers != nil {
		cfg.Site.Makers = &MakersConfig{
			ProjectID: policy.Site.Makers.ProjectID,
			Region:    policy.Site.Makers.Region,
		}
		if profile.Site.Makers != nil {
			cfg.Site.Makers.TokenEnv = profile.Site.Makers.TokenEnv
		}
		if cfg.Site.Makers.TokenEnv == "" {
			cfg.Site.Makers.TokenEnv = DefaultMakersTokenEnv
		}
	}
	raw, err := mergedRaw(cfg)
	if err != nil {
		return nil, err
	}
	cfg.Raw = raw
	return cfg, nil
}

// Merge is a concise alias for MergeProductPolicy.
func Merge(policy *ProductPolicy, profile *PublishProfile, productRoot string) (*Config, error) {
	return MergeProductPolicy(policy, profile, productRoot)
}

func validateProductPolicy(policy *ProductPolicy) error {
	if policy == nil {
		return Error{Message: "product policy is nil"}
	}
	if err := model.CheckIdentifier(policy.Product, "product"); err != nil {
		return err
	}
	if err := model.CheckIdentifier(policy.DefaultChannel, "defaultChannel"); err != nil {
		return err
	}
	if len(policy.Channels) == 0 {
		return Error{Message: "channels must not be empty"}
	}
	if !contains(policy.Channels, policy.DefaultChannel) {
		return Error{Message: fmt.Sprintf("defaultChannel %q is not listed in channels %q", policy.DefaultChannel, policy.Channels)}
	}
	for _, channel := range policy.Channels {
		if err := model.CheckIdentifier(channel, "channel"); err != nil {
			return err
		}
	}
	if err := checkCodeStrategy(policy.CodeStrategy); err != nil {
		return err
	}
	if policy.RetainVersions < 0 {
		return Error{Message: "retainVersions must be a non-negative integer"}
	}
	if policy.Signing.KeyID != "" {
		if err := model.CheckIdentifier(policy.Signing.KeyID, "signing.keyId"); err != nil {
			return err
		}
	}
	for i, key := range policy.Signing.PublicKeys {
		if key.KeyID == "" || (key.PublicKeyBase64 == "" && key.Key == "") {
			return Error{Message: fmt.Sprintf("signing.publicKeys[%d] requires keyId and publicKeyBase64 (or legacy key)", i)}
		}
	}
	if policy.Site.Makers != nil {
		if policy.Site.Makers.ProjectID == "" {
			return Error{Message: "site.makers.projectId is required"}
		}
		switch policy.Site.Makers.Region {
		case "", "china", "global":
		default:
			return Error{Message: `site.makers.region must be "china" or "global"`}
		}
	}
	if _, err := json.Marshal(policy); err != nil {
		return Error{Message: fmt.Sprintf("product policy is not serializable: %v", err)}
	}
	return nil
}

func validatePublishProfile(profile *PublishProfile) error {
	if profile == nil {
		return Error{Message: "publish profile is nil"}
	}
	if err := model.CheckIdentifier(profile.Product, "product"); err != nil {
		return err
	}
	if err := model.CheckIdentifier(profile.Signing.KeyID, "signing.keyId"); err != nil {
		return err
	}
	if len(profile.Backends) == 0 {
		return Error{Message: "at least one backend must be configured"}
	}
	for name, backend := range profile.Backends {
		if backend == nil {
			return Error{Message: fmt.Sprintf("backend %q must be an object", name)}
		}
	}
	checkTargets := func(field string, targets []string) error {
		for _, name := range targets {
			if _, ok := profile.Backends[name]; !ok {
				return Error{Message: fmt.Sprintf("%s names unknown backend %q (configured: %s)", field, name, strings.Join(sortedKeys(profile.Backends), ", "))}
			}
		}
		return nil
	}
	if err := checkTargets("publishTo", profile.PublishTo); err != nil {
		return err
	}
	if profile.Directory != nil {
		if err := checkTargets("directory.publishTo", profile.Directory.PublishTo); err != nil {
			return err
		}
	}
	if _, err := json.Marshal(profile); err != nil {
		return Error{Message: fmt.Sprintf("publish profile is not serializable: %v", err)}
	}
	return nil
}

func loadStrict(path string, dest any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dest); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err == nil {
		return fmt.Errorf("unexpected trailing JSON content")
	} else if err != io.EOF {
		return err
	}
	return nil
}

func cloneBackends(backends map[string]map[string]any) map[string]map[string]any {
	cloned := make(map[string]map[string]any, len(backends))
	for name, backend := range backends {
		data, err := json.Marshal(backend)
		if err != nil {
			cloned[name] = backend
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal(data, &entry); err != nil {
			cloned[name] = backend
			continue
		}
		cloned[name] = entry
	}
	return cloned
}

func mergedRaw(cfg *Config) (map[string]any, error) {
	doc := map[string]any{
		"product":        cfg.Product,
		"defaultChannel": cfg.DefaultChannel,
		"channels":       cfg.Channels,
		"codeStrategy":   cfg.CodeStrategy,
		"signing":        cfg.Signing,
		"backends":       cfg.Backends,
		"publishTo":      cfg.PublishTo,
	}
	if cfg.RetainVersions != 0 {
		doc["retainVersions"] = cfg.RetainVersions
	}
	if cfg.Changelog.File != "" || cfg.Changelog.URLTemplate != "" {
		doc["changelog"] = cfg.Changelog
	}
	if cfg.Directory != nil {
		doc["directory"] = cfg.Directory
	}
	if cfg.Site.Title != "" || cfg.Site.Description != "" || cfg.Site.Homepage != "" || cfg.Site.Makers != nil {
		doc["site"] = cfg.Site
	}
	data, err := jsonio.MarshalCompact(doc)
	if err != nil {
		return nil, err
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	return raw, nil
}
