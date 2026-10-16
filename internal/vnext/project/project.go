// Package project owns relkit.project/1, the only vNext control-plane
// declaration committed by a product repository.
package project

import (
	"fmt"

	"github.com/shichao402/relkit/internal/model"
	"github.com/shichao402/relkit/internal/vnext/canonical"
	"github.com/shichao402/relkit/internal/vnext/contracts"
	"github.com/shichao402/relkit/internal/vnext/document"
)

const Schema = "relkit.project/1"

type Role string

const (
	RolePublisher         Role = "publisher"
	RoleUpdaterHost       Role = "updater-host"
	RoleSubscriber        Role = "subscriber"
	RoleUpdaterProjection Role = "updater-projection"
)

type Command struct {
	Command         []string `json:"command"`
	ExtensionSchema string   `json:"extensionSchema,omitempty"`
	TimeoutSeconds  int      `json:"timeoutSeconds,omitempty"`
	EnvAllowlist    []string `json:"envAllowlist,omitempty"`
	Permissions     []string `json:"permissions,omitempty"`
}

type Subscription struct {
	ProductRef      string `json:"productRef"`
	RootFingerprint string `json:"rootFingerprint"`
}

type RuntimeProfilePublication struct {
	RootFingerprint string         `json:"rootFingerprint"`
	RUPSchemaSet    string         `json:"rupSchemaSet"`
	Recovery        map[string]any `json:"recovery,omitempty"`
}

type CapabilityFingerprint struct {
	OSArch            []string               `json:"osArch"`
	Facades           []string               `json:"facades"`
	Projections       []string               `json:"projections"`
	Topology          string                 `json:"topology"`
	Subscriptions     []string               `json:"subscriptions"`
	RequiredContracts []contracts.Capability `json:"requiredContracts"`
}

type Document struct {
	Schema                      string                     `json:"schema"`
	Product                     string                     `json:"product"`
	Channels                    []string                   `json:"channels,omitempty"`
	VersionPolicy               map[string]any             `json:"versionPolicy,omitempty"`
	SelectorDimensions          []string                   `json:"selectorDimensions,omitempty"`
	OperatorRef                 string                     `json:"operatorRef"`
	TrustFingerprint            string                     `json:"trustFingerprint"`
	Roles                       []Role                     `json:"roles"`
	ArtifactProducer            *Command                   `json:"artifactProducer,omitempty"`
	VersionProvider             *Command                   `json:"versionProvider,omitempty"`
	RuntimeProfileSubscriptions []Subscription             `json:"runtimeProfileSubscriptions,omitempty"`
	RuntimeProfilePublication   *RuntimeProfilePublication `json:"runtimeProfilePublication,omitempty"`
	CapabilityFingerprint       CapabilityFingerprint      `json:"capabilityFingerprint"`
}

func Load(path string) (*Document, error) {
	var value Document
	if err := document.Load(path, &value); err != nil {
		return nil, err
	}
	if err := value.Validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &value, nil
}

