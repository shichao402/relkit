// Package changelog extracts release notes from a Keep-a-Changelog-style
// markdown file and builds notesUrl values from a URL template.
//
// Design (product contract):
//   - The newest release carries full markdown in VersionNode.notes (and
//     Manifest.notes).
//   - Older releases keep a repository link in VersionNode.notes_url and drop
//     the inlined body so the signed index stays small.
package changelog

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Config is the optional relkit.json "changelog" object.
type Config struct {
	// File is a path relative to the project root (relkit.json directory).
	File string
	// URLTemplate builds notesUrl. Placeholders: {version}, {version_slug},
	// {anchor}. Empty disables automatic notesUrl / prior compaction.
	URLTemplate string
}

var (
	// Version sections use H1/H2 only. H3+ are body headings (Added/Fixed/…).
	headingLine = regexp.MustCompile(`(?m)^#{1,2}\s+(.+)$`)
	bracketVer  = regexp.MustCompile(`^\[([^\]]+)\]`)
	// Loose version token: digits/dots plus optional +build / -prerelease.
	versionLike = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z.+_-]*$`)
)

// LoadFile reads a changelog markdown file.
func LoadFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// ResolvePath joins root with a relative changelog path.
func ResolvePath(root, relative string) string {
	if relative == "" {
		return ""
	}
	if filepath.IsAbs(relative) {
		return relative
	}
	return filepath.Join(root, relative)
}

// ExtractSection returns the markdown body for version (heading included).
//
// Accepted headings (Keep a Changelog and common variants):
//
//	## [1.2.0] - 2026-01-02
//	## 1.2.0
//	## [1.2.0+13]
//
// Matching is exact on the version token after optional brackets.
func ExtractSection(markdown, version string) (string, bool) {
	version = strings.TrimSpace(version)
	if version == "" || strings.TrimSpace(markdown) == "" {
		return "", false
	}

	lines := strings.Split(markdown, "\n")
	start := -1
	for i, line := range lines {
		if sectionVersion(line) == version {
			start = i
			break
		}
	}
	if start < 0 {
		return "", false
	}

	end := len(lines)
	for i := start + 1; i < len(lines); i++ {
		if isVersionHeading(lines[i]) {
			end = i
			break
		}
	}

	section := strings.TrimSpace(strings.Join(lines[start:end], "\n"))
	if section == "" {
		return "", false
	}
	return section, true
}

func isVersionHeading(line string) bool {
	return sectionVersion(line) != ""
}

func sectionVersion(line string) string {
	match := headingLine.FindStringSubmatch(strings.TrimRight(line, "\r"))
	if match == nil {
		return ""
	}
	title := strings.TrimSpace(match[1])
	title = strings.TrimSpace(strings.SplitN(title, " - ", 2)[0])
	title = strings.TrimSpace(strings.SplitN(title, " – ", 2)[0])
	if m := bracketVer.FindStringSubmatch(title); m != nil {
		return strings.TrimSpace(m[1])
	}
	lower := strings.ToLower(title)
	if lower == "changelog" || lower == "unreleased" {
		return ""
	}
	if !versionLike.MatchString(title) {
		return ""
	}
	return title
}

// VersionSlug makes a path/fragment-safe token: '+' → '-', spaces removed.
func VersionSlug(version string) string {
	s := strings.TrimSpace(version)
	s = strings.ReplaceAll(s, "+", "-")
	s = strings.ReplaceAll(s, " ", "")
	return s
}

// Anchor is the default fragment for a version section: "v" + slug.
func Anchor(version string) string {
	return "v" + VersionSlug(version)
}

// FormatURL substitutes placeholders in template. Empty template → "".
func FormatURL(template, version string) (string, error) {
	template = strings.TrimSpace(template)
	if template == "" {
		return "", nil
	}
	slug := VersionSlug(version)
	anchor := Anchor(version)
	replacer := strings.NewReplacer(
		"{version}", url.PathEscape(version),
		"{version_slug}", url.PathEscape(slug),
		"{anchor}", url.PathEscape(anchor),
	)
	out := replacer.Replace(template)
	if strings.Contains(out, "{") {
		return "", fmt.Errorf("changelog.urlTemplate still contains unresolved placeholders: %q", out)
	}
	if !strings.HasPrefix(out, "http://") && !strings.HasPrefix(out, "https://") {
		return "", fmt.Errorf("changelog.urlTemplate must produce an http(s) URL, got %q", out)
	}
	return out, nil
}

// ResolveNotes fills notes / notesUrl from CLI overrides and optional config.
//
// Precedence for notes: explicitNotes > notesFile > changelog file section.
// Precedence for notesUrl: explicitURL > urlTemplate.
func ResolveNotes(cfg Config, root, version, explicitNotes, notesFile, explicitURL string) (notes, notesURL string, err error) {
	notes = strings.TrimSpace(explicitNotes)
	if notes == "" && notesFile != "" {
		data, readErr := os.ReadFile(notesFile)
		if readErr != nil {
			return "", "", fmt.Errorf("notes file not found: %s", notesFile)
		}
		notes = strings.TrimSpace(string(data))
	}
	if notes == "" && cfg.File != "" {
		path := ResolvePath(root, cfg.File)
		body, readErr := LoadFile(path)
		if readErr != nil {
			return "", "", fmt.Errorf("changelog file not found: %s", path)
		}
		section, ok := ExtractSection(body, version)
		if !ok {
			return "", "", fmt.Errorf("changelog %s has no section for version %q", path, version)
		}
		notes = section
	}

	notesURL = strings.TrimSpace(explicitURL)
	if notesURL == "" && cfg.URLTemplate != "" {
		notesURL, err = FormatURL(cfg.URLTemplate, version)
		if err != nil {
			return "", "", err
		}
	}
	if notesURL != "" {
		if !strings.HasPrefix(notesURL, "http://") && !strings.HasPrefix(notesURL, "https://") {
			return "", "", fmt.Errorf("notesUrl must be an http(s) URL, got %q", notesURL)
		}
	}
	return notes, notesURL, nil
}
