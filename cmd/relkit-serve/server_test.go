package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"

	rupv2 "cnb.cool/shichao402/relkit/api/rup/v2"
	"cnb.cool/shichao402/relkit/internal/publishproto"
	"google.golang.org/protobuf/proto"
)

const testToken = "s3cr3t-token"

func newTestConfig(t *testing.T, withToken bool) (*config, string) {
	t.Helper()
	dir := t.TempDir()

	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatalf("OpenRoot: %v", err)
	}
	t.Cleanup(func() { root.Close() })

	cfg := &config{
		root:          root,
		rootPath:      dir,
		maxUpload:     1 << 20,
		noCache:       []string{"index/"},
		immutable:     []string{"manifest/", "artifact/"},
		defaultMaxAge: 60,
		stats:         newDownloadStats(defaultStatsPath(dir), dir),
	}
	if withToken {
		cfg.credentials = []credential{{hash: hashToken(testToken)}}
	}
	t.Cleanup(cfg.stats.stop)
	return cfg, dir
}

func TestPublishProtocolIsEnforcedBeforeWrite(t *testing.T) {
	cfg, dir := newTestConfig(t, true)
	cfg.minPublishProtocol = publishproto.Current
	srv := newLocalServer(t, cfg)
	t.Cleanup(srv.Close)

	put := func(protocol string, authorized bool) *http.Response {
		t.Helper()
		req, _ := http.NewRequest(http.MethodPut, srv.URL+"/artifact/app/1.0.0/app.zip", strings.NewReader("payload"))
		if authorized {
			req.Header.Set("Authorization", "Bearer "+testToken)
		}
		if protocol != "" {
			req.Header.Set(publishproto.ProtocolHeader, protocol)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("PUT: %v", err)
		}
		return resp
	}

	// Authentication remains the first gate; anonymous callers do not learn
	// which publisher generation the deployment requires.
	resp := put("", false)
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthorized PUT status = %d, want 401", resp.StatusCode)
	}

	for _, protocol := range []string{"", "1", "not-a-number"} {
		resp = put(protocol, true)
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusUpgradeRequired {
			t.Errorf("protocol %q status = %d, want 426; body=%s", protocol, resp.StatusCode, body)
		}
		if !strings.Contains(string(body), "publisher_upgrade_required") {
			t.Errorf("protocol %q body = %s", protocol, body)
		}
		if _, err := os.Stat(filepath.Join(dir, "artifact", "app")); !os.IsNotExist(err) {
			t.Errorf("protocol %q created files before rejection", protocol)
		}
	}

	resp = put(strconv.Itoa(publishproto.Current), true)
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("current protocol status = %d, want 201", resp.StatusCode)
	}
}

func TestPublishPreflight(t *testing.T) {
	cfg, _ := newTestConfig(t, true)
	cfg.minPublishProtocol = publishproto.Current
	srv := newLocalServer(t, cfg)
	t.Cleanup(srv.Close)

	request := func(protocol string) *http.Response {
		t.Helper()
		req, _ := http.NewRequest(http.MethodPost, srv.URL+publishproto.PreflightPath, nil)
		req.Header.Set("Authorization", "Bearer "+testToken)
		if protocol != "" {
			req.Header.Set(publishproto.ProtocolHeader, protocol)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("POST preflight: %v", err)
		}
		return resp
	}

	resp := request("1")
	resp.Body.Close()
	if resp.StatusCode != http.StatusUpgradeRequired {
		t.Fatalf("old protocol status = %d, want 426", resp.StatusCode)
	}
	if got := resp.Header.Get("Upgrade"); got != "relkit-publish/2" {
		t.Errorf("Upgrade = %q, want relkit-publish/2", got)
	}

	resp = request(strconv.Itoa(publishproto.Current))
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), `"ok":true`) {
		t.Fatalf("current protocol status = %d body=%s", resp.StatusCode, body)
	}
}

func newTestServer(t *testing.T, withToken bool) (*httptest.Server, string) {
	t.Helper()
	cfg, dir := newTestConfig(t, withToken)
	srv := newLocalServer(t, cfg)
	t.Cleanup(srv.Close)
	return srv, dir
}

