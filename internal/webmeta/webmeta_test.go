package webmeta

import (
	"testing"

	rupv2 "github.com/shichao402/relkit/api/rup/v2"
)

func selectors(pairs ...string) []*rupv2.Selector {
	out := make([]*rupv2.Selector, 0, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		out = append(out, &rupv2.Selector{Key: pairs[i], Value: pairs[i+1]})
	}
	return out
}

func install(kind rupv2.ArtifactKind, id string, sels ...string) *rupv2.Artifact {
	return &rupv2.Artifact{
		Id:        id,
		Filename:  id + ".bin",
		Size:      1,
		Sha256:    "deadbeef",
		Kind:      kind,
		Selectors: selectors(sels...),
		Urls:      []string{"https://mem.invalid/" + id},
	}
}

// A manifest with no audience selectors at all keeps the full artifact list:
// older products and test fixtures predate the convention, and the filter
// must never empty a page that predates it.
func TestArtifactsFromManifestNoAudienceKeepsAll(t *testing.T) {
	manifest := &rupv2.Manifest{
		Schema: rupv2.SchemaManifest,
		Artifacts: []*rupv2.Artifact{
			install(rupv2.ArtifactKind_ARTIFACT_KIND_INSTALLER, "setup-exe", "os", "windows", "arch", "amd64"),
			install(rupv2.ArtifactKind_ARTIFACT_KIND_BINARY, "dec-darwin", "os", "darwin", "arch", "arm64"),
			install(rupv2.ArtifactKind_ARTIFACT_KIND_BLOB, "manifest-blob", "component", "manifest"),
		},
	}
	got := ArtifactsFromManifest(manifest)
	if len(got) != 3 {
		t.Fatalf("artifacts=%d want 3 (no-audience fallback keeps all)", len(got))
	}
}

// When any artifact carries an audience selector, only audience=user
// artifacts are projected: the web page lists what a human downloads
// (dec: the console installers), not the runtime fleet behind the
// update protocol.
func TestArtifactsFromManifestAudienceFiltersRuntime(t *testing.T) {
	manifest := &rupv2.Manifest{
		Schema: rupv2.SchemaManifest,
		Artifacts: []*rupv2.Artifact{
			install(rupv2.ArtifactKind_ARTIFACT_KIND_BINARY, "dec-windows", "os", "windows", "arch", "amd64", "component", "dec", "audience", "runtime"),
			install(rupv2.ArtifactKind_ARTIFACT_KIND_BINARY, "dec-darwin", "os", "darwin", "arch", "arm64", "component", "dec", "audience", "runtime"),
			install(rupv2.ArtifactKind_ARTIFACT_KIND_BLOB, "manifest-blob", "component", "manifest", "audience", "runtime"),
			install(rupv2.ArtifactKind_ARTIFACT_KIND_INSTALLER, "console-exe", "os", "windows", "arch", "amd64", "component", "console", "audience", "user"),
			install(rupv2.ArtifactKind_ARTIFACT_KIND_INSTALLER, "console-dmg", "os", "darwin", "arch", "arm64", "component", "console", "audience", "user"),
			{Id: "payload", Kind: rupv2.ArtifactKind_ARTIFACT_KIND_PAYLOAD, Selectors: selectors("audience", "user")},
		},
	}
	got := ArtifactsFromManifest(manifest)
	if len(got) != 2 {
		t.Fatalf("artifacts=%d want 2 user-facing installers", len(got))
	}
	for _, a := range got {
		if a.Selectors["audience"] != "user" {
			t.Fatalf("projected artifact %s is not audience=user: %v", a.ID, a.Selectors)
		}
		if a.Kind == "payload" {
			t.Fatalf("payload artifact %s leaked into the human-facing list", a.ID)
		}
	}
}

// A manifest that labels every artifact audience=runtime is a product with
// nothing human-downloadable; the projection is an empty list, which is the
// honest answer, not a bug. The unmarshal layer refuses empty lists for
// latest/, so this shape only appears in release docs.
func TestArtifactsFromManifestAllRuntimeYieldsEmpty(t *testing.T) {
	manifest := &rupv2.Manifest{
		Schema: rupv2.SchemaManifest,
		Artifacts: []*rupv2.Artifact{
			install(rupv2.ArtifactKind_ARTIFACT_KIND_BINARY, "dec-windows", "os", "windows", "arch", "amd64", "audience", "runtime"),
			install(rupv2.ArtifactKind_ARTIFACT_KIND_BLOB, "manifest-blob", "component", "manifest", "audience", "runtime"),
		},
	}
	got := ArtifactsFromManifest(manifest)
	if len(got) != 0 {
		t.Fatalf("artifacts=%d want 0 for an all-runtime manifest", len(got))
	}
}