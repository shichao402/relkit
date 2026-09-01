package main

// Operator-panel authentication.
//
// A freshly inited box has a one-shot bootstrap token (hash only on disk).
// That token can create the first operator account and is consumed in the
// same write; afterwards the panel is ordinary username/password plus a
// signed cookie. Recovery is SSH: `relkit-serve init -reset-admin`.
//
// This is not a token-issuance API and does not protect the download tree.
// Upload tokens stay PUT-only; adding products still requires SSH.

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const (
	adminStateFileName = ".relkit-serve-admin.json"
	adminCookieName    = "relkit_admin"
	adminLoginPath     = "/-/admin/login"
	adminSetupPath     = "/-/admin/setup"
	adminLogoutPath    = "/-/admin/logout"
	adminSessionTTL    = 12 * time.Hour
	minAdminPassword   = 8
)

type panelUserKey struct{}

var (
	passwordCost  = bcrypt.DefaultCost
	authFailDelay = 400 * time.Millisecond
)

type adminUser struct {
	Username     string `json:"username"`
	PasswordHash string `json:"passwordHash"`
}

type adminDoc struct {
	SessionKey    string      `json:"sessionKey"`
	BootstrapHash string      `json:"bootstrapHash,omitempty"`
	Users         []adminUser `json:"users,omitempty"`
}

type adminAuth struct {
	mu            sync.Mutex
	path          string
	serveKey      string
	sessionKey    []byte
	bootstrapHash []byte
	users         []adminUser
}

type panelGate int

const (
	gateLocked panelGate = iota
	gateSetup
	gateLogin
)

type authPage struct {
	pageChrome
	Action string
	Next   string
	Error  string
}

func resolveAdminPath(rootPath, configPath, configured string) string {
	configured = strings.TrimSpace(configured)
	if configured == "" {
		return filepath.Join(rootPath, adminStateFileName)
	}
	if filepath.IsAbs(configured) {
		return filepath.Clean(configured)
	}
	if configPath != "" {
		return resolveRelative(configured, configPath)
	}
	return filepath.Join(rootPath, filepath.FromSlash(configured))
}

func adminStateFileFrom(cfg *FileConfig) string {
	if cfg == nil {
		return ""
	}
	return cfg.AdminStateFile
}

func reservedAdminKey(name string) bool {
	base := path.Base(name)
	return base == adminStateFileName ||
		base == "admin.json" ||
		strings.HasPrefix(base, adminStateFileName+".") ||
		strings.HasPrefix(base, "admin.json.")
}

func hiddenAdminKey(name string, a *adminAuth) bool {
	if reservedAdminKey(name) {
		return true
	}
	if a == nil || a.serveKey == "" {
		return false
	}
	return name == a.serveKey || name == a.serveKey+".tmp~"
}

func (c *config) hiddenKey(name string) bool {
	return hiddenServeKey(name, c.stats) || hiddenAdminKey(name, c.admin)
}

func mintAdminDoc() (plaintext string, doc adminDoc, err error) {
	token, err := generateToken()
	if err != nil {
		return "", adminDoc{}, err
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return "", adminDoc{}, err
	}
	return token, adminDoc{
		SessionKey:    base64.RawURLEncoding.EncodeToString(key),
		BootstrapHash: hex.EncodeToString(hashToken(token)),
	}, nil
}

func writeAdminDoc(path string, doc adminDoc) error {
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

// writeNewAdminState prefers the serve directory (writable under the default
// systemd unit). It will not create that directory: a typo in -dir must not
// mkdir /srv/releases. If the serve dir is missing, it falls back next to
// the config so `init` still produces a usable pair; the caller then records
// adminStateFile so the running process can find it.
func writeNewAdminState(serveDir, outDir string) (plaintext, written, relForConfig string, err error) {
	token, doc, err := mintAdminDoc()
	if err != nil {
		return "", "", "", err
	}
	if info, statErr := os.Stat(serveDir); statErr == nil && info.IsDir() {
		servePath := filepath.Join(serveDir, adminStateFileName)
		if err := writeAdminDoc(servePath, doc); err == nil {
			return token, servePath, "", nil
		}
	}
	fallback := filepath.Join(outDir, "admin.json")
	if err := writeAdminDoc(fallback, doc); err != nil {
		return "", "", "", fmt.Errorf("admin state: %w", err)
	}
	return token, fallback, "admin.json", nil
}

func openAdminAuth(path, rootPath string) (*adminAuth, error) {
	a := &adminAuth{path: path, serveKey: statsServeKey(rootPath, path)}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return a, nil
		}
		return nil, err
	}
	var doc adminDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if err := a.apply(doc); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return a, nil
}

