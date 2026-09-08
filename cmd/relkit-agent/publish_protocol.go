package main

import (
	"encoding/json"
	"net/http"
	"strconv"

	"cnb.cool/shichao402/relkit/internal/publishproto"
)

type upgradeRequiredResponse struct {
	Error            string `json:"error"`
	Message          string `json:"message"`
	MinProtocol      int    `json:"minProtocol"`
	Protocol         int    `json:"protocol"`
	ServerVersion    string `json:"serverVersion"`
	PublisherVersion string `json:"publisherVersion,omitempty"`
}

// requirePublishProtocol refuses publishers that predate the current write
// contract. Without it a stale publisher still authenticates, reads a document
// whose shape it does not know, and silently degrades: the pre-4bf302b cas-put
// looked for a `putUrl` that no longer exists, resolved the empty string
// against the agent origin, and sent `PUT /`. Failing the handshake turns that
// class of drift into one legible error instead of a mangled request.
//
// Called only after authentication, so an anonymous probe cannot read the
// deployment's protocol floor.
func (s *Server) requirePublishProtocol(w http.ResponseWriter, r *http.Request) bool {
	minProtocol := s.cfg.MinPublishProtocol
	if minProtocol <= 0 {
		return true
	}
	declared := publishproto.Declared(r.Header)
	if declared >= minProtocol {
		return true
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Upgrade", "relkit-publish/"+strconv.Itoa(minProtocol))
	w.WriteHeader(http.StatusUpgradeRequired)
	_ = json.NewEncoder(w).Encode(upgradeRequiredResponse{
		Error: "publisher_upgrade_required",
		Message: "this relkit publisher is too old for the agent's write contract; " +
			"rebuild it from the relkit revision this agent was deployed from",
		MinProtocol:      minProtocol,
		Protocol:         declared,
		ServerVersion:    version,
		PublisherVersion: r.Header.Get(publishproto.VersionHeader),
	})
	return false
}
