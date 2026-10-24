// Package custody owns the credential manifest: the single place that knows
// where a key lives. Programs never carry key paths or credential environment
// variable names; they carry a keyId and ask this package for the value.
//
// Two effects follow from that inversion:
//
//   - A broken manifest fails fast. A missing entry, an unreadable carrier, or
//     a seed that does not match the registered public key fingerprint stops
//     the caller at startup with the offending keyId and reason.
//   - Keys can move or change carrier (file to environment variable) by editing
//     the manifest only, with no code or configuration change.
//
// The manifest itself never holds secrets: it records a keyId, a kind, a
// purpose, a carrier, and (for asymmetric keys) the fingerprint of the public
// half. Token instances live in the agent management data; the manifest only
// records where that storage lives.
package custody

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	rupv2 "github.com/shichao402/relkit/api/rup/v2"
	"github.com/shichao402/relkit/internal/model"
	"github.com/shichao402/relkit/internal/vnext/trust"
)

// ManifestSchema is the document schema for every manifest copy.
const ManifestSchema = "relkit.custody-manifest/1"

// Key kinds. Each kind narrows what the carrier must contain and which
// validations apply.
const (
	// KindRoot is an Ed25519 root key pair for one agent server. The server
	// signs with it; hosts and clients verify the server with it.
	KindRoot = "root"
	// KindTokenRef points at the storage that holds token hashes on a server,
	// or at the environment variable a host CI injects the token through.
	KindTokenRef = "token"
	// KindMirrorCredential is a mirrors username/password pair, kept only for
	// the CI file-transfer path.
	KindMirrorCredential = "mirror-credential"
)

// Carrier types. A carrier says how the value is fetched; it never holds the
// value itself.
const (
	CarrierFile = "file"
	CarrierEnv  = "env"
)

// Scope names which copy of the manifest a caller is reading. Server manifests
// carry private keys, host CI manifests carry only keyIds and environment
// variable names, and the offline backup carries the private copies.
const (
	ScopeServer  = "server"
	ScopeHostCI  = "host-ci"
	ScopeArchive = "archive"
)

// Manifest is one manifest copy. Every holder of credentials keeps its own,
// in the same format, read by the same code.
type Manifest struct {
	Schema    string `json:"schema"`
	Scope     string `json:"scope"`
	Note      string `json:"note,omitempty"`
	UpdatedAt string `json:"updatedAt,omitempty"`
	Keys      []Entry `json:"keys"`

	// root is the directory relative carriers are resolved against. It is the
	// directory the manifest file itself lives in.
	root string
}

// Entry registers one keyId.
type Entry struct {
	KeyID string `json:"keyId"`
	Kind  string `json:"kind"`
	// Purpose is prose for humans: what this key is for and who uses it.
	Purpose string `json:"purpose,omitempty"`
	// Carrier is "file" or "env".
	Carrier string `json:"carrier"`
	// Path is the carrier: a file path (absolute, or relative to the manifest)
	// or an environment variable name.
	Path string `json:"path"`
	// Fingerprint is the SHA-256 fingerprint of the public half, required for
	// asymmetric kinds. It is the authoritative identity of the key.
	Fingerprint string `json:"fingerprint,omitempty"`
	// PublicPath is where the public half is stored, for kinds that have one.
	PublicPath string `json:"publicPath,omitempty"`
	// Formats tells a reader that the carrier holds a token or credential
	// rather than an Ed25519 document.
	Format string `json:"format,omitempty"`
}

// Error reports a custody failure with the keyId and the reason attached, so
// startup logs name the broken entry instead of a generic "key unavailable".
type Error struct {
	KeyID  string
	Reason string
}

func (e Error) Error() string {
	if e.KeyID == "" {
		return "custody manifest: " + e.Reason
	}
	return fmt.Sprintf("custody key %q: %s", e.KeyID, e.Reason)
}

// Load reads and structurally validates a manifest file. Validation is
// limited to shape and internal consistency; carriers are checked only when a
// key is actually acquired, so a host CI manifest can be loaded on a machine
// that holds none of the secrets.
func Load(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, Error{Reason: fmt.Sprintf("cannot read manifest %s: %v", path, err)}
	}
	manifest, err := Parse(data)
	if err != nil {
		return nil, Error{Reason: fmt.Sprintf("%s: %v", path, err)}
	}
	absolute, absErr := filepath.Abs(path)
	if absErr != nil {
		return nil, Error{Reason: fmt.Sprintf("cannot resolve manifest %s: %v", path, absErr)}
	}
	manifest.root = filepath.Dir(absolute)
	return manifest, nil
}

