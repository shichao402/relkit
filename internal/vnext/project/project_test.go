package project

import (
	"strings"
	"testing"

	"github.com/shichao402/relkit/internal/vnext/contracts"
	"github.com/shichao402/relkit/internal/vnext/document"
)

const testFingerprint = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func validDocument() Document {
	return Document{
		Schema:           Schema,
		Product:          "example-product",
		OperatorRef:      "production",
		TrustFingerprint: testFingerprint,
		Roles:            []Role{RolePublisher, RoleUpdaterHost},
		ArtifactProducer: &Command{
			Command:         []string{"./scripts/build"},
			ExtensionSchema: "relkit.extension/1",
		},
		CapabilityFingerprint: CapabilityFingerprint{
			OSArch:   []string{"windows-amd64"},
			Facades:  []string{"rust"},
			Topology: "self-update",
			RequiredContracts: []contracts.Capability{
				{Domain: contracts.DomainUpdaterIPC, Peer: "updater", Min: 3, Max: 3},
			},
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

func TestPublisherRequiresProducer(t *testing.T) {
	value := validDocument()
	value.ArtifactProducer = nil
	if err := value.Validate(); err == nil || !strings.Contains(err.Error(), "requires artifactProducer") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRoleAndSubscriptionCombinationsFailClosed(t *testing.T) {
	tests := []struct {
		name  string
		patch func(*Document)
		want  string
	}{
		{
			name: "updater host without facade",
			patch: func(value *Document) {
				value.CapabilityFingerprint.Facades = nil
			},
			want: "updater-host role requires at least one facade",
		},
		{
			name: "projection role without projection",
			patch: func(value *Document) {
				value.Roles = append(value.Roles, RoleUpdaterProjection)
			},
			want: "updater-projection role requires at least one projection",
		},
		{
			name: "runtime profile publication without publisher",
			patch: func(value *Document) {
				value.Roles = []Role{RoleUpdaterHost}
				value.ArtifactProducer = nil
				value.RuntimeProfilePublication = &RuntimeProfilePublication{
					RootFingerprint: testFingerprint,
					RUPSchemaSet:    "rup.v2",
				}
			},
			want: "runtimeProfilePublication requires publisher role",
		},
		{
			name: "subscription missing from capability fingerprint",
			patch: func(value *Document) {
				value.Roles = append(value.Roles, RoleSubscriber)
				value.RuntimeProfileSubscriptions = []Subscription{{
					ProductRef:      "managed-product",
					RootFingerprint: testFingerprint,
				}}
			},
			want: "capabilityFingerprint.subscriptions must match",
		},
		{
			name: "fingerprint subscription without runtime profile",
			patch: func(value *Document) {
				value.CapabilityFingerprint.Subscriptions = []string{"managed-product"}
			},
			want: "capabilityFingerprint.subscriptions must match",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := validDocument()
			test.patch(&value)
			if err := value.Validate(); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestStrictLoadRejectsSecretAndVersionFields(t *testing.T) {
	for _, field := range []string{`"token":"secret",`, `"relkitVersion":"v1.0.0",`} {
		data := `{
			"schema":"relkit.project/1",
			"product":"demo",
			"operatorRef":"prod",
			"trustFingerprint":"` + testFingerprint + `",
			` + field + `
			"roles":["updater-host"],
			"capabilityFingerprint":{
				"osArch":[],"facades":[],"projections":[],"topology":"self-update",
				"subscriptions":[],"requiredContracts":[]
			}
		}`
		var value Document
		if err := document.Decode([]byte(data), &value); err == nil {
			t.Fatalf("field fragment %q unexpectedly accepted", field)
		}
	}
}

// TestSiteDeclaration: the vNext replacement for the legacy relkit.json "site"
// block. Omitting it is a deliberate opt-out; declaring an empty block is a
// typo and must fail closed, because a silently empty site block is exactly
// how a product drops out of the rendered catalog without anyone noticing.
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

// TestSiteDecodesFromProjectDocument pins the on-disk spelling: products
// declare site next to channels in relkit.project.json.
func TestSiteDecodesFromProjectDocument(t *testing.T) {
	data := `{
		"schema":"relkit.project/1",
		"product":"demo",
		"operatorRef":"prod",
		"trustFingerprint":"` + testFingerprint + `",
		"roles":["updater-host"],
		"site":{"title":"Demo","description":"A demo product.","homepage":"https://example.com/demo"},
		"capabilityFingerprint":{
			"osArch":["windows-amd64"],"facades":["rust"],"projections":[],"topology":"self-update",
			"subscriptions":[],"requiredContracts":[]
		}
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
