package updater

import (
	"encoding/json"
	"fmt"

	"github.com/shichao402/relkit/internal/vnext/runtimebootstrap"
)

// decodeBootstrap parses the Runtime Bootstrap text a host forwarded verbatim.
//
// Unknown fields are tolerated on purpose: the bootstrap is a forward-growing
// document and an engine that rejected them would break every host on the
// release that adds a field. What must not be tolerated is a wrong *type* —
// the 2026-10-17 accident wrote rootPublicKey as an array of numbers and every
// host happily shipped it. Encoding those into a typed struct makes that fail
// loudly here, which is exactly the fixture requirement 3.9 asks for.
func decodeBootstrap(raw []byte) (*runtimebootstrap.Document, error) {
	var doc runtimebootstrap.Document
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("invalid runtime bootstrap JSON: %w", err)
	}
	return &doc, nil
}
