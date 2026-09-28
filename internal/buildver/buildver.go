// Package buildver resolves the running binary's own version.
//
// Release artifacts get their version stamped by scripts/deploy/relkit.py
// via -ldflags "-X main.version=<stamp>". The module channel (go run /
// go install github.com/shichao402/relkit/cmd/...@vX) carries no ldflags,
// so the placeholder must fall back to debug.ReadBuildInfo: a binary built
// from a module at a specific version reports that version in Main.Version,
// while a working-copy build reports "(devel)". The placeholder itself is a
// marker, never a real-looking semver, so an unstamped binary cannot
// masquerade as a release.
package buildver

import (
	"runtime/debug"
	"strings"
)

// Placeholder is the build-time default for main.version.
const Placeholder = "devel"

// Resolve returns the effective binary version.
//
// Priority:
//  1. An ldflags-injected value (anything other than the placeholder).
//  2. debug.ReadBuildInfo Main.Version for module builds ("v0.4.24" is
//     normalized to "0.4.24"); "(devel)" working-copy builds fall through.
//  3. The placeholder itself.
func Resolve(injected string) string {
	if v := strings.TrimSpace(injected); v != "" && v != Placeholder && v != "(devel)" {
		return v
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		if v := strings.TrimSpace(info.Main.Version); v != "" && v != "(devel)" {
			return strings.TrimPrefix(v, "v")
		}
	}
	return Placeholder
}
