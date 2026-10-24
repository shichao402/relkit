// Package tools owns the relkit tool surface a server hands to host CI.
//
// The epoch upgrade makes the server the single source of truth for relkit
// tooling (requirement 12): a host project keeps only the server address, the
// root public key, a product name and a CI token, and asks the server for the
// tools on every build. The server signs each tool with its one root key and
// also emits an OpenSSH SSHSIG signature, so host CI can verify with the
// ssh-keygen that ships with Git for Windows and macOS instead of running a
// relkit-provided verifier (requirement 12.3).
package tools

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/shichao402/relkit/internal/vnext/canonical"
	"github.com/shichao402/relkit/internal/vnext/catalog"
	"github.com/shichao402/relkit/internal/vnext/document"
	"github.com/shichao402/relkit/internal/vnext/trust"
)

const (
	// Schema names the tool manifest document.
	Schema = "relkit.tools/1"
	// SignatureNamespace is the SSHSIG namespace every tool signature uses.
	// `ssh-keygen -Y verify -n relkit-tool` must name it or verification is
	// rejected, which keeps a signature from one purpose from being replayed
	// as another.
	SignatureNamespace = "relkit-tool"
	// AllowlistName is the authorized_keys-style file host CI writes from the
	// project's root public key. It is the `-f` argument of ssh-keygen.
	AllowlistName = "relkit_allowed_signers"
)

// Tool is one downloadable component of one relkit version.
type Tool struct {
	// Component names the relkit component this file is (cli, updater, ops,
	// sdk-dart, ...). The Catalog is aggregated from these names, so a host
	// never has to parse a file name to learn what it downloaded.
	Component string `json:"component"`
	Name     string `json:"name"`
	Version  string `json:"version"`
	Platform string `json:"platform"`
	OS       string `json:"os"`
	Arch     string `json:"arch"`
	Size     int64  `json:"size"`
	SHA256   string `json:"sha256"`
	// URL and SignatureURL are absolute: a host CI downloads both without
	// guessing a path layout, and nothing points at GitHub or a mirror
	// (requirement 12.2).
	URL          string `json:"url"`
	SignatureURL string `json:"signatureUrl"`
}

// Document is the signed list of tools one server serves.
type Document struct {
	Schema  string `json:"schema"`
	Version string `json:"version"`
	Commit  string `json:"commit"`
	// RootKeyID names the root that signed this manifest, so a host can look
	// the matching public key up instead of trying every pinned key.
	RootKeyID string `json:"rootKeyId"`
	// RootPublicKey is the base64 Ed25519 key itself: the host project holds
	// the key, not a fingerprint (requirement 12.1).
	RootPublicKey string `json:"rootPublicKey"`
	// RootFingerprint is informational; verification always uses the key.
	RootFingerprint string `json:"rootFingerprint"`
	Tools           []Tool `json:"tools"`
	Signatures      []trust.Signature `json:"signatures"`
}

type unsignedDocument struct {
	Schema          string `json:"schema"`
	Version         string `json:"version"`
	Commit          string `json:"commit"`
	RootKeyID       string `json:"rootKeyId"`
	RootPublicKey   string `json:"rootPublicKey"`
	RootFingerprint string `json:"rootFingerprint"`
	Tools           []Tool `json:"tools"`
}

// Load reads a tool manifest from disk.
func Load(filePath string) (*Document, error) {
	var value Document
	if err := document.Load(filePath, &value); err != nil {
		return nil, err
	}
	if err := value.Validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", filePath, err)
	}
	return &value, nil
}

