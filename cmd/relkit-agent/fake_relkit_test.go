package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path"
	"strconv"
	"strings"
	"sync"
	"testing"

	"firoyang.com/relkit/internal/publishproto"
)

type fakeRelkitServe struct {
	token   string
	mu      sync.Mutex
	objects map[string][]byte
	URL     string
}

func startFakeRelkitServe(t *testing.T, token string) *fakeRelkitServe {
	t.Helper()
	f := &fakeRelkitServe{token: token, objects: map[string][]byte{}}
	ts := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(ts.Close)
	f.URL = ts.URL
	t.Setenv("RELKIT_SERVE_TOKEN", token)
	return f
}

func (f *fakeRelkitServe) backendConfig() map[string]any {
	return map[string]any{
		"type":     "relkit-compatible",
		"baseUrl":  strings.TrimRight(f.URL, "/") + "/",
		"tokenEnv": "RELKIT_SERVE_TOKEN",
	}
}

func (f *fakeRelkitServe) get(key string) []byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.objects[key]
}

func (f *fakeRelkitServe) serve(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/-/cas/uploads":
		if !f.bearerOK(r) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		var req struct {
			Key  string `json:"key"`
			Size int64  `json:"size"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		putURL := strings.TrimRight(f.URL, "/") + "/" + req.Key + "?sig=ok&size=" + strconv.FormatInt(req.Size, 10)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"url": putURL})
	case r.Method == http.MethodPost && r.URL.Path == publishproto.PreflightPath:
		if !f.bearerOK(r) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"ok":true,"protocol":2,"minProtocol":0}`)
	case r.Method == http.MethodHead:
		key := strings.TrimPrefix(r.URL.Path, "/")
		f.mu.Lock()
		data, ok := f.objects[key]
		f.mu.Unlock()
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	case r.Method == http.MethodGet:
		key := strings.TrimPrefix(r.URL.Path, "/")
		f.mu.Lock()
		data := f.objects[key]
		f.mu.Unlock()
		if data == nil {
			http.NotFound(w, r)
			return
		}
		w.Write(data)
	case r.Method == http.MethodDelete:
		if !f.bearerOK(r) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		key := strings.TrimPrefix(r.URL.Path, "/")
		f.mu.Lock()
		delete(f.objects, key)
		f.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodPut:
		key := strings.TrimPrefix(r.URL.Path, "/")
		if src := r.Header.Get("X-Relkit-Copy-Source"); src != "" {
			if !f.bearerOK(r) {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			f.mu.Lock()
			data := append([]byte(nil), f.objects[src]...)
			f.objects[key] = data
			f.mu.Unlock()
			w.WriteHeader(http.StatusCreated)
			return
		}
		body, _ := io.ReadAll(r.Body)
		if r.URL.Query().Get("sig") == "ok" {
			if want, err := strconv.ParseInt(r.URL.Query().Get("size"), 10, 64); err == nil && int64(len(body)) != want {
				http.Error(w, "size", http.StatusBadRequest)
				return
			}
			sum := sha256.Sum256(body)
			if path.Base(key) != hex.EncodeToString(sum[:]) {
				http.Error(w, "sha256", http.StatusBadRequest)
				return
			}
			f.mu.Lock()
			f.objects[key] = body
			f.mu.Unlock()
			w.WriteHeader(http.StatusCreated)
			return
		}
		if f.bearerOK(r) {
			f.mu.Lock()
			f.objects[key] = body
			f.mu.Unlock()
			w.WriteHeader(http.StatusCreated)
			return
		}
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (f *fakeRelkitServe) bearerOK(r *http.Request) bool {
	return r.Header.Get("Authorization") == "Bearer "+f.token
}
