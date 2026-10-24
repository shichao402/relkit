// Package trust implements signatures for vNext control-plane JSON
// documents. RUP protobuf envelopes remain owned by internal/envelope.
package trust

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"strings"

	"github.com/shichao402/relkit/internal/vnext/canonical"
)

type Signature struct {
	KeyID     string `json:"keyId"`
	Algorithm string `json:"algorithm"`
	Value     string `json:"value"`
}

func ValidateSignatures(signatures []Signature) error {
	if len(signatures) == 0 {
		return fmt.Errorf("signatures must not be empty")
	}
	seen := map[string]bool{}
	for _, signature := range signatures {
		if signature.KeyID == "" {
			return fmt.Errorf("signature keyId must not be empty")
		}
		if seen[signature.KeyID] {
			return fmt.Errorf("duplicate signature keyId %q", signature.KeyID)
		}
		seen[signature.KeyID] = true
		if signature.Algorithm != "ed25519" {
			return fmt.Errorf("unsupported signature algorithm %q", signature.Algorithm)
		}
		raw, err := base64.StdEncoding.DecodeString(signature.Value)
		if err != nil || len(raw) != ed25519.SignatureSize {
			return fmt.Errorf("signature value is not a valid Ed25519 signature")
		}
	}
	return nil
}

func Fingerprint(publicKey ed25519.PublicKey) (string, error) {
	if len(publicKey) != ed25519.PublicKeySize {
		return "", fmt.Errorf("public key has %d bytes, expected %d", len(publicKey), ed25519.PublicKeySize)
	}
	sum := sha256.Sum256(publicKey)
	return canonical.DigestPrefix + hex.EncodeToString(sum[:]), nil
}

// sshWirePublicKey renders an Ed25519 key in the OpenSSH wire format
// ("ssh-ed25519" + key blob). This is the payload of an authorized_keys line,
// an allowed_signers line and an SSHSIG envelope.
func sshWirePublicKey(publicKey ed25519.PublicKey) []byte {
	var buffer bytes.Buffer
	var length [4]byte
	binary.BigEndian.PutUint32(length[:], uint32(len("ssh-ed25519")))
	buffer.Write(length[:])
	buffer.WriteString("ssh-ed25519")
	binary.BigEndian.PutUint32(length[:], uint32(len(publicKey)))
	buffer.Write(length[:])
	buffer.Write(publicKey)
	return buffer.Bytes()
}

// SSHPublicKeyLine renders the key the way OpenSSH writes a .pub file:
// "ssh-ed25519 <base64 wire key>".
//
// A host project stores exactly this line as rootPublicKey, because the same
// string is what CI writes into allowed_signers. Keeping one textual form
// removes any need for CI to assemble binary key blobs by hand, which is the
// step that silently produces a line ssh-keygen rejects.
func SSHPublicKeyLine(publicKey ed25519.PublicKey) string {
	return "ssh-ed25519 " + base64.StdEncoding.EncodeToString(sshWirePublicKey(publicKey))
}

// ParsePublicKey accepts both forms a deployment hands out: the OpenSSH line
// ("ssh-ed25519 <base64 wire key>", what /v1/tools/root.pub serves) and the
// bare base64 of the 32 raw key bytes. Documents already written with the bare
// form keep loading, and hosted keys can be pasted verbatim.
func ParsePublicKey(text string) (ed25519.PublicKey, error) {
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return nil, fmt.Errorf("public key is empty")
	}
	switch {
	case len(fields) == 1:
		raw, err := base64.StdEncoding.DecodeString(fields[0])
		if err != nil {
			return nil, fmt.Errorf("public key is not base64: %v", err)
		}
		if len(raw) != ed25519.PublicKeySize {
			return nil, fmt.Errorf("public key has %d bytes, expected %d", len(raw), ed25519.PublicKeySize)
		}
		return ed25519.PublicKey(raw), nil
	case len(fields) == 2 || len(fields) == 3:
		if fields[0] != "ssh-ed25519" {
			return nil, fmt.Errorf("public key type is %q, expected ssh-ed25519", fields[0])
		}
		wire, err := base64.StdEncoding.DecodeString(fields[1])
		if err != nil {
			return nil, fmt.Errorf("public key is not base64: %v", err)
		}
		return parseSSHWirePublicKey(wire)
	default:
		return nil, fmt.Errorf("public key has %d fields, expected a bare base64 key or an ssh-ed25519 line", len(fields))
	}
}

