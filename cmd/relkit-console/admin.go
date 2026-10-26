package main

// Panel sessions, in front of the agent's management API.
//
// The epoch upgrade moved the authority for users, roles, products and tokens
// into the agent (requirement 13.6). The panel therefore keeps no account
// document of its own: it holds one signing key, so the browser can carry the
// agent's session bearer in a cookie the panel can trust, and everything else is
// an HTTP call.
//
// What the panel still owns is exactly two things: the cookie (so a page load
// costs one agent request, not one per fragment) and the redirect rules for the
// first-run screen. Both are presentation, not authority.
//
// A consequence worth stating because it used to be the opposite: `console init`
// no longer mints a bootstrap. The agent does (requirement 13.2, 11.3), and the
// panel simply renders the form that consumes it.

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	// adminStateFileName holds the panel's cookie signing key and nothing else.
	// It is panel state, not agent state: losing it signs everyone out.
	adminStateFileName = ".relkit-console-panel.json"
	adminCookieName    = "relkit_admin"
	adminLoginPath     = "/-/admin/login"
	adminSetupPath     = "/-/admin/setup"
	adminLogoutPath    = "/-/admin/logout"
	adminSessionTTL    = 12 * time.Hour
	minAdminPassword   = 8
)

type panelUserKey struct{}

var (
	authFailDelay = 400 * time.Millisecond
)

// panelDoc is the whole of the panel's local state.
type panelDoc struct {
	SessionKey string `json:"sessionKey"`
}

// panelSession is the panel's view of "who is signed in here": one signing key
// plus the agent client that answers everything else.
type panelSession struct {
	mu    sync.Mutex
	path  string
	key   []byte
	agent *agentClient
}

type panelGate int

const (
	gateUnavailable panelGate = iota
	gateLocked
	gateSetup
	gateLogin
)

// resolveAdminPath picks where the cookie key lives. It is the console's own
// state, so it lives in the console's state directory, never inside the release
// tree: the console reads that tree only through the store, like any client.
func resolveAdminPath(configured string) (string, error) {
	configured = strings.TrimSpace(configured)
	if configured == "" {
		return defaultPanelStatePath, nil
	}
	if !filepath.IsAbs(configured) {
		return "", fmt.Errorf("adminStateFile must be an absolute path, got %q", configured)
	}
	return filepath.Clean(configured), nil
}

func adminStateFileFrom(cfg *FileConfig) string {
	if cfg == nil {
		return ""
	}
	return cfg.AdminStateFile
}

// reservedAdminKey hides the panel's own state file from the file tree. The
// agent's admin document is not in this tree at all any more, but a box that
// migrated still has the old name lying around, so both stay reserved.
func reservedAdminKey(name string) bool {
	base := path.Base(name)
	return base == adminStateFileName ||
		base == adminStateFileName+".tmp~" ||
		base == legacyAdminStateFileName ||
		base == "admin.json" ||
		strings.HasPrefix(base, legacyAdminStateFileName+".") ||
		strings.HasPrefix(base, "admin.json.")
}

// legacyAdminStateFileName is the serve-era account document. The console does
// not read it; it is reserved so a leftover copy is never listed or served.
const legacyAdminStateFileName = ".relkit-serve-admin.json"

// hiddenKey is a second line behind the store's own filter: the store already
// refuses its internal files, and this keeps a leftover account or panel file
// from an older layout out of the file tree even if a store in front of an old
// tree does not know its name.
func (c *console) hiddenKey(name string) bool {
	base := path.Base(name)
	return reservedAdminKey(name) || strings.HasPrefix(base, ".relkit-serve-")
}

func mintPanelDoc() (panelDoc, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return panelDoc{}, err
	}
	return panelDoc{SessionKey: base64.RawURLEncoding.EncodeToString(key)}, nil
}

func writePanelDoc(path string, doc panelDoc) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp~"
	if err := os.WriteFile(tmp, append(raw, '\n'), 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// openPanelSession loads the signing key, minting one on first start so a fresh
// box needs no hand-written secret. The agent connection is separate: a panel
// with no agent configured still renders, it just cannot show any data.
func openPanelSession(path, agentURL string) (*panelSession, error) {
	s := &panelSession{path: path}
	if agentURL != "" {
		agent, err := newAgentClient(agentURL)
		if err != nil {
			return nil, err
		}
		s.agent = agent
	}
	raw, err := os.ReadFile(path)
	if err == nil {
		var doc panelDoc
		if err := json.Unmarshal(raw, &doc); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		key, err := decodeSessionKey(doc.SessionKey)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		s.key = key
		return s, nil
	}
	if !os.IsNotExist(err) {
		return nil, err
	}
	doc, err := mintPanelDoc()
	if err != nil {
		return nil, err
	}
	if path != "" {
		if err := writePanelDoc(path, doc); err != nil {
			return nil, err
		}
	}
	key, err := decodeSessionKey(doc.SessionKey)
	if err != nil {
		return nil, err
	}
	s.key = key
	return s, nil
}

func decodeSessionKey(raw string) ([]byte, error) {
	key, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(raw))
	if err != nil || len(key) < 32 {
		return nil, fmt.Errorf("sessionKey is missing or too short")
	}
	return key, nil
}

// agentClientOrNil keeps the "no agent configured" case explicit at call sites
// instead of scattering nil checks through the handlers.
func (s *panelSession) agentClient() *agentClient {
	if s == nil {
		return nil
	}
	return s.agent
}

