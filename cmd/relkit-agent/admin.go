package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/shichao402/relkit/internal/agentadmin"
	"github.com/shichao402/relkit/internal/publish"
	vnextrelease "github.com/shichao402/relkit/internal/vnext/release"
	vnexttrust "github.com/shichao402/relkit/internal/vnext/trust"
)

// The admin surface is the one place an operator changes what this server
// authorizes: users, roles, products, tokens. It exists because the epoch
// upgrade moved products and credentials out of the config file, and because the
// panel's writes have to travel through the agent rather than touch its storage
// directly (requirements 11.3, 13.6).
//
// Two facts shape the design:
//
//   - A token answers exactly one question: which product may this caller
//     operate. It is never a trust anchor, so nothing here signs anything
//     (requirement 11.2).
//   - Deleting a product is the same operation as retiring it (requirements 8.3,
//     13.5). The registration is what the read face answers with, so the assets
//     go first and the registration goes last.
const adminPrefix = "/v1/admin/"

// handleAdmin is one dispatcher because every route needs the same three
// preamble steps: method check, session, role check. Splitting them across
// handlers would duplicate the authorization decision, which is exactly how a
// hole appears.
func (s *Server) handleAdmin(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, adminPrefix)
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	switch parts[0] {
	case "status":
		s.adminStatus(w, r)
	case "bootstrap":
		s.adminBootstrap(w, r)
	case "login":
		s.adminLogin(w, r)
	case "logout":
		s.adminLogout(w, r)
	case "whoami":
		s.adminWhoami(w, r)
	case "users":
		if len(parts) == 1 {
			s.adminUsers(w, r)
			return
		}
		s.adminUser(w, r, parts[1], parts[2:])
	case "products":
		if len(parts) == 1 {
			s.adminProducts(w, r)
			return
		}
		s.adminProduct(w, r, parts[1], parts[2:])
	case "tokens":
		if len(parts) >= 3 {
			s.adminTokenAction(w, r, parts[1], parts[2])
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
	default:
		http.Error(w, "not found", http.StatusNotFound)
	}
}

func (s *Server) adminStore() *agentadmin.Store { return s.cfg.admin }

// adminStatus is deliberately unauthenticated: the panel has to know whether to
// draw the first-run screen before anyone can sign in. It answers only that.
func (s *Server) adminStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	store := s.adminStore()
	if store == nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "bootstrapLive": false})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":             true,
		"bootstrapLive":  store.BootstrapLive(),
		"operatorExists": len(store.Users()) > 0,
	})
}

