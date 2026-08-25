package backends

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"cnb.cool/shichao402/relkit/internal/publishproto"
)

func testHTTPPutBackend(serverURL string) *httpPutBackend {
	return &httpPutBackend{
		pathStyleBackend: &pathStyleBackend{
			baseBackend: baseBackend{name: "upload", backendType: "http-put"},
			baseURL:     serverURL + "/",
		},
		uploadURL: serverURL + "/",
		tokenEnv:  "RELKIT_TEST_UPLOAD_TOKEN",
		timeout:   time.Second,
	}
}

func TestHTTPPutPreflightAndWritesAdvertisePublisher(t *testing.T) {
	t.Setenv("RELKIT_TEST_UPLOAD_TOKEN", "secret")
	oldVersion := publishproto.PublisherVersion
	publishproto.PublisherVersion = "0.2.0-test"
	t.Cleanup(func() { publishproto.PublisherVersion = oldVersion })

	var sawPreflight, sawPut bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer secret" {
			t.Errorf("Authorization = %q", got)
		}
		if got := r.Header.Get(publishproto.ProtocolHeader); got != "2" {
			t.Errorf("%s = %q", publishproto.ProtocolHeader, got)
		}
		if got := r.Header.Get(publishproto.VersionHeader); got != "0.2.0-test" {
			t.Errorf("%s = %q", publishproto.VersionHeader, got)
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == publishproto.PreflightPath:
			sawPreflight = true
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"ok":true,"protocol":2,"minProtocol":2}`)
		case r.Method == http.MethodPut && r.URL.Path == "/index/app/stable.pb":
			sawPut = true
			w.WriteHeader(http.StatusCreated)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	backend := testHTTPPutBackend(srv.URL)
	if err := backend.Preflight(); err != nil {
		t.Fatalf("Preflight: %v", err)
	}
	if _, err := backend.PutPointer([]byte("index"), "index/app/stable.pb"); err != nil {
		t.Fatalf("PutPointer: %v", err)
	}
	if !sawPreflight || !sawPut {
		t.Fatalf("sawPreflight=%v sawPut=%v", sawPreflight, sawPut)
	}
}

func TestHTTPPutPreflightReportsRequiredUpgrade(t *testing.T) {
	t.Setenv("RELKIT_TEST_UPLOAD_TOKEN", "secret")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUpgradeRequired)
		io.WriteString(w, `{"error":"publisher_upgrade_required","minProtocol":3}`)
	}))
	defer srv.Close()

	err := testHTTPPutBackend(srv.URL).Preflight()
	if err == nil || !strings.Contains(err.Error(), "publish protocol 3 is required") ||
		!strings.Contains(err.Error(), "upgrade relkit") {
		t.Fatalf("Preflight error = %v", err)
	}
}

func TestHTTPPutPreflightAllowsLegacyGenericEndpoint(t *testing.T) {
	t.Setenv("RELKIT_TEST_UPLOAD_TOKEN", "secret")
	for _, status := range []int{http.StatusNotFound, http.StatusMethodNotAllowed} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
		}))
		err := testHTTPPutBackend(srv.URL).Preflight()
		srv.Close()
		if err != nil {
			t.Errorf("status %d: %v", status, err)
		}
	}
}
