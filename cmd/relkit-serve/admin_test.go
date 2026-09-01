package main

import (
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

func TestPublicTreeStaysAnonymous(t *testing.T) {
	cfg, dir := newTestConfig(t, false)
	_, admin := bootstrapAdmin(t, dir)
	cfg.admin = admin
	srv := newLocalServer(t, cfg)
	t.Cleanup(srv.Close)
	writeFile(t, dir, "artifact/app/1.0.0/app.zip", []byte("pkg"))

	resp, err := noRedirectClient().Get(srv.URL + "/artifact/app/1.0.0/app.zip")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || string(body) != "pkg" {
		t.Fatalf("public GET status=%d body=%q", resp.StatusCode, body)
	}

	resp, err = noRedirectClient().Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET / status = %d, want 200", resp.StatusCode)
	}
}

func TestFirstOperatorConsumesBootstrap(t *testing.T) {
	cfg, dir := newTestConfig(t, false)
	token, admin := bootstrapAdmin(t, dir)
	cfg.admin = admin
	srv := newLocalServer(t, cfg)
	t.Cleanup(srv.Close)
	client := jarClient(t, srv)

	resp, err := client.PostForm(srv.URL+adminSetupPath, url.Values{
		"bootstrap": {token},
		"username":  {"alice"},
		"password":  {"long-enough"},
		"password2": {"long-enough"},
	})
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("setup status = %d body=%s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), "alice") {
		t.Fatalf("expected signed-in panel, got:\n%s", body)
	}

	raw, err := os.ReadFile(admin.path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), token) {
		t.Fatal("plaintext bootstrap must not be written to disk")
	}
	var doc adminDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.BootstrapHash != "" {
		t.Fatalf("bootstrap hash still present after setup: %s", doc.BootstrapHash)
	}
	if len(doc.Users) != 1 || doc.Users[0].Username != "alice" {
		t.Fatalf("users = %+v", doc.Users)
	}

	resp, err = noRedirectClient().PostForm(srv.URL+adminSetupPath, url.Values{
		"bootstrap": {token},
		"username":  {"bob"},
		"password":  {"long-enough"},
		"password2": {"long-enough"},
	})
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("second setup status = %d, want 302 to login", resp.StatusCode)
	}
}

func TestWrongBootstrapDoesNotCreateAccount(t *testing.T) {
	cfg, dir := newTestConfig(t, false)
	_, admin := bootstrapAdmin(t, dir)
	cfg.admin = admin
	srv := newLocalServer(t, cfg)
	t.Cleanup(srv.Close)

	resp, err := noRedirectClient().PostForm(srv.URL+adminSetupPath, url.Values{
		"bootstrap": {"not-the-token"},
		"username":  {"alice"},
		"password":  {"long-enough"},
		"password2": {"long-enough"},
	})
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if !strings.Contains(string(body), "Could not create the account.") {
		t.Fatalf("body=%s", body)
	}
	if admin.gate() != gateSetup {
		t.Fatalf("gate = %d, want setup still live", admin.gate())
	}
}

func TestLoginAndLogout(t *testing.T) {
	srv, _ := newTestServer(t, false)
	client := jarClient(t, srv)

	resp, err := noRedirectClient().Get(srv.URL + "/-/admin")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("unauthenticated GET status = %d, want 302", resp.StatusCode)
	}

	resp, err = client.PostForm(srv.URL+adminLoginPath, url.Values{
		"username": {testPanelUser},
		"password": {testPanelPassword},
	})
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), testPanelUser) {
		t.Fatalf("login status=%d body=%s", resp.StatusCode, body)
	}

	resp, err = client.Get(srv.URL + adminLogoutPath)
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(body), "Sign in") {
		t.Fatalf("logout did not reach login page:\n%s", body)
	}
}

