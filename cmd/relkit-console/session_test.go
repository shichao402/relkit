package main

// Console session and bootstrap tests.
//
// The presentation tests that used to live here drove the server-rendered forms
// (POST the login form, read the panel HTML back). Those pages are gone: sign-in
// is a JSON API call now, and the JSON shape is covered by api_test.go. What
// remains here is the part that is neither API nor page: the cookie itself, and
// the `init` command that mints the panel's signing key.

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func init() {
	// Every auth refusal sleeps to blunt guessing; tests do not need to.
	authFailDelay = 0
}

// TestSessionCookieStaysOnAdminPrefix: the cookie never travels with a download.
func TestSessionCookieStaysOnAdminPrefix(t *testing.T) {
	c, _ := newTestConsole(t)
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
// readable over HTTP. The file tree moved to the JSON API, so this asks the API
// rather than the old listing page.
func TestPanelStateIsNotServed(t *testing.T) {
	api, _, dir := newAPITestDir(t)
	writeFile(t, dir, adminStateFileName, []byte(`{"sessionKey":"leaked"}`))
	api.login()

	status, body := api.call(http.MethodGet, apiPrefix+"files", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("files = %d %v", status, body)
	}
	dump := fmtJSON(body)
	if strings.Contains(dump, adminStateFileName) {
		t.Fatalf("listing leaked the panel state file: %s", dump)
	}
	if strings.Contains(dump, "sessionKey") {
		t.Fatal("listing leaked key material")
	}
}
