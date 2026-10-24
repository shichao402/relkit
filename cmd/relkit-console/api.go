package main

// JSON API for the panel front end.
//
// The old panel rendered HTML on the server and answered every POST with a full
// page, which is how a browser refresh could re-send "rotate token" (acceptance
// finding #3). The rebuilt front end talks to this API instead: reads are GET,
// writes are POST/PUT/DELETE fired from a button, and nothing a browser replays
// on refresh or back can reach a write handler.
//
// Authority does not move. Every write still travels to the agent's
// /v1/admin/* with the operator's own session bearer (requirement 13.6); this
// file only adds a JSON shape, a CSRF check and a uniform error envelope.

import (
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
)

const (
	apiPrefix      = "/-/admin/api/"
	apiCSRFHeader  = "X-Relkit-CSRF"
	apiMaxBodySize = 1 << 20
)

// apiError is the one error envelope every API answer uses. Field names the
// form input the message belongs to, so the front end can show it in place.
type apiError struct {
	OK      bool   `json:"ok"`
	Code    string `json:"code"`
	Message string `json:"message"`
	Field   string `json:"field,omitempty"`
}

func writeAPIJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	// Responses can carry a one-time plaintext; nothing here may be cached.
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeAPIError(w http.ResponseWriter, status int, code, message, field string) {
	writeAPIJSON(w, status, apiError{OK: false, Code: code, Message: message, Field: field})
}

// writeAgentFailure turns an agent call failure into the envelope. A structured
// agent refusal ({code,message,field}) passes through untouched, so the field
// hint the agent computed (for example product_root_unwritable on "root")
// reaches the input it belongs to. A plain-text refusal becomes the message
// verbatim; an unreachable agent is 502.
func writeAgentFailure(w http.ResponseWriter, err error) {
	var refusal *agentError
	if !errors.As(err, &refusal) {
		writeAPIError(w, http.StatusBadGateway, "agent_unreachable", err.Error(), "")
		return
	}
	status := refusal.StatusCode
	if status < 400 || status > 599 {
		status = http.StatusBadGateway
	}
	var structured struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Field   string `json:"field"`
	}
	if json.Unmarshal([]byte(refusal.Message), &structured) == nil && structured.Message != "" {
		code := structured.Code
		if code == "" {
			code = agentStatusCode(status)
		}
		writeAPIError(w, status, code, structured.Message, structured.Field)
		return
	}
	writeAPIError(w, status, agentStatusCode(status), refusal.Message, "")
}

func agentStatusCode(status int) string {
	switch status {
	case http.StatusUnauthorized:
		return "unauthenticated"
	case http.StatusForbidden:
		return "forbidden"
	case http.StatusNotFound:
		return "not_found"
	case http.StatusConflict:
		return "conflict"
	case http.StatusServiceUnavailable:
		return "agent_unavailable"
	}
	if status >= 500 {
		return "agent_error"
	}
	return "rejected"
}

// csrfToken binds a token to the agent bearer inside the signed cookie. The
// browser cannot forge it without the panel's key, and a cross-site page cannot
// read it, so a write that carries it was issued by our own front end.
func (s *panelSession) csrfToken(session signedSession) string {
	if s == nil {
		return ""
	}
	s.mu.Lock()
	key := append([]byte(nil), s.key...)
	s.mu.Unlock()
	if len(key) < 32 {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(hmacSHA256(key, []byte("csrf|"+session.Agent)))
}

// apiWriteAllowed is the guard every state-changing API request passes. JSON
// content type forces a CORS preflight on any cross-origin attempt; the origin
// check refuses a same-site form that slips past it. Unlike the old form
// handlers a request with neither Origin nor Referer is refused: every browser
// that runs our front end sends Origin on a fetch POST.
func apiWriteAllowed(r *http.Request) (string, bool) {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if r.ContentLength != 0 && (err != nil || mediaType != "application/json") {
		return "request body must be application/json", false
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		origin = r.Header.Get("Referer")
	}
	if origin == "" {
		return "missing Origin header", false
	}
	u, err := url.Parse(origin)
	if err != nil || !strings.EqualFold(u.Host, r.Host) {
		return "cross-origin request refused", false
	}
	return "", true
}

func isReadMethod(method string) bool {
	return method == http.MethodGet || method == http.MethodHead
}

// decodeAPIBody reads a JSON body strictly: unknown fields are a client bug,
// and a misspelled key silently defaulting is exactly the failure mode the
// strict config decoder already refuses.
func decodeAPIBody(w http.ResponseWriter, r *http.Request, out any) bool {
	raw, err := io.ReadAll(io.LimitReader(r.Body, apiMaxBodySize+1))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "bad_request", "could not read the request body", "")
		return false
	}
	if len(raw) > apiMaxBodySize {
		writeAPIError(w, http.StatusRequestEntityTooLarge, "too_large", "request body is too large", "")
		return false
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		raw = []byte("{}")
	}
	if err := jsonDecodeStrict(raw, out); err != nil {
		writeAPIError(w, http.StatusBadRequest, "bad_request", "invalid JSON: "+err.Error(), "")
		return false
	}
	return true
}

