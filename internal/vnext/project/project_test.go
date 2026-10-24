package project

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/shichao402/relkit/internal/vnext/document"
)

// testRootPublicKey is an opaque 32-byte key: the project declaration only
// carries the server root it trusts, and nothing here verifies signatures.
var testRootPublicKey = base64.StdEncoding.EncodeToString(make([]byte, 32))

func validDocument() Document {
	return Document{
		Schema:        Schema,
		Product:       "example-product",
		Server:        "https://updates.example",
		RootPublicKey: testRootPublicKey,
		Channels:      []string{"stable", "beta"},
		ArtifactProducer: &Command{
			Command:         []string{"./scripts/build"},
			ExtensionSchema: "relkit.extension/1",
		},
	}
}

func TestValidateAndDigest(t *testing.T) {
	value := validDocument()
	if err := value.Validate(); err != nil {
		t.Fatal(err)
	}
	digest, err := value.Digest()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(digest, "sha256:") {
		t.Fatalf("unexpected digest %q", digest)
	}
}

func TestServerAndRootKeyAreRequired(t *testing.T) {
	value := validDocument()
	value.Server = ""
	if err := value.Validate(); err == nil || !strings.Contains(err.Error(), "server must be an absolute") {
		t.Fatalf("expected a server rejection, got %v", err)
	}

	value = validDocument()
	value.Server = "ftp://updates.example"
	if err := value.Validate(); err == nil || !strings.Contains(err.Error(), "http or https") {
		t.Fatalf("expected a scheme rejection, got %v", err)
	}

	value = validDocument()
	value.Server = "https://user:secret@updates.example"
	if err := value.Validate(); err == nil || !strings.Contains(err.Error(), "must not contain credentials") {
		t.Fatalf("expected a credentials rejection, got %v", err)
	}

	value = validDocument()
	value.RootPublicKey = ""
	if err := value.Validate(); err == nil || !strings.Contains(err.Error(), "rootPublicKey") {
		t.Fatalf("expected a rootPublicKey rejection, got %v", err)
	}

	value = validDocument()
	value.RootPublicKey = base64.StdEncoding.EncodeToString([]byte{1, 2, 3})
	if err := value.Validate(); err == nil || !strings.Contains(err.Error(), "expected 32") {
		t.Fatalf("expected a root key length rejection, got %v", err)
	}

	// The form the server publishes at /v1/tools/root.pub must load verbatim:
	// a host copies that line, and CI reuses it for allowed_signers.
	value = validDocument()
	value.RootPublicKey = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	if err := value.Validate(); err != nil {
		t.Fatalf("an ssh-ed25519 public key line must be accepted, got %v", err)
	}

	value = validDocument()
	value.RootPublicKey = "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAAAgQ=="
	if err := value.Validate(); err == nil || !strings.Contains(err.Error(), "expected ssh-ed25519") {
		t.Fatalf("expected a key type rejection, got %v", err)
	}
}

func TestServerURLAndRootKeyAccessors(t *testing.T) {
	value := validDocument()
	server, err := value.ServerURL()
	if err != nil {
		t.Fatal(err)
	}
	if server != "https://updates.example" {
		t.Fatalf("ServerURL = %q", server)
	}
	key, err := value.RootKey()
	if err != nil {
		t.Fatal(err)
	}
	if len(key) != 32 {
		t.Fatalf("RootKey returned %d bytes", len(key))
	}
}

func TestChannelsAndSelectorsRejectDuplicates(t *testing.T) {
	value := validDocument()
	value.Channels = []string{"stable", "stable"}
	if err := value.Validate(); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("expected a duplicate channel rejection, got %v", err)
	}

	value = validDocument()
	value.SelectorDimensions = []string{"os-arch", ""}
	if err := value.Validate(); err == nil || !strings.Contains(err.Error(), "empty or duplicate") {
		t.Fatalf("expected a selector rejection, got %v", err)
	}
}

