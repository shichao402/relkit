package main

import (
	"encoding/json"
	"io"
	"net/http"

	"go.firoyang.com/relkit/internal/publishproto"
)

func (s *Server) publishWindow() publishproto.Window {
	return publishproto.Window{Min: s.cfg.MinPublishProtocol, Max: s.cfg.MaxPublishProtocol}
}

func (s *Server) requirePublishProtocol(w http.ResponseWriter, r *http.Request) bool {
	return publishproto.Check(w, r, s.publishWindow(), version)
}

func (s *Server) handlePublishPreflight(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Product string `json:"product"`
	}
	_ = json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req)
	if req.Product == "" {
		req.Product = r.URL.Query().Get("product")
	}
	if req.Product == "" {
		http.Error(w, "product is required", http.StatusBadRequest)
		return
	}
	if !s.requireAuthFor(w, r, req.Product) {
		return
	}
	writeJSON(w, http.StatusOK, publishproto.Success(s.publishWindow(), publishproto.Current, version))
}
