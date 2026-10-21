package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/shichao402/relkit/internal/keys"
	"github.com/shichao402/relkit/internal/model"
)

// PublicKeyConfig is one client-embedded verification key. PublishProfile keeps
// the set so a release can carry key rotation windows without the private half.
type PublicKeyConfig struct {
	KeyID           string `json:"keyId"`
	PublicKeyBase64 string `json:"publicKeyBase64,omitempty"`
	// Key preserves the legacy public-key spelling accepted by older configs.
	Key string `json:"key,omitempty"`
}

// PublishProfile is the machine-owned publishing part of relkit
// configuration. vNext binds it to one product through the product id plus the
// staged project/resolution bundle; the retired relkit.json control plane is
// no longer a source for any of these fields (ADR 0019 §1).
type PublishProfile struct {
	Product   string                    `json:"product"`
	Signing   PublishSigningProfile     `json:"signing"`
	Backends  map[string]map[string]any `json:"backends"`
	PublishTo []string                  `json:"publishTo,omitempty"`
	Directory *PublishDirectoryProfile  `json:"directory,omitempty"`
}

type PublishSigningProfile struct {
	KeyID          string              `json:"keyId,omitempty"`
	PrivateKeyEnv  string              `json:"privateKeyEnv,omitempty"`
	PrivateKeyPath string              `json:"privateKeyPath,omitempty"`
	Keys           []PublishSigningKey `json:"keys,omitempty"`
	PublicKeys     []PublicKeyConfig   `json:"publicKeys,omitempty"`
}

type PublishSigningKey struct {
	KeyID          string `json:"keyId"`
	PrivateKeyPath string `json:"privateKeyPath"`
}

type PublishDirectoryProfile struct {
	PublishTo []string                 `json:"publishTo,omitempty"`
	EntryURLs []string                 `json:"entryUrls,omitempty"`
	Services  []DirectoryServiceConfig `json:"services,omitempty"`
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

func validatePublishProfile(profile *PublishProfile) error {
	if profile == nil {
		return Error{Message: "publish profile is nil"}
	}
	if err := model.CheckIdentifier(profile.Product, "product"); err != nil {
		return err
	}
	if len(profile.Signing.Keys) == 0 {
		if err := model.CheckIdentifier(profile.Signing.KeyID, "signing.keyId"); err != nil {
			return err
		}
	} else {
		if profile.Signing.KeyID != "" || profile.Signing.PrivateKeyPath != "" || profile.Signing.PrivateKeyEnv != "" {
			return Error{Message: "signing.keys cannot be combined with signing.keyId/privateKeyPath/privateKeyEnv"}
		}
		seen := map[string]bool{}
		for index, key := range profile.Signing.Keys {
			if err := model.CheckIdentifier(key.KeyID, fmt.Sprintf("signing.keys[%d].keyId", index)); err != nil {
				return err
			}
			if strings.TrimSpace(key.PrivateKeyPath) == "" {
				return Error{Message: fmt.Sprintf("signing.keys[%d].privateKeyPath is required", index)}
			}
			if seen[key.KeyID] {
				return Error{Message: fmt.Sprintf("signing.keys contains duplicate keyId %q", key.KeyID)}
			}
			seen[key.KeyID] = true
		}
	}
	publicKeyIDs := map[string]bool{}
	for index, key := range profile.Signing.PublicKeys {
		if key.KeyID == "" || (key.PublicKeyBase64 == "" && key.Key == "") {
			return Error{Message: fmt.Sprintf("signing.publicKeys[%d] requires keyId and publicKeyBase64 (or legacy key)", index)}
		}
		if publicKeyIDs[key.KeyID] {
			return Error{Message: fmt.Sprintf("signing.publicKeys contains duplicate keyId %q", key.KeyID)}
		}
		publicKeyIDs[key.KeyID] = true
		encoded := key.PublicKeyBase64
		if encoded == "" {
			encoded = key.Key
		}
		if _, err := keys.DecodePublicKey(encoded, fmt.Sprintf("signing.publicKeys[%s]", key.KeyID)); err != nil {
			return err
		}
	}
	if len(profile.Signing.Keys) > 0 && len(profile.Signing.PublicKeys) > 0 {
		for _, signer := range profile.Signing.Keys {
			if !publicKeyIDs[signer.KeyID] {
				return Error{Message: fmt.Sprintf("signer %q is missing from signing.publicKeys", signer.KeyID)}
			}
		}
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
