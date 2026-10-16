// Package catalog owns the signed distribution catalog and its lifecycle.
package catalog

import (
	"crypto/ed25519"
	"fmt"
	"slices"

	"github.com/shichao402/relkit/internal/vnext/canonical"
	"github.com/shichao402/relkit/internal/vnext/contracts"
	"github.com/shichao402/relkit/internal/vnext/document"
	"github.com/shichao402/relkit/internal/vnext/trust"
)

const Schema = "relkit.distribution-catalog/1"

type State string

const (
	StateCandidate  State = "candidate"
	StateCertified  State = "certified"
	StateSuperseded State = "superseded"
	StateRevoked    State = "revoked"
)

type Source struct {
	Kind   string `json:"kind"`
	URL    string `json:"url,omitempty"`
	Module string `json:"module,omitempty"`
	H1     string `json:"h1,omitempty"`
	SHA256 string `json:"sha256,omitempty"`
}

type Component struct {
	Name     string                 `json:"name"`
	Version  string                 `json:"version"`
	Platform string                 `json:"platform"`
	SHA256   string                 `json:"sha256"`
	Source   Source                 `json:"source"`
	Requires []contracts.Capability `json:"requires,omitempty"`
	Provides []contracts.Capability `json:"provides,omitempty"`
}

type Distribution struct {
	Version               string                 `json:"version"`
	Commit                string                 `json:"commit"`
	Digest                string                 `json:"digest"`
	ModulePath            string                 `json:"modulePath,omitempty"`
	ModuleH1              string                 `json:"moduleH1,omitempty"`
	ReleaseAssets         map[string]string      `json:"releaseAssets,omitempty"`
	State                 State                  `json:"state"`
	CertificationSequence int64                  `json:"certificationSequence,omitempty"`
	Components            []Component            `json:"components"`
	Requires              []contracts.Capability `json:"requires,omitempty"`
	Provides              []contracts.Capability `json:"provides,omitempty"`
}

type Document struct {
	Schema        string            `json:"schema"`
	Sequence      int64             `json:"sequence"`
	Distributions []Distribution    `json:"distributions"`
	Signatures    []trust.Signature `json:"signatures"`
}

