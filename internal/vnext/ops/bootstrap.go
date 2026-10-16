package ops

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/shichao402/relkit/internal/fsx"
	"github.com/shichao402/relkit/internal/vnext/bootstrap"
	"github.com/shichao402/relkit/internal/vnext/canonical"
)

// ProvisionBootstrap publishes only public/read-only references needed by the
// canonical wrapper. Private signing keys and service credentials never enter
// this document.
func ProvisionBootstrap(inventory Inventory, home string) (string, error) {
	if _, _, _, err := loadTrustedState(inventory); err != nil {
		return "", operationError("operator.control_plane_invalid", "bootstrap", "Operator control plane is not trusted", nil, err)
	}
	for _, path := range append(append([]string{}, inventory.TrustKeyPaths...), inventory.AssetRoots...) {
		if _, err := os.Stat(path); err != nil {
			return "", fmt.Errorf("bootstrap input %s: %w", path, err)
		}
	}
	for product, profile := range inventory.RuntimeProfiles {
		if _, err := os.Stat(profile.Profile); err != nil {
			return "", fmt.Errorf("runtime profile %s: %w", product, err)
		}
		for _, root := range profile.Roots {
			if _, err := os.Stat(root); err != nil {
				return "", fmt.Errorf("runtime profile %s root: %w", product, err)
			}
		}
	}
	for product, roots := range inventory.ProductRoots {
		for _, root := range roots {
			if _, err := os.Stat(root); err != nil {
				return "", fmt.Errorf("product %s root: %w", product, err)
			}
		}
	}
	if home == "" {
		var err error
		home, err = bootstrap.Home()
		if err != nil {
			return "", err
		}
	}
	binaries := make(map[string]bootstrap.Binary, len(inventory.BootstrapBinaries))
	bootstrapPath := bootstrap.Path(home, inventory.Operator)
	for target, source := range inventory.BootstrapBinaries {
		data, err := os.ReadFile(source)
		if err != nil {
			return "", fmt.Errorf("bootstrap binary %s: %w", target, err)
		}
		sum := sha256.Sum256(data)
		name := "relkit"
		if strings.HasPrefix(target, "windows-") {
			name += ".exe"
		}
		destination := filepath.Join(filepath.Dir(bootstrapPath), "bootstrap", target, name)
		if err := writeBinaryAtomic(destination, data); err != nil {
			return "", err
		}
		binaries[target] = bootstrap.Binary{
			Path:   destination,
			SHA256: canonical.DigestPrefix + fmt.Sprintf("%x", sum[:]),
		}
	}
	value := bootstrap.Document{
		Schema:            bootstrap.Schema,
		OperatorRef:       inventory.Operator,
		OperatorProfile:   inventory.ActiveProfilePath,
		Catalog:           inventory.CatalogPath,
		DistributionRoots: append([]string(nil), inventory.TrustKeyPaths...),
		ProductRoots:      cloneStringLists(inventory.ProductRoots),
		RuntimeProfiles:   cloneRuntimeProfiles(inventory.RuntimeProfiles),
		AssetRoots:        append([]string(nil), inventory.AssetRoots...),
		BootstrapBinaries: binaries,
	}
	return bootstrap.Write(home, value)
}

func cloneStringLists(value map[string][]string) map[string][]string {
	result := make(map[string][]string, len(value))
	for name, entries := range value {
		result[name] = append([]string(nil), entries...)
	}
	return result
}

func cloneRuntimeProfiles(value map[string]bootstrap.RuntimeProfile) map[string]bootstrap.RuntimeProfile {
	result := make(map[string]bootstrap.RuntimeProfile, len(value))
	for product, profile := range value {
		result[product] = bootstrap.RuntimeProfile{
			Profile: profile.Profile,
			Roots:   append([]string(nil), profile.Roots...),
		}
	}
	return result
}

func writeBinaryAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, data, 0o755); err != nil {
		return err
	}
	defer os.Remove(temporary)
	if err := os.Chmod(temporary, 0o755); err != nil {
		return err
	}
	if err := fsx.Replace(temporary, path); err != nil {
		return err
	}
	return nil
}
