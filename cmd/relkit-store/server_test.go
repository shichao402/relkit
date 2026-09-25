package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	rupv2 "go.firoyang.com/relkit/api/rup/v2"
	"go.firoyang.com/relkit/internal/publishproto"
	"google.golang.org/protobuf/proto"
)

// The storage-plane half of serve's server_test.go, adapted: the store has no
// panel login, so every request here is anonymous or Bearer-token'd exactly
// like real publishers and clients talk to it.

func TestPublishProtocolIsEnforcedBeforeWrite(t *testing.T) {
	cfg, dir := newTestConfig(t, true)
	cfg.minPublishProtocol = publishproto.Current
	srv := newLocalServer(t, cfg)

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

	req, _ := http.NewRequest(http.MethodPost, srv.URL+publishproto.PreflightPath, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("preflight without token = %d, want 401", resp.StatusCode)
	}
	if !strings.Contains(string(body), "unauthorized") {
		t.Fatalf("preflight body: %s", body)
	}

	req, _ = http.NewRequest(http.MethodPost, srv.URL+publishproto.PreflightPath, nil)
	req.Header.Set("Authorization", "Bearer "+testToken)
	req.Header.Set(publishproto.ProtocolHeader, strconv.Itoa(publishproto.Current))
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("preflight with token = %d, want 200", resp2.StatusCode)
	}
}

func TestRangeRequestServesExactSlice(t *testing.T) {
	cfg, dir := newTestConfig(t, false)
	srv := newLocalServer(t, cfg)
	writeFile(t, dir, "artifact/app/1.0.0/app.zip", []byte("0123456789"))

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/artifact/app/1.0.0/app.zip", nil)
	req.Header.Set("Range", "bytes=2-5")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusPartialContent {
		t.Fatalf("range = %d, want 206", resp.StatusCode)
	}
	if string(body) != "2345" {
		t.Fatalf("range body = %q, want 2345", body)
	}
}

func TestParallelRangeDownload(t *testing.T) {
	cfg, dir := newTestConfig(t, false)
	srv := newLocalServer(t, cfg)
	payload := make([]byte, 1<<20)
	for i := range payload {
		payload[i] = byte(i)
	}
	writeFile(t, dir, "artifact/app/1.0.0/app.zip", payload)

	const workers = 4
	errc := make(chan error, workers)
	for i := 0; i < workers; i++ {
		go func(i int) {
			lo := i * (len(payload) / workers)
			hi := lo + len(payload)/workers - 1
			req, _ := http.NewRequest(http.MethodGet, srv.URL+"/artifact/app/1.0.0/app.zip", nil)
			req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", lo, hi))
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				errc <- err
				return
			}
			defer resp.Body.Close()
			got, err := io.ReadAll(resp.Body)
			if err != nil {
				errc <- err
				return
			}
			if !bytes.Equal(got, payload[lo:lo+len(payload)/workers]) {
				errc <- fmt.Errorf("slice %d mismatch", i)
				return
			}
			errc <- nil
		}(i)
	}
	for i := 0; i < workers; i++ {
		if err := <-errc; err != nil {
			t.Fatal(err)
		}
	}
}

func TestHeadReportsSizeWithoutBody(t *testing.T) {
	cfg, dir := newTestConfig(t, false)
	srv := newLocalServer(t, cfg)
	writeFile(t, dir, "artifact/app/1.0.0/app.zip", []byte("0123456789"))

	resp, err := http.Head(srv.URL + "/artifact/app/1.0.0/app.zip")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("HEAD = %d", resp.StatusCode)
	}
	if resp.ContentLength != 10 {
		t.Fatalf("Content-Length = %d, want 10", resp.ContentLength)
	}
	body, _ := io.ReadAll(resp.Body)
	if len(body) != 0 {
		t.Fatalf("HEAD returned %d bytes", len(body))
	}
}

func TestCacheHeadersDistinguishMutableFromImmutable(t *testing.T) {
	cfg, dir := newTestConfig(t, false)
	srv := newLocalServer(t, cfg)
	writeFile(t, dir, "index/app/stable.pb", []byte("x"))
	writeFile(t, dir, "manifest/app/1.0.0.pb", []byte("x"))

	for path, want := range map[string]string{
		"/index/app/stable.pb":   "no-cache, must-revalidate",
		"/manifest/app/1.0.0.pb": "public, max-age=31536000, immutable",
	} {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if got := resp.Header.Get("Cache-Control"); got != want {
			t.Errorf("%s Cache-Control = %q, want %q", path, got, want)
		}
	}
}

