package publish

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	rupv2 "github.com/shichao402/relkit/api/rup/v2"
	"github.com/shichao402/relkit/internal/backends"
	"github.com/shichao402/relkit/internal/config"
	"github.com/shichao402/relkit/internal/envelope"
	"github.com/shichao402/relkit/internal/keys"
	"github.com/shichao402/relkit/internal/model"
	"github.com/shichao402/relkit/internal/webmeta"
)

// unpublishMem is a full in-memory backend: it stores every key it receives so
// the index, manifests, web documents and deletes are all observable.
type unpublishMem struct {
	files   map[string][]byte
	deleted []string
}

func (b *unpublishMem) Name() string                               { return "mem" }
func (b *unpublishMem) Type() string                               { return "mem" }
func (b *unpublishMem) Describe() string                           { return "mem" }
func (b *unpublishMem) URLsAreLive() bool                          { return true }
func (b *unpublishMem) Writable() bool                             { return true }
func (b *unpublishMem) HostsBrowse() bool                          { return false }
func (b *unpublishMem) URLFor(key string) *string                  { s := "https://mem.invalid/" + key; return &s }
func (b *unpublishMem) Probe(rawURL string) (bool, *int64, string) { return false, nil, "" }
func (b *unpublishMem) PutArtifact(string, string) ([]string, error) {
	return nil, nil
}
func (b *unpublishMem) PutImmutable(data []byte, key string) ([]string, error) {
	b.files[key] = data
	return []string{"https://mem.invalid/" + key}, nil
}
func (b *unpublishMem) PutPointer(data []byte, key string) ([]string, error) {
	b.files[key] = data
	return []string{"https://mem.invalid/" + key}, nil
}
func (b *unpublishMem) Get(key string) ([]byte, error) {
	return b.files[key], nil
}
func (b *unpublishMem) Delete(key string) error {
	b.deleted = append(b.deleted, key)
	delete(b.files, key)
	return nil
}

var _ backends.Deleter = (*unpublishMem)(nil)