func (a *adminAuth) apply(doc adminDoc) error {
	key, err := decodeSessionKey(doc.SessionKey)
	if err != nil {
		return err
	}
	var bootstrap []byte
	if doc.BootstrapHash != "" {
		sum, err := hex.DecodeString(doc.BootstrapHash)
		if err != nil || len(sum) != sha256.Size {
			return fmt.Errorf("bootstrapHash is not a sha256 hex digest")
		}
		bootstrap = sum
	}
	a.sessionKey = key
	a.bootstrapHash = bootstrap
	a.users = append([]adminUser(nil), doc.Users...)
	return nil
}

func decodeSessionKey(raw string) ([]byte, error) {
	key, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(raw))
	if err != nil || len(key) < 32 {
		return nil, fmt.Errorf("sessionKey is missing or too short")
	}
	return key, nil
}

func (a *adminAuth) snapshot() adminDoc {
	doc := adminDoc{
		SessionKey: base64.RawURLEncoding.EncodeToString(a.sessionKey),
		Users:      append([]adminUser(nil), a.users...),
	}
	if len(a.bootstrapHash) == sha256.Size {
		doc.BootstrapHash = hex.EncodeToString(a.bootstrapHash)
	}
	return doc
}

func (a *adminAuth) persistLocked() error {
	if a.path == "" {
		return fmt.Errorf("admin state path is empty")
	}
	return writeAdminDoc(a.path, a.snapshot())
}

