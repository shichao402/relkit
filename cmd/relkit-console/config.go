package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// SiteConfig is the operator's copy for the human-facing pages, carried over
// from relkit-serve unchanged. None of it reaches protocol clients.
type SiteConfig struct {
	Title string `json:"title"`
	// Keyed by the directory name under index/, which is the product id used
	// everywhere else in RUP.
	Products map[string]*ProductConfig `json:"products"`
}

type ProductConfig struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Homepage    string `json:"homepage"`
}

func (s *SiteConfig) product(id string) *ProductConfig {
	if s == nil || s.Products == nil {
		return nil
	}
	return s.Products[id]
}

// casKeyFileName mirrors the serve-side CAS secret file name: the stats
// loader refuses to serve it, so the reserved-key list stays aligned even
// though the console never mints CAS URLs.
const casKeyFileName = ".relkit-serve-cas.key"

func resolveRelative(path, configPath string) string {
	if filepath.IsAbs(path) || configPath == "" {
		return path
	}
	return filepath.Join(filepath.Dir(configPath), path)
}

// generateToken is the shared secret mint (32 random bytes, url-safe base64).
func generateToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// hashToken reduces a token to a fixed-size digest so comparison is
// constant-time without leaking length.
func hashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

// humanBytes renders sizes for panel rows, carried over from serve.
func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	value := float64(n)
	for _, suffix := range []string{"KiB", "MiB", "GiB", "TiB"} {
		value /= unit
		if value < unit {
			return fmt.Sprintf("%.1f %s", value, suffix)
		}
	}
	return fmt.Sprintf("%.1f PiB", value)
}

// writeFileConfig persists a config with stable formatting.
func writeFileConfig(path string, cfg *FileConfig) error {
	out, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(out, '\n'), 0o644)
}
