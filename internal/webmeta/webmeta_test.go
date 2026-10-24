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

// The human-facing list is exactly what the manifest lists: every install
// artifact, no selector-based filtering.
func TestArtifactsFromManifestKeepsEveryInstallArtifact(t *testing.T) {
	manifest := &rupv2.Manifest{
		Schema: rupv2.SchemaManifest,
		Artifacts: []*rupv2.Artifact{
			install(rupv2.ArtifactKind_ARTIFACT_KIND_INSTALLER, "setup-exe", "os", "windows", "arch", "amd64"),
			install(rupv2.ArtifactKind_ARTIFACT_KIND_BINARY, "app-darwin", "os", "darwin", "arch", "arm64"),
			install(rupv2.ArtifactKind_ARTIFACT_KIND_BLOB, "manifest-blob", "component", "manifest"),
		},
	}
	got := ArtifactsFromManifest(manifest)
	if len(got) != 3 {
		t.Fatalf("artifacts=%d want 3", len(got))
	}
	for _, a := range got {
		if a.Kind == "payload" {
			t.Fatalf("payload artifact %s leaked into the human-facing list", a.ID)
		}
	}
}

// Payloads are protocol-internal: the engine stages them, a human never
// downloads them from the release page.
func TestArtifactsFromManifestDropsPayloads(t *testing.T) {
	manifest := &rupv2.Manifest{
		Schema: rupv2.SchemaManifest,
		Artifacts: []*rupv2.Artifact{
			install(rupv2.ArtifactKind_ARTIFACT_KIND_BINARY, "app-windows", "os", "windows", "arch", "amd64"),
			{Id: "payload", Kind: rupv2.ArtifactKind_ARTIFACT_KIND_PAYLOAD, Selectors: selectors("os", "windows")},
			{Id: "payload-2", Kind: rupv2.ArtifactKind_ARTIFACT_KIND_PAYLOAD},
		},
	}
	got := ArtifactsFromManifest(manifest)
	if len(got) != 1 || got[0].ID != "app-windows" {
		t.Fatalf("artifacts=%+v want only app-windows", got)
	}
}

// An old "audience" selector in a manifest is inert data, not a filter: it is
// carried through with the rest of the selectors and changes nothing.
func TestArtifactsFromManifestIgnoresAudienceSelector(t *testing.T) {
	manifest := &rupv2.Manifest{
		Schema: rupv2.SchemaManifest,
		Artifacts: []*rupv2.Artifact{
			install(rupv2.ArtifactKind_ARTIFACT_KIND_BINARY, "app-windows", "os", "windows", "audience", "runtime"),
			install(rupv2.ArtifactKind_ARTIFACT_KIND_INSTALLER, "setup-exe", "os", "windows", "audience", "user"),
		},
	}
	got := ArtifactsFromManifest(manifest)
	if len(got) != 2 {
		t.Fatalf("artifacts=%d want 2: audience is no longer a filter", len(got))
	}
	if got[0].Selectors["audience"] != "runtime" {
		t.Fatalf("selectors were not carried through: %v", got[0].Selectors)
	}
}

func TestArtifactsFromManifestNilManifest(t *testing.T) {
	if got := ArtifactsFromManifest(nil); got != nil {
		t.Fatalf("nil manifest must project nothing, got %+v", got)
	}
}
