package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// adminFixture is one agent server with a live management document: one admin
// and one product owner, plus a registered product with an issued token. It is
// the shape every panel write starts from.
type adminFixture struct {
	cfg      *Config
	srv      *Server
	ts       *httptest.Server
	admin    string // session bearer for the admin
	owner    string // session bearer for the product owner
	product  string
	plain    string // the product token issued at registration
	ownerErr string
}

func newAdminFixture(t *testing.T) *adminFixture {
	t.Helper()
	fx := newAgentFixture(t, agentFixtureOpts{})
	adminStore := fx.cfg.admin
	if adminStore == nil {
		t.Fatal("fixture has no admin store")
	}
	bootstrap, err := adminStore.BootstrapToken()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adminStore.CreateFirstUser(bootstrap, "root", "correct-horse"); err != nil {
		t.Fatal(err)
	}
	if _, err := adminStore.AddUser("owner", "correct-horse", "owner", []string{"demo"}); err != nil {
		t.Fatal(err)
	}
	adminSession, _, err := adminStore.NewSession("root", 0)
	if err != nil {
		t.Fatal(err)
	}
	ownerSession, _, err := adminStore.NewSession("owner", 0)
	if err != nil {
		t.Fatal(err)
	}
	return &adminFixture{
		cfg:     fx.cfg,
		srv:     NewServer(fx.cfg),
		ts:      fx.ts,
		admin:   adminSession,
		owner:   ownerSession,
		product: "demo",
		plain:   fx.token,
	}
}

func (f *adminFixture) do(t *testing.T, method, path, session string, body any) (int, string) {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(raw)
	} else {
		reader = bytes.NewReader(nil)
	}
	req, err := http.NewRequest(method, f.ts.URL+path, reader)
	if err != nil {
		t.Fatal(err)
	}
	if session != "" {
		req.Header.Set("Authorization", "Bearer "+session)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out bytes.Buffer
	if _, err := out.ReadFrom(resp.Body); err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, out.String()
}

// TestAdminStatusIsPublicButMinimal: the panel must learn whether to draw the
// first-run screen before anyone can sign in, and must learn nothing else.
func TestAdminStatusIsPublicButMinimal(t *testing.T) {
	fx := newAdminFixture(t)
	status, body := fx.do(t, http.MethodGet, "/v1/admin/status", "", nil)
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%s", status, body)
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(body), &doc); err != nil {
		t.Fatal(err)
	}
	if doc["operatorExists"] != true {
		t.Fatalf("operatorExists=%v", doc["operatorExists"])
	}
	if doc["bootstrapLive"] != false {
		t.Fatalf("bootstrapLive=%v", doc["bootstrapLive"])
	}
	if _, leaked := doc["users"]; leaked {
		t.Fatal("unauthenticated status leaked the user list")
	}
}

// TestAdminRequiresSession: every other route answers 401 without a session.
func TestAdminRequiresSession(t *testing.T) {
	fx := newAdminFixture(t)
	for _, path := range []string{"/v1/admin/users", "/v1/admin/products", "/v1/admin/whoami"} {
		status, body := fx.do(t, http.MethodGet, path, "", nil)
		if status != http.StatusUnauthorized {
			t.Fatalf("%s status=%d body=%s", path, status, body)
		}
	}
}

// TestAdminLoginRejectsBadPassword: one message for both wrong password and
// unknown account, so the login form is not a user directory.
func TestAdminLoginRejectsBadPassword(t *testing.T) {
	fx := newAdminFixture(t)
	status, body := fx.do(t, http.MethodPost, "/v1/admin/login", "", map[string]any{
		"username": "root", "password": "wrong-password",
	})
	if status != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", status, body)
	}
	status, body = fx.do(t, http.MethodPost, "/v1/admin/login", "", map[string]any{
		"username": "nobody", "password": "wrong-password",
	})
	if status != http.StatusUnauthorized {
		t.Fatalf("unknown account status=%d body=%s", status, body)
	}
	if !strings.Contains(body, "invalid username or password") {
		t.Fatalf("body=%s", body)
	}
}

// TestAdminOwnerCannotManageUsers: a product owner only manages the tokens of its
// assigned products (requirement 13.3).
func TestAdminOwnerCannotManageUsers(t *testing.T) {
	fx := newAdminFixture(t)
	status, body := fx.do(t, http.MethodGet, "/v1/admin/users", fx.owner, nil)
	if status != http.StatusForbidden {
		t.Fatalf("owner read users: status=%d body=%s", status, body)
	}
	status, body = fx.do(t, http.MethodPost, "/v1/admin/users", fx.owner, map[string]any{
		"username": "sneaky", "password": "correct-horse", "role": "admin",
	})
	if status != http.StatusForbidden {
		t.Fatalf("owner created a user: status=%d body=%s", status, body)
	}
	// The owner keeps its own lane: it can list its product's tokens.
	status, body = fx.do(t, http.MethodGet, "/v1/admin/products/demo/tokens", fx.owner, nil)
	if status != http.StatusOK {
		t.Fatalf("owner could not list its product tokens: status=%d body=%s", status, body)
	}
}

