package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"cnb.cool/shichao402/relkit/internal/backends"
	"cnb.cool/shichao402/relkit/internal/config"
	"cnb.cool/shichao402/relkit/internal/model"
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
	SHA256  string            `json:"sha256"`
	Size    int64             `json:"size"`
	PutURL  string            `json:"putUrl"`
	Headers map[string]string `json:"headers,omitempty"`
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
	if req.Product == "" || len(req.Blobs) == 0 {
		http.Error(w, "product and blobs required", http.StatusBadRequest)
		return
	}
	if !s.requireAuthFor(w, r, req.Product) {
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

		upload, err := s.authorizeCASUpload(r, req.Product, backend, key, blob.Size, expiresAt)
		if err != nil {
			http.Error(w, "authorize cas upload: "+err.Error(), http.StatusBadRequest)
			return
		}
		response.Uploads = append(response.Uploads, casUploadDocument{
			SHA256:  strings.ToLower(blob.SHA256),
			Size:    blob.Size,
			PutURL:  upload.PutURL,
			Headers: upload.Headers,
		})
		if upload.ExpiresAt.Before(response.ExpiresAt) {
			response.ExpiresAt = upload.ExpiresAt
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
	if len(profile.PublishTo) != 1 {
		return nil, nil, fmt.Errorf("CAS upload currently requires exactly one publishTo backend; Materialize is not implemented")
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

func (s *Server) authorizeCASUpload(r *http.Request, product string, backend backends.Backend, key string, size int64, expiresAt time.Time) (*backends.CASUpload, error) {
	if authorizer, ok := backend.(backends.CASUploadAuthorizer); ok {
		return authorizer.AuthorizeCASUpload(key, size, time.Until(expiresAt))
	}
	if _, ok := backend.(backends.CASProxyReceiver); !ok {
		return nil, fmt.Errorf("ingest backend %q cannot authorize direct or proxied CAS uploads", backend.Name())
	}
	token := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	return &backends.CASUpload{
		PutURL:    fmt.Sprintf("/v1/cas/%s/%s?size=%d", url.PathEscape(product), path.Base(key), size),
		Headers:   map[string]string{"Authorization": "Bearer " + token},
		ExpiresAt: expiresAt,
	}, nil
}

func (s *Server) handleCASPut(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	product, digest, ok := parseCASPutPath(r.URL.Path)
	if !ok {
		http.Error(w, "expected /v1/cas/{product}/{sha256}", http.StatusBadRequest)
		return
	}
	if !s.requireAuthFor(w, r, product) {
		return
	}
	if _, ok := s.cfg.Products[product]; !ok {
		http.Error(w, "unknown product", http.StatusNotFound)
		return
	}
	size, err := strconv.ParseInt(r.URL.Query().Get("size"), 10, 64)
	if err != nil || size < 0 || size > s.cfg.MaxUpload {
		http.Error(w, "valid size query required", http.StatusBadRequest)
		return
	}
	if r.ContentLength >= 0 && r.ContentLength != size {
		http.Error(w, "content length does not match size", http.StatusBadRequest)
		return
	}
	key, err := model.CasKey(digest)
	if err != nil {
		http.Error(w, "invalid sha256", http.StatusBadRequest)
		return
	}
	_, ingest, err := s.productIngest(product)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	receiver, ok := ingest.(backends.CASProxyReceiver)
	if !ok {
		http.Error(w, "ingest uses direct upload, not agent proxy", http.StatusBadRequest)
		return
	}
	if existing, found, err := ingest.Head(key); err == nil && found && existing == size {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err := receiver.ReceiveCAS(key, http.MaxBytesReader(w, r.Body, size+1), size); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func parseCASPutPath(urlPath string) (product, digest string, ok bool) {
	cleaned := path.Clean("/" + strings.TrimSpace(urlPath))
	rest := strings.TrimPrefix(cleaned, "/v1/cas/")
	if rest == cleaned {
		return "", "", false
	}
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) != 2 || model.CheckIdentifier(parts[0], "product") != nil {
		return "", "", false
	}
	if _, err := model.CasKey(parts[1]); err != nil {
		return "", "", false
	}
	return parts[0], strings.ToLower(parts[1]), true
}
