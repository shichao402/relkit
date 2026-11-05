// Package registry owns the single declarative vNext component registry.
package registry

import (
	_ "embed"
	"fmt"
	"strings"

	"github.com/shichao402/relkit/internal/vnext/document"
)

const Schema = "relkit.component-registry/1"

//go:embed components.json
var defaultBytes []byte

type Role string

const (
	RoleInfrastructureBinary Role = "infrastructure-binary"
	RoleProductBinary        Role = "product-binary"
)

type Transport string

const (
	TransportModule       Transport = "module"
	TransportReleaseAsset Transport = "release-asset"
)

// Build declares how a component is produced and named in a relkit release.
// It replaces the retired internal/registry table: one declarative registry
// now owns both the distribution identity (name/role/targets/transport) and
// the release asset naming rule (binary prefix). Every component is a Go
// binary: the product-tree roles (facade/projection) went with the SDKs and
// bindings (ADR 0026), leaving relkit-updater plus its proto as the only host
// surface.
type Build struct {
	Flag         string `json:"flag"`
	GoPackage    string `json:"goPackage"`
	BinaryPrefix string `json:"binaryPrefix"`
}

type Component struct {
	Name        string    `json:"name"`
	Role        Role      `json:"role"`
	Transport   Transport `json:"transport"`
	Targets     []string  `json:"targets"`
	Destination string    `json:"destination,omitempty"`
	Default     bool      `json:"default,omitempty"`
	Build       Build     `json:"build"`
}

type Document struct {
	Schema string `json:"schema"`
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
		case RoleInfrastructureBinary, RoleProductBinary:
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

// validateBuild enforces the release asset naming rule: every component
// carries a build flag (so `relkit build --<flag>` stays derivable from this
// file alone), a goPackage, and a binaryPrefix.
func validateBuild(component Component) error {
	build := component.Build
	if build.Flag == "" {
		return fmt.Errorf("component %q has no build flag", component.Name)
	}
	if build.GoPackage == "" || build.BinaryPrefix == "" {
		return fmt.Errorf("component %q must declare goPackage and binaryPrefix", component.Name)
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
// target, plus the manifest os-arch label that names it. Windows binaries
// append .exe.
func (c Component) ReleaseAssetName(target string) (string, string, error) {
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
