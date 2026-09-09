package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"go.firoyang.com/relkit/internal/backends"
	"go.firoyang.com/relkit/internal/config"
	"go.firoyang.com/relkit/internal/model"
)

const casCredentialTTL = time.Hour

type casCredentialBlob struct {
	SHA256 string   `json:"sha256"`
	Size   int64    `json:"size"`
	URLs   []string `json:"urls,omitempty"`
}

type casCredentialRequest struct {
	Product string              `json:"product"`
	Blobs   []casCredentialBlob `json:"blobs"`
}

type casUploadDocument struct {
	SHA256   string                `json:"sha256"`
	Size     int64                 `json:"size"`
	Requests []backends.CASRequest `json:"requests"`
}

type casCredentialResponse struct {
	ExpiresAt time.Time           `json:"expiresAt"`
	Uploads   []casUploadDocument `json:"uploads"`
}

func (s *Server) handleCASCredentials(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	defer r.Body.Close()
	var req casCredentialRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if req.Product == "" {
		http.Error(w, "product is required", http.StatusBadRequest)
		return
	}
	if !s.requireAuthFor(w, r, req.Product) {
		return
	}
	if len(req.Blobs) == 0 {
		http.Error(w, "product and blobs required", http.StatusBadRequest)
		return
	}
	if _, ok := s.cfg.Products[req.Product]; !ok {
		http.Error(w, "unknown product", http.StatusNotFound)
		return
	}
	if len(req.Blobs) > s.cfg.MaxFiles {
		http.Error(w, "too many blobs", http.StatusBadRequest)
		return
	}

	backend, ingest, err := s.productIngest(req.Product)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	expiresAt := time.Now().UTC().Add(casCredentialTTL)
	response := casCredentialResponse{ExpiresAt: expiresAt, Uploads: []casUploadDocument{}}
	seen := map[string]int64{}
	for _, blob := range req.Blobs {
		key, err := model.CasKey(blob.SHA256)
		if err != nil || blob.Size < 0 || blob.Size > s.cfg.MaxUpload {
			http.Error(w, "invalid blob sha256 or size", http.StatusBadRequest)
			return
		}
		if previousSize, ok := seen[key]; ok {
			if previousSize != blob.Size {
				http.Error(w, "same blob sha256 has conflicting sizes", http.StatusBadRequest)
				return
			}
			continue
		}
		seen[key] = blob.Size
		if blobAlreadyAvailable(ingest, key, blob) {
			continue
		}

		upload, err := s.authorizeCASUpload(backend, key, blob.Size, expiresAt)
		if err != nil {
			http.Error(w, "authorize cas upload: "+err.Error(), http.StatusBadRequest)
			return
		}
		response.Uploads = append(response.Uploads, casUploadDocument{
			SHA256:   strings.ToLower(blob.SHA256),
			Size:     blob.Size,
			Requests: upload.Requests,
		})
		if exp := upload.ExpiresAt(); !exp.IsZero() && exp.Before(response.ExpiresAt) {
			response.ExpiresAt = exp
		}
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) productIngest(product string) (backends.Backend, backends.Ingest, error) {
	pc := s.cfg.Products[product]
	profile, err := config.LoadPublishProfile(pc.Profile)
	if err != nil {
		return nil, nil, fmt.Errorf("profile %s: %w", pc.Profile, err)
	}
	if profile.Product != product {
		return nil, nil, fmt.Errorf("profile product %q does not match %q", profile.Product, product)
	}
	cfg := &config.Config{
		Root:      pc.Root,
		Product:   profile.Product,
		Backends:  profile.Backends,
		PublishTo: profile.PublishTo,
	}
	for _, name := range profile.PublishTo {
		backend, err := backends.Create(name, cfg, pc.Root)
		if err != nil {
			return nil, nil, err
		}
		if ingest, ok := backend.(backends.Ingest); ok {
			return backend, ingest, nil
		}
	}
	return nil, nil, fmt.Errorf("product %q has no ingest backend in publishTo; use whole staged upload", product)
}

func blobAlreadyAvailable(ingest backends.Ingest, key string, blob casCredentialBlob) bool {
	if size, exists, err := ingest.Head(key); err == nil && exists && size == blob.Size {
		return true
	}
	return false
}

func (s *Server) authorizeCASUpload(backend backends.Backend, key string, size int64, expiresAt time.Time) (*backends.CASUpload, error) {
	authorizer, ok := backend.(backends.CASUploadAuthorizer)
	if !ok {
		return nil, fmt.Errorf("ingest backend %q cannot authorize CAS uploads", backend.Name())
	}
	return authorizer.AuthorizeCASUpload(backends.CASUploadRequest{
		Key:  key,
		Size: size,
		TTL:  time.Until(expiresAt),
	})
}
