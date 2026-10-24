// Package project owns relkit.project/1, the only control-plane declaration a
// product repository commits.
//
// A host project holds four things (requirement 12.1): the server it talks to,
// that server's root public key, the product name, and a token that lives in CI
// secrets. Nothing else is versioned here: no resolution, no component digests,
// no pinned relkit version, no capability vector, no product signing key. The
// server is the single source of truth for which relkit version is current.
package project

import (
	"crypto/ed25519"
	"fmt"
	"net/url"

	"github.com/shichao402/relkit/internal/model"
	"github.com/shichao402/relkit/internal/vnext/canonical"
	"github.com/shichao402/relkit/internal/vnext/document"
	"github.com/shichao402/relkit/internal/vnext/trust"
)

const Schema = "relkit.project/1"

type Command struct {
	Command         []string `json:"command"`
	ExtensionSchema string   `json:"extensionSchema,omitempty"`
	TimeoutSeconds  int      `json:"timeoutSeconds,omitempty"`
	EnvAllowlist    []string `json:"envAllowlist,omitempty"`
	Permissions     []string `json:"permissions,omitempty"`
}

// Site is the optional human-facing site declaration. Omitting it is a
// deliberate opt-out: the product publishes protocol documents only and never
// appears in the rendered catalog. Declaring it makes every publish write the
// site/<product>.json document the site rebuild reads.
type Site struct {
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	Homepage    string `json:"homepage,omitempty"`
}

// IsZero reports whether the declaration carries no displayable field, which
// the publisher treats as an absent site document.
func (s Site) IsZero() bool {
	return s.Title == "" && s.Description == "" && s.Homepage == ""
}

type Document struct {
	Schema string `json:"schema"`
	// Product is the name this project publishes as.
	Product string `json:"product"`
	// Server is the agent server this project talks to. It is also the only
	// place relkit components and control-plane documents come from
	// (requirement 12.1).
	Server string `json:"server"`
	// RootPublicKey is that server's root public key, base64 std encoding of a
	// 32-byte ed25519 key. It is written verbatim from the access configuration
	// the server handed out at product registration (requirements 10.7, 13.4).
	RootPublicKey string `json:"rootPublicKey"`
	// Channels lists the channels this project publishes to. It is a publish
	// fact, not a control-plane choice: the server still decides which version
	// is current.
	Channels           []string `json:"channels,omitempty"`
	SelectorDimensions []string `json:"selectorDimensions,omitempty"`
	ArtifactProducer   *Command `json:"artifactProducer,omitempty"`
	VersionProvider    *Command `json:"versionProvider,omitempty"`
	Site               *Site    `json:"site,omitempty"`
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
	if err := model.CheckIdentifier(d.Product, "product"); err != nil {
		return err
	}
	if _, err := d.ServerURL(); err != nil {
		return err
	}
	if _, err := d.RootKey(); err != nil {
		return err
	}
	for name, values := range map[string][]string{
		"channels":           d.Channels,
		"selectorDimensions": d.SelectorDimensions,
	} {
		seen := map[string]bool{}
		for _, value := range values {
			if value == "" || seen[value] {
				return fmt.Errorf("%s contains an empty or duplicate value", name)
			}
			seen[value] = true
		}
	}
	for name, command := range map[string]*Command{
		"artifactProducer": d.ArtifactProducer,
		"versionProvider":  d.VersionProvider,
	} {
		if command == nil {
			continue
		}
		if len(command.Command) == 0 {
			return fmt.Errorf("%s.command must not be empty", name)
		}
		if command.ExtensionSchema != "" && command.ExtensionSchema != "relkit.extension/1" {
			return fmt.Errorf("%s.extensionSchema is %q, expected relkit.extension/1", name, command.ExtensionSchema)
		}
		if command.TimeoutSeconds < 0 {
			return fmt.Errorf("%s.timeoutSeconds must not be negative", name)
		}
		seenEnvironment := map[string]bool{}
		for _, variable := range command.EnvAllowlist {
			if variable == "" || seenEnvironment[variable] {
				return fmt.Errorf("%s.envAllowlist contains an empty or duplicate variable", name)
			}
			seenEnvironment[variable] = true
		}
	}
	// A declared site block must say something: an all-empty block is a typo,
	// not an opt-out. Omitting the block entirely is the opt-out.
	if d.Site != nil && d.Site.IsZero() {
		return fmt.Errorf("site needs at least one of title / description / homepage")
	}
	return nil
}

// ServerURL validates and returns the agent server base URL.
func (d Document) ServerURL() (string, error) {
	parsed, err := url.Parse(d.Server)
	if err != nil || !parsed.IsAbs() || parsed.Host == "" {
		return "", fmt.Errorf("server must be an absolute HTTP(S) URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("server must use http or https")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("server must not contain credentials, query, or fragment")
	}
	return d.Server, nil
}

// RootKey decodes the server root public key this project trusts. There is
// exactly one: an application only ever trusts the server it registered with
// (requirements 10.6, 10.7).
//
// The accepted forms are the OpenSSH line the server publishes at
// /v1/tools/root.pub and the bare base64 key. The line form is the one a host
// copies verbatim, and it is also what CI writes into allowed_signers, so a
// project never has to convert the key to use it.
func (d Document) RootKey() (ed25519.PublicKey, error) {
	publicKey, err := trust.ParsePublicKey(d.RootPublicKey)
	if err != nil {
		return nil, fmt.Errorf("rootPublicKey: %w", err)
	}
	return publicKey, nil
}

func (d Document) Digest() (string, error) {
	if err := d.Validate(); err != nil {
		return "", err
	}
	return canonical.Digest(d)
}
