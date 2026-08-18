package version

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"cnb.cool/shichao402/relkit/internal/jsonio"
)

const (
	// FileName is the mandatory project version file next to relkit.json.
	FileName = "VERSION.json"
	// SchemaID is the required schema marker.
	SchemaID = "rup.version/1"
)

// Document is the on-disk project version SSOT.
//
// Extra top-level fields (for example host-specific compatibility metadata) are
// preserved on save when present in Raw.
type Document struct {
	Schema  string
	Version string
	Path    string
	// Raw holds the full JSON object so unknown fields survive round-trips.
	Raw map[string]any
}

// Parts is a parsed x.y.z+build identity.
type Parts struct {
	Major int
	Minor int
	Patch int
	Build int
}

func (p Parts) String() string {
	return fmt.Sprintf("%d.%d.%d+%d", p.Major, p.Minor, p.Patch, p.Build)
}

// Number returns the display core without build: x.y.z.
func (p Parts) Number() string {
	return fmt.Sprintf("%d.%d.%d", p.Major, p.Minor, p.Patch)
}

// Find looks for VERSION.json starting at start (or cwd) and walking parents,
// stopping at the filesystem root. Empty path means not found.
func Find(start string) (string, error) {
	current := start
	if current == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		current = cwd
	}
	resolved, err := filepath.Abs(current)
	if err != nil {
		return "", err
	}
	for {
		candidate := filepath.Join(resolved, FileName)
		info, err := os.Stat(candidate)
		if err == nil && !info.IsDir() {
			return candidate, nil
		}
		parent := filepath.Dir(resolved)
		if parent == resolved {
			return "", nil
		}
		resolved = parent
	}
}

// LoadPath reads and validates a VERSION.json file.
func LoadPath(path string) (*Document, error) {
	var raw map[string]any
	if err := jsonio.LoadPathLenient(path, &raw); err != nil {
		return nil, fmt.Errorf("%s is not valid JSON: %w", path, err)
	}
	doc, err := parseRaw(raw)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	doc.Path = path
	return doc, nil
}

// Load finds VERSION.json from start (or cwd) and loads it.
func Load(start string) (*Document, error) {
	path, err := Find(start)
	if err != nil {
		return nil, err
	}
	if path == "" {
		return nil, fmt.Errorf("no %s found in the current directory or any parent; run 'relkit init' or 'relkit version set'", FileName)
	}
	return LoadPath(path)
}

// Skeleton returns a new official document.
func Skeleton(version string) (*Document, error) {
	if version == "" {
		version = "0.1.0+1"
	}
	parts, err := Parse(version)
	if err != nil {
		return nil, err
	}
	raw := map[string]any{
		"schema":  SchemaID,
		"version": parts.String(),
	}
	return &Document{
		Schema:  SchemaID,
		Version: parts.String(),
		Raw:     raw,
	}, nil
}

// Write persists the document in official schema form, preserving extra Raw fields.
func (d *Document) Write() error {
	if d == nil {
		return fmt.Errorf("nil version document")
	}
	if d.Path == "" {
		return fmt.Errorf("version document has no path")
	}
	parts, err := Parse(d.Version)
	if err != nil {
		return err
	}
	out := map[string]any{}
	for key, value := range d.Raw {
		if key == "schema" || key == "version" || key == "app" {
			continue
		}
		out[key] = value
	}
	out["schema"] = SchemaID
	out["version"] = parts.String()
	d.Schema = SchemaID
	d.Version = parts.String()
	d.Raw = out
	data, err := jsonio.MarshalPretty(out)
	if err != nil {
		return err
	}
	return jsonio.WritePath(d.Path, data)
}

// SetVersion validates and assigns a new version string (does not write).
func (d *Document) SetVersion(version string) error {
	parts, err := Parse(version)
	if err != nil {
		return err
	}
	d.Version = parts.String()
	if d.Raw == nil {
		d.Raw = map[string]any{}
	}
	d.Raw["schema"] = SchemaID
	d.Raw["version"] = d.Version
	d.Schema = SchemaID
	return nil
}