// Validate enforces the manifest shape without verifying the signature.
func (d Document) Validate() error {
	if d.Schema != Schema {
		return fmt.Errorf("schema is %q, expected %q", d.Schema, Schema)
	}
	if d.Version == "" {
		return fmt.Errorf("version must not be empty")
	}
	if d.RootKeyID == "" {
		return fmt.Errorf("rootKeyId must not be empty")
	}
	if _, err := decodePublicKey(d.RootPublicKey); err != nil {
		return err
	}
	if len(d.Tools) == 0 {
		return fmt.Errorf("tools must not be empty")
	}
	seen := map[string]bool{}
	for index, tool := range d.Tools {
		if tool.Component == "" || tool.Name == "" || tool.Version == "" || tool.Platform == "" {
			return fmt.Errorf("tools[%d] is missing component, name, version, or platform", index)
		}
		if tool.Version != d.Version {
			return fmt.Errorf("tools[%d] %q is version %q, manifest is %q", index, tool.Name, tool.Version, d.Version)
		}
		if err := canonical.ValidateDigest(tool.SHA256); err != nil {
			return fmt.Errorf("tools[%d].sha256: %w", index, err)
		}
		if tool.Size <= 0 {
			return fmt.Errorf("tools[%d] %q declares size %d", index, tool.Name, tool.Size)
		}
		if !strings.HasPrefix(tool.URL, "http://") && !strings.HasPrefix(tool.URL, "https://") {
			return fmt.Errorf("tools[%d] url must be absolute, got %q", index, tool.URL)
		}
		if tool.SignatureURL == "" {
			return fmt.Errorf("tools[%d] %q declares no signature url", index, tool.Name)
		}
		key := tool.Name + "\x00" + tool.Platform
		if seen[key] {
			return fmt.Errorf("duplicate tool %s for %s", tool.Name, tool.Platform)
		}
		seen[key] = true
	}
	if err := trust.ValidateSignatures(d.Signatures); err != nil {
		return err
	}
	return nil
}

func (d Document) unsigned() unsignedDocument {
	return unsignedDocument{
		Schema:          d.Schema,
		Version:         d.Version,
		Commit:          d.Commit,
		RootKeyID:       d.RootKeyID,
		RootPublicKey:   d.RootPublicKey,
		RootFingerprint: d.RootFingerprint,
		Tools:           d.Tools,
	}
}

// Sign attaches the server root signature and validates the result.
func (d *Document) Sign(keyID string, seed []byte) error {
	if d == nil {
		return fmt.Errorf("nil tool manifest")
	}
	signature, err := trust.Sign(d.unsigned(), keyID, seed)
	if err != nil {
		return err
	}
	d.Signatures = []trust.Signature{signature}
	return d.Validate()
}

// Verify checks the manifest signature against the caller's pinned roots.
func (d Document) Verify(trusted map[string]ed25519.PublicKey) error {
	if err := d.Validate(); err != nil {
		return err
	}
	return trust.VerifyAny(d.unsigned(), d.Signatures, trusted)
}

// PublicKey returns the manifest's declared root public key.
func (d Document) PublicKey() (ed25519.PublicKey, error) {
	return decodePublicKey(d.RootPublicKey)
}

// Tool returns the row for one name on one platform.
func (d Document) Tool(name, platform string) (Tool, bool) {
	for _, tool := range d.Tools {
		if tool.Name == name && tool.Platform == platform {
			return tool, true
		}
	}
	return Tool{}, false
}

// Component returns the row for one component on one platform. Host CI asks
// "give me the updater for windows-amd64" and never has to know the file name.
func (d Document) Component(name, platform string) (Tool, bool) {
	for _, tool := range d.Tools {
		if tool.Component == name && tool.Platform == platform {
			return tool, true
		}
	}
	return Tool{}, false
}

// CatalogRelease projects this manifest into the Catalog release row the
// server marks current, so tools and Catalog cannot drift: both are rendered
// from the same signed upload (requirement 12.6).
func (d Document) CatalogRelease() catalog.Release {
	release := catalog.Release{Version: d.Version, Commit: d.Commit}
	for _, tool := range d.SortedTools() {
		release.Components = append(release.Components, catalog.Component{
			Name: tool.Component, Version: tool.Version, Platform: tool.Platform,
			SHA256: tool.SHA256,
			Source: catalog.Source{Kind: "server", URL: tool.URL},
		})
	}
	return release
}

