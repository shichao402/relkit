package buildver

import (
	"runtime/debug"
	"strings"
	"testing"
)

func TestResolveInjectedUnchanged(t *testing.T) {
	for _, stamp := range []string{
		"0.4.24",
		"0.4.24+abcdef123456",
		"0.5.0+abcdef123456-dirty",
	} {
		if got := Resolve(stamp); got != stamp {
			t.Fatalf("Resolve(%q) = %q, want it unchanged", stamp, got)
		}
	}
}

func TestResolvePlaceholderFallsBackToBuildInfo(t *testing.T) {
	// The test binary is built from a working copy, so its Main.Version is
	// "(devel)" and the expected answer is the placeholder. Compute the
	// expectation from the same build info so the test stays correct if the
	// toolchain ever stamps test binaries with a module version instead.
	want := Placeholder
	if info, ok := debug.ReadBuildInfo(); ok {
		if v := strings.TrimSpace(info.Main.Version); v != "" && v != "(devel)" {
			want = strings.TrimPrefix(v, "v")
		}
	}
	for _, injected := range []string{"", Placeholder, "(devel)", "  "} {
		if got := Resolve(injected); got != want {
			t.Fatalf("Resolve(%q) = %q, want %q", injected, got, want)
		}
	}
}