func TestCommandsRejectEmptyAndBadSchema(t *testing.T) {
	value := validDocument()
	value.ArtifactProducer = &Command{}
	if err := value.Validate(); err == nil || !strings.Contains(err.Error(), "artifactProducer.command") {
		t.Fatalf("expected an empty command rejection, got %v", err)
	}

	value = validDocument()
	value.VersionProvider = &Command{Command: []string{"version"}, ExtensionSchema: "relkit.extension/2"}
	if err := value.Validate(); err == nil || !strings.Contains(err.Error(), "relkit.extension/1") {
		t.Fatalf("expected an extension schema rejection, got %v", err)
	}

	value = validDocument()
	value.ArtifactProducer.TimeoutSeconds = -1
	if err := value.Validate(); err == nil || !strings.Contains(err.Error(), "timeoutSeconds") {
		t.Fatalf("expected a timeout rejection, got %v", err)
	}

	value = validDocument()
	value.ArtifactProducer.EnvAllowlist = []string{"TOKEN", "TOKEN"}
	if err := value.Validate(); err == nil || !strings.Contains(err.Error(), "envAllowlist") {
		t.Fatalf("expected an env allowlist rejection, got %v", err)
	}
}

// TestSiteDeclaration: omitting it is a deliberate opt-out; declaring an empty
// block is a typo and must fail closed, because a silently empty site block is
// exactly how a product drops out of the rendered catalog unnoticed.
func TestSiteDeclaration(t *testing.T) {
	value := validDocument()
	if value.Validate() != nil {
		t.Fatalf("site is optional: %v", value.Validate())
	}

	value = validDocument()
	value.Site = &Site{}
	if err := value.Validate(); err == nil || !strings.Contains(err.Error(), "site needs at least one") {
		t.Fatalf("empty site block must fail closed, got %v", err)
	}

	value = validDocument()
	value.Site = &Site{Title: "Demo"}
	if err := value.Validate(); err != nil {
		t.Fatalf("title-only site must be valid: %v", err)
	}

	value = validDocument()
	value.Site = &Site{Homepage: "https://example.com/demo"}
	if err := value.Validate(); err != nil {
		t.Fatalf("homepage-only site must be valid: %v", err)
	}
}

// TestLoadRejectsRetiredControlPlaneFields pins the strict decoder: an old
// project file that still names a token, a pinned relkit version, an operator
// or a resolution must stop the load instead of being silently ignored.
func TestLoadRejectsRetiredControlPlaneFields(t *testing.T) {
	for _, field := range []string{
		`"token":"secret",`,
		`"relkitVersion":"v1.0.0",`,
		`"operatorRef":"prod",`,
		`"trustFingerprint":"` + testRootPublicKey + `",`,
		`"roles":["updater-host"],`,
		`"capabilityFingerprint":{"osArch":[]},`,
	} {
		data := `{
			"schema":"relkit.project/1",
			"product":"demo",
			"server":"https://updates.example",
			"rootPublicKey":"` + testRootPublicKey + `",
			` + field + `
			"channels":["stable"]
		}`
		var value Document
		if err := document.Decode([]byte(data), &value); err == nil {
			t.Fatalf("field fragment %q unexpectedly accepted", field)
		}
	}
}

// TestSiteDecodesFromProjectDocument pins the on-disk spelling: products
// declare site next to channels in relkit.project.json.
func TestSiteDecodesFromProjectDocument(t *testing.T) {
	data := `{
		"schema":"relkit.project/1",
		"product":"demo",
		"server":"https://updates.example",
		"rootPublicKey":"` + testRootPublicKey + `",
		"channels":["stable"],
		"site":{"title":"Demo","description":"A demo product.","homepage":"https://example.com/demo"}
	}`
	var value Document
	if err := document.Decode([]byte(data), &value); err != nil {
		t.Fatal(err)
	}
	if err := value.Validate(); err != nil {
		t.Fatal(err)
	}
	if value.Site == nil || value.Site.Title != "Demo" || value.Site.Description != "A demo product." {
		t.Fatalf("site did not decode: %+v", value.Site)
	}
}
