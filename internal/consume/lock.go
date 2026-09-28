// Package consume implements the product-side consumption pipeline that
// ADR 0017 moves from scripts/host/relkit_consume.py into the CLI: lock
// parsing (consume/2 and consume/3), stack detection, checksum-pinned
// downloads, atomic installs, and verification. During the dual-implementation
// window this package must stay behaviorally identical to the Python
// consumer; conformance fixtures under conformance/consume/ pin both.
package consume

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// SchemaV2 is the legacy lock schema produced by relkit releases up to
// v0.4.x. Read-only support; install writes only consume/3 locks.
const SchemaV2 = "relkit.consume/2"

// SchemaV3 is the lock schema introduced by ADR 0017: a source block (module
// channel) plus an artifacts block reduced to SDK zips and the prebuilt-CLI
// fallback. The updater exits the artifacts block (module channel covers it).
const SchemaV3 = "relkit.consume/3"

// SourceBlock pins the module channel for CLI/updater/facade consumption.
type SourceBlock struct {
	Module  string `json:"module"`
	Version string `json:"version"`
	H1      string `json:"h1"`
	Commit  string `json:"commit"`
}

// ArtifactSpec is one pinned artifact: a list of mirror URLs (GitHub primary,
// CNB fallback) plus the sha256 of the bytes.
type ArtifactSpec struct {
	URLs   []string `json:"urls"`
	SHA256 string   `json:"sha256"`
}

// ArtifactTargetSpec is one pinned artifact for a specific host target.
type ArtifactTargetSpec struct {
	URLs   []string `json:"urls"`
	SHA256 string   `json:"sha256"`
}

// Lock is the parsed relkit lock file. Both consume/2 and consume/3 parse
// into this shape; V2ArtifactURL preserves the legacy single-url form for
// compatibility reads.
type Lock struct {
	Schema            string                     `json:"schema"`
	Release           string                     `json:"release"`
	Commit            string                     `json:"commit"`
	Source            *SourceBlock               `json:"source,omitempty"`
	ConsumerSHA256    string                     `json:"consumerSha256,omitempty"`
	HostScriptsSHA256 string                     `json:"hostScriptsSha256,omitempty"`
	Protocol          IntWindow                  `json:"protocol"`
	UpdaterIPC        IntWindow                  `json:"updaterIpc"`
	Artifacts         map[string]json.RawMessage `json:"artifacts"`

	// V2Artifacts holds consume/2 shaped entries (single url string) after a
	// successful parse of a consume/2 lock, keyed by component name. For
	// product-binary rows the value is per-target map; for product-tree rows
	// it is a flat spec.
	V2Artifacts map[string]V2ArtifactEntry `json:"-"`
}

// V2ArtifactEntry is a consume/2 artifact row: either per-target specs for
// binaries or a single spec for portable trees.
type V2ArtifactEntry struct {
	Flat     *ArtifactSpec
	ByTarget map[string]ArtifactSpec
}

// IntWindow is an inclusive min/max protocol window.
type IntWindow struct {
	Min int `json:"min"`
	Max int `json:"max"`
}

// ParseLock decodes and validates a lock file of either schema.
func ParseLock(data []byte) (*Lock, error) {
	var lock Lock
	if err := json.Unmarshal(data, &lock); err != nil {
		return nil, fmt.Errorf("lock is not valid JSON: %w", err)
	}
	switch lock.Schema {
	case SchemaV2:
	case SchemaV3:
		if lock.Source == nil {
			return nil, fmt.Errorf("lock must use schema %q; source block missing", SchemaV3)
		}
		if lock.Source.Module == "" || lock.Source.Version == "" {
			return nil, fmt.Errorf("consume/3 source block needs module and version")
		}
	default:
		return nil, fmt.Errorf("lock must use schema %q or %q; source-build locks are unsupported", SchemaV2, SchemaV3)
	}
	if strings.TrimSpace(lock.Release) == "" || strings.TrimSpace(lock.Commit) == "" {
		return nil, fmt.Errorf("lock must pin non-empty release and commit")
	}
	if lock.Artifacts == nil {
		return nil, fmt.Errorf("lock must contain an artifacts object")
	}
	if lock.Schema == SchemaV2 {
		v2, err := parseV2Artifacts(lock.Artifacts)
		if err != nil {
			return nil, err
		}
		lock.V2Artifacts = v2
	}
	return &lock, nil
}

