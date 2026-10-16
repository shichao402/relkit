package directory

import (
	"crypto/ed25519"
	"strings"
	"testing"

	rupv2 "github.com/shichao402/relkit/api/rup/v2"
	"github.com/shichao402/relkit/internal/backends"
	"github.com/shichao402/relkit/internal/envelope"
	"github.com/shichao402/relkit/internal/model"
)

type testBackend struct {
	name string
	data []byte
}

func (b *testBackend) Name() string                                  { return b.name }
func (b *testBackend) Type() string                                  { return "test" }
func (b *testBackend) Describe() string                              { return b.name }
func (b *testBackend) URLsAreLive() bool                             { return false }
func (b *testBackend) Writable() bool                                { return true }
func (b *testBackend) PutArtifact(string, string) ([]string, error)  { return nil, nil }
func (b *testBackend) PutImmutable([]byte, string) ([]string, error) { return nil, nil }
func (b *testBackend) PutPointer(data []byte, _ string) ([]string, error) {
	b.data = data
	return nil, nil
}
func (b *testBackend) Get(string) ([]byte, error)          { return b.data, nil }
func (b *testBackend) URLFor(string) *string               { return nil }
func (b *testBackend) Probe(string) (bool, *int64, string) { return false, nil, "" }
func (b *testBackend) HostsBrowse() bool                   { return false }

func signedDirectory(t *testing.T, signer envelope.Signer, sequence int64) []byte {
	t.Helper()
	value := &model.UpdateDirectory{
		Schema:            model.SchemaDirectory,
		Product:           "demo",
		DirectorySequence: sequence,
		UpdatedAt:         "2026-10-16T00:00:00Z",
		Services: []*model.DirectoryService{
			{Id: "primary", Priority: 1, IndexUrl: "https://example.invalid/index.pb"},
		},
	}
	sealed, err := envelope.SealDirectory(value, []envelope.Signer{signer})
	if err != nil {
		t.Fatal(err)
	}
	data, err := rupv2.MarshalEnvelope(sealed)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestReadBaseSequenceRequiresByteIdenticalBackends(t *testing.T) {
	signer := envelope.Signer{KeyID: "demo", Seed: make([]byte, ed25519.SeedSize)}
	trusted := map[string]ed25519.PublicKey{"demo": signer.PublicKey()}
	data := signedDirectory(t, signer, 7)
	backends := []backends.Backend{
		&testBackend{name: "one", data: data},
		&testBackend{name: "two", data: append([]byte(nil), data...)},
	}
	sequence, err := readBaseSequence(backends, model.DirectoryKey("demo"), "demo", trusted, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	if sequence != 7 {
		t.Fatalf("sequence = %d, want 7", sequence)
	}
}

func TestReadBaseSequenceRejectsMissingOrDifferentBackend(t *testing.T) {
	signer := envelope.Signer{KeyID: "demo", Seed: make([]byte, ed25519.SeedSize)}
	trusted := map[string]ed25519.PublicKey{"demo": signer.PublicKey()}
	data := signedDirectory(t, signer, 7)
	cases := []struct {
		name string
		data []byte
	}{
		{name: "missing", data: nil},
		{name: "different", data: signedDirectory(t, signer, 8)},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			backends := []backends.Backend{
				&testBackend{name: "one", data: data},
				&testBackend{name: "two", data: test.data},
			}
			_, err := readBaseSequence(backends, model.DirectoryKey("demo"), "demo", trusted, func(string) {})
			if err == nil || !strings.Contains(err.Error(), "disagree") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