// apiAuthed is an API handler that already knows who is calling.
type apiAuthed func(w http.ResponseWriter, r *http.Request, session signedSession)

// requireAPIAuth is requirePanelAuth for JSON: no redirects, a 401 envelope the
// front end turns into "go to sign-in and come back". Writes additionally need
// the origin check and the CSRF token. An agent that has since dropped the
// session answers 401 through writeAgentFailure, and the front end handles it
// the same way.
func (c *console) requireAPIAuth(next apiAuthed) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		session, ok := c.admin.session(r)
		if !ok {
			writeAPIError(w, http.StatusUnauthorized, "unauthenticated", "sign in to continue", "")
			return
		}
		if !isReadMethod(r.Method) {
			if reason, ok := apiWriteAllowed(r); !ok {
				writeAPIError(w, http.StatusForbidden, "forbidden", reason, "")
				return
			}
			want := c.admin.csrfToken(session)
			got := r.Header.Get(apiCSRFHeader)
			if want == "" || subtle.ConstantTimeCompare([]byte(got), []byte(want)) != 1 {
				writeAPIError(w, http.StatusForbidden, "csrf", "missing or stale CSRF token; reload the page", "")
				return
			}
		}
		if c.admin.agentClient() == nil {
			writeAPIError(w, http.StatusServiceUnavailable, "agent_unavailable", "the panel has no agent configured", "")
			return
		}
		next(w, withPanel(r, session), session)
	}
}

// apiMethods answers 405 with an Allow header for anything not listed.
func apiMethods(w http.ResponseWriter, r *http.Request, allowed ...string) bool {
	for _, method := range allowed {
		if r.Method == method || (method == http.MethodGet && r.Method == http.MethodHead) {
			return true
		}
	}
	w.Header().Set("Allow", strings.Join(allowed, ", "))
	writeAPIError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", "")
	return false
}

func gateName(gate panelGate) string {
	switch gate {
	case gateSetup:
		return "setup"
	case gateLogin:
		return "login"
	case gateUnavailable:
		return "unavailable"
	default:
		return "locked"
	}
}

// apiSessionView is what the front end needs on every boot: which screen to
// show, who is signed in, the CSRF token for writes, and the version for the
// footer.
type apiSessionView struct {
	OK       bool       `json:"ok"`
	Gate     string     `json:"gate"`
	User     *agentUser `json:"user,omitempty"`
	CSRF     string     `json:"csrf,omitempty"`
	Version  string     `json:"version"`
	Agent    string     `json:"agent"`
	Settings *apiPanelSettings `json:"settings,omitempty"`
}

// apiPanelSettings carries the server-side facts forms depend on (requirement
// 4.3): nothing about a path is hardcoded in the front end.
type apiPanelSettings struct {
	ProductRootParent       string `json:"productRootParent"`
	ProductRootParentSource string `json:"productRootParentSource"`
}

// serveAPISession: GET reports the gate (and the signed-in user when the cookie
// is good and the agent still honours it). A cookie the agent no longer knows is
// cleared here, so the front end never shows a ghost session.
func (c *console) serveAPISession(w http.ResponseWriter, r *http.Request) {
	if !apiMethods(w, r, http.MethodGet) {
		return
	}
	view := apiSessionView{OK: true, Version: version, Agent: c.admin.agentBase()}
	agent := c.admin.agentClient()
	if session, ok := c.admin.session(r); ok && agent != nil {
		user, err := agent.Whoami(session.Agent)
		if err == nil {
			view.Gate = "ready"
			view.User = &user
			view.CSRF = c.admin.csrfToken(session)
			if settings, err := agent.Settings(session.Agent); err == nil {
				view.Settings = &settings
			}
			writeAPIJSON(w, http.StatusOK, view)
			return
		}
		var refusal *agentError
		if !errors.As(err, &refusal) {
			view.Gate = "unavailable"
			writeAPIJSON(w, http.StatusOK, view)
			return
		}
		if refusal.Unauthorized() {
			http.SetCookie(w, expireCookie())
		}
	}
	view.Gate = gateName(c.admin.gate())
	writeAPIJSON(w, http.StatusOK, view)
}

