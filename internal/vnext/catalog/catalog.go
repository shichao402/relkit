// Package catalog owns the signed list of relkit components a server offers.
//
// The Catalog answers exactly one question: which relkit version does this
// server serve, and what are the digests of its files. There is no candidate /
// certified split, no lifecycle state machine, no certification sequence and no
// capability vector any more (requirements 12.6, 14.4): a release either is the
// version the server marks current, or it is a historical entry kept so an
// older host can still fetch what it pinned.
package catalog

import (
	"crypto/ed25519"
	"fmt"

	"github.com/shichao402/relkit/internal/vnext/canonical"
	"github.com/shichao402/relkit/internal/vnext/document"
	"github.com/shichao402/relkit/internal/vnext/trust"
)

const Schema = "relkit.distribution-catalog/1"

// Source says where one component file is fetched from. A server hands out its
// own URLs; nothing points at GitHub or a mirror any more (requirement 12.2).
type Source struct {
	Kind string `json:"kind"`
	URL  string `json:"url,omitempty"`
}

type Component struct {
	Name     string `json:"name"`
	Version  string `json:"version"`
	Platform string `json:"platform"`
	SHA256   string `json:"sha256"`
	Source   Source `json:"source"`
}

// Release is one relkit version and the component files that make it up.
type Release struct {
	Version    string      `json:"version"`
	Commit     string      `json:"commit"`
	Components []Component `json:"components"`
}

type Document struct {
	Schema string `json:"schema"`
	// Sequence increases when the Catalog changes. It is informational: CI
	// keeps no watermark any more, so a replayed older Catalog is accepted by
	// design (requirement 14 M4, accepted cost).
	Sequence int64 `json:"sequence"`
	// Current names the release the server serves right now. Hosts do not
	// choose a relkit version; they take whatever this says (requirement 12.6).
	Current  string            `json:"current"`
	Releases []Release         `json:"releases"`
	Signatures []trust.Signature `json:"signatures"`
}

type unsignedDocument struct {
	Schema   string    `json:"schema"`
	Sequence int64     `json:"sequence"`
	Current  string    `json:"current"`
	Releases []Release `json:"releases"`
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
	if len(d.Releases) == 0 {
		return fmt.Errorf("releases must not be empty")
	}
	seenVersions := map[string]bool{}
	for index, release := range d.Releases {
		if release.Version == "" {
			return fmt.Errorf("releases[%d].version must not be empty", index)
		}
		if seenVersions[release.Version] {
			return fmt.Errorf("duplicate release version %s; an immutable correction requires a new version", release.Version)
		}
		seenVersions[release.Version] = true
		if release.Commit == "" {
			return fmt.Errorf("releases[%d].commit must not be empty", index)
		}
		if len(release.Components) == 0 {
			return fmt.Errorf("releases[%d].components must not be empty", index)
		}
		componentKeys := map[string]bool{}
		for componentIndex, component := range release.Components {
			if component.Name == "" || component.Version == "" || component.Platform == "" {
				return fmt.Errorf("releases[%d].components[%d] is missing name, version, or platform", index, componentIndex)
			}
			if err := canonical.ValidateDigest(component.SHA256); err != nil {
				return fmt.Errorf("releases[%d].components[%d].sha256: %w", index, componentIndex, err)
			}
			if component.Source.Kind != "server" || component.Source.URL == "" {
				return fmt.Errorf("releases[%d].components[%d] must be served by the server: source.kind is %q", index, componentIndex, component.Source.Kind)
			}
			key := component.Name + "\x00" + component.Platform
			if componentKeys[key] {
				return fmt.Errorf("release %s contains duplicate component %s for %s", release.Version, component.Name, component.Platform)
			}
			componentKeys[key] = true
		}
	}
	if d.Current == "" {
		return fmt.Errorf("current must name the release this server serves")
	}
	if !seenVersions[d.Current] {
		return fmt.Errorf("current names %q, which is not one of the releases", d.Current)
	}
	if err := trust.ValidateSignatures(d.Signatures); err != nil {
		return err
	}
	return nil
}

func (d Document) unsigned() unsignedDocument {
	return unsignedDocument{
		Schema:   d.Schema,
		Sequence: d.Sequence,
		Current:  d.Current,
		Releases: d.Releases,
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

// ReleaseByVersion returns one release row.
func (d Document) ReleaseByVersion(version string) (Release, bool) {
	for _, release := range d.Releases {
		if release.Version == version {
			return release, true
		}
	}
	return Release{}, false
}

// CurrentRelease returns the release the server serves right now.
func (d Document) CurrentRelease() (Release, error) {
	release, ok := d.ReleaseByVersion(d.Current)
	if !ok {
		return Release{}, fmt.Errorf("current release %q is missing from the catalog", d.Current)
	}
	return release, nil
}
