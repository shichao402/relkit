// Package bootstrap maps a project's logical Operator reference to
// machine-owned control-plane inputs. The mapping lives outside product
// repositories and contains no private keys or tokens.
package bootstrap

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"github.com/shichao402/relkit/internal/vnext/document"
)

const Schema = "relkit.operator-bootstrap/1"

type RuntimeProfile struct {
	Profile string   `json:"profile"`
	Roots   []string `json:"roots"`
}

type Binary struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type Document struct {
	Schema            string                    `json:"schema"`
	OperatorRef       string                    `json:"operatorRef"`
	OperatorProfile   string                    `json:"operatorProfile"`
	Catalog           string                    `json:"catalog"`
	DistributionRoots []string                  `json:"distributionRoots"`
	ProductRoots      map[string][]string       `json:"productRoots,omitempty"`
	RuntimeProfiles   map[string]RuntimeProfile `json:"runtimeProfiles,omitempty"`
	AssetRoots        []string                  `json:"assetRoots,omitempty"`
	BootstrapBinaries map[string]Binary         `json:"bootstrapBinaries,omitempty"`
}

func Load(path string) (*Document, error) {
	var value Document
	if err := document.Load(path, &value); err != nil {
		return nil, err
	}
	base := filepath.Dir(path)
	value.OperatorProfile = resolve(base, value.OperatorProfile)
	value.Catalog = resolve(base, value.Catalog)
	for index := range value.DistributionRoots {
		value.DistributionRoots[index] = resolve(base, value.DistributionRoots[index])
	}
	for product, roots := range value.ProductRoots {
		for index := range roots {
			roots[index] = resolve(base, roots[index])
		}
		value.ProductRoots[product] = roots
	}
	for product, profile := range value.RuntimeProfiles {
		profile.Profile = resolve(base, profile.Profile)
		for index := range profile.Roots {
			profile.Roots[index] = resolve(base, profile.Roots[index])
		}
		value.RuntimeProfiles[product] = profile
	}
	for index := range value.AssetRoots {
		value.AssetRoots[index] = resolve(base, value.AssetRoots[index])
	}
	for target, binary := range value.BootstrapBinaries {
		binary.Path = resolve(base, binary.Path)
		value.BootstrapBinaries[target] = binary
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
	if d.OperatorRef == "" || d.OperatorProfile == "" || d.Catalog == "" {
		return fmt.Errorf("operatorRef, operatorProfile, and catalog are required")
	}
	if len(d.DistributionRoots) == 0 {
		return fmt.Errorf("distributionRoots must not be empty")
	}
	for product, roots := range d.ProductRoots {
		if product == "" || len(roots) == 0 {
			return fmt.Errorf("productRoots entries require product and roots")
		}
		for _, root := range roots {
			if root == "" {
				return fmt.Errorf("productRoots[%s] contains an empty root", product)
			}
		}
	}
	for product, profile := range d.RuntimeProfiles {
		if product == "" || profile.Profile == "" || len(profile.Roots) == 0 {
			return fmt.Errorf("runtimeProfiles entries require product, profile, and roots")
		}
	}
	for target, binary := range d.BootstrapBinaries {
		if target == "" || binary.Path == "" || binary.SHA256 == "" {
			return fmt.Errorf("bootstrapBinaries entries require target, path, and sha256")
		}
	}
	return nil
}

func DefaultPath(operatorRef string) (string, error) {
	home, err := Home()
	if err != nil {
		return "", err
	}
	return Path(home, operatorRef), nil
}

func Home() (string, error) {
	if value := os.Getenv("RELKIT_HOME"); value != "" {
		return filepath.Abs(value)
	}
	config, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(config, "relkit"), nil
}

func Path(home, operatorRef string) string {
	sum := sha256.Sum256([]byte(operatorRef))
	return filepath.Join(home, "operators", hex.EncodeToString(sum[:]), "bootstrap.json")
}

func Write(home string, value Document) (string, error) {
	if home == "" {
		var err error
		home, err = Home()
		if err != nil {
			return "", err
		}
	}
	if err := value.Validate(); err != nil {
		return "", err
	}
	path := Path(home, value.OperatorRef)
	if err := document.WriteAtomic(path, value); err != nil {
		return "", err
	}
	return path, nil
}

func resolve(base, value string) string {
	if value == "" || filepath.IsAbs(value) {
		return value
	}
	return filepath.Clean(filepath.Join(base, value))
}