// newUnpublishFixture writes a product relkit.json with a real signing key and
// returns the loaded config. The config declares a "mem" backend that tests
// replace before calling the internal kernel below, because the mem type is
// not in the backends.Create registry.
func newUnpublishFixture(t *testing.T) (*config.Config, string) {
	t.Helper()
	dir := t.TempDir()
	seed, err := keys.GenerateSeed()
	if err != nil {
		t.Fatal(err)
	}
	privDoc := keys.PrivateKeyDocument("k1", seed)
	privBytes, err := rupv2.MarshalPrivateKey(&privDoc)
	if err != nil {
		t.Fatal(err)
	}
	privPath := filepath.Join(dir, "k1.private.pb")
	if err := os.WriteFile(privPath, privBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	public := keys.PublicKey(seed)
	cfgDoc := map[string]any{
		"product":        "demo",
		"defaultChannel": "stable",
		"channels":       []any{"stable"},
		"codeStrategy":   "explicit",
		"signing": map[string]any{
			"keyId":          "k1",
			"privateKeyPath": "k1.private.pb",
			"publicKeys": []any{map[string]any{
				"keyId":           "k1",
				"publicKeyBase64": base64.StdEncoding.EncodeToString(public),
			}},
		},
		"backends": map[string]any{
			"mem": map[string]any{"type": "mem"},
		},
		"publishTo": []any{"mem"},
	}
	cfgBytes, err := json.MarshalIndent(cfgDoc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "relkit.json"), cfgBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(filepath.Join(dir, "relkit.json"))
	if err != nil {
		t.Fatal(err)
	}
	return cfg, dir
}

// runUnpublishOn drives the injectable kernel with the given backend, so tests
// never depend on the backends.Create type registry.
func runUnpublishOn(t *testing.T, cfg *config.Config, backend *unpublishMem, version string, dryRun bool) *model.IndexDocument {
	t.Helper()
	index, err := unpublishOnBackends(cfg, version, []backends.Backend{backend}, dryRun, func(string) {})
	if err != nil {
		t.Fatalf("unpublish %s: %v", version, err)
	}
	return index
}

func runUnpublishErr(t *testing.T, cfg *config.Config, backend *unpublishMem, version string, dryRun bool) error {
	t.Helper()
	_, err := unpublishOnBackends(cfg, version, []backends.Backend{backend}, dryRun, func(string) {})
	return err
}

func seedSignedIndex(t *testing.T, backend *unpublishMem, cfg *config.Config, sequence int, versions ...*model.VersionNode) {
	t.Helper()
	index, err := model.NewIndex("demo", "stable", sequence, versions, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	signers, err := cfg.LoadSigners()
	if err != nil {
		t.Fatal(err)
	}
	env, err := envelope.Seal(index, signers)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := rupv2.MarshalEnvelope(env)
	if err != nil {
		t.Fatal(err)
	}
	backend.files[model.IndexKey("demo", "stable")] = raw
}

func seedNode(version string, code int64, digest string) *model.VersionNode {
	return &model.VersionNode{
		Version: version, Code: code, MinFrom: 0, ReleasedAt: "2026-01-01T00:00:00Z",
		Manifest: &model.ManifestRef{
			Sha256: digest, Size: 10,
			Urls: []string{"https://mem.invalid/" + model.ManifestKey("demo", version)},
		},
	}
}

func TestUnpublishRemovesNodeAndRewritesWebDocs(t *testing.T) {
	cfg, _ := newUnpublishFixture(t)
	backend := &unpublishMem{files: map[string][]byte{}}
	seedSignedIndex(t, backend, cfg, 7, seedNode("1.0.0", 1, testDigest('a')), seedNode("2.0.0", 2, testDigest('b')))
	backend.files[model.ManifestKey("demo", "1.0.0")] = mustManifest(t, "1.0.0", testDigest('a'))
	backend.files[model.ManifestKey("demo", "2.0.0")] = mustManifest(t, "2.0.0", testDigest('b'))

	out := runUnpublishOn(t, cfg, backend, "2.0.0", false)
	if len(out.Versions) != 1 || out.Versions[0].Version != "1.0.0" {
		t.Fatalf("versions=%v", out.Versions)
	}
	if out.Sequence != 8 {
		t.Fatalf("sequence=%d want 8", out.Sequence)
	}

	raw := backend.files[model.IndexKey("demo", "stable")]
	if raw == nil {
		t.Fatal("index pointer was not written")
	}
	env, err := rupv2.UnmarshalEnvelope(raw)
	if err != nil {
		t.Fatal(err)
	}
	trusted, _ := cfg.TrustedPublicKeys()
	reopened, err := envelope.OpenEnvelope(env, trusted)
	if err != nil {
		t.Fatal(err)
	}
	for _, node := range reopened.Versions {
		if node.Version == "2.0.0" {
			t.Fatal("committed index still lists 2.0.0")
		}
	}

	latestDoc, err := webmeta.UnmarshalLatest(backend.files[webmeta.LatestKey("demo", "stable")])
	if err != nil {
		t.Fatal(err)
	}
	if latestDoc.Version != "1.0.0" {
		t.Fatalf("latest version=%s want 1.0.0", latestDoc.Version)
	}
	channelDoc, err := webmeta.UnmarshalChannel(backend.files[webmeta.ChannelKey("demo", "stable")])
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range channelDoc.Versions {
		if entry.Version == "2.0.0" {
			t.Fatal("channel still lists 2.0.0")
		}
	}

	for _, key := range backend.deleted {
		if key == webmeta.ReleaseKey("demo", "stable", "2.0.0") {
			return
		}
	}
	t.Fatalf("release doc was not deleted; deleted=%v", backend.deleted)
}

func TestUnpublishDryRunTouchesNothing(t *testing.T) {
	cfg, _ := newUnpublishFixture(t)
	backend := &unpublishMem{files: map[string][]byte{}}
	seedSignedIndex(t, backend, cfg, 7, seedNode("1.0.0", 1, testDigest('a')), seedNode("2.0.0", 2, testDigest('b')))
	before := len(backend.files)

	runUnpublishOn(t, cfg, backend, "2.0.0", true)
	if len(backend.files) != before {
		t.Fatalf("dry run wrote files: before=%d after=%d", before, len(backend.files))
	}
	if len(backend.deleted) != 0 {
		t.Fatalf("dry run deleted: %v", backend.deleted)
	}
}

func TestUnpublishUnknownVersionFails(t *testing.T) {
	cfg, _ := newUnpublishFixture(t)
	backend := &unpublishMem{files: map[string][]byte{}}
	seedSignedIndex(t, backend, cfg, 7, seedNode("1.0.0", 1, testDigest('a')), seedNode("2.0.0", 2, testDigest('b')))

	err := runUnpublishErr(t, cfg, backend, "9.9.9", false)
	if err == nil {
		t.Fatal("expected error for unknown version")
	}
	if len(backend.deleted) != 0 {
		t.Fatalf("failed unpublish deleted files: %v", backend.deleted)
	}
}

func TestUnpublishLastVersionRefused(t *testing.T) {
	cfg, _ := newUnpublishFixture(t)
	backend := &unpublishMem{files: map[string][]byte{}}

	seedSignedIndex(t, backend, cfg, 7, seedNode("1.0.0", 1, testDigest('a')))
	backend.files[model.ManifestKey("demo", "1.0.0")] = mustManifest(t, "1.0.0", testDigest('a'))
	if err := runUnpublishErr(t, cfg, backend, "1.0.0", false); err == nil {
		t.Fatal("expected refusal when removing the last remaining version")
	}
}

func TestUnpublishRemovesMiddleNodeKeepsReachability(t *testing.T) {
	cfg, _ := newUnpublishFixture(t)
	backend := &unpublishMem{files: map[string][]byte{}}
	seedSignedIndex(t, backend, cfg, 9,
		seedNode("1.0.0", 1, testDigest('a')),
		seedNode("1.5.0", 2, testDigest('m')),
		seedNode("2.0.0", 3, testDigest('b')))
	backend.files[model.ManifestKey("demo", "1.0.0")] = mustManifest(t, "1.0.0", testDigest('a'))
	backend.files[model.ManifestKey("demo", "1.5.0")] = mustManifest(t, "1.5.0", testDigest('m'))
	backend.files[model.ManifestKey("demo", "2.0.0")] = mustManifest(t, "2.0.0", testDigest('b'))

	out := runUnpublishOn(t, cfg, backend, "1.5.0", false)
	if len(out.Versions) != 2 {
		t.Fatalf("versions=%v", out.Versions)
	}
	if out.Sequence != 10 {
		t.Fatalf("sequence=%d want 10", out.Sequence)
	}
	channelDoc, err := webmeta.UnmarshalChannel(backend.files[webmeta.ChannelKey("demo", "stable")])
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range channelDoc.Versions {
		if entry.Version == "1.5.0" {
			t.Fatal("channel still lists 1.5.0")
		}
	}
}
