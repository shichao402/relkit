// Package build assembles relkit's release artifacts from a source tree:
// cross-compiled binaries, deterministic product-tree zips, and the
// relkit.release/1 manifest. It is the Go single source for what deploy's
// Python build face produced (ADR 0017 phase 1 batch 3), with one deliberate
// shape change: manifests record the tree hashes the lock pins, not the
// Python-era consumerSha256.
package build

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	vnextregistry "github.com/shichao402/relkit/internal/vnext/registry"
)

// Entry is one file to pack into an archive: source path on disk plus the
// archive-internal name (posix, forward slashes).
type Entry struct {
	Source      string
	ArchiveName string
}

// Identity is the commit/dirty pair a stamp derives from.
type Identity struct {
	Commit string
	Dirty  bool
}

// RepoContext carries repo-level inputs a build needs: where the repo is,
// the SSOT version number, and how to shell out to git and go.
type RepoContext struct {
	Root   string
	Number string // SSOT version number, e.g. "0.5.0"
}

// Stamp returns the version stamp: number + short commit (+ "-dirty").
func (c *RepoContext) Stamp() (string, error) {
	ident, err := c.Identity()
	if err != nil {
		return "", err
	}
	stamp := c.Number + "+" + ident.Commit[:12]
	if ident.Dirty {
		stamp += "-dirty"
	}
	return stamp, nil
}

// Identity returns the current commit and dirty state.
func (c *RepoContext) Identity() (*Identity, error) {
	commit, err := c.git("rev-parse", "HEAD")
	if err != nil {
		return nil, fmt.Errorf("git rev-parse HEAD: %w", err)
	}
	status, err := c.git("status", "--porcelain")
	if err != nil {
		return nil, fmt.Errorf("git status: %w", err)
	}
	return &Identity{Commit: strings.TrimSpace(commit), Dirty: strings.TrimSpace(status) != ""}, nil
}

func (c *RepoContext) git(args ...string) (string, error) {
	command := exec.Command("git", args...)
	command.Dir = c.Root
	out, err := command.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// releaseSourcePaths lists tracked-or-untracked-but-not-ignored files under
// the given prefixes, sorted, posix-style, relative to the repo root.
func (c *RepoContext) releaseSourcePaths(prefixes ...string) ([]string, error) {
	args := append([]string{"ls-files", "-z", "--cached", "--others", "--exclude-standard", "--"}, prefixes...)
	out, err := c.git(args...)
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, item := range strings.Split(out, "\x00") {
		if item = strings.TrimSpace(item); item != "" {
			paths = append(paths, item)
		}
	}
	sort.Strings(paths)
	return paths, nil
}

// treeEntries implements pack kind tracked-tree: every tracked file under
// source, relative to the source root.
func (c *RepoContext) treeEntries(source string) ([]Entry, error) {
	sourceRoot := filepath.Join(c.Root, filepath.FromSlash(source))
	relatives, err := c.releaseSourcePaths(source)
	if err != nil {
		return nil, err
	}
	if len(relatives) == 0 {
		return nil, fmt.Errorf("%s has no tracked files", source)
	}
	var entries []Entry
	for _, relative := range relatives {
		full := filepath.Join(c.Root, filepath.FromSlash(relative))
		if info, err := os.Stat(full); err == nil && !info.IsDir() {
			entries = append(entries, Entry{Source: full, ArchiveName: relWithin(sourceRoot, full)})
		}
	}
	return entries, nil
}

// workingTreeEntries implements pack kind working-tree: every file under
// source on disk (no git), excluding path components in excludeParts.
func (c *RepoContext) workingTreeEntries(source string, excludeParts []string) ([]Entry, error) {
	sourceRoot := filepath.Join(c.Root, filepath.FromSlash(source))
	if info, err := os.Stat(sourceRoot); err != nil || !info.IsDir() {
		return nil, fmt.Errorf("%s is missing", source)
	}
	excluded := map[string]bool{}
	for _, part := range excludeParts {
		excluded[part] = true
	}
	var entries []Entry
	walkErr := filepath.WalkDir(sourceRoot, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			// Skip excluded directories wholesale.
			for _, part := range strings.Split(filepath.ToSlash(relWithin(sourceRoot, path)), "/") {
				if excluded[part] {
					return filepath.SkipDir
				}
			}
			return nil
		}
		rel := relWithin(sourceRoot, path)
		for _, part := range strings.Split(rel, "/") {
			if excluded[part] {
				return nil
			}
		}
		entries = append(entries, Entry{Source: path, ArchiveName: rel})
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}
	return entries, nil
}

