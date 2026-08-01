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
			var fixture selectorFixture
			loadSelectorFixture(t, file, &fixture)
			for _, tc := range fixture.Cases {
				chosen := selectors.SelectArtifact(&fixture.Manifest, tc.ClientSelectors)
				var got *string
				if chosen != nil {
					got = &chosen.ID
				}
				if !equalSelectorString(got, tc.ExpectArtifactID) {
					t.Fatalf("selectors=%v mismatch: got %v want %v", tc.ClientSelectors, derefSelectorString(got), derefSelectorString(tc.ExpectArtifactID))
				}
			}
		})
	}
}

func loadSelectorFixture(t *testing.T, path string, dest any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, dest); err != nil {
		t.Fatal(err)
	}
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