// serveAPILogin and serveAPISetup are the only writes without a session, so
// they carry the origin check but no CSRF token (there is no session to bind it
// to yet). Every refusal is delayed to blunt guessing, as the form flow did.
func (c *console) serveAPILogin(w http.ResponseWriter, r *http.Request) {
	if !apiMethods(w, r, http.MethodPost) {
		return
	}
	if reason, ok := apiWriteAllowed(r); !ok {
		writeAPIError(w, http.StatusForbidden, "forbidden", reason, "")
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !decodeAPIBody(w, r, &req) {
		return
	}
	agent := c.admin.agentClient()
	if agent == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "agent_unavailable", "the panel has no agent configured", "")
		return
	}
	login, err := agent.Login(req.Username, req.Password)
	if err != nil {
		sleepAuthFail()
		var refusal *agentError
		if errors.As(err, &refusal) && refusal.Unauthorized() {
			writeAPIError(w, http.StatusUnauthorized, "invalid_credentials", "用户名或密码错误", "")
			return
		}
		writeAgentFailure(w, err)
		return
	}
	c.answerSignedIn(w, login)
}

func (c *console) serveAPISetup(w http.ResponseWriter, r *http.Request) {
	if !apiMethods(w, r, http.MethodPost) {
		return
	}
	if reason, ok := apiWriteAllowed(r); !ok {
		writeAPIError(w, http.StatusForbidden, "forbidden", reason, "")
		return
	}
	var req struct {
		Bootstrap string `json:"bootstrap"`
		Username  string `json:"username"`
		Password  string `json:"password"`
	}
	if !decodeAPIBody(w, r, &req) {
		return
	}
	if len(req.Password) < minAdminPassword {
		writeAPIError(w, http.StatusBadRequest, "weak_password", "密码至少 8 位", "password")
		return
	}
	if c.admin.gate() != gateSetup {
		writeAPIError(w, http.StatusConflict, "not_in_setup", "面板不处于首次设置状态", "")
		return
	}
	agent := c.admin.agentClient()
	login, err := agent.Bootstrap(req.Bootstrap, req.Username, req.Password)
	if err != nil {
		sleepAuthFail()
		// One message for every refusal: a wrong bootstrap and a consumed one
		// must look the same from outside.
		writeAPIError(w, http.StatusBadRequest, "setup_rejected", "无法创建账户：一次性凭据无效或已被使用", "bootstrap")
		return
	}
	c.answerSignedIn(w, login)
}

func (c *console) answerSignedIn(w http.ResponseWriter, login agentLogin) {
	session := signedSession{
		User:    login.User.Username,
		Agent:   login.Session,
		Display: login.User.Role,
	}
	cookie, err := c.admin.issueCookie(session)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "session_error", err.Error(), "")
		return
	}
	http.SetCookie(w, cookie)
	user := login.User
	writeAPIJSON(w, http.StatusOK, apiSessionView{
		OK:      true,
		Gate:    "ready",
		User:    &user,
		CSRF:    c.admin.csrfToken(session),
		Version: version,
		Agent:   c.admin.agentBase(),
	})
}

// serveAPILogout is a write (it ends a session) but is allowed without a valid
// cookie: signing out of an already expired session must still succeed.
func (c *console) serveAPILogout(w http.ResponseWriter, r *http.Request) {
	if !apiMethods(w, r, http.MethodPost) {
		return
	}
	if reason, ok := apiWriteAllowed(r); !ok {
		writeAPIError(w, http.StatusForbidden, "forbidden", reason, "")
		return
	}
	if session, ok := c.admin.session(r); ok {
		want := c.admin.csrfToken(session)
		if subtle.ConstantTimeCompare([]byte(r.Header.Get(apiCSRFHeader)), []byte(want)) != 1 {
			writeAPIError(w, http.StatusForbidden, "csrf", "missing or stale CSRF token; reload the page", "")
			return
		}
		if agent := c.admin.agentClient(); agent != nil {
			_ = agent.Logout(session.Agent)
		}
	}
	http.SetCookie(w, expireCookie())
	writeAPIJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// registerAPI mounts every API route. Unknown API paths answer a JSON 404
// rather than falling through to the front end's index.html.
func (c *console) registerAPI(mux *http.ServeMux) {
	mux.HandleFunc(apiPrefix+"session", c.serveAPISession)
	mux.HandleFunc(apiPrefix+"session/login", c.serveAPILogin)
	mux.HandleFunc(apiPrefix+"session/setup", c.serveAPISetup)
	mux.HandleFunc(apiPrefix+"session/logout", c.serveAPILogout)
	c.registerResourceAPI(mux)
	mux.HandleFunc(apiPrefix, func(w http.ResponseWriter, r *http.Request) {
		writeAPIError(w, http.StatusNotFound, "not_found", "no such API endpoint", "")
	})
}