// listedFileEntries implements pack kind listed-files: exact paths.
func (c *RepoContext) listedFileEntries(source string, names []string, component string) ([]Entry, error) {
	sourceRoot := filepath.Join(c.Root, filepath.FromSlash(source))
	var entries []Entry
	var missing []string
	for _, name := range names {
		full := filepath.Join(sourceRoot, filepath.FromSlash(name))
		if info, err := os.Stat(full); err != nil || info.IsDir() {
			missing = append(missing, name)
		} else {
			entries = append(entries, Entry{Source: full, ArchiveName: name})
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("%s is incomplete: %s", component, strings.Join(missing, ", "))
	}
	return entries, nil
}

// ComponentEntries derives the full entry list for one product-tree
// component: pack kind dispatch, pack extras, exclude paths, and the host
// surface check for updater-carrying components. The pack shape is declared in
// the single vNext component registry.
func (c *RepoContext) ComponentEntries(document *vnextregistry.Document, name string) ([]Entry, error) {
	row, ok := document.ByName(name)
	if !ok {
		return nil, fmt.Errorf("unknown component %q", name)
	}
	if row.Role != vnextregistry.RoleFacade && row.Role != vnextregistry.RoleProjection {
		return nil, fmt.Errorf("%s is not a packed product-tree", name)
	}
	if row.Build.Pack == nil {
		return nil, fmt.Errorf("%s declares no pack block", name)
	}
	pack := row.Build.Pack
	kind := pack.Kind
	if kind == "" {
		kind = vnextregistry.PackTrackedTree
	}
	var entries []Entry
	var err error
	switch kind {
	case vnextregistry.PackListedFiles:
		entries, err = c.listedFileEntries(row.Build.Source, pack.Files, row.Name)
	case vnextregistry.PackWorkingTree:
		entries, err = c.workingTreeEntries(row.Build.Source, pack.ExcludeParts)
	case vnextregistry.PackTrackedTree:
		entries, err = c.treeEntries(row.Build.Source)
	default:
		return nil, fmt.Errorf("%s has unknown pack %q", row.Name, kind)
	}
	if err != nil {
		return nil, err
	}
	for _, extra := range pack.Extras {
		full := filepath.Join(c.Root, filepath.FromSlash(extra[0]))
		if info, err := os.Stat(full); err != nil || info.IsDir() {
			return nil, fmt.Errorf("%s extra %s is missing", row.Name, extra[0])
		}
		entries = append(entries, Entry{Source: full, ArchiveName: extra[1]})
	}
	if len(pack.ExcludePaths) > 0 {
		entries = filterExcludedPaths(entries, pack.ExcludePaths)
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("%s packed no files", row.Name)
	}
	// The retired source-grep facade gate (ADR 0019 §1) used to scan packed
	// sources for an in-process engine. What stays enforceable is structural:
	// every facade path a component declares must really be inside its archive.
	if len(row.Build.FacadePaths) > 0 {
		packed := make(map[string]bool, len(entries))
		for _, entry := range entries {
			packed[entry.ArchiveName] = true
		}
		for _, facade := range row.Build.FacadePaths {
			if !packed[facade] {
				return nil, fmt.Errorf("%s facade %s is not packed", row.Name, facade)
			}
		}
	}
	return entries, nil
}
