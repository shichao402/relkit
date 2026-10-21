// Package registry owns the single declarative vNext component registry.
package registry

import (
	_ "embed"
	"fmt"
	"slices"
	"strings"

	"github.com/shichao402/relkit/internal/vnext/contracts"
	"github.com/shichao402/relkit/internal/vnext/document"
)

const Schema = "relkit.component-registry/1"

//go:embed components.json
var defaultBytes []byte

type Role string

const (
	RoleInfrastructureBinary Role = "infrastructure-binary"
	RoleProductBinary        Role = "product-binary"
	RoleFacade               Role = "facade"
	RoleProjection           Role = "projection"
)

type Transport string

const (
	TransportModule       Transport = "module"
	TransportReleaseAsset Transport = "release-asset"
)

// PackKind identifies how a product-tree component is zipped.
type PackKind string

const (
	PackTrackedTree PackKind = "tracked-tree"
	PackWorkingTree PackKind = "working-tree"
	PackListedFiles PackKind = "listed-files"
	PackGoDeps      PackKind = "go-deps"
)

// Pack declares the zip shape of one product-tree component.
type Pack struct {
	Kind            PackKind    `json:"kind"`
	Files           []string    `json:"files,omitempty"`
	ExcludeParts    []string    `json:"excludeParts,omitempty"`
	ExcludeSuffixes []string    `json:"excludeSuffixes,omitempty"`
	ExcludePaths    []string    `json:"excludePaths,omitempty"`
	Extras          [][2]string `json:"extras,omitempty"`
	GoEntrypoints   []string    `json:"goEntrypoints,omitempty"`
	GoModule        string      `json:"goModule,omitempty"`
}

// Build declares how a component is produced and named in a relkit release.
// It replaces the retired internal/registry table: one declarative registry
// now owns both the distribution identity (name/role/targets/transport) and
// the release asset naming rule (binary prefix or archive name).
type Build struct {
	Flag           string   `json:"flag"`
	GoPackage      string   `json:"goPackage,omitempty"`
	BinaryPrefix   string   `json:"binaryPrefix,omitempty"`
	Archive        string   `json:"archive,omitempty"`
	Source         string   `json:"source,omitempty"`
	Pack           *Pack    `json:"pack,omitempty"`
	UpdaterProcess string   `json:"updaterProcess,omitempty"`
	FacadePaths    []string `json:"facadePaths,omitempty"`
}

type Component struct {
	Name        string                 `json:"name"`
	Role        Role                   `json:"role"`
	Transport   Transport              `json:"transport"`
	Targets     []string               `json:"targets"`
	Destination string                 `json:"destination,omitempty"`
	Default     bool                   `json:"default,omitempty"`
	Requires    []contracts.Capability `json:"requires,omitempty"`
	Provides    []contracts.Capability `json:"provides,omitempty"`
	Build       Build                  `json:"build"`
}

type Document struct {
	Schema     string      `json:"schema"`
	// BuildTargets lists every platform `relkit build` cross-compiles for.
	// darwin-universal is absent on purpose: it is lipo-composed from the
	// thin darwin binaries, never compiled.
	BuildTargets []string    `json:"buildTargets"`
	Components   []Component `json:"components"`
}

func Default() (*Document, error) {
	var value Document
	if err := document.Decode(defaultBytes, &value); err != nil {
		return nil, fmt.Errorf("embedded component registry: %w", err)
	}
	if err := value.Validate(); err != nil {
		return nil, fmt.Errorf("embedded component registry: %w", err)
	}
	return &value, nil
}

