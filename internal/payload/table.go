package payload

import (
	"fmt"
	"path"
	"strings"

	enginestate "github.com/shichao402/relkit/api/updater/enginestate/v1"
	"github.com/shichao402/relkit/internal/model"
)

// Table formats and the apply selector each one is published under
// (ADR 0013 isolation, ADR 0027 §3). A /2 table is invisible to engines that
// only know /1 because its artifact carries a different apply value.
const (
	SchemaV1 = "relkit.payload/1"
	SchemaV2 = "relkit.payload/2"

	ApplySelectorKey = "apply"
	ApplySelectorV1  = "relkit-payload"
	ApplySelectorV2  = "relkit-payload/2"

	// MaxSymlinkDepth bounds symlink chains during resolution; deeper chains
	// and cycles are rejected (ADR 0027 §1 rule 5).
	MaxSymlinkDepth = 40

	// BundleInfoPlist must be a FILE entry of every BUNDLE table.
	BundleInfoPlist = "Contents/Info.plist"
)

// EngineApplyPreference is the ordered list of apply selectors this engine
// accepts, most preferred first.
func EngineApplyPreference() []string {
	return []string{ApplySelectorV2, ApplySelectorV1}
}

// IsApplySelector reports whether value names a payload format.
func IsApplySelector(value string) bool {
	return value == ApplySelectorV1 || value == ApplySelectorV2
}

// UnitOf reads the content unit, treating UNSPECIFIED (every /1 table) as TREE.
func UnitOf(table *enginestate.FileTable) enginestate.ContentUnit {
	if table.GetUnit() == enginestate.ContentUnit_CONTENT_UNIT_BUNDLE {
		return enginestate.ContentUnit_CONTENT_UNIT_BUNDLE
	}
	return enginestate.ContentUnit_CONTENT_UNIT_TREE
}

// IsSymlink reports whether a table entry is a symlink. UNSPECIFIED reads as FILE.
func IsSymlink(entry *enginestate.FileEntry) bool {
	return entry.GetKind() == enginestate.EntryKind_ENTRY_KIND_SYMLINK
}

// HasSymlinks reports whether any entry is a symlink.
func HasSymlinks(table *enginestate.FileTable) bool {
	for _, entry := range table.GetFiles() {
		if IsSymlink(entry) {
			return true
		}
	}
	return false
}

// RequiredSchema is the only schema a table with this content may carry: /2
// exactly when it has a symlink or is a bundle, so /1 output stays byte for
// byte what older builds produced.
func RequiredSchema(table *enginestate.FileTable) string {
	if HasSymlinks(table) || UnitOf(table) == enginestate.ContentUnit_CONTENT_UNIT_BUNDLE {
		return SchemaV2
	}
	return SchemaV1
}

// ApplySelectorFor is the apply selector an artifact carrying this table is
// published under.
func ApplySelectorFor(table *enginestate.FileTable) string {
	if RequiredSchema(table) == SchemaV2 {
		return ApplySelectorV2
	}
	return ApplySelectorV1
}

// CheckBundleName validates a bundle directory name: one portable path
// segment ending in ".app".
func CheckBundleName(name string) error {
	if name == "" || strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("invalid bundle name %q", name)
	}
	if err := model.CheckFilename(name); err != nil {
		return fmt.Errorf("invalid bundle name %q: %w", name, err)
	}
	if !strings.HasSuffix(name, ".app") || name == ".app" {
		return fmt.Errorf("bundle name %q must end in .app", name)
	}
	return nil
}

// CheckPublishSelectors is the stage gate for one payload artifact
// (ADR 0027 §2, §3, §1 rule 6): its apply selector must name the table's
// format, a BUNDLE must target os=darwin, and a table with symlinks must not
// target os=windows.
func CheckPublishSelectors(table *enginestate.FileTable, selectors map[string]string) error {
	if got, want := selectors[ApplySelectorKey], ApplySelectorFor(table); got != want {
		return fmt.Errorf("payload is %s and must carry selector apply=%s, has apply=%q", table.GetSchema(), want, got)
	}
	if UnitOf(table) == enginestate.ContentUnit_CONTENT_UNIT_BUNDLE && selectors["os"] != "darwin" {
		return fmt.Errorf("a BUNDLE payload must target os=darwin, has os=%q", selectors["os"])
	}
	if HasSymlinks(table) && selectors["os"] == "windows" {
		return fmt.Errorf("a payload with symlinks cannot target os=windows")
	}
	return nil
}

