package updater

import (
	"path/filepath"
	"strings"
)

// listRunningExecutables returns the absolute executable paths of the
// processes this user can see. It is best effort: a process the OS will not
// describe is skipped, never reported as an error. A package variable so tests
// can simulate running versions.
var listRunningExecutables = runningExecutables

// versionsInUse names the versions/<name> directories that hold the
// executable of a running process (ADR 0027 §4.4). The engine finds this out
// itself instead of asking the host, so it also covers versions run by other
// instances and, in a managed library, versions of the managed product.
func versionsInUse(installRoot string) map[string]struct{} {
	base := filepath.Join(installRoot, VersionsDirName)
	bases := []string{base}
	if resolved, err := filepath.EvalSymlinks(base); err == nil && resolved != base {
		bases = append(bases, resolved)
	}
	out := map[string]struct{}{}
	for _, exe := range listRunningExecutables() {
		if !filepath.IsAbs(exe) {
			continue
		}
		for _, b := range bases {
			if name, ok := firstComponentUnder(b, exe); ok {
				out[name] = struct{}{}
			}
		}
	}
	return out
}

// firstComponentUnder returns the first path element of p below base, or
// false when p is not strictly inside base. filepath.Rel compares
// case-insensitively on Windows.
func firstComponentUnder(base, p string) (string, bool) {
	rel, err := filepath.Rel(base, p)
	if err != nil || rel == "." || rel == ".." || filepath.IsAbs(rel) ||
		strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	first := strings.SplitN(rel, string(filepath.Separator), 2)[0]
	if first == "" || first == "." {
		return "", false
	}
	return first, true
}
