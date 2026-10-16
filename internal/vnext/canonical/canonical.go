// Package canonical provides the byte representation and digest used by
// vNext signed documents. It deliberately reuses relkit's existing canonical
// JSON encoder so every platform computes the same bytes.
package canonical

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/shichao402/relkit/internal/jsonio"
)

const DigestPrefix = "sha256:"

// Bytes returns canonical UTF-8 JSON: sorted object keys, no insignificant
// whitespace, and no trailing newline.
func Bytes(value any) ([]byte, error) {
	return jsonio.MarshalCompact(value)
}

// Digest returns the canonical SHA-256 digest of value.
func Digest(value any) (string, error) {
	data, err := Bytes(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return DigestPrefix + hex.EncodeToString(sum[:]), nil
}

// ValidateDigest checks only the stable digest representation. Callers still
// need to compare it with Digest(document).
func ValidateDigest(value string) error {
	if !strings.HasPrefix(value, DigestPrefix) {
		return fmt.Errorf("digest %q must start with %q", value, DigestPrefix)
	}
	raw := strings.TrimPrefix(value, DigestPrefix)
	decoded, err := hex.DecodeString(raw)
	if err != nil || len(decoded) != sha256.Size {
		return fmt.Errorf("digest %q is not a SHA-256 value", value)
	}
	return nil
}
