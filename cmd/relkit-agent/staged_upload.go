package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	uploadStatusOpen       = "open"
	uploadStatusCompleting = "completing"
	partSHAHeader          = "X-Relkit-Part-SHA256"
)

type createUploadRequest struct {
	Bytes    int64  `json:"bytes"`
	SHA256   string `json:"sha256"`
	PartSize any    `json:"partSize"`
}

type uploadRecord struct {
	ID        string            `json:"id"`
	Product   string            `json:"product"`
	Version   string            `json:"version"`
	Bytes     int64             `json:"bytes"`
	SHA256    string            `json:"sha256"`
	PartSize  int64             `json:"partSize"`
	PartCount int               `json:"partCount"`
	Status    string            `json:"status"`
	CreatedAt time.Time         `json:"createdAt"`
	Parts     map[string]string `json:"parts"`
}

type liveUpload struct {
	slots chan struct{}
	mu    sync.Mutex
}

func (s *Server) handleStagedUpload(w http.ResponseWriter, r *http.Request, product, version string, rest []string) {
	switch {
	case len(rest) == 1 && rest[0] == "uploads" && r.Method == http.MethodPost:
		s.createUpload(w, r, product, version)
	case len(rest) == 2 && rest[0] == "uploads":
		switch r.Method {
		case http.MethodGet:
			s.getUpload(w, r, product, version, rest[1])
		case http.MethodDelete:
			s.abortUpload(w, r, product, version, rest[1])
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	case len(rest) == 3 && rest[0] == "uploads" && rest[2] == "complete" && r.Method == http.MethodPost:
		s.completeUpload(w, r, product, version, rest[1])
	case len(rest) == 4 && rest[0] == "uploads" && rest[2] == "parts" && r.Method == http.MethodPut:
		s.putUploadPart(w, r, product, version, rest[1], rest[3])
	default:
		http.Error(w, "expected /v1/staged/{product}/{version}/uploads[/id[/parts/n|/complete]]", http.StatusBadRequest)
	}
}

func (s *Server) createUpload(w http.ResponseWriter, r *http.Request, product, version string) {
	defer r.Body.Close()
	var req createUploadRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	sum := strings.ToLower(strings.TrimSpace(req.SHA256))
	if req.Bytes <= 0 || req.Bytes > s.cfg.MaxUpload {
		http.Error(w, "bytes must be between 1 and maxUpload", http.StatusBadRequest)
		return
	}
	if !validSHA256Hex(sum) {
		http.Error(w, "sha256 required", http.StatusBadRequest)
		return
	}
	s.expireUploads()
	if rec := s.findOpenUpload(product, version, sum, req.Bytes); rec != nil {
		writeJSON(w, http.StatusOK, s.uploadView(rec))
		return
	}
	partSize, err := s.resolvePartSize(req.PartSize, req.Bytes)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	partCount := int((req.Bytes + partSize - 1) / partSize)
	id, err := newUploadID()
	if err != nil {
		http.Error(w, "mint upload id", http.StatusInternalServerError)
		return
	}
	rec := &uploadRecord{
		ID:        id,
		Product:   product,
		Version:   version,
		Bytes:     req.Bytes,
		SHA256:    sum,
		PartSize:  partSize,
		PartCount: partCount,
		Status:    uploadStatusOpen,
		CreatedAt: time.Now().UTC(),
		Parts:     map[string]string{},
	}
	if err := s.writeUploadRecord(rec); err != nil {
		http.Error(w, "persist upload: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, s.uploadView(rec))
}

func (s *Server) getUpload(w http.ResponseWriter, r *http.Request, product, version, id string) {
	rec, err := s.loadUpload(product, version, id)
	if err != nil {
		http.Error(w, err.Error(), statusForUploadErr(err))
		return
	}
	writeJSON(w, http.StatusOK, s.uploadView(rec))
}

func (s *Server) abortUpload(w http.ResponseWriter, r *http.Request, product, version, id string) {
	if _, err := s.loadUpload(product, version, id); err != nil {
		http.Error(w, err.Error(), statusForUploadErr(err))
		return
	}
	s.uploads.Delete(id)
	_ = os.RemoveAll(s.uploadDir(id))
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) putUploadPart(w http.ResponseWriter, r *http.Request, product, version, id, partRaw string) {
	part, err := strconv.Atoi(partRaw)
	if err != nil || part < 0 {
		http.Error(w, "invalid part number", http.StatusBadRequest)
		return
	}
	rec, err := s.loadUpload(product, version, id)
	if err != nil {
		http.Error(w, err.Error(), statusForUploadErr(err))
		return
	}
	if rec.Status != uploadStatusOpen {
		http.Error(w, "upload is not accepting parts", http.StatusConflict)
		return
	}
	if part >= rec.PartCount {
		http.Error(w, "part out of range", http.StatusBadRequest)
		return
	}
	wantLen := partLength(part, rec.PartCount, rec.PartSize, rec.Bytes)
	live := s.liveFor(id)
	select {
	case live.slots <- struct{}{}:
	default:
		w.Header().Set("Retry-After", "1")
		http.Error(w, "too many in-flight parts for this upload", http.StatusTooManyRequests)
		return
	}
	defer func() { <-live.slots }()

	body := http.MaxBytesReader(w, r.Body, wantLen+1)
	dir := filepath.Join(s.uploadDir(id), "parts")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		http.Error(w, "mkdir parts", http.StatusInternalServerError)
		return
	}
	tmp, err := os.CreateTemp(dir, fmt.Sprintf(".part-%d-*", part))
	if err != nil {
		http.Error(w, "temp file", http.StatusInternalServerError)
		return
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	h := sha256.New()
	n, copyErr := io.Copy(io.MultiWriter(tmp, h), body)
	closeErr := tmp.Close()
	if copyErr != nil {
		http.Error(w, "read body failed", http.StatusBadRequest)
		return
	}
	if closeErr != nil {
		http.Error(w, "temp close failed", http.StatusInternalServerError)
		return
	}
	if n != wantLen {
		http.Error(w, fmt.Sprintf("part length %d, want %d", n, wantLen), http.StatusBadRequest)
		return
	}
	sum := hex.EncodeToString(h.Sum(nil))
	if presented := strings.ToLower(strings.TrimSpace(r.Header.Get(partSHAHeader))); presented != "" {
		if !sha256HexEqual(presented, sum) {
			http.Error(w, "part sha256 mismatch", http.StatusBadRequest)
			return
		}
	}

	live.mu.Lock()
	defer live.mu.Unlock()
	fresh, err := s.loadUpload(product, version, id)
	if err != nil {
		http.Error(w, err.Error(), statusForUploadErr(err))
		return
	}
	if fresh.Status != uploadStatusOpen {
		http.Error(w, "upload is not accepting parts", http.StatusConflict)
		return
	}
	key := strconv.Itoa(part)
	if existing, ok := fresh.Parts[key]; ok {
		if !sha256HexEqual(existing, sum) {
			http.Error(w, "part already stored with a different sha256", http.StatusConflict)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"part": part, "sha256": sum, "bytes": n})
		return
	}
	dest := filepath.Join(dir, key)
	_ = os.Remove(dest)
	if err := os.Rename(tmpPath, dest); err != nil {
		http.Error(w, "store part: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if fresh.Parts == nil {
		fresh.Parts = map[string]string{}
	}
	fresh.Parts[key] = sum
	if err := s.writeUploadRecord(fresh); err != nil {
		_ = os.Remove(dest)
		http.Error(w, "persist upload: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"part": part, "sha256": sum, "bytes": n})
}

func (s *Server) completeUpload(w http.ResponseWriter, r *http.Request, product, version, id string) {
	live := s.liveFor(id)
	live.mu.Lock()
	defer live.mu.Unlock()
	rec, err := s.loadUpload(product, version, id)
	if err != nil {
		http.Error(w, err.Error(), statusForUploadErr(err))
		return
	}
	if rec.Status != uploadStatusOpen {
		http.Error(w, "upload is not open", http.StatusConflict)
		return
	}
	missing := missingParts(rec)
	if len(missing) > 0 {
		http.Error(w, fmt.Sprintf("missing parts: %v", missing), http.StatusConflict)
		return
	}
	rec.Status = uploadStatusCompleting
	if err := s.writeUploadRecord(rec); err != nil {
		http.Error(w, "persist upload: "+err.Error(), http.StatusInternalServerError)
		return
	}

	assembled, err := os.CreateTemp("", "relkit-staged-*.tar.gz")
	if err != nil {
		rec.Status = uploadStatusOpen
		_ = s.writeUploadRecord(rec)
		http.Error(w, "temp file", http.StatusInternalServerError)
		return
	}
	assembledPath := assembled.Name()
	defer os.Remove(assembledPath)

	h := sha256.New()
	out := io.MultiWriter(assembled, h)
	for i := 0; i < rec.PartCount; i++ {
		partPath := filepath.Join(s.uploadDir(id), "parts", strconv.Itoa(i))
		f, err := os.Open(partPath)
		if err != nil {
			_ = assembled.Close()
			rec.Status = uploadStatusOpen
			_ = s.writeUploadRecord(rec)
			http.Error(w, "open part: "+err.Error(), http.StatusInternalServerError)
			return
		}
		_, copyErr := io.Copy(out, f)
		_ = f.Close()
		if copyErr != nil {
			_ = assembled.Close()
			rec.Status = uploadStatusOpen
			_ = s.writeUploadRecord(rec)
			http.Error(w, "assemble: "+copyErr.Error(), http.StatusInternalServerError)
			return
		}
	}
	if err := assembled.Close(); err != nil {
		rec.Status = uploadStatusOpen
		_ = s.writeUploadRecord(rec)
		http.Error(w, "temp close failed", http.StatusInternalServerError)
		return
	}
	sum := hex.EncodeToString(h.Sum(nil))
	if !sha256HexEqual(sum, rec.SHA256) {
		rec.Status = uploadStatusOpen
		_ = s.writeUploadRecord(rec)
		http.Error(w, "assembled sha256 mismatch", http.StatusBadRequest)
		return
	}

	dest, err := s.installStagedArchive(product, version, assembledPath, sum)
	if err != nil {
		rec.Status = uploadStatusOpen
		_ = s.writeUploadRecord(rec)
		writeStagedErr(w, err)
		return
	}
	s.uploads.Delete(id)
	_ = os.RemoveAll(s.uploadDir(id))
	writeJSON(w, http.StatusCreated, map[string]any{
		"product": product,
		"version": version,
		"bytes":   rec.Bytes,
		"sha256":  sum,
		"path":    dest,
	})
}

func (s *Server) resolvePartSize(requested any, total int64) (int64, error) {
	size := s.cfg.PartSize
	if requested != nil {
		switch v := requested.(type) {
		case float64:
			size = int64(v)
		case string:
			if strings.TrimSpace(v) != "" {
				n, err := parseSize(v)
				if err != nil {
					return 0, fmt.Errorf("partSize: %w", err)
				}
				size = n
			}
		default:
			return 0, fmt.Errorf("partSize must be a number or size string")
		}
	}
	if size < s.cfg.MinPartSize {
		size = s.cfg.MinPartSize
	}
	if size > s.cfg.MaxPartSize {
		size = s.cfg.MaxPartSize
	}
	if total < size {
		size = total
	}
	partCount := (total + size - 1) / size
	if partCount > int64(s.cfg.MaxParts) {
		size = (total + int64(s.cfg.MaxParts) - 1) / int64(s.cfg.MaxParts)
		if size > s.cfg.MaxPartSize {
			return 0, fmt.Errorf("object needs more than %d parts at maxPartSize", s.cfg.MaxParts)
		}
		if size < 1 {
			size = 1
		}
	}
	return size, nil
}

func (s *Server) uploadView(rec *uploadRecord) map[string]any {
	received := make([]int, 0, len(rec.Parts))
	for key := range rec.Parts {
		n, err := strconv.Atoi(key)
		if err == nil {
			received = append(received, n)
		}
	}
	sort.Ints(received)
	return map[string]any{
		"id":             rec.ID,
		"product":        rec.Product,
		"version":        rec.Version,
		"bytes":          rec.Bytes,
		"sha256":         rec.SHA256,
		"partSize":       rec.PartSize,
		"partCount":      rec.PartCount,
		"maxConcurrency": s.cfg.MaxPartConcurrency,
		"status":         rec.Status,
		"received":       received,
	}
}

func (s *Server) uploadDir(id string) string {
	return filepath.Join(s.cfg.StateDir, "uploads", id)
}

func (s *Server) writeUploadRecord(rec *uploadRecord) error {
	dir := s.uploadDir(rec.ID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".session-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	dest := filepath.Join(dir, "session.json")
	_ = os.Remove(dest)
	return os.Rename(tmpPath, dest)
}

func (s *Server) loadUpload(product, version, id string) (*uploadRecord, error) {
	id, ok := cleanUploadID(id)
	if !ok {
		return nil, errBadUpload
	}
	data, err := os.ReadFile(filepath.Join(s.uploadDir(id), "session.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errUnknownUpload
		}
		return nil, err
	}
	var rec uploadRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		return nil, err
	}
	if rec.Product != product || rec.Version != version || rec.ID != id {
		return nil, errUnknownUpload
	}
	if time.Since(rec.CreatedAt) > s.cfg.UploadTTL {
		s.uploads.Delete(id)
		_ = os.RemoveAll(s.uploadDir(id))
		return nil, errExpiredUpload
	}
	return &rec, nil
}

func (s *Server) findOpenUpload(product, version, sum string, bytes int64) *uploadRecord {
	root := filepath.Join(s.cfg.StateDir, "uploads")
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		rec, err := s.loadUpload(product, version, entry.Name())
		if err != nil {
			continue
		}
		if rec.Status == uploadStatusOpen && rec.SHA256 == sum && rec.Bytes == bytes {
			return rec
		}
	}
	return nil
}

func (s *Server) expireUploads() {
	root := filepath.Join(s.cfg.StateDir, "uploads")
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		id := entry.Name()
		data, err := os.ReadFile(filepath.Join(s.uploadDir(id), "session.json"))
		if err != nil {
			continue
		}
		var rec uploadRecord
		if json.Unmarshal(data, &rec) != nil {
			continue
		}
		if time.Since(rec.CreatedAt) > s.cfg.UploadTTL {
			s.uploads.Delete(id)
			_ = os.RemoveAll(s.uploadDir(id))
		}
	}
}

func (s *Server) liveFor(id string) *liveUpload {
	if v, ok := s.uploads.Load(id); ok {
		return v.(*liveUpload)
	}
	created := &liveUpload{slots: make(chan struct{}, s.cfg.MaxPartConcurrency)}
	actual, _ := s.uploads.LoadOrStore(id, created)
	return actual.(*liveUpload)
}

func partLength(part, partCount int, partSize, total int64) int64 {
	if part < 0 || part >= partCount {
		return 0
	}
	if part == partCount-1 {
		return total - partSize*int64(part)
	}
	return partSize
}

func missingParts(rec *uploadRecord) []int {
	var missing []int
	for i := 0; i < rec.PartCount; i++ {
		if _, ok := rec.Parts[strconv.Itoa(i)]; !ok {
			missing = append(missing, i)
		}
	}
	return missing
}

func newUploadID() (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf[:]), nil
}

func cleanUploadID(id string) (string, bool) {
	id = strings.ToLower(strings.TrimSpace(id))
	if len(id) != 32 {
		return "", false
	}
	for i := 0; i < len(id); i++ {
		c := id[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return "", false
		}
	}
	return id, true
}

type uploadError string

func (e uploadError) Error() string { return string(e) }

const (
	errUnknownUpload uploadError = "unknown upload"
	errExpiredUpload uploadError = "upload expired"
	errBadUpload     uploadError = "invalid upload id"
)

func statusForUploadErr(err error) int {
	switch err {
	case errUnknownUpload:
		return http.StatusNotFound
	case errExpiredUpload:
		return http.StatusGone
	case errBadUpload:
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}