// CheckTable enforces every structural rule of a file table. Build, inspect
// and the engine's apply all call it, so the rules hold wherever a table is
// produced or consumed.
func CheckTable(table *enginestate.FileTable) error {
	if table == nil {
		return fmt.Errorf("nil file table")
	}
	switch table.Unit {
	case enginestate.ContentUnit_CONTENT_UNIT_UNSPECIFIED, enginestate.ContentUnit_CONTENT_UNIT_TREE:
		if table.BundleName != "" {
			return fmt.Errorf("bundle_name is set on a TREE table")
		}
	case enginestate.ContentUnit_CONTENT_UNIT_BUNDLE:
		if err := CheckBundleName(table.BundleName); err != nil {
			return err
		}
		if len(table.Preserve) != 0 {
			return fmt.Errorf("a BUNDLE table cannot preserve entries: a sealed bundle holds no mutable data")
		}
	default:
		return fmt.Errorf("unknown content unit %d", table.Unit)
	}
	if want := RequiredSchema(table); table.Schema != want {
		return fmt.Errorf("payload schema %q does not match its content (want %q)", table.Schema, want)
	}
	for _, entry := range table.Files {
		if entry == nil {
			return fmt.Errorf("payload contains nil file entry")
		}
		if err := CheckRelpath(entry.Relpath); err != nil {
			return err
		}
		switch entry.Kind {
		case enginestate.EntryKind_ENTRY_KIND_UNSPECIFIED, enginestate.EntryKind_ENTRY_KIND_FILE:
			if entry.LinkTarget != "" {
				return fmt.Errorf("file entry %q has a link target", entry.Relpath)
			}
		case enginestate.EntryKind_ENTRY_KIND_SYMLINK:
			if entry.Size != 0 || entry.Sha256 != "" || entry.Mode != 0 {
				return fmt.Errorf("symlink entry %q carries file content fields", entry.Relpath)
			}
		default:
			return fmt.Errorf("entry %q has unknown kind %d", entry.Relpath, entry.Kind)
		}
	}
	if UnitOf(table) == enginestate.ContentUnit_CONTENT_UNIT_BUNDLE {
		found := false
		for _, entry := range table.Files {
			if entry.Relpath == BundleInfoPlist && !IsSymlink(entry) {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("bundle %q has no %s", table.BundleName, BundleInfoPlist)
		}
	}
	return CheckSymlinks(table)
}

// CheckSymlinks applies the symlink rules of ADR 0027 §1 to a table:
//
//  1. a target is non-empty, relative, without backslash or NUL;
//  2. resolved lexically from the link's directory it stays inside the
//     content root;
//  3. it resolves to an entry of the table or an ancestor directory of one
//     (no dangling links);
//  4. no entry's relpath passes through a symlink entry, so a write can never
//     cross a link the table created;
//  5. chains resolve within MaxSymlinkDepth hops and without cycles.
//
// Preserved paths may not be symlinks or lie beneath one.
func CheckSymlinks(table *enginestate.FileTable) error {
	links := map[string]*enginestate.FileEntry{}
	files := map[string]struct{}{}
	dirs := map[string]struct{}{}
	for _, entry := range table.GetFiles() {
		if IsSymlink(entry) {
			links[entry.Relpath] = entry
		} else {
			files[entry.Relpath] = struct{}{}
		}
		for dir := path.Dir(entry.Relpath); dir != "."; dir = path.Dir(dir) {
			dirs[dir] = struct{}{}
		}
	}
	if len(links) == 0 {
		return nil
	}
	throughLink := func(rel string) string {
		for dir := path.Dir(rel); dir != "."; dir = path.Dir(dir) {
			if _, ok := links[dir]; ok {
				return dir
			}
		}
		return ""
	}
	for _, entry := range table.GetFiles() {
		if via := throughLink(entry.Relpath); via != "" {
			return fmt.Errorf("entry %q passes through symlink %q", entry.Relpath, via)
		}
	}
	for _, rel := range table.GetPreserve() {
		if _, ok := links[rel]; ok {
			return fmt.Errorf("preserve %q is a symlink", rel)
		}
		if via := throughLink(rel); via != "" {
			return fmt.Errorf("preserve %q lies beneath symlink %q", rel, via)
		}
	}
	r := &linkResolver{links: links, memo: map[string]string{}}
	for rel := range links {
		resolved, err := r.resolveLink(rel, 0)
		if err != nil {
			return err
		}
		_, isFile := files[resolved]
		_, isDir := dirs[resolved]
		if !isFile && !isDir {
			return fmt.Errorf("symlink %q dangles: %q is not in the table", rel, resolved)
		}
	}
	return nil
}

type linkResolver struct {
	links map[string]*enginestate.FileEntry
	memo  map[string]string
}

// resolveLink returns the non-link path a symlink entry ends at.
func (r *linkResolver) resolveLink(rel string, depth int) (string, error) {
	if done, ok := r.memo[rel]; ok {
		return done, nil
	}
	if depth >= MaxSymlinkDepth {
		return "", fmt.Errorf("symlink %q is part of a cycle or a chain deeper than %d", rel, MaxSymlinkDepth)
	}
	target := r.links[rel].LinkTarget
	if target == "" || path.IsAbs(target) || strings.ContainsAny(target, "\\\x00") {
		return "", fmt.Errorf("symlink %q has invalid target %q", rel, target)
	}
	joined := path.Clean(path.Join(path.Dir(rel), target))
	if joined == "." || joined == ".." || strings.HasPrefix(joined, "../") {
		return "", fmt.Errorf("symlink %q resolves outside the content: %q", rel, target)
	}
	resolved, err := r.resolvePath(joined, depth+1)
	if err != nil {
		return "", err
	}
	r.memo[rel] = resolved
	return resolved, nil
}

// resolvePath expands every symlink component of a clean relative path.
func (r *linkResolver) resolvePath(p string, depth int) (string, error) {
	current := ""
	for _, part := range strings.Split(p, "/") {
		next := part
		if current != "" {
			next = current + "/" + part
		}
		if _, isLink := r.links[next]; isLink {
			resolved, err := r.resolveLink(next, depth)
			if err != nil {
				return "", err
			}
			next = resolved
		}
		current = next
	}
	return current, nil
}
