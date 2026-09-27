package keys

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"runtime"

	"github.com/shichao402/relkit/internal/model"
)

const (
	SchemaPublicKey  = model.SchemaPublicKey
	SchemaPrivateKey = model.SchemaPrivateKey
	SeedBytes        = ed25519.SeedSize
)

type Error struct {
	Message string
}

func (e Error) Error() string {
	return e.Message
}

func GenerateSeed() ([]byte, error) {
	seed := make([]byte, SeedBytes)
	if _, err := rand.Read(seed); err != nil {
		return nil, err
	}
	return seed, nil
}

func PublicKeyDocument(keyID string, seed []byte) model.PublicKeyDocument {
	return model.PublicKeyDocument{
		Schema:    SchemaPublicKey,
		KeyId:     keyID,
		Alg:       "ed25519",
		PublicKey: append([]byte(nil), PublicKey(seed)...),
	}
}

func PrivateKeyDocument(keyID string, seed []byte) model.PrivateKeyDocument {
	return model.PrivateKeyDocument{
		Schema: SchemaPrivateKey,
		KeyId:  keyID,
		Alg:    "ed25519",
		Seed:   append([]byte(nil), seed...),
	}
}

func DecodeSeed(encoded, source string) ([]byte, error) {
	seed, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, Error{Message: fmt.Sprintf("%s does not contain valid base64", source)}
	}
	if len(seed) != SeedBytes {
		return nil, Error{Message: fmt.Sprintf("%s decodes to %d bytes, expected %d", source, len(seed), SeedBytes)}
	}
	return seed, nil
}

func DecodePublicKey(encoded, source string) (ed25519.PublicKey, error) {
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, Error{Message: fmt.Sprintf("%s does not contain valid base64", source)}
	}
	if len(raw) != ed25519.PublicKeySize {
		return nil, Error{Message: fmt.Sprintf("%s decodes to %d bytes, expected %d", source, len(raw), ed25519.PublicKeySize)}
	}
	return ed25519.PublicKey(raw), nil
}

func LoadPrivateSeed(document model.PrivateKeyDocument, source string) ([]byte, error) {
	if document.Schema != SchemaPrivateKey {
		return nil, Error{Message: fmt.Sprintf("%s has schema %q, expected %q", source, document.Schema, SchemaPrivateKey)}
	}
	if len(document.Seed) != SeedBytes {
		return nil, Error{Message: fmt.Sprintf("%s decodes to %d bytes, expected %d", source, len(document.Seed), SeedBytes)}
	}
	return append([]byte(nil), document.Seed...), nil
}

func LoadPublicKey(document model.PublicKeyDocument, source string) (ed25519.PublicKey, error) {
	if document.Schema != SchemaPublicKey {
		return nil, Error{Message: fmt.Sprintf("%s has schema %q, expected %q", source, document.Schema, SchemaPublicKey)}
	}
	if len(document.PublicKey) != ed25519.PublicKeySize {
		return nil, Error{Message: fmt.Sprintf("%s decodes to %d bytes, expected %d", source, len(document.PublicKey), ed25519.PublicKeySize)}
	}
	cloned := make(ed25519.PublicKey, len(document.PublicKey))
	copy(cloned, document.PublicKey)
	return cloned, nil
}

func RestrictPermissions(path string) {
	if runtime.GOOS == "windows" {
		return
	}
	_ = os.Chmod(path, 0o600)
}

func IsGitIgnored(path, repoRoot string) (*bool, error) {
	cmd := exec.Command("git", "check-ignore", "-q", path)
	cmd.Dir = repoRoot
	cmd.Stdout = nil
	cmd.Stderr = nil
	err := cmd.Run()
	if err == nil {
		value := true
		return &value, nil
	}
	var exitErr *exec.ExitError
	if ok := AsExitError(err, &exitErr); ok {
		switch exitErr.ExitCode() {
		case 1:
			value := false
			return &value, nil
		default:
			return nil, nil
		}
	}
	return nil, nil
}

func PublicKey(seed []byte) ed25519.PublicKey {
	privateKey := ed25519.NewKeyFromSeed(seed)
	publicKey := privateKey.Public().(ed25519.PublicKey)
	cloned := make(ed25519.PublicKey, len(publicKey))
	copy(cloned, publicKey)
	return cloned
}

func AsExitError(err error, target **exec.ExitError) bool {
	exitErr, ok := err.(*exec.ExitError)
	if ok {
		*target = exitErr
	}
	return ok
}
