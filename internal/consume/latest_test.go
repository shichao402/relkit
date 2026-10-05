package consume

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestParseLockFollowLatest: the follow-latest form (release = "latest")
// parses with empty commit; the pinned form still demands both fields.
func TestParseLockFollowLatest(t *testing.T) {
	follow := `{
  "schema": "relkit.consume/3",
  "release": "latest",
  "commit": "",
  "source": {"module": "github.com/shichao402/relkit", "version": "latest", "commit": ""},
  "protocol": {"min": 2, "max": 2},
  "updaterIpc": {"min": 3, "max": 3},
  "artifacts": {}
}`
	lock, err := ParseLock([]byte(follow))
	if err != nil {
		t.Fatalf("follow-latest lock must parse: %v", err)
	}
	if !IsLatest(lock.Release) {
		t.Fatalf("expected follow-latest release, got %q", lock.Release)
	}

	pinnedMissingCommit := strings.Replace(follow, `"release": "latest"`, `"release": "v0.5.17"`, 1)
	if _, err := ParseLock([]byte(pinnedMissingCommit)); err == nil {
		t.Fatal("pinned form without commit must be rejected")
	}
}

// TestIsLatestKeywordCase: keyword matching is case-insensitive and
// whitespace-tolerant at the edges only.
func TestIsLatestKeywordCase(t *testing.T) {
	for _, in := range []string{"latest", "Latest", "LATEST", " latest "} {
		if !IsLatest(in) {
			t.Fatalf("%q must be recognized as latest", in)
		}
	}
	for _, in := range []string{"v0.5.17", "", "latestest", "relatest"} {
		if IsLatest(in) {
			t.Fatalf("%q must not be recognized as latest", in)
		}
	}
}

// TestParseSHA256SUMSWire: sums parsing stays line-format tolerant.
func TestParseSHA256SUMSWire(t *testing.T) {
	sums := ParseSHA256SUMS("ABC123  relkit-cli\ndef456 *relkit-sdk\n")
	if sums["relkit-cli"] != "abc123" || sums["relkit-sdk"] != "def456" {
		t.Fatalf("unexpected sums: %#v", sums)
	}
}

// TestTagFromRedirectLocation: the releases/latest redirect target must
// yield a vX.Y.Z tag; other shapes are rejected.
func TestTagFromRedirectLocation(t *testing.T) {
	for _, tc := range []struct{ location, want string }{
		{"https://github.com/shichao402/relkit/releases/tag/v0.5.18", "v0.5.18"},
		{"/releases/tag/v0.5.18", "v0.5.18"},
		{"https://github.com/shichao402/relkit/releases/tag/v0.5.18/", "v0.5.18"},
	} {
		tag, ok := tagFromRedirectLocation(tc.location)
		if !ok || tag != tc.want {
			t.Fatalf("tagFromRedirectLocation(%q) = %q, %v; want %q", tc.location, tag, ok, tc.want)
		}
	}
	for _, bad := range []string{
		"https://github.com/shichao402/relkit/releases",
		"https://github.com/shichao402/relkit/releases/tag/nightly",
		"",
	} {
		if tag, ok := tagFromRedirectLocation(bad); ok {
			t.Fatalf("tagFromRedirectLocation(%q) = %q must be rejected", bad, tag)
		}
	}
}

// TestMirrorDirListingParse: the mirrors "Index of" HTML yields the newest
// generation via the shared regex, covering real wire shapes.
func TestMirrorDirListingParse(t *testing.T) {
	body := `<!DOCTYPE html>
<html><head><title>Index of /tencent/relkit/mirror/github/relkit/</title></head>
<body><h1>Index of</h1><pre>
<a href="../">../</a>
<a href="v0.5.12/">v0.5.12/</a>    mirrors       2026-10-04 21:31    -       -
<a href="v0.5.13/">v0.5.13/</a>    mirrors       2026-10-04 21:59    -       -
<a href="v0.5.15/">v0.5.15/</a>    mirrors       2026-10-04 20:00    -       -
<a href="v0.5.17/">v0.5.17/</a>    mirrors       2026-10-04 16:50    -       -
</pre></body></html>`
	best := ""
	for _, match := range mirrorDirPattern.FindAllStringSubmatch(body, -1) {
		if best == "" || semverLess(best, match[1]) {
			best = match[1]
		}
	}
	if best != "v0.5.17" {
		t.Fatalf("newest mirrors generation = %q; want v0.5.17", best)
	}
}

// TestSemverLess: patch/minor/major ordering plus stability on equality.
func TestSemverLess(t *testing.T) {
	if !semverLess("v0.5.9", "v0.5.17") {
		t.Fatal("v0.5.9 < v0.5.17")
	}
	if !semverLess("v0.5.17", "v0.6.0") {
		t.Fatal("v0.5.17 < v0.6.0")
	}
	if !semverLess("v0.9.99", "v1.0.0") {
		t.Fatal("v0.9.99 < v1.0.0")
	}
	if semverLess("v0.5.17", "v0.5.17") {
		t.Fatal("equal versions must not compare less")
	}
}

// TestPrependMirrorURLs: every resolved artifact row gains the mirrors URL
// ahead of the GitHub URL, in both flat and by-target shapes; idempotent on
// re-run.
func TestPrependMirrorURLs(t *testing.T) {
	lock := &Lock{
		Schema:  SchemaV3,
		Release: "v0.5.18",
		Artifacts: map[string]json.RawMessage{
			"host-scripts": mustMarshal(t, ArtifactSpec{
				URLs:   []string{"https://github.com/shichao402/relkit/releases/download/v0.5.18/relkit-host-scripts.zip"},
				SHA256: strings.Repeat("a", 64),
			}),
			"cli": mustMarshal(t, map[string]ArtifactSpec{
				"windows-amd64": {
					URLs:   []string{"https://github.com/shichao402/relkit/releases/download/v0.5.18/relkit-windows-amd64.exe"},
					SHA256: strings.Repeat("b", 64),
				},
			}),
		},
	}
	PrependMirrorURLs(lock, "v0.5.18")

	spec, err := lock.ArtifactSpecFor("host-scripts", "host")
	if err != nil {
		t.Fatal(err)
	}
	if spec.URLs[0] != MirrorAssetURL("v0.5.18", "relkit-host-scripts.zip") {
		t.Fatalf("flat row first URL = %q", spec.URLs[0])
	}

	cliSpec, err := lock.ArtifactSpecFor("cli", "windows-amd64")
	if err != nil {
		t.Fatal(err)
	}
	if cliSpec.URLs[0] != MirrorAssetURL("v0.5.18", "relkit-windows-amd64.exe") {
		t.Fatalf("by-target row first URL = %q", cliSpec.URLs[0])
	}

	// Idempotence: the second pass must not duplicate the mirrors URL.
	PrependMirrorURLs(lock, "v0.5.18")
	cliSpec, err = lock.ArtifactSpecFor("cli", "windows-amd64")
	if err != nil {
		t.Fatal(err)
	}
	if len(cliSpec.URLs) != 2 {
		t.Fatalf("re-run duplicated mirrors URL: %v", cliSpec.URLs)
	}
}

func mustMarshal(t *testing.T, v any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
