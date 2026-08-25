package main

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"cnb.cool/shichao402/relkit/internal/publishproto"
)

type publishProtocolResponse struct {
	OK               bool   `json:"ok"`
	Protocol         int    `json:"protocol,omitempty"`
	MinProtocol      int    `json:"minProtocol"`
	ServerVersion    string `json:"serverVersion"`
	Error            string `json:"error,omitempty"`
	Message          string `json:"message,omitempty"`
	PublisherVersion string `json:"publisherVersion,omitempty"`
}

// servePublishPreflight lets a current publisher fail before it hashes, signs,
// or uploads large artifacts. Enforcement still happens on every PUT because
// an old publisher does not know this endpoint exists.
func (c *config) servePublishPreflight(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeProtocolJSON(w, http.StatusMethodNotAllowed, publishProtocolResponse{
			Error: "method_not_allowed", Message: "use POST",
		})
		return
	}
	if c.uploadToken == nil {
		writeProtocolJSON(w, http.StatusMethodNotAllowed, publishProtocolResponse{
			Error: "uploads_disabled", Message: "uploads are disabled on this server",
		})
		return
	}
	if !c.authorized(r) {
		w.Header().Set("WWW-Authenticate", `Bearer realm="relkit-serve"`)
		writeProtocolJSON(w, http.StatusUnauthorized, publishProtocolResponse{
			Error: "unauthorized", Message: "unauthorized",
		})
		return
	}
	if !c.requirePublishProtocol(w, r) {
		return
	}
	writeProtocolJSON(w, http.StatusOK, publishProtocolResponse{
		OK:            true,
		Protocol:      publishproto.Current,
		MinProtocol:   c.minPublishProtocol,
		ServerVersion: version,
	})
}

// requirePublishProtocol is called only after authentication. This avoids
// revealing deployment policy to anonymous probes and, more importantly, runs
// before upload creates a directory or temporary file.
func (c *config) requirePublishProtocol(w http.ResponseWriter, r *http.Request) bool {
	if c.minPublishProtocol <= 0 {
		return true
	}
	received, err := strconv.Atoi(strings.TrimSpace(r.Header.Get(publishproto.ProtocolHeader)))
	if err == nil && received >= c.minPublishProtocol {
		return true
	}
	if err != nil {
		received = 0
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Upgrade", "relkit-publish/"+strconv.Itoa(c.minPublishProtocol))
	writeProtocolJSON(w, http.StatusUpgradeRequired, publishProtocolResponse{
		MinProtocol:      c.minPublishProtocol,
		ServerVersion:    version,
		Error:            "publisher_upgrade_required",
		Message:          "upgrade relkit publisher and retry",
		PublisherVersion: r.Header.Get(publishproto.VersionHeader),
		Protocol:         received,
	})
	return false
}

func writeProtocolJSON(w http.ResponseWriter, status int, response publishProtocolResponse) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(response)
}
