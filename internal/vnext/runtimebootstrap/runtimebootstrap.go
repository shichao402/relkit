// Package runtimebootstrap produces the document a host build embeds into its
// updater: the one server root the application trusts, plus the signed
// Runtime Profile describing that server's directories.
//
// Requirement 10.6: an application trusts exactly one server root. The
// document is deliberately unsigned — the Runtime Profile it carries is signed
// by the server root, and the updater verifies it with the embedded key.
package runtimebootstrap

import (
	"crypto/ed25519"
	"fmt"

	"github.com/shichao402/relkit/internal/vnext/canonical"
	"github.com/shichao402/relkit/internal/vnext/runtimeprofile"
	"github.com/shichao402/relkit/internal/vnext/trust"
)

const Schema = "relkit.runtime-bootstrap/2"

// Document is the build input consumed by updater facades.
//
// RootKeyID and RootPublicKey name the single server root. There is no key set
// and no product root: one application talks to one server (requirement 10.7).
type Document struct {
	Schema        string                  `json:"schema"`
	RootKeyID     string                  `json:"rootKeyId"`
	RootPublicKey string                  `json:"rootPublicKey"`
	RootFingerprint string                `json:"rootFingerprint"`
	Product       string                  `json:"product"`
	Profile       runtimeprofile.Document `json:"profile"`
}

// New builds the bootstrap document for a product from the resolved server
// root. The profile must already be signed by that same root.
func New(rootKeyID string, root ed25519.PublicKey, product string, profile runtimeprofile.Document) (*Document, error) {
	fingerprint, err := trust.Fingerprint(root)
	if err != nil {
		return nil, err
	}
	value := &Document{
		Schema:          Schema,
		RootKeyID:       rootKeyID,
		RootPublicKey:   trust.SSHPublicKeyLine(root),
		RootFingerprint: fingerprint,
		Product:         product,
		Profile:         profile,
	}
	if err := value.Validate(); err != nil {
		return nil, err
	}
	return value, nil
}

// PublicKey decodes the embedded server root.
func (d Document) PublicKey() (ed25519.PublicKey, error) {
	publicKey, err := trust.ParsePublicKey(d.RootPublicKey)
	if err != nil {
		return nil, fmt.Errorf("rootPublicKey: %w", err)
	}
	return publicKey, nil
}

func (d Document) Validate() error {
	if d.Schema != Schema {
		return fmt.Errorf("schema is %q, expected %q", d.Schema, Schema)
	}
	if d.RootKeyID == "" {
		return fmt.Errorf("rootKeyId must not be empty")
	}
	root, err := d.PublicKey()
	if err != nil {
		return err
	}
	fingerprint, err := trust.Fingerprint(root)
	if err != nil {
		return err
	}
	if err := canonical.ValidateDigest(d.RootFingerprint); err != nil {
		return fmt.Errorf("rootFingerprint: %w", err)
	}
	if fingerprint != d.RootFingerprint {
		return fmt.Errorf("rootFingerprint %s does not match rootPublicKey %s", d.RootFingerprint, fingerprint)
	}
	if err := d.Profile.Validate(); err != nil {
		return fmt.Errorf("profile: %w", err)
	}
	if d.Product != "" && d.Profile.Product != d.Product {
		return fmt.Errorf("profile belongs to product %q, bootstrap declares %q", d.Profile.Product, d.Product)
	}
	// The profile must be signed by the same single root the bootstrap embeds.
	// Verify runs last so a malformed key reports before a signature mismatch.
	if err := d.Profile.Verify(map[string]ed25519.PublicKey{d.RootKeyID: root}); err != nil {
		return fmt.Errorf("profile is not signed by the embedded server root: %w", err)
	}
	return nil
}
