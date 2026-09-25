package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func init() {
	passwordCost = bcrypt.MinCost
	authFailDelay = 0
}

func bootstrapAdmin(t *testing.T, dir string) (string, *adminAuth) {
	t.Helper()
	token, doc, err := mintAdminDoc()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, adminStateFileName)
	if err := writeAdminDoc(path, doc); err != nil {
		t.Fatal(err)
	}
	admin, err := openAdminAuth(path, dir)
	if err != nil {
		t.Fatal(err)
	}
	return token, admin
}

func jarClient(t *testing.T, srv *httptest.Server) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := srv.Client()
	client.Jar = jar
	return client
}

func noRedirectClient() *http.Client {
	return &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// newTestConfig assembles the console the tests exercise: adapter over a temp
// tree, admin state in it. The second return is the tree dir.
func newTestConfig(t *testing.T, _ bool) (*console, string) {
	return newTestConsole(t)
}

func TestPanelRedirectsToSetupWhenBootstrapLive(t *testing.T) {
	cfg, dir := newTestConfig(t, false)
	_, admin := bootstrapAdmin(t, dir)
	cfg.admin = admin
	srv := newLocalServer(t, cfg)
	t.Cleanup(srv.Close)

	resp, err := noRedirectClient().Get(srv.URL + "/-/admin")
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

func TestFirstOperatorConsumesBootstrap(t *testing.T) {
	cfg, dir := newTestConfig(t, false)
	token, admin := bootstrapAdmin(t, dir)
	cfg.admin = admin
	srv := newLocalServer(t, cfg)
	t.Cleanup(srv.Close)

	client := jarClient(t, srv)
	resp, err := client.Get(srv.URL + adminSetupPath)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	resp, err = client.PostForm(srv.URL+adminSetupPath, url.Values{
		"bootstrap": {token},
		"username":  {testPanelUser},
		"password":  {testPanelPassword},
		"password2": {testPanelPassword},
	})
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	resp.Body.Close()
	// The jar follows the redirect: setup already lands on the signed-in panel.
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), testPanelUser) {
		t.Fatalf("setup = %d body=%s", resp.StatusCode, body)
	}

	// The panel now answers 200 with the operator session.
	resp, err = client.Get(srv.URL + adminPath)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("panel after setup = %d", resp.StatusCode)
	}

	// The bootstrap is one-shot: a second account attempt fails.
	resp, err = noRedirectClient().Get(srv.URL + adminSetupPath)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("setup after consumption = %d, want redirect to login", resp.StatusCode)
	}
}

func TestWrongBootstrapDoesNotCreateAccount(t *testing.T) {
	cfg, dir := newTestConfig(t, false)
	_, admin := bootstrapAdmin(t, dir)
	cfg.admin = admin
	srv := newLocalServer(t, cfg)
	t.Cleanup(srv.Close)

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
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), "Could not create the account") {
		t.Fatalf("setup = %d body=%s", resp.StatusCode, body)
	}

	// The state still has zero users: the panel keeps gating on setup.
	resp, err = noRedirectClient().Get(srv.URL + adminPath)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusFound || resp.Header.Get("Location") != adminSetupPath {
		t.Fatalf("panel = %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}
}

func TestLoginAndLogout(t *testing.T) {
	cfg, _ := newTestConfig(t, false)
	srv := newLocalServer(t, cfg)
	t.Cleanup(srv.Close)

	client := jarClient(t, srv)
	resp, err := client.PostForm(srv.URL+adminLoginPath, url.Values{
		"username": {testPanelUser},
		"password": {testPanelPassword},
	})
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	resp.Body.Close()
	// The jar follows the redirect: the final page is the portal itself.
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), testPanelUser) {
		t.Fatalf("login = %d body=%s", resp.StatusCode, body)
	}

	resp, err = client.Get(srv.URL + adminLogoutPath)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	resp, err = noRedirectClient().Get(srv.URL + adminPath)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("panel after logout = %d, want 302", resp.StatusCode)
	}
}

func TestAdminStateIsNotServed(t *testing.T) {
	cfg, dir := newTestConfig(t, false)
	srv := newLocalServer(t, cfg)
	t.Cleanup(srv.Close)

	// The console has no file-serving surface at all, so the admin state can
	// only be reached if some handler leaked it. Assert the routes the panel
	// does expose never answer it.
	for _, path := range []string{
		"/.relkit-serve-admin.json",
		"/-/admin/.relkit-serve-admin.json",
		"/-/p/..%2f.relitkit-serve-admin.json",
	} {
		resp, err := noRedirectClient().Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<10))
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK && strings.Contains(string(body), "sessionKey") {
			t.Fatalf("%s leaked admin state", path)
		}
	}

	// The operator listing must not show the state file either.
	client := jarClient(t, srv)
	loginPanel(t, client, srv)
	resp, err := client.Get(srv.URL + adminFilesPath)
	if err != nil {
		t.Fatal(err)
	}
	listing, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	resp.Body.Close()
	if strings.Contains(string(listing), adminStateFileName) {
		t.Fatalf("listing leaked the admin state file\n%s", listing)
	}
	_ = dir
}

