// Package sdk holds protocol helpers hosts may import: semver codes, fetch,
// artifact download, state, and apply helpers.
//
// Check/download/apply for products goes through sdk/updaterfacade and the
// relkit-updater sidecar. The in-process engine lives in internal/inprocess
// and is not part of this package.
//
//	replace go.firoyang.com/relkit => ./third_party/relkit
package sdk