// Parse decodes manifest bytes and validates the document shape.
func Parse(data []byte) (*Manifest, error) {
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("manifest is not valid JSON: %w", err)
	}
	if err := manifest.Validate(); err != nil {
		return nil, err
	}
	return &manifest, nil
}

// Validate checks the document shape: schema, scope, and one well-formed
// entry per keyId.
func (m *Manifest) Validate() error {
	if m.Schema != ManifestSchema {
		return fmt.Errorf("manifest schema is %q, expected %q", m.Schema, ManifestSchema)
	}
	switch m.Scope {
	case ScopeServer, ScopeHostCI, ScopeArchive:
	case "":
		return fmt.Errorf("manifest scope is required (%s, %s or %s)", ScopeServer, ScopeHostCI, ScopeArchive)
	default:
		return fmt.Errorf("manifest scope %q is not one of %s, %s or %s", m.Scope, ScopeServer, ScopeHostCI, ScopeArchive)
	}
	if len(m.Keys) == 0 {
		return fmt.Errorf("manifest registers no keys")
	}
	seen := map[string]bool{}
	for index, entry := range m.Keys {
		if err := entry.validate(index); err != nil {
			return err
		}
		if seen[entry.KeyID] {
			return fmt.Errorf("manifest registers keyId %q twice", entry.KeyID)
		}
		seen[entry.KeyID] = true
	}
	return nil
}

func (e Entry) validate(index int) error {
	if err := model.CheckIdentifier(e.KeyID, fmt.Sprintf("keys[%d].keyId", index)); err != nil {
		return err
	}
	switch e.Kind {
	case KindRoot, KindTokenRef, KindMirrorCredential:
	default:
		return fmt.Errorf("keys[%d] (%s) has kind %q, expected %s, %s or %s",
			index, e.KeyID, e.Kind, KindRoot, KindTokenRef, KindMirrorCredential)
	}
	switch e.Carrier {
	case CarrierFile, CarrierEnv:
	case "":
		return fmt.Errorf("keys[%d] (%s) has no carrier", index, e.KeyID)
	default:
		return fmt.Errorf("keys[%d] (%s) has carrier %q, expected %s or %s",
			index, e.KeyID, e.Carrier, CarrierFile, CarrierEnv)
	}
	if strings.TrimSpace(e.Path) == "" {
		return fmt.Errorf("keys[%d] (%s) has no path", index, e.KeyID)
	}
	if e.Kind == KindRoot {
		if e.Carrier != CarrierFile {
			return fmt.Errorf("keys[%d] (%s) is a root key and must use the %s carrier", index, e.KeyID, CarrierFile)
		}
		if e.Fingerprint == "" {
			return fmt.Errorf("keys[%d] (%s) is a root key and needs a fingerprint", index, e.KeyID)
		}
	}
	return nil
}

// Entry returns the registration for one keyId.
func (m *Manifest) Entry(keyID string) (Entry, error) {
	for _, entry := range m.Keys {
		if entry.KeyID == keyID {
			return entry, nil
		}
	}
	known := make([]string, 0, len(m.Keys))
	for _, entry := range m.Keys {
		known = append(known, entry.KeyID)
	}
	sort.Strings(known)
	return Entry{}, Error{
		KeyID:  keyID,
		Reason: fmt.Sprintf("is not registered in the manifest (registered: %s)", strings.Join(known, ", ")),
	}
}

// resolve turns an entry's path into an absolute path. Relative paths are
// resolved against the manifest directory, so a manifest copy stays portable
// as a directory rather than depending on the caller's working directory.
func (m *Manifest) resolve(entry Entry) string {
	if entry.Carrier != CarrierFile || filepath.IsAbs(entry.Path) {
		return entry.Path
	}
	return filepath.Join(m.root, entry.Path)
}

