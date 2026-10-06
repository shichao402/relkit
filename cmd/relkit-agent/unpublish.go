package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/shichao402/relkit/internal/config"
	"github.com/shichao402/relkit/internal/publish"
	"github.com/shichao402/relkit/internal/stage"
)

// handleUnpublish removes one published version from a channel's signed index
// (POST /v1/unpublish). It shares the product lock with publish and staged
// uploads, so a concurrent publish for the same product is rejected with 409
// instead of racing the index rewrite.
type unpublishRequest struct {
	Product string `json:"product"`
	Version string `json:"version"`
	To      []string `json:"to"`
	DryRun  bool   `json:"dryRun"`
}

// loadUnpublishConfig builds a publish config for a version whose staged tree
// may already be gone. It prefers the staged release-policy.json (the same
// source publish uses); when no staged tree remains it falls back to the most
// recent staged release's policy, and finally to the machine profile with the
// policy reconstructed from nothing (channels/retain from the agent's view).
func loadUnpublishConfig(pc ProductConfig, product, version string) (*config.Config, string, error) {
	policyPath := stage.ReleasePolicyPath(pc.Root, version)
	if _, err := os.Stat(policyPath); err == nil {
		policy, err := config.LoadProductPolicy(policyPath)
		if err != nil {
			return nil, "", fmt.Errorf("release-policy.json at %s: %w", policyPath, err)
		}
		profile, err := config.LoadPublishProfile(pc.Profile)
		if err != nil {
			return nil, "", fmt.Errorf("profile %s: %w", pc.Profile, err)
		}
		cfg, err := config.MergeProductPolicy(policy, profile, pc.Root)
		if err != nil {
			return nil, "", err
		}
		return cfg, "staged release-policy.json + " + pc.Profile, nil
	}

	// The staged tree for this version is gone. Any remaining staged release
	// carries the same repository-owned policy (product, channels, retain,
	// public keys), so use the newest one instead of guessing.
	versions, err := stage.StagedVersions(pc.Root)
	if err == nil {
		for _, candidate := range versions {
			candidatePath := stage.ReleasePolicyPath(pc.Root, candidate)
			policy, err := config.LoadProductPolicy(candidatePath)
			if err != nil {
				continue
			}
			if policy.Product != product {
				continue
			}
			profile, err := config.LoadPublishProfile(pc.Profile)
			if err != nil {
				return nil, "", fmt.Errorf("profile %s: %w", pc.Profile, err)
			}
			cfg, err := config.MergeProductPolicy(policy, profile, pc.Root)
			if err != nil {
				return nil, "", err
			}
			return cfg, fmt.Sprintf("release-policy.json from staged %s (its own staged tree is gone) + %s", candidate, pc.Profile), nil
		}
	}

	// No staged policy anywhere: reject. The signed index is only as good as
	// the policy that verifies it, and inventing one from the profile alone
	// (which carries no public keys) would let unpublish strip verification
	// from the index it rewrites.
	return nil, "", fmt.Errorf("no staged release-policy.json for %s under %s; re-stage the version or publish a replacement first (the policy carries the public keys that verify the index)", version, pc.Root)
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
