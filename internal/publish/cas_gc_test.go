package publish

import (
	"strings"
	"testing"

	rupv2 "cnb.cool/shichao402/relkit/api/rup/v2"
	"cnb.cool/shichao402/relkit/internal/backends"
	"cnb.cool/shichao402/relkit/internal/config"
	"cnb.cool/shichao402/relkit/internal/model"
)

type casMem struct {
	typeOnlyBackend
	files   map[string][]byte
	deleted []string
}

func (b *casMem) Get(key string) ([]byte, error) {
	return b.files[key], nil
}

func (b *casMem) Delete(key string) error {
	b.deleted = append(b.deleted, key)
	delete(b.files, key)
	return nil
}

func testDigest(ch byte) string {
	return strings.Repeat(string(ch), 64)
}

func mustManifest(t *testing.T, version, digest string) []byte {
	t.Helper()
	raw, err := rupv2.MarshalManifest(&rupv2.Manifest{
		Schema:  rupv2.SchemaManifest,
		Product: "demo",
		Version: version,
		Artifacts: []*rupv2.Artifact{{
			Id: "app", Filename: "app.bin", Size: 1, Sha256: digest, Kind: "binary",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestSweepOrphanCASDeletesHashOnlyInPrunedVersion(t *testing.T) {
	oldH, newH := testDigest('a'), testDigest('b')
	oldKey, _ := model.CasKey(oldH)
	newKey, _ := model.CasKey(newH)
	backend := &casMem{files: map[string][]byte{
		model.ManifestKey("demo", "1.0.0"): mustManifest(t, "1.0.0", oldH),
		model.ManifestKey("demo", "2.0.0"): mustManifest(t, "2.0.0", newH),
		oldKey:                             []byte("old"),
		newKey:                             []byte("new"),
	}}
	cfg := &config.Config{Product: "demo", Channels: []string{"stable"}}
	published := &model.IndexDocument{
		Product:  "demo",
		Channel:  "stable",
		Versions: []*model.VersionNode{{Version: "2.0.0"}},
	}
	staged := &model.StagedDocument{
		Artifacts: []*model.StagedArtifact{{Sha256: newH}},
	}
	sweepOrphanCAS(cfg, published, []*model.VersionNode{{Version: "1.0.0"}}, staged, []backends.Backend{backend}, nil)
	if len(backend.deleted) != 1 || backend.deleted[0] != oldKey {
		t.Fatalf("deleted=%v want [%s]", backend.deleted, oldKey)
	}
	if _, ok := backend.files[newKey]; !ok {
		t.Fatal("live cas blob was deleted")
	}
}

func TestSweepOrphanCASKeepsHashStillOnOtherChannel(t *testing.T) {
	shared := testDigest('c')
	key, _ := model.CasKey(shared)
	devEnv, err := rupv2.MarshalEnvelope(&rupv2.Envelope{
		Schema:  rupv2.SchemaEnvelope,
		Payload: mustIndexPayload(t, "dev", "1.0.0"),
	})
	if err != nil {
		t.Fatal(err)
	}
	backend := &casMem{files: map[string][]byte{
		model.ManifestKey("demo", "1.0.0"): mustManifest(t, "1.0.0", shared),
		model.IndexKey("demo", "dev"):      devEnv,
		key:                                []byte("blob"),
	}}
	cfg := &config.Config{Product: "demo", Channels: []string{"stable", "dev"}}
	published := &model.IndexDocument{
		Product:  "demo",
		Channel:  "stable",
		Versions: []*model.VersionNode{{Version: "2.0.0"}},
	}
	staged := &model.StagedDocument{
		Artifacts: []*model.StagedArtifact{{Sha256: testDigest('d')}},
	}
	backend.files[model.ManifestKey("demo", "2.0.0")] = mustManifest(t, "2.0.0", testDigest('d'))
	sweepOrphanCAS(cfg, published, []*model.VersionNode{{Version: "1.0.0"}}, staged, []backends.Backend{backend}, nil)
	if len(backend.deleted) != 0 {
		t.Fatalf("deleted shared cas %v", backend.deleted)
	}
}

func mustIndexPayload(t *testing.T, channel, version string) []byte {
	t.Helper()
	raw, err := rupv2.MarshalIndex(&rupv2.Index{
		Schema:   rupv2.SchemaIndex,
		Product:  "demo",
		Channel:  channel,
		Sequence: 1,
		Versions: []*rupv2.VersionNode{{Version: version}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