func (s *Server) adminBootstrap(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	store := s.adminStore()
	if store == nil {
		http.Error(w, "admin state is not configured", http.StatusServiceUnavailable)
		return
	}
	var req struct {
		Bootstrap string `json:"bootstrap"`
		Username  string `json:"username"`
		Password  string `json:"password"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	user, err := store.CreateFirstUser(req.Bootstrap, req.Username, req.Password)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	token, expires, err := store.NewSession(user.Username, agentadmin.DefaultSessionTTL)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":        true,
		"user":      user,
		"session":   token,
		"expiresAt": expires.UTC().Format("2006-01-02T15:04:05Z07:00"),
	})
}

func (s *Server) adminLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	store := s.adminStore()
	if store == nil {
		http.Error(w, "admin state is not configured", http.StatusServiceUnavailable)
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	user, ok := store.Authenticate(req.Username, req.Password)
	if !ok {
		// One message for both wrong password and unknown account: the login form
		// must not become a user directory.
		http.Error(w, "invalid username or password", http.StatusUnauthorized)
		return
	}
	token, expires, err := store.NewSession(user.Username, agentadmin.DefaultSessionTTL)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":        true,
		"user":      user,
		"session":   token,
		"expiresAt": expires.UTC().Format("2006-01-02T15:04:05Z07:00"),
	})
}

func (s *Server) adminLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	store := s.adminStore()
	if store == nil {
		http.Error(w, "admin state is not configured", http.StatusServiceUnavailable)
		return
	}
	if token := bearerValue(r); token != "" {
		store.DropSession(token)
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) adminWhoami(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	user, ok := s.requireOperator(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "user": user})
}

// requireOperator resolves the session bearer. Sessions live in process memory:
// a restart signs everyone out, which is the right default for a surface that can
// mint credentials.
func (s *Server) requireOperator(w http.ResponseWriter, r *http.Request) (agentadmin.User, bool) {
	store := s.adminStore()
	if store == nil {
		http.Error(w, "admin state is not configured", http.StatusServiceUnavailable)
		return agentadmin.User{}, false
	}
	user, ok := store.SessionUser(bearerValue(r))
	if !ok {
		w.Header().Set("WWW-Authenticate", `Bearer realm="relkit-agent-admin"`)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return agentadmin.User{}, false
	}
	return user, true
}

// requireAdmin is the gate for anything that changes identity or the product
// list itself. A product owner only manages the tokens of its own products.
func (s *Server) requireAdmin(w http.ResponseWriter, r *http.Request) (agentadmin.User, bool) {
	user, ok := s.requireOperator(w, r)
	if !ok {
		return agentadmin.User{}, false
	}
	if user.Role != agentadmin.RoleAdmin {
		http.Error(w, fmt.Sprintf("forbidden: %q is %s; only an admin manages users and products", user.Username, user.Role), http.StatusForbidden)
		return agentadmin.User{}, false
	}
	return user, true
}

// requireProductAccess lets an admin reach every product and an owner reach only
// the products it was assigned.
func (s *Server) requireProductAccess(w http.ResponseWriter, r *http.Request, product string) (agentadmin.User, bool) {
	user, ok := s.requireOperator(w, r)
	if !ok {
		return agentadmin.User{}, false
	}
	if !operatorCovers(user, product) {
		http.Error(w, fmt.Sprintf("forbidden: %q is not assigned to product %q", user.Username, product), http.StatusForbidden)
		return agentadmin.User{}, false
	}
	return user, true
}

func operatorCovers(user agentadmin.User, product string) bool {
	if user.Role == agentadmin.RoleAdmin {
		return true
	}
	return slices.Contains(user.Products, product)
}

func (s *Server) adminUsers(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	store := s.adminStore()
	switch r.Method {
	case http.MethodGet, http.MethodHead:
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "users": store.Users()})
	case http.MethodPost:
		var req struct {
			Username string            `json:"username"`
			Password string            `json:"password"`
			Role     agentadmin.Role   `json:"role"`
			Products []string          `json:"products"`
		}
		if !decodeJSONBody(w, r, &req) {
			return
		}
		user, err := store.AddUser(req.Username, req.Password, req.Role, req.Products)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "user": user})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) adminUser(w http.ResponseWriter, r *http.Request, name string, rest []string) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	store := s.adminStore()
	if len(rest) == 0 {
		if r.Method != http.MethodDelete {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if err := store.DeleteUser(name); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "deleted": name})
		return
	}
	switch rest[0] {
	case "password":
		if r.Method != http.MethodPost && r.Method != http.MethodPut {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Password string `json:"password"`
		}
		if !decodeJSONBody(w, r, &req) {
			return
		}
		if err := store.SetPassword(name, req.Password); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "user": name})
	case "products":
		if r.Method != http.MethodPut && r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Products []string `json:"products"`
		}
		if !decodeJSONBody(w, r, &req) {
			return
		}
		user, err := store.SetUserProducts(name, req.Products)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "user": user})
	case "role":
		// The panel has a roles page, and a role is not a field it may write
		// directly (requirement 13.6). Changing role and product assignment
		// together is one operation because demoting an admin requires naming
		// the products it will own.
		if r.Method != http.MethodPut && r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Role     agentadmin.Role `json:"role"`
			Products []string        `json:"products"`
		}
		if !decodeJSONBody(w, r, &req) {
			return
		}
		user, err := store.SetRole(name, req.Role, req.Products)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "user": user})
	default:
		http.Error(w, "not found", http.StatusNotFound)
	}
}

func (s *Server) adminProducts(w http.ResponseWriter, r *http.Request) {
	store := s.adminStore()
	if store == nil {
		http.Error(w, "admin state is not configured", http.StatusServiceUnavailable)
		return
	}
	switch r.Method {
	case http.MethodGet, http.MethodHead:
		user, ok := s.requireOperator(w, r)
		if !ok {
			return
		}
		products := store.Products()
		// The panel shows the paste-able connect block for every product, not
		// only in the response that created it: an operator signing in next
		// month still needs the server address and root public key, and those
		// are not secrets. The token is not part of it (requirement 13.4).
		visible := make([]map[string]any, 0, len(products))
		for _, product := range products {
			if !operatorCovers(user, product.ID) {
				continue
			}
			visible = append(visible, map[string]any{
				"product":       product,
				"connectConfig": s.connectConfig(r, product.ID),
			})
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "products": visible})
	case http.MethodPost:
		if _, ok := s.requireAdmin(w, r); !ok {
			return
		}
		var req struct {
			ID      string `json:"id"`
			Root    string `json:"root"`
			Profile string `json:"profile"`
			Label   string `json:"label"`
		}
		if !decodeJSONBody(w, r, &req) {
			return
		}
		root := strings.TrimSpace(req.Root)
		if root == "" {
			root = filepath.Join("/srv/relkit", req.ID)
		}
		if !filepath.IsAbs(root) {
			root = filepath.Join(filepath.Dir(s.cfg.ConfigPath), root)
		}
		root = mustAbs(root)
		if err := os.MkdirAll(root, 0o755); err != nil {
			http.Error(w, "create product root: "+err.Error(), http.StatusInternalServerError)
			return
		}
		product, err := store.AddProduct(req.ID, root, req.Profile)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		// Registering a product issues its first token in the same breath: a
		// product nobody can publish to is not yet a product (requirement 13.4).
		token, plaintext, err := store.IssueToken(product.ID, req.Label)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := s.reloadAdmin(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		resp := map[string]any{
			"ok":      true,
			"product": product,
			"token":   token,
			// The plaintext appears here and nowhere else, ever (requirement 11.3).
			"plaintext":         plaintext,
			"connectConfig":     s.connectConfig(r, product.ID),
			"tokenShownOnce":    true,
			"plaintextNotice":   "this value is not stored and cannot be shown again; put it in the host CI secret now",
			"profileMissing":    !fileExists(s.resolvedProfile(product)),
			"profileExpectedAt": s.resolvedProfile(product),
		}
		writeJSON(w, http.StatusOK, resp)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) adminProduct(w http.ResponseWriter, r *http.Request, id string, rest []string) {
	store := s.adminStore()
	if store == nil {
		http.Error(w, "admin state is not configured", http.StatusServiceUnavailable)
		return
	}
	if len(rest) == 0 {
		if r.Method != http.MethodDelete {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		s.adminRetireProduct(w, r, id)
		return
	}
	if rest[0] != "tokens" {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if _, ok := s.requireProductAccess(w, r, id); !ok {
		return
	}
	if _, ok := store.Product(id); !ok {
		http.Error(w, "unknown product", http.StatusNotFound)
		return
	}
	switch r.Method {
	case http.MethodGet, http.MethodHead:
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "tokens": store.Tokens(id)})
	case http.MethodPost:
		var req struct {
			Label string `json:"label"`
		}
		if r.ContentLength > 0 && !decodeJSONBody(w, r, &req) {
			return
		}
		token, plaintext, err := store.IssueToken(id, req.Label)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := s.reloadAdmin(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ok": true, "token": token, "plaintext": plaintext, "tokenShownOnce": true,
		})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// adminRetireProduct is requirement 8.3 and 13.5 in one place: retire the
// published assets through the official path, then forget the registration, then
// let the site rebuild reflect a product that no longer exists.
func (s *Server) adminRetireProduct(w http.ResponseWriter, r *http.Request, id string) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	store := s.adminStore()
	product, ok := s.product(id)
	if !ok {
		http.Error(w, "unknown product", http.StatusNotFound)
		return
	}
	dryRun := r.URL.Query().Get("dryRun") == "1" || r.URL.Query().Get("dryRun") == "true"

	mu := s.productLock(id)
	if !mu.TryLock() {
		http.Error(w, "publish or staged upload already in progress for this product", http.StatusConflict)
		return
	}
	defer mu.Unlock()

	cfg, source, err := loadUnpublishConfig(product, id, "", s.cfg.Root)
	if err != nil {
		// No project document means nothing was ever published from this tree:
		// publishing requires it, so there is no asset to retire. Removing the
		// registration is the whole operation, and refusing here would leave an
		// operator stuck with a product that cannot be deleted.
		if !fileExists(filepath.Join(product.Root, vnextrelease.ProjectFile)) {
			// A dry run must not carry out the very operation it is previewing.
			// Only the registration and its tokens are in scope here, so the
			// preview names exactly what a real run would drop.
			if dryRun {
				writeJSON(w, http.StatusOK, map[string]any{
					"ok": true, "dryRun": true, "removedTokens": 0,
					"log": []string{
						"product tree has no " + vnextrelease.ProjectFile + "; nothing was published",
						fmt.Sprintf("would remove registration %q and its %d token(s)", id, len(store.Tokens(id))),
					},
				})
				return
			}
			removedTokens, removeErr := store.RemoveProduct(id)
			if removeErr != nil {
				http.Error(w, removeErr.Error(), http.StatusInternalServerError)
				return
			}
			if reloadErr := s.reloadAdmin(); reloadErr != nil {
				http.Error(w, reloadErr.Error(), http.StatusInternalServerError)
				return
			}
			s.triggerSiteRebuild(id, "")
			writeJSON(w, http.StatusOK, map[string]any{
				"ok": true, "dryRun": dryRun, "removedTokens": removedTokens,
				"log": []string{"product tree has no " + vnextrelease.ProjectFile + "; nothing was published, registration removed"},
			})
			return
		}
		http.Error(w, "load product config: "+err.Error(), http.StatusInternalServerError)
		return
	}
	var logs []string
	logs = append(logs, "retire config: "+source)
	printer := func(line string) { logs = append(logs, line) }
	result, err := publish.Retire(cfg, nil, dryRun, printer)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error(), "log": logs})
		return
	}
	if len(result.Failures) > 0 {
		// The registration must outlive the assets, so a partial retirement stops
		// here: the operator reruns it instead of ending up with a product that is
		// half deleted and no longer addressable.
		writeJSON(w, http.StatusConflict, map[string]any{
			"ok": false, "error": "some documents could not be deleted; re-run retirement", "result": result, "log": logs,
		})
		return
	}
	resp := map[string]any{"ok": true, "dryRun": dryRun, "result": result, "log": logs}
	if !dryRun {
		removedTokens, err := store.RemoveProduct(id)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{
				"ok": false, "error": "assets retired but the registration could not be removed: " + err.Error(), "result": result, "log": logs,
			})
			return
		}
		if err := s.reloadAdmin(); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		resp["removedTokens"] = removedTokens
		s.triggerSiteRebuild(id, "")
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) adminTokenAction(w http.ResponseWriter, r *http.Request, id, action string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	store := s.adminStore()
	if store == nil {
		http.Error(w, "admin state is not configured", http.StatusServiceUnavailable)
		return
	}
	token, ok := store.Token(id)
	if !ok {
		http.Error(w, "unknown token", http.StatusNotFound)
		return
	}
	if _, ok := s.requireProductAccess(w, r, token.Product); !ok {
		return
	}
	switch action {
	case "rotate":
		rotated, plaintext, err := store.RotateToken(id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := s.reloadAdmin(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ok": true, "token": rotated, "plaintext": plaintext, "tokenShownOnce": true,
		})
	case "revoke":
		revoked, err := store.RevokeToken(id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := s.reloadAdmin(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "token": revoked})
	default:
		http.Error(w, "not found", http.StatusNotFound)
	}
}

// connectConfig is the block an operator pastes into a host project
// (requirement 13.4). It carries no secret: the token travels separately and is
// shown once.
func (s *Server) connectConfig(r *http.Request, product string) map[string]any {
	config := map[string]any{
		"server":  s.serverURL(r),
		"product": product,
	}
	if s.cfg.Root != nil {
		config["rootKeyId"] = s.cfg.Root.KeyID
		config["rootPublicKey"] = vnexttrust.SSHPublicKeyLine(s.cfg.Root.PublicKey)
		config["rootFingerprint"] = s.cfg.Root.Fingerprint
	}
	return config
}

func (s *Server) serverURL(r *http.Request) string {
	if url := strings.TrimSpace(s.cfg.PublicURL); url != "" {
		return strings.TrimSuffix(url, "/")
	}
	if r != nil {
		if url := requestBaseURL(r); url != "" {
			return strings.TrimSuffix(url, "/")
		}
	}
	addr := strings.TrimSpace(s.cfg.Addr)
	if addr == "" {
		return ""
	}
	if strings.HasPrefix(addr, "http://") || strings.HasPrefix(addr, "https://") {
		return strings.TrimSuffix(addr, "/")
	}
	return "https://" + addr
}

func (s *Server) resolvedProfile(product agentadmin.Product) string {
	if strings.TrimSpace(product.Profile) != "" {
		profile := product.Profile
		if !filepath.IsAbs(profile) {
			profile = filepath.Join(filepath.Dir(s.cfg.ConfigPath), profile)
		}
		return mustAbs(profile)
	}
	return filepath.Join(filepath.Dir(s.cfg.ConfigPath), "products", product.ID+".json")
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func bearerValue(r *http.Request) string {
	value, found := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !found {
		return ""
	}
	return strings.TrimSpace(value)
}

func decodeJSONBody(w http.ResponseWriter, r *http.Request, target any) bool {
	defer r.Body.Close()
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(target); err != nil {
		http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
		return false
	}
	return true
}
