package chain_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/shichao402/relkit/internal/chain"
	"github.com/shichao402/relkit/internal/model"
	"github.com/shichao402/relkit/internal/testutil"
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
			var fixture versionSelectFixture
			loadFixture(t, file, &fixture)
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
			var fixture reachabilityFixture
			loadFixture(t, file, &fixture)
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

func loadFixture(t *testing.T, path string, dest any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, dest); err != nil {
		t.Fatal(err)
	}
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