// ReadCarrier returns the raw bytes an entry points at, whichever carrier it
// uses. Environment carriers return the value as bytes.
func (m *Manifest) ReadCarrier(keyID string) ([]byte, Entry, error) {
	entry, err := m.Entry(keyID)
	if err != nil {
		return nil, Entry{}, err
	}
	switch entry.Carrier {
	case CarrierEnv:
		value, ok := os.LookupEnv(entry.Path)
		if !ok {
			return nil, entry, Error{
				KeyID:  keyID,
				Reason: fmt.Sprintf("environment variable %s is not set", entry.Path),
			}
		}
		if strings.TrimSpace(value) == "" {
			return nil, entry, Error{
				KeyID:  keyID,
				Reason: fmt.Sprintf("environment variable %s is empty", entry.Path),
			}
		}
		return []byte(value), entry, nil
	default:
		path := m.resolve(entry)
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil, entry, Error{
				KeyID:  keyID,
				Reason: fmt.Sprintf("cannot read %s: %v", path, readErr),
			}
		}
		return data, entry, nil
	}
}

// PrivateSeed reads the root private key registered under keyID and proves the
// seed belongs to the registered public key before handing it back. The
// round-trip is what catches a fingerprint that matches on paper while the
// carrier holds a different key.
func (m *Manifest) PrivateSeed(keyID string, signKeyIDs ...string) ([]byte, error) {
	data, entry, err := m.ReadCarrier(keyID)
	if err != nil {
		return nil, err
	}
	if entry.Kind != KindRoot {
		return nil, Error{KeyID: keyID, Reason: fmt.Sprintf("has kind %q; only a root key yields a private seed", entry.Kind)}
	}
	document, err := rupv2.UnmarshalPrivateKey(data)
	if err != nil {
		return nil, Error{KeyID: keyID, Reason: fmt.Sprintf("%s does not hold a private key document: %v", m.resolve(entry), err)}
	}
	seed, err := loadSeed(*document, keyID, m.resolve(entry))
	if err != nil {
		return nil, err
	}
	if document.KeyId != "" && document.KeyId != keyID {
		return nil, Error{KeyID: keyID, Reason: fmt.Sprintf("key document declares keyId %q", document.KeyId)}
	}
	publicKey := ed25519.NewKeyFromSeed(seed).Public().(ed25519.PublicKey)
	fingerprint, fingerprintErr := trust.Fingerprint(publicKey)
	if fingerprintErr != nil {
		return nil, Error{KeyID: keyID, Reason: fingerprintErr.Error()}
	}
	if entry.Fingerprint != "" && fingerprint != entry.Fingerprint {
		return nil, Error{
			KeyID: keyID,
			Reason: fmt.Sprintf("private key yields public fingerprint %s, manifest registers %s",
				fingerprint, entry.Fingerprint),
		}
	}
	if err := m.verifySignatureRoundTrip(keyID, seed, signKeyIDs); err != nil {
		return nil, err
	}
	return seed, nil
}

// verifySignatureRoundTrip signs a fixed probe with the seed and verifies it
// with the registered public key. Callers can pass the signature keyIds they
// intend to use so the probe also covers the exact identity they will stamp on
// documents.
func (m *Manifest) verifySignatureRoundTrip(keyID string, seed []byte, signKeyIDs []string) error {
	probe := []byte("relkit: custody self-check " + keyID)
	signature := ed25519.Sign(ed25519.NewKeyFromSeed(seed), probe)
	publicKey, err := m.PublicKey(keyID)
	if err != nil {
		return err
	}
	if !ed25519.Verify(publicKey, probe, signature) {
		return Error{KeyID: keyID, Reason: "signature round-trip failed: private key does not match the registered public key"}
	}
	for _, signKeyID := range signKeyIDs {
		if signKeyID != keyID {
			return Error{
				KeyID:  keyID,
				Reason: fmt.Sprintf("is asked to sign documents as %q, but a server signs only as its own root", signKeyID),
			}
		}
	}
	return nil
}

