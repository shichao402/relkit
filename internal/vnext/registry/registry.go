// Package registry owns the single declarative vNext component registry.
package registry

import (
	_ "embed"
	"fmt"
	"slices"

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

type Component struct {
	Name        string                 `json:"name"`
	Role        Role                   `json:"role"`
	Transport   Transport              `json:"transport"`
	Targets     []string               `json:"targets"`
	Destination string                 `json:"destination,omitempty"`
	Default     bool                   `json:"default,omitempty"`
	Requires    []contracts.Capability `json:"requires,omitempty"`
	Provides    []contracts.Capability `json:"provides,omitempty"`
}

type Document struct {
	Schema     string      `json:"schema"`
	Components []Component `json:"components"`
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
	seen := map[string]bool{}
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
	}
	if seen["host-scripts"] {
		return fmt.Errorf("vNext registry must not contain retired host-scripts")
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
