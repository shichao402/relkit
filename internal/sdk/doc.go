// Package sdk holds the engine's protocol helpers: semver codes, fetch,
// artifact download, state, and apply helpers.
//
// It is internal despite the name: hosts integrate the relkit-updater binary
// over its ProtoJSON contract (ADR 0026) and never link relkit code. The
// in-process engine lives in internal/inprocess and is not part of this
// package.
package sdk
