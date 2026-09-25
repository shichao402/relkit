package main

import (
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"testing"

	rupv2 "go.firoyang.com/relkit/api/rup/v2"
	"go.firoyang.com/relkit/internal/webmeta"
)

func selectors(pairs map[string]string) []*rupv2.Selector {
	keys := make([]string, 0, len(pairs))
	for key := range pairs {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]*rupv2.Selector, 0, len(keys))
	for _, key := range keys {
		out = append(out, &rupv2.Selector{Key: key, Value: pairs[key]})
	}
	return out
}

func getBody(t *testing.T, url string) string {
	t.Helper()
	resp := testGet(t, url)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: status = %d, want 200", url, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("GET %s: read body: %v", url, err)
	}
	return string(body)
}

func mustContain(t *testing.T, what, body string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(body, want) {
			t.Errorf("%s missing %q\n%s", what, want, body)
		}
	}
}

// The operator panel lists every product this box distributes.
func TestRootPortalListsEveryProduct(t *testing.T) {
	srv, dir := newTestServer(t, false)
	writeRelease(t, dir, "svn-auto-merge", "stable", "1.4.2", 142)
	writeRelease(t, dir, "svn-auto-merge", "dev", "1.5.0", 150)
	writeRelease(t, dir, "otherapp", "stable", "0.9.0", 90)

	body := getBody(t, srv.URL+"/-/admin")
	mustContain(t, "portal", body,
		"svn-auto-merge",
		"otherapp",
		`href="/-/p/svn-auto-merge"`,
		`href="/-/p/otherapp"`,
		"1.4.2",
		"1.5.0",
		"2 products",
	)
	if strings.Contains(body, `href="/index/`) {
		t.Errorf("portal should not link into the key space\n%s", body)
	}

	stable := strings.Index(body, "<td>stable</td>")
	dev := strings.Index(body, "<td>dev</td>")
	if stable < 0 || dev < 0 || stable > dev {
		t.Errorf("stable should precede dev (stable=%d dev=%d)\n%s", stable, dev, body)
	}
}

func TestPortalFallsBackToListingWithoutReadableIndex(t *testing.T) {
	srv, dir := newTestServer(t, false)
	writeFile(t, dir, "public/notes.txt", []byte("hello"))

	body := getBody(t, srv.URL+"/-/admin")
	mustContain(t, "listing", body, "entries")
	if strings.Contains(body, productPathPrefix) {
		t.Errorf("expected a plain listing, got the portal\n%s", body)
	}
}

// Operators still need the raw key space, and the portal must not take it away.
func TestFilesQueryForcesListing(t *testing.T) {
	srv, dir := newTestServer(t, false)
	writeRelease(t, dir, "app", "stable", "1.0.0", 100)

	body := getBody(t, srv.URL+"/-/admin/files")
	mustContain(t, "forced listing", body, "entries", "index", "manifest", "artifact")
	if strings.Contains(body, productPathPrefix) {
		t.Errorf("/-/admin/files should render the listing, not the portal\n%s", body)
	}
}

func TestProductPageShowsArtifactsOfLatestVersion(t *testing.T) {
	srv, dir := newTestServer(t, false)
	writeRelease(t, dir, "app", "stable", "1.0.0", 100)
	writeRelease(t, dir, "app", "stable", "2.0.0", 200)

	body := getBody(t, srv.URL+"/-/p/app")
	mustContain(t, "product page", body,
		"app",
		"stable",
		"2.0.0",
		"app.zip",
		`class="release-card"`,
		`class="download-btn"`,
		`<details class="technical">`,
		`href="/artifact/app/2.0.0/app.zip"`,
		`href="/manifest/app/2.0.0.pb"`,
		strings.Repeat("b", 12), // sha256 prefix from the manifest fixture
		"Sequence",
	)
	// The page links back to where the visitor came from.
	mustContain(t, "product page", body, `href="/-/admin"`)
}

func TestProductPageUnknownProductIs404(t *testing.T) {
	srv, dir := newTestServer(t, false)
	writeRelease(t, dir, "app", "stable", "1.0.0", 100)

	for _, path := range []string{"/-/p/nope", "/-/p/", "/-/p/app/extra", "/-/p/..%2f.."} {
		resp := testGet(t, srv.URL+path)
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("GET %s: status = %d, want 404", path, resp.StatusCode)
		}
	}
}

