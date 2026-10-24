package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	rupv2 "github.com/shichao402/relkit/api/rup/v2"
	"github.com/shichao402/relkit/internal/envelope"
	"github.com/shichao402/relkit/internal/model"
	"github.com/shichao402/relkit/internal/vnext/canonical"
	"github.com/shichao402/relkit/internal/vnext/document"
	"github.com/shichao402/relkit/internal/vnext/project"
	"github.com/shichao402/relkit/internal/vnext/runtimebootstrap"
	"github.com/shichao402/relkit/internal/vnext/runtimeprofile"
	"github.com/shichao402/relkit/internal/vnext/tools"
)

const testRootKeyID = "demo-server-root"

// bootstrapFixture is a fake agent that serves the two documents a bootstrap
// needs: the signed directory (which points at the Profile) and the signed
// Profile itself.
type bootstrapFixture struct {
	server  *httptest.Server
	root    ed25519.PublicKey
	seed    []byte
	profile *runtimeprofile.Document
	// omitProfile makes the directory carry no runtimeProfile reference, the
	// state a server is in before its first Profile publication.
	omitProfile bool
	// profileSignedBy names the key id actually used to sign the Profile, so a
	// mismatch against the server root can be exercised.
	profileSignedBy string
}

func newBootstrapFixture(t *testing.T) *bootstrapFixture {
	t.Helper()
	seed := make([]byte, ed25519.SeedSize)
	for i := range seed {
		seed[i] = byte(i + 11)
	}
	fixture := &bootstrapFixture{
		root:            ed25519.NewKeyFromSeed(seed).Public().(ed25519.PublicKey),
		seed:            seed,
		profileSignedBy: testRootKeyID,
	}
	fixture.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/v1/tools/current/manifest.json"):
			body, err := fixture.manifestBytes()
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			_, _ = w.Write(body)
		case strings.HasSuffix(r.URL.Path, "/"+model.DirectoryKey("demo")):
			body, err := fixture.directoryBytes()
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			_, _ = w.Write(body)
		case strings.Contains(r.URL.Path, "runtime-profile/"):
			raw, err := canonical.Bytes(*fixture.profile)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			_, _ = w.Write(raw)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(fixture.server.Close)

	value, err := runtimeprofile.Create(
		project.Document{
			Schema:        project.Schema,
			Product:       "demo",
			Server:        fixture.server.URL,
			RootPublicKey: base64.StdEncoding.EncodeToString(fixture.root),
			// Channels steer allowedChannels in the generated Profile.
			Channels: []string{"dev", "stable"},
		},
		[]string{fixture.server.URL + "/directory/demo.pb"},
		3,
		envelope.Signer{KeyID: fixture.profileSignedBy, Seed: seed},
	)
	if err != nil {
		t.Fatal(err)
	}
	fixture.profile = value
	return fixture
}

// manifestBytes seals the tool manifest, which is the authority for the server
// root key id. It declares the same id it is signed with.
func (f *bootstrapFixture) manifestBytes() ([]byte, error) {
	encoded, err := tools.EncodePublicKey(f.root)
	if err != nil {
		return nil, err
	}
	value := tools.Document{
		Schema:        tools.Schema,
		Version:       "1.0.0",
		Commit:        "test",
		RootKeyID:     testRootKeyID,
		RootPublicKey: encoded,
		Tools: []tools.Tool{
			{
				Component:    "cli",
				Name:         "relkit",
				Version:      "1.0.0",
				Platform:     "windows-amd64",
				OS:           "windows",
				Arch:         "amd64",
				SHA256:       "sha256:" + strings.Repeat("a", 64),
				Size:         1,
				URL:          f.server.URL + "/v1/tools/current/relkit",
				SignatureURL: f.server.URL + "/v1/tools/current/relkit.sig",
			},
		},
	}
	if err := value.Sign(testRootKeyID, f.seed); err != nil {
		return nil, err
	}
	return canonical.Bytes(value)
}

// directoryBytes seals the signed directory that points at the Profile.
func (f *bootstrapFixture) directoryBytes() ([]byte, error) {
	raw, err := canonical.Bytes(*f.profile)
	if err != nil {
		return nil, err
	}
	directory := &model.UpdateDirectory{
		Schema:            model.SchemaDirectory,
		Product:           "demo",
		DirectorySequence: 4,
	}
	if !f.omitProfile {
		digest := model.Sha256Bytes(raw)
		directory.RuntimeProfile = &rupv2.DigestRef{
			Sha256: digest,
			Size:   int64(len(raw)),
			Urls:   []string{f.server.URL + "/runtime-profile/demo/" + strings.TrimPrefix(digest, "sha256:") + ".json"},
		}
	}
	sealed, err := envelope.SealDirectory(directory, []envelope.Signer{{KeyID: testRootKeyID, Seed: f.seed}})
	if err != nil {
		return nil, err
	}
	return rupv2.MarshalEnvelope(sealed)
}

// writeProject writes the four-field project declaration a host commits.
func writeBootstrapProject(t *testing.T, root, server string) string {
	t.Helper()
	path := filepath.Join(root, defaultProjectPath)
	value := project.Document{
		Schema:        project.Schema,
		Product:       "demo",
		Server:        server,
		RootPublicKey: base64.StdEncoding.EncodeToString(mustFixtureRoot(t)),
	}
	if err := document.Write(path, value); err != nil {
		t.Fatal(err)
	}
	return path
}

