package updater

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"

	updaterv1 "github.com/shichao402/relkit/api/updater/v1"
	"github.com/shichao402/relkit/internal/testutil"
	"github.com/shichao402/relkit/internal/vnext/runtimebootstrap"
	"github.com/shichao402/relkit/internal/vnext/runtimeprofile"
)

// updateFixtures rewrites the conformance fixtures instead of asserting them.
//
//	 go test ./internal/updater/ -run RuntimeBootstrapConformance -args -update
//
// The fixtures are golden files: they are produced by the real generator
// (runtimebootstrap.New over a signed runtime profile), never typed by hand, so
// they cannot drift from what the publisher emits (requirement 3.9).
var updateFixtures = flag.Bool("update", false, "rewrite conformance fixtures from the generator")

// conformanceRootSeed is the fixed key every bootstrap fixture is built from.
// It is a test vector, not a deployment key: it exists only so the fixtures are
// reproducible byte for byte.
var conformanceRootSeed = bytes.Repeat([]byte{0x42}, ed25519.SeedSize)

const conformanceRootKeyID = "conformance-root"

// conformanceBootstrap renders the bootstrap the publisher would ship for the
// conformance product, using the fixed seed above.
func conformanceBootstrap(t *testing.T) []byte {
	t.Helper()
	privateKey := ed25519.NewKeyFromSeed(conformanceRootSeed)
	publicKey := privateKey.Public().(ed25519.PublicKey)
	profile := runtimeprofile.Document{
		Schema:          runtimeprofile.Schema,
		Product:         "loom",
		ProfileVersion:  7,
		AllowedChannels: []string{"dev", "stable"},
		DirectoryBootstrap: runtimeprofile.DirectoryBootstrap{
			EntryURLs: []string{"https://update.example.invalid/"},
		},
		RUPSchemaSet: runtimeprofile.SchemaSet,
	}
	if err := profile.Sign(conformanceRootKeyID, conformanceRootSeed); err != nil {
		t.Fatalf("sign runtime profile: %v", err)
	}
	doc, err := runtimebootstrap.New(conformanceRootKeyID, publicKey, "loom", profile)
	if err != nil {
		t.Fatalf("build runtime bootstrap: %v", err)
	}
	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatalf("marshal runtime bootstrap: %v", err)
	}
	return append(raw, '\n')
}

// numericKeyBootstrap reproduces the 2026-10-17 accident: rootPublicKey written
// as an array of numbers instead of an OpenSSH line. Hosts shipped it happily;
// the typed reader must reject it.
func numericKeyBootstrap(t *testing.T, good []byte) []byte {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal(good, &doc); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	doc["rootPublicKey"] = []int{115, 115, 104, 45, 101, 100, 50, 53, 53, 49, 57}
	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatalf("marshal broken fixture: %v", err)
	}
	return append(raw, '\n')
}

func fixturePath(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join(testutil.ConformanceRoot(t), "updater", name)
}

// fixtureBytes reads a golden fixture with its line endings normalized to LF.
//
// The comparison below is byte for byte against what the generator emits, and
// the generator always emits LF. Git's end-of-line conversion can still hand us
// CRLF on a Windows checkout — `.gitattributes` marks conformance/ as -text so
// it should not, but a clone made before that file existed, or a checkout with
// `--config core.autocrlf=true` overriding attributes, would otherwise report
// "drifted from the generator" for a file nobody edited. Normalizing the read
// side keeps the assertion about the JSON, which is what it is meant to guard.
func fixtureBytes(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return bytes.ReplaceAll(raw, []byte("\r\n"), []byte("\n"))
}

func TestRuntimeBootstrapConformanceFixture(t *testing.T) {
	want := conformanceBootstrap(t)
	path := fixturePath(t, "runtime-bootstrap.json")
	if *updateFixtures {
		if err := os.WriteFile(path, want, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	got := fixtureBytes(t, path)
	if !bytes.Equal(got, want) {
		t.Fatal("runtime-bootstrap.json drifted from the generator; " +
			"regenerate with -run RuntimeBootstrapConformance -args -update")
	}
	// The fixture must be accepted by the engine exactly as a host would send it.
	resolved, report, rerr := resolveBootstrap(
		&updaterv1.ClientProfile{Bootstrap: string(got), ExpectedProduct: "loom"}, "dev")
	if rerr != nil {
		t.Fatalf("fixture must resolve: %v", rerr)
	}
	if resolved.Product != "loom" || len(resolved.TrustedKeys) != 1 {
		t.Fatalf("resolved profile = %+v", resolved)
	}
	if report == nil || report.RootKeyId != conformanceRootKeyID || report.DefaultChannel != "dev" {
		t.Fatalf("handshake report = %+v", report)
	}
	if len(report.AllowedChannels) != 2 {
		t.Fatalf("allowedChannels = %v", report.AllowedChannels)
	}
}

func TestRuntimeBootstrapNumericKeyConformanceFixture(t *testing.T) {
	path := fixturePath(t, "runtime-bootstrap-numeric-key.json")
	if *updateFixtures {
		broken := numericKeyBootstrap(t, conformanceBootstrap(t))
		if err := os.WriteFile(path, broken, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	raw := fixtureBytes(t, path)
	_, _, rerr := resolveBootstrap(&updaterv1.ClientProfile{Bootstrap: string(raw)}, "dev")
	if rerr == nil {
		t.Fatal("a numeric rootPublicKey must be rejected, not hashed")
	}
	if rerr.Code != updaterv1.ErrorCode_ERROR_CODE_PROFILE_INVALID {
		t.Fatalf("code = %v, want PROFILE_INVALID", rerr.Code)
	}
}
