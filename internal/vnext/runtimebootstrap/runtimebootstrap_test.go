package runtimebootstrap

import (
	"crypto/ed25519"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/shichao402/relkit/internal/jsonio"
	"github.com/shichao402/relkit/internal/vnext/runtimeprofile"
	"github.com/shichao402/relkit/internal/vnext/trust"
	"github.com/shichao402/relkit/sdk"
)

func TestGeneratedRuntimeBootstrapMatchesPublicSDK(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	fingerprint, err := trust.Fingerprint(publicKey)
	if err != nil {
		t.Fatal(err)
	}
	profile := runtimeprofile.Document{
		Schema:          runtimeprofile.Schema,
		Product:         "demo",
		ProfileVersion:  1,
		AllowedChannels: []string{"stable"},
		DirectoryBootstrap: runtimeprofile.DirectoryBootstrap{
			EntryURLs: []string{"https://updates.example/directory/demo.pb"},
		},
		RUPSchemaSet: "rup/v2",
	}
	if err := profile.Sign("root-1", privateKey.Seed()); err != nil {
		t.Fatal(err)
	}
	resolutionDigest := "sha256:" + strings.Repeat("a", 64)
	value, err := New(
		resolutionDigest,
		fingerprint,
		profile,
		map[string]ed25519.PublicKey{"root-1": publicKey},
	)
	if err != nil {
		t.Fatal(err)
	}
	if value.Profile.Product != "demo" || value.ProfileDigest != profile.Digest {
		t.Fatalf("unexpected bootstrap: %#v", value)
	}
	if len(value.TrustedKeys) != 1 || value.TrustedKeys[0].KeyID != "root-1" {
		t.Fatalf("unexpected trusted keys: %#v", value.TrustedKeys)
	}
	if value.TrustedKeys[0].Key != base64.StdEncoding.EncodeToString(publicKey) {
		t.Fatalf("trusted key is not canonical base64: %q", value.TrustedKeys[0].Key)
	}
	data, err := jsonio.MarshalPretty(value)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := sdk.RuntimeBootstrapFromJSON(data)
	if err != nil {
		t.Fatalf("public SDK rejected generated runtime bootstrap: %v", err)
	}
	if parsed.ClientProfile.Product != "demo" {
		t.Fatalf("SDK parsed product %q, want demo", parsed.ClientProfile.Product)
	}
}

func TestValidateRejectsSelfCertifiedOrWrongRoot(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	otherPublicKey, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	fingerprint, err := trust.Fingerprint(publicKey)
	if err != nil {
		t.Fatal(err)
	}
	profile := runtimeprofile.Document{
		Schema:          runtimeprofile.Schema,
		Product:         "demo",
		ProfileVersion:  1,
		AllowedChannels: []string{"stable"},
		DirectoryBootstrap: runtimeprofile.DirectoryBootstrap{
			EntryURLs: []string{"https://updates.example/directory/demo.pb"},
		},
		RUPSchemaSet: "rup/v2",
	}
	if err := profile.Sign("root-1", privateKey.Seed()); err != nil {
		t.Fatal(err)
	}
	value := Document{
		Schema:           Schema,
		ResolutionDigest: "sha256:" + strings.Repeat("a", 64),
		ProfileDigest:    profile.Digest,
		RootFingerprint:  fingerprint,
		Profile:          profile,
		TrustedKeys: []TrustedKey{{
			KeyID: "root-1",
			Key:   base64.StdEncoding.EncodeToString(otherPublicKey),
		}},
	}
	if err := value.Validate(); err == nil || !strings.Contains(err.Error(), "does not match rootFingerprint") {
		t.Fatalf("expected wrong root rejection, got %v", err)
	}
}