func (d Document) Validate() error {
	if d.Schema != Schema {
		return fmt.Errorf("schema is %q, expected %q", d.Schema, Schema)
	}
	if err := model.CheckIdentifier(d.Product, "product"); err != nil {
		return err
	}
	if d.OperatorRef == "" {
		return fmt.Errorf("operatorRef must not be empty")
	}
	if err := canonical.ValidateDigest(d.TrustFingerprint); err != nil {
		return fmt.Errorf("trustFingerprint: %w", err)
	}
	if len(d.Roles) == 0 {
		return fmt.Errorf("roles must not be empty")
	}
	seenRoles := map[Role]bool{}
	for _, role := range d.Roles {
		switch role {
		case RolePublisher, RoleUpdaterHost, RoleSubscriber, RoleUpdaterProjection:
		default:
			return fmt.Errorf("unknown role %q", role)
		}
		if seenRoles[role] {
			return fmt.Errorf("duplicate role %q", role)
		}
		seenRoles[role] = true
	}
	if seenRoles[RolePublisher] && d.ArtifactProducer == nil {
		return fmt.Errorf("publisher role requires artifactProducer")
	}
	if seenRoles[RoleSubscriber] && len(d.RuntimeProfileSubscriptions) == 0 {
		return fmt.Errorf("subscriber role requires runtimeProfileSubscriptions")
	}
	if len(d.RuntimeProfileSubscriptions) > 0 && !seenRoles[RoleSubscriber] {
		return fmt.Errorf("runtimeProfileSubscriptions require subscriber role")
	}
	if d.RuntimeProfilePublication != nil {
		if !seenRoles[RolePublisher] {
			return fmt.Errorf("runtimeProfilePublication requires publisher role")
		}
		if err := canonical.ValidateDigest(d.RuntimeProfilePublication.RootFingerprint); err != nil {
			return fmt.Errorf("runtimeProfilePublication.rootFingerprint: %w", err)
		}
		if d.RuntimeProfilePublication.RUPSchemaSet == "" {
			return fmt.Errorf("runtimeProfilePublication.rupSchemaSet must not be empty")
		}
	}
	if seenRoles[RoleUpdaterHost] && len(d.CapabilityFingerprint.Facades) == 0 {
		return fmt.Errorf("updater-host role requires at least one facade")
	}
	if seenRoles[RoleUpdaterProjection] && len(d.CapabilityFingerprint.Projections) == 0 {
		return fmt.Errorf("updater-projection role requires at least one projection")
	}
	for name, command := range map[string]*Command{
		"artifactProducer": d.ArtifactProducer,
		"versionProvider":  d.VersionProvider,
	} {
		if command != nil && len(command.Command) == 0 {
			return fmt.Errorf("%s.command must not be empty", name)
		}
		if command != nil && command.ExtensionSchema != "" && command.ExtensionSchema != "relkit.extension/1" {
			return fmt.Errorf("%s.extensionSchema is %q, expected relkit.extension/1", name, command.ExtensionSchema)
		}
		if command != nil && command.TimeoutSeconds < 0 {
			return fmt.Errorf("%s.timeoutSeconds must not be negative", name)
		}
		if command != nil {
			seenEnvironment := map[string]bool{}
			for _, variable := range command.EnvAllowlist {
				if variable == "" || seenEnvironment[variable] {
					return fmt.Errorf("%s.envAllowlist contains an empty or duplicate variable", name)
				}
				seenEnvironment[variable] = true
			}
		}
	}
	seenSubscriptions := map[string]bool{}
	for _, subscription := range d.RuntimeProfileSubscriptions {
		if err := model.CheckIdentifier(subscription.ProductRef, "runtime profile subscription productRef"); err != nil {
			return err
		}
		if seenSubscriptions[subscription.ProductRef] {
			return fmt.Errorf("duplicate runtime profile subscription %q", subscription.ProductRef)
		}
		seenSubscriptions[subscription.ProductRef] = true
		if err := canonical.ValidateDigest(subscription.RootFingerprint); err != nil {
			return fmt.Errorf("runtime profile %q rootFingerprint: %w", subscription.ProductRef, err)
		}
	}
	fingerprintSubscriptions := map[string]bool{}
	for _, subscription := range d.CapabilityFingerprint.Subscriptions {
		if subscription == "" || fingerprintSubscriptions[subscription] {
			return fmt.Errorf("capabilityFingerprint.subscriptions contains an empty or duplicate product")
		}
		fingerprintSubscriptions[subscription] = true
	}
	if len(fingerprintSubscriptions) != len(seenSubscriptions) {
		return fmt.Errorf("capabilityFingerprint.subscriptions must match runtimeProfileSubscriptions")
	}
	for subscription := range seenSubscriptions {
		if !fingerprintSubscriptions[subscription] {
			return fmt.Errorf("capabilityFingerprint.subscriptions is missing %q", subscription)
		}
	}
	for name, values := range map[string][]string{
		"selectorDimensions":                d.SelectorDimensions,
		"capabilityFingerprint.osArch":      d.CapabilityFingerprint.OSArch,
		"capabilityFingerprint.facades":     d.CapabilityFingerprint.Facades,
		"capabilityFingerprint.projections": d.CapabilityFingerprint.Projections,
	} {
		seen := map[string]bool{}
		for _, value := range values {
			if value == "" || seen[value] {
				return fmt.Errorf("%s contains an empty or duplicate value", name)
			}
			seen[value] = true
		}
	}
	if len(d.CapabilityFingerprint.OSArch) == 0 {
		return fmt.Errorf("capabilityFingerprint.osArch must not be empty")
	}
	if d.CapabilityFingerprint.Topology == "" {
		return fmt.Errorf("capabilityFingerprint.topology must not be empty")
	}
	vector := contracts.Vector{Requires: d.CapabilityFingerprint.RequiredContracts}
	if err := vector.Validate(); err != nil {
		return fmt.Errorf("capabilityFingerprint: %w", err)
	}
	return nil
}

func (d Document) Digest() (string, error) {
	if err := d.Validate(); err != nil {
		return "", err
	}
	return canonical.Digest(d)
}
