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
