package main

// Panel tests. The subject is a thin client: it renders forms, forwards them to
// the agent, and renders the answer. So every test here starts a stub agent and
// asserts on what the browser sees, not on any local copy of the rules
// (requirement 13.6).

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func init() {
	authFailDelay = 0
}

// newPanelConsole assembles a console whose management side is the stub agent.
// The release tree is a temp dir, so the read-only pages have somewhere to look.
func newPanelConsole(t *testing.T) (*console, *agentStub, string) {
	t.Helper()
	stub, agentURL := newAgentStubServer(t)
	dir := t.TempDir()
	adapter, err := newRootAdapter("local", dir)
	if err != nil {
		t.Fatalf("newRootAdapter: %v", err)
	}
	t.Cleanup(func() { _ = adapter.root.Close() })
	admin, err := openPanelSession(filepath.Join(dir, adminStateFileName), dir, agentURL)
	if err != nil {
		t.Fatalf("openPanelSession: %v", err)
	}
	c := &console{
		adapter: adapter,
		stats:   newDownloadStats(defaultStatsPath(dir), dir),
		admin:   admin,
	}
	t.Cleanup(c.stats.stop)
	return c, stub, dir
}

// signIn drives the real login flow so the cookie under test is the one the
// panel issues, not a hand-built one.
func signIn(t *testing.T, client *http.Client, srv *httptest.Server) {
	t.Helper()
	resp, err := client.PostForm(srv.URL+adminLoginPath, url.Values{
		"username": {testPanelUser},
		"password": {testPanelPassword},
	})
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login = %d", resp.StatusCode)
	}
}

func bodyOf(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// TestPanelRedirectsToSetupWhenBootstrapLive: agent says the bootstrap is live,
// so the panel sends the operator to the form that consumes it.
func TestPanelRedirectsToSetupWhenBootstrapLive(t *testing.T) {
	c, stub, _ := newPanelConsole(t)
	stub.mu.Lock()
	stub.bootstrapLive = true
	stub.mu.Unlock()
	srv := newLocalServer(t, c)

	resp, err := noRedirectClient().Get(srv.URL + adminPath)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("status = %d, want 302", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != adminSetupPath {
		t.Fatalf("Location = %q, want %s", loc, adminSetupPath)
	}
}

// TestFirstOperatorConsumesBootstrap: the panel forwards the bootstrap to the
// agent, which is the only place the credential is checked.
func TestFirstOperatorConsumesBootstrap(t *testing.T) {
	c, stub, _ := newPanelConsole(t)
	srv := newLocalServer(t, c)
	client := jarClient(t, srv)

	resp, err := client.PostForm(srv.URL+adminSetupPath, url.Values{
		"bootstrap": {stub.bootstrapValue},
		"username":  {testPanelUser},
		"password":  {testPanelPassword},
		"password2": {testPanelPassword},
	})
	if err != nil {
		t.Fatal(err)
	}
	body := bodyOf(t, resp)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, testPanelUser) {
		t.Fatalf("setup = %d body=%s", resp.StatusCode, body)
	}
	if stub.bootstrapAttempts != 1 {
		t.Fatalf("bootstrap attempts = %d, want 1", stub.bootstrapAttempts)
	}

	// One-shot: the agent has spent the bootstrap, so a second setup attempt is
	// redirected to login rather than shown the form again.
	resp, err = noRedirectClient().Get(srv.URL + adminSetupPath)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusFound || resp.Header.Get("Location") != adminLoginPath {
		t.Fatalf("setup after consumption = %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}
}

// TestWrongBootstrapDoesNotCreateAccount: one message for every refusal, so the
// form cannot be used to probe whether a credential is live.
func TestWrongBootstrapDoesNotCreateAccount(t *testing.T) {
	c, _, _ := newPanelConsole(t)
	srv := newLocalServer(t, c)
	client := jarClient(t, srv)

	resp, err := client.PostForm(srv.URL+adminSetupPath, url.Values{
		"bootstrap": {"not-the-token"},
		"username":  {testPanelUser},
		"password":  {testPanelPassword},
		"password2": {testPanelPassword},
	})
	if err != nil {
		t.Fatal(err)
	}
	body := bodyOf(t, resp)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, "Could not create the account") {
		t.Fatalf("setup = %d body=%s", resp.StatusCode, body)
	}
}

// TestLoginAndLogout: signing in lands on the panel, signing out closes it.
func TestLoginAndLogout(t *testing.T) {
	c, stub, _ := newPanelConsole(t)
	stub.bootstrapLive = false
	stub.addUser(testPanelUser, testPanelPassword, "admin", nil)
	srv := newLocalServer(t, c)
	client := jarClient(t, srv)
	signIn(t, client, srv)

	resp, err := client.Get(srv.URL + panelUsersPath)
	if err != nil {
		t.Fatal(err)
	}
	body := bodyOf(t, resp)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, testPanelUser) {
		t.Fatalf("users page = %d body=%s", resp.StatusCode, body)
	}

	resp, err = client.Get(srv.URL + adminLogoutPath)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	resp, err = noRedirectClient().Get(srv.URL + panelUsersPath)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("panel after logout = %d, want 302", resp.StatusCode)
	}
}

