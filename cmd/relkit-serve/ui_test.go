package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	rupv2 "cnb.cool/shichao402/relkit/api/rup/v2"
	"cnb.cool/shichao402/relkit/internal/webmeta"
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
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
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

	root := getBody(t, srv.URL+"/")
	mustContain(t, "catalog stub", root, "No published catalog yet", `href="/-/admin"`)
	if strings.Contains(root, "2 products") {
		t.Errorf("GET / must not render the operator portal\n%s", root)
	}
}

func TestPortalFallsBackToListingWithoutReadableIndex(t *testing.T) {
	srv, dir := newTestServer(t, false)
	writeFile(t, dir, "public/notes.txt", []byte("hello"))

	body := getBody(t, srv.URL+"/-/admin")
	mustContain(t, "listing", body, `href="/public/"`, "entries")
	if strings.Contains(body, productPathPrefix) {
		t.Errorf("expected a plain listing, got the portal\n%s", body)
	}
}

// Operators still need the raw key space, and the portal must not take it away.
func TestFilesQueryForcesListing(t *testing.T) {
	srv, dir := newTestServer(t, false)
	writeRelease(t, dir, "app", "stable", "1.0.0", 100)

	body := getBody(t, srv.URL+"/-/admin/files")
	mustContain(t, "forced listing", body, `href="/index/"`, `href="/manifest/"`, `href="/artifact/"`)
	if strings.Contains(body, productPathPrefix) {
		t.Errorf("/-/admin/files should render the listing, not the portal\n%s", body)
	}
}

func TestWrittenBrowseIndexIsServedAtRoot(t *testing.T) {
	srv, dir := newTestServer(t, false)
	writeRelease(t, dir, "app", "stable", "1.0.0", 100)
	writeFile(t, dir, "browse/index.html", []byte("<html><body>static catalog app-from-disk</body></html>"))
	writeFile(t, dir, "browse/app.html", []byte("<html><body>static product page</body></html>"))

	root := getBody(t, srv.URL+"/")
	if !strings.Contains(root, "static catalog app-from-disk") {
		t.Fatalf("GET / should serve browse/index.html\n%s", root)
	}
	browse := getBody(t, srv.URL+"/browse/")
	if !strings.Contains(browse, "static catalog app-from-disk") {
		t.Fatalf("GET /browse/ should serve browse/index.html\n%s", browse)
	}
	product := getBody(t, srv.URL+"/browse/app.html")
	if !strings.Contains(product, "static product page") {
		t.Fatalf("GET /browse/app.html should serve the dump\n%s", product)
	}
	rooted := getBody(t, srv.URL+"/app.html")
	if !strings.Contains(rooted, "static product page") {
		t.Fatalf("GET /app.html should fall back to browse/app.html\n%s", rooted)
	}
	live := getBody(t, srv.URL+"/-/p/app")
	if strings.Contains(live, "static product page") {
		t.Fatalf("GET /-/p/app is the operator page, not the dump\n%s", live)
	}
	mustContain(t, "operator product", live, `class="release-card"`)
	listing := getBody(t, srv.URL+"/-/admin/files")
	if !strings.Contains(listing, `href="/index/"`) {
		t.Fatalf("/-/admin/files should still list the tree\n%s", listing)
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
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("GET %s: status = %d, want 404", path, resp.StatusCode)
		}
	}
}