func TestRejectsTraversal(t *testing.T) {
	cfg, dir := newTestConfig(t, false)
	srv := newLocalServer(t, cfg)
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
	cfg, dir := newTestConfig(t, false)
	srv := newLocalServer(t, cfg)
	writeFile(t, dir, "artifact/app/1.0.0/app.zip", []byte("inside"))
	// An index that does not decode leaves no product to show, so the root
	// stays the catalog stub. That is also the plain-static-host case.
	writeFile(t, dir, "index/app/stable.pb", []byte("x"))

	// The storage root is the catalog stub page, not a listing: the human
	// landing page moved to console, and protocol clients never read this.
	root := getBody(t, srv.URL+"/")
	if !strings.Contains(root, "No published catalog yet") {
		t.Errorf("GET / should be the catalog stub\n%s", root)
	}

	for _, listing := range []string{"/artifact/", "/index/"} {
		body := getBody(t, srv.URL+listing)
		for _, want := range []string{
			`<th>Name</th><th class="num">Size</th><th class="num">Modified</th>`,
		} {
			if !strings.Contains(body, want) {
				t.Errorf("GET %s body missing %q\n%s", listing, want, body)
			}
		}
		if !regexp.MustCompile(`class="num">\d{4}-\d{2}-\d{2} \d{2}:\d{2}<`).MatchString(body) {
			t.Errorf("GET %s body missing formatted mtime\n%s", listing, body)
		}
	}

	artifactBody := getBody(t, srv.URL+"/artifact/")
	if !strings.Contains(artifactBody, `href="app/"`) {
		t.Errorf("GET /artifact/ missing app/\n%s", artifactBody)
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
	cfg, _ := newTestConfig(t, false)
	srv := newLocalServer(t, cfg)

	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/index/app/stable.pb", strings.NewReader("x"))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("put = %d, want 405", resp.StatusCode)
	}
}

func TestUploadRequiresCorrectToken(t *testing.T) {
	cfg, _ := newTestConfig(t, true)
	srv := newLocalServer(t, cfg)

	for _, auth := range []string{"", "Bearer wrong", "Basic " + testToken} {
		req, _ := http.NewRequest(http.MethodPut, srv.URL+"/index/app/stable.pb", strings.NewReader("x"))
		if auth != "" {
			req.Header.Set("Authorization", auth)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("auth %q: put = %d, want 401", auth, resp.StatusCode)
		}
	}
}

func TestUploadWritesAndServesBack(t *testing.T) {
	cfg, _ := newTestConfig(t, true)
	srv := newLocalServer(t, cfg)

	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/index/app/stable.pb", strings.NewReader("body"))
	req.Header.Set("Authorization", "Bearer "+testToken)
	req.Header.Set(publishproto.ProtocolHeader, fmt.Sprintf("%d", publishproto.Current))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("put = %d, want 201", resp.StatusCode)
	}

	body := getBody(t, srv.URL+"/index/app/stable.pb")
	if body != "body" {
		t.Fatalf("served body = %q, want body", body)
	}
}

func TestUploadRejectsOversizedBody(t *testing.T) {
	cfg, _ := newTestConfig(t, true)
	srv := newLocalServer(t, cfg)

	big := make([]byte, cfg.maxUpload+1)
	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/artifact/app/1.0.0/app.zip", bytes.NewReader(big))
	req.Header.Set("Authorization", "Bearer "+testToken)
	req.Header.Set(publishproto.ProtocolHeader, fmt.Sprintf("%d", publishproto.Current))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("put = %d, want 413", resp.StatusCode)
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
		req := httptest.NewRequest(http.MethodPut, target, strings.NewReader("nope"))
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
	dir := t.TempDir()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
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
		casSecret:     bytes.Repeat([]byte{7}, 32),
		gc:            newGCState(false, 0, defaultGCDebounce, defaultCASGrace),
		credentials: []credential{{
			hash:     hashToken(testToken),
			products: []string{"app"},
		}},
	}
	t.Cleanup(cfg.stats.stop)
	srv := newLocalServer(t, cfg)

	put := func(path string) int {
		req, _ := http.NewRequest(http.MethodPut, srv.URL+path, strings.NewReader("x"))
		req.Header.Set("Authorization", "Bearer "+testToken)
		req.Header.Set(publishproto.ProtocolHeader, fmt.Sprintf("%d", publishproto.Current))
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}

	if code := put("/index/app/stable.pb"); code != http.StatusCreated {
		t.Fatalf("own tree put = %d, want 201", code)
	}
	if code := put("/index/other/stable.pb"); code != http.StatusForbidden {
		t.Fatalf("other tree put = %d, want 403", code)
	}
}

