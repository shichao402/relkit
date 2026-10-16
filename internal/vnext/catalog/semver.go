package catalog

import (
	"fmt"
	"strconv"
	"strings"
)

type semVersion struct {
	core       [3]uint64
	prerelease []string
}

func parseSemVersion(raw string) (semVersion, error) {
	value := strings.TrimPrefix(strings.TrimSpace(raw), "v")
	if value == "" {
		return semVersion{}, fmt.Errorf("version %q is empty", raw)
	}
	if plus := strings.IndexByte(value, '+'); plus >= 0 {
		if plus == len(value)-1 || !validIdentifiers(value[plus+1:], false) {
			return semVersion{}, fmt.Errorf("version %q has invalid build metadata", raw)
		}
		value = value[:plus]
	}
	var prerelease []string
	if dash := strings.IndexByte(value, '-'); dash >= 0 {
		if dash == len(value)-1 || !validIdentifiers(value[dash+1:], true) {
			return semVersion{}, fmt.Errorf("version %q has invalid prerelease", raw)
		}
		prerelease = strings.Split(value[dash+1:], ".")
		value = value[:dash]
	}
	parts := strings.Split(value, ".")
	if len(parts) != 3 {
		return semVersion{}, fmt.Errorf("version %q must have major.minor.patch", raw)
	}
	var parsed semVersion
	parsed.prerelease = prerelease
	for i, part := range parts {
		if part == "" || !digits(part) || (len(part) > 1 && part[0] == '0') {
			return semVersion{}, fmt.Errorf("version %q has invalid numeric component %q", raw, part)
		}
		number, err := strconv.ParseUint(part, 10, 64)
		if err != nil {
			return semVersion{}, fmt.Errorf("version %q has invalid numeric component %q", raw, part)
		}
		parsed.core[i] = number
	}
	return parsed, nil
}

func validIdentifiers(value string, prerelease bool) bool {
	parts := strings.Split(value, ".")
	for _, part := range parts {
		if part == "" {
			return false
		}
		numeric := true
		for _, char := range part {
			if (char < '0' || char > '9') && (char < 'A' || char > 'Z') &&
				(char < 'a' || char > 'z') && char != '-' {
				return false
			}
			if char < '0' || char > '9' {
				numeric = false
			}
		}
		if prerelease && numeric && len(part) > 1 && part[0] == '0' {
			return false
		}
	}
	return true
}

func digits(value string) bool {
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return value != ""
}

// CompareVersions implements SemVer precedence. It returns -1, 0, or 1.
// Build metadata is accepted but does not affect precedence.
func CompareVersions(left, right string) (int, error) {
	a, err := parseSemVersion(left)
	if err != nil {
		return 0, err
	}
	b, err := parseSemVersion(right)
	if err != nil {
		return 0, err
	}
	for i := range a.core {
		if a.core[i] < b.core[i] {
			return -1, nil
		}
		if a.core[i] > b.core[i] {
			return 1, nil
		}
	}
	if len(a.prerelease) == 0 && len(b.prerelease) == 0 {
		return 0, nil
	}
	if len(a.prerelease) == 0 {
		return 1, nil
	}
	if len(b.prerelease) == 0 {
		return -1, nil
	}
	limit := min(len(a.prerelease), len(b.prerelease))
	for i := 0; i < limit; i++ {
		comparison := compareIdentifier(a.prerelease[i], b.prerelease[i])
		if comparison != 0 {
			return comparison, nil
		}
	}
	switch {
	case len(a.prerelease) < len(b.prerelease):
		return -1, nil
	case len(a.prerelease) > len(b.prerelease):
		return 1, nil
	default:
		return 0, nil
	}
}

func compareIdentifier(left, right string) int {
	leftNumeric, rightNumeric := digits(left), digits(right)
	if leftNumeric && rightNumeric {
		left = strings.TrimLeft(left, "0")
		right = strings.TrimLeft(right, "0")
		if len(left) < len(right) {
			return -1
		}
		if len(left) > len(right) {
			return 1
		}
	}
	if leftNumeric != rightNumeric {
		if leftNumeric {
			return -1
		}
		return 1
	}
	switch {
	case left < right:
		return -1
	case left > right:
		return 1
	default:
		return 0
	}
}

func isPrerelease(version string) (bool, error) {
	parsed, err := parseSemVersion(version)
	if err != nil {
		return false, err
	}
	return len(parsed.prerelease) > 0, nil
}
