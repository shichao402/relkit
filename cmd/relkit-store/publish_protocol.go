package main

import (
	"encoding/json"
	"net/http"

	"go.firoyang.com/relkit/internal/publishproto"
)

func (c *config) publishWindow() publishproto.Window {
	max := publishproto.Max
	if c.maxPublishProtocol > 0 {
		max = c.maxPublishProtocol
	}
	return publishproto.Window{Min: c.minPublishProtocol, Max: max}
}

func (c *config) servePublishPreflight(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeProtocolJSON(w, http.StatusMethodNotAllowed, publishproto.Decision{
			Error: "method_not_allowed", Message: "use POST",
		})
		return
	}
	if !c.uploadsEnabled() {
		writeProtocolJSON(w, http.StatusMethodNotAllowed, publishproto.Decision{
			Error: "uploads_disabled", Message: "uploads are disabled on this server",
		})
		return
	}
	if !c.authorized(r) {
		w.Header().Set("WWW-Authenticate", `Bearer realm="relkit-serve"`)
		writeProtocolJSON(w, http.StatusUnauthorized, publishproto.Decision{
			Error: "unauthorized", Message: "unauthorized",
		})
		return
	}
	if !c.requirePublishProtocol(w, r) {
		return
	}
	writeProtocolJSON(w, http.StatusOK, publishproto.Success(c.publishWindow(), publishproto.Current, version))
}

func (c *config) requirePublishProtocol(w http.ResponseWriter, r *http.Request) bool {
	return publishproto.Check(w, r, c.publishWindow(), version)
}

func writeProtocolJSON(w http.ResponseWriter, status int, response publishproto.Decision) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(response)
}
