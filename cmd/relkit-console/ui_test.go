package main

import (
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"testing"

	rupv2 "github.com/shichao402/relkit/api/rup/v2"
	"github.com/shichao402/relkit/internal/webmeta"
)

// The front end is a single-page app, so the page tests that used to read
// server-rendered HTML are gone: the resources they asserted on are covered by
// api_resources_test.go, which reads the same facts as JSON. What stays here is
// exactly the surface the console still serves by itself: the app shell, the
// fixed /-/latest/ redirects, and the download counters behind them.

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

// appShellRoutes are the paths the front end owns. They all answer with the
// same shell; the router inside decides what to render.
var appShellRoutes = []string{
	appPrefix + "products",
	appPrefix + "users",
	appPrefix + "files",
	adminFilesPath,
}

func TestAppShellServesEveryClientRoute(t *testing.T) {
	srv, dir := newTestServer(t, false)
	writeRelease(t, dir, "app", "stable", "1.0.0", 100)

	for _, path := range appShellRoutes {
		body := getBody(t, srv.URL+path)
		if !strings.Contains(body, `<div id="root">`) {
			t.Errorf("GET %s should serve the app shell\n%s", path, body)
		}
		if strings.Contains(body, productPathPrefix) {
			t.Errorf("GET %s should not render a server-side page\n%s", path, body)
		}
	}
}

// A deep link that is not an API or asset path still has to reach the router,
// otherwise a bookmark to a product detail page would 404 on reload.
func TestAppShellServesDeepLinks(t *testing.T) {
	srv, _ := newTestServer(t, false)
	mustContain(t, "deep link", getBody(t, srv.URL+appPrefix+"products/app/tokens"), `<div id="root">`)
}

// The unknown-product case moved to the API (which answers JSON 404), so the
// shell must not try to judge the id: a brand new product's page is opened
// before its first publish.
func TestAppShellDoesNotValidateIds(t *testing.T) {
	srv, _ := newTestServer(t, false)
	mustContain(t, "unknown id", getBody(t, srv.URL+appPrefix+"products/not-registered-yet"), `<div id="root">`)
}

func TestAppShellHeadRequestSendsNoBody(t *testing.T) {
	srv, _ := newTestServer(t, false)

	for _, path := range appShellRoutes {
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

// The shell names a hashed bundle, so caching it would pin a bundle that a
// later console build no longer ships.
func TestAppShellIsNotCached(t *testing.T) {
	srv, dir := newTestServer(t, false)
	writeRelease(t, dir, "app", "stable", "1.0.0", 100)

	for _, path := range appShellRoutes {
		resp := testGet(t, srv.URL+path)
		resp.Body.Close()
		got := resp.Header.Get("Cache-Control")
		if got != "no-cache" && got != "no-store" {
			t.Errorf("GET %s: Cache-Control = %q, want no-cache or no-store", path, got)
		}
	}
}

// writePlatformRelease publishes one version with a per-platform artifact set,
// which is what the fixed /-/latest/ links need to resolve against.
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

// Downloads are recorded by whoever serves the tree; the console only reads the
// counters and reports them on the product list and detail. The counters live
// in a shared stats file so a console restart does not reset them.
func TestAPIProductsReportRecordedDownloads(t *testing.T) {
	c, dir := newTestConsole(t)
	writeRelease(t, dir, "app", "stable", "1.0.0", 100)
	c.stats.record("artifact/app/1.0.0/app.zip")
	if err := c.stats.flush(); err != nil {
		t.Fatalf("flush stats: %v", err)
	}

	srv := newLocalServer(t, c)
	api := newAPIClient(t, srv)
	api.login()

	status, body := api.call(http.MethodGet, apiPrefix+"products", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("products = %d %v", status, body)
	}
	var found bool
	for _, row := range asSlice(t, body["products"]) {
		m := asMap(t, row)
		if m["id"] != "app" {
			continue
		}
		found = true
		if m["downloads"] != float64(1) {
			t.Fatalf("downloads = %v, want 1", m["downloads"])
		}
	}
	if !found {
		t.Fatalf("app missing from %v", body)
	}

	status, body = api.call(http.MethodGet, apiPrefix+"products/app", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("detail = %d %v", status, body)
	}
	product := asMap(t, body["product"])
	if product["statsSince"] == nil || product["statsSince"] == "" {
		t.Errorf("detail should say since when the counters are counted: %v", product)
	}
	channel := asMap(t, asSlice(t, product["detail"])[0])
	artifacts := asSlice(t, channel["artifacts"])
	if len(artifacts) != 1 || asMap(t, artifacts[0])["downloads"] != float64(1) {
		t.Fatalf("artifacts = %v", artifacts)
	}
}
