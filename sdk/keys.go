package sdk

import "crypto/ed25519"

// TrustedKeys maps keyId -> raw 32-byte ed25519 public key.
type TrustedKeys map[string]ed25519.PublicKey
