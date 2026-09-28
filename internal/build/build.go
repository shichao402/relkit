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

	"github.com/shichao402/relkit/internal/registry"
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

// goDepPackages resolves the module-internal packages a Go host needs via
// `go list -deps`. Asking the compiler keeps the artifact honest when sdk/
// grows a dependency; a hand-written list would only surface the gap at the
// host's build.
func (c *RepoContext) goDepPackages(module string, entrypoints []string) ([]string, error) {
	args := append([]string{"list", "-deps"}, entrypoints...)
	command := exec.Command("go", args...)
	command.Dir = c.Root
	out, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("go list -deps failed: %w", err)
	}
	prefix := module + "/"
	seen := map[string]bool{}
	var packages []string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, prefix) {
			pkg := line[len(prefix):]
			if pkg == "" {
				continue
			}
			if !seen[pkg] {
				seen[pkg] = true
				packages = append(packages, pkg)
			}
		}
	}
	if len(packages) == 0 {
		return nil, fmt.Errorf("go list resolved no relkit packages for the Go SDK artifact")
	}
	sort.Strings(packages)
	return packages, nil
}

// goDepsEntries implements pack kind go-deps: listed files plus every
// module-internal package's non-test sources, archived relative to source.
func (c *RepoContext) goDepsEntries(source, module string, entrypoints, files, excludeSuffixes []string) ([]Entry, error) {
	entries, err := c.listedFileEntries(source, files, "sdk-go")
	if err != nil {
		return nil, err
	}
	packages, err := c.goDepPackages(module, entrypoints)
	if err != nil {
		return nil, err
	}
	for _, pkg := range packages {
		tracked := pkg
		if source != "." {
			tracked = source + "/" + pkg
		}
		sources, err := c.releaseSourcePaths(tracked)
		if err != nil {
			return nil, err
		}
		var kept []string
		for _, item := range sources {
			if !strings.HasSuffix(item, ".go") {
				continue
			}
			excluded := false
			for _, suffix := range excludeSuffixes {
				if strings.HasSuffix(item, suffix) {
					excluded = true
					break
				}
			}
			if excluded {
				continue
			}
			if filepath.ToSlash(filepath.Dir(item)) != tracked {
				continue
			}
			if _, err := os.Stat(filepath.Join(c.Root, filepath.FromSlash(item))); err != nil {
				continue
			}
			kept = append(kept, item)
		}
		if len(kept) == 0 {
			return nil, fmt.Errorf("Go SDK package %s has no tracked sources", pkg)
		}
		for _, item := range kept {
			full := filepath.Join(c.Root, filepath.FromSlash(item))
			sourceRoot := filepath.Join(c.Root, filepath.FromSlash(source))
			entries = append(entries, Entry{Source: full, ArchiveName: relWithin(sourceRoot, full)})
		}
	}
	return entries, nil
}

// ComponentEntries derives the full entry list for one product-tree
// component: pack kind dispatch, pack extras, exclude paths, and the host
// surface check for updater-carrying components.
func (c *RepoContext) ComponentEntries(name string) ([]Entry, error) {
	row, ok := registry.ByName[name]
	if !ok {
		return nil, fmt.Errorf("unknown component %q", name)
	}
	if row.Role != registry.RoleProductTree {
		return nil, fmt.Errorf("%s is not a packed product-tree", name)
	}
	kind := row.Pack.Kind
	if kind == "" {
		kind = registry.PackTrackedTree
	}
	var entries []Entry
	var err error
	switch kind {
	case registry.PackListedFiles:
		entries, err = c.listedFileEntries(row.Source, row.Pack.Files, row.Name)
	case registry.PackWorkingTree:
		entries, err = c.workingTreeEntries(row.Source, row.Pack.ExcludeParts)
	case registry.PackGoDeps:
		entries, err = c.goDepsEntries(row.Source, row.Pack.GoModule, row.Pack.GoEntrypoints, row.Pack.Files, row.Pack.ExcludeSuffixes)
	case registry.PackTrackedTree:
		entries, err = c.treeEntries(row.Source)
	default:
		return nil, fmt.Errorf("%s has unknown pack %q", row.Name, kind)
	}
	if err != nil {
		return nil, err
	}
	for _, extra := range row.Pack.PackExtras {
		full := filepath.Join(c.Root, filepath.FromSlash(extra[0]))
		if info, err := os.Stat(full); err != nil || info.IsDir() {
			return nil, fmt.Errorf("%s extra %s is missing", row.Name, extra[0])
		}
		entries = append(entries, Entry{Source: full, ArchiveName: extra[1]})
	}
	if len(row.Pack.ExcludePaths) > 0 {
		entries = filterExcludedPaths(entries, row.Pack.ExcludePaths)
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("%s packed no files", row.Name)
	}
	if row.UpdaterProcess != "" {
		if err := c.hostSurfaceCheck(&row, entries); err != nil {
			return nil, err
		}
	}
	return entries, nil
}

// hostSurfaceCheck mirrors packed_host_surface_errors: host zips may ship
// protocol helpers plus a facade, never an in-process engine.
func (c *RepoContext) hostSurfaceCheck(row *registry.Component, entries []Entry) error {
	names := []string{}
	texts := map[string]string{}
	for _, entry := range entries {
		ext := strings.ToLower(filepath.Ext(entry.Source))
		if !hostSourceSuffixSet[ext] {
			continue
		}
		data, err := os.ReadFile(entry.Source)
		if err != nil {
			return fmt.Errorf("read %s: %w", entry.Source, err)
		}
		names = append(names, entry.ArchiveName)
		texts[entry.ArchiveName] = string(data)
	}
	sort.Strings(names)
	problems := packedHostSurfaceErrors(row, names, texts)
	if len(problems) > 0 {
		return fmt.Errorf("%s", strings.Join(problems, "; "))
	}
	return nil
}
