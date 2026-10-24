// Package runtimeprofile owns the signed cross-product bootstrap description
// consumed by subscribers such as a launcher.
package runtimeprofile

import (
	"crypto/ed25519"
	"fmt"

	"github.com/shichao402/relkit/internal/model"
	"github.com/shichao402/relkit/internal/vnext/canonical"
	"github.com/shichao402/relkit/internal/vnext/document"
	"github.com/shichao402/relkit/internal/vnext/trust"
)

const Schema = "relkit.runtime-profile/1"

// SchemaSet is the RUP wire schema set every profile advertises. It is a
// property of the engine build, not a per-project choice: hosts no longer
// negotiate protocols (requirement 14.4).
const SchemaSet = "rup.v2"

type DirectoryBootstrap struct {
	EntryURLs []string       `json:"entryUrls"`
	Recovery  map[string]any `json:"recovery,omitempty"`
}

type Document struct {
	Schema             string             `json:"schema"`
	Product            string             `json:"product"`
	ProfileVersion     int64              `json:"profileVersion"`
	AllowedChannels    []string           `json:"allowedChannels"`
	DirectoryBootstrap DirectoryBootstrap `json:"directoryBootstrap"`
	RUPSchemaSet       string             `json:"rupSchemaSet"`
	Digest             string             `json:"digest"`
	Signatures         []trust.Signature  `json:"signatures"`
}

type content struct {
	Schema             string             `json:"schema"`
	Product            string             `json:"product"`
	ProfileVersion     int64              `json:"profileVersion"`
	AllowedChannels    []string           `json:"allowedChannels"`
	DirectoryBootstrap DirectoryBootstrap `json:"directoryBootstrap"`
	RUPSchemaSet       string             `json:"rupSchemaSet"`
}

type signedContent struct {
	Schema             string             `json:"schema"`
	Product            string             `json:"product"`
	ProfileVersion     int64              `json:"profileVersion"`
	AllowedChannels    []string           `json:"allowedChannels"`
	DirectoryBootstrap DirectoryBootstrap `json:"directoryBootstrap"`
	RUPSchemaSet       string             `json:"rupSchemaSet"`
	Digest             string             `json:"digest"`
}

func Load(path string) (*Document, error) {
	var value Document
	if err := document.Load(path, &value); err != nil {
		return nil, err
	}
	if err := value.Validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &value, nil
}

func (d Document) body() content {
	return content{
		Schema:             d.Schema,
		Product:            d.Product,
		ProfileVersion:     d.ProfileVersion,
		AllowedChannels:    d.AllowedChannels,
		DirectoryBootstrap: d.DirectoryBootstrap,
		RUPSchemaSet:       d.RUPSchemaSet,
	}
}

func (d Document) signedBody() signedContent {
	return signedContent{
		Schema:             d.Schema,
		Product:            d.Product,
		ProfileVersion:     d.ProfileVersion,
		AllowedChannels:    d.AllowedChannels,
		DirectoryBootstrap: d.DirectoryBootstrap,
		RUPSchemaSet:       d.RUPSchemaSet,
		Digest:             d.Digest,
	}
}

func (d Document) validateBody() error {
	if d.Schema != Schema {
		return fmt.Errorf("schema is %q, expected %q", d.Schema, Schema)
	}
	if err := model.CheckIdentifier(d.Product, "product"); err != nil {
		return err
	}
	if d.ProfileVersion < 1 {
		return fmt.Errorf("profileVersion must be positive")
	}
	if len(d.AllowedChannels) == 0 {
		return fmt.Errorf("allowedChannels must not be empty")
	}
	if len(d.DirectoryBootstrap.EntryURLs) == 0 {
		return fmt.Errorf("directoryBootstrap.entryUrls must not be empty")
	}
	if d.RUPSchemaSet == "" {
		return fmt.Errorf("rupSchemaSet must not be empty")
	}
	return nil
}

func (d Document) Validate() error {
	if err := d.validateBody(); err != nil {
		return err
	}
	if err := canonical.ValidateDigest(d.Digest); err != nil {
		return fmt.Errorf("digest: %w", err)
	}
	expected, err := canonical.Digest(d.body())
	if err != nil {
		return err
	}
	if d.Digest != expected {
		return fmt.Errorf("runtime profile digest mismatch: got %s, expected %s", d.Digest, expected)
	}
	if err := trust.ValidateSignatures(d.Signatures); err != nil {
		return err
	}
	return nil
}

func (d *Document) Sign(keyID string, seed []byte) error {
	if d == nil {
		return fmt.Errorf("nil runtime profile")
	}
	if err := d.validateBody(); err != nil {
		return err
	}
	digest, err := canonical.Digest(d.body())
	if err != nil {
		return err
	}
	d.Digest = digest
	signature, err := trust.Sign(d.signedBody(), keyID, seed)
	if err != nil {
		return err
	}
	d.Signatures = []trust.Signature{signature}
	return d.Validate()
}

func (d *Document) AddSignature(keyID string, seed []byte) error {
	if d == nil {
		return fmt.Errorf("nil runtime profile")
	}
	if err := d.validateBody(); err != nil {
		return err
	}
	if d.Digest == "" {
		digest, err := canonical.Digest(d.body())
		if err != nil {
			return err
		}
		d.Digest = digest
	}
	signature, err := trust.Sign(d.signedBody(), keyID, seed)
	if err != nil {
		return err
	}
	for _, existing := range d.Signatures {
		if existing.KeyID == keyID {
			return fmt.Errorf("signature for key %q already exists", keyID)
		}
	}
	d.Signatures = append(d.Signatures, signature)
	return d.Validate()
}

// Verify requires externally supplied product-root keys. A profile never
// trusts a key carried inside itself.
func (d Document) Verify(trustedProductRoots map[string]ed25519.PublicKey) error {
	if err := d.Validate(); err != nil {
		return err
	}
	return trust.VerifyAny(d.signedBody(), d.Signatures, trustedProductRoots)
}
