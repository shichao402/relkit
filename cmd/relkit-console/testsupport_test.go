package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	rupv2 "go.firoyang.com/relkit/api/rup/v2"
)

// Panel tests share a cookie jar keyed by server URL, same trick as serve's
// tests: login once per server, then plain http.Get carries the session.
var testPanelCookies sync.Map

func attachTestPanelCookie(req *http.Request) {
	if req.URL == nil {
		return
	}
	base := req.URL.Scheme + "://" + req.URL.Host
	if v, ok := testPanelCookies.Load(base); ok {
		req.AddCookie(v.(*http.Cookie))
	}
}

func rememberPanelCookie(tb testing.TB, srv *httptest.Server, c *console) {
	tb.Helper()
	if c == nil || c.admin == nil {
		return
	}
	user := c.admin.firstUsername()
	if user == "" {
		return
	}
	cookie, err := c.admin.issueCookie(user, time.Now().Add(time.Hour))
	if err != nil {
		tb.Fatal(err)
	}
	testPanelCookies.Store(srv.URL, cookie)
	tb.Cleanup(func() { testPanelCookies.Delete(srv.URL) })
}

func testGet(t *testing.T, rawURL string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	attachTestPanelCookie(req)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

// newTestServer assembles a console server over a temp tree and logs the
// panel in, so plain GETs reach the panel pages.
func newTestServer(t *testing.T, _ bool) (*httptest.Server, string) {
	t.Helper()
	c, dir := newTestConsole(t)
	srv := newLocalServer(t, c)
	rememberPanelCookie(t, srv, c)
	return srv, dir
}

func mustEnvelope(t *testing.T, index *rupv2.Index) []byte {
	t.Helper()
	payload, err := rupv2.MarshalIndex(index)
	if err != nil {
		t.Fatal(err)
	}
	env := &rupv2.Envelope{
		Schema:  rupv2.SchemaEnvelope,
		Payload: payload,
		Signatures: []*rupv2.Signature{
			{KeyId: "test", Alg: "ed25519", Sig: make([]byte, 64)},
		},
	}
	raw, err := rupv2.MarshalEnvelope(env)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func indexDoc(product, channel, version string, code int, manifestURL string) *rupv2.Index {
	return &rupv2.Index{
		Schema:      rupv2.SchemaIndex,
		Product:     product,
		Channel:     channel,
		Sequence:    int64(code),
		GeneratedAt: "2026-08-01T00:00:00Z",
		Versions: []*rupv2.VersionNode{
			{
				Version: version,
				Code:    int64(code),
				Manifest: &rupv2.DigestRef{
					Sha256: strings.Repeat("a", 64),
					Size:   1,
					Urls:   []string{manifestURL},
				},
			},
		},
	}
}

func manifestDoc(product, version string, code int, artifactURL string) []byte {
	raw, _ := rupv2.MarshalManifest(&rupv2.Manifest{
		Schema:  rupv2.SchemaManifest,
		Product: product,
		Version: version,
		Code:    int64(code),
		Artifacts: []*rupv2.Artifact{
			{
				Id:       "app",
				Filename: "app.zip",
				Size:     3,
				Sha256:   strings.Repeat("b", 64),
				Kind:     rupv2.ArtifactKind_ARTIFACT_KIND_ARCHIVE,
				Urls:     []string{artifactURL},
			},
		},
	})
	return raw
}

func writeRelease(t *testing.T, dir, product, channel, version string, code int) {
	t.Helper()
	base := "http://example.com"
	manURL := fmt.Sprintf("%s/manifest/%s/%s.pb", base, product, version)
	artURL := fmt.Sprintf("%s/artifact/%s/%s/app.zip", base, product, version)
	writeFile(t, dir, fmt.Sprintf("manifest/%s/%s.pb", product, version), manifestDoc(product, version, code, artURL))
	writeFile(t, dir, fmt.Sprintf("artifact/%s/%s/app.zip", product, version), []byte("pkg"))
	writeFile(t, dir, fmt.Sprintf("index/%s/%s.pb", product, channel),
		mustEnvelope(t, indexDoc(product, channel, version, code, manURL)))
}

var _ = os.Stat
var _ = filepath.Join