// gate asks the agent whether this deployment still needs its first operator.
// The answer is not cached: the whole point of the bootstrap state is that it
// changes exactly once, and a stale cache would show the wrong screen at that
// moment.
func (s *panelSession) gate() panelGate {
	agent := s.agentClient()
	if agent == nil {
		return gateUnavailable
	}
	status, err := agent.Status()
	if err != nil {
		return gateUnavailable
	}
	if status.BootstrapLive {
		return gateSetup
	}
	if status.OperatorExists {
		return gateLogin
	}
	return gateLocked
}

func (s *panelSession) statusLog() string {
	switch s.gate() {
	case gateSetup:
		return "waiting for first operator (agent bootstrap live)"
	case gateLogin:
		return "ready (" + s.agentClient().base + ")"
	case gateUnavailable:
		return "agent unavailable: " + s.agentBase()
	default:
		return "locked (no operator on the agent: run `relkit-agent init -bootstrap`)"
	}
}

func (s *panelSession) agentBase() string {
	if agent := s.agentClient(); agent != nil {
		return agent.base
	}
	return "(not configured)"
}

// signedSession is what the cookie carries: the agent's bearer plus the name to
// display. Both are inside the MAC, so neither can be edited by the browser.
type signedSession struct {
	User    string `json:"u"`
	Expiry  int64  `json:"x"`
	Agent   string `json:"a"`
	Display string `json:"d,omitempty"`
}

func (s *panelSession) issueCookie(session signedSession) (*http.Cookie, error) {
	if s == nil {
		return nil, fmt.Errorf("panel state is not loaded")
	}
	s.mu.Lock()
	key := append([]byte(nil), s.key...)
	s.mu.Unlock()
	if len(key) < 32 {
		return nil, fmt.Errorf("session key is missing")
	}
	session.Expiry = time.Now().Add(adminSessionTTL).Unix()
	payload, err := json.Marshal(session)
	if err != nil {
		return nil, err
	}
	mac := hmacSHA256(key, []byte("v2|"+string(payload)))
	value := "v2." +
		base64.RawURLEncoding.EncodeToString(payload) + "." +
		base64.RawURLEncoding.EncodeToString(mac)
	return &http.Cookie{
		Name:     adminCookieName,
		Value:    value,
		Path:     "/-/",
		Expires:  time.Unix(session.Expiry, 0),
		MaxAge:   int(time.Until(time.Unix(session.Expiry, 0)).Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}, nil
}

func hmacSHA256(key, payload []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(payload)
	return mac.Sum(nil)
}

// session returns the agent bearer carried by this request, if the cookie is
// intact and unexpired. It does not ask the agent: callers that need the user
// record call Whoami, so a revoked session costs one request rather than being
// trusted from the cookie forever.
func (s *panelSession) session(r *http.Request) (signedSession, bool) {
	if s == nil || r == nil {
		return signedSession{}, false
	}
	cookie, err := r.Cookie(adminCookieName)
	if err != nil || cookie.Value == "" {
		return signedSession{}, false
	}
	s.mu.Lock()
	key := append([]byte(nil), s.key...)
	s.mu.Unlock()
	if len(key) < 32 {
		return signedSession{}, false
	}
	session, ok := parseSessionCookie(cookie.Value, key, time.Now())
	if !ok || session.Agent == "" {
		return signedSession{}, false
	}
	return session, true
}

func parseSessionCookie(value string, key []byte, now time.Time) (signedSession, bool) {
	parts := strings.Split(value, ".")
	if len(parts) != 3 || parts[0] != "v2" {
		return signedSession{}, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return signedSession{}, false
	}
	mac, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return signedSession{}, false
	}
	want := hmacSHA256(key, []byte("v2|"+string(payload)))
	if subtle.ConstantTimeCompare(mac, want) != 1 {
		return signedSession{}, false
	}
	var session signedSession
	if err := json.Unmarshal(payload, &session); err != nil {
		return signedSession{}, false
	}
	if session.User == "" || now.Unix() > session.Expiry {
		return signedSession{}, false
	}
	return session, true
}

func expireCookie() *http.Cookie {
	return &http.Cookie{
		Name:     adminCookieName,
		Value:    "",
		Path:     "/-/",
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}
}

// panelAgentKey carries the agent bearer through the request context.
type panelAgentKey struct{}

// withPanel stamps the signed-in operator and the agent bearer onto the request
// context, so an API handler can call the agent without re-reading the cookie.
func withPanel(r *http.Request, session signedSession) *http.Request {
	ctx := context.WithValue(r.Context(), panelUserKey{}, session.User)
	ctx = context.WithValue(ctx, panelAgentKey{}, session.Agent)
	return r.WithContext(ctx)
}

func sleepAuthFail() {
	if authFailDelay > 0 {
		time.Sleep(authFailDelay)
	}
}

func printPanelNote(out io.Writer, path string) {
	fmt.Fprintf(out, "panel state %s (mode 0600, cookie signing key only)\n", path)
	fmt.Fprintf(out, "Operators are managed by the agent: create the first one with\n")
	fmt.Fprintf(out, "  relkit-agent init -bootstrap\n")
	fmt.Fprintf(out, "then open /-/admin here and paste the value it prints.\n")
}