type unsignedDocument struct {
	Schema        string         `json:"schema"`
	Sequence      int64          `json:"sequence"`
	Distributions []Distribution `json:"distributions"`
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
	if d.Sequence < 1 {
		return fmt.Errorf("sequence must be positive")
	}
	if len(d.Distributions) == 0 {
		return fmt.Errorf("distributions must not be empty")
	}
	seen := map[string]bool{}
	seenVersions := map[string]bool{}
	for index, distribution := range d.Distributions {
		if _, err := parseSemVersion(distribution.Version); err != nil {
			return fmt.Errorf("distributions[%d]: %w", index, err)
		}
		if seenVersions[distribution.Version] {
			return fmt.Errorf("duplicate distribution version %s; immutable corrections require a new version", distribution.Version)
		}
		seenVersions[distribution.Version] = true
		if distribution.Commit == "" {
			return fmt.Errorf("distributions[%d].commit must not be empty", index)
		}
		if err := canonical.ValidateDigest(distribution.Digest); err != nil {
			return fmt.Errorf("distributions[%d].digest: %w", index, err)
		}
		if (distribution.ModulePath == "") != (distribution.ModuleH1 == "") {
			return fmt.Errorf("distributions[%d] must declare modulePath and moduleH1 together", index)
		}
		for asset, digest := range distribution.ReleaseAssets {
			if asset == "" {
				return fmt.Errorf("distributions[%d].releaseAssets contains an empty asset name", index)
			}
			if err := canonical.ValidateDigest(digest); err != nil {
				return fmt.Errorf("distributions[%d].releaseAssets[%q]: %w", index, asset, err)
			}
		}
		switch distribution.State {
		case StateCandidate, StateCertified, StateSuperseded, StateRevoked:
		default:
			return fmt.Errorf("distributions[%d] has unknown state %q", index, distribution.State)
		}
		if distribution.State == StateCertified && distribution.CertificationSequence < 1 {
			return fmt.Errorf("distributions[%d] certified state requires certificationSequence", index)
		}
		key := distribution.Version + "\x00" + distribution.Digest
		if seen[key] {
			return fmt.Errorf("duplicate distribution %s (%s)", distribution.Version, distribution.Digest)
		}
		seen[key] = true
		if len(distribution.Components) == 0 {
			return fmt.Errorf("distributions[%d].components must not be empty", index)
		}
		componentKeys := map[string]bool{}
		for componentIndex, component := range distribution.Components {
			if component.Name == "" || component.Version == "" || component.Platform == "" {
				return fmt.Errorf("distributions[%d].components[%d] is missing name, version, or platform", index, componentIndex)
			}
			if err := canonical.ValidateDigest(component.SHA256); err != nil {
				return fmt.Errorf("distributions[%d].components[%d].sha256: %w", index, componentIndex, err)
			}
			switch component.Source.Kind {
			case "release-asset":
				if component.Source.URL == "" || component.Source.Module != "" {
					return fmt.Errorf("distributions[%d].components[%d] release-asset source requires url only", index, componentIndex)
				}
			case "module":
				if component.Source.Module == "" || component.Source.URL != "" {
					return fmt.Errorf("distributions[%d].components[%d] module source requires module only", index, componentIndex)
				}
			default:
				return fmt.Errorf("distributions[%d].components[%d] has unknown source kind %q", index, componentIndex, component.Source.Kind)
			}
			componentKey := component.Name + "\x00" + component.Platform
			if componentKeys[componentKey] {
				return fmt.Errorf("distribution %s contains duplicate component %s for %s", distribution.Version, component.Name, component.Platform)
			}
			componentKeys[componentKey] = true
			if err := (contracts.Vector{Requires: component.Requires, Provides: component.Provides}).Validate(); err != nil {
				return fmt.Errorf("distribution %s component %s: %w", distribution.Version, component.Name, err)
			}
		}
		if err := (contracts.Vector{Requires: distribution.Requires, Provides: distribution.Provides}).Validate(); err != nil {
			return fmt.Errorf("distribution %s: %w", distribution.Version, err)
		}
	}
	if err := trust.ValidateSignatures(d.Signatures); err != nil {
		return err
	}
	return nil
}

func (d Document) unsigned() unsignedDocument {
	return unsignedDocument{
		Schema:        d.Schema,
		Sequence:      d.Sequence,
		Distributions: d.Distributions,
	}
}

func (d *Document) Sign(keyID string, seed []byte) error {
	if d == nil {
		return fmt.Errorf("nil distribution catalog")
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
		return fmt.Errorf("nil distribution catalog")
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

// Production returns certified, non-prerelease distributions ordered by
// descending SemVer precedence. Certification sequence never changes version
// ordering; it exists for anti-rollback and audit only.
func (d Document) Production() ([]Distribution, error) {
	var result []Distribution
	for _, distribution := range d.Distributions {
		if distribution.State != StateCertified {
			continue
		}
		prerelease, err := isPrerelease(distribution.Version)
		if err != nil {
			return nil, err
		}
		if !prerelease {
			result = append(result, distribution)
		}
	}
	slices.SortStableFunc(result, func(left, right Distribution) int {
		comparison, _ := CompareVersions(left.Version, right.Version)
		return -comparison
	})
	return result, nil
}

// ValidateTransition enforces the catalog lifecycle for one immutable
// distribution identity.
func ValidateTransition(from, to State) error {
	if from == to {
		return nil
	}
	switch from {
	case StateCandidate:
		if to == StateCertified || to == StateRevoked {
			return nil
		}
	case StateCertified:
		if to == StateSuperseded || to == StateRevoked {
			return nil
		}
	case StateSuperseded:
		if to == StateRevoked {
			return nil
		}
	case StateRevoked:
	}
	return fmt.Errorf("invalid distribution state transition %s -> %s", from, to)
}
