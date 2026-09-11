// Package sdk is the official Go client for RUP v2 (protobuf wire format).
//
// Install the checksum-pinned SDK Release artifact with
// scripts/host/relkit_host.py install (relkit_consume.py).
//
//	replace go.firoyang.com/relkit => ./third_party/relkit
//
// Do not go get go.firoyang.com/relkit (no vanity / GOPROXY path).
//
// Host wiring is product-repo python scripts/host/relkit_host.py.
//
// The client resolves entryUrls → signed directory (optional), fetches a signed
// Index envelope, verifies Ed25519 signatures, selects the next version along
// the upgrade chain, and downloads a hash-verified artifact. Single-binary
// self-replace helpers live in sdk/apply; directory-swap apply remains host/Dart.
package sdk
