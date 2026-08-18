// Package sdk is the official Go client for RUP v2 (protobuf wire format).
//
// Install:
//
//	go get cnb.cool/shichao402/relkit/sdk@latest
//
// Agent onboarding (first read when wiring a host): see AGENT-QUICKSTART.md
// in this directory, and docs/agent/README.md at the repo root for the
// toolchain + SDK cascade entrypoint.
//
// The client resolves entryUrls → signed directory (optional), fetches a signed
// Index envelope, verifies Ed25519 signatures, selects the next version along
// the upgrade chain, and downloads a hash-verified artifact. Single-binary
// self-replace helpers live in sdk/apply; directory-swap apply remains host/Dart.
package sdk
