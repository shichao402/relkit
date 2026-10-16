// Package runtimebootstrap projects a verified Runtime Profile and its
// externally trusted product roots into the build input consumed by updater
// facades. It is deliberately unsigned: the immutable resolution bundle is
// the signed source and the resolution digest records that provenance.
package runtimebootstrap

import (
	"crypto/ed25519"
	"fmt"
	"sort"

	"github.com/shichao402/relkit/internal/vnext/canonical"
	"github.com/shichao402/relkit/internal/vnext/runtimeprofile"
	"github.com/shichao402/relkit/internal/vnext/trust"
)

const Schema = "relkit.runtime-bootstrap/1"

type TrustedKey struct {
	KeyID string `json:"keyId"`
	Key   []byte `json:"key"`
}

type Document struct {
	Schema           string                  `json:"schema"`
	ResolutionDigest string                  `json:"resolutionDigest"`
	ProfileDigest    string                  `json:"profileDigest"`
	RootFingerprint  string                  `json:"rootFingerprint"`
	Profile          runtimeprofile.Document `json:"profile"`
	TrustedKeys      []TrustedKey            `json:"trustedKeys"`
}

func New(
	resolutionDigest string,
	rootFingerprint string,
	profile runtimeprofile.Document,
	roots map[string]ed25519.PublicKey,
) (*Document, error) {
	keys := make([]TrustedKey, 0, len(roots))
	for keyID, key := range roots {
		keys = append(keys, TrustedKey{KeyID: keyID, Key: append([]byte(nil), key...)})
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i].KeyID < keys[j].KeyID })
	value := &Document{
		Schema:           Schema,
		ResolutionDigest: resolutionDigest,
		ProfileDigest:    profile.Digest,
		RootFingerprint:  rootFingerprint,
		Profile:          profile,
		TrustedKeys:      keys,
	}
	if err := value.Validate(); err != nil {
		return nil, err
	}
	return value, nil
}

func (d Document) Validate() error {
	if d.Schema != Schema {
		return fmt.Errorf("schema is %q, expected %q", d.Schema, Schema)
	}
	if err := canonical.ValidateDigest(d.ResolutionDigest); err != nil {
		return fmt.Errorf("resolutionDigest: %w", err)
	}
	if d.ProfileDigest != d.Profile.Digest {
		return fmt.Errorf("profileDigest %s does not match embedded profile %s", d.ProfileDigest, d.Profile.Digest)
	}
	if err := canonical.ValidateDigest(d.ProfileDigest); err != nil {
		return fmt.Errorf("profileDigest: %w", err)
	}
	if err := canonical.ValidateDigest(d.RootFingerprint); err != nil {
		return fmt.Errorf("rootFingerprint: %w", err)
	}
	if len(d.TrustedKeys) == 0 {
		return fmt.Errorf("trustedKeys must not be empty")
	}
	roots := make(map[string]ed25519.PublicKey, len(d.TrustedKeys))
	previous := ""
	for _, key := range d.TrustedKeys {
		if key.KeyID == "" {
			return fmt.Errorf("trusted key id must not be empty")
		}
		if previous != "" && key.KeyID <= previous {
			return fmt.Errorf("trustedKeys must be unique and sorted by keyId")
		}
		if len(key.Key) != ed25519.PublicKeySize {
			return fmt.Errorf("trusted key %q must contain a %d-byte ed25519 key", key.KeyID, ed25519.PublicKeySize)
		}
		fingerprint, err := trust.Fingerprint(ed25519.PublicKey(key.Key))
		if err != nil {
			return err
		}
		if fingerprint != d.RootFingerprint {
			return fmt.Errorf("trusted key %q fingerprint %s does not match rootFingerprint %s", key.KeyID, fingerprint, d.RootFingerprint)
		}
		roots[key.KeyID] = ed25519.PublicKey(append([]byte(nil), key.Key...))
		previous = key.KeyID
	}
	if err := d.Profile.Verify(roots); err != nil {
		return fmt.Errorf("profile: %w", err)
	}
	return nil
}
