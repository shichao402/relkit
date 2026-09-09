package main

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"time"

	"firoyang.com/relkit/internal/model"
)

const (
	casKeyFileName      = ".relkit-serve-cas.key"
	casUploadsPath      = "/-/cas/uploads"
	copySourceHeader    = "X-Relkit-Copy-Source"
	defaultCASUploadTTL = time.Hour
	defaultCASGrace     = 24 * time.Hour
)

func loadOrCreateCASSecret(dir string) ([]byte, error) {
	path := filepath.Join(dir, casKeyFileName)
	data, err := os.ReadFile(path)
	if err == nil && len(data) >= 32 {
		return data[:32], nil
	}
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, secret, 0o600); err != nil {
		return nil, err
	}
	return secret, nil
}

func (c *config) serveCASMint(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !c.uploadsEnabled() {
		http.Error(w, "uploads are disabled on this server", http.StatusMethodNotAllowed)
		return
	}
	cred := c.lookupCredential(r)
	if cred == nil {
		w.Header().Set("WWW-Authenticate", `Bearer realm="relkit-serve"`)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if cred.products != nil {
		http.Error(w, "operator token required", http.StatusForbidden)
		return
	}
	if !c.requirePublishProtocol(w, r) {
		return
	}
	defer r.Body.Close()
	var req struct {
		Key  string `json:"key"`
		Size int64  `json:"size"`
		TTL  int    `json:"ttl"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	canonical, err := model.CasKey(path.Base(req.Key))
	if err != nil || req.Key != canonical {
		http.Error(w, "invalid cas key", http.StatusBadRequest)
		return
	}
	if req.Size < 0 || req.Size > c.maxUpload {
		http.Error(w, "invalid size", http.StatusBadRequest)
		return
	}
	ttl := time.Duration(req.TTL) * time.Second
	if ttl < time.Second {
		ttl = defaultCASUploadTTL
	}
	expires := time.Now().UTC().Add(ttl)
	putURL, err := c.signCASPutURL(r, req.Key, req.Size, expires)
	if err != nil {
		http.Error(w, "cannot mint upload url", http.StatusInternalServerError)
		return
	}
	if c.gc != nil {
		c.gc.retainCAS(req.Key, expires)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"url":       putURL,
		"expiresAt": expires,
	})
}

func (c *config) signCASPutURL(r *http.Request, key string, size int64, expires time.Time) (string, error) {
	if len(c.casSecret) == 0 {
		return "", fmt.Errorf("cas secret missing")
	}
	exp := expires.Unix()
	sig := c.casMAC(http.MethodPut, key, size, exp)
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if proto := r.Header.Get("X-Forwarded-Proto"); proto == "http" || proto == "https" {
		scheme = proto
	}
	host := r.Host
	if host == "" {
		host = r.URL.Host
	}
	return fmt.Sprintf("%s://%s/%s?exp=%d&size=%d&sig=%s", scheme, host, key, exp, size, sig), nil
}

func (c *config) casMAC(method, key string, size, exp int64) string {
	mac := hmac.New(sha256.New, c.casSecret)
	fmt.Fprintf(mac, "%s\n%s\n%d\n%d", method, key, size, exp)
	return hex.EncodeToString(mac.Sum(nil))
}

func (c *config) verifyCASCapability(r *http.Request, key string) (int64, bool) {
	if len(c.casSecret) == 0 {
		return 0, false
	}
	query := r.URL.Query()
	exp, err := strconv.ParseInt(query.Get("exp"), 10, 64)
	if err != nil || time.Now().Unix() > exp {
		return 0, false
	}
	size, err := strconv.ParseInt(query.Get("size"), 10, 64)
	if err != nil || size < 0 {
		return 0, false
	}
	want := c.casMAC(r.Method, key, size, exp)
	got := query.Get("sig")
	if subtle.ConstantTimeCompare([]byte(want), []byte(got)) != 1 {
		return 0, false
	}
	return size, true
}

func (c *config) copyObject(dst, src string) error {
	srcFile, err := c.root.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()
	if dir := path.Dir(dst); dir != "." {
		if err := c.root.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	tmp := dst + ".tmp~"
	out, err := c.root.Create(tmp)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, srcFile)
	syncErr := out.Sync()
	closeErr := out.Close()
	if copyErr != nil || syncErr != nil || closeErr != nil {
		c.root.Remove(tmp)
		if copyErr != nil {
			return copyErr
		}
		if syncErr != nil {
			return syncErr
		}
		return closeErr
	}
	if err := c.root.Rename(tmp, dst); err != nil {
		c.root.Remove(tmp)
		return err
	}
	return nil
}