// newLocalServer is shared with the benchmarks, which use testing.B.
func newLocalServer(tb testing.TB, cfg *config) *httptest.Server {
	tb.Helper()
	return httptest.NewServer(cfg.handler())
}

func writeFile(t *testing.T, dir, name string, data []byte) {
	t.Helper()
	full := filepath.Join(dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(full, data, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

// payload is deterministic and larger than a single TCP window so that ranged
// reads land in different parts of the file.
func payload(size int) []byte {
	data := make([]byte, size)
	for i := range data {
		data[i] = byte(i % 251)
	}
	return data
}

func TestRangeRequestServesExactSlice(t *testing.T) {
	srv, dir := newTestServer(t, false)
	body := payload(500000)
	writeFile(t, dir, "artifact/app/1.0.0/app.zip", body)

	req, _ := http.NewRequest("GET", srv.URL+"/artifact/app/1.0.0/app.zip", nil)
	req.Header.Set("Range", "bytes=100-199")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusPartialContent {
		t.Fatalf("status = %d, want 206", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Range"); got != "bytes 100-199/500000" {
		t.Errorf("Content-Range = %q", got)
	}
	got, _ := io.ReadAll(resp.Body)
	if !bytes.Equal(got, body[100:200]) {
		t.Errorf("ranged body does not match the file slice")
	}
}

// TestParallelRangeDownload is the actual "multi-threaded download" case: many
// connections, each fetching one slice, reassembled by the client.
func TestParallelRangeDownload(t *testing.T) {
	srv, dir := newTestServer(t, false)
	body := payload(1 << 20)
	writeFile(t, dir, "artifact/app/1.0.0/big.zip", body)

	const workers = 16
	chunk := len(body) / workers
	parts := make([][]byte, workers)
	var wg sync.WaitGroup
	errs := make(chan error, workers)

	for i := range workers {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			start := i * chunk
			end := start + chunk - 1
			if i == workers-1 {
				end = len(body) - 1
			}
			req, _ := http.NewRequest("GET", srv.URL+"/artifact/app/1.0.0/big.zip", nil)
			req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				errs <- err
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusPartialContent {
				errs <- fmt.Errorf("worker %d: status %d", i, resp.StatusCode)
				return
			}
			data, err := io.ReadAll(resp.Body)
			if err != nil {
				errs <- err
				return
			}
			parts[i] = data
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("parallel fetch: %v", err)
	}

	if !bytes.Equal(bytes.Join(parts, nil), body) {
		t.Error("reassembled parallel download does not match the original file")
	}
}

func TestHeadReportsSizeWithoutBody(t *testing.T) {
	srv, dir := newTestServer(t, false)
	body := payload(40000)
	writeFile(t, dir, "artifact/app/1.0.0/app.zip", body)

	resp, err := http.Head(srv.URL + "/artifact/app/1.0.0/app.zip")
	if err != nil {
		t.Fatalf("HEAD: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if resp.ContentLength != int64(len(body)) {
		t.Errorf("Content-Length = %d, want %d", resp.ContentLength, len(body))
	}
	if resp.Header.Get("Accept-Ranges") != "bytes" {
		t.Error("Accept-Ranges should advertise byte ranges")
	}
	got, _ := io.ReadAll(resp.Body)
	if len(got) != 0 {
		t.Errorf("HEAD returned %d bytes of body", len(got))
	}
}

func TestCacheHeadersDistinguishMutableFromImmutable(t *testing.T) {
	srv, dir := newTestServer(t, false)
	writeFile(t, dir, "index/app/stable.pb", []byte("x"))
	writeFile(t, dir, "manifest/app/1.0.0.pb", []byte("y"))
	writeFile(t, dir, "other/notes.txt", []byte("hello"))

	cases := []struct {
		path string
		want string
	}{
		{"/index/app/stable.pb", "no-cache, must-revalidate"},
		{"/manifest/app/1.0.0.pb", "public, max-age=31536000, immutable"},
		{"/other/notes.txt", "public, max-age=60"},
	}
	for _, tc := range cases {
		resp, err := http.Get(srv.URL + tc.path)
		if err != nil {
			t.Fatalf("GET %s: %v", tc.path, err)
		}
		resp.Body.Close()
		if got := resp.Header.Get("Cache-Control"); got != tc.want {
			t.Errorf("%s: Cache-Control = %q, want %q", tc.path, got, tc.want)
		}
	}
}

func TestRejectsTraversal(t *testing.T) {
	srv, dir := newTestServer(t, false)
	writeFile(t, dir, "artifact/app/1.0.0/app.zip", []byte("inside"))

	secret := filepath.Join(filepath.Dir(dir), "outside.txt")
	if err := os.WriteFile(secret, []byte("must not be served"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	t.Cleanup(func() { os.Remove(secret) })

	for _, path := range []string{
		"/../outside.txt",
		"/artifact/../../outside.txt",
		"/%2e%2e/outside.txt",
	} {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("GET %s: status = %d, want 404", path, resp.StatusCode)
		}
		if bytes.Contains(body, []byte("must not be served")) {
			t.Fatalf("GET %s leaked a file outside the served directory", path)
		}
	}
}

func TestDirectoryListing(t *testing.T) {
	srv, dir := newTestServer(t, false)
	writeFile(t, dir, "artifact/app/1.0.0/app.zip", []byte("inside"))
	// An index that does not decode leaves no product to show, so the root
	// stays a file listing. That is also the plain-static-host case.
	writeFile(t, dir, "index/app/stable.pb", []byte("x"))

	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /: status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	html := string(body)
	for _, want := range []string{
		`<th>Name</th><th class="num">Size</th><th class="num">Modified</th>`,
		`href="artifact/"`,
		`href="index/"`,
		"artifact/",
		"index/",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("GET / body missing %q\n%s", want, html)
		}
	}
	// mtime column should show a formatted timestamp for each entry.
	if !regexp.MustCompile(`class="num">\d{4}-\d{2}-\d{2} \d{2}:\d{2}<`).MatchString(html) {
		t.Errorf("GET / body missing formatted mtime\n%s", html)
	}

	resp2, err := http.Get(srv.URL + "/artifact/")
	if err != nil {
		t.Fatalf("GET /artifact/: %v", err)
	}
	defer resp2.Body.Close()
	body2, _ := io.ReadAll(resp2.Body)
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("GET /artifact/: status = %d", resp2.StatusCode)
	}
	if !strings.Contains(string(body2), `href="app/"`) {
		t.Errorf("GET /artifact/ missing app/\n%s", body2)
	}

	// Directory without trailing slash redirects so relative links work.
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp3, err := client.Get(srv.URL + "/artifact")
	if err != nil {
		t.Fatalf("GET /artifact: %v", err)
	}
	defer resp3.Body.Close()
	if resp3.StatusCode != http.StatusMovedPermanently {
		t.Fatalf("GET /artifact: status = %d, want 301", resp3.StatusCode)
	}
	if loc := resp3.Header.Get("Location"); !strings.HasSuffix(loc, "/artifact/") {
		t.Errorf("Location = %q, want .../artifact/", loc)
	}
}

func TestUploadDisabledWithoutToken(t *testing.T) {
	srv, _ := newTestServer(t, false)

	req, _ := http.NewRequest("PUT", srv.URL+"/artifact/app/1.0.0/app.zip",
		strings.NewReader("data"))
	req.Header.Set("Authorization", "Bearer "+testToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", resp.StatusCode)
	}
	if got := resp.Header.Get("Allow"); got != "GET, HEAD" {
		t.Errorf("Allow = %q", got)
	}
}

func TestUploadRequiresCorrectToken(t *testing.T) {
	srv, dir := newTestServer(t, true)

	for _, header := range []string{"", "Bearer wrong", "Basic " + testToken, testToken} {
		req, _ := http.NewRequest("PUT", srv.URL+"/artifact/app/1.0.0/app.zip",
			strings.NewReader("data"))
		if header != "" {
			req.Header.Set("Authorization", header)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("PUT: %v", err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("Authorization %q: status = %d, want 401", header, resp.StatusCode)
		}
	}

	if _, err := os.Stat(filepath.Join(dir, "artifact")); !os.IsNotExist(err) {
		t.Error("a rejected upload created directories")
	}
}

func TestUploadWritesAndServesBack(t *testing.T) {
	srv, dir := newTestServer(t, true)
	body := payload(70000)

	req, _ := http.NewRequest("PUT", srv.URL+"/artifact/app/1.0.0/app.zip",
		bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+testToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", resp.StatusCode)
	}

	stored, err := os.ReadFile(filepath.Join(dir, "artifact", "app", "1.0.0", "app.zip"))
	if err != nil {
		t.Fatalf("stored file: %v", err)
	}
	if !bytes.Equal(stored, body) {
		t.Error("stored bytes differ from the upload")
	}

	entries, err := os.ReadDir(filepath.Join(dir, "artifact", "app", "1.0.0"))
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".tmp~") {
			t.Errorf("temporary file %s was left behind", entry.Name())
		}
	}

	fetched, err := http.Get(srv.URL + "/artifact/app/1.0.0/app.zip")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer fetched.Body.Close()
	got, _ := io.ReadAll(fetched.Body)
	if !bytes.Equal(got, body) {
		t.Error("served bytes differ from the upload")
	}
}

// A pointer is republished on every release, so overwriting has to be the
// normal case rather than an error.
func TestUploadOverwritesPointer(t *testing.T) {
	srv, dir := newTestServer(t, true)

	for _, content := range []string{"first", "second"} {
		req, _ := http.NewRequest("PUT", srv.URL+"/index/app/stable.pb",
			strings.NewReader(content))
		req.Header.Set("Authorization", "Bearer "+testToken)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("PUT: %v", err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("status = %d, want 201", resp.StatusCode)
		}
	}

	stored, err := os.ReadFile(filepath.Join(dir, "index", "app", "stable.pb"))
	if err != nil {
		t.Fatalf("stored file: %v", err)
	}
	if string(stored) != "second" {
		t.Errorf("pointer = %s, want the second write", stored)
	}
}

func TestUploadRejectsOversizedBody(t *testing.T) {
	srv, dir := newTestServer(t, true)

	req, _ := http.NewRequest("PUT", srv.URL+"/artifact/app/1.0.0/huge.zip",
		bytes.NewReader(payload(2<<20)))
	req.Header.Set("Authorization", "Bearer "+testToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %d, want 413", resp.StatusCode)
	}
	if _, err := os.Stat(filepath.Join(dir, "artifact", "app", "1.0.0", "huge.zip")); !os.IsNotExist(err) {
		t.Error("an oversized upload left a file behind")
	}
}

// TestUploadRefusesTraversal drives the handler directly. Going through an HTTP
// client would prove nothing: net/http normalizes "/../escaped.txt" to
// "/escaped.txt" before it leaves the client, so the server never sees the
// traversal. A hostile client is under no such obligation.
func TestUploadRefusesTraversal(t *testing.T) {
	cfg, dir := newTestConfig(t, true)
	handler := cfg.handler()

	outside := filepath.Join(filepath.Dir(dir), "escaped.txt")
	t.Cleanup(func() { os.Remove(outside) })

	for _, target := range []string{
		"/../escaped.txt",
		"/artifact/../../escaped.txt",
		"/./../escaped.txt",
	} {
		req := httptest.NewRequest("PUT", target, strings.NewReader("nope"))
		req.Header.Set("Authorization", "Bearer "+testToken)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code == http.StatusCreated {
			t.Errorf("PUT %s was accepted", target)
		}
		if _, err := os.Stat(outside); err == nil {
			t.Fatalf("PUT %s escaped the served directory", target)
		}
	}
}

func TestUploadScopedTokenIsolatesProducts(t *testing.T) {
	cfg, dir := newTestConfig(t, false)
	cfg.credentials = []credential{
		{hash: hashToken("app-token"), products: []string{"app"}},
		{hash: hashToken("other-token"), products: []string{"other"}},
		{hash: hashToken(testToken)},
	}
	srv := newLocalServer(t, cfg)
	t.Cleanup(srv.Close)

	put := func(token, path, body string) int {
		t.Helper()
		req, _ := http.NewRequest(http.MethodPut, srv.URL+path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("PUT %s: %v", path, err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}

	if code := put("app-token", "/index/app/stable.pb", "ok"); code != http.StatusCreated {
		t.Errorf("own index: status = %d, want 201", code)
	}
	if code := put("app-token", "/directory/app.pb", "ok"); code != http.StatusCreated {
		t.Errorf("own directory: status = %d, want 201", code)
	}
	if code := put("app-token", "/site/app.json", "{}"); code != http.StatusCreated {
		t.Errorf("own site: status = %d, want 201", code)
	}
	if code := put("app-token", "/index/other/stable.pb", "nope"); code != http.StatusForbidden {
		t.Errorf("other product: status = %d, want 403", code)
	}
	if code := put("app-token", "/index/app2/stable.pb", "nope"); code != http.StatusForbidden {
		t.Errorf("prefix cousin: status = %d, want 403", code)
	}
	if code := put("app-token", "/directory/other.pb", "nope"); code != http.StatusForbidden {
		t.Errorf("other directory: status = %d, want 403", code)
	}
	if code := put("app-token", "/probe.txt", "nope"); code != http.StatusForbidden {
		t.Errorf("unscoped path: status = %d, want 403", code)
	}
	if code := put("other-token", "/index/app/stable.pb", "nope"); code != http.StatusForbidden {
		t.Errorf("foreign token: status = %d, want 403", code)
	}
	if code := put("wrong", "/index/app/stable.pb", "nope"); code != http.StatusUnauthorized {
		t.Errorf("bad token: status = %d, want 401", code)
	}
	if code := put(testToken, "/probe.txt", "ops"); code != http.StatusCreated {
		t.Errorf("operator probe: status = %d, want 201", code)
	}

	if _, err := os.Stat(filepath.Join(dir, "index", "other")); !os.IsNotExist(err) {
		t.Error("a forbidden upload created another product's directory")
	}
}

func TestPublishPreflightAcceptsScopedToken(t *testing.T) {
	cfg, _ := newTestConfig(t, false)
	cfg.credentials = []credential{{hash: hashToken("app-token"), products: []string{"app"}}}
	srv := newLocalServer(t, cfg)
	t.Cleanup(srv.Close)

	req, _ := http.NewRequest(http.MethodPost, srv.URL+publishproto.PreflightPath, nil)
	req.Header.Set("Authorization", "Bearer app-token")
	req.Header.Set(publishproto.ProtocolHeader, strconv.Itoa(publishproto.Current))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("preflight status = %d, want 200", resp.StatusCode)
	}
}

func TestCleanKeyRejectsEscapes(t *testing.T) {
	rejected := []string{"/..", "/../x", "/a/../../x", "/\x00"}
	for _, in := range rejected {
		if key, ok := cleanKey(in); ok {
			t.Errorf("cleanKey(%q) = %q, want rejection", in, key)
		}
	}
	accepted := map[string]string{
		"/":                    ".",
		"":                     ".",
		"/index/app/stable.pb": "index/app/stable.pb",
		"/a/./b":               "a/b",
		"/a/b/../c":            "a/c",
		"/artifact/":           "artifact",
	}
	for in, want := range accepted {
		key, ok := cleanKey(in)
		if !ok || key != want {
			t.Errorf("cleanKey(%q) = (%q, %v), want (%q, true)", in, key, ok, want)
		}
	}
}

func TestHealthEndpoint(t *testing.T) {
	srv, _ := newTestServer(t, false)

	resp, err := http.Get(srv.URL + healthPath)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if got := resp.Header.Get("Content-Type"); got != "application/protobuf" {
		t.Errorf("Content-Type = %q", got)
	}
	var health rupv2.Health
	if err := proto.Unmarshal(body, &health); err != nil {
		t.Fatalf("health proto: %v", err)
	}
	if health.Status != "ok" {
		t.Errorf("health status = %q", health.Status)
	}
}