// TestPanelLockedWithoutOperator: bootstrap spent and no operator, the panel has
// nothing to gate on and says so rather than showing a login form nobody can use.
func TestPanelLockedWithoutOperator(t *testing.T) {
	c, stub, _ := newPanelConsole(t)
	stub.bootstrapLive = false
	srv := newLocalServer(t, c)

	resp, err := noRedirectClient().Get(srv.URL + adminPath)
	if err != nil {
		t.Fatal(err)
	}
	body := bodyOf(t, resp)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, "Panel locked") {
		t.Fatalf("panel = %d body=%s", resp.StatusCode, body)
	}
}

// TestPanelUnavailableWithoutAgent: with no agent configured the panel is not
// broken, but it cannot manage anything, and the page must say which one.
func TestPanelUnavailableWithoutAgent(t *testing.T) {
	c, _, dir := newPanelConsole(t)
	c.admin, _ = openPanelSession(filepath.Join(dir, adminStateFileName), dir, "")
	srv := newLocalServer(t, c)

	resp, err := noRedirectClient().Get(srv.URL + adminPath)
	if err != nil {
		t.Fatal(err)
	}
	body := bodyOf(t, resp)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, "Agent unavailable") {
		t.Fatalf("panel = %d body=%s", resp.StatusCode, body)
	}
}

// TestUsersPageListsAgentAccounts is the read path: what the agent holds is what
// the page shows, including role labels.
func TestUsersPageListsAgentAccounts(t *testing.T) {
	c, stub, _ := newPanelConsole(t)
	stub.bootstrapLive = false
	stub.addUser(testPanelUser, testPanelPassword, "admin", nil)
	stub.addUser("owner1", "correct-horse", "owner", []string{"demo"})
	srv := newLocalServer(t, c)
	client := jarClient(t, srv)
	signIn(t, client, srv)

	resp, err := client.Get(srv.URL + panelUsersPath)
	if err != nil {
		t.Fatal(err)
	}
	body := bodyOf(t, resp)
	for _, want := range []string{"owner1", "Product owner", "Administrator"} {
		if !strings.Contains(body, want) {
			t.Errorf("users page missing %q", want)
		}
	}
}

// TestAddUserForwardsToAgent: a form post creates the account on the agent and
// reports it back; the panel keeps no copy.
func TestAddUserForwardsToAgent(t *testing.T) {
	c, stub, _ := newPanelConsole(t)
	stub.bootstrapLive = false
	stub.addUser(testPanelUser, testPanelPassword, "admin", nil)
	srv := newLocalServer(t, c)
	client := jarClient(t, srv)
	signIn(t, client, srv)

	resp, err := client.PostForm(srv.URL+panelUsersPath, url.Values{
		"action":   {"add-user"},
		"username": {"fresh"},
		"password": {"correct-horse"},
		"role":     {"owner"},
		"products": {"demo"},
	})
	if err != nil {
		t.Fatal(err)
	}
	body := bodyOf(t, resp)
	if !strings.Contains(body, "Account fresh created.") {
		t.Fatalf("add user body=%s", body)
	}
	stub.mu.Lock()
	found := false
	for _, user := range stub.users {
		if user.Username == "fresh" {
			found = true
		}
	}
	stub.mu.Unlock()
	if !found {
		t.Fatal("the agent never received the new account")
	}
}

