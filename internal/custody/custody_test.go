package custody

import (
	"crypto/ed25519"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	rupv2 "github.com/shichao402/relkit/api/rup/v2"
	"github.com/shichao402/relkit/internal/keys"
	"github.com/shichao402/relkit/internal/vnext/trust"
)

type fixture struct {
	dir        string
	manifest   *Manifest
	seed       []byte
	publicKey  ed25519.PublicKey
	fingerprint string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	dir := t.TempDir()

	seed := make([]byte, ed25519.SeedSize)
	for i := range seed {
		seed[i] = byte(i + 7)
	}
	publicKey := ed25519.NewKeyFromSeed(seed).Public().(ed25519.PublicKey)
	fingerprint, err := trust.Fingerprint(publicKey)
	if err != nil {
		t.Fatal(err)
	}

	writeKeyFiles(t, dir, "internal-root", seed)

	manifest := &Manifest{
		Schema: ManifestSchema,
		Scope:  ScopeArchive,
		Keys: []Entry{
			{
				KeyID:       "internal-root",
				Kind:        KindRoot,
				Purpose:     "signs profile, catalog, index and manifest for the internal server",
				Carrier:     CarrierFile,
				Path:        "internal-root.private.pb",
				PublicPath:  "internal-root.public.pb",
				Fingerprint: fingerprint,
			},
			{
				KeyID:   "ci-upload-token",
				Kind:    KindTokenRef,
				Purpose: "host CI publishes to the internal server",
				Carrier: CarrierEnv,
				Path:    "RELKIT_TEST_CI_TOKEN",
			},
		},
	}
	path := filepath.Join(dir, "custody-manifest.json")
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	return &fixture{
		dir: dir, manifest: loaded, seed: seed,
		publicKey: publicKey, fingerprint: fingerprint,
	}
}

func writeKeyFiles(t *testing.T, dir, keyID string, seed []byte) {
	t.Helper()
	privateDocument := keys.PrivateKeyDocument(keyID, seed)
	privateBytes, err := rupv2.MarshalPrivateKey(&privateDocument)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, keyID+".private.pb"), privateBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	publicDocument := keys.PublicKeyDocument(keyID, seed)
	publicBytes, err := rupv2.MarshalPublicKey(&publicDocument)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, keyID+".public.pb"), publicBytes, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestPrivateSeedRoundTripsAgainstRegisteredFingerprint(t *testing.T) {
	fx := newFixture(t)

	seed, err := fx.manifest.PrivateSeed("internal-root", "internal-root")
	if err != nil {
		t.Fatal(err)
	}
	if string(seed) != string(fx.seed) {
		t.Fatal("returned seed does not match the registered key")
	}
	fingerprint, err := fx.manifest.Fingerprint("internal-root")
	if err != nil {
		t.Fatal(err)
	}
	if fingerprint != fx.fingerprint {
		t.Fatalf("fingerprint = %q, want %q", fingerprint, fx.fingerprint)
	}
}

func TestPrivateSeedRejectsUnknownKeyID(t *testing.T) {
	fx := newFixture(t)

	_, err := fx.manifest.PrivateSeed("no-such-key")
	if err == nil {
		t.Fatal("expected an error for an unregistered keyId")
	}
	custodyErr, ok := err.(Error)
	if !ok {
		t.Fatalf("error type = %T, want custody.Error", err)
	}
	if custodyErr.KeyID != "no-such-key" {
		t.Fatalf("error keyId = %q, want no-such-key", custodyErr.KeyID)
	}
	if !strings.Contains(custodyErr.Reason, "internal-root") {
		t.Fatalf("error reason should list registered keyIds, got %q", custodyErr.Reason)
	}
}

func TestPrivateSeedRejectsFingerprintDrift(t *testing.T) {
	fx := newFixture(t)
	fx.manifest.Keys[0].Fingerprint = "sha256:" + strings.Repeat("a", 64)

	_, err := fx.manifest.PrivateSeed("internal-root")
	if err == nil {
		t.Fatal("expected a fingerprint drift error")
	}
	if !strings.Contains(err.Error(), "manifest registers") {
		t.Fatalf("error = %q, want a fingerprint mismatch", err.Error())
	}
}

