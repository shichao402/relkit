package changelog

import (
	"testing"

	rupv2 "github.com/shichao402/relkit/api/rup/v2"
)

func TestExtractSection(t *testing.T) {
	const doc = `# Changelog

## [1.2.0] - 2026-01-02

### Added
- feature A

## [1.1.0] - 2025-12-01

### Fixed
- bug B

## Unreleased

- wip
`

	got, ok := ExtractSection(doc, "1.2.0")
	if !ok {
		t.Fatal("expected section for 1.2.0")
	}
	if !containsAll(got, "## [1.2.0]", "### Added", "feature A") {
		t.Fatalf("unexpected section:\n%s", got)
	}
	if containsAll(got, "1.1.0") {
		t.Fatalf("section leaked into next heading:\n%s", got)
	}

	got, ok = ExtractSection(doc, "1.1.0")
	if !ok || !containsAll(got, "bug B") {
		t.Fatalf("1.1.0 section: ok=%v body=%q", ok, got)
	}

	if _, ok := ExtractSection(doc, "9.9.9"); ok {
		t.Fatal("missing version should not match")
	}
}

func TestExtractSectionBuildMeta(t *testing.T) {
	const doc = `## [0.0.22+17] - 2026-08-06

- notes

## [0.0.22+16]

- older
`
	got, ok := ExtractSection(doc, "0.0.22+17")
	if !ok || !containsAll(got, "0.0.22+17", "notes") {
		t.Fatalf("build-meta section: ok=%v body=%q", ok, got)
	}
}

func TestFormatURL(t *testing.T) {
	u, err := FormatURL(
		"https://git.example.com/p/blob/main/CHANGELOG.md#{anchor}",
		"0.0.22+17",
	)
	if err != nil {
		t.Fatal(err)
	}
	want := "https://git.example.com/p/blob/main/CHANGELOG.md#v0.0.22-17"
	if u != want {
		t.Fatalf("got %q want %q", u, want)
	}
}

func TestCompactPriorNodes(t *testing.T) {
	nodes := []*rupv2.VersionNode{
		{Version: "1.0.0", Code: 100, Notes: "old body"},
		{Version: "1.1.0", Code: 110, Notes: "new body"},
	}
	if err := CompactPriorNodes(
		nodes,
		"https://example.com/CHANGELOG.md#{anchor}",
		110,
	); err != nil {
		t.Fatal(err)
	}
	if nodes[0].Notes != "" {
		t.Fatalf("prior notes should be cleared, got %q", nodes[0].Notes)
	}
	if nodes[0].NotesUrl != "https://example.com/CHANGELOG.md#v1.0.0" {
		t.Fatalf("prior notesUrl: %q", nodes[0].NotesUrl)
	}
	if nodes[1].Notes != "new body" {
		t.Fatalf("head notes must stay, got %q", nodes[1].Notes)
	}
}

func containsAll(s string, parts ...string) bool {
	for _, p := range parts {
		if !stringContains(s, p) {
			return false
		}
	}
	return true
}

func stringContains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
