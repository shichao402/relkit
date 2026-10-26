package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/shichao402/relkit/internal/webmeta"
)

const (
	testPanelUser     = "op"
	testPanelPassword = "password123"
)

// newTestConsole assembles a console over a temp release tree plus a stub
// agent holding one operator, the test counterpart of main(): the tree is
// served by a fake store and read through storeAdapter, exactly the path
// production takes, and the panel state lives outside the tree. The agent is a
// stub because the panel has no accounts of its own any more (requirement
// 13.6); a test that needs a specific agent answer builds its own with
// newPanelConsole.
func newTestConsole(t *testing.T) (*console, string) {
	t.Helper()
	dir := t.TempDir()
	adapter := newTreeAdapter(t, dir)
	stub, agentURL := newAgentStubServer(t)
	stub.bootstrapLive = false
	stub.addUser(testPanelUser, testPanelPassword, "admin", nil)
	admin, err := openPanelSession(filepath.Join(t.TempDir(), "panel.json"), agentURL)
	if err != nil {
		t.Fatalf("openPanelSession: %v", err)
	}
	c := &console{
		adapter: adapter,
		admin:   admin,
	}
	return c, dir
}

// newTreeAdapter serves dir through a fake store and returns the adapter the
// console would build for it. There is no other adapter to build.
func newTreeAdapter(t *testing.T, dir string) Adapter {
	t.Helper()
	srv := newFakeStoreServer(t, dir)
	adapter, err := newStoreAdapter("store:test", srv.URL)
	if err != nil {
		t.Fatalf("newStoreAdapter: %v", err)
	}
	return adapter
}

// readyTestAdmin is the serve version carried over: one user, deterministic
// session key, so requirePanelAuth passes after login.
func readyTestAdmin(t *testing.T, dir string) *panelSession {
	t.Helper()
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 1)
	}
	path := filepath.Join(dir, "panel.json")
	doc := panelDoc{SessionKey: base64.RawURLEncoding.EncodeToString(key)}
	if err := writePanelDoc(path, doc); err != nil {
		t.Fatal(err)
	}
	admin, err := openPanelSession(path, "")
	if err != nil {
		t.Fatal(err)
	}
	return admin
}

func newLocalServer(t *testing.T, c *console) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(c.handler())
	t.Cleanup(srv.Close)
	return srv
}

// loginPanel signs in through the JSON API, which is the only sign-in path the
// console has now: the old form endpoint answers a redirect.
func loginPanel(t *testing.T, client *http.Client, srv *httptest.Server) {
	t.Helper()
	body, err := json.Marshal(map[string]string{
		"username": testPanelUser,
		"password": testPanelPassword,
	})
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, srv.URL+apiPrefix+"session/login", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", srv.URL)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	answer := bodyText(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login = %d body=%s", resp.StatusCode, answer)
	}
	var logged struct {
		Gate string `json:"gate"`
	}
	if err := json.Unmarshal([]byte(answer), &logged); err != nil || logged.Gate != "ready" {
		t.Fatalf("login = %d body=%s", resp.StatusCode, answer)
	}
}

