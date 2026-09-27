package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// console replaces relkit-serve's *config as the management-plane assembly.
// It keeps the same handler names and templates (ADR 0016: the split moves
// code, it does not redesign the panel), but every storage read goes through
// the Adapter instead of an os.Root of the served tree.
type console struct {
	adapter  Adapter
	admin    *adminAuth
	stats    *downloadStats
	site     *SiteConfig
	stateDir string
	makers   *makersPanel
}

func (c *console) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc(adminLoginPath, c.serveAdminLogin)
	mux.HandleFunc(adminSetupPath, c.serveAdminSetup)
	mux.HandleFunc(adminLogoutPath, c.serveAdminLogout)
	mux.HandleFunc(adminPath, c.requirePanelAuth(c.serveAdmin))
	mux.HandleFunc(adminFilesPath, c.requirePanelAuth(c.serveAdminFiles))
	mux.HandleFunc(productPathPrefix, c.requirePanelAuth(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		c.serveProduct(w, r)
	}))
	mux.HandleFunc(latestPathPrefix, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		c.serveLatest(w, r)
	})
	mux.HandleFunc("/-/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		fmt.Fprintln(w, "ok")
	})
	return mux
}

func trimSpace(s string) string { return strings.TrimSpace(s) }

// jsonDecodeStrict rejects unknown fields, same policy as relkit-store: a
// misspelled key silently keeping a default is the worst config bug.
func jsonDecodeStrict(raw []byte, v any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(v); err != nil {
		return err
	}
	return nil
}
