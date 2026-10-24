// Package ops owns machine-side Operator workflows: the deployment operations a
// server administrator runs. Product repositories do not import this package
// and never receive its credentials or inventory.
//
// After the epoch there is exactly one signing identity per server (the root
// resolved from the custody manifest), so this package no longer carries a
// distribution trust key set, a candidate lane, or a product root table
// (requirements 10.5, 14.5).
package ops

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/shichao402/relkit/internal/custody"
	"github.com/shichao402/relkit/internal/vnext/catalog"
	"github.com/shichao402/relkit/internal/vnext/document"
	"github.com/shichao402/relkit/internal/vnext/extension"
	"github.com/shichao402/relkit/internal/vnext/operator"
)

const (
	InventorySchema      = "relkit.ops-inventory/2"
	StepResultSchema     = "relkit.ops-step-result/1"
	UpgradeInputSchema   = "relkit.ops-upgrade-step/2"
	RotateInputSchema    = "relkit.ops-rotate-step/1"
	UpgradePlanSchema    = "relkit.ops-upgrade-plan/2"
	OperationStateSchema = "relkit.ops-operation-state/1"
)

type UpgradeAdapters struct {
	Sync     extension.Spec `json:"sync"`
	Deploy   extension.Spec `json:"deploy"`
	Verify   extension.Spec `json:"verify"`
	Activate extension.Spec `json:"activate"`
	Rollback extension.Spec `json:"rollback"`
}

type RotateAdapters struct {
	Mint      extension.Spec `json:"mint"`
	Update    extension.Spec `json:"updateSecretStore"`
	CheckRead extension.Spec `json:"checkServiceRead"`
	Reload    extension.Spec `json:"reload"`
	Health    extension.Spec `json:"healthAndWrite"`
	Revoke    extension.Spec `json:"revokeOld"`
}

type Inventory struct {
	Schema            string                    `json:"schema"`
	Operator          string                    `json:"operator"`
	ActiveProfilePath string                    `json:"activeProfilePath"`
	CatalogPath       string                    `json:"catalogPath"`
	CustodyManifest   string                    `json:"custodyManifest"`
	RootKeyID         string                    `json:"rootKeyId"`
	StateDirectory    string                    `json:"stateDirectory"`
	Services          []string                  `json:"services,omitempty"`
	Mirrors           []string                  `json:"mirrors,omitempty"`
	Upgrade           UpgradeAdapters           `json:"upgrade"`
	Rotate            map[string]RotateAdapters `json:"rotate,omitempty"`
}

func LoadInventory(path string) (*Inventory, error) {
	var value Inventory
	if err := document.Load(path, &value); err != nil {
		return nil, err
	}
	base := filepath.Dir(path)
	value.ActiveProfilePath = resolvePath(base, value.ActiveProfilePath)
	value.CatalogPath = resolvePath(base, value.CatalogPath)
	value.CustodyManifest = resolvePath(base, value.CustodyManifest)
	value.StateDirectory = resolvePath(base, value.StateDirectory)
	resolveSpec := func(spec *extension.Spec) {
		if spec.WorkingDirectory != "" {
			spec.WorkingDirectory = resolvePath(base, spec.WorkingDirectory)
		}
	}
	for _, spec := range []*extension.Spec{
		&value.Upgrade.Sync,
		&value.Upgrade.Deploy,
		&value.Upgrade.Verify,
		&value.Upgrade.Activate,
		&value.Upgrade.Rollback,
	} {
		resolveSpec(spec)
	}
	for name, workflow := range value.Rotate {
		for _, spec := range []*extension.Spec{
			&workflow.Mint,
			&workflow.Update,
			&workflow.CheckRead,
			&workflow.Reload,
			&workflow.Health,
			&workflow.Revoke,
		} {
			resolveSpec(spec)
		}
		value.Rotate[name] = workflow
	}
	if err := value.Validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &value, nil
}

func (i Inventory) Validate() error {
	if i.Schema != InventorySchema {
		return fmt.Errorf("schema is %q, expected %q", i.Schema, InventorySchema)
	}
	if i.Operator == "" || i.ActiveProfilePath == "" || i.CatalogPath == "" ||
		i.CustodyManifest == "" || i.RootKeyID == "" || i.StateDirectory == "" {
		return fmt.Errorf("operator, activeProfilePath, catalogPath, custodyManifest, rootKeyId, and stateDirectory are required")
	}
	if err := validateUniqueEntries("services", i.Services); err != nil {
		return err
	}
	if err := validateUniqueEntries("mirrors", i.Mirrors); err != nil {
		return err
	}
	for _, mirror := range i.Mirrors {
		parsed, err := url.Parse(mirror)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") ||
			parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
			return fmt.Errorf("mirrors entry must be an HTTP(S) URL without credentials, query, or fragment")
		}
	}
	if err := validateUpgradeAdapters(i.Upgrade); err != nil {
		return err
	}
	for name, workflow := range i.Rotate {
		if name == "" {
			return fmt.Errorf("rotate contains an empty credential name")
		}
		if err := validateRotateAdapters(name, workflow); err != nil {
			return err
		}
	}
	return nil
}

func validateUniqueEntries(field string, values []string) error {
	seen := map[string]bool{}
	for _, value := range values {
		if value == "" || seen[value] {
			return fmt.Errorf("%s contains an empty or duplicate entry", field)
		}
		seen[value] = true
	}
	return nil
}