// SortedTools returns the tool rows in a stable order.
func (d Document) SortedTools() []Tool {
	out := append([]Tool(nil), d.Tools...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Platform != out[j].Platform {
			return out[i].Platform < out[j].Platform
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func decodePublicKey(encoded string) (ed25519.PublicKey, error) {
	return trust.ParsePublicKey(encoded)
}

// EncodePublicKey renders an Ed25519 key as the OpenSSH public key line host
// projects store as rootPublicKey.
//
// One textual form is deliberate: the host pastes this into its project, and
// CI copies it into allowed_signers. If the project held one encoding and
// allowed_signers needed another, every host would have to convert a binary
// key in shell, which is the step that silently produces a line ssh-keygen
// rejects.
func EncodePublicKey(publicKey ed25519.PublicKey) (string, error) {
	if len(publicKey) != ed25519.PublicKeySize {
		return "", fmt.Errorf("public key has %d bytes, expected %d", len(publicKey), ed25519.PublicKeySize)
	}
	return trust.SSHPublicKeyLine(publicKey), nil
}

// Digest is the manifest digest used for change detection.
func (d Document) Digest() (string, error) {
	if err := d.Validate(); err != nil {
		return "", err
	}
	return canonical.Digest(d)
}

// FileSHA256 renders the canonical sha256: prefix form of a file digest.
func FileSHA256(sum [32]byte) string {
	return canonical.DigestPrefix + hex.EncodeToString(sum[:])
}

// Sum256 is the canonical digest of one tool file.
func Sum256(data []byte) string {
	sum := sha256.Sum256(data)
	return FileSHA256(sum)
}

// --- SSHSIG ---------------------------------------------------------------

const sshsigMagic = "SSHSIG"

func writeSSHString(buffer *bytes.Buffer, value []byte) {
	var length [4]byte
	binary.BigEndian.PutUint32(length[:], uint32(len(value)))
	buffer.Write(length[:])
	buffer.Write(value)
}

// sshsigSignedData is the byte string an SSHSIG signature covers: the magic,
// the namespace, an empty reserved field, the hash algorithm and the message
// digest. Anything signed for another namespace therefore cannot be replayed.
func sshsigSignedData(namespace string, message []byte) []byte {
	var buffer bytes.Buffer
	buffer.WriteString(sshsigMagic)
	writeSSHString(&buffer, []byte(namespace))
	writeSSHString(&buffer, nil)
	writeSSHString(&buffer, []byte("sha512"))
	digest := sha512.Sum512(message)
	writeSSHString(&buffer, digest[:])
	return buffer.Bytes()
}

// sshWirePublicKey renders an Ed25519 key in the OpenSSH wire format
// ("ssh-ed25519" + key blob), which is what an authorized_keys line and an
// SSHSIG envelope carry.
func sshWirePublicKey(publicKey ed25519.PublicKey) []byte {
	var buffer bytes.Buffer
	writeSSHString(&buffer, []byte("ssh-ed25519"))
	writeSSHString(&buffer, publicKey)
	return buffer.Bytes()
}

// sshWireSignature renders an Ed25519 signature in the OpenSSH wire format.
func sshWireSignature(raw []byte) []byte {
	var buffer bytes.Buffer
	writeSSHString(&buffer, []byte("ssh-ed25519"))
	writeSSHString(&buffer, raw)
	return buffer.Bytes()
}

// SSHSign produces the SSHSIG blob that `ssh-keygen -Y verify` accepts, for
// one tool payload. The namespace is fixed to SignatureNamespace.
func SSHSign(seed, message []byte) ([]byte, error) {
	if len(seed) != ed25519.SeedSize {
		return nil, fmt.Errorf("seed has %d bytes, expected %d", len(seed), ed25519.SeedSize)
	}
	privateKey := ed25519.NewKeyFromSeed(seed)
	publicKey := privateKey.Public().(ed25519.PublicKey)
	signature := ed25519.Sign(privateKey, sshsigSignedData(SignatureNamespace, message))

	var envelope bytes.Buffer
	envelope.WriteString(sshsigMagic)
	var version [4]byte
	binary.BigEndian.PutUint32(version[:], 1)
	envelope.Write(version[:])
	writeSSHString(&envelope, sshWirePublicKey(publicKey))
	writeSSHString(&envelope, []byte(SignatureNamespace))
	writeSSHString(&envelope, nil)
	writeSSHString(&envelope, []byte("sha512"))
	writeSSHString(&envelope, sshWireSignature(signature))
	return envelope.Bytes(), nil
}

// Armor wraps an SSHSIG blob in the PEM-like framing ssh-keygen writes.
func Armor(blob []byte) string {
	const begin = "-----BEGIN SSH SIGNATURE-----"
	const end = "-----END SSH SIGNATURE-----"
	encoded := base64.StdEncoding.EncodeToString(blob)
	var builder strings.Builder
	builder.WriteString(begin)
	builder.WriteString("\n")
	for len(encoded) > 76 {
		builder.WriteString(encoded[:76])
		builder.WriteString("\n")
		encoded = encoded[76:]
	}
	builder.WriteString(encoded)
	builder.WriteString("\n")
	builder.WriteString(end)
	builder.WriteString("\n")
	return builder.String()
}

// Unarmor strips the SSHSIG framing.
func Unarmor(armored string) ([]byte, error) {
	var payload strings.Builder
	for _, line := range strings.Split(armored, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "-----") {
			continue
		}
		payload.WriteString(line)
	}
	if payload.Len() == 0 {
		return nil, fmt.Errorf("no SSH signature payload found")
	}
	return base64.StdEncoding.DecodeString(payload.String())
}

// VerifySSHSignature checks one armored SSHSIG blob against a public key.
// It mirrors what ssh-keygen -Y verify does, so a broken signature is caught
// in Go tests as well as in CI.
func VerifySSHSignature(armored string, publicKey ed25519.PublicKey, message []byte) error {
	blob, err := Unarmor(armored)
	if err != nil {
		return err
	}
	reader := bytes.NewReader(blob)
	magic := make([]byte, len(sshsigMagic))
	if _, err := reader.Read(magic); err != nil || string(magic) != sshsigMagic {
		return fmt.Errorf("not an SSH signature: bad magic")
	}
	var version [4]byte
	if _, err := reader.Read(version[:]); err != nil {
		return fmt.Errorf("truncated SSH signature")
	}
	if binary.BigEndian.Uint32(version[:]) != 1 {
		return fmt.Errorf("unsupported SSH signature version %d", binary.BigEndian.Uint32(version[:]))
	}
	wireKey, err := readSSHString(reader)
	if err != nil {
		return err
	}
	if !bytes.Equal(wireKey, sshWirePublicKey(publicKey)) {
		return fmt.Errorf("SSH signature was made by a different key")
	}
	namespace, err := readSSHString(reader)
	if err != nil {
		return err
	}
	if string(namespace) != SignatureNamespace {
		return fmt.Errorf("SSH signature namespace is %q, expected %q", string(namespace), SignatureNamespace)
	}
	if _, err := readSSHString(reader); err != nil {
		return err
	}
	algorithm, err := readSSHString(reader)
	if err != nil {
		return err
	}
	if string(algorithm) != "sha512" {
		return fmt.Errorf("SSH signature hash is %q, expected sha512", string(algorithm))
	}
	wireSignature, err := readSSHString(reader)
	if err != nil {
		return err
	}
	signatureReader := bytes.NewReader(wireSignature)
	format, err := readSSHString(signatureReader)
	if err != nil {
		return err
	}
	if string(format) != "ssh-ed25519" {
		return fmt.Errorf("SSH signature format is %q, expected ssh-ed25519", string(format))
	}
	raw, err := readSSHString(signatureReader)
	if err != nil {
		return err
	}
	if !ed25519.Verify(publicKey, sshsigSignedData(SignatureNamespace, message), raw) {
		return fmt.Errorf("SSH signature verification failed")
	}
	return nil
}

func readSSHString(reader *bytes.Reader) ([]byte, error) {
	var length [4]byte
	if _, err := reader.Read(length[:]); err != nil {
		return nil, fmt.Errorf("truncated SSH signature")
	}
	size := binary.BigEndian.Uint32(length[:])
	if uint32(reader.Len()) < size {
		return nil, fmt.Errorf("truncated SSH signature")
	}
	value := make([]byte, size)
	if _, err := reader.Read(value); err != nil {
		return nil, err
	}
	return value, nil
}

// AllowedSignerLine renders one line of the allowed_signers file host CI
// writes from the project's root public key.
//
// The format is the allowed_signers format, not authorized_keys: the line
// starts with the principal, then the key. Putting the keyId last as a comment
// makes ssh-keygen read the key type as a principal and reject every signature
// with "Could not verify signature."
//
// The renderer is shared with EncodePublicKey so the project's rootPublicKey
// and this line can never disagree about the key's textual form.
func AllowedSignerLine(principal string, publicKey ed25519.PublicKey) string {
	if principal == "" {
		principal = "relkit-root"
	}
	return principal + " " + trust.SSHPublicKeyLine(publicKey)
}

// ToolURL composes the download URL for one tool file under a base URL.
func ToolURL(baseURL, version, fileName string) string {
	return strings.TrimRight(baseURL, "/") + "/v1/tools/" + path.Join(version, fileName)
}
