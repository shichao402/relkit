// Package publishproto defines the small HTTP capability contract between the
// relkit publisher and the write endpoints it talks to, both relkit-serve and
// relkit-agent. It is deliberately separate from RUP: update clients never use
// it, and raising this number only gates writers.
package publishproto

import (
	"net/http"
	"strconv"
	"strings"
)

const (
	Current = 2

	ProtocolHeader = "X-Relkit-Publish-Protocol"
	VersionHeader  = "X-Relkit-Version"
	PreflightPath  = "/-/publish/preflight"
)

// PublisherVersion is set by cmd/relkit from its build-time version. Protocol
// compatibility is decided by Current; this value is diagnostic only.
var PublisherVersion = "dev"

// Apply stamps every request a publisher sends to a write endpoint. A request
// without these headers is indistinguishable from one sent by a publisher too
// old to understand the current response shapes, so the server rejects it.
func Apply(h http.Header) {
	h.Set(ProtocolHeader, strconv.Itoa(Current))
	h.Set(VersionHeader, PublisherVersion)
}

// Declared reports the protocol a request claims to speak. Missing or
// unparsable headers read as 0, which is below every enforced minimum.
func Declared(h http.Header) int {
	n, err := strconv.Atoi(strings.TrimSpace(h.Get(ProtocolHeader)))
	if err != nil {
		return 0
	}
	return n
}