func TestInitWritesBootstrapHashNotPlaintext(t *testing.T) {
	dir := t.TempDir()
	serve := filepath.Join(dir, "releases")
	if err := os.MkdirAll(serve, 0o755); err != nil {
		t.Fatal(err)
	}
	var buf strings.Builder
	if err := runInit(&buf, []string{"-dir", serve, "-out", dir}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "RELKIT_ADMIN_BOOTSTRAP=") {
		t.Fatalf("init did not print the bootstrap:\n%s", out)
	}
	if !strings.Contains(out, "RELKIT_UPLOAD_TOKEN=") {
		t.Fatalf("init did not print the upload token:\n%s", out)
	}
	start := strings.Index(out, "RELKIT_ADMIN_BOOTSTRAP='")
	rest := out[start+len("RELKIT_ADMIN_BOOTSTRAP='"):]
	end := strings.IndexByte(rest, '\'')
	token := rest[:end]
	if len(token) < 40 {
		t.Fatalf("bootstrap too short: %q", token)
	}

	adminPath := filepath.Join(serve, adminStateFileName)
	raw, err := os.ReadFile(adminPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), token) {
		t.Fatal("admin state contains the bootstrap plaintext")
	}
	var doc adminDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.BootstrapHash == "" || len(doc.Users) != 0 {
		t.Fatalf("admin doc = %+v", doc)
	}

	cfg, _, err := LoadFileConfig(filepath.Join(dir, ConfigName))
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := openAdminAuth(resolveAdminPath(serve, filepath.Join(dir, ConfigName), cfg.AdminStateFile), serve)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.gate() != gateSetup {
		t.Fatalf("gate = %d, want setup", loaded.gate())
	}
}

func TestInitTokenOnlyLeavesAdminAlone(t *testing.T) {
	dir := t.TempDir()
	serve := filepath.Join(dir, "releases")
	if err := os.MkdirAll(serve, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := runInit(io.Discard, []string{"-dir", serve, "-out", dir}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(serve, adminStateFileName))
	if err != nil {
		t.Fatal(err)
	}
	if err := runInit(io.Discard, []string{"-out", dir, "-token-only"}); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(filepath.Join(serve, adminStateFileName))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("-token-only must not rotate the panel bootstrap")
	}
}

func TestInitResetAdminIssuesNewBootstrap(t *testing.T) {
	dir := t.TempDir()
	serve := filepath.Join(dir, "releases")
	if err := os.MkdirAll(serve, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := runInit(io.Discard, []string{"-dir", serve, "-out", dir}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(serve, adminStateFileName)
	hash, err := hashPassword("old-password")
	if err != nil {
		t.Fatal(err)
	}
	if err := writeAdminDoc(path, adminDoc{
		SessionKey: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		Users:      []adminUser{{Username: "old", PasswordHash: hash}},
	}); err != nil {
		t.Fatal(err)
	}

	var buf strings.Builder
	if err := runInit(&buf, []string{"-out", dir, "-reset-admin"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "RELKIT_ADMIN_BOOTSTRAP=") {
		t.Fatalf("reset did not print a bootstrap:\n%s", buf.String())
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc adminDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Users) != 0 || doc.BootstrapHash == "" {
		t.Fatalf("reset admin doc = %+v", doc)
	}
}

func TestAdminStateIsNotServed(t *testing.T) {
	srv, dir := newTestServer(t, true)
	resp := testGet(t, srv.URL+"/"+adminStateFileName)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("GET admin state status = %d, want 404", resp.StatusCode)
	}
	listing := getBody(t, srv.URL+"/-/admin/files")
	if strings.Contains(listing, adminStateFileName) {
		t.Fatalf("listing leaked admin state:\n%s", listing)
	}

	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/"+adminStateFileName, strings.NewReader("{}"))
	req.Header.Set("Authorization", "Bearer "+testToken)
	put, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT admin state: %v", err)
	}
	put.Body.Close()
	if put.StatusCode != http.StatusBadRequest {
		t.Fatalf("PUT admin state status = %d, want 400", put.StatusCode)
	}
	_ = dir
}

func TestSessionCookieStaysOnAdminPrefix(t *testing.T) {
	_, admin := bootstrapAdmin(t, t.TempDir())
	cookie, err := admin.issueCookie("alice", time.Now().Add(time.Hour))
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
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if !strings.Contains(string(body), "Panel locked") {
		t.Fatalf("body=%s", body)
	}
}