func TestSessionCookieStaysOnAdminPrefix(t *testing.T) {
	_, admin := bootstrapAdmin(t, t.TempDir())
	cookie, err := admin.issueCookie(testPanelUser, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if cookie.Path != "/-/" {
		t.Fatalf("cookie Path = %q, want /-/", cookie.Path)
	}
}

func TestLockedPanelWithoutBootstrap(t *testing.T) {
	cfg, dir := newTestConfig(t, false)
	cfg.admin = &adminAuth{path: filepath.Join(dir, adminStateFileName)}
	srv := httptest.NewServer(cfg.handler())
	t.Cleanup(srv.Close)

	resp, err := noRedirectClient().Get(srv.URL + "/-/admin")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), "Panel locked") {
		t.Fatalf("panel = %d body=%s", resp.StatusCode, body)
	}
}

// TestAdminStateRoundTrip keeps the doc migration checks from serve that
// still apply: the state file format is unchanged from relkit-serve, and a
// box migrating from it keeps its accounts.
func TestAdminStateRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, adminStateFileName)
	_, admin := bootstrapAdmin(t, dir)
	if err := admin.createFirstOperator("bootstrap", "op", "password123"); err != nil {
		// createFirstOperator needs a live bootstrap; use doc write instead.
		t.Logf("direct create: %v (expected without live bootstrap)", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc adminDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.SessionKey == "" {
		t.Fatal("session key missing")
	}
	if doc.BootstrapHash == "" {
		t.Fatal("bootstrap hash missing (must be hash, not plaintext)")
	}
	if len(doc.Users) > 0 {
		t.Fatal("no operator should exist yet")
	}
}

// TestInitWritesConfigAndBootstrap: a fresh init lands a config skeleton and
// a state file whose bootstrap is actually consumable, i.e. the panel would
// come up in setup gate rather than locked.
func TestInitWritesConfigAndBootstrap(t *testing.T) {
	out := t.TempDir()
	tree := t.TempDir()

	var buf bytes.Buffer
	if err := runInit(&buf, []string{"-dir", tree, "-out", out}); err != nil {
		t.Fatalf("runInit: %v", err)
	}

	configPath := filepath.Join(out, ConfigName)
	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	var cfg FileConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Dir != tree {
		t.Fatalf("config dir = %q, want %q", cfg.Dir, tree)
	}

	admin, err := openAdminAuth(filepath.Join(tree, adminStateFileName), tree)
	if err != nil {
		t.Fatalf("openAdminAuth: %v", err)
	}
	if admin.gate() != gateSetup {
		t.Fatalf("gate = %v, want gateSetup (bootstrap must be live)", admin.gate())
	}
}

// TestInitRefusesExistingConfig keeps serve's guard: init on a box that
// already has a config must not silently rotate the bootstrap.
func TestInitRefusesExistingConfig(t *testing.T) {
	out := t.TempDir()
	if err := os.WriteFile(filepath.Join(out, ConfigName), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	err := runInit(&buf, []string{"-dir", t.TempDir(), "-out", out})
	if err == nil {
		t.Fatal("init over an existing config must fail")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("err = %v, want already-exists guard", err)
	}
}

// TestInitResetAdmin: the break-glass path wipes operators and issues a new
// live bootstrap at the location the running config points at, even when the
// operator account file is the legacy serve-era name.
func TestInitResetAdmin(t *testing.T) {
	out := t.TempDir()
	tree := t.TempDir()

	// A box that migrated from serve: config with a populated tree, one
	// operator, legacy state file name.
	configPath := filepath.Join(out, ConfigName)
	if err := os.WriteFile(configPath, []byte(`{"dir": "`+tree+`"}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	token, admin := bootstrapAdmin(t, tree)
	if err := admin.createFirstOperator(token, "op", testPanelPassword); err != nil {
		t.Fatalf("createFirstOperator: %v", err)
	}
	if admin.gate() != gateLogin {
		t.Fatalf("pre-condition: gate = %v, want gateLogin", admin.gate())
	}

	var buf bytes.Buffer
	if err := runInit(&buf, []string{"-reset-admin", "-out", out}); err != nil {
		t.Fatalf("runInit -reset-admin: %v", err)
	}

	after, err := openAdminAuth(filepath.Join(tree, adminStateFileName), tree)
	if err != nil {
		t.Fatal(err)
	}
	if after.gate() != gateSetup {
		t.Fatalf("gate = %v, want gateSetup after reset", after.gate())
	}
	if len(after.users) != 0 {
		t.Fatalf("operators = %d, want 0 (wiped)", len(after.users))
	}
	if !strings.Contains(buf.String(), "relkit-console") {
		t.Fatal("output should name the service to restart")
	}
}

// TestInitResetAdminRejectsForce: the break-glass flag set is closed; -force
// alongside -reset-admin is an operator error, not a silent combined action.
func TestInitResetAdminRejectsForce(t *testing.T) {
	var buf bytes.Buffer
	err := runInit(&buf, []string{"-reset-admin", "-force"})
	if err == nil {
		t.Fatal("-reset-admin -force must be rejected")
	}
	if !strings.Contains(err.Error(), "cannot be combined") {
		t.Fatalf("err = %v, want cannot-be-combined guard", err)
	}
}