func TestPrivateSeedRejectsMismatchedKeyMaterial(t *testing.T) {
	fx := newFixture(t)
	// Replace the carrier with a key that is valid but is not the registered one.
	otherSeed := make([]byte, ed25519.SeedSize)
	for i := range otherSeed {
		otherSeed[i] = byte(i + 99)
	}
	writeKeyFiles(t, fx.dir, "internal-root", otherSeed)

	_, err := fx.manifest.PrivateSeed("internal-root")
	if err == nil {
		t.Fatal("expected a mismatch error")
	}
	if !strings.Contains(err.Error(), "private key yields public fingerprint") {
		t.Fatalf("error = %q, want a derived-fingerprint mismatch", err.Error())
	}
}

func TestPrivateSeedRejectsForeignSigningIdentity(t *testing.T) {
	fx := newFixture(t)

	_, err := fx.manifest.PrivateSeed("internal-root", "other-server-root")
	if err == nil {
		t.Fatal("expected an error when asked to sign as another server's root")
	}
	if !strings.Contains(err.Error(), "signs only as its own root") {
		t.Fatalf("error = %q", err.Error())
	}
}

func TestCarrierUnreadableIsReported(t *testing.T) {
	fx := newFixture(t)
	if err := os.Remove(filepath.Join(fx.dir, "internal-root.private.pb")); err != nil {
		t.Fatal(err)
	}

	_, err := fx.manifest.PrivateSeed("internal-root")
	if err == nil {
		t.Fatal("expected an unreadable carrier error")
	}
	if !strings.Contains(err.Error(), "cannot read") {
		t.Fatalf("error = %q, want a carrier read failure", err.Error())
	}
}

func TestEnvCarrierMissingIsReported(t *testing.T) {
	fx := newFixture(t)
	t.Setenv("RELKIT_TEST_CI_TOKEN", "")

	_, err := fx.manifest.Token("ci-upload-token")
	if err == nil {
		t.Fatal("expected an error for an empty environment carrier")
	}
	if !strings.Contains(err.Error(), "RELKIT_TEST_CI_TOKEN") {
		t.Fatalf("error = %q, want the variable name", err.Error())
	}
}

func TestTokenReadsFromEnvironmentCarrier(t *testing.T) {
	fx := newFixture(t)
	t.Setenv("RELKIT_TEST_CI_TOKEN", "rkt_example_token")

	token, err := fx.manifest.Token("ci-upload-token")
	if err != nil {
		t.Fatal(err)
	}
	if token != "rkt_example_token" {
		t.Fatalf("token = %q", token)
	}
}

func TestTokenRejectsRootEntry(t *testing.T) {
	fx := newFixture(t)

	if _, err := fx.manifest.Token("internal-root"); err == nil {
		t.Fatal("expected an error when asking a root entry for a token")
	}
}

func TestManifestRejectsMissingSchemaAndBadShape(t *testing.T) {
	cases := map[string]string{
		"no schema":        `{"scope":"server","keys":[{"keyId":"k","kind":"root","carrier":"file","path":"k.pb","fingerprint":"sha256:x"}]}`,
		"bad scope":        `{"schema":"` + ManifestSchema + `","scope":"laptop","keys":[{"keyId":"k","kind":"root","carrier":"file","path":"k.pb","fingerprint":"sha256:x"}]}`,
		"no keys":          `{"schema":"` + ManifestSchema + `","scope":"server","keys":[]}`,
		"unknown kind":     `{"schema":"` + ManifestSchema + `","scope":"server","keys":[{"keyId":"k","kind":"skeleton","carrier":"file","path":"k.pb"}]}`,
		"bad carrier":      `{"schema":"` + ManifestSchema + `","scope":"server","keys":[{"keyId":"k","kind":"token","carrier":"vault","path":"x"}]}`,
		"root without fp":  `{"schema":"` + ManifestSchema + `","scope":"server","keys":[{"keyId":"k","kind":"root","carrier":"file","path":"k.pb"}]}`,
		"root from env":    `{"schema":"` + ManifestSchema + `","scope":"server","keys":[{"keyId":"k","kind":"root","carrier":"env","path":"K","fingerprint":"sha256:x"}]}`,
		"duplicate keyId":  `{"schema":"` + ManifestSchema + `","scope":"server","keys":[{"keyId":"k","kind":"token","carrier":"env","path":"A"},{"keyId":"k","kind":"token","carrier":"env","path":"B"}]}`,
	}
	for name, document := range cases {
		if _, err := Parse([]byte(document)); err == nil {
			t.Fatalf("%s: expected a validation error", name)
		}
	}
}

