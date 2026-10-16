// Package ops owns machine-side Operator workflows. Product repositories do
// not import this package and never receive its credentials or inventory.
package ops

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	rupv2 "github.com/shichao402/relkit/api/rup/v2"
	"github.com/shichao402/relkit/internal/keys"
	"github.com/shichao402/relkit/internal/vnext/bootstrap"
	"github.com/shichao402/relkit/internal/vnext/catalog"
	"github.com/shichao402/relkit/internal/vnext/document"
	"github.com/shichao402/relkit/internal/vnext/extension"
	"github.com/shichao402/relkit/internal/vnext/operator"
)

const (
	InventorySchema      = "relkit.ops-inventory/1"
	StepResultSchema     = "relkit.ops-step-result/1"
	UpgradeInputSchema   = "relkit.ops-upgrade-step/1"
	RotateInputSchema    = "relkit.ops-rotate-step/1"
	UpgradePlanSchema    = "relkit.ops-upgrade-plan/1"
	OperationStateSchema = "relkit.ops-operation-state/1"
)

type UpgradeAdapters struct {
	Candidate extension.Spec `json:"candidate"`
	Sync      extension.Spec `json:"sync"`
	Deploy    extension.Spec `json:"deploy"`
	Verify    extension.Spec `json:"verify"`
	Activate  extension.Spec `json:"activate"`
	Rollback  extension.Spec `json:"rollback"`
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
	Schema            string                              `json:"schema"`
	Operator          string                              `json:"operator"`
	ActiveProfilePath string                              `json:"activeProfilePath"`
	CatalogPath       string                              `json:"catalogPath"`
	TrustKeyPaths     []string                            `json:"trustKeyPaths"`
	SigningKeyPath    string                              `json:"signingKeyPath"`
	StateDirectory    string                              `json:"stateDirectory"`
	Services          []string                            `json:"services,omitempty"`
	Mirrors           []string                            `json:"mirrors,omitempty"`
	RuntimeProfiles   map[string]bootstrap.RuntimeProfile `json:"runtimeProfiles,omitempty"`
	ProductRoots      map[string][]string                 `json:"productRoots,omitempty"`
	AssetRoots        []string                            `json:"assetRoots,omitempty"`
	BootstrapBinaries map[string]string                   `json:"bootstrapBinaries,omitempty"`
	Upgrade           UpgradeAdapters                     `json:"upgrade"`
	Rotate            map[string]RotateAdapters           `json:"rotate,omitempty"`
}

