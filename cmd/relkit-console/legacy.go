package main

// Legacy operator URLs.
//
// Before the rebuild this console answered /-/admin, /-/admin/panel/*,
// /-/admin/login, /-/admin/setup and /-/p/* with server-rendered HTML. Those
// URLs are bookmarks, they appear in release notes and in shell history, so
// they stay valid entry points into the rebuilt front end (requirement 7.3):
// each one answers a 302 to the page that replaced it.
//
// A redirect is deliberately anonymous-safe. The app shell is public and gates
// itself through /-/admin/api/session, so there is nothing here to protect: a
// session check would only add an agent round trip per old link, and it could
// not tell a signed-out operator from a signed-in one without reading the
// cookie anyway. That check lives in exactly one place, the API.
//
// /-/latest/* and /-/health are not legacy pages: they are machine-facing
// endpoints that keep their paths and behaviour, and they are served by their
// own handlers.

import (
	"net/http"
	"net/url"
	"strings"
)

const (
	// The old panel's tab strip. These names survive only as redirect sources;
	// nothing renders them any more.
	panelPath       = "/-/admin/panel"
	panelUsersPath  = "/-/admin/panel/users"
	panelRolesPath  = "/-/admin/panel/roles"
	panelHostsPath  = "/-/admin/panel/products"
	panelTokensPath = "/-/admin/panel/tokens"
)

// redirectLegacy answers one old page URL with a 302 to its replacement.
// Redirects are never cached: the target is computed from the front end's route
// table, and a cached 302 would outlive a change to it.
func redirectLegacy(w http.ResponseWriter, r *http.Request, target string) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, target, http.StatusFound)
}

// serveLegacyAdmin answers /-/admin, the old portal root. The front end also
// redirects "/" to /products, but doing it here saves the operator a second
// navigation and keeps the first paint off a page that is about to be replaced.
func serveLegacyAdmin(w http.ResponseWriter, r *http.Request) {
	redirectLegacy(w, r, appPrefix+"products")
}

// serveLegacyPanel maps the old tab strip onto the new routes. The token tab
// used to be filtered by ?product=<id>; that filter is now the product's own
// tab, so the query survives the move instead of being dropped.
func serveLegacyPanel(w http.ResponseWriter, r *http.Request) {
	target := appPrefix + "products"
	switch r.URL.Path {
	case panelUsersPath:
		target = appPrefix + "users"
	case panelRolesPath:
		target = appPrefix + "roles"
	case panelTokensPath:
		target = appPrefix + "tokens"
		if product := strings.TrimSpace(r.URL.Query().Get("product")); product != "" && safeSegment(product) {
			target = appPrefix + "products/" + url.PathEscape(product) + "/tokens"
		}
	}
	redirectLegacy(w, r, target)
}

// serveLegacyProduct answers /-/p/<product>. The product is validated exactly as
// the old page validated it, because a redirect to a page that will 404 is worse
// than saying "not found" here.
func serveLegacyProduct(w http.ResponseWriter, r *http.Request) {
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, productPathPrefix), "/")
	if rest == "" || strings.Contains(rest, "/") {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	product, err := url.PathUnescape(rest)
	if err != nil || !safeSegment(product) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	redirectLegacy(w, r, appPrefix+"products/"+url.PathEscape(product))
}

// serveLegacyLogin answers the old form URL. Signing in is a JSON API call now,
// so the form endpoint is a signpost rather than a page.
func serveLegacyLogin(w http.ResponseWriter, r *http.Request) {
	redirectLegacy(w, r, appPrefix+"sign-in")
}

// serveLegacySetup answers the old first-run form URL. Which screen the operator
// actually sees is the agent's answer (bootstrap live, operator exists, or
// neither), and the front end asks that question on boot; so this redirects to
// the app root and lets the gate decide.
func serveLegacySetup(w http.ResponseWriter, r *http.Request) {
	redirectLegacy(w, r, appPrefix)
}

// serveLegacyLogout keeps the old sign-out link working. It is the one legacy
// URL with a side effect: the cookie is cleared here, so a bookmark still signs
// the operator out, and the agent session is dropped best-effort exactly as the
// API's logout does.
func (c *console) serveLegacyLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodPost {
		w.Header().Set("Allow", "GET, HEAD, POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if session, ok := c.admin.session(r); ok {
		if agent := c.admin.agentClient(); agent != nil {
			_ = agent.Logout(session.Agent)
		}
	}
	http.SetCookie(w, expireCookie())
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, appPrefix+"sign-in", http.StatusFound)
}