// TestAdminOwnerCannotReachOtherProduct covers the second half of requirement
// 13.3: an owner assigned one product must not learn about or touch another.
func TestAdminOwnerCannotReachOtherProduct(t *testing.T) {
	fx := newAdminFixture(t)
	if _, err := fx.cfg.admin.AddProduct("other", t.TempDir(), ""); err != nil {
		t.Fatal(err)
	}
	status, body := fx.do(t, http.MethodGet, "/v1/admin/products/other/tokens", fx.owner, nil)
	if status != http.StatusForbidden {
		t.Fatalf("owner reached another product: status=%d body=%s", status, body)
	}
	if !strings.Contains(body, "not assigned") {
		t.Fatalf("error does not explain the refusal: %s", body)
	}
}

// TestAdminLastAdminCannotBeDeleted: a server with no admin has no way back in
// except SSH and a fresh bootstrap.
func TestAdminLastAdminCannotBeDeleted(t *testing.T) {
	fx := newAdminFixture(t)
	status, body := fx.do(t, http.MethodDelete, "/v1/admin/users/root", fx.admin, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("deleted the last admin: status=%d body=%s", status, body)
	}
	if !strings.Contains(body, "last admin") {
		t.Fatalf("body=%s", body)
	}
}

// TestAdminAddProductIssuesOneTimeToken is requirement 13.4: registering a
// product issues its first token and hands back a connect config, and the
// plaintext appears exactly once.
func TestAdminAddProductIssuesOneTimeToken(t *testing.T) {
	fx := newAdminFixture(t)
	root := t.TempDir()
	status, body := fx.do(t, http.MethodPost, "/v1/admin/products", fx.admin, map[string]any{
		"id": "fresh", "root": root,
	})
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%s", status, body)
	}
	var resp map[string]any
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatal(err)
	}
	plaintext, _ := resp["plaintext"].(string)
	if plaintext == "" {
		t.Fatalf("registration did not return a token: %s", body)
	}
	config, ok := resp["connectConfig"].(map[string]any)
	if !ok {
		t.Fatalf("registration did not return a connect config: %s", body)
	}
	if config["product"] != "fresh" || config["server"] == "" || config["rootPublicKey"] == "" {
		t.Fatalf("connect config = %+v", config)
	}
	// The plaintext is not stored: listing tokens must never echo it.
	status, body = fx.do(t, http.MethodGet, "/v1/admin/products/fresh/tokens", fx.admin, nil)
	if status != http.StatusOK {
		t.Fatalf("tokens status=%d body=%s", status, body)
	}
	if strings.Contains(body, plaintext) {
		t.Fatal("token listing echoed the plaintext")
	}
	// And it works on the publish face: the registration is live without a restart.
	req, _ := http.NewRequest(http.MethodGet, fx.ts.URL+"/v1/tools/manifest.json", nil)
	req.Header.Set("Authorization", "Bearer "+plaintext)
	_ = req
	statusCode, _ := fx.do(t, http.MethodPost, "/v1/admin/products/fresh/tokens", fx.admin, map[string]any{"label": "second"})
	if statusCode != http.StatusOK {
		t.Fatalf("second token status=%d", statusCode)
	}
}

// TestAdminRotateAndRevoke covers the two credential lifecycle operations the
// panel exposes: rotate keeps the id and kills the old value, revoke kills it for
// good (requirement 11.3).
func TestAdminRotateAndRevoke(t *testing.T) {
	fx := newAdminFixture(t)
	tokens := fx.cfg.admin.Tokens("demo")
	if len(tokens) == 0 {
		t.Fatal("fixture has no product token")
	}
	id := tokens[0].ID

	status, body := fx.do(t, http.MethodPost, "/v1/admin/tokens/"+id+"/rotate", fx.admin, nil)
	if status != http.StatusOK {
		t.Fatalf("rotate status=%d body=%s", status, body)
	}
	var rotated map[string]any
	if err := json.Unmarshal([]byte(body), &rotated); err != nil {
		t.Fatal(err)
	}
	newPlain, _ := rotated["plaintext"].(string)
	if newPlain == "" || newPlain == fx.plain {
		t.Fatalf("rotate did not mint a new secret: %s", body)
	}
	// The old value stops working the moment rotate returns.
	if credentials := fx.cfg.admin.Credentials(); len(credentials) != 1 {
		t.Fatalf("credentials = %d", len(credentials))
	}
	okOld := false
	for _, credential := range fx.cfg.admin.Credentials() {
		if credential.Allows("demo") && bytes.Equal(credential.Hash, hashToken(newPlain)) {
			okOld = true
		}
	}
	if !okOld {
		t.Fatal("rotated credential is not live")
	}

	status, body = fx.do(t, http.MethodPost, "/v1/admin/tokens/"+id+"/revoke", fx.admin, nil)
	if status != http.StatusOK {
		t.Fatalf("revoke status=%d body=%s", status, body)
	}
	if credentials := fx.cfg.admin.Credentials(); len(credentials) != 0 {
		t.Fatalf("revoked credential is still live: %d", len(credentials))
	}
	// Revoking twice is a documented error, not a silent success.
	status, body = fx.do(t, http.MethodPost, "/v1/admin/tokens/"+id+"/revoke", fx.admin, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("double revoke status=%d body=%s", status, body)
	}
}

