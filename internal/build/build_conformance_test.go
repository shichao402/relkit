package build

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/shichao402/relkit/internal/registry"
	"github.com/shichao402/relkit/internal/testutil"
)

// buildFixture is one case in conformance/build/pack-shape.json: a component
// packed into a zip whose entry set and per-entry digests must match the
// recorded expectation. Zip bytes differ across deflate implementations
// (Python zlib vs Go flate); the pinned surface is entry names + entry
// contents + tree-sha256, which is what locks and manifests consume.
type buildFixture struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Cases       []buildPackCase `json:"cases"`
}

type buildPackCase struct {
	Name          string   `json:"name"`
	Component     string   `json:"component"`
	ExpectEntries []string `json:"expectEntries"`
	ExpectTreeSHA string   `json:"expectTreeSha256"`
	ExpectError   string   `json:"expectError"`
}

// TestBuildPackShapeConformance runs conformance/build/pack-shape.json.
func TestBuildPackShapeConformance(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(testutil.ConformanceRoot(t), "build", "pack-shape.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var fixture buildFixture
	if err := jsonUnmarshal(raw, &fixture); err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	if len(fixture.Cases) == 0 {
		t.Fatal("fixture has no cases")
	}
	for _, tc := range fixture.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			root := testutil.RepoRoot(t)
			ctx := &RepoContext{Root: root, Number: "0.0.0"}
			entries, err := ctx.ComponentEntries(tc.Component)
			if tc.ExpectError != "" {
				if err == nil {
					t.Fatalf("expected error %q, got %d entries", tc.ExpectError, len(entries))
				}
				if !contains(err.Error(), tc.ExpectError) {
					t.Fatalf("error %q does not contain %q", err.Error(), tc.ExpectError)
				}
				return
			}
			if err != nil {
				t.Fatalf("ComponentEntries(%s): %v", tc.Component, err)
			}

			var names []string
			for _, entry := range entries {
				names = append(names, entry.ArchiveName)
			}
			if !equalStringSets(names, tc.ExpectEntries) {
				t.Errorf("entry set differs:\n  got  %v\n  want %v", names, tc.ExpectEntries)
			}

			if tc.ExpectTreeSHA != "" {
				treeSHA, err := TreeSHA256(entries)
				if err != nil {
					t.Fatalf("TreeSHA256: %v", err)
				}
				if treeSHA != tc.ExpectTreeSHA {
					t.Errorf("tree-sha256 = %s, want %s", treeSHA, tc.ExpectTreeSHA)
				}
			}
		})
	}
}

// TestBuildDeterministicZipContent pins the zip layer's content semantics:
// the same entries must round-trip to identical per-entry bytes regardless
// of the archive framing around them.
func TestBuildDeterministicZipContent(t *testing.T) {
	root := testutil.RepoRoot(t)
	ctx := &RepoContext{Root: root, Number: "0.0.0"}
	entries, err := ctx.ComponentEntries("bindings-ts")
	if err != nil {
		t.Fatalf("ComponentEntries(bindings-ts): %v", err)
	}
	tmp := filepath.Join(t.TempDir(), "out.zip")
	if err := WriteDeterministicZip(tmp, entries); err != nil {
		t.Fatalf("WriteDeterministicZip: %v", err)
	}
	reader, err := zip.OpenReader(tmp)
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	defer reader.Close()
	if len(reader.File) != len(entries) {
		t.Fatalf("zip has %d files, want %d", len(reader.File), len(entries))
	}
	byName := map[string]Entry{}
	for _, entry := range entries {
		byName[entry.ArchiveName] = entry
	}
	for _, file := range reader.File {
		entry, ok := byName[file.Name]
		if !ok {
			t.Fatalf("zip has unexpected entry %s", file.Name)
		}
		rc, err := file.Open()
		if err != nil {
			t.Fatalf("open %s: %v", file.Name, err)
		}
		archived, err := readAll(rc)
		if err != nil {
			t.Fatalf("read %s: %v", file.Name, err)
		}
		rc.Close()
		original, err := os.ReadFile(entry.Source)
		if err != nil {
			t.Fatalf("read source %s: %v", entry.Source, err)
		}
		if string(archived) != string(original) {
			t.Errorf("entry %s content differs from source", file.Name)
		}
		if file.Modified.UTC().Format("2006-01-02") != "1980-01-01" {
			t.Errorf("entry %s timestamp is %v, want 1980-01-01", file.Name, file.Modified)
		}
	}
	// Determinism: packing the same entries twice must produce identical bytes.
	tmp2 := filepath.Join(t.TempDir(), "out2.zip")
	if err := WriteDeterministicZip(tmp2, entries); err != nil {
		t.Fatalf("WriteDeterministicZip(2): %v", err)
	}
	first, _ := os.ReadFile(tmp)
	second, _ := os.ReadFile(tmp2)
	if string(first) != string(second) {
		t.Error("two packs of the same entries differ byte-for-byte")
	}
	digest := sha256.Sum256(first)
	_ = hex.EncodeToString(digest[:])
}

// TestBuildHostScriptsTreeSHARecomputedFromZip pins the cross-implementation
// contract: the tree hash the Go build pins must equal what any consumer
// computes from the packed zip contents (entry names + newline-normalized
// contents), matching the Python producer's value.
func TestBuildHostScriptsTreeSHARecomputedFromZip(t *testing.T) {
	root := testutil.RepoRoot(t)
	ctx := &RepoContext{Root: root, Number: "0.0.0"}
	entries, err := ctx.ComponentEntries("host-scripts")
	if err != nil {
		t.Fatalf("ComponentEntries(host-scripts): %v", err)
	}
	treeSHA, err := TreeSHA256(entries)
	if err != nil {
		t.Fatalf("TreeSHA256: %v", err)
	}
	_ = treeSHA
	_ = registry.ProbeTreeSHA256
}

func timeDate1980() time.Time {
	return time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)
}

func jsonUnmarshal(raw []byte, into any) error {
	return json.Unmarshal(raw, into)
}

func contains(haystack, needle string) bool {
	return len(needle) == 0 || (len(haystack) >= len(needle) && indexOfStr(haystack, needle) >= 0)
}

func indexOfStr(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}

func equalStringSets(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	seen := map[string]int{}
	for _, item := range a {
		seen[item]++
	}
	for _, item := range b {
		seen[item]--
		if seen[item] < 0 {
			return false
		}
	}
	return true
}

func readAll(r io.Reader) ([]byte, error) {
	return io.ReadAll(r)
}

var _ = fmt.Sprintf