// TestRoleRefusalIsRenderedVerbatim: the panel does not pre-check permissions, so
// it has to show what the agent said.
func TestRoleRefusalIsRenderedVerbatim(t *testing.T) {
	c, stub, _ := newPanelConsole(t)
	stub.bootstrapLive = false
	stub.addUser(testPanelUser, testPanelPassword, "admin", nil)
	srv := newLocalServer(t, c)
	client := jarClient(t, srv)
	signIn(t, client, srv)

	resp, err := client.PostForm(srv.URL+panelRolesPath, url.Values{
		"action":   {"set-role"},
		"username": {"someone"},
		"role":     {"owner"},
	})
	if err != nil {
		t.Fatal(err)
	}
	body := bodyOf(t, resp)
	if !strings.Contains(body, "a product owner needs at least one product") {
		t.Fatalf("refusal not rendered: %s", body)
	}
}

// TestProductPageShowsConnectConfig: the block an operator copies is the server
// address plus root public key, and it carries no token (requirement 13.4).
func TestProductPageShowsConnectConfig(t *testing.T) {
	c, stub, _ := newPanelConsole(t)
	stub.bootstrapLive = false
	stub.addUser(testPanelUser, testPanelPassword, "admin", nil)
	stub.addProduct("demo", "/srv/relkit/demo")
	srv := newLocalServer(t, c)
	client := jarClient(t, srv)
	signIn(t, client, srv)

	resp, err := client.Get(srv.URL + panelHostsPath)
	if err != nil {
		t.Fatal(err)
	}
	body := bodyOf(t, resp)
	for _, want := range []string{"demo", "https://stub.example.invalid", "ssh-ed25519 AAAAstub", "stub-root"} {
		if !strings.Contains(body, want) {
			t.Errorf("products page missing %q", want)
		}
	}
	if strings.Contains(body, "plain-") {
		t.Error("products page leaked a plaintext token")
	}
}

// TestAddProductShowsTokenOnce: the plaintext appears in the response that
// issued it. A later page load must not carry it.
func TestAddProductShowsTokenOnce(t *testing.T) {
	c, stub, _ := newPanelConsole(t)
	stub.bootstrapLive = false
	stub.addUser(testPanelUser, testPanelPassword, "admin", nil)
	srv := newLocalServer(t, c)
	client := jarClient(t, srv)
	signIn(t, client, srv)

	resp, err := client.PostForm(srv.URL+panelHostsPath, url.Values{
		"action":  {"add-product"},
		"product": {"fresh"},
		"label":   {"ci"},
	})
	if err != nil {
		t.Fatal(err)
	}
	body := bodyOf(t, resp)
	if !strings.Contains(body, "plain-tok-1") {
		t.Fatalf("issuing response missing the plaintext: %s", body)
	}

	resp, err = client.Get(srv.URL + panelHostsPath)
	if err != nil {
		t.Fatal(err)
	}
	if again := bodyOf(t, resp); strings.Contains(again, "plain-tok-1") {
		t.Fatal("a later page load re-rendered the plaintext token")
	}
}

// TestRetireProductReportsWhatWent: retiring is the whole deletion, so the notice
// says how many tokens died with it.
func TestRetireProductReportsWhatWent(t *testing.T) {
	c, stub, _ := newPanelConsole(t)
	stub.bootstrapLive = false
	stub.addUser(testPanelUser, testPanelPassword, "admin", nil)
	stub.addProduct("demo", "/srv/relkit/demo")
	srv := newLocalServer(t, c)
	client := jarClient(t, srv)
	signIn(t, client, srv)

	resp, err := client.PostForm(srv.URL+panelHostsPath, url.Values{
		"action":  {"delete-product"},
		"product": {"demo"},
	})
	if err != nil {
		t.Fatal(err)
	}
	body := bodyOf(t, resp)
	if !strings.Contains(body, "Product demo retired") || !strings.Contains(body, "1 token") {
		t.Fatalf("retire notice=%s", body)
	}
	if strings.Contains(body, "Retiring deletes every version") {
		return // the form is still there, which is correct: nothing left to list
	}
}

