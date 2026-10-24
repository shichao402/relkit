package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// console replaces relkit-serve's *config as the management-plane assembly
// (ADR 0016). It owns the JSON API the single-page front end talks to, the app
// shell, the fixed download URLs and the legacy redirects. Every storage read
// goes through the Adapter instead of an os.Root of the served tree, and every
// write is forwarded to relkit-agent with the operator's own bearer.
type console struct {
	adapter Adapter
	admin   *panelSession
	stats   *downloadStats
	// site is the server-side product metadata fallback: a product that has not
	// published its own site document still gets its configured title.
	site *SiteConfig
}

func (c *console) handler() http.Handler {
	mux := http.NewServeMux()
	c.registerAPI(mux)
	// The rebuilt front end is a single-page app under /-/admin/. Its assets and
	// every page route below /-/admin/ are served here; the specific patterns
	// registered afterwards win over this catch-all.
	mux.HandleFunc(assetsPrefix, c.serveApp())
	mux.HandleFunc(appPrefix, func(w http.ResponseWriter, r *http.Request) {
		if !isAppPath(r.URL.Path) {
			http.NotFound(w, r)
			return
		}
		c.serveApp()(w, r)
	})
	// Old page URLs stay valid entry points (requirement 7.3): every one of them
	// answers a 302 to the page that replaced it, so a bookmark or a link in a
	// release note still lands on a working screen.
	mux.HandleFunc(adminPath, serveLegacyAdmin)
	mux.HandleFunc(panelPath, serveLegacyPanel)
	mux.HandleFunc(panelUsersPath, serveLegacyPanel)
	mux.HandleFunc(panelRolesPath, serveLegacyPanel)
	mux.HandleFunc(panelHostsPath, serveLegacyPanel)
	mux.HandleFunc(panelTokensPath, serveLegacyPanel)
	mux.HandleFunc(adminLoginPath, serveLegacyLogin)
	mux.HandleFunc(adminSetupPath, serveLegacySetup)
	mux.HandleFunc(adminLogoutPath, c.serveLegacyLogout)
	mux.HandleFunc(productPathPrefix, serveLegacyProduct)
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
