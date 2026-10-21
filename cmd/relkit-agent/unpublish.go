package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"

	"github.com/shichao402/relkit/internal/config"
	"github.com/shichao402/relkit/internal/publish"
	vnextproject "github.com/shichao402/relkit/internal/vnext/project"
	vnextrelease "github.com/shichao402/relkit/internal/vnext/release"
)

// handleUnpublish removes one published version from a channel's signed index
// (POST /v1/unpublish). It shares the product lock with publish and staged
// uploads, so a concurrent publish for the same product is rejected with 409
// instead of racing the index rewrite.
type unpublishRequest struct {
	Product string   `json:"product"`
	Version string   `json:"version"`
	To      []string `json:"to"`
	DryRun  bool     `json:"dryRun"`
}

// loadUnpublishConfig builds a publish config for a version whose staged tree
// may already be gone. ADR 0019 §1 retired the release-policy.json control
// plane: the vNext project document plus the machine publish profile is the
// only authority, and it is readable whether or not the staged tree survives
// because it lives in the product repository, not in .relkit/cache.
func loadUnpublishConfig(pc ProductConfig, product, version string) (*config.Config, string, error) {
	if pc.Profile == "" {
		return nil, "", fmt.Errorf("publish profile path is empty for product %q", product)
	}
	productDocument, err := vnextproject.Load(filepath.Join(pc.Root, vnextrelease.ProjectFile))
	if err != nil {
		return nil, "", fmt.Errorf("vNext project document for %q: %w", product, err)
	}
	if productDocument.Product != product {
		return nil, "", fmt.Errorf("project document declares product %q, not %q", productDocument.Product, product)
	}
	profile, err := config.LoadPublishProfile(pc.Profile)
	if err != nil {
		return nil, "", fmt.Errorf("profile %s: %w", pc.Profile, err)
	}
	cfg, err := mergeVNextPublishConfig(productDocument, profile, pc.Root)
	if err != nil {
		return nil, "", err
	}
	return cfg, "vNext project document + " + pc.Profile, nil
}

func (s *Server) handleUnpublish(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	defer r.Body.Close()
	var req unpublishRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	version, ok := cleanVersion(req.Version)
	if !ok || req.Product == "" {
		http.Error(w, "product and version required", http.StatusBadRequest)
		return
	}
	if !s.requireAuthFor(w, r, req.Product) {
		return
	}
	pc, ok := s.cfg.Products[req.Product]
	if !ok {
		http.Error(w, "unknown product", http.StatusNotFound)
		return
	}

	mu := s.productLock(req.Product)
	if !mu.TryLock() {
		http.Error(w, "publish already in progress for this product", http.StatusConflict)
		return
	}
	defer mu.Unlock()

	cfg, source, err := loadUnpublishConfig(pc, req.Product, version)
	if err != nil {
		http.Error(w, "load unpublish config: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if cfg.Product != "" && cfg.Product != req.Product {
		http.Error(w, "product mismatch with publish config", http.StatusBadRequest)
		return
	}

	var logs []string
	logs = append(logs, "unpublish config: "+source)
	printer := func(line string) { logs = append(logs, line) }
	index, err := publish.Unpublish(cfg, version, req.To, req.DryRun, printer)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error(), "log": logs})
		return
	}
	resp := map[string]any{
		"ok":      true,
		"product": req.Product,
		"version": version,
		"dryRun":  req.DryRun,
		"log":     logs,
	}
	if index != nil {
		resp["sequence"] = index.Sequence
		resp["channel"] = index.Channel
	}
	writeJSON(w, http.StatusOK, resp)
	if !req.DryRun {
		s.triggerSiteRebuild(req.Product, version)
	}
}