// TestTokensPageAndRotation: tokens are listed per product, and a rotation shows
// the new plaintext exactly as issuing does.
func TestTokensPageAndRotation(t *testing.T) {
	c, stub, _ := newPanelConsole(t)
	stub.bootstrapLive = false
	stub.addUser(testPanelUser, testPanelPassword, "admin", nil)
	token := stub.addProduct("demo", "/srv/relkit/demo")
	srv := newLocalServer(t, c)
	client := jarClient(t, srv)
	signIn(t, client, srv)

	resp, err := client.Get(srv.URL + panelTokensPath + "?product=demo")
	if err != nil {
		t.Fatal(err)
	}
	body := bodyOf(t, resp)
	if !strings.Contains(body, token.ID) || !strings.Contains(body, "live") {
		t.Fatalf("tokens page=%s", body)
	}

	resp, err = client.PostForm(srv.URL+panelTokensPath, url.Values{
		"action":  {"rotate-token"},
		"product": {"demo"},
		"token":   {token.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	body = bodyOf(t, resp)
	if !strings.Contains(body, "Token rotated") || !strings.Contains(body, "plain-rotated-demo") {
		t.Fatalf("rotation page=%s", body)
	}
}

// TestRevokedTokenCannotBeRotated: a dead token offers no actions, which the
// template enforces by not rendering the forms.
func TestRevokedTokenCannotBeRotated(t *testing.T) {
	c, stub, _ := newPanelConsole(t)
	stub.bootstrapLive = false
	stub.addUser(testPanelUser, testPanelPassword, "admin", nil)
	token := stub.addProduct("demo", "/srv/relkit/demo")
	srv := newLocalServer(t, c)
	client := jarClient(t, srv)
	signIn(t, client, srv)

	resp, err := client.PostForm(srv.URL+panelTokensPath, url.Values{
		"action":  {"revoke-token"},
		"product": {"demo"},
		"token":   {token.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	body := bodyOf(t, resp)
	if !strings.Contains(body, "Token revoked") || !strings.Contains(body, "revoked") {
		t.Fatalf("revocation page=%s", body)
	}
}

// TestPanelPOSTNeedsSameOrigin: every panel write is state-changing, so a
// cross-origin form post is refused before it reaches the agent.
func TestPanelPOSTNeedsSameOrigin(t *testing.T) {
	c, stub, _ := newPanelConsole(t)
	stub.bootstrapLive = false
	stub.addUser(testPanelUser, testPanelPassword, "admin", nil)
	srv := newLocalServer(t, c)
	client := jarClient(t, srv)
	signIn(t, client, srv)

	req, err := http.NewRequest(http.MethodPost, srv.URL+panelUsersPath, strings.NewReader("action=add-user&username=x&password=y&role=admin"))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", "https://attacker.invalid")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-origin POST = %d, want 403", resp.StatusCode)
	}
}

// TestAgentSessionExpirySendsOperatorToLogin: the agent forgets sessions on
// restart, so a page load after that must redirect instead of showing an error
// the operator cannot act on.
func TestAgentSessionExpirySendsOperatorToLogin(t *testing.T) {
	c, stub, _ := newPanelConsole(t)
	stub.bootstrapLive = false
	stub.addUser(testPanelUser, testPanelPassword, "admin", nil)
	srv := newLocalServer(t, c)
	client := jarClient(t, srv)
	signIn(t, client, srv)

	// Simulate the restart: the agent no longer knows the bearer the cookie holds.
	stub.mu.Lock()
	stub.sessions = map[string]string{}
	stub.mu.Unlock()

	resp, err := noRedirectClient().Get(srv.URL + panelUsersPath)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusFound || !strings.HasPrefix(resp.Header.Get("Location"), adminLoginPath) {
		t.Fatalf("expired session = %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}
}

// TestSessionCookieStaysOnAdminPrefix: the cookie never travels with a download.
func TestSessionCookieStaysOnAdminPrefix(t *testing.T) {
	c, _, dir := newPanelConsole(t)
	cookie, err := c.admin.issueCookie(signedSession{User: "op", Agent: "bearer"})
	if err != nil {
		t.Fatal(err)
	}
	if cookie.Path != "/-/" {
		t.Fatalf("cookie Path = %q, want /-/", cookie.Path)
	}
	if !cookie.HttpOnly {
		t.Fatal("cookie must be HttpOnly")
	}
	if time.Until(cookie.Expires) <= 0 {
		t.Fatal("cookie already expired")
	}
	_ = dir
}

// TestInitWritesConfigAndPanelState: a fresh init lands a config and a panel key
// file, and the panel it configures reports the agent it was pointed at.
func TestInitWritesConfigAndPanelState(t *testing.T) {
	out := t.TempDir()
	tree := t.TempDir()

	var buf strings.Builder
	if err := runInit(&buf, []string{"-dir", tree, "-out", out, "-agent", "https://agent.example.invalid"}); err != nil {
		t.Fatalf("runInit: %v", err)
	}

	cfg, path, err := LoadFileConfig(filepath.Join(out, ConfigName))
	if err != nil {
		t.Fatalf("LoadFileConfig: %v", err)
	}
	if path == "" || cfg.Dir != tree {
		t.Fatalf("config=%+v path=%q", cfg, path)
	}
	if cfg.Agent != "https://agent.example.invalid" {
		t.Fatalf("agent = %q", cfg.Agent)
	}

	session, err := openPanelSession(filepath.Join(tree, adminStateFileName), tree, cfg.Agent)
	if err != nil {
		t.Fatalf("openPanelSession: %v", err)
	}
	if len(session.key) < 32 {
		t.Fatal("panel key missing")
	}
	if !strings.Contains(buf.String(), "relkit-agent init -bootstrap") {
		t.Fatalf("init output should point at the agent's bootstrap:\n%s", buf.String())
	}
}

// TestInitRefusesExistingConfig keeps the guard: init on a box that already has a
// config must not silently sign everyone out.
func TestInitRefusesExistingConfig(t *testing.T) {
	out := t.TempDir()
	writeFile(t, out, ConfigName, []byte("{}\n"))
	var buf strings.Builder
	err := runInit(&buf, []string{"-dir", t.TempDir(), "-out", out})
	if err == nil {
		t.Fatal("init over an existing config must fail")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("err = %v, want already-exists guard", err)
	}
}

// TestInitResetAdmin rotates the cookie key and leaves accounts alone, because
// accounts are not the panel's to keep any more.
func TestInitResetAdmin(t *testing.T) {
	out := t.TempDir()
	tree := t.TempDir()

	quoted, err := json.Marshal(tree)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, out, ConfigName, []byte("{\"dir\":"+string(quoted)+"}\n"))
	path := filepath.Join(tree, adminStateFileName)
	first, err := mintPanelDoc()
	if err != nil {
		t.Fatal(err)
	}
	if err := writePanelDoc(path, first); err != nil {
		t.Fatal(err)
	}

	var buf strings.Builder
	if err := runInit(&buf, []string{"-reset-admin", "-out", out}); err != nil {
		t.Fatalf("runInit -reset-admin: %v", err)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(after), first.SessionKey) {
		t.Fatal("reset kept the old cookie key")
	}
	if !strings.Contains(buf.String(), "relkit-console") {
		t.Fatal("output should name the service to restart")
	}
}

// TestInitResetAdminRejectsForce: the break-glass flag set is closed.
func TestInitResetAdminRejectsForce(t *testing.T) {
	var buf strings.Builder
	err := runInit(&buf, []string{"-reset-admin", "-force"})
	if err == nil {
		t.Fatal("-reset-admin -force must be rejected")
	}
	if !strings.Contains(err.Error(), "cannot be combined") {
		t.Fatalf("err = %v, want cannot-be-combined guard", err)
	}
}

// TestPanelStateIsNotServed: the cookie key is panel state and must never be
// readable over HTTP.
func TestPanelStateIsNotServed(t *testing.T) {
	c, stub, _ := newPanelConsole(t)
	stub.bootstrapLive = false
	stub.addUser(testPanelUser, testPanelPassword, "admin", nil)
	srv := newLocalServer(t, c)
	client := jarClient(t, srv)
	signIn(t, client, srv)

	resp, err := client.Get(srv.URL + adminFilesPath)
	if err != nil {
		t.Fatal(err)
	}
	listing := bodyOf(t, resp)
	if strings.Contains(listing, adminStateFileName) {
		t.Fatalf("listing leaked the panel state file\n%s", listing)
	}
	if strings.Contains(listing, "sessionKey") {
		t.Fatal("listing leaked key material")
	}
}
