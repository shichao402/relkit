// Package publishproto defines the small HTTP capability contract between the
// relkit publisher and relkit-serve. It is deliberately separate from RUP:
// update clients never use it, and raising this number only gates writers.
package publishproto

const (
	Current = 2

	ProtocolHeader = "X-Relkit-Publish-Protocol"
	VersionHeader  = "X-Relkit-Version"
	PreflightPath  = "/-/publish/preflight"
)

// PublisherVersion is set by cmd/relkit from its build-time version. Protocol
// compatibility is decided by Current; this value is diagnostic only.
var PublisherVersion = "dev"
