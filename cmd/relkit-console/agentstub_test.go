package main

// A stand-in for the agent's management API.
//
// The panel has no authority of its own any more (requirement 13.6): it renders
// forms and forwards them. Testing it therefore means testing the forwarding and
// the rendering, not a second implementation of the rules. This stub answers the
// same routes with the same shapes, and keeps just enough state (one admin, one
// product, tokens) for the panel's pages to have something to draw.
//
// It deliberately does not re-implement agentadmin's policy. Where the real agent
// refuses, this stub refuses with the same status code and a message the panel
// must render verbatim, so a regression in "the panel swallows the refusal" still
// shows up.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

type agentStub struct {
	mu             sync.Mutex
	bootstrapLive  bool
	bootstrapValue string
	users          []agentUser
	products       []agentProduct
	tokens         map[string][]agentToken // product -> tokens
	sessions       map[string]string       // bearer -> username
	nextToken      int
	// refuse, when set, makes every authenticated call answer 403 with this
	// text so the panel's refusal path can be exercised.
	refuse string
	// bootstrapRefusals counts the bootstrap attempts, so a test can prove the
	// panel forwards each one rather than caching a verdict.
	bootstrapAttempts int
}

func newAgentStub() *agentStub {
	return &agentStub{
		bootstrapLive: true,
		bootstrapValue: "stub-bootstrap",
		products:      nil,
		tokens:        map[string][]agentToken{},
		sessions:      map[string]string{},
	}
}

func (s *agentStub) addUser(username, password, role string, products []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.users = append(s.users, agentUser{
		Username:  username,
		Role:      role,
		Products:  products,
		CreatedAt: "2026-10-01T00:00:00Z",
	})
}

func (s *agentStub) addProduct(id, root string) agentToken {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.products = append(s.products, agentProduct{
		ID:        id,
		Root:      root,
		CreatedAt: "2026-10-01T00:00:00Z",
	})
	s.nextToken++
	token := agentToken{
		ID:        fmt.Sprintf("tok-%d", s.nextToken),
		Product:   id,
		Label:     "ci",
		CreatedAt: "2026-10-01T00:00:00Z",
	}
	s.tokens[id] = append(s.tokens[id], token)
	return token
}

func (s *agentStub) session(username string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	bearer := "stub-session-" + username
	s.sessions[bearer] = username
	return bearer
}

func (s *agentStub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	switch {
	case path == "/v1/admin/status":
		s.status(w, r)
	case path == "/v1/admin/bootstrap":
		s.bootstrap(w, r)
	case path == "/v1/admin/login":
		s.login(w, r)
	case path == "/v1/admin/logout":
		writeStubJSON(w, map[string]any{"ok": true})
	case path == "/v1/admin/whoami":
		s.whoami(w, r)
	case path == "/v1/admin/settings":
		if _, ok := s.gate(w, r); !ok {
			return
		}
		writeStubJSON(w, map[string]any{
			"ok": true, "productRootParent": "/data/relkit", "productRootParentSource": "config",
		})
	case path == "/v1/admin/users":
		s.usersRoute(w, r)
	case strings.HasPrefix(path, "/v1/admin/users/"):
		s.userRoute(w, r)
	case path == "/v1/admin/products":
		s.productsRoute(w, r)
	case strings.HasPrefix(path, "/v1/admin/products/"):
		s.productRoute(w, r)
	case strings.HasPrefix(path, "/v1/admin/tokens/"):
		s.tokenRoute(w, r)
	default:
		http.Error(w, "not found", http.StatusNotFound)
	}
}

func (s *agentStub) currentUser(r *http.Request) (agentUser, bool) {
	bearer := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	bearer = strings.TrimSpace(bearer)
	s.mu.Lock()
	defer s.mu.Unlock()
	name, ok := s.sessions[bearer]
	if !ok {
		return agentUser{}, false
	}
	for _, user := range s.users {
		if user.Username == name {
			return user, true
		}
	}
	return agentUser{}, false
}

func (s *agentStub) gate(w http.ResponseWriter, r *http.Request) (agentUser, bool) {
	user, ok := s.currentUser(r)
	if !ok {
		w.Header().Set("WWW-Authenticate", `Bearer realm="stub"`)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return agentUser{}, false
	}
	s.mu.Lock()
	refusal := s.refuse
	s.mu.Unlock()
	if refusal != "" {
		http.Error(w, refusal, http.StatusForbidden)
		return agentUser{}, false
	}
	return user, true
}

func (s *agentStub) status(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	live := s.bootstrapLive
	exists := len(s.users) > 0
	s.mu.Unlock()
	writeStubJSON(w, map[string]any{
		"ok":             true,
		"bootstrapLive":  live,
		"operatorExists": exists,
	})
}

