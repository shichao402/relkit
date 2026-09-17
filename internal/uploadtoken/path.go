// Package uploadtoken names product upload token files.
//
// Exclusive tokens stay tokens/<product>.token. A shared family is renamed to
// tokens/shared.token (or shared-N on collision). Callers must not invent a
// path from a share-with product id.
package uploadtoken

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const FamilyRelPath = "tokens/shared.token"

func ProductRelPath(product string) string {
	return "tokens/" + product + ".token"
}

func Resolve(configPath, rel string) string {
	if filepath.IsAbs(rel) {
		return rel
	}
	return filepath.Join(filepath.Dir(configPath), filepath.FromSlash(rel))
}

func IsProductNamed(rel string, products []string) bool {
	base := strings.TrimSuffix(filepath.Base(filepath.FromSlash(rel)), ".token")
	if base == "" || base == filepath.Base(filepath.FromSlash(rel)) {
		return false
	}
	for _, id := range products {
		if id != "" && base == id {
			return true
		}
	}
	return false
}

func NextFamilyRel(used []string) string {
	taken := make(map[string]struct{}, len(used))
	for _, name := range used {
		taken[name] = struct{}{}
	}
	if _, ok := taken[FamilyRelPath]; !ok {
		return FamilyRelPath
	}
	for n := 2; ; n++ {
		candidate := fmt.Sprintf("tokens/shared-%d.token", n)
		if _, ok := taken[candidate]; !ok {
			return candidate
		}
	}
}

// PromoteFile moves a product-named token onto a family name. rel is unchanged
// when it is already not named after a product in the family.
func PromoteFile(configPath, rel string, products, used []string) (string, error) {
	if !IsProductNamed(rel, products) {
		return rel, nil
	}
	destRel := NextFamilyRel(used)
	src := Resolve(configPath, rel)
	dest := Resolve(configPath, destRel)
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return "", err
	}
	if err := os.Rename(src, dest); err != nil {
		return "", fmt.Errorf("rename shared token %s -> %s: %w", rel, destRel, err)
	}
	return destRel, nil
}