// Bump increments one component. major/minor/patch reset lower number parts but
// never reset build (build only grows via bump build), matching release practice
// where build is the RUP code under codeStrategy version-build.
func (d *Document) Bump(part string) (Parts, error) {
	parts, err := Parse(d.Version)
	if err != nil {
		return Parts{}, err
	}
	switch part {
	case "major":
		parts.Major++
		parts.Minor = 0
		parts.Patch = 0
	case "minor":
		parts.Minor++
		parts.Patch = 0
	case "patch":
		parts.Patch++
	case "build":
		parts.Build++
	default:
		return Parts{}, fmt.Errorf("invalid bump part %q; expected major, minor, patch, or build", part)
	}
	if err := d.SetVersion(parts.String()); err != nil {
		return Parts{}, err
	}
	return parts, nil
}

// Parse accepts x.y.z or x.y.z+build. Bare x.y.z implies build 0.
func Parse(version string) (Parts, error) {
	version = strings.TrimSpace(version)
	if version == "" {
		return Parts{}, fmt.Errorf("version must not be empty")
	}
	core := version
	build := 0
	if i := strings.IndexByte(version, '+'); i >= 0 {
		core = version[:i]
		buildRaw := version[i+1:]
		if buildRaw == "" || strings.ContainsAny(buildRaw, "+-.") {
			return Parts{}, fmt.Errorf("version %q has an invalid +build segment", version)
		}
		parsed, err := strconv.Atoi(buildRaw)
		if err != nil || parsed < 0 {
			return Parts{}, fmt.Errorf("version %q has a non-integer build", version)
		}
		build = parsed
	}
	if strings.Contains(core, "-") {
		return Parts{}, fmt.Errorf("version %q must not contain a pre-release suffix; encode that in notes instead", version)
	}
	bits := strings.Split(core, ".")
	if len(bits) != 3 {
		return Parts{}, fmt.Errorf("version %q must be x.y.z or x.y.z+build", version)
	}
	nums := make([]int, 3)
	for i, bit := range bits {
		if bit == "" || !digitsOnly(bit) {
			return Parts{}, fmt.Errorf("version %q has non-numeric components", version)
		}
		n, err := strconv.Atoi(bit)
		if err != nil || n < 0 {
			return Parts{}, fmt.Errorf("version %q has non-numeric components", version)
		}
		nums[i] = n
	}
	return Parts{Major: nums[0], Minor: nums[1], Patch: nums[2], Build: build}, nil
}

func parseRaw(raw map[string]any) (*Document, error) {
	if raw == nil {
		return nil, fmt.Errorf("document must be a JSON object")
	}

	if schema, _ := raw["schema"].(string); schema == SchemaID {
		version, _ := raw["version"].(string)
		if strings.TrimSpace(version) == "" {
			return nil, fmt.Errorf("missing required field %q", "version")
		}
		parts, err := Parse(version)
		if err != nil {
			return nil, err
		}
		return &Document{
			Schema:  SchemaID,
			Version: parts.String(),
			Raw:     raw,
		}, nil
	}

	// Legacy SvnMergeTool / nested shape: {"app":{"version":"x.y.z+n"}, ...}
	if app, ok := raw["app"].(map[string]any); ok {
		version, _ := app["version"].(string)
		if strings.TrimSpace(version) == "" {
			return nil, fmt.Errorf("legacy app.version is missing")
		}
		parts, err := Parse(version)
		if err != nil {
			return nil, err
		}
		return &Document{
			Schema:  SchemaID,
			Version: parts.String(),
			Raw:     raw,
		}, nil
	}

	if version, ok := raw["version"].(string); ok && strings.TrimSpace(version) != "" {
		parts, err := Parse(version)
		if err != nil {
			return nil, err
		}
		return &Document{
			Schema:  SchemaID,
			Version: parts.String(),
			Raw:     raw,
		}, nil
	}

	return nil, fmt.Errorf("unrecognized %s; expected schema %q with top-level version", FileName, SchemaID)
}

func digitsOnly(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}
