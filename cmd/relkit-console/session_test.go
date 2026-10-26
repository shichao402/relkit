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
	state := filepath.Join(t.TempDir(), "panel.json")

	var buf strings.Builder
	if err := runInit(&buf, []string{"-store", "http://127.0.0.1:8080", "-state", state, "-out", out, "-agent", "https://agent.example.invalid"}); err != nil {
		t.Fatalf("runInit: %v", err)
	}

	cfg, path, err := LoadFileConfig(filepath.Join(out, ConfigName))
	if err != nil {
		t.Fatalf("LoadFileConfig: %v", err)
	}
	if path == "" || cfg.Store != "http://127.0.0.1:8080" || cfg.AdminStateFile != state {
		t.Fatalf("config=%+v path=%q", cfg, path)
	}
	if cfg.Agent != "https://agent.example.invalid" {
		t.Fatalf("agent = %q", cfg.Agent)
	}

	session, err := openPanelSession(state, cfg.Agent)
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

// TestInitRequiresStore: the store is the only way the panel reads the tree,
// so a config without one is not a config.
func TestInitRequiresStore(t *testing.T) {
	var buf strings.Builder
	err := runInit(&buf, []string{"-out", t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "-store is required") {
		t.Fatalf("err = %v, want -store is required", err)
	}
}

// TestPanelStateMustBeAbsolute: a relative state path would land wherever the
// service happened to start, which is how panel keys ended up in the tree.
func TestPanelStateMustBeAbsolute(t *testing.T) {
	if _, err := resolveAdminPath("panel.json"); err == nil {
		t.Fatal("relative adminStateFile must be rejected")
	}
	got, err := resolveAdminPath("")
	if err != nil || got != defaultPanelStatePath {
		t.Fatalf("default = %q err=%v", got, err)
	}
}

// TestInitRefusesExistingConfig keeps the guard: init on a box that already has a
// config must not silently sign everyone out.
func TestInitRefusesExistingConfig(t *testing.T) {
	out := t.TempDir()
	writeFile(t, out, ConfigName, []byte("{}\n"))
	var buf strings.Builder
	err := runInit(&buf, []string{"-store", "http://127.0.0.1:8080", "-state", filepath.Join(t.TempDir(), "panel.json"), "-out", out})
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
	path := filepath.Join(t.TempDir(), "panel.json")

	quoted, err := json.Marshal(path)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, out, ConfigName, []byte("{\"store\":\"http://127.0.0.1:8080\",\"adminStateFile\":"+string(quoted)+"}\n"))
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

// TestLeftoverStateIsNotListed: the panel state no longer lives in the tree,
// but a box migrated from the old layout can still have the files there. The
// store hides its own names; the console hides the panel's, so neither shows up
// in the file tree even behind a store that does not know them.
func TestLeftoverStateIsNotListed(t *testing.T) {
	api, _, dir := newAPITestDir(t)
	writeFile(t, dir, adminStateFileName, []byte(`{"sessionKey":"leaked"}`))
	writeFile(t, dir, "admin.json", []byte(`{"sessionKey":"leaked"}`))
	writeFile(t, dir, "artifact/app/1.0.0/app.zip", []byte("pkg"))
	api.login()

	status, body := api.call(http.MethodGet, apiPrefix+"files", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("files = %d %v", status, body)
	}
	dump := fmtJSON(body)
	if !strings.Contains(dump, "artifact") {
		t.Fatalf("listing should still show the tree: %s", dump)
	}
	if strings.Contains(dump, adminStateFileName) || strings.Contains(dump, "admin.json") {
		t.Fatalf("listing leaked a panel state file: %s", dump)
	}
	if strings.Contains(dump, "sessionKey") {
		t.Fatal("listing leaked key material")
	}
}
