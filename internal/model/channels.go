package model

import "slices"

// standardChannels is the one channel set every product on a relkit server
// publishes to. It is a server fact, not a product declaration: hosts do not
// choose channels, so registering a product needs nothing beyond its id and the
// server can sign the product's first Runtime Profile on its own.
//
// Order matters. The Runtime Profile carries it as allowedChannels and the
// updater compares that list positionally, so changing the order is a
// client-visible change and must not happen casually.
var standardChannels = []string{"dev", "stable"}

// StandardChannels returns a fresh copy of the server channel set.
func StandardChannels() []string {
	return append([]string(nil), standardChannels...)
}

// IsStandardChannel reports whether channel belongs to the server channel set.
func IsStandardChannel(channel string) bool {
	return slices.Contains(standardChannels, channel)
}