func TestSelfCheckCoversEveryRequestedKey(t *testing.T) {
	fx := newFixture(t)
	t.Setenv("RELKIT_TEST_CI_TOKEN", "rkt_example_token")

	report, err := fx.manifest.SelfCheck("internal-root", "ci-upload-token")
	if err != nil {
		t.Fatal(err)
	}
	if !report.OK() || len(report.Keys) != 2 {
		t.Fatalf("report = %+v", report)
	}
	if report.Keys[0].Fingerprint != fx.fingerprint {
		t.Fatalf("root fingerprint = %q", report.Keys[0].Fingerprint)
	}
	if report.Keys[0].Verified != "signature round-trip" {
		t.Fatalf("root verified = %q", report.Keys[0].Verified)
	}
	if report.Keys[1].Verified != "carrier readable" {
		t.Fatalf("token verified = %q", report.Keys[1].Verified)
	}
	encoded, err := report.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(string(encoded), "\n") {
		t.Fatal("encoded report should end with a newline")
	}
}

func TestSelfCheckStopsOnFirstFailure(t *testing.T) {
	fx := newFixture(t)
	report, err := fx.manifest.SelfCheck("internal-root", "absent-key")
	if err == nil {
		t.Fatal("expected a self-check failure")
	}
	// The report records the failing key as its last entry so the caller can
	// name it, and stops there without touching later keys.
	if len(report.Keys) != 2 {
		t.Fatalf("report should record the failing key and stop, got %d entries", len(report.Keys))
	}
	if report.Keys[1].KeyID != "absent-key" || report.Keys[1].Verified != "" {
		t.Fatalf("last report entry = %+v, want the unverified failing key", report.Keys[1])
	}
}

func TestRelativeCarrierResolvesAgainstManifestDirectory(t *testing.T) {
	fx := newFixture(t)
	nested := filepath.Join(fx.dir, "nested")
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	writeKeyFiles(t, nested, "nested-root", fx.seed)
	if err := os.WriteFile(filepath.Join(nested, "manifest.json"), mustJSON(t, &Manifest{
		Schema: ManifestSchema,
		Scope:  ScopeServer,
		Keys: []Entry{{
			KeyID: "nested-root", Kind: KindRoot, Purpose: "nested",
			Carrier: CarrierFile, Path: "nested-root.private.pb",
			Fingerprint: fx.fingerprint,
		}},
	}), 0o600); err != nil {
		t.Fatal(err)
	}

	manifest, err := Load(filepath.Join(nested, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manifest.PrivateSeed("nested-root"); err != nil {
		t.Fatalf("relative carrier should resolve against the manifest directory: %v", err)
	}
}

func TestPublicKeyFallsBackToPrivateCarrier(t *testing.T) {
	fx := newFixture(t)
	fx.manifest.Keys[0].PublicPath = "absent.public.pb"

	publicKey, err := fx.manifest.PublicKey("internal-root")
	if err != nil {
		t.Fatal(err)
	}
	if string(publicKey) != string(fx.publicKey) {
		t.Fatal("derived public key does not match")
	}
}

func TestEncodeDecodePublicKey(t *testing.T) {
	fx := newFixture(t)
	encoded := EncodePublicKey(fx.publicKey)
	decoded, err := DecodePublicKey(encoded, "project.trustRoot")
	if err != nil {
		t.Fatal(err)
	}
	if string(decoded) != string(fx.publicKey) {
		t.Fatal("decoded public key does not match")
	}
	if _, err := DecodePublicKey("not base64", "project.trustRoot"); err == nil {
		t.Fatal("expected a decode error")
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return data
}
