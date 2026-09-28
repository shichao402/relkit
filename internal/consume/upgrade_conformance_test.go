package consume

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shichao402/relkit/internal/testutil"
)

// upgradeCase is one case in conformance/consume/upgrade-ci.json.
type upgradeCase struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Release     string            `json:"release"`
	Manifest    *ReleaseManifest  `json:"manifest"`
	SHA256SUMS  map[string]string `json:"sha256sums"`
	Previous    *Lock             `json:"previousLock"`
	ExpectLock  *struct {
		Schema             string   `json:"schema"`
		Release            string   `json:"release"`
		Commit             string   `json:"commit"`
		SourceModule       string   `json:"sourceModule"`
		SourceVersion      string   `json:"sourceVersion"`
		HostScriptsSHA256  string   `json:"hostScriptsSha256"`
		ArtifactComponents []string `json:"artifactComponents"`
		CLIURLsLen         int      `json:"cliUrlsLen"`
		ProtocolMin        int      `json:"protocolMin"`
		UpdaterIpcMin      int      `json:"updaterIpcMin"`
		UpdaterInArtifacts bool     `json:"updaterInArtifacts"`
	} `json:"expectLock"`
	ExpectError string `json:"expectError"`
}

type upgradeFixture struct {
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Cases       []upgradeCase `json:"cases"`
}

// TestConsumeUpgradeConformance runs conformance/consume/upgrade-ci.json:
// lock rebuild semantics shared by every consumer implementation.
func TestConsumeUpgradeConformance(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(testutil.ConformanceRoot(t), "consume", "upgrade-ci.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var fixture upgradeFixture
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	if len(fixture.Cases) == 0 {
		t.Fatal("fixture has no cases")
	}
	for _, tc := range fixture.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			previous := tc.Previous
			if previous == nil {
				// A case without a previousLock still exercises the rebuild
				// from nothing; hand UpgradeLock an empty prior.
				previous = &Lock{Schema: "relkit.consume/2"}
			}
			lock, err := UpgradeLock(tc.Release, tc.Manifest, tc.SHA256SUMS, previous)
			if tc.ExpectError != "" {
				if err == nil {
					t.Fatalf("expected error %q, got lock", tc.ExpectError)
				}
				if !strings.Contains(err.Error(), tc.ExpectError) {
					t.Fatalf("error %q does not contain %q", err.Error(), tc.ExpectError)
				}
				return
			}
			if err != nil {
				t.Fatalf("UpgradeLock: %v", err)
			}
			want := tc.ExpectLock
			if want == nil {
				t.Fatal("case has neither expectLock nor expectError")
			}
			if lock.Schema != want.Schema {
				t.Errorf("schema = %q, want %q", lock.Schema, want.Schema)
			}
			if lock.Release != want.Release {
				t.Errorf("release = %q, want %q", lock.Release, want.Release)
			}
			if lock.Commit != want.Commit {
				t.Errorf("commit = %q, want %q", lock.Commit, want.Commit)
			}
			if lock.Source == nil {
				t.Fatal("source block missing")
			}
			if lock.Source.Module != want.SourceModule {
				t.Errorf("source.module = %q, want %q", lock.Source.Module, want.SourceModule)
			}
			if lock.Source.Version != want.SourceVersion {
				t.Errorf("source.version = %q, want %q", lock.Source.Version, want.SourceVersion)
			}
			if lock.HostScriptsSHA256 != want.HostScriptsSHA256 {
				t.Errorf("hostScriptsSha256 = %q, want %q", lock.HostScriptsSHA256, want.HostScriptsSHA256)
			}
			var got []string
			for name := range lock.Artifacts {
				got = append(got, name)
			}
			sortStrings(got)
			if !equalStrings(got, want.ArtifactComponents) {
				t.Errorf("artifactComponents = %v, want %v", got, want.ArtifactComponents)
			}
			if want.UpdaterInArtifacts {
				if _, ok := lock.Artifacts["updater"]; !ok {
					t.Error("updater should be pinned in artifacts")
				}
			} else if _, ok := lock.Artifacts["updater"]; ok {
				t.Error("updater must not be pinned in artifacts (module channel)")
			}
			if want.ProtocolMin != 0 && lock.Protocol.Min != want.ProtocolMin {
				t.Errorf("protocol.min = %d, want %d", lock.Protocol.Min, want.ProtocolMin)
			}
			if want.UpdaterIpcMin != 0 && lock.UpdaterIPC.Min != want.UpdaterIpcMin {
				t.Errorf("updaterIpc.min = %d, want %d", lock.UpdaterIPC.Min, want.UpdaterIpcMin)
			}
			if want.CLIURLsLen > 0 {
				spec, err := lock.ArtifactSpecFor("cli", "windows-amd64")
				if err != nil {
					t.Fatalf("cli spec: %v", err)
				}
				if len(spec.URLs) != want.CLIURLsLen {
					t.Errorf("cli urls len = %d, want %d", len(spec.URLs), want.CLIURLsLen)
				}
			}
			// Round-trip: the rebuilt lock must parse back.
			if err := lock.WriteLock(filepath.Join(t.TempDir(), "relkit.lock.json")); err != nil {
				t.Fatalf("WriteLock: %v", err)
			}
		})
	}
}

func sortStrings(values []string) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}

func equalStrings(a, b []string) bool {
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
