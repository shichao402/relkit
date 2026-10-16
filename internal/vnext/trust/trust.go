// Package trust implements signatures for vNext control-plane JSON
// documents. RUP protobuf envelopes remain owned by internal/envelope.
package trust

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"

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
