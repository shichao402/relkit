package ops

import (
	"fmt"
	"os"
	"path/filepath"

	rupv2 "github.com/shichao402/relkit/api/rup/v2"
	"github.com/shichao402/relkit/internal/keys"
	"github.com/shichao402/relkit/internal/model"
	"github.com/shichao402/relkit/internal/vnext/trust"
)

type KeyResult struct {
	KeyID       string `json:"keyId"`
	Fingerprint string `json:"fingerprint"`
	PrivatePath string `json:"privatePath,omitempty"`
	PublicPath  string `json:"publicPath"`
}

func GenerateKeyPair(keyID, privatePath, publicPath string) (*KeyResult, error) {
	if err := model.CheckIdentifier(keyID, "keyId"); err != nil {
		return nil, err
	}
	if privatePath == "" || publicPath == "" {
		return nil, fmt.Errorf("private and public key output paths are required")
	}
	if privatePath == publicPath {
		return nil, fmt.Errorf("private and public key output paths must differ")
	}
	seed, err := keys.GenerateSeed()
	if err != nil {
		return nil, err
	}
	privateDocument := keys.PrivateKeyDocument(keyID, seed)
	privateBytes, err := rupv2.MarshalPrivateKey(&privateDocument)
	if err != nil {
		return nil, err
	}
	publicDocument := keys.PublicKeyDocument(keyID, seed)
	publicBytes, err := rupv2.MarshalPublicKey(&publicDocument)
	if err != nil {
		return nil, err
	}
	if err := writeNewKey(privatePath, privateBytes, 0o600); err != nil {
		return nil, err
	}
	if err := writeNewKey(publicPath, publicBytes, 0o644); err != nil {
		_ = os.Remove(privatePath)
		return nil, err
	}
	fingerprint, err := trust.Fingerprint(keys.PublicKey(seed))
	if err != nil {
		return nil, err
	}
	return &KeyResult{
		KeyID: keyID, Fingerprint: fingerprint,
		PrivatePath: privatePath, PublicPath: publicPath,
	}, nil
}

func FingerprintKey(publicPath string) (*KeyResult, error) {
	raw, err := os.ReadFile(publicPath)
	if err != nil {
		return nil, err
	}
	document, err := rupv2.UnmarshalPublicKey(raw)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", publicPath, err)
	}
	publicKey, err := keys.LoadPublicKey(*document, publicPath)
	if err != nil {
		return nil, err
	}
	fingerprint, err := trust.Fingerprint(publicKey)
	if err != nil {
		return nil, err
	}
	return &KeyResult{
		KeyID: document.KeyId, Fingerprint: fingerprint, PublicPath: publicPath,
	}, nil
}

// ReissueKeyPair writes the same key material under a new keyId.
//
// The epoch upgrade renames each server's root from a candidate-style name to
// the server's own identity (requirement 10.1). A keyId lives inside the key
// documents themselves, so a rename is a rewrite, not a file move. Keeping the
// seed is what makes this a rename: the fingerprint a host pinned in its project
// configuration stays valid, so nothing already deployed stops verifying.
//
// It refuses to overwrite an existing destination, so a mistyped path cannot
// destroy the only copy of a root.
func ReissueKeyPair(oldKeyID, newKeyID, oldPrivatePath, privatePath, publicPath string) (*KeyResult, error) {
	if err := model.CheckIdentifier(newKeyID, "keyId"); err != nil {
		return nil, err
	}
	if privatePath == "" || publicPath == "" {
		return nil, fmt.Errorf("private and public key output paths are required")
	}
	if privatePath == publicPath {
		return nil, fmt.Errorf("private and public key output paths must differ")
	}
	raw, err := os.ReadFile(oldPrivatePath)
	if err != nil {
		return nil, err
	}
	document, err := rupv2.UnmarshalPrivateKey(raw)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", oldPrivatePath, err)
	}
	if oldKeyID != "" && document.KeyId != "" && document.KeyId != oldKeyID {
		return nil, fmt.Errorf("%s declares keyId %q, not %q", oldPrivatePath, document.KeyId, oldKeyID)
	}
	seed, err := keys.LoadPrivateSeed(*document, oldPrivatePath)
	if err != nil {
		return nil, err
	}
	fingerprint, err := trust.Fingerprint(keys.PublicKey(seed))
	if err != nil {
		return nil, err
	}
	privateDocument := keys.PrivateKeyDocument(newKeyID, seed)
	privateBytes, err := rupv2.MarshalPrivateKey(&privateDocument)
	if err != nil {
		return nil, err
	}
	publicDocument := keys.PublicKeyDocument(newKeyID, seed)
	publicBytes, err := rupv2.MarshalPublicKey(&publicDocument)
	if err != nil {
		return nil, err
	}
	if err := writeNewKey(privatePath, privateBytes, 0o600); err != nil {
		return nil, err
	}
	if err := writeNewKey(publicPath, publicBytes, 0o644); err != nil {
		_ = os.Remove(privatePath)
		return nil, err
	}
	return &KeyResult{
		KeyID: newKeyID, Fingerprint: fingerprint,
		PrivatePath: privatePath, PublicPath: publicPath,
	}, nil
}

func writeNewKey(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return err
	}
	return os.Chmod(path, mode)
}