// bodyText reads a response body and closes it.
func bodyText(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func writeFile(t *testing.T, dir, name string, body []byte) {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestConsoleHealth(t *testing.T) {
	c, _ := newTestConsole(t)
	srv := newLocalServer(t, c)
	resp, err := http.Get(srv.URL + "/-/health")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("health = %d", resp.StatusCode)
	}
}

func TestPanelRedirectsAnonymousToLogin(t *testing.T) {
	c, _ := newTestConsole(t)
	srv := newLocalServer(t, c)
	resp, err := noRedirectClient().Get(srv.URL + adminPath)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	// The old portal root is a signpost now: the app shell gates itself through
	// the session API, so this answers a redirect rather than a page.
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("panel = %d", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != appPrefix+"products" {
		t.Fatalf("Location = %q", loc)
	}
}

// TestLoginServesTheApp: after signing in the rebuilt front end is what answers,
// and the file tree moved from a server-rendered listing to the JSON API.
func TestLoginServesTheApp(t *testing.T) {
	c, dir := newTestConsole(t)
	writeFile(t, dir, "artifact/app/1.0.0/app.zip", []byte("pkg"))
	srv := newLocalServer(t, c)
	client := jarClient(t, srv)
	loginPanel(t, client, srv)

	// /-/admin/files kept its path: it is a route in the app now, so it answers
	// the shell directly instead of redirecting anywhere.
	resp, err := client.Get(srv.URL + adminFilesPath)
	if err != nil {
		t.Fatal(err)
	}
	body := bodyText(t, resp)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, `<div id="root">`) {
		t.Fatalf("files = %d body=%s", resp.StatusCode, body)
	}

	// The old panel tabs are gone, so they redirect into the routes that
	// replaced them.
	resp, err = noRedirectClient().Get(srv.URL + panelUsersPath)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusFound || resp.Header.Get("Location") != appPrefix+"users" {
		t.Fatalf("users tab = %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}
}

// fakeStore is a hand-rolled stand-in for relkit-store inside the console
// package's tests: main packages cannot be imported, and the adapter's
// contract is the wire (GET tree + /-/list/ JSON), which this fake mirrors
// including the server-side hidden-key filtering.
type fakeStore struct {
	root *os.Root
}

func (f fakeStore) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/-/list/") {
		f.serveList(w, r)
		return
	}
	body, err := f.root.ReadFile(strings.TrimPrefix(path.Clean(r.URL.Path), "/"))
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	_, _ = w.Write(body)
}

func (f fakeStore) serveList(w http.ResponseWriter, r *http.Request) {
	dir := strings.Trim(strings.TrimPrefix(r.URL.Path, "/-/list/"), "/")
	if dir == "" {
		dir = "."
	}
	if dir != path.Clean(dir) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	dirEntries, err := fs.ReadDir(f.root.FS(), dir)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	// Mirror the store's row shape, including its hidden-key filtering: the
	// console test tree plants reserved files that must never come back.
	hidden := func(name string) bool {
		return strings.HasPrefix(name, ".relkit-serve-")
	}
	rows := make([]storeEntryJSON, 0, len(dirEntries))
	for _, entry := range dirEntries {
		if hidden(entry.Name()) {
			continue
		}
		row := storeEntryJSON{Name: entry.Name(), IsDir: entry.IsDir()}
		if info, err := entry.Info(); err == nil {
			row.Size = info.Size()
			row.Mtime = info.ModTime().UTC().Format(time.RFC3339)
		}
		rows = append(rows, row)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(rows)
}

func newFakeStoreServer(t *testing.T, dir string) *httptest.Server {
	t.Helper()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatalf("OpenRoot: %v", err)
	}
	t.Cleanup(func() { root.Close() })
	srv := httptest.NewServer(fakeStore{root: root})
	t.Cleanup(srv.Close)
	return srv
}

func TestStoreAdapterReadsRemoteTree(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "index/loom/dev.pb", []byte("x"))
	writeFile(t, dir, "artifact/loom/1.0.0/loom.zip", []byte("pkg"))
	writeFile(t, dir, ".relkit-serve-admin.json", []byte(`{}`))

	srv := newFakeStoreServer(t, dir)
	a, err := newStoreAdapter("store:test", srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if a.Name() != "store:test" {
		t.Fatalf("name = %q", a.Name())
	}

	// ReadKey: a 404 is an error, never an empty body.
	raw, err := a.ReadKey("index/loom/dev.pb")
	if err != nil || string(raw) != "x" {
		t.Fatalf("read=%q err=%v", raw, err)
	}
	if _, err := a.ReadKey("missing/key"); err == nil {
		t.Fatal("missing key must be an error, not nil")
	}

	// ReadDir: names, isDir, sizes come from the listing payload.
	entries, err := a.ReadDir("index")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name != "loom" || !entries[0].IsDir {
		t.Fatalf("entries=%+v", entries)
	}
	rootEntries, err := a.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	if len(rootEntries) != 2 {
		t.Fatalf("root entries=%+v", rootEntries)
	}

	// ModTime comes from the listing payload.
	if a.ModTime("index/loom/dev.pb").IsZero() {
		t.Fatal("modtime missing")
	}
	if !a.ModTime("missing").IsZero() {
		t.Fatal("missing key should be zero time")
	}

	// ReadDir on a missing directory is an error, mirroring fs.ReadDir.
	if _, err := a.ReadDir("nowhere"); err == nil {
		t.Fatal("missing dir must be an error")
	}
}

func TestStoreAdapterRejectsBadURLs(t *testing.T) {
	for _, raw := range []string{"", "ftp://example.com", "http://", "not a url"} {
		if _, err := newStoreAdapter("store:test", raw); err == nil {
			t.Errorf("newStoreAdapter(%q) should fail", raw)
		}
	}
	if _, err := newStoreAdapter("store:test", "http://127.0.0.1:9999"); err != nil {
		t.Fatalf("valid URL rejected: %v", err)
	}
}

// TestLegacyPanelURLsRedirectToTheirReplacement: the old tab strip is gone from
// the templates but stays valid as URLs (requirement 7.3), because operators
// bookmarked it and release notes link to product pages by the old path.
func TestLegacyPanelURLsRedirectToTheirReplacement(t *testing.T) {
	c, dir := newTestConsole(t)
	writeRelease(t, dir, "app", "stable", "1.0.0", 1)
	srv := newLocalServer(t, c)

	for _, tc := range []struct{ from, to string }{
		{panelPath, "/-/admin/products"},
		{panelUsersPath, "/-/admin/users"},
		{panelRolesPath, "/-/admin/roles"},
		{panelTokensPath, "/-/admin/tokens"},
		{panelHostsPath, "/-/admin/products"},
		{panelTokensPath + "?product=app", "/-/admin/products/app/tokens"},
		{"/-/p/app", "/-/admin/products/app"},
		{adminLoginPath, "/-/admin/sign-in"},
		{adminSetupPath, "/-/admin/"},
	} {
		resp, err := noRedirectClient().Get(srv.URL + tc.from)
		if err != nil {
			t.Fatalf("GET %s: %v", tc.from, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusFound {
			t.Errorf("GET %s = %d, want 302", tc.from, resp.StatusCode)
			continue
		}
		if loc := resp.Header.Get("Location"); loc != tc.to {
			t.Errorf("GET %s Location = %q, want %q", tc.from, loc, tc.to)
		}
		// A cached redirect would outlive a change to the route table.
		if got := resp.Header.Get("Cache-Control"); got != "no-store" {
			t.Errorf("GET %s Cache-Control = %q, want no-store", tc.from, got)
		}
	}
}

// TestLegacyProductURLRejectsJunk: a redirect to a page that cannot exist is
// worse than a plain 404, so the old product path validates before it moves.
func TestLegacyProductURLRejectsJunk(t *testing.T) {
	c, dir := newTestConsole(t)
	writeRelease(t, dir, "app", "stable", "1.0.0", 1)
	srv := newLocalServer(t, c)

	for _, path := range []string{"/-/p/", "/-/p/app/extra", "/-/p/..%2f..", "/-/p/.hidden"} {
		resp, err := noRedirectClient().Get(srv.URL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", path, resp.StatusCode)
		}
	}
}

// TestLegacyLogoutStillSignsOut: the one old URL with a side effect. A bookmark
// to it must clear the cookie rather than just point at the sign-in page.
func TestLegacyLogoutStillSignsOut(t *testing.T) {
	c, dir := newTestConsole(t)
	writeRelease(t, dir, "app", "stable", "1.0.0", 1)
	srv := newLocalServer(t, c)
	client := jarClient(t, srv)
	loginPanel(t, client, srv)

	// Read the redirect itself rather than the page it points at: the point of
	// the test is that the old URL still answers a 302 and clears the cookie.
	client.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	resp, err := client.Get(srv.URL + adminLogoutPath)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusFound || resp.Header.Get("Location") != appPrefix+"sign-in" {
		t.Fatalf("logout = %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}
	client.CheckRedirect = nil

	// The session is really gone, not merely redirected around.
	resp, err = client.Get(srv.URL + apiPrefix + "session")
	if err != nil {
		t.Fatal(err)
	}
	var view struct {
		Gate string `json:"gate"`
	}
	if err := json.Unmarshal([]byte(bodyText(t, resp)), &view); err != nil {
		t.Fatal(err)
	}
	if view.Gate != "login" {
		t.Fatalf("gate after logout = %q, want login", view.Gate)
	}
}

func TestConsoleServesLatestRedirectFromPointer(t *testing.T) {
	c, dir := newTestConsole(t)
	latest, err := webmeta.MarshalLatest(webmeta.Latest{
		Product:     "app",
		Channel:     "stable",
		Version:     "1.0.0",
		Code:        1,
		PublishedAt: "2026-09-16T00:00:00Z",
		Artifacts: []webmeta.Artifact{
			{ID: "app", Filename: "app.zip", Size: 3, Kind: "archive", URLs: []string{"https://example.com/artifact/app/1.0.0/app.zip"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, "latest/app/stable.json", latest)
	srv := newLocalServer(t, c)
	resp, err := noRedirectClient().Get(srv.URL + "/-/latest/app/stable/app")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("latest = %d", resp.StatusCode)
	}
	// downloadHref prefers the local key over the published absolute URL,
	// same rule as serve: the console box is the origin for its own tree.
	if loc := resp.Header.Get("Location"); loc != "/artifact/app/1.0.0/app.zip" {
		t.Fatalf("Location = %q", loc)
	}
}