func TestPortalHeadRequestSendsNoBody(t *testing.T) {
	srv, dir := newTestServer(t, false)
	writeRelease(t, dir, "app", "stable", "1.0.0", 100)

	for _, path := range []string{"/-/admin", "/-/p/app"} {
		req, _ := http.NewRequest(http.MethodHead, srv.URL+path, nil)
		attachTestPanelCookie(req)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("HEAD %s: %v", path, err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("HEAD %s: status = %d, want 200", path, resp.StatusCode)
		}
		if got := resp.Header.Get("Content-Type"); !strings.HasPrefix(got, "text/html") {
			t.Errorf("HEAD %s: Content-Type = %q", path, got)
		}
		if len(body) != 0 {
			t.Errorf("HEAD %s returned %d bytes of body", path, len(body))
		}
	}
}

// The pages read the index on every request, so a mutable pointer must never
// be served from a cache that would keep showing yesterday's release.
func TestPortalIsNotCached(t *testing.T) {
	srv, dir := newTestServer(t, false)
	writeRelease(t, dir, "app", "stable", "1.0.0", 100)

	for _, path := range []string{"/-/admin", "/-/p/app", "/-/admin/files"} {
		resp := testGet(t, srv.URL+path)
		resp.Body.Close()
		if got := resp.Header.Get("Cache-Control"); got != "no-cache" {
			t.Errorf("GET %s: Cache-Control = %q, want no-cache", path, got)
		}
	}
}

// writePlatformRelease publishes one version with a per-platform artifact set,
// which is what the recommended-download line needs to work with.
func writePlatformRelease(t *testing.T, dir, product, channel, version string, code int) {
	t.Helper()
	base := "http://example.com"
	manifestKey := fmt.Sprintf("manifest/%s/%s.pb", product, version)

	var artifacts []*rupv2.Artifact
	for _, target := range []struct{ os, arch, file string }{
		{"windows", "x64", "app-windows-x64.zip"},
		{"macos", "arm64", "app-macos-arm64.zip"},
	} {
		writeFile(t, dir, fmt.Sprintf("artifact/%s/%s/%s", product, version, target.file), []byte("pkg"))
		artifacts = append(artifacts, &rupv2.Artifact{
			Id:       target.os,
			Filename: target.file,
			Size:     3,
			Sha256:   strings.Repeat("c", 64),
			Kind:     rupv2.ArtifactKind_ARTIFACT_KIND_ARCHIVE,
			Selectors: selectors(map[string]string{
				"os": target.os, "arch": target.arch,
			}),
			Urls: []string{fmt.Sprintf("%s/artifact/%s/%s/%s", base, product, version, target.file)},
		})
	}

	manifest := &rupv2.Manifest{
		Schema:    rupv2.SchemaManifest,
		Product:   product,
		Version:   version,
		Code:      int64(code),
		Artifacts: artifacts,
	}
	raw, err := rupv2.MarshalManifest(manifest)
	if err != nil {
		t.Fatalf("MarshalManifest: %v", err)
	}
	writeFile(t, dir, manifestKey, raw)
	writeFile(t, dir, fmt.Sprintf("index/%s/%s.pb", product, channel),
		mustEnvelope(t, indexDoc(product, channel, version, code, base+"/"+manifestKey)))
	latest, err := webmeta.MarshalLatest(webmeta.Latest{
		Product:     product,
		Channel:     channel,
		Version:     version,
		Code:        int64(code),
		PublishedAt: "2026-08-25T00:00:00Z",
		Artifacts:   webmeta.ArtifactsFromManifest(manifest),
	})
	if err != nil {
		t.Fatalf("MarshalLatest: %v", err)
	}
	writeFile(t, dir, webmeta.LatestKey(product, channel), latest)
}

func getWithAgent(t *testing.T, url, agent string) string {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("User-Agent", agent)
	attachTestPanelCookie(req)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return string(body)
}