func (s *agentStub) bootstrap(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.bootstrapAttempts++
	live := s.bootstrapLive
	expected := s.bootstrapValue
	s.mu.Unlock()
	var req struct {
		Bootstrap string `json:"bootstrap"`
		Username  string `json:"username"`
		Password  string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if !live || req.Bootstrap != expected {
		http.Error(w, "could not create the account", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	s.bootstrapLive = false
	s.users = append(s.users, agentUser{
		Username:  req.Username,
		Role:      "admin",
		CreatedAt: "2026-10-01T00:00:00Z",
	})
	bearer := "stub-session-" + req.Username
	s.sessions[bearer] = req.Username
	user := s.users[len(s.users)-1]
	s.mu.Unlock()
	writeStubJSON(w, map[string]any{
		"ok": true, "user": user, "session": bearer,
		"expiresAt": "2026-12-31T00:00:00Z",
	})
}

func (s *agentStub) login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	found := false
	var user agentUser
	for _, candidate := range s.users {
		if candidate.Username == req.Username {
			user, found = candidate, true
			break
		}
	}
	if found {
		bearer := "stub-session-" + req.Username
		s.sessions[bearer] = req.Username
		defer func() {
			s.mu.Unlock()
			writeStubJSON(w, map[string]any{
				"ok": true, "user": user, "session": bearer,
				"expiresAt": "2026-12-31T00:00:00Z",
			})
		}()
		return
	}
	s.mu.Unlock()
	http.Error(w, "invalid username or password", http.StatusUnauthorized)
}

func (s *agentStub) whoami(w http.ResponseWriter, r *http.Request) {
	user, ok := s.gate(w, r)
	if !ok {
		return
	}
	writeStubJSON(w, map[string]any{"ok": true, "user": user})
}

func (s *agentStub) usersRoute(w http.ResponseWriter, r *http.Request) {
	user, ok := s.gate(w, r)
	if !ok {
		return
	}
	if user.Role != "admin" {
		http.Error(w, fmt.Sprintf("forbidden: %q is %s; only an admin manages users and products", user.Username, user.Role), http.StatusForbidden)
		return
	}
	switch r.Method {
	case http.MethodGet, http.MethodHead:
		s.mu.Lock()
		users := append([]agentUser(nil), s.users...)
		s.mu.Unlock()
		writeStubJSON(w, map[string]any{"ok": true, "users": users})
	case http.MethodPost:
		var req struct {
			Username string   `json:"username"`
			Password string   `json:"password"`
			Role     string   `json:"role"`
			Products []string `json:"products"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(req.Username) == "" {
			http.Error(w, "username is required", http.StatusBadRequest)
			return
		}
		s.addUser(req.Username, req.Password, req.Role, req.Products)
		s.mu.Lock()
		created := s.users[len(s.users)-1]
		s.mu.Unlock()
		writeStubJSON(w, map[string]any{"ok": true, "user": created})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *agentStub) userRoute(w http.ResponseWriter, r *http.Request) {
	user, ok := s.gate(w, r)
	if !ok {
		return
	}
	if user.Role != "admin" {
		http.Error(w, "forbidden: only an admin manages users", http.StatusForbidden)
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/v1/admin/users/")
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	name := parts[0]
	if len(parts) == 1 {
		if r.Method != http.MethodDelete {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		s.mu.Lock()
		kept := s.users[:0]
		for _, candidate := range s.users {
			if candidate.Username != name {
				kept = append(kept, candidate)
			}
		}
		s.users = kept
		s.mu.Unlock()
		writeStubJSON(w, map[string]any{"ok": true, "deleted": name})
		return
	}
	switch parts[1] {
	case "password":
		writeStubJSON(w, map[string]any{"ok": true, "user": name})
	case "products":
		var req struct {
			Products []string `json:"products"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		writeStubJSON(w, map[string]any{"ok": true, "user": agentUser{Username: name, Products: req.Products}})
	case "role":
		var req struct {
			Role     string   `json:"role"`
			Products []string `json:"products"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if req.Role == "owner" && len(req.Products) == 0 {
			http.Error(w, "forbidden: a product owner needs at least one product", http.StatusBadRequest)
			return
		}
		s.mu.Lock()
		for i := range s.users {
			if s.users[i].Username == name {
				s.users[i].Role = req.Role
				s.users[i].Products = req.Products
			}
		}
		s.mu.Unlock()
		writeStubJSON(w, map[string]any{"ok": true, "user": agentUser{Username: name, Role: req.Role, Products: req.Products}})
	default:
		http.Error(w, "not found", http.StatusNotFound)
	}
}

func (s *agentStub) connectConfig(product string) map[string]any {
	return map[string]any{
		"server":        "https://stub.example.invalid",
		"product":       product,
		"rootKeyId":     "stub-root",
		"rootPublicKey": "ssh-ed25519 AAAAstub",
	}
}

func (s *agentStub) productsRoute(w http.ResponseWriter, r *http.Request) {
	user, ok := s.gate(w, r)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet, http.MethodHead:
		s.mu.Lock()
		views := make([]map[string]any, 0, len(s.products))
		for _, product := range s.products {
			views = append(views, map[string]any{
				"product":       product,
				"connectConfig": s.connectConfig(product.ID),
			})
		}
		s.mu.Unlock()
		writeStubJSON(w, map[string]any{"ok": true, "products": views})
	case http.MethodPost:
		if user.Role != "admin" {
			http.Error(w, "forbidden: only an admin registers products", http.StatusForbidden)
			return
		}
		var req struct {
			ID    string `json:"id"`
			Root  string `json:"root"`
			Label string `json:"label"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(req.ID) == "" {
			http.Error(w, "product id is required", http.StatusBadRequest)
			return
		}
		token := s.addProduct(req.ID, req.Root)
		writeStubJSON(w, map[string]any{
			"ok":            true,
			"product":       agentProduct{ID: req.ID, Root: req.Root},
			"token":         token,
			"plaintext":     "plain-" + token.ID,
			"connectConfig": s.connectConfig(req.ID),
		})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *agentStub) productRoute(w http.ResponseWriter, r *http.Request) {
	user, ok := s.gate(w, r)
	if !ok {
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/v1/admin/products/")
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	id := parts[0]
	if len(parts) == 1 {
		if r.Method != http.MethodDelete {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if user.Role != "admin" {
			http.Error(w, "forbidden: only an admin retires products", http.StatusForbidden)
			return
		}
		s.mu.Lock()
		kept := s.products[:0]
		for _, product := range s.products {
			if product.ID != id {
				kept = append(kept, product)
			}
		}
		s.products = kept
		removed := len(s.tokens[id])
		delete(s.tokens, id)
		s.mu.Unlock()
		writeStubJSON(w, map[string]any{"ok": true, "removedTokens": removed})
		return
	}
	if parts[1] != "tokens" {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	switch r.Method {
	case http.MethodGet, http.MethodHead:
		s.mu.Lock()
		tokens := append([]agentToken(nil), s.tokens[id]...)
		s.mu.Unlock()
		writeStubJSON(w, map[string]any{"ok": true, "tokens": tokens})
	case http.MethodPost:
		var req struct {
			Label string `json:"label"`
		}
		if r.ContentLength > 0 {
			_ = json.NewDecoder(r.Body).Decode(&req)
		}
		s.mu.Lock()
		s.nextToken++
		token := agentToken{
			ID:        fmt.Sprintf("tok-%d", s.nextToken),
			Product:   id,
			Label:     req.Label,
			CreatedAt: "2026-10-02T00:00:00Z",
		}
		s.tokens[id] = append(s.tokens[id], token)
		s.mu.Unlock()
		writeStubJSON(w, map[string]any{"ok": true, "token": token, "plaintext": "plain-" + token.ID})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *agentStub) tokenRoute(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.gate(w, r); !ok {
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/v1/admin/tokens/")
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) != 2 {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	id, action := parts[0], parts[1]
	s.mu.Lock()
	defer s.mu.Unlock()
	for product, tokens := range s.tokens {
		for i := range tokens {
			if tokens[i].ID != id {
				continue
			}
			switch action {
			case "rotate":
				tokens[i].RotatedAt = "2026-10-03T00:00:00Z"
				tokens[i].ID = id + "-r"
				s.tokens[product] = tokens
				writeStubJSON(w, map[string]any{
					"ok": true, "token": tokens[i], "plaintext": "plain-rotated-" + product,
				})
				return
			case "revoke":
				tokens[i].RevokedAt = "2026-10-03T00:00:00Z"
				s.tokens[product] = tokens
				writeStubJSON(w, map[string]any{"ok": true, "token": tokens[i]})
				return
			}
		}
	}
	http.Error(w, "unknown token", http.StatusNotFound)
}

func writeStubJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

// newAgentStubServer starts the stub and returns it with its URL. Cleanup is
// registered so a test never leaks the listener.
func newAgentStubServer(t *testing.T) (*agentStub, string) {
	t.Helper()
	stub := newAgentStub()
	srv := httptest.NewServer(stub)
	t.Cleanup(srv.Close)
	return stub, srv.URL
}