// mustFixtureRoot is replaced per-test; see writeBootstrapProjectAt.
func mustFixtureRoot(t *testing.T) ed25519.PublicKey {
	t.Helper()
	seed := make([]byte, ed25519.SeedSize)
	for i := range seed {
		seed[i] = byte(i + 11)
	}
	return ed25519.NewKeyFromSeed(seed).Public().(ed25519.PublicKey)
}

// TestBootstrapBuildsDocumentFromServedProfile covers the whole point of the
// command: a host obtains the embedded document without knowing its format,
// and the result verifies against the project's own root key
// (requirements 3.8, 4.4, 12.1).
func TestBootstrapBuildsDocumentFromServedProfile(t *testing.T) {
	fixture := newBootstrapFixture(t)
	root := t.TempDir()
	projectPath := writeBootstrapProject(t, root, fixture.server.URL)
	outputPath := filepath.Join(root, "bootstrap.json")

	if err := cmdBootstrap([]string{"build", "--project", projectPath, "--out", outputPath}); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	var value runtimebootstrap.Document
	if err := document.Decode(raw, &value); err != nil {
		t.Fatal(err)
	}
	if value.Schema != runtimebootstrap.Schema {
		t.Fatalf("schema=%q", value.Schema)
	}
	if value.Product != "demo" {
		t.Fatalf("product=%q", value.Product)
	}
	if value.RootKeyID != testRootKeyID {
		t.Fatalf("rootKeyId=%q", value.RootKeyID)
	}
	// Validate is the real check: it re-derives the fingerprint and re-verifies
	// the embedded Profile against the embedded root.
	if err := value.Validate(); err != nil {
		t.Fatal(err)
	}
	if got, want := value.Profile.ProfileVersion, int64(3); got != want {
		t.Fatalf("profileVersion=%d want %d", got, want)
	}
}

// TestBootstrapDefaultsOutputUnderDotRelkit pins where an argument-free build
// puts the document, so a host build script has one stable path to embed.
func TestBootstrapDefaultsOutputUnderDotRelkit(t *testing.T) {
	fixture := newBootstrapFixture(t)
	root := t.TempDir()
	projectPath := writeBootstrapProject(t, root, fixture.server.URL)

	if err := cmdBootstrap([]string{"build", "--project", projectPath}); err != nil {
		t.Fatal(err)
	}
	defaultPath := filepath.Join(root, ".relkit", "runtime-bootstrap.json")
	if _, err := os.Stat(defaultPath); err != nil {
		t.Fatalf("%s: %v", defaultPath, err)
	}
}

// TestBootstrapFailsWithoutPublishedProfile covers the honest failure: a server
// that has never published a Profile cannot hand out a bootstrap, and the
// message has to say so instead of emitting a document with an empty profile.
func TestBootstrapFailsWithoutPublishedProfile(t *testing.T) {
	fixture := newBootstrapFixture(t)
	fixture.omitProfile = true
	root := t.TempDir()
	projectPath := writeBootstrapProject(t, root, fixture.server.URL)
	outputPath := filepath.Join(root, "bootstrap.json")

	err := cmdBootstrap([]string{"build", "--project", projectPath, "--out", outputPath})
	if err == nil {
		t.Fatal("expected a server without a Runtime Profile to fail")
	}
	if !strings.Contains(err.Error(), "no Runtime Profile") && !strings.Contains(err.Error(), "serves no Runtime Profile") {
		t.Fatalf("err=%v", err)
	}
	if _, statErr := os.Stat(outputPath); !os.IsNotExist(statErr) {
		t.Fatalf("a failed build must not leave a document behind: %v", statErr)
	}
}

// TestBootstrapRejectsProfileSignedByAnotherKey is the guard that matters: the
// document must not be assembled from a Profile the embedded root did not sign.
// A stale pre-epoch Profile has exactly this defect.
func TestBootstrapRejectsProfileSignedByAnotherKey(t *testing.T) {
	fixture := newBootstrapFixture(t)
	fixture.profileSignedBy = "some-old-product-key"
	// Re-create the Profile under the wrong key id.
	value, err := runtimeprofile.Create(
		project.Document{
			Schema:        project.Schema,
			Product:       "demo",
			Server:        fixture.server.URL,
			RootPublicKey: base64.StdEncoding.EncodeToString(fixture.root),
			Channels:      []string{"dev", "stable"},
		},
		[]string{fixture.server.URL + "/directory/demo.pb"},
		3,
		envelope.Signer{KeyID: fixture.profileSignedBy, Seed: fixture.seed},
	)
	if err != nil {
		t.Fatal(err)
	}
	fixture.profile = value

	root := t.TempDir()
	projectPath := writeBootstrapProject(t, root, fixture.server.URL)
	outputPath := filepath.Join(root, "bootstrap.json")

	err = cmdBootstrap([]string{"build", "--project", projectPath, "--out", outputPath})
	if err == nil {
		t.Fatal("expected a Profile signed by a different key id to be rejected")
	}
	if !strings.Contains(err.Error(), "not signed by the server root") {
		t.Fatalf("err=%v", err)
	}
}
