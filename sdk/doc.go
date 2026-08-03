// Package sdk is the official Go client for RUP v2 (protobuf wire format).
//
// Install:
//
//	go get github.com/shichao402/relkit/sdk@latest
//
// Agent onboarding (first read when wiring a host): see AGENT-QUICKSTART.md
// in this directory, and docs/agent/README.md at the repo root for the
// toolchain + SDK cascade entrypoint.
//
// The client fetches a signed Index envelope, verifies Ed25519 signatures over
// the Index protobuf bytes, selects the next version along the upgrade chain,
// and downloads a hash-verified artifact. Applying the update is left to the host.
package sdk
