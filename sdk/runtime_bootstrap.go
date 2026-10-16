package sdk

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"regexp"

	updaterv1 "github.com/shichao402/relkit/api/updater/v1"
)

const runtimeBootstrapSchema = "relkit.runtime-bootstrap/1"

var sha256DigestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

type RuntimeBootstrap struct {
	ResolutionDigest string
	ProfileDigest    string
	RootFingerprint  string
	ClientProfile    *updaterv1.ClientProfile
}

type runtimeBootstrapDocument struct {
	Schema           string                `json:"schema"`
	ResolutionDigest string                `json:"resolutionDigest"`
	ProfileDigest    string                `json:"profileDigest"`
	RootFingerprint  string                `json:"rootFingerprint"`
	Profile          runtimeProfile        `json:"profile"`
	TrustedKeys      []runtimeBootstrapKey `json:"trustedKeys"`
}

type runtimeProfile struct {
	Product            string                    `json:"product"`
	AllowedChannels    []string                  `json:"allowedChannels"`
	DirectoryBootstrap runtimeDirectoryBootstrap `json:"directoryBootstrap"`
	Digest             string                    `json:"digest"`
}

type runtimeDirectoryBootstrap struct {
	EntryURLs []string         `json:"entryUrls"`
	Recovery  *runtimeRecovery `json:"recovery,omitempty"`
}

type runtimeRecovery struct {
	Message string                `json:"message"`
	Links   []runtimeRecoveryLink `json:"links,omitempty"`
}

type runtimeRecoveryLink struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

type runtimeBootstrapKey struct {
	KeyID string `json:"keyId"`
	Key   string `json:"key"`
}

func RuntimeBootstrapFromJSON(data []byte) (*RuntimeBootstrap, error) {
	var document runtimeBootstrapDocument
	if err := json.Unmarshal(data, &document); err != nil {
		return nil, fmt.Errorf("invalid runtime bootstrap JSON: %w", err)
	}
	if document.Schema != runtimeBootstrapSchema {
		return nil, fmt.Errorf("runtime bootstrap schema is %q, expected %q", document.Schema, runtimeBootstrapSchema)
	}
	for name, value := range map[string]string{
		"resolutionDigest": document.ResolutionDigest,
		"profileDigest":    document.ProfileDigest,
		"rootFingerprint":  document.RootFingerprint,
	} {
		if !sha256DigestPattern.MatchString(value) {
			return nil, fmt.Errorf("%s must be a sha256 digest", name)
		}
	}
	if document.Profile.Digest != document.ProfileDigest {
		return nil, fmt.Errorf("embedded profile digest does not match profileDigest")
	}
	if document.Profile.Product == "" {
		return nil, fmt.Errorf("profile product must not be empty")
	}
	if err := validateRuntimeStrings("profile allowedChannels", document.Profile.AllowedChannels); err != nil {
		return nil, err
	}
	if err := validateRuntimeStrings("profile directoryBootstrap.entryUrls", document.Profile.DirectoryBootstrap.EntryURLs); err != nil {
		return nil, err
	}
	if len(document.TrustedKeys) == 0 {
		return nil, fmt.Errorf("trustedKeys must not be empty")
	}
	keys := make([]*updaterv1.TrustedKey, 0, len(document.TrustedKeys))
	seen := map[string]bool{}
	for _, key := range document.TrustedKeys {
		if key.KeyID == "" || seen[key.KeyID] {
			return nil, fmt.Errorf("trusted key ids must be non-empty and unique")
		}
		seen[key.KeyID] = true
		raw, err := base64.StdEncoding.DecodeString(key.Key)
		if err != nil {
			return nil, fmt.Errorf("trusted key %q is not base64: %w", key.KeyID, err)
		}
		if len(raw) != ed25519.PublicKeySize {
			return nil, fmt.Errorf("trusted key %q must contain a %d-byte ed25519 key", key.KeyID, ed25519.PublicKeySize)
		}
		keys = append(keys, &updaterv1.TrustedKey{KeyId: key.KeyID, PublicKey: raw})
	}
	var recovery *updaterv1.RecoveryHelp
	if document.Profile.DirectoryBootstrap.Recovery != nil {
		value := document.Profile.DirectoryBootstrap.Recovery
		recovery = &updaterv1.RecoveryHelp{Message: value.Message}
		for _, link := range value.Links {
			if link.Label == "" || link.URL == "" {
				return nil, fmt.Errorf("recovery links require non-empty label and url")
			}
			recovery.Links = append(recovery.Links, &updaterv1.RecoveryLink{
				Label: link.Label,
				Url:   link.URL,
			})
		}
	}
	return &RuntimeBootstrap{
		ResolutionDigest: document.ResolutionDigest,
		ProfileDigest:    document.ProfileDigest,
		RootFingerprint:  document.RootFingerprint,
		ClientProfile: &updaterv1.ClientProfile{
			Product:         document.Profile.Product,
			AllowedChannels: append([]string(nil), document.Profile.AllowedChannels...),
			EntryUrls:       append([]string(nil), document.Profile.DirectoryBootstrap.EntryURLs...),
			TrustedKeys:     keys,
			Recovery:        recovery,
		},
	}, nil
}

func validateRuntimeStrings(name string, values []string) error {
	if len(values) == 0 {
		return fmt.Errorf("%s must not be empty", name)
	}
	for _, value := range values {
		if value == "" {
			return fmt.Errorf("%s must contain non-empty strings", name)
		}
	}
	return nil
}