// PublicKey returns the public half for a root key, preferring the registered
// public file and falling back to deriving it from the private key so a server
// only needs its private copy on disk.
func (m *Manifest) PublicKey(keyID string) (ed25519.PublicKey, error) {
	entry, err := m.Entry(keyID)
	if err != nil {
		return nil, err
	}
	if entry.PublicPath != "" {
		path := entry.PublicPath
		if !filepath.IsAbs(path) {
			path = filepath.Join(m.root, path)
		}
		if data, readErr := os.ReadFile(path); readErr == nil {
			if document, unmarshalErr := rupv2.UnmarshalPublicKey(data); unmarshalErr == nil {
				return publicKeyFromDocument(*document, keyID, path)
			}
		}
	}
	data, carrier, err := m.ReadCarrier(keyID)
	if err != nil {
		return nil, err
	}
	document, unmarshalErr := rupv2.UnmarshalPrivateKey(data)
	if unmarshalErr != nil {
		return nil, Error{
			KeyID:  keyID,
			Reason: fmt.Sprintf("has no readable public key file and %s does not hold a private key document", m.resolve(carrier)),
		}
	}
	seed, seedErr := loadSeed(*document, keyID, m.resolve(carrier))
	if seedErr != nil {
		return nil, seedErr
	}
	publicKey := ed25519.NewKeyFromSeed(seed).Public().(ed25519.PublicKey)
	fingerprint, fingerprintErr := trust.Fingerprint(publicKey)
	if fingerprintErr != nil {
		return nil, Error{KeyID: keyID, Reason: fingerprintErr.Error()}
	}
	if entry.Fingerprint != "" && fingerprint != entry.Fingerprint {
		return nil, Error{
			KeyID: keyID,
			Reason: fmt.Sprintf("derived public fingerprint %s does not match manifest registration %s",
				fingerprint, entry.Fingerprint),
		}
	}
	return publicKey, nil
}

// Fingerprint returns the manifest-registered fingerprint for keyID, resolving
// it from the key material when the entry left it blank.
func (m *Manifest) Fingerprint(keyID string) (string, error) {
	entry, err := m.Entry(keyID)
	if err != nil {
		return "", err
	}
	if entry.Fingerprint != "" {
		return entry.Fingerprint, nil
	}
	publicKey, err := m.PublicKey(keyID)
	if err != nil {
		return "", err
	}
	return trust.Fingerprint(publicKey)
}

// Root is the single signing identity a server holds. KeyID is the only thing
// configuration ever names; the seed, the derived public key, and the
// registered fingerprint all come from the manifest.
type Root struct {
	KeyID       string
	Seed        []byte
	PublicKey   ed25519.PublicKey
	Fingerprint string
}

// Root resolves keyID into a usable signing identity. It runs the same
// checks as PrivateSeed (carrier readable, document keyId matches, fingerprint
// matches, signature round-trip) so a server cannot start with a root that
// would produce documents nobody can verify.
func (m *Manifest) Root(keyID string) (*Root, error) {
	seed, err := m.PrivateSeed(keyID)
	if err != nil {
		return nil, err
	}
	publicKey := ed25519.NewKeyFromSeed(seed).Public().(ed25519.PublicKey)
	fingerprint, err := m.Fingerprint(keyID)
	if err != nil {
		return nil, err
	}
	return &Root{
		KeyID:       keyID,
		Seed:        seed,
		PublicKey:   publicKey,
		Fingerprint: fingerprint,
	}, nil
}

// Token returns the token string registered under keyID. This covers the two
// token carriers in the model: a host CI exposes its token through an
// environment variable named by the manifest, and a server reads the token
// from its management storage. The manifest never holds a token instance.
func (m *Manifest) Token(keyID string) (string, error) {
	data, entry, err := m.ReadCarrier(keyID)
	if err != nil {
		return "", err
	}
	if entry.Kind != KindTokenRef {
		return "", Error{KeyID: keyID, Reason: fmt.Sprintf("has kind %q; only a token entry yields a token", entry.Kind)}
	}
	value := strings.TrimSpace(string(data))
	if value == "" {
		return "", Error{KeyID: keyID, Reason: "carrier holds an empty token"}
	}
	return value, nil
}

// SelfCheck validates every keyId the caller needs. It is meant to run at
// startup: any failure stops the process instead of degrading silently. The
// returned report names each entry and what was verified.
func (m *Manifest) SelfCheck(keyIDs ...string) (*Report, error) {
	report := &Report{Scope: m.Scope, Keys: make([]KeyReport, 0, len(keyIDs))}
	for _, keyID := range keyIDs {
		keyReport := KeyReport{KeyID: keyID}
		entry, err := m.Entry(keyID)
		if err != nil {
			report.Keys = append(report.Keys, keyReport)
			return report, err
		}
		keyReport.Kind = entry.Kind
		keyReport.Carrier = entry.Carrier

		switch entry.Kind {
		case KindRoot:
			seed, seedErr := m.PrivateSeed(keyID)
			if seedErr != nil {
				report.Keys = append(report.Keys, keyReport)
				return report, seedErr
			}
			publicKey := ed25519.NewKeyFromSeed(seed).Public().(ed25519.PublicKey)
			fingerprint, fingerprintErr := trust.Fingerprint(publicKey)
			if fingerprintErr != nil {
				report.Keys = append(report.Keys, keyReport)
				return report, Error{KeyID: keyID, Reason: fingerprintErr.Error()}
			}
			keyReport.Fingerprint = fingerprint
			keyReport.Verified = "signature round-trip"
		default:
			if _, _, carrierErr := m.ReadCarrier(keyID); carrierErr != nil {
				report.Keys = append(report.Keys, keyReport)
				return report, carrierErr
			}
			keyReport.Verified = "carrier readable"
		}
		report.Keys = append(report.Keys, keyReport)
	}
	return report, nil
}