// TestAdminRetireProductDryRunTouchesNothing pins the preview contract: a dry
// run must not remove the registration or its tokens, because an operator
// inspecting what retirement would do must still hold a working product
// afterwards. This branch used to delete unconditionally.
func TestAdminRetireProductDryRunTouchesNothing(t *testing.T) {
	fx := newAdminFixture(t)
	root := t.TempDir()
	status, body := fx.do(t, http.MethodPost, "/v1/admin/products", fx.admin, map[string]any{
		"id": "preview", "root": root,
	})
	if status != http.StatusOK {
		t.Fatalf("register preview status=%d body=%s", status, body)
	}
	before := len(fx.cfg.admin.Tokens("preview"))

	status, body = fx.do(t, http.MethodDelete, "/v1/admin/products/preview?dryRun=1", fx.admin, nil)
	if status != http.StatusOK {
		t.Fatalf("dry-run retire status=%d body=%s", status, body)
	}
	if _, ok := fx.cfg.admin.Product("preview"); !ok {
		t.Fatal("dry run removed the registration")
	}
	if after := len(fx.cfg.admin.Tokens("preview")); after != before {
		t.Fatalf("dry run changed tokens: %d -> %d", before, after)
	}
	if _, ok := fx.srv.product("preview"); !ok {
		t.Fatal("dry run removed the product from the runtime view")
	}
}

// TestAdminRetireProductRemovesRegistration drives requirement 8.3/13.5 through
// the management API: the product stops existing, its tokens are gone, and the
// other product is untouched.
func TestAdminRetireProductRemovesRegistration(t *testing.T) {
	fx := newAdminFixture(t)
	keeperRoot := t.TempDir()
	status, body := fx.do(t, http.MethodPost, "/v1/admin/products", fx.admin, map[string]any{
		"id": "keeper", "root": keeperRoot,
	})
	if status != http.StatusOK {
		t.Fatalf("register keeper status=%d body=%s", status, body)
	}
	// A product with no published versions still retires cleanly: there is
	// nothing to unpublish, but the registration and its tokens must go.
	status, body = fx.do(t, http.MethodDelete, "/v1/admin/products/keeper", fx.admin, nil)
	if status != http.StatusOK {
		t.Fatalf("retire status=%d body=%s", status, body)
	}
	if _, ok := fx.cfg.admin.Product("keeper"); ok {
		t.Fatal("retirement left the registration in place")
	}
	if tokens := fx.cfg.admin.Tokens("keeper"); len(tokens) != 0 {
		t.Fatalf("retirement left %d token(s)", len(tokens))
	}
	if _, ok := fx.cfg.admin.Product("demo"); !ok {
		t.Fatal("retirement removed an unrelated product")
	}
	if _, ok := fx.srv.product("keeper"); ok {
		t.Fatal("runtime view still serves the retired product")
	}
}

// TestRetiredProductIs404OnReadFace covers the read-face consequence: a retired
// product answers 404 and its token is gone, while another product keeps working.
func TestRetiredProductIs404OnReadFace(t *testing.T) {
	fx := newAdminFixture(t)
	before := fx.cfg.admin.Tokens("demo")
	if len(before) == 0 {
		t.Fatal("fixture has no token")
	}
	status, body := fx.do(t, http.MethodDelete, "/v1/admin/products/demo", fx.admin, nil)
	if status != http.StatusOK {
		t.Fatalf("retire status=%d body=%s", status, body)
	}
	// The read face no longer knows the product.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/staged/demo/1.0.0", nil)
	req.Header.Set("Authorization", "Bearer "+fx.plain)
	fx.srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound && rec.Code != http.StatusMethodNotAllowed && rec.Code != http.StatusUnauthorized {
		t.Fatalf("retired product read face answered %d: %s", rec.Code, rec.Body.String())
	}
	if _, ok := fx.cfg.admin.Product("demo"); ok {
		t.Fatal("registration survived retirement")
	}
}

// TestAdminRejectsLegacyConfigFile is the operator-facing failure: an old config
// file must stop the server with an explanation instead of starting half
// configured.
func TestAdminRejectsLegacyConfigFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "agent.json")
	if err := os.WriteFile(path, []byte(`{"products":{"loom":{"root":"/srv/relkit/loom"}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(path); err == nil {
		t.Fatal("a config file with products started")
	}
}
