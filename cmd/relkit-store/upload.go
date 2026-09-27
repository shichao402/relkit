package main

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"strings"
)

func (c *config) upload(w http.ResponseWriter, r *http.Request) {
	if !c.uploadsEnabled() {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "uploads are disabled on this server", http.StatusMethodNotAllowed)
		return
	}

	name, ok := cleanKey(r.URL.Path)
	if !ok {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	if name == "." || strings.HasSuffix(name, "/") {
		http.Error(w, "cannot write to a directory path", http.StatusBadRequest)
		return
	}
	if c.hiddenKey(name) {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}

	if source := strings.TrimSpace(r.Header.Get(copySourceHeader)); source != "" {
		c.uploadCopy(w, r, name, source)
		return
	}
	if strings.HasPrefix(name, "cas/") && r.URL.Query().Get("sig") != "" {
		c.uploadCASCapability(w, r, name)
		return
	}

	cred := c.lookupCredential(r)
	if cred == nil {
		w.Header().Set("WWW-Authenticate", `Bearer realm="relkit-serve"`)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if !c.requirePublishProtocol(w, r) {
		return
	}
	if !cred.allowsKey(name) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	written, err := c.commitUpload(w, r, name, c.maxUpload, nil)
	if err != nil {
		return
	}
	if strings.HasPrefix(name, "index/") {
		c.scheduleGC()
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusCreated)
	fmt.Fprintf(w, "wrote %s (%d bytes)\n", name, written)
}

func (c *config) uploadCopy(w http.ResponseWriter, r *http.Request, dst, src string) {
	cred := c.lookupCredential(r)
	if cred == nil {
		w.Header().Set("WWW-Authenticate", `Bearer realm="relkit-serve"`)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if !c.requirePublishProtocol(w, r) {
		return
	}
	srcKey, ok := cleanKey("/" + src)
	if !ok || srcKey == "." || c.hiddenKey(srcKey) {
		http.Error(w, "invalid copy source", http.StatusBadRequest)
		return
	}
	if !strings.HasPrefix(srcKey, "cas/") || !strings.HasPrefix(dst, "artifact/") {
		http.Error(w, "copy only supports cas/ to artifact/", http.StatusBadRequest)
		return
	}
	if !cred.allowsKey(dst) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if _, err := c.root.Stat(srcKey); err != nil {
		http.Error(w, "copy source not found", http.StatusNotFound)
		return
	}
	if err := c.copyObject(dst, srcKey); err != nil {
		http.Error(w, "copy failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusCreated)
	fmt.Fprintf(w, "copied %s -> %s\n", srcKey, dst)
}

func (c *config) uploadCASCapability(w http.ResponseWriter, r *http.Request, name string) {
	declared, ok := c.verifyCASCapability(r, name)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if r.ContentLength >= 0 && r.ContentLength != declared {
		http.Error(w, "content length does not match ticket", http.StatusBadRequest)
		return
	}
	hash := sha256.New()
	written, err := c.commitUpload(w, r, name, declared, hash)
	if err != nil {
		return
	}
	if written != declared {
		c.root.Remove(name)
		http.Error(w, "cas body size does not match ticket", http.StatusBadRequest)
		return
	}
	if got := hex.EncodeToString(hash.Sum(nil)); path.Base(name) != got {
		c.root.Remove(name)
		http.Error(w, "cas body sha256 does not match key", http.StatusBadRequest)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusCreated)
	fmt.Fprintf(w, "wrote %s (%d bytes)\n", name, written)
}

func (c *config) commitUpload(w http.ResponseWriter, r *http.Request, name string, limit int64, hash io.Writer) (int64, error) {
	if dir := path.Dir(name); dir != "." {
		if err := c.root.MkdirAll(dir, 0o755); err != nil {
			http.Error(w, "cannot create parent directory", http.StatusInternalServerError)
			return 0, err
		}
	}
	temp := name + ".tmp~"
	file, err := c.root.Create(temp)
	if err != nil {
		http.Error(w, "cannot create file", http.StatusInternalServerError)
		return 0, err
	}
	var dest io.Writer = file
	if hash != nil {
		dest = io.MultiWriter(file, hash)
	}
	body := http.MaxBytesReader(w, r.Body, limit)
	written, copyErr := io.Copy(dest, body)
	syncErr := file.Sync()
	closeErr := file.Close()
	if copyErr != nil || syncErr != nil || closeErr != nil {
		c.root.Remove(temp)
		if copyErr != nil {
			var tooLarge *http.MaxBytesError
			if ok := asMaxBytes(copyErr, &tooLarge); ok {
				http.Error(w, fmt.Sprintf("upload exceeds the %s limit",
					humanBytes(c.maxUpload)), http.StatusRequestEntityTooLarge)
				return 0, copyErr
			}
		}
		http.Error(w, "write failed", http.StatusInternalServerError)
		if copyErr != nil {
			return 0, copyErr
		}
		if syncErr != nil {
			return 0, syncErr
		}
		return 0, closeErr
	}
	if err := c.root.Rename(temp, name); err != nil {
		c.root.Remove(temp)
		http.Error(w, "cannot commit file", http.StatusInternalServerError)
		return 0, err
	}
	return written, nil
}

func (c *config) deleteObject(w http.ResponseWriter, r *http.Request) {
	if !c.uploadsEnabled() {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "uploads are disabled on this server", http.StatusMethodNotAllowed)
		return
	}
	cred := c.lookupCredential(r)
	if cred == nil {
		w.Header().Set("WWW-Authenticate", `Bearer realm="relkit-serve"`)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if !c.requirePublishProtocol(w, r) {
		return
	}
	name, ok := cleanKey(r.URL.Path)
	if !ok || name == "." || strings.HasSuffix(name, "/") || c.hiddenKey(name) {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	if !cred.allowsKey(name) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if err := c.root.Remove(name); err != nil {
		if os.IsNotExist(err) {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.Error(w, "delete failed", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (c *config) uploadsEnabled() bool {
	return len(c.credentials) > 0
}

func (c *config) lookupCredential(r *http.Request) *credential {
	header := r.Header.Get("Authorization")
	value, found := strings.CutPrefix(header, "Bearer ")
	if !found {
		return nil
	}
	presented := hashToken(strings.TrimSpace(value))
	var matched *credential
	for i := range c.credentials {
		if subtle.ConstantTimeCompare(presented, c.credentials[i].hash) == 1 {
			matched = &c.credentials[i]
		}
	}
	return matched
}

func (c *config) authorized(r *http.Request) bool {
	return c.lookupCredential(r) != nil
}

func asMaxBytes(err error, target **http.MaxBytesError) bool {
	for err != nil {
		if e, ok := err.(*http.MaxBytesError); ok {
			*target = e
			return true
		}
		unwrapped, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = unwrapped.Unwrap()
	}
	return false
}