func (a *adminAuth) gate() panelGate {
	if a == nil {
		return gateLocked
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.gateLocked()
}

func (a *adminAuth) gateLocked() panelGate {
	if len(a.users) > 0 {
		return gateLogin
	}
	if len(a.bootstrapHash) == sha256.Size {
		return gateSetup
	}
	return gateLocked
}

func (a *adminAuth) statusLog() string {
	switch a.gate() {
	case gateSetup:
		return "waiting for first operator (bootstrap live)"
	case gateLogin:
		n := 0
		a.mu.Lock()
		n = len(a.users)
		a.mu.Unlock()
		if n == 1 {
			return "authenticated (1 operator)"
		}
		return fmt.Sprintf("authenticated (%d operators)", n)
	default:
		return "locked (run relkit-serve init -reset-admin)"
	}
}

func (a *adminAuth) firstUsername() string {
	if a == nil {
		return ""
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.users) == 0 {
		return ""
	}
	return a.users[0].Username
}

func hashPassword(password string) (string, error) {
	raw, err := bcrypt.GenerateFromPassword([]byte(password), passwordCost)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func normalizeUsername(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if len(s) < 2 || len(s) > 32 {
		return "", fmt.Errorf("username must be 2–32 characters")
	}
	for _, r := range s {
		ok := r == '.' || r == '_' || r == '-' ||
			(r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
		if !ok {
			return "", fmt.Errorf("username may contain letters, digits, '.', '_' and '-'")
		}
	}
	return strings.ToLower(s), nil
}

func (a *adminAuth) createFirstOperator(bootstrap, username, password string) error {
	name, err := normalizeUsername(username)
	if err != nil {
		return err
	}
	if len(password) < minAdminPassword {
		return fmt.Errorf("password must be at least %d characters", minAdminPassword)
	}
	passHash, err := hashPassword(password)
	if err != nil {
		return err
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.users) > 0 {
		return fmt.Errorf("an operator account already exists")
	}
	if len(a.bootstrapHash) != sha256.Size {
		return fmt.Errorf("bootstrap is not live")
	}
	sum := hashToken(strings.TrimSpace(bootstrap))
	if subtle.ConstantTimeCompare(sum, a.bootstrapHash) != 1 {
		return fmt.Errorf("invalid bootstrap")
	}
	a.users = []adminUser{{Username: name, PasswordHash: passHash}}
	a.bootstrapHash = nil
	if err := a.persistLocked(); err != nil {
		a.users = nil
		a.bootstrapHash = sum
		return err
	}
	return nil
}

func (a *adminAuth) lookupPasswordHash(username string) string {
	name, err := normalizeUsername(username)
	if err != nil {
		return ""
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, user := range a.users {
		if user.Username == name {
			return user.PasswordHash
		}
	}
	return ""
}

var (
	dummyHashOnce sync.Once
	dummyHash     []byte
)

func passwordOK(hash, password string) bool {
	if hash == "" {
		dummyHashOnce.Do(func() {
			dummyHash, _ = bcrypt.GenerateFromPassword([]byte("timing-dummy"), passwordCost)
		})
		_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

func (a *adminAuth) issueCookie(username string, expiry time.Time) (*http.Cookie, error) {
	if a == nil {
		return nil, fmt.Errorf("admin state is not loaded")
	}
	a.mu.Lock()
	key := append([]byte(nil), a.sessionKey...)
	a.mu.Unlock()
	if len(key) < 32 {
		return nil, fmt.Errorf("session key is missing")
	}
	payload := username + "|" + fmt.Sprintf("%d", expiry.Unix())
	mac := hmacSHA256(key, []byte("v1|"+payload))
	value := "v1." +
		base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." +
		base64.RawURLEncoding.EncodeToString(mac)
	return &http.Cookie{
		Name:     adminCookieName,
		Value:    value,
		Path:     "/-/",
		Expires:  expiry,
		MaxAge:   int(time.Until(expiry).Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}, nil
}

func hmacSHA256(key, payload []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(payload)
	return mac.Sum(nil)
}

func (a *adminAuth) sessionUser(r *http.Request) (string, bool) {
	if a == nil || r == nil {
		return "", false
	}
	cookie, err := r.Cookie(adminCookieName)
	if err != nil || cookie.Value == "" {
		return "", false
	}
	a.mu.Lock()
	key := append([]byte(nil), a.sessionKey...)
	users := append([]adminUser(nil), a.users...)
	a.mu.Unlock()
	if len(key) < 32 {
		return "", false
	}
	user, ok := parseSessionCookie(cookie.Value, key, time.Now())
	if !ok {
		return "", false
	}
	for _, u := range users {
		if u.Username == user {
			return user, true
		}
	}
	return "", false
}

func parseSessionCookie(value string, key []byte, now time.Time) (string, bool) {
	parts := strings.Split(value, ".")
	if len(parts) != 3 || parts[0] != "v1" {
		return "", false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", false
	}
	mac, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return "", false
	}
	want := hmacSHA256(key, []byte("v1|"+string(payload)))
	if subtle.ConstantTimeCompare(mac, want) != 1 {
		return "", false
	}
	user, stamp, ok := strings.Cut(string(payload), "|")
	if !ok || user == "" {
		return "", false
	}
	var unix int64
	if _, err := fmt.Sscan(stamp, &unix); err != nil || now.Unix() > unix {
		return "", false
	}
	return user, true
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

func panelUsername(r *http.Request) string {
	u, _ := r.Context().Value(panelUserKey{}).(string)
	return u
}

func (c *config) stampChrome(r *http.Request, data any) {
	user := panelUsername(r)
	if user == "" {
		return
	}
	switch page := data.(type) {
	case *portalPage:
		page.LoggedIn = true
		page.Username = user
	case *productPage:
		page.LoggedIn = true
		page.Username = user
	case *listingPage:
		page.LoggedIn = true
		page.Username = user
	case *authPage:
		page.LoggedIn = true
		page.Username = user
	}
}

func (c *config) requirePanelAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if user, ok := c.admin.sessionUser(r); ok {
			ctx := context.WithValue(r.Context(), panelUserKey{}, user)
			next(w, r.WithContext(ctx))
			return
		}
		switch c.admin.gate() {
		case gateSetup:
			http.Redirect(w, r, adminSetupPath, http.StatusFound)
		case gateLogin:
			target := adminLoginPath
			if next := r.URL.RequestURI(); next != "" && next != adminPath {
				target += "?next=" + url.QueryEscape(next)
			}
			http.Redirect(w, r, target, http.StatusFound)
		default:
			c.serveAdminLocked(w, r)
		}
	}
}

func (c *config) serveAdminLogin(w http.ResponseWriter, r *http.Request) {
	if user, ok := c.admin.sessionUser(r); ok {
		ctx := context.WithValue(r.Context(), panelUserKey{}, user)
		http.Redirect(w, r.WithContext(ctx), safeNext(r.URL.Query().Get("next")), http.StatusFound)
		return
	}
	switch r.Method {
	case http.MethodGet, http.MethodHead:
		c.renderAuth(w, r, "login", "", "")
	case http.MethodPost:
		if !sameOriginPOST(r) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if err := r.ParseForm(); err != nil {
			c.renderAuth(w, r, "login", "Could not read the form.", r.FormValue("next"))
			return
		}
		next := safeNext(r.FormValue("next"))
		user := r.FormValue("username")
		pass := r.FormValue("password")
		hash := c.admin.lookupPasswordHash(user)
		if !passwordOK(hash, pass) {
			sleepAuthFail()
			c.renderAuth(w, r, "login", "Wrong username or password.", next)
			return
		}
		name, _ := normalizeUsername(user)
		c.setSession(w, name)
		http.Redirect(w, r, next, http.StatusFound)
	default:
		w.Header().Set("Allow", "GET, HEAD, POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (c *config) serveAdminSetup(w http.ResponseWriter, r *http.Request) {
	if user, ok := c.admin.sessionUser(r); ok {
		ctx := context.WithValue(r.Context(), panelUserKey{}, user)
		http.Redirect(w, r.WithContext(ctx), adminPath, http.StatusFound)
		return
	}
	if c.admin.gate() != gateSetup {
		if c.admin.gate() == gateLogin {
			http.Redirect(w, r, adminLoginPath, http.StatusFound)
			return
		}
		c.serveAdminLocked(w, r)
		return
	}
	switch r.Method {
	case http.MethodGet, http.MethodHead:
		c.renderAuth(w, r, "setup", "", "")
	case http.MethodPost:
		if !sameOriginPOST(r) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if err := r.ParseForm(); err != nil {
			c.renderAuth(w, r, "setup", "Could not read the form.", "")
			return
		}
		if r.FormValue("password") != r.FormValue("password2") {
			c.renderAuth(w, r, "setup", "Passwords do not match.", "")
			return
		}
		err := c.admin.createFirstOperator(
			r.FormValue("bootstrap"),
			r.FormValue("username"),
			r.FormValue("password"),
		)
		if err != nil {
			sleepAuthFail()
			msg := err.Error()
			if strings.Contains(msg, "invalid bootstrap") ||
				strings.Contains(msg, "not live") ||
				strings.Contains(msg, "already exists") {
				msg = "Could not create the account."
			}
			c.renderAuth(w, r, "setup", msg, "")
			return
		}
		name, _ := normalizeUsername(r.FormValue("username"))
		c.setSession(w, name)
		http.Redirect(w, r, adminPath, http.StatusFound)
	default:
		w.Header().Set("Allow", "GET, HEAD, POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (c *config) serveAdminLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodPost {
		w.Header().Set("Allow", "GET, HEAD, POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	http.SetCookie(w, expireCookie())
	http.Redirect(w, r, adminLoginPath, http.StatusFound)
}

func (c *config) serveAdminLocked(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	c.renderAuth(w, r, "locked", "", "")
}

func (c *config) renderAuth(w http.ResponseWriter, r *http.Request, mode, errMsg, next string) {
	title := "Sign in"
	switch mode {
	case "setup":
		title = "Create operator"
	case "locked":
		title = "Panel locked"
	}
	page := &authPage{
		pageChrome: pageChrome{Title: title, Version: version},
		Action:     r.URL.Path,
		Next:       next,
		Error:      errMsg,
	}
	c.renderPage(w, r, mode, page)
}

func (c *config) setSession(w http.ResponseWriter, username string) {
	cookie, err := c.admin.issueCookie(username, time.Now().Add(adminSessionTTL))
	if err != nil {
		return
	}
	http.SetCookie(w, cookie)
}

func sleepAuthFail() {
	if authFailDelay > 0 {
		time.Sleep(authFailDelay)
	}
}

func sameOriginPOST(r *http.Request) bool {
	if origin := r.Header.Get("Origin"); origin != "" {
		u, err := url.Parse(origin)
		return err == nil && strings.EqualFold(u.Host, r.Host)
	}
	if ref := r.Header.Get("Referer"); ref != "" {
		u, err := url.Parse(ref)
		return err == nil && strings.EqualFold(u.Host, r.Host)
	}
	return true
}

func safeNext(raw string) string {
	if raw == "" {
		return adminPath
	}
	u, err := url.Parse(raw)
	if err != nil || u.IsAbs() || u.Host != "" {
		return adminPath
	}
	if !strings.HasPrefix(u.Path, "/-/") {
		return adminPath
	}
	switch u.Path {
	case adminLoginPath, adminSetupPath, adminLogoutPath:
		return adminPath
	}
	return u.RequestURI()
}

func printAdminBootstrap(out io.Writer, path, token string) {
	fmt.Fprintf(out, "admin  %s (mode 0600, hash only)\n", path)
	fmt.Fprintf(out, "\nOpen /-/admin and create the first operator with this one-shot bootstrap.\n")
	fmt.Fprintf(out, "It is consumed the moment that account exists; do not store it:\n")
	fmt.Fprintf(out, "  export RELKIT_ADMIN_BOOTSTRAP='%s'\n", token)
}