func LoadInventory(path string) (*Inventory, error) {
	var value Inventory
	if err := document.Load(path, &value); err != nil {
		return nil, err
	}
	base := filepath.Dir(path)
	value.ActiveProfilePath = resolvePath(base, value.ActiveProfilePath)
	value.CatalogPath = resolvePath(base, value.CatalogPath)
	value.SigningKeyPath = resolvePath(base, value.SigningKeyPath)
	value.StateDirectory = resolvePath(base, value.StateDirectory)
	for index := range value.TrustKeyPaths {
		value.TrustKeyPaths[index] = resolvePath(base, value.TrustKeyPaths[index])
	}
	for product, profile := range value.RuntimeProfiles {
		profile.Profile = resolvePath(base, profile.Profile)
		for index := range profile.Roots {
			profile.Roots[index] = resolvePath(base, profile.Roots[index])
		}
		value.RuntimeProfiles[product] = profile
	}
	for product, roots := range value.ProductRoots {
		for index := range roots {
			roots[index] = resolvePath(base, roots[index])
		}
		value.ProductRoots[product] = roots
	}
	for index := range value.AssetRoots {
		value.AssetRoots[index] = resolvePath(base, value.AssetRoots[index])
	}
	for target, binary := range value.BootstrapBinaries {
		value.BootstrapBinaries[target] = resolvePath(base, binary)
	}
	resolveSpec := func(spec *extension.Spec) {
		if spec.WorkingDirectory != "" {
			spec.WorkingDirectory = resolvePath(base, spec.WorkingDirectory)
		}
	}
	for _, spec := range []*extension.Spec{
		&value.Upgrade.Candidate,
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
		i.SigningKeyPath == "" || i.StateDirectory == "" {
		return fmt.Errorf("operator, activeProfilePath, catalogPath, signingKeyPath, and stateDirectory are required")
	}
	if len(i.TrustKeyPaths) == 0 {
		return fmt.Errorf("trustKeyPaths must not be empty")
	}
	for product, profile := range i.RuntimeProfiles {
		if product == "" || profile.Profile == "" || len(profile.Roots) == 0 {
			return fmt.Errorf("runtimeProfiles entries require product, profile, and roots")
		}
	}
	for product, roots := range i.ProductRoots {
		if product == "" || len(roots) == 0 {
			return fmt.Errorf("productRoots entries require product and roots")
		}
		for _, root := range roots {
			if root == "" {
				return fmt.Errorf("productRoots[%s] contains an empty root", product)
			}
		}
	}
	for target, binary := range i.BootstrapBinaries {
		if target == "" || binary == "" {
			return fmt.Errorf("bootstrapBinaries contains an empty target or path")
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

func validateUpgradeAdapters(adapters UpgradeAdapters) error {
	rows := []struct {
		name string
		spec extension.Spec
		kind extension.Kind
	}{
		{"candidate", adapters.Candidate, extension.KindCIAdapter},
		{"sync", adapters.Sync, extension.KindMirrorProducer},
		{"deploy", adapters.Deploy, extension.KindDeployAdapter},
		{"verify", adapters.Verify, extension.KindDeployAdapter},
		{"activate", adapters.Activate, extension.KindDeployAdapter},
		{"rollback", adapters.Rollback, extension.KindDeployAdapter},
	}
	for _, row := range rows {
		if err := validateAdapter(row.name, row.spec, row.kind, UpgradeInputSchema); err != nil {
			return fmt.Errorf("upgrade.%s: %w", row.name, err)
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

func loadSigningKey(path string) (string, []byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", nil, err
	}
	value, err := rupv2.UnmarshalPrivateKey(data)
	if err != nil {
		return "", nil, fmt.Errorf("%s: %w", path, err)
	}
	seed, err := keys.LoadPrivateSeed(*value, path)
	if err != nil {
		return "", nil, err
	}
	if value.KeyId == "" {
		return "", nil, fmt.Errorf("%s has no keyId", path)
	}
	return value.KeyId, seed, nil
}

func loadTrustKeys(paths []string) (map[string]ed25519.PublicKey, error) {
	result := make(map[string]ed25519.PublicKey, len(paths))
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		value, err := rupv2.UnmarshalPublicKey(data)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		publicKey, err := keys.LoadPublicKey(*value, path)
		if err != nil {
			return nil, err
		}
		if _, exists := result[value.KeyId]; exists {
			return nil, fmt.Errorf("duplicate trust keyId %q", value.KeyId)
		}
		result[value.KeyId] = publicKey
	}
	return result, nil
}

func loadTrustedState(inventory Inventory) (*operator.Document, *catalog.Document, map[string]ed25519.PublicKey, error) {
	trusted, err := loadTrustKeys(inventory.TrustKeyPaths)
	if err != nil {
		return nil, nil, nil, err
	}
	profile, err := operator.Load(inventory.ActiveProfilePath)
	if err != nil {
		return nil, nil, nil, err
	}
	if profile.DeploymentID != inventory.Operator {
		return nil, nil, nil, fmt.Errorf("active profile deploymentId %q does not match inventory operator %q", profile.DeploymentID, inventory.Operator)
	}
	if err := profile.Verify(trusted); err != nil {
		return nil, nil, nil, fmt.Errorf("active profile: %w", err)
	}
	distributions, err := catalog.Load(inventory.CatalogPath)
	if err != nil {
		return nil, nil, nil, err
	}
	if err := distributions.Verify(trusted); err != nil {
		return nil, nil, nil, fmt.Errorf("catalog: %w", err)
	}
	return profile, distributions, trusted, nil
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
