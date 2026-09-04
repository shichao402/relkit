package humansize

import (
	"fmt"
	"strconv"
	"strings"
)

// Parse accepts a decimal integer, optionally with a size suffix.
// Longest suffix wins, so "8GiB" is 8<<30 and never 8 bytes.
func Parse(text string) (int64, error) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return 0, fmt.Errorf("empty size")
	}
	units := []struct {
		suffix string
		factor int64
	}{
		{"tib", 1 << 40},
		{"gib", 1 << 30},
		{"mib", 1 << 20},
		{"kib", 1 << 10},
		{"tb", 1000 * 1000 * 1000 * 1000},
		{"gb", 1000 * 1000 * 1000},
		{"mb", 1000 * 1000},
		{"kb", 1000},
		{"g", 1 << 30},
		{"m", 1 << 20},
		{"k", 1 << 10},
		{"b", 1},
	}
	lower := strings.ToLower(trimmed)
	for _, unit := range units {
		if !strings.HasSuffix(lower, unit.suffix) {
			continue
		}
		number := strings.TrimSpace(lower[:len(lower)-len(unit.suffix)])
		n, err := strconv.ParseInt(number, 10, 64)
		if err != nil || n < 0 {
			return 0, fmt.Errorf("invalid size %q", text)
		}
		return n * unit.factor, nil
	}
	n, err := strconv.ParseInt(lower, 10, 64)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("invalid size %q", text)
	}
	return n, nil
}
