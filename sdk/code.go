package sdk

import (
	"fmt"
	"strconv"
	"strings"
)

// SemverCode encodes major.minor.patch as major*1e6 + minor*1e3 + patch.
// Prerelease / build metadata after '-' or '+' is stripped first.
// Minor and patch must each stay under 1000.
func SemverCode(version string) (int, error) {
	core := strings.Split(strings.Split(strings.TrimPrefix(version, "v"), "+")[0], "-")[0]
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return 0, fmt.Errorf("semver code needs major.minor.patch, got %q", version)
	}
	values := make([]int, 3)
	for i, part := range parts {
		value, err := strconv.Atoi(part)
		if err != nil {
			return 0, fmt.Errorf("version %q has non-numeric components", version)
		}
		values[i] = value
	}
	if values[1] > 999 || values[2] > 999 {
		return 0, fmt.Errorf("version %q overflows semver code encoding (minor/patch must stay under 1000)", version)
	}
	return values[0]*1_000_000 + values[1]*1_000 + values[2], nil
}