// parseSSHWirePublicKey splits "ssh-ed25519" + key blob and returns the key.
func parseSSHWirePublicKey(wire []byte) (ed25519.PublicKey, error) {
	reader := bytes.NewReader(wire)
	readString := func(what string) ([]byte, error) {
		var length [4]byte
		if _, err := io.ReadFull(reader, length[:]); err != nil {
			return nil, fmt.Errorf("public key is truncated before %s", what)
		}
		size := binary.BigEndian.Uint32(length[:])
		if uint32(reader.Len()) < size {
			return nil, fmt.Errorf("public key is truncated inside %s", what)
		}
		value := make([]byte, size)
		if _, err := io.ReadFull(reader, value); err != nil {
			return nil, err
		}
		return value, nil
	}
	keyType, err := readString("the key type")
	if err != nil {
		return nil, err
	}
	if string(keyType) != "ssh-ed25519" {
		return nil, fmt.Errorf("public key type is %q, expected ssh-ed25519", string(keyType))
	}
	raw, err := readString("the key material")
	if err != nil {
		return nil, err
	}
	if len(raw) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("public key has %d bytes, expected %d", len(raw), ed25519.PublicKeySize)
	}
	if reader.Len() != 0 {
		return nil, fmt.Errorf("public key has %d trailing bytes", reader.Len())
	}
	return ed25519.PublicKey(raw), nil
}

func Sign(value any, keyID string, seed []byte) (Signature, error) {
	if keyID == "" {
		return Signature{}, fmt.Errorf("keyId must not be empty")
	}
	if len(seed) != ed25519.SeedSize {
		return Signature{}, fmt.Errorf("seed has %d bytes, expected %d", len(seed), ed25519.SeedSize)
	}
	data, err := canonical.Bytes(value)
	if err != nil {
		return Signature{}, err
	}
	privateKey := ed25519.NewKeyFromSeed(seed)
	raw := ed25519.Sign(privateKey, data)
	return Signature{
		KeyID:     keyID,
		Algorithm: "ed25519",
		Value:     base64.StdEncoding.EncodeToString(raw),
	}, nil
}

func Verify(value any, signature Signature, trusted map[string]ed25519.PublicKey) error {
	if signature.KeyID == "" {
		return fmt.Errorf("signature keyId must not be empty")
	}
	if signature.Algorithm != "ed25519" {
		return fmt.Errorf("unsupported signature algorithm %q", signature.Algorithm)
	}
	publicKey, ok := trusted[signature.KeyID]
	if !ok {
		return fmt.Errorf("signature key %q is not trusted", signature.KeyID)
	}
	if len(publicKey) != ed25519.PublicKeySize {
		return fmt.Errorf("trusted key %q has invalid length", signature.KeyID)
	}
	raw, err := base64.StdEncoding.DecodeString(signature.Value)
	if err != nil || len(raw) != ed25519.SignatureSize {
		return fmt.Errorf("signature value is not a valid Ed25519 signature")
	}
	data, err := canonical.Bytes(value)
	if err != nil {
		return err
	}
	if !ed25519.Verify(publicKey, data, raw) {
		return fmt.Errorf("signature verification failed for key %q", signature.KeyID)
	}
	return nil
}

// VerifyAny accepts a rotation envelope when at least one valid signature is
// rooted in the caller's pinned trust set. Invalid trusted signatures are not
// ignored: if a trusted key is present but its signature is malformed, the
// document is rejected instead of silently falling through.
func VerifyAny(value any, signatures []Signature, trusted map[string]ed25519.PublicKey) error {
	if err := ValidateSignatures(signatures); err != nil {
		return err
	}
	matched := false
	for _, signature := range signatures {
		if _, ok := trusted[signature.KeyID]; !ok {
			continue
		}
		matched = true
		if err := Verify(value, signature, trusted); err != nil {
			return err
		}
		return nil
	}
	if !matched {
		return fmt.Errorf("none of the signature keys are trusted")
	}
	return fmt.Errorf("no valid trusted signature")
}
