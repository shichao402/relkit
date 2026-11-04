package main

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/shichao402/relkit/internal/model"
	"github.com/shichao402/relkit/internal/publish"
)

// releaseCodePath is where a host claims the release code for its next build
// (ADR 0025). Release codes order releases and name installs on the client, so
// the only authority that can hand out a fresh one is the server that already
// holds every published code.
const releaseCodePath = "/v1/release-code"

type releaseCodeRequest struct {
	Product string `json:"product"`
}

// releaseCodeState is the answer to both verbs. LastAllocated is the counter;
// HighestPublished is the largest code on any server channel. A claim returns
// the newly allocated Code as well.
type releaseCodeState struct {
	OK               bool   `json:"ok"`
	Product          string `json:"product"`
	Code             int64  `json:"code,omitempty"`
	LastAllocated    int64  `json:"lastAllocated"`
	HighestPublished int64  `json:"highestPublished"`
}

// handleReleaseCode serves:
//
//	POST /v1/release-code {"product":"<id>"}  claim the next code
//	GET  /v1/release-code?product=<id>        read the counter and published floor
//
// Both need the product's publisher token. GET exists so `relkit release` can
// refuse a stale code before it uploads hundreds of megabytes.
func (s *Server) handleReleaseCode(w http.ResponseWriter, r *http.Request) {
	var product string
	switch r.Method {
	case http.MethodGet:
		product = strings.TrimSpace(r.URL.Query().Get("product"))
	case http.MethodPost:
		defer r.Body.Close()
		var req releaseCodeRequest
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&req); err != nil {
			writeReleaseCodeError(w, http.StatusBadRequest, "release_code.invalid_request", "invalid json")
			return
		}
		product = strings.TrimSpace(req.Product)
	default:
		w.Header().Set("Allow", "GET, POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if product == "" {
		writeReleaseCodeError(w, http.StatusBadRequest, "release_code.invalid_request", "product is required")
		return
	}
	if !s.requireAuthFor(w, r, product) {
		return
	}
	store := s.adminStore()
	if store == nil {
		writeReleaseCodeError(w, http.StatusServiceUnavailable, "release_code.state_unavailable", "agent admin state is not loaded")
		return
	}
	registration, ok := store.Product(product)
	if !ok {
		writeReleaseCodeError(w, http.StatusNotFound, "release_code.unknown_product", "unknown product")
		return
	}
	highest, status, code, err := s.highestPublishedCode(product)
	if err != nil {
		writeReleaseCodeError(w, status, code, err.Error())
		return
	}
	state := releaseCodeState{OK: true, Product: product, LastAllocated: registration.LastReleaseCode, HighestPublished: highest}
	if r.Method == http.MethodGet {
		writeJSON(w, http.StatusOK, state)
		return
	}
	allocated, err := store.AllocateReleaseCode(product, highest)
	if err != nil {
		writeReleaseCodeError(w, http.StatusInternalServerError, "release_code.persist_failed", err.Error())
		return
	}
	state.Code = allocated
	state.LastAllocated = allocated
	writeJSON(w, http.StatusOK, state)
}

// highestPublishedCode reads every server channel's verified index for product.
// It builds the publish config from server facts only, exactly as the Runtime
// Profile signer does, because a claim happens before any staged tree exists.
func (s *Server) highestPublishedCode(product string) (int64, int, string, error) {
	if s.cfg.Root == nil {
		return 0, http.StatusConflict, "release_code.root_unavailable", errString("agent server root is not resolved; configure rootKeyId and custodyManifest")
	}
	pc, ok := s.product(product)
	if !ok {
		return 0, http.StatusNotFound, "release_code.unknown_product", errString("unknown product")
	}
	profile, _, err := pc.loadPublishProfile()
	if err != nil {
		return 0, http.StatusConflict, "release_code.publish_profile_invalid", err
	}
	cfg, err := mergeVNextPublishConfig(nil, profile, pc.Root, s.cfg.Root)
	if err != nil {
		return 0, http.StatusConflict, "release_code.config_invalid", err
	}
	highest, err := publish.HighestPublishedCode(cfg, model.StandardChannels())
	if err != nil {
		return 0, http.StatusBadGateway, "release_code.index_unreadable", err
	}
	return highest, 0, "", nil
}

type errString string

func (e errString) Error() string { return string(e) }

func writeReleaseCodeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"ok": false, "code": code, "error": message})
}
