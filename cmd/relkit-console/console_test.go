package main

import (
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.firoyang.com/relkit/internal/webmeta"
)

const (
	testPanelUser     = "op"
	testPanelPassword = "password123"
)

// newTestConsole assembles a console over a temp release tree, the test
// counterpart of main(): adapter over the tree, admin state in it, stats at
// the default hidden path.
func newTestConsole(t *testing.T) (*console, string) {
	t.Helper()
	dir := t.TempDir()
	adapter, err := newRootAdapter("local", dir)
	if err != nil {
		t.Fatalf("newRootAdapter: %v", err)
	}
	c := &console{
		adapter: adapter,
		stats:   newDownloadStats(defaultStatsPath(dir), dir),
		admin:   readyTestAdmin(t, dir),
	}
	t.Cleanup(c.stats.stop)
	return c, dir
}

// readyTestAdmin is the serve version carried over: one user, deterministic
// session key, so requirePanelAuth passes after login.
func readyTestAdmin(t *testing.T, dir string) *adminAuth {
	t.Helper()
	hash, err := hashPassword(testPanelPassword)
	if err != nil {
		t.Fatal(err)
	}
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 1)
	}
	path := filepath.Join(dir, adminStateFileName)
	doc := adminDoc{
		SessionKey: base64.RawURLEncoding.EncodeToString(key),
		Users:      []adminUser{{Username: testPanelUser, PasswordHash: hash}},
	}
	if err := writeAdminDoc(path, doc); err != nil {
		t.Fatal(err)
	}
	admin, err := openAdminAuth(path, dir)
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

func loginPanel(t *testing.T, client *http.Client, srv *httptest.Server) {
	t.Helper()
	resp, err := client.PostForm(srv.URL+adminLoginPath, map[string][]string{
		"username": {testPanelUser},
		"password": {testPanelPassword},
	})
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	resp.Body.Close()
	// The jar follows the redirect: login already landed on the panel page.
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), testPanelUser) {
		t.Fatalf("login = %d body=%s", resp.StatusCode, body)
	}
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
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("panel = %d", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != adminLoginPath {
		t.Fatalf("Location = %q", loc)
	}
}

func TestLoginServesPortalAndFiles(t *testing.T) {
	c, dir := newTestConsole(t)
	writeFile(t, dir, "artifact/app/1.0.0/app.zip", []byte("pkg"))
	srv := newLocalServer(t, c)
	client := jarClient(t, srv)
	loginPanel(t, client, srv)

	resp, err := client.Get(srv.URL + adminPath)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("portal = %d", resp.StatusCode)
	}
	// No parsable index documents: the panel falls back to the file listing,
	// and the listing mentions the artifact directory.
	if !strings.Contains(string(body), "app") {
		t.Fatalf("listing missing app entry")
	}

	resp, err = client.Get(srv.URL + adminFilesPath)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("files = %d", resp.StatusCode)
	}
}

func TestRootAdapterListsAndReads(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "index/dec/dev.pb", []byte("x"))
	a, err := newRootAdapter("local", dir)
	if err != nil {
		t.Fatal(err)
	}
	if a.Name() != "local" {
		t.Fatalf("name = %q", a.Name())
	}
	entries, err := a.ReadDir("index/dec")
	if err != nil || len(entries) != 1 || entries[0].Name != "dev.pb" {
		t.Fatalf("entries=%+v err=%v", entries, err)
	}
	raw, err := a.ReadKey("index/dec/dev.pb")
	if err != nil || string(raw) != "x" {
		t.Fatalf("read=%q err=%v", raw, err)
	}
	if a.ModTime("index/dec/dev.pb").IsZero() {
		t.Fatal("modtime missing")
	}
	if !a.ModTime("missing").IsZero() {
		t.Fatal("missing key should be zero time")
	}
}

func TestSiteStatusViewReadsSnapshot(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "site/status.json", []byte(
		`{"at":"2026-09-25T10:00:00Z","sinks":[{"name":"makers:relkit-updates-index","ok":true,"deploymentId":"dep-1"}]}`))
	v := siteStatusView{StateDir: dir}
	status, ok := v.readStatus()
	if !ok {
		t.Fatal("snapshot missing")
	}
	if status.At != "2026-09-25T10:00:00Z" || len(status.Sinks) != 1 {
		t.Fatalf("status=%+v", status)
	}
	if status.Sinks[0].DeploymentID != "dep-1" {
		t.Fatalf("sink=%+v", status.Sinks[0])
	}

	v2 := siteStatusView{StateDir: filepath.Join(dir, "nowhere")}
	if _, ok := v2.readStatus(); ok {
		t.Fatal("missing snapshot should not be found")
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