func TestEachChannelLinksItsOwnLatestBuild(t *testing.T) {
	srv, dir := newTestServer(t, false)
	writePlatformRelease(t, dir, "app", "stable", "1.0.0", 100)
	writePlatformRelease(t, dir, "app", "beta", "1.1.0", 110)

	windows := getWithAgent(t, srv.URL+"/-/admin",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/140 Safari/537.36")
	mustContain(t, "windows portal", windows,
		`href="/-/latest/app/stable/windows"`,
		`href="/-/latest/app/beta/windows"`,
		`title="app-windows-x64.zip`,
	)
	if strings.Contains(windows, "/-/latest/app/stable/macos") {
		t.Errorf("a Windows visitor should not be offered the macOS build\n%s", windows)
	}

	mac := getWithAgent(t, srv.URL+"/-/admin",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 Safari/605.1.15")
	mustContain(t, "mac portal", mac, `href="/-/latest/app/stable/macos"`)

	// An unrecognized client is offered nothing rather than a wrong guess.
	robot := getWithAgent(t, srv.URL+"/-/admin", "curl/8.4.0")
	if strings.Contains(robot, "/-/latest/app/stable/") {
		t.Errorf("unknown platform should get no recommendation\n%s", robot)
	}
}

// The product page has to hand out the durable URL, because that is the one
// people paste into release notes.
func TestProductPageListsFixedChannelLinks(t *testing.T) {
	srv, dir := newTestServer(t, false)
	writePlatformRelease(t, dir, "app", "stable", "1.0.0", 100)

	mustContain(t, "product page", getBody(t, srv.URL+"/-/p/app"),
		`href="/-/latest/app/stable/windows"`,
		`href="/-/latest/app/stable/macos"`,
	)
}

func TestFixedLatestURLUsesPublishedPointer(t *testing.T) {
	srv, dir := newTestServer(t, false)
	writePlatformRelease(t, dir, "app", "stable", "1.0.0", 100)

	client := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Get(srv.URL + "/-/latest/app/stable/windows")
	if err != nil {
		t.Fatalf("GET latest: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("GET latest: status = %d, want 302", resp.StatusCode)
	}
	if got, want := resp.Header.Get("Location"), "/artifact/app/1.0.0/app-windows-x64.zip"; got != want {
		t.Errorf("Location = %q, want %q", got, want)
	}
	if got := resp.Header.Get("Cache-Control"); got != "no-cache" {
		t.Errorf("Cache-Control = %q, want no-cache", got)
	}

	for _, path := range []string{
		"/-/latest/app/stable/no-such-artifact",
		"/-/latest/app/no-such-channel/windows",
		"/-/latest/no-such-product/stable/windows",
		"/-/latest/app/windows",
		"/-/latest/app/stable",
	} {
		response, err := client.Get(srv.URL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusNotFound {
			t.Errorf("GET %s: status = %d, want 404", path, response.StatusCode)
		}
	}
}

// Downloads are recorded by whoever serves the tree; the console only reads
// the counters and surfaces them on the panel. The counts must survive a
// console restart, which is what the shared stats file is for.
func TestDownloadCountsPersistAcrossConsoleRestart(t *testing.T) {
	c, dir := newTestConsole(t)
	writeRelease(t, dir, "app", "stable", "1.0.0", 100)
	c.stats.record("artifact/app/1.0.0/app.zip")
	if err := c.stats.flush(); err != nil {
		t.Fatalf("flush stats: %v", err)
	}
	since := c.stats.startedAt()

	// A fresh console instance over the same tree picks the counters up.
	adapter, err := newRootAdapter("local", dir)
	if err != nil {
		t.Fatalf("newRootAdapter: %v", err)
	}
	c2 := &console{
		adapter: adapter,
		stats:   newDownloadStats(defaultStatsPath(dir), dir),
		admin:   readyTestAdmin(t, dir),
	}
	t.Cleanup(c2.stats.stop)
	srv := newLocalServer(t, c2)
	rememberPanelCookie(t, srv, c2)

	portal := getBody(t, srv.URL+"/-/admin")
	mustContain(t, "persisted portal", portal, "1 downloads", "persist across restarts", since)
	product := getBody(t, srv.URL+"/-/p/app")
	mustContain(t, "persisted product", product, "1 downloads", "persist across restarts")
}
