package runtimebootstrap

import (
	"crypto/ed25519"
	"strings"
	"testing"

	"github.com/shichao402/relkit/internal/jsonio"
	"github.com/shichao402/relkit/internal/vnext/runtimeprofile"
	"github.com/shichao402/relkit/internal/vnext/trust"
)

func testProfile(t *testing.T, privateKey ed25519.PrivateKey) runtimeprofile.Document {
	t.Helper()
	profile := runtimeprofile.Document{
		Schema:          runtimeprofile.Schema,
		Product:         "demo",
		ProfileVersion:  1,
		AllowedChannels: []string{"stable"},
		DirectoryBootstrap: runtimeprofile.DirectoryBootstrap{
			EntryURLs: []string{"https://updates.example/directory/demo.pb"},
		},
		RUPSchemaSet: runtimeprofile.SchemaSet,
	}
	if err := profile.Sign("root-1", privateKey.Seed()); err != nil {
		t.Fatal(err)
	}
	return profile
}

func TestBootstrapEmbedsTheOneRoot(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	profile := testProfile(t, privateKey)
	value, err := New("root-1", publicKey, "demo", profile)
	if err != nil {
		t.Fatal(err)
	}
	if value.Schema != "relkit.runtime-bootstrap/2" {
		t.Fatalf("schema = %q", value.Schema)
	}
	if value.RootKeyID != "root-1" || value.RootPublicKey != trust.SSHPublicKeyLine(publicKey) {
		t.Fatalf("unexpected root: %#v", value)
	}
	// The embedded line must parse back to the same key: a host embeds this
	// text verbatim, so a form it cannot read is as bad as a wrong key.
	parsed, err := value.PublicKey()
	if err != nil {
		t.Fatal(err)
	}
	if !parsed.Equal(publicKey) {
		t.Fatal("embedded rootPublicKey does not round-trip")
	}
}

func TestBootstrapRejectsProfileFromAnotherRoot(t *testing.T) {
	publicKey, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	_, otherPrivateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	profile := testProfile(t, otherPrivateKey)
	if _, err := New("root-1", publicKey, "demo", profile); err == nil ||
		!strings.Contains(err.Error(), "not signed by the embedded server root") {
		t.Fatalf("expected a foreign-root rejection, got %v", err)
	}
}

func TestBootstrapRejectsProductMismatch(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	profile := testProfile(t, privateKey)
	if _, err := New("root-1", publicKey, "other", profile); err == nil ||
		!strings.Contains(err.Error(), "belongs to product") {
		t.Fatalf("expected a product mismatch rejection, got %v", err)
	}
}

func TestValidateRejectsFingerprintMismatch(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	profile := testProfile(t, privateKey)
	value, err := New("root-1", publicKey, "demo", profile)
	if err != nil {
		t.Fatal(err)
	}
	value.RootFingerprint = "sha256:" + strings.Repeat("a", 64)
	if err := value.Validate(); err == nil || !strings.Contains(err.Error(), "does not match rootPublicKey") {
		t.Fatalf("expected a fingerprint mismatch rejection, got %v", err)
	}
}

// TestBootstrapJSONShape pins the document a host embeds: one root key id, one
// base64 public key, and the signed profile. There is no key set any more.
func TestBootstrapJSONShape(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	value, err := New("root-1", publicKey, "demo", testProfile(t, privateKey))
	if err != nil {
		t.Fatal(err)
	}
	data, err := jsonio.MarshalPretty(value)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{`"rootKeyId"`, `"rootPublicKey"`, `"rootFingerprint"`, `"profile"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("bootstrap JSON is missing %s:\n%s", want, text)
		}
	}
	for _, gone := range []string{"trustedKeys", "resolutionDigest", "profileDigest"} {
		if strings.Contains(text, gone) {
			t.Fatalf("bootstrap JSON still carries %q:\n%s", gone, text)
		}
	}
}
