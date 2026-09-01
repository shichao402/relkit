package main

import (
	"crypto/subtle"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
)

// upload accepts PUT of a single file.
//
// Writes go to a temporary name and are then renamed into place. rename(2) is
// atomic within a filesystem, which matters more here than it looks: the index
// is the commit point of a release, so a client fetching it must see either the
// previous release or the new one, never a half-written file. A reader that
// already has the old file open keeps reading it unharmed, since the rename
// only replaces the directory entry.
func (c *config) upload(w http.ResponseWriter, r *http.Request) {
	if !c.uploadsEnabled() {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "uploads are disabled on this server", http.StatusMethodNotAllowed)
		return
	}
	cred := c.lookupCredential(r)
	if cred == nil {
		// No detail about which part was wrong, and no hint about whether the
		// path exists.
		w.Header().Set("WWW-Authenticate", `Bearer realm="relkit-serve"`)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if !c.requirePublishProtocol(w, r) {
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
	if !cred.allowsKey(name) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	if dir := path.Dir(name); dir != "." {
		if err := c.root.MkdirAll(dir, 0o755); err != nil {
			http.Error(w, "cannot create parent directory", http.StatusInternalServerError)
			return
		}
	}

	temp := name + ".tmp~"
	file, err := c.root.Create(temp)
	if err != nil {
		http.Error(w, "cannot create file", http.StatusInternalServerError)
		return
	}

	body := http.MaxBytesReader(w, r.Body, c.maxUpload)
	written, copyErr := io.Copy(file, body)
	syncErr := file.Sync()
	closeErr := file.Close()

	if copyErr != nil || syncErr != nil || closeErr != nil {
		c.root.Remove(temp)
		if copyErr != nil {
			var tooLarge *http.MaxBytesError
			if ok := asMaxBytes(copyErr, &tooLarge); ok {
				http.Error(w, fmt.Sprintf("upload exceeds the %s limit",
					humanBytes(c.maxUpload)), http.StatusRequestEntityTooLarge)
				return
			}
		}
		http.Error(w, "write failed", http.StatusInternalServerError)
		return
	}

	if err := c.root.Rename(temp, name); err != nil {
		c.root.Remove(temp)
		http.Error(w, "cannot commit file", http.StatusInternalServerError)
		return
	}

	// Index is the release commit point: artifacts and manifests are already
	// on disk, so a sweep now can drop anything the new indexes no longer name.
	if strings.HasPrefix(name, "index/") {
		c.scheduleGC()
	}

	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusCreated)
	fmt.Fprintf(w, "wrote %s (%d bytes)\n", name, written)
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
