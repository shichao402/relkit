package consume_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/shichao402/relkit/internal/consume"
	"github.com/shichao402/relkit/internal/testutil"
)

// fixtureCase mirrors conformance/consume/install-basics.json.
type fixtureCase struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Lock        json.RawMessage `json:"lock"`
	ExpectParse *struct {
		Schema             string   `json:"schema"`
		Release            string   `json:"release"`
		ArtifactComponents []string `json:"artifactComponents"`
		CliWindowsAmd64URL string   `json:"cliWindowsAmd64URL"`
		SourceModule       string   `json:"sourceModule"`
	} `json:"expectParse"`
}

type fixtureFile struct {
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Cases       []fixtureCase `json:"cases"`
}

func TestConsumeLockConformance(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(testutil.ConformanceRoot(t), "consume", "install-basics.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture fixtureFile
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	for _, tc := range fixture.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			lock, err := consume.ParseLock(tc.Lock)
			if tc.ExpectParse == nil {
				if err == nil {
					t.Fatalf("expected parse failure, got %+v", lock)
				}
				return
			}
			if err != nil {
				t.Fatalf("parse failed: %v", err)
			}
			if lock.Schema != tc.ExpectParse.Schema {
				t.Errorf("schema = %q, want %q", lock.Schema, tc.ExpectParse.Schema)
			}
			if lock.Release != tc.ExpectParse.Release {
				t.Errorf("release = %q, want %q", lock.Release, tc.ExpectParse.Release)
			}
			var names []string
			for name := range lock.Artifacts {
				names = append(names, name)
			}
			sort.Strings(names)
			if fmt.Sprint(names) != fmt.Sprint(tc.ExpectParse.ArtifactComponents) {
				t.Errorf("artifactComponents = %v, want %v", names, tc.ExpectParse.ArtifactComponents)
			}
			if tc.ExpectParse.CliWindowsAmd64URL != "" {
				spec, err := lock.ArtifactSpecFor("cli", "windows-amd64")
				if err != nil {
					t.Fatalf("cli spec: %v", err)
				}
				if spec.URLs[0] != tc.ExpectParse.CliWindowsAmd64URL {
					t.Errorf("cli url = %q, want %q", spec.URLs[0], tc.ExpectParse.CliWindowsAmd64URL)
				}
			}
			if tc.ExpectParse.SourceModule != "" {
				if lock.Source == nil || lock.Source.Module != tc.ExpectParse.SourceModule {
					t.Errorf("source module mismatch: %+v", lock.Source)
				}
			}
		})
	}
}
