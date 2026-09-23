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
	Product  string              `json:"product"`
	PartSize int64               `json:"partSize,omitempty"`
	Blobs    []casCredentialBlob `json:"blobs"`
}

type casUploadDocument struct {
	SHA256   string                `json:"sha256"`
	Size     int64                 `json:"size"`
	UploadID string                `json:"uploadId,omitempty"`
	Requests []backends.CASRequest `json:"requests"`
}

type casPartReport struct {
	PartNumber int    `json:"partNumber"`
	ETag       string `json:"etag"`
}

type casMultipartFinish struct {
	Product  string          `json:"product"`
	SHA256   string          `json:"sha256"`
	UploadID string          `json:"uploadId"`
	Parts    []casPartReport `json:"parts,omitempty"`
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

		upload, err := s.authorizeBlobUpload(r, backend, key, blob.Size, req.PartSize, expiresAt)
		if err != nil {
			http.Error(w, "authorize cas upload: "+err.Error(), http.StatusBadRequest)
			return
		}
		response.Uploads = append(response.Uploads, casUploadDocument{
			SHA256:   strings.ToLower(blob.SHA256),
			Size:     blob.Size,
			UploadID: upload.UploadID,
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

func (s *Server) authorizeBlobUpload(r *http.Request, backend backends.Backend, key string, size, requestedPartSize int64, expiresAt time.Time) (*backends.CASUpload, error) {
	partSize, multipart, err := s.casMultipartPlan(requestedPartSize, size)
	if err != nil {
		return nil, err
	}
	if multipart && s.negotiatedProtocol(r) >= 3 {
		if authorizer, ok := backend.(backends.CASMultipartAuthorizer); ok {
			return authorizer.AuthorizeCASMultipart(backends.CASMultipartRequest{
				Key:      key,
				Size:     size,
				PartSize: partSize,
				TTL:      time.Until(expiresAt),
			})
		}
	}
	return s.authorizeCASUpload(backend, key, size, expiresAt)
}

func (s *Server) casMultipartPlan(requested, total int64) (partSize int64, multipart bool, err error) {
	partSize = requested
	if partSize <= 0 {
		partSize = s.cfg.PartSize
	}
	if partSize < s.cfg.MinPartSize {
		partSize = s.cfg.MinPartSize
	}
	if s.cfg.MaxPartSize > 0 && partSize > s.cfg.MaxPartSize {
		partSize = s.cfg.MaxPartSize
	}
	if total <= partSize {
		return partSize, false, nil
	}
	parts := (total + partSize - 1) / partSize
	if s.cfg.MaxParts > 0 && parts > int64(s.cfg.MaxParts) {
		partSize = (total + int64(s.cfg.MaxParts) - 1) / int64(s.cfg.MaxParts)
		if partSize < s.cfg.MinPartSize {
			partSize = s.cfg.MinPartSize
		}
		if partSize > s.cfg.MaxPartSize {
			return 0, false, fmt.Errorf("object needs more than %d parts at maxPartSize", s.cfg.MaxParts)
		}
		if total <= partSize {
			return partSize, false, nil
		}
	}
	return partSize, true, nil
}

func (s *Server) handleCASComplete(w http.ResponseWriter, r *http.Request) {
	finish, backend, key, ok := s.beginCASMultipart(w, r)
	if !ok {
		return
	}
	authorizer, ok := backend.(backends.CASMultipartAuthorizer)
	if !ok {
		http.Error(w, "ingest backend cannot complete multipart CAS uploads", http.StatusBadRequest)
		return
	}
	if len(finish.Parts) == 0 {
		http.Error(w, "parts are required", http.StatusBadRequest)
		return
	}
	parts := make([]backends.CASPart, 0, len(finish.Parts))
	for _, part := range finish.Parts {
		if part.PartNumber < 1 || strings.TrimSpace(part.ETag) == "" {
			http.Error(w, "each part needs a partNumber and etag", http.StatusBadRequest)
			return
		}
		parts = append(parts, backends.CASPart{PartNumber: part.PartNumber, ETag: part.ETag})
	}
	if err := authorizer.CompleteCASMultipart(key, finish.UploadID, parts); err != nil {
		http.Error(w, "complete cas upload: "+err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleCASAbort(w http.ResponseWriter, r *http.Request) {
	finish, backend, key, ok := s.beginCASMultipart(w, r)
	if !ok {
		return
	}
	authorizer, ok := backend.(backends.CASMultipartAuthorizer)
	if !ok {
		http.Error(w, "ingest backend cannot abort multipart CAS uploads", http.StatusBadRequest)
		return
	}
	if err := authorizer.AbortCASMultipart(key, finish.UploadID); err != nil {
		http.Error(w, "abort cas upload: "+err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) beginCASMultipart(w http.ResponseWriter, r *http.Request) (casMultipartFinish, backends.Backend, string, bool) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return casMultipartFinish{}, nil, "", false
	}
	defer r.Body.Close()
	var finish casMultipartFinish
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&finish); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return casMultipartFinish{}, nil, "", false
	}
	if finish.Product == "" || finish.UploadID == "" {
		http.Error(w, "product and uploadId are required", http.StatusBadRequest)
		return casMultipartFinish{}, nil, "", false
	}
	if !s.requireAuthFor(w, r, finish.Product) {
		return casMultipartFinish{}, nil, "", false
	}
	if _, ok := s.cfg.Products[finish.Product]; !ok {
		http.Error(w, "unknown product", http.StatusNotFound)
		return casMultipartFinish{}, nil, "", false
	}
	key, err := model.CasKey(finish.SHA256)
	if err != nil {
		http.Error(w, "invalid blob sha256", http.StatusBadRequest)
		return casMultipartFinish{}, nil, "", false
	}
	backend, _, err := s.productIngest(finish.Product)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return casMultipartFinish{}, nil, "", false
	}
	return finish, backend, key, true
}
