// Package operator owns the signed relkit.operator-profile/1 document. A
// profile describes deployment facts; it is not a product preference file.
package operator

import (
	"crypto/ed25519"
	"fmt"
	"net/url"

	"github.com/shichao402/relkit/internal/vnext/canonical"
	"github.com/shichao402/relkit/internal/vnext/contracts"
	"github.com/shichao402/relkit/internal/vnext/document"
	"github.com/shichao402/relkit/internal/vnext/trust"
)

const Schema = "relkit.operator-profile/1"

type SelectionPolicy string

const (
	SelectionExact            SelectionPolicy = "exact"
	SelectionLatestCompatible SelectionPolicy = "latest-compatible"
)

type ServerRelease struct {
	Version            string `json:"version"`
	Commit             string `json:"commit"`
	DistributionDigest string `json:"distributionDigest"`
}

type Sources struct {
	Mirrors []string `json:"mirrors,omitempty"`
	Bundles []string `json:"bundles,omitempty"`
}

type Document struct {
	Schema             string                 `json:"schema"`
	DeploymentID       string                 `json:"deploymentId"`
	ServerRelease      ServerRelease          `json:"serverRelease"`
	SelectionPolicy    SelectionPolicy        `json:"selectionPolicy"`
	Provides           []contracts.Capability `json:"provides"`
	PublishTransaction []string               `json:"publishTransaction,omitempty"`
	Endpoints          map[string]string      `json:"endpoints,omitempty"`
	Backends           []string               `json:"backends,omitempty"`
	ServiceIdentity    string                 `json:"serviceIdentity,omitempty"`
	SecretRefs         map[string]string      `json:"secretRefs,omitempty"`
	Sources            Sources                `json:"sources,omitempty"`
	InventoryRef       string                 `json:"inventoryRef,omitempty"`
	CIAdapterRef       string                 `json:"ciAdapterRef,omitempty"`
	Sequence           int64                  `json:"sequence"`
	Signatures         []trust.Signature      `json:"signatures"`
}

type unsignedDocument struct {
	Schema             string                 `json:"schema"`
	DeploymentID       string                 `json:"deploymentId"`
	ServerRelease      ServerRelease          `json:"serverRelease"`
	SelectionPolicy    SelectionPolicy        `json:"selectionPolicy"`
	Provides           []contracts.Capability `json:"provides"`
	PublishTransaction []string               `json:"publishTransaction,omitempty"`
	Endpoints          map[string]string      `json:"endpoints,omitempty"`
	Backends           []string               `json:"backends,omitempty"`
	ServiceIdentity    string                 `json:"serviceIdentity,omitempty"`
	SecretRefs         map[string]string      `json:"secretRefs,omitempty"`
	Sources            Sources                `json:"sources,omitempty"`
	InventoryRef       string                 `json:"inventoryRef,omitempty"`
	CIAdapterRef       string                 `json:"ciAdapterRef,omitempty"`
	Sequence           int64                  `json:"sequence"`
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

func (d Document) Validate() error {
	if d.Schema != Schema {
		return fmt.Errorf("schema is %q, expected %q", d.Schema, Schema)
	}
	if d.DeploymentID == "" {
		return fmt.Errorf("deploymentId must not be empty")
	}
	if d.ServerRelease.Version == "" || d.ServerRelease.Commit == "" {
		return fmt.Errorf("serverRelease version and commit must not be empty")
	}
	if err := canonical.ValidateDigest(d.ServerRelease.DistributionDigest); err != nil {
		return fmt.Errorf("serverRelease.distributionDigest: %w", err)
	}
	switch d.SelectionPolicy {
	case SelectionExact, SelectionLatestCompatible:
	default:
		return fmt.Errorf("unknown selectionPolicy %q", d.SelectionPolicy)
	}
	if d.Sequence < 1 {
		return fmt.Errorf("sequence must be positive")
	}
	if err := (contracts.Vector{Provides: d.Provides}).Validate(); err != nil {
		return fmt.Errorf("provides: %w", err)
	}
	for name, ref := range d.SecretRefs {
		if name == "" || ref == "" {
			return fmt.Errorf("secretRefs must contain non-empty names and references")
		}
	}
	if err := validateMirrorSources(d.Sources.Mirrors); err != nil {
		return err
	}
	if err := validateSources("sources.bundles", d.Sources.Bundles); err != nil {
		return err
	}
	if err := trust.ValidateSignatures(d.Signatures); err != nil {
		return err
	}
	return nil
}

func validateMirrorSources(values []string) error {
	if err := validateSources("sources.mirrors", values); err != nil {
		return err
	}
	for _, value := range values {
		parsed, err := url.Parse(value)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") ||
			parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
			return fmt.Errorf("sources.mirrors entry must be an HTTP(S) URL without credentials, query, or fragment")
		}
	}
	return nil
}

func validateSources(field string, values []string) error {
	seen := map[string]bool{}
	for _, value := range values {
		if value == "" || seen[value] {
			return fmt.Errorf("%s contains an empty or duplicate entry", field)
		}
		seen[value] = true
	}
	return nil
}

func (d Document) unsigned() unsignedDocument {
	return unsignedDocument{
		Schema:             d.Schema,
		DeploymentID:       d.DeploymentID,
		ServerRelease:      d.ServerRelease,
		SelectionPolicy:    d.SelectionPolicy,
		Provides:           d.Provides,
		PublishTransaction: d.PublishTransaction,
		Endpoints:          d.Endpoints,
		Backends:           d.Backends,
		ServiceIdentity:    d.ServiceIdentity,
		SecretRefs:         d.SecretRefs,
		Sources:            d.Sources,
		InventoryRef:       d.InventoryRef,
		CIAdapterRef:       d.CIAdapterRef,
		Sequence:           d.Sequence,
	}
}

func (d *Document) Sign(keyID string, seed []byte) error {
	if d == nil {
		return fmt.Errorf("nil operator profile")
	}
	signature, err := trust.Sign(d.unsigned(), keyID, seed)
	if err != nil {
		return err
	}
	d.Signatures = []trust.Signature{signature}
	return d.Validate()
}

func (d *Document) AddSignature(keyID string, seed []byte) error {
	if d == nil {
		return fmt.Errorf("nil operator profile")
	}
	signature, err := trust.Sign(d.unsigned(), keyID, seed)
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

func (d Document) Verify(trusted map[string]ed25519.PublicKey) error {
	if err := d.Validate(); err != nil {
		return err
	}
	return trust.VerifyAny(d.unsigned(), d.Signatures, trusted)
}

func (d Document) Digest() (string, error) {
	if err := d.Validate(); err != nil {
		return "", err
	}
	return canonical.Digest(d)
}