func TestCleanKeyRejectsEscapes(t *testing.T) {
	for _, tc := range []struct {
		path string
		ok   bool
	}{
		{"/", true},
		{"/index/app/stable.pb", true},
		{"/../escape", false},
		{"/index/../../escape", false},
		{"/a\x00b", false},
	} {
		if _, ok := cleanKey(tc.path); ok != tc.ok {
			t.Errorf("cleanKey(%q) ok = %v, want %v", tc.path, ok, tc.ok)
		}
	}
}

func TestHealthEndpoint(t *testing.T) {
	cfg, _ := newTestConfig(t, false)
	srv := newLocalServer(t, cfg)

	resp, err := http.Get(srv.URL + "/-/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	health := &rupv2.Health{}
	if err := proto.Unmarshal(body, health); err != nil {
		t.Fatalf("unmarshal health: %v", err)
	}
	if health.Status != "ok" {
		t.Fatalf("health status = %q", health.Status)
	}
}

func TestListJSONEndpoint(t *testing.T) {
	cfg, dir := newTestConfig(t, false)
	srv := newLocalServer(t, cfg)
	writeFile(t, dir, "index/app/stable.pb", []byte("x"))
	writeFile(t, dir, "artifact/app/1.0.0/app.zip", []byte("pkg"))
	// Reserved files exist on every real box; the listing must not mention
	// them even though they sit in the same directory.
	writeFile(t, dir, ".relkit-serve-admin.json", []byte(`{}`))
	writeFile(t, dir, ".relkit-serve-stats.json", []byte(`{}`))
	writeFile(t, dir, ".relkit-serve-cas.key", []byte("k"))

	// Root listing maps /-/list/ to "." and hides the reserved files.
	root := getBody(t, srv.URL+"/-/list/")
	var rootRows []listEntryJSON
	if err := json.Unmarshal([]byte(root), &rootRows); err != nil {
		t.Fatalf("root payload not JSON: %v\n%s", err, root)
	}
	gotRoot := map[string]bool{}
	for _, row := range rootRows {
		gotRoot[row.Name] = true
		if row.IsDir != (row.Name == "index" || row.Name == "artifact") {
			t.Errorf("%s: isDir = %v", row.Name, row.IsDir)
		}
	}
	for _, want := range []string{"index", "artifact"} {
		if !gotRoot[want] {
			t.Errorf("root listing missing %q: %+v", want, rootRows)
		}
	}
	for _, hidden := range []string{
		".relkit-serve-admin.json", ".relkit-serve-stats.json", ".relkit-serve-cas.key",
	} {
		if gotRoot[hidden] {
			t.Errorf("root listing leaked %q", hidden)
		}
	}

	// Subdirectory listing carries size and RFC3339 mtime.
	sub := getBody(t, srv.URL+"/-/list/index/app")
	var rows []listEntryJSON
	if err := json.Unmarshal([]byte(sub), &rows); err != nil {
		t.Fatalf("sub payload not JSON: %v\n%s", err, sub)
	}
	if len(rows) != 1 || rows[0].Name != "stable.pb" || rows[0].Size != 1 {
		t.Fatalf("rows = %+v", rows)
	}
	if _, err := time.Parse(time.RFC3339, rows[0].Mtime); err != nil {
		t.Errorf("mtime %q is not RFC3339: %v", rows[0].Mtime, err)
	}
}

func TestListJSONRejectsBadPathsAndMethods(t *testing.T) {
	cfg, dir := newTestConfig(t, false)
	srv := newLocalServer(t, cfg)
	writeFile(t, dir, "index/app/stable.pb", []byte("x"))

	// Files are not listable, only directories.
	resp, err := http.Get(srv.URL + "/-/list/index/app/stable.pb")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("file listing = %d, want 404", resp.StatusCode)
	}

	// Traversal is rejected the same way the GET tree rejects it.
	resp, err = http.Get(srv.URL + "/-/list/../outside")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("traversal = %d, want 404", resp.StatusCode)
	}

	// Missing directory is a plain 404.
	resp, err = http.Get(srv.URL + "/-/list/nope")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("missing dir = %d, want 404", resp.StatusCode)
	}

	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/-/list/index", strings.NewReader("x"))
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("PUT = %d, want 405", resp.StatusCode)
	}
}