func TestPortalHeadRequestSendsNoBody(t *testing.T) {
	srv, dir := newTestServer(t, false)
	writeRelease(t, dir, "app", "stable", "1.0.0", 100)

	for _, path := range []string{"/", "/-/admin", "/-/p/app"} {
		req, _ := http.NewRequest(http.MethodHead, srv.URL+path, nil)
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

	for _, path := range []string{"/", "/-/admin", "/-/p/app", "/-/admin/files"} {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
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
			Kind:     "archive",
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

func TestArtifactDownloadsAreCounted(t *testing.T) {
	srv, dir := newTestServer(t, false)
	writeRelease(t, dir, "app", "stable", "1.0.0", 100)
	target := srv.URL + "/artifact/app/1.0.0/app.zip"

	getBody(t, target)

	// A split download still asks for one range from zero; the other slices
	// must not each count as a download.
	for _, rng := range []string{"bytes=0-1", "bytes=1-2"} {
		req, _ := http.NewRequest(http.MethodGet, target, nil)
		req.Header.Set("Range", rng)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("ranged GET: %v", err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}

	body := getBody(t, srv.URL+"/-/p/app")
	if !strings.Contains(body, `2 downloads`) {
		t.Errorf("expected 2 counted downloads (plain + range from zero)\n%s", body)
	}

	portal := getBody(t, srv.URL+"/-/admin")
	mustContain(t, "portal", portal, "2 downloads")

	// HEAD is a metadata probe, not a download.
	req, _ := http.NewRequest(http.MethodHead, target, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("HEAD artifact: %v", err)
	}
	resp.Body.Close()
	if got := getBody(t, srv.URL+"/-/admin"); !strings.Contains(got, "2 downloads") {
		t.Errorf("HEAD should not change the count\n%s", got)
	}
}

func TestDownloadCountsPersistAcrossRestart(t *testing.T) {
	cfg, dir := newTestConfig(t, false)
	writeRelease(t, dir, "app", "stable", "1.0.0", 100)
	srv := newLocalServer(t, cfg)
	getBody(t, srv.URL+"/artifact/app/1.0.0/app.zip")
	if err := cfg.stats.flush(); err != nil {
		t.Fatalf("flush stats: %v", err)
	}
	since := cfg.stats.startedAt()
	srv.Close()

	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	t.Cleanup(func() { root.Close() })
	cfg2 := &config{
		root:          root,
		rootPath:      dir,
		maxUpload:     1 << 20,
		noCache:       []string{"index/"},
		immutable:     []string{"manifest/", "artifact/"},
		defaultMaxAge: 60,
		stats:         newDownloadStats(defaultStatsPath(dir), dir),
	}
	t.Cleanup(cfg2.stats.stop)
	srv2 := newLocalServer(t, cfg2)
	t.Cleanup(srv2.Close)

	portal := getBody(t, srv2.URL+"/-/admin")
	mustContain(t, "persisted portal", portal, "1 downloads", "persist across restarts", since)
	product := getBody(t, srv2.URL+"/-/p/app")
	mustContain(t, "persisted product", product, "1 downloads", "persist across restarts")
}

func TestDownloadStatsFileIsNotPublic(t *testing.T) {
	cfg, dir := newTestConfig(t, true)
	writeRelease(t, dir, "app", "stable", "1.0.0", 100)
	srv := newLocalServer(t, cfg)
	t.Cleanup(srv.Close)
	getBody(t, srv.URL+"/artifact/app/1.0.0/app.zip")
	if err := cfg.stats.flush(); err != nil {
		t.Fatalf("flush stats: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, statsFileName)); err != nil {
		t.Fatalf("stats file: %v", err)
	}

	resp, err := http.Get(srv.URL + "/" + statsFileName)
	if err != nil {
		t.Fatalf("GET stats file: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("GET %s status = %d, want 404", statsFileName, resp.StatusCode)
	}

	listing := getBody(t, srv.URL+"/-/admin/files")
	if strings.Contains(listing, statsFileName) {
		t.Errorf("listing should omit the stats file\n%s", listing)
	}

	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/"+statsFileName, strings.NewReader("{}"))
	req.Header.Set("Authorization", "Bearer "+testToken)
	put, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT stats file: %v", err)
	}
	put.Body.Close()
	if put.StatusCode != http.StatusBadRequest {
		t.Errorf("PUT %s status = %d, want 400", statsFileName, put.StatusCode)
	}
}

func TestSiteConfigSuppliesTitlesAndBlurbs(t *testing.T) {
	cfg, dir := newTestConfig(t, false)
	cfg.site = &SiteConfig{
		Title: "OSGame updates",
		Products: map[string]*ProductConfig{
			"app": {
				Title:       "Demo App",
				Description: "Internal build distribution for the demo app.",
				Homepage:    "https://example.com/demo",
			},
		},
	}
	srv := newLocalServer(t, cfg)
	t.Cleanup(srv.Close)
	writeRelease(t, dir, "app", "stable", "1.0.0", 100)

	portal := getBody(t, srv.URL+"/-/admin")
	mustContain(t, "portal", portal,
		"OSGame updates",
		"Demo App",
		"Internal build distribution for the demo app.",
		`href="https://example.com/demo"`,
	)

	product := getBody(t, srv.URL+"/-/p/app")
	mustContain(t, "product page", product, "Demo App", "Internal build distribution")
}

func TestPagesOfferSystemLightAndDarkThemes(t *testing.T) {
	srv, dir := newTestServer(t, false)
	writeRelease(t, dir, "app", "stable", "1.0.0", 100)

	for _, page := range []string{getBody(t, srv.URL+"/-/admin"), getBody(t, srv.URL+"/-/p/app")} {
		mustContain(t, "theme control", page,
			`<html lang="en" data-theme="system">`,
			`id="theme-select"`,
			`<option value="system">System</option>`,
			`<option value="light">Light</option>`,
			`<option value="dark">Dark</option>`,
			`prefers-color-scheme:dark`,
			`localStorage.setItem("relkit-theme",v)`,
		)
	}
}

func TestPublishedSiteCopyOverridesServerFallback(t *testing.T) {
	cfg, dir := newTestConfig(t, false)
	cfg.site = &SiteConfig{
		Title: "Server title",
		Products: map[string]*ProductConfig{
			"app": {Title: "Old server copy", Description: "old"},
		},
	}
	srv := newLocalServer(t, cfg)
	t.Cleanup(srv.Close)
	writeRelease(t, dir, "app", "stable", "1.0.0", 100)

	raw, err := webmeta.MarshalSite(webmeta.Site{
		Product:     "app",
		Title:       "Team-owned title",
		Description: "Published by the product team.",
		Homepage:    "https://example.com/team",
		UpdatedAt:   "2026-08-25T00:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, webmeta.SiteKey("app"), raw)

	body := getBody(t, srv.URL+"/-/admin")
	mustContain(t, "portal", body, "Team-owned title", "Published by the product team.")
	if strings.Contains(body, "Old server copy") {
		t.Errorf("server fallback won over published site document\n%s", body)
	}
}

func TestGuessPlatform(t *testing.T) {
	cases := []struct {
		agent string
		want  platformGuess
	}{
		{"Mozilla/5.0 (Windows NT 10.0; Win64; x64)", platformGuess{"windows", "amd64"}},
		{"Mozilla/5.0 (Windows NT 10.0; ARM64)", platformGuess{"windows", "arm64"}},
		{"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)", platformGuess{"darwin", ""}},
		{"Mozilla/5.0 (X11; Linux x86_64)", platformGuess{"linux", "amd64"}},
		// Android carries "Linux" in the same string, so order matters.
		{"Mozilla/5.0 (Linux; Android 14; Pixel 8)", platformGuess{"android", ""}},
		{"curl/8.4.0", platformGuess{}},
	}
	for _, tc := range cases {
		if got := guessPlatform(tc.agent); got != tc.want {
			t.Errorf("guessPlatform(%q) = %+v, want %+v", tc.agent, got, tc.want)
		}
	}
}

func TestBestMatchPrefersExactArch(t *testing.T) {
	rows := []artifactRow{
		{Filename: "win-arm64.zip", Href: "/a/win-arm64.zip", OS: "windows", Arch: "arm64"},
		{Filename: "win-x64.zip", Href: "/a/win-x64.zip", OS: "windows", Arch: "x64"},
		{Filename: "any.zip", Href: "/a/any.zip"},
	}
	if got := bestMatch(rows, platformGuess{"windows", "amd64"}); got == nil || got.Filename != "win-x64.zip" {
		t.Errorf("bestMatch picked %+v, want win-x64.zip", got)
	}
	// Unknown arch still gets the right OS rather than nothing.
	if got := bestMatch(rows, platformGuess{os: "windows"}); got == nil || got.OS != "windows" {
		t.Errorf("bestMatch picked %+v, want a windows build", got)
	}
	if got := bestMatch(rows, platformGuess{"linux", "amd64"}); got != nil {
		t.Errorf("bestMatch picked %+v for an unpublished platform, want nil", got)
	}
}

func TestPlatformLabelFromSelectors(t *testing.T) {
	cases := []struct {
		name string
		in   map[string]string
		want string
	}{
		{"os and arch", map[string]string{"os": "windows", "arch": "x64"}, "windows/x64"},
		{"os only", map[string]string{"os": "linux"}, "linux"},
		{"none", nil, "any"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := platformLabel(selectors(tc.in)); got != tc.want {
				t.Errorf("platformLabel = %q, want %q", got, tc.want)
			}
		})
	}
}
