package main

import (
	"path"
	"strings"
)

// legacyStatsFileName is the download-counter file serve and the early store
// kept at the tree root. Nothing writes it any more (download counting was
// removed; it will come back as its own design, not as a file in the tree), but
// boxes that ran the old build still have it, so it stays hidden like the
// other internal files.
const legacyStatsFileName = ".relkit-serve-stats.json"

// reservedServeKey is the store's hiding rule: internal files that sit in the
// tree are neither listed nor served nor writable over HTTP.
func reservedServeKey(name string) bool {
	base := path.Base(name)
	return base == legacyStatsFileName || strings.HasPrefix(base, legacyStatsFileName+".") ||
		base == casKeyFileName || strings.HasPrefix(base, casKeyFileName+".") ||
		reservedAdminKey(name)
}
