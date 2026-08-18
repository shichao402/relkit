package chain_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"cnb.cool/shichao402/relkit/internal/chain"
	"cnb.cool/shichao402/relkit/internal/model"
	"cnb.cool/shichao402/relkit/internal/testutil"
)

type versionSelectFixture struct {
	Name  string              `json:"name"`
	Index model.IndexDocument `json:"index"`
	Cases []struct {
		CurrentCode     int      `json:"currentCode"`
		ExpectTarget    *string  `json:"expectTarget"`
		ExpectPath      []string `json:"expectPath"`
		ExpectMandatory *bool    `json:"expectMandatory"`
	} `json:"cases"`
}

type reachabilityFixture struct {
	Name           string              `json:"name"`
	Index          model.IndexDocument `json:"index"`
	ExpectValid    bool                `json:"expectValid"`
	ExpectErrors   []string            `json:"expectErrors"`
	ExpectWarnings []string            `json:"expectWarnings"`
}

func TestVersionSelectConformance(t *testing.T) {
	root := filepath.Join(testutil.ConformanceRoot(t), "version-select")
	files, err := filepath.Glob(filepath.Join(root, "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		file := file
		t.Run(filepath.Base(file), func(t *testing.T) {
			fixture := loadVersionSelectFixture(t, file)
			for _, tc := range fixture.Cases {
				target := chain.SelectNextTarget(&fixture.Index, tc.CurrentCode)
				var targetVersion *string
				if target != nil {
					targetVersion = &target.Version
				}
				if !equalOptionalString(targetVersion, tc.ExpectTarget) {
					t.Fatalf("currentCode=%d target mismatch: got %v want %v", tc.CurrentCode, derefString(targetVersion), derefString(tc.ExpectTarget))
				}

				path := chain.ResolveUpgradePath(&fixture.Index, tc.CurrentCode)
				gotPath := make([]string, 0, len(path))
				for _, node := range path {
					gotPath = append(gotPath, node.Version)
				}
				if !equalStringSlices(gotPath, tc.ExpectPath) {
					t.Fatalf("currentCode=%d path mismatch: got %v want %v", tc.CurrentCode, gotPath, tc.ExpectPath)
				}

				if tc.ExpectMandatory != nil {
					got := chain.IsMandatory(&fixture.Index, tc.CurrentCode)
					if got != *tc.ExpectMandatory {
						t.Fatalf("currentCode=%d mandatory mismatch: got %v want %v", tc.CurrentCode, got, *tc.ExpectMandatory)
					}
				}
			}
		})
	}
}

func TestReachabilityConformance(t *testing.T) {
	root := filepath.Join(testutil.ConformanceRoot(t), "reachability")
	files, err := filepath.Glob(filepath.Join(root, "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		file := file
		t.Run(filepath.Base(file), func(t *testing.T) {
			fixture := loadReachabilityFixture(t, file)
			errors, warnings := chain.ValidateReachability(&fixture.Index)
			if !equalStringSlices(errors, fixture.ExpectErrors) {
				t.Fatalf("errors mismatch: got %v want %v", errors, fixture.ExpectErrors)
			}
			if !equalStringSlices(warnings, fixture.ExpectWarnings) {
				t.Fatalf("warnings mismatch: got %v want %v", warnings, fixture.ExpectWarnings)
			}
			if got := len(errors) == 0; got != fixture.ExpectValid {
				t.Fatalf("valid mismatch: got %v want %v", got, fixture.ExpectValid)
			}
		})
	}
}

func loadVersionSelectFixture(t *testing.T, path string) versionSelectFixture {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var raw struct {
		Name  string          `json:"name"`
		Index json.RawMessage `json:"index"`
		Cases []struct {
			CurrentCode     int      `json:"currentCode"`
			ExpectTarget    *string  `json:"expectTarget"`
			ExpectPath      []string `json:"expectPath"`
			ExpectMandatory *bool    `json:"expectMandatory"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	index, err := decodeLegacyIndex(raw.Index)
	if err != nil {
		t.Fatal(err)
	}
	return versionSelectFixture{
		Name:  raw.Name,
		Index: *index,
		Cases: raw.Cases,
	}
}

func loadReachabilityFixture(t *testing.T, path string) reachabilityFixture {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var raw struct {
		Name           string          `json:"name"`
		Index          json.RawMessage `json:"index"`
		ExpectValid    bool            `json:"expectValid"`
		ExpectErrors   []string        `json:"expectErrors"`
		ExpectWarnings []string        `json:"expectWarnings"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	index, err := decodeLegacyIndex(raw.Index)
	if err != nil {
		t.Fatal(err)
	}
	return reachabilityFixture{
		Name:           raw.Name,
		Index:          *index,
		ExpectValid:    raw.ExpectValid,
		ExpectErrors:   raw.ExpectErrors,
		ExpectWarnings: raw.ExpectWarnings,
	}
}

type legacyIndexDocument struct {
	Schema       string              `json:"schema"`
	Product      string              `json:"product"`
	Channel      string              `json:"channel"`
	Sequence     int64               `json:"sequence"`
	GeneratedAt  string              `json:"generatedAt"`
	MinSupported *int64              `json:"minSupported"`
	ExpiresAt    string              `json:"expiresAt"`
	Versions     []legacyVersionNode `json:"versions"`
}

type legacyVersionNode struct {
	Version    string            `json:"version"`
	Code       int64             `json:"code"`
	MinFrom    int64             `json:"minFrom"`
	ReleasedAt string            `json:"releasedAt"`
	Yanked     bool              `json:"yanked"`
	Notes      string            `json:"notes"`
	NotesURL   string            `json:"notesUrl"`
	Manifest   legacyManifestRef `json:"manifest"`
}

type legacyManifestRef struct {
	SHA256 string   `json:"sha256"`
	Size   int64    `json:"size"`
	URLs   []string `json:"urls"`
}

func decodeLegacyIndex(data []byte) (*model.IndexDocument, error) {
	var legacy legacyIndexDocument
	if err := json.Unmarshal(data, &legacy); err != nil {
		return nil, err
	}
	index := &model.IndexDocument{
		Schema:      model.SchemaIndex,
		Product:     legacy.Product,
		Channel:     legacy.Channel,
		Sequence:    legacy.Sequence,
		GeneratedAt: legacy.GeneratedAt,
		ExpiresAt:   legacy.ExpiresAt,
		Versions:    make([]*model.VersionNode, 0, len(legacy.Versions)),
	}
	if legacy.MinSupported != nil {
		index.MinSupported = *legacy.MinSupported
		index.HasMinSupported = true
	}
	for _, version := range legacy.Versions {
		index.Versions = append(index.Versions, &model.VersionNode{
			Version:    version.Version,
			Code:       version.Code,
			MinFrom:    version.MinFrom,
			ReleasedAt: version.ReleasedAt,
			Yanked:     version.Yanked,
			Notes:      version.Notes,
			NotesUrl:   version.NotesURL,
			Manifest: &model.ManifestRef{
				Sha256: version.Manifest.SHA256,
				Size:   version.Manifest.Size,
				Urls:   append([]string(nil), version.Manifest.URLs...),
			},
		})
	}
	return index, nil
}

func equalOptionalString(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func derefString(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