func validateUpgradeAdapters(adapters UpgradeAdapters) error {
	rows := []struct {
		name string
		spec extension.Spec
	}{
		{"sync", adapters.Sync},
		{"deploy", adapters.Deploy},
		{"verify", adapters.Verify},
		{"activate", adapters.Activate},
		{"rollback", adapters.Rollback},
	}
	for _, row := range rows {
		if err := validateAdapter(row.name, row.spec, extension.KindDeployAdapter, UpgradeInputSchema); err != nil {
			return fmt.Errorf("upgrade.%s: %w", row.name, err)
		}
	}
	// The mirror sync adapter is optional: a single-server deployment with no
	// mirror has nothing to synchronise.
	if adapters.Sync.Kind != "" {
		if err := validateAdapter("sync", adapters.Sync, extension.KindMirrorProducer, UpgradeInputSchema); err != nil {
			return fmt.Errorf("upgrade.sync: %w", err)
		}
	}
	return nil
}

func validateRotateAdapters(name string, adapters RotateAdapters) error {
	rows := []struct {
		name string
		spec extension.Spec
	}{
		{"mint", adapters.Mint},
		{"updateSecretStore", adapters.Update},
		{"checkServiceRead", adapters.CheckRead},
		{"reload", adapters.Reload},
		{"healthAndWrite", adapters.Health},
		{"revokeOld", adapters.Revoke},
	}
	for _, row := range rows {
		if err := validateAdapter(row.name, row.spec, extension.KindRotateAdapter, RotateInputSchema); err != nil {
			return fmt.Errorf("rotate.%s.%s: %w", name, row.name, err)
		}
	}
	return nil
}

func validateAdapter(name string, spec extension.Spec, kind extension.Kind, inputSchema string) error {
	if err := spec.Validate(); err != nil {
		return err
	}
	if spec.Kind != kind {
		return fmt.Errorf("kind is %q, expected %q", spec.Kind, kind)
	}
	if spec.StdinSchema != inputSchema || spec.StdoutSchema != StepResultSchema {
		return fmt.Errorf("schemas are %q -> %q, expected %q -> %q", spec.StdinSchema, spec.StdoutSchema, inputSchema, StepResultSchema)
	}
	return nil
}

type StepResult struct {
	Schema    string         `json:"schema"`
	OK        bool           `json:"ok"`
	SecretRef string         `json:"secretRef,omitempty"`
	Evidence  map[string]any `json:"evidence,omitempty"`
}

func runStep(ctx context.Context, phase string, spec extension.Spec, input any) (*StepResult, error) {
	var result StepResult
	if err := extension.Run(ctx, spec, input, &result); err != nil {
		return nil, fmt.Errorf("%s adapter: %w", phase, err)
	}
	if result.Schema != StepResultSchema {
		return nil, fmt.Errorf("%s adapter returned schema %q, expected %q", phase, result.Schema, StepResultSchema)
	}
	if !result.OK {
		return nil, fmt.Errorf("%s adapter reported failure", phase)
	}
	return &result, nil
}

// root resolves the one signing identity this server holds. Key material comes
// from the custody manifest by keyId; nothing is read from the inventory
// (requirements 15.1, 15.3).
func (i Inventory) root() (*custody.Root, error) {
	manifest, err := custody.Load(i.CustodyManifest)
	if err != nil {
		return nil, err
	}
	root, err := manifest.Root(i.RootKeyID)
	if err != nil {
		return nil, err
	}
	if root.Fingerprint == "" {
		return nil, fmt.Errorf("custody entry %q has no registered fingerprint", i.RootKeyID)
	}
	return root, nil
}

// trustedRoot returns the single public key both the Operator Profile and the
// Catalog must verify against.
func (i Inventory) trustedRoot() (map[string]ed25519.PublicKey, error) {
	root, err := i.root()
	if err != nil {
		return nil, err
	}
	return map[string]ed25519.PublicKey{root.KeyID: root.PublicKey}, nil
}

func loadTrustedState(inventory Inventory) (*operator.Document, *catalog.Document, error) {
	trusted, err := inventory.trustedRoot()
	if err != nil {
		return nil, nil, err
	}
	profile, err := operator.Load(inventory.ActiveProfilePath)
	if err != nil {
		return nil, nil, err
	}
	if profile.DeploymentID != inventory.Operator {
		return nil, nil, fmt.Errorf("active profile deploymentId %q does not match inventory operator %q", profile.DeploymentID, inventory.Operator)
	}
	if err := profile.Verify(trusted); err != nil {
		return nil, nil, fmt.Errorf("active profile: %w", err)
	}
	value, err := catalog.Load(inventory.CatalogPath)
	if err != nil {
		return nil, nil, err
	}
	if err := value.Verify(trusted); err != nil {
		return nil, nil, fmt.Errorf("catalog: %w", err)
	}
	return profile, value, nil
}

func resolvePath(base, value string) string {
	if value == "" || filepath.IsAbs(value) {
		return value
	}
	return filepath.Clean(filepath.Join(base, value))
}

func validateOperationID(value string) error {
	if value == "" {
		return fmt.Errorf("operation id must not be empty")
	}
	if strings.ContainsAny(value, `/\`) || value == "." || value == ".." {
		return fmt.Errorf("operation id %q is not a safe file name", value)
	}
	return nil
}