// Report is the outcome of a self-check, printable as JSON so scripts can
// consume it without re-parsing the manifest.
type Report struct {
	Scope string      `json:"scope"`
	Keys  []KeyReport `json:"keys"`
}

// KeyReport is one entry's self-check outcome.
type KeyReport struct {
	KeyID       string `json:"keyId"`
	Kind        string `json:"kind"`
	Carrier     string `json:"carrier"`
	Fingerprint string `json:"fingerprint,omitempty"`
	Verified    string `json:"verified"`
}

// OK reports whether every requested key was registered and verified.
func (r *Report) OK() bool {
	return r != nil && len(r.Keys) > 0
}

// Encode renders a report as JSON with a trailing newline.
func (r *Report) Encode() ([]byte, error) {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// ReadString returns the carrier contents as a trimmed string. It is the
// generic accessor used for kinds whose payload is a plain secret (tokens,
// mirror credentials) rather than an Ed25519 document.
func (m *Manifest) ReadString(keyID string) (string, error) {
	data, _, err := m.ReadCarrier(keyID)
	if err != nil {
		return "", err
	}
	value := strings.TrimSpace(string(data))
	if value == "" {
		return "", Error{KeyID: keyID, Reason: "carrier holds an empty value"}
	}
	return value, nil
}

// EncodeSeed renders a private seed as base64, the form the key document
// itself stores. Scripts that need to hand a seed to another relkit command
// use this instead of base64-encoding by hand.
func EncodeSeed(seed []byte) string {
	return base64.StdEncoding.EncodeToString(seed)
}

func loadSeed(document model.PrivateKeyDocument, keyID, source string) ([]byte, error) {
	if document.Schema != model.SchemaPrivateKey {
		return nil, Error{KeyID: keyID, Reason: fmt.Sprintf("%s has schema %q, expected %q", source, document.Schema, model.SchemaPrivateKey)}
	}
	if len(document.Seed) != ed25519.SeedSize {
		return nil, Error{KeyID: keyID, Reason: fmt.Sprintf("%s holds %d seed bytes, expected %d", source, len(document.Seed), ed25519.SeedSize)}
	}
	return append([]byte(nil), document.Seed...), nil
}

func publicKeyFromDocument(document model.PublicKeyDocument, keyID, source string) (ed25519.PublicKey, error) {
	if document.Schema != model.SchemaPublicKey {
		return nil, Error{KeyID: keyID, Reason: fmt.Sprintf("%s has schema %q, expected %q", source, document.Schema, model.SchemaPublicKey)}
	}
	if document.KeyId != "" && document.KeyId != keyID {
		return nil, Error{KeyID: keyID, Reason: fmt.Sprintf("%s declares keyId %q", source, document.KeyId)}
	}
	if len(document.PublicKey) != ed25519.PublicKeySize {
		return nil, Error{KeyID: keyID, Reason: fmt.Sprintf("%s holds %d public key bytes, expected %d", source, len(document.PublicKey), ed25519.PublicKeySize)}
	}
	cloned := make(ed25519.PublicKey, len(document.PublicKey))
	copy(cloned, document.PublicKey)
	return cloned, nil
}

// EncodePublicKey renders a public key as the base64 form hosts paste into
// their project configuration and embed in Runtime Bootstrap.
func EncodePublicKey(publicKey ed25519.PublicKey) string {
	return base64.StdEncoding.EncodeToString(publicKey)
}

// DecodePublicKey parses the base64 public key form used in project
// configuration and Runtime Bootstrap.
func DecodePublicKey(encoded, source string) (ed25519.PublicKey, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil {
		return nil, fmt.Errorf("%s does not contain valid base64", source)
	}
	if len(raw) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("%s decodes to %d bytes, expected %d", source, len(raw), ed25519.PublicKeySize)
	}
	return ed25519.PublicKey(raw), nil
}