func (d Document) Validate() error {
	if d.Schema != Schema {
		return fmt.Errorf("schema is %q, expected %q", d.Schema, Schema)
	}
	if len(d.Components) == 0 {
		return fmt.Errorf("components must not be empty")
	}
	if len(d.BuildTargets) == 0 {
		return fmt.Errorf("buildTargets must not be empty")
	}
	for index, target := range d.BuildTargets {
		parts := strings.SplitN(target, "-", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return fmt.Errorf("buildTargets[%d] %q is not os-arch", index, target)
		}
	}
	seen := map[string]bool{}
	flags := map[string]bool{}
	for index, component := range d.Components {
		if component.Name == "" {
			return fmt.Errorf("components[%d].name must not be empty", index)
		}
		if seen[component.Name] {
			return fmt.Errorf("duplicate component %q", component.Name)
		}
		seen[component.Name] = true
		switch component.Role {
		case RoleInfrastructureBinary, RoleProductBinary, RoleFacade, RoleProjection:
		default:
			return fmt.Errorf("component %q has unknown role %q", component.Name, component.Role)
		}
		switch component.Transport {
		case TransportModule, TransportReleaseAsset:
		default:
			return fmt.Errorf("component %q has unknown transport %q", component.Name, component.Transport)
		}
		if len(component.Targets) == 0 {
			return fmt.Errorf("component %q has no targets", component.Name)
		}
		if component.Role != RoleInfrastructureBinary && component.Destination == "" {
			return fmt.Errorf("component %q has no destination", component.Name)
		}
		if err := (contracts.Vector{Requires: component.Requires, Provides: component.Provides}).Validate(); err != nil {
			return fmt.Errorf("component %q: %w", component.Name, err)
		}
		if err := validateBuild(component); err != nil {
			return err
		}
		if flags[component.Build.Flag] {
			return fmt.Errorf("duplicate build flag %q", component.Build.Flag)
		}
		flags[component.Build.Flag] = true
	}
	if seen["host-scripts"] {
		return fmt.Errorf("vNext registry must not contain retired host-scripts")
	}
	return nil
}

// validateBuild enforces the release asset naming rule each component needs.
// Role decides the shape: binaries carry a goPackage plus a binaryPrefix (or
// an archive for portable trees); every component carries a build flag so
// `relkit build --<flag>` stays derivable from this file alone.
func validateBuild(component Component) error {
	build := component.Build
	if build.Flag == "" {
		return fmt.Errorf("component %q has no build flag", component.Name)
	}
	switch component.Role {
	case RoleInfrastructureBinary, RoleProductBinary:
		if build.GoPackage == "" || build.BinaryPrefix == "" {
			return fmt.Errorf("component %q must declare goPackage and binaryPrefix", component.Name)
		}
		if build.Archive != "" || build.Source != "" || build.Pack != nil {
			return fmt.Errorf("component %q is a binary and must not declare archive/source/pack", component.Name)
		}
		if build.UpdaterProcess != "" || len(build.FacadePaths) > 0 {
			return fmt.Errorf("component %q is a binary and must not declare host surface fields", component.Name)
		}
	case RoleFacade, RoleProjection:
		if build.Archive == "" || build.Source == "" || build.Pack == nil {
			return fmt.Errorf("component %q must declare archive, source, and pack", component.Name)
		}
		if build.GoPackage != "" || build.BinaryPrefix != "" {
			return fmt.Errorf("component %q is a product tree and must not declare goPackage/binaryPrefix", component.Name)
		}
		if err := validatePack(component.Name, build); err != nil {
			return err
		}
	}
	return nil
}

