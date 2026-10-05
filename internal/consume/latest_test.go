package consume

import (
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
