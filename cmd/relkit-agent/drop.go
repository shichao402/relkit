package main

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

func dropDir(root, version string) string {
	return filepath.Join(root, ".relkit", "drop", version)
}

func (s *Server) handleDrop(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPut, http.MethodGet, http.MethodHead:
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.requireAuth(w, r) {
		return
	}
	product, versionRaw, filename, ok := parseDropRoute(r.URL.Path)
	if !ok {
		http.Error(w, "expected /v1/drop/{product}/{version}/{filename}", http.StatusBadRequest)
		return
	}
	version, ok := cleanVersion(versionRaw)
	if !ok {
		http.Error(w, "invalid version", http.StatusBadRequest)
		return
	}
	filename, ok = cleanDropName(filename)
	if !ok {
		http.Error(w, "invalid filename", http.StatusBadRequest)
		return
	}
	pc, ok := s.cfg.Products[product]
	if !ok {
		http.Error(w, "unknown product", http.StatusNotFound)
		return
	}
	dest := filepath.Join(dropDir(pc.Root, version), filename)
	switch r.Method {
	case http.MethodPut:
		s.putDropFile(w, r, product, version, dest)
	case http.MethodGet, http.MethodHead:
		http.ServeFile(w, r, dest)
	}
}

func (s *Server) putDropFile(w http.ResponseWriter, r *http.Request, product, version, dest string) {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		http.Error(w, "mkdir drop", http.StatusInternalServerError)
		return
	}
	body := http.MaxBytesReader(w, r.Body, s.cfg.MaxUpload)
	tmp, err := os.CreateTemp(filepath.Dir(dest), ".drop-*.tmp")
	if err != nil {
		http.Error(w, "temp file", http.StatusInternalServerError)
		return
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()

	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(tmp, h), body)
	closeErr := tmp.Close()
	if err != nil {
		http.Error(w, "read body failed", http.StatusBadRequest)
		return
	}
	if closeErr != nil {
		http.Error(w, "temp close failed", http.StatusInternalServerError)
		return
	}
	if err := os.Rename(tmpPath, dest); err != nil {
		http.Error(w, "commit drop", http.StatusInternalServerError)
		return
	}
	log.Printf("drop put %s/%s %s (%d bytes)", product, version, filepath.Base(dest), n)
	writeJSON(w, http.StatusCreated, map[string]any{
		"product": product,
		"version": version,
		"file":    filepath.Base(dest),
		"bytes":   n,
		"sha256":  hex.EncodeToString(h.Sum(nil)),
	})
}

// parseDropRoute accepts /v1/drop/{product}/{version}/{filename}.
func parseDropRoute(urlPath string) (product, version, filename string, ok bool) {
	cleaned := path.Clean("/" + strings.TrimSpace(urlPath))
	rest := strings.TrimPrefix(cleaned, "/v1/drop/")
	if rest == cleaned {
		return "", "", "", false
	}
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return "", "", "", false
	}
	return parts[0], parts[1], parts[2], true
}

func cleanDropName(name string) (string, bool) {
	name = path.Base(strings.TrimSpace(name))
	if name == "" || name == "." || name == ".." {
		return "", false
	}
	if strings.ContainsAny(name, `/\`) {
		return "", false
	}
	return name, true
}