// parseV2Artifacts decodes the legacy artifacts layout: product-tree rows
// carry a flat {url, sha256}; product-binary rows carry per-target maps.
func parseV2Artifacts(raw map[string]json.RawMessage) (map[string]V2ArtifactEntry, error) {
	out := make(map[string]V2ArtifactEntry, len(raw))
	for name, payload := range raw {
		var flat v2FlatSpec
		if err := json.Unmarshal(payload, &flat); err == nil && flat.SHA256 != "" && flat.URL != "" {
			out[name] = V2ArtifactEntry{Flat: &ArtifactSpec{
				URLs:   []string{flat.URL},
				SHA256: flat.SHA256,
			}}
			continue
		}
		var byTarget map[string]v2FlatSpec
		if err := json.Unmarshal(payload, &byTarget); err == nil && len(byTarget) > 0 {
			entry := V2ArtifactEntry{ByTarget: make(map[string]ArtifactSpec, len(byTarget))}
			for target, spec := range byTarget {
				entry.ByTarget[target] = ArtifactSpec{URLs: []string{spec.URL}, SHA256: spec.SHA256}
			}
			out[name] = entry
			continue
		}
		return nil, fmt.Errorf("lock has no artifact spec for %s", name)
	}
	return out, nil
}

// v2FlatSpec is the consume/2 artifact shape: a single url string.
type v2FlatSpec struct {
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
}

// LoadLock reads and parses the lock at path.
func LoadLock(path string) (*Lock, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseLock(data)
}

// ArtifactSpecFor returns the pinned spec for component at target. For
// consume/3 locks, product-tree rows and the prebuilt-CLI fallback live in
// Artifacts as flat specs; product-binaries other than the fallback have no
// pinned artifact (module channel covers them) and report ErrNoArtifact.
var ErrNoArtifact = fmt.Errorf("component has no pinned artifact (module channel)")

func (l *Lock) ArtifactSpecFor(component, target string) (*ArtifactSpec, error) {
	row, ok := l.Artifacts[component]
	if !ok {
		return nil, fmt.Errorf("lock has no %s artifact", component)
	}
	if l.Schema == SchemaV2 {
		entry := l.V2Artifacts[component]
		if entry.Flat != nil {
			spec := *entry.Flat
			return &ArtifactSpec{URLs: append([]string(nil), spec.URLs...), SHA256: spec.SHA256}, nil
		}
		for t, spec := range entry.ByTarget {
			if t == target {
				return &ArtifactSpec{URLs: append([]string(nil), spec.URLs...), SHA256: spec.SHA256}, nil
			}
		}
		return nil, fmt.Errorf("lock has no %s artifact for %s", component, target)
	}
	// consume/3: flat {urls: [...], sha256}
	var spec ArtifactSpec
	if err := json.Unmarshal(row, &spec); err != nil {
		return nil, fmt.Errorf("lock artifact %s is malformed: %w", component, err)
	}
	if len(spec.URLs) == 0 || spec.SHA256 == "" {
		return nil, fmt.Errorf("lock artifact %s needs urls and sha256", component)
	}
	return &spec, nil
}

// PinnedComponents returns the component names the lock pins artifacts for,
// or nil when the artifacts block is absent (legacy unusable lock).
func (l *Lock) PinnedComponents() map[string]bool {
	if l.Artifacts == nil {
		return nil
	}
	out := make(map[string]bool, len(l.Artifacts))
	for name := range l.Artifacts {
		out[name] = true
	}
	return out
}

// WriteResolved writes the resolved-installation JSON report (the
// --resolved-out payload) to path with parent directories created.
func WriteResolved(path string, resolved *ResolvedArtifacts) error {
	if resolved.Artifacts == nil {
		resolved.Artifacts = map[string]string{}
	}
	data, err := marshalResolved(resolved)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func marshalResolved(resolved *ResolvedArtifacts) ([]byte, error) {
	return json.MarshalIndent(resolved, "", "  ")
}

// FileSHA256 hashes a file in streaming fashion.
func FileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", digest.Sum(nil)), nil
}

// TreeSHA256 mirrors hostlib/digest.tree_sha256: sorted file list (skipping
// __pycache__), LF-normalized content, path + \0 + content + \0 per file.
func TreeSHA256(dir string) (string, error) {
	digest := sha256.New()
	var files []string
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "__pycache__" {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return "", err
	}
	if len(files) == 0 {
		return "", fmt.Errorf("%s is empty", dir)
	}
	sort.Strings(files)
	for _, rel := range files {
		data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
		if err != nil {
			return "", err
		}
		normalized := strings.ReplaceAll(string(data), "\r\n", "\n")
		normalized = strings.ReplaceAll(normalized, "\r", "\n")
		digest.Write([]byte(rel))
		digest.Write([]byte{0})
		digest.Write([]byte(normalized))
		digest.Write([]byte{0})
	}
	return fmt.Sprintf("%x", digest.Sum(nil)), nil
}
