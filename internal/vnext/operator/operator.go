// Package operator owns the signed relkit.operator-profile/1 document. A
// profile describes deployment facts: who the server is and where it answers.
// It is not a product preference file and it carries no selection policy or
// capability vector (requirement 14.4).
package operator

import (
	"crypto/ed25519"
	"fmt"
	"net/url"

	"github.com/shichao402/relkit/internal/vnext/canonical"
	"github.com/shichao402/relkit/internal/vnext/document"
	"github.com/shichao402/relkit/internal/vnext/trust"
)

const Schema = "relkit.operator-profile/1"

type ServerRelease struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
}

type Document struct {
	Schema        string            `json:"schema"`
	DeploymentID  string            `json:"deploymentId"`
	ServerRelease ServerRelease     `json:"serverRelease"`
	Endpoints     map[string]string `json:"endpoints,omitempty"`
	ServiceIdentity string          `json:"serviceIdentity,omitempty"`
	// SecretRefs names where a client finds a credential, never the credential
	// itself. After the epoch there is one credential kind: the agent token.
	SecretRefs map[string]string `json:"secretRefs,omitempty"`
	Sequence   int64             `json:"sequence"`
	Signatures []trust.Signature `json:"signatures"`
}

type unsignedDocument struct {
	Schema          string            `json:"schema"`
	DeploymentID    string            `json:"deploymentId"`
	ServerRelease   ServerRelease     `json:"serverRelease"`
	Endpoints       map[string]string `json:"endpoints,omitempty"`
	ServiceIdentity string            `json:"serviceIdentity,omitempty"`
	SecretRefs      map[string]string `json:"secretRefs,omitempty"`
	Sequence        int64             `json:"sequence"`
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
	if d.Sequence < 1 {
		return fmt.Errorf("sequence must be positive")
	}
	for name, endpoint := range d.Endpoints {
		if name == "" || endpoint == "" {
			return fmt.Errorf("endpoints must contain non-empty names and URLs")
		}
		parsed, err := url.Parse(endpoint)
		if err != nil || !parsed.IsAbs() || parsed.Host == "" ||
			(parsed.Scheme != "http" && parsed.Scheme != "https") ||
			parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
			return fmt.Errorf("endpoints[%q] must be an HTTP(S) URL without credentials, query, or fragment", name)
		}
	}
	for name, ref := range d.SecretRefs {
		if name == "" || ref == "" {
			return fmt.Errorf("secretRefs must contain non-empty names and references")
		}
	}
	if err := trust.ValidateSignatures(d.Signatures); err != nil {
		return err
	}
	return nil
}

func (d Document) unsigned() unsignedDocument {
	return unsignedDocument{
		Schema:          d.Schema,
		DeploymentID:    d.DeploymentID,
		ServerRelease:   d.ServerRelease,
		Endpoints:       d.Endpoints,
		ServiceIdentity: d.ServiceIdentity,
		SecretRefs:      d.SecretRefs,
		Sequence:        d.Sequence,
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
