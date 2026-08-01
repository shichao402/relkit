package selectors_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/shichao402/relkit/internal/model"
	"github.com/shichao402/relkit/internal/selectors"
	"github.com/shichao402/relkit/internal/testutil"
)

type selectorFixture struct {
	Name     string                 `json:"name"`
	Manifest model.ManifestDocument `json:"manifest"`
	Cases    []struct {
		ClientSelectors  map[string]string `json:"clientSelectors"`
		ExpectArtifactID *string           `json:"expectArtifactId"`
	} `json:"cases"`
}

func TestSelectorConformance(t *testing.T) {
	root := filepath.Join(testutil.ConformanceRoot(t), "selector")
	files, err := filepath.Glob(filepath.Join(root, "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		file := file
		t.Run(filepath.Base(file), func(t *testing.T) {
			fixture := loadSelectorFixture(t, file)
			for _, tc := range fixture.Cases {
				chosen := selectors.SelectArtifact(&fixture.Manifest, tc.ClientSelectors)
				var got *string
				if chosen != nil {
					got = &chosen.Id
				}
				if !equalSelectorString(got, tc.ExpectArtifactID) {
					t.Fatalf("selectors=%v mismatch: got %v want %v", tc.ClientSelectors, derefSelectorString(got), derefSelectorString(tc.ExpectArtifactID))
				}
			}
		})
	}
}

func loadSelectorFixture(t *testing.T, path string) selectorFixture {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var raw struct {
		Name     string          `json:"name"`
		Manifest json.RawMessage `json:"manifest"`
		Cases    []struct {
			ClientSelectors  map[string]string `json:"clientSelectors"`
			ExpectArtifactID *string           `json:"expectArtifactId"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	manifest, err := decodeLegacyManifest(raw.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	return selectorFixture{
		Name:     raw.Name,
		Manifest: *manifest,
		Cases:    raw.Cases,
	}
}

type legacyManifestDocument struct {
	Schema     string                   `json:"schema"`
	Product    string                   `json:"product"`
	Version    string                   `json:"version"`
	Code       int64                    `json:"code"`
	ReleasedAt string                   `json:"releasedAt"`
	Notes      string                   `json:"notes"`
	Artifacts  []legacyManifestArtifact `json:"artifacts"`
}

type legacyManifestArtifact struct {
	ID        string            `json:"id"`
	Filename  string            `json:"filename"`
	Size      int64             `json:"size"`
	SHA256    string            `json:"sha256"`
	Kind      string            `json:"kind"`
	Selectors map[string]string `json:"selectors"`
	URLs      []string          `json:"urls"`
	Meta      map[string]string `json:"meta"`
}

func decodeLegacyManifest(data []byte) (*model.ManifestDocument, error) {
	var legacy legacyManifestDocument
	if err := json.Unmarshal(data, &legacy); err != nil {
		return nil, err
	}
	manifest := &model.ManifestDocument{
		Schema:     model.SchemaManifest,
		Product:    legacy.Product,
		Version:    legacy.Version,
		Code:       legacy.Code,
		ReleasedAt: legacy.ReleasedAt,
		Notes:      legacy.Notes,
		Artifacts:  make([]*model.ManifestArtifact, 0, len(legacy.Artifacts)),
	}
	for _, artifact := range legacy.Artifacts {
		metaAny := map[string]any{}
		for key, value := range artifact.Meta {
			metaAny[key] = value
		}
		metaEntries, err := model.MetaEntriesFromAnyMap(metaAny)
		if err != nil {
			return nil, err
		}
		manifest.Artifacts = append(manifest.Artifacts, &model.ManifestArtifact{
			Id:        artifact.ID,
			Filename:  artifact.Filename,
			Size:      artifact.Size,
			Sha256:    artifact.SHA256,
			Kind:      artifact.Kind,
			Selectors: model.SelectorsFromMap(artifact.Selectors),
			Urls:      append([]string(nil), artifact.URLs...),
			Meta:      metaEntries,
		})
	}
	return manifest, nil
}

func equalSelectorString(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func derefSelectorString(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}