func validatePack(name string, build Build) error {
	pack := build.Pack
	kind := pack.Kind
	if kind == "" {
		return fmt.Errorf("component %q pack has no kind", name)
	}
	switch kind {
	case PackTrackedTree:
		if len(pack.Files) > 0 || len(pack.ExcludeParts) > 0 || len(pack.ExcludeSuffixes) > 0 ||
			len(pack.GoEntrypoints) > 0 || pack.GoModule != "" {
			return fmt.Errorf("component %q tracked-tree pack carries fields of another kind", name)
		}
	case PackWorkingTree:
		if len(pack.Files) > 0 || len(pack.ExcludeSuffixes) > 0 ||
			len(pack.GoEntrypoints) > 0 || pack.GoModule != "" {
			return fmt.Errorf("component %q working-tree pack carries fields of another kind", name)
		}
	case PackListedFiles:
		if len(pack.Files) == 0 {
			return fmt.Errorf("component %q listed-files pack requires files", name)
		}
		if len(pack.ExcludeParts) > 0 || len(pack.ExcludeSuffixes) > 0 ||
			len(pack.GoEntrypoints) > 0 || pack.GoModule != "" {
			return fmt.Errorf("component %q listed-files pack carries fields of another kind", name)
		}
	case PackGoDeps:
		if len(pack.GoEntrypoints) == 0 || pack.GoModule == "" {
			return fmt.Errorf("component %q go-deps pack requires entrypoints and module", name)
		}
		if len(pack.ExcludeParts) > 0 || len(pack.ExcludePaths) > 0 {
			return fmt.Errorf("component %q go-deps pack carries fields of another kind", name)
		}
	default:
		return fmt.Errorf("component %q has unknown pack kind %q", name, kind)
	}
	if build.UpdaterProcess != "" && len(build.FacadePaths) == 0 {
		return fmt.Errorf("component %q updaterProcess requires facadePaths", name)
	}
	if build.UpdaterProcess == "" && len(build.FacadePaths) > 0 {
		return fmt.Errorf("component %q facadePaths require updaterProcess", name)
	}
	return nil
}

func (d Document) ByName(name string) (Component, bool) {
	for _, component := range d.Components {
		if component.Name == name {
			return component, true
		}
	}
	return Component{}, false
}

// ByBuildFlag resolves a `relkit build --<flag>` selector.
func (d Document) ByBuildFlag(flag string) (Component, bool) {
	for _, component := range d.Components {
		if component.Build.Flag == flag {
			return component, true
		}
	}
	return Component{}, false
}

// InRoles returns components whose role is one of the given values, in
// registry order. Build and distribute iterate the same declaration instead
// of two parallel tables.
func (d Document) InRoles(roles ...Role) []Component {
	wanted := map[Role]bool{}
	for _, role := range roles {
		wanted[role] = true
	}
	var selected []Component
	for _, component := range d.Components {
		if wanted[component.Role] {
			selected = append(selected, component)
		}
	}
	return selected
}

// ReleaseAssetName returns the release attachment name for the component at
// target, plus the manifest os-arch label that names it. Product trees use
// "all" and report any-any; binaries append .exe on Windows.
func (c Component) ReleaseAssetName(target string) (string, string, error) {
	if c.Role == RoleFacade || c.Role == RoleProjection {
		if c.Build.Archive == "" {
			return "", "", fmt.Errorf("portable component has no archive")
		}
		return c.Build.Archive, "any-any", nil
	}
	if c.Build.BinaryPrefix == "" {
		return "", "", fmt.Errorf("targeted component has no binary prefix")
	}
	name := c.Build.BinaryPrefix + "-" + target
	if strings.HasPrefix(target, "windows-") {
		name += ".exe"
	}
	return name, target, nil
}

// ShellName is the executable file name a product-binary materializes as.
func (c Component) ShellName(target string) (string, error) {
	if c.Role != RoleProductBinary {
		return "", fmt.Errorf("%s is not a product binary", c.Name)
	}
	name := c.Build.BinaryPrefix
	if target != "" && strings.HasPrefix(target, "windows-") {
		name += ".exe"
	}
	return name, nil
}

// Required returns component names for the declared platform/language surface.
func (d Document) Required(platform string, facades, projections []string, updaterHost bool) ([]string, error) {
	required := []string{"cli"}
	if updaterHost {
		required = append(required, "updater")
	}
	for _, facade := range facades {
		required = append(required, "sdk-"+facade)
	}
	for _, projection := range projections {
		if projection == "ts" || projection == "typescript" {
			required = append(required, "bindings-ts")
		} else {
			required = append(required, "bindings-"+projection)
		}
	}
	required = unique(required)
	for _, name := range required {
		component, ok := d.ByName(name)
		if !ok {
			return nil, fmt.Errorf("required component %q is not registered", name)
		}
		if !slices.Contains(component.Targets, "all") && !slices.Contains(component.Targets, platform) {
			return nil, fmt.Errorf("component %q does not support platform %q", name, platform)
		}
	}
	return required, nil
}

func unique(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}
