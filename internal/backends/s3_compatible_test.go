package backends

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestS3CompatiblePutGetRoundTrip(t *testing.T) {
	store := t.TempDir()
	accessKey := "AKIATEST"
	secretKey := "secret-test-key"
	server := httptest.NewServer(newFakeS3(t, store, accessKey, secretKey, "us-east-1"))
	defer server.Close()

	t.Setenv("COS_SECRET_ID", accessKey)
	t.Setenv("COS_SECRET_KEY", secretKey)

	backend, err := newS3CompatibleBackend("cos", map[string]any{
		"type":           "s3-compatible",
		"endpoint":       server.URL,
		"bucket":         "release",
		"prefix":         "rup/",
		"baseUrl":        server.URL + "/release/rup/",
		"accessKeyEnv":   "COS_SECRET_ID",
		"secretKeyEnv":   "COS_SECRET_KEY",
		"region":         "us-east-1",
		"forcePathStyle": true,
	}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	artifact := filepath.Join(t.TempDir(), "app.zip")
	if err := os.WriteFile(artifact, bytes.Repeat([]byte("x"), 128), 0o644); err != nil {
		t.Fatal(err)
	}
	urls, err := backend.PutArtifact(artifact, "artifact/demo/1.0.0/app.zip")
	if err != nil {
		t.Fatal(err)
	}
	if len(urls) != 1 || !strings.HasSuffix(urls[0], "artifact/demo/1.0.0/app.zip") {
		t.Fatalf("unexpected artifact urls: %v", urls)
	}

	pointer := []byte("pointer-bytes")
	if _, err := backend.PutPointer(pointer, "index/demo/stable.pb"); err != nil {
		t.Fatal(err)
	}
	got, err := backend.Get("index/demo/stable.pb")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, pointer) {
		t.Fatalf("get mismatch: got %q want %q", got, pointer)
	}

	missing, err := backend.Get("index/demo/missing.pb")
	if err != nil {
		t.Fatal(err)
	}
	if missing != nil {
		t.Fatalf("missing key should return nil, got %q", missing)
	}
}

func TestS3CompatibleRequiresCredentials(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
	}))
	defer server.Close()

	t.Setenv("COS_SECRET_ID", "")
	t.Setenv("COS_SECRET_KEY", "")

	backend, err := newS3CompatibleBackend("cos", map[string]any{
		"baseUrl":        server.URL + "/",
		"endpoint":       server.URL,
		"bucket":         "release",
		"accessKeyEnv":   "COS_SECRET_ID",
		"secretKeyEnv":   "COS_SECRET_KEY",
		"region":         "us-east-1",
		"forcePathStyle": true,
	}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_, err = backend.PutImmutable([]byte("x"), "index/a.pb")
	if err == nil || !strings.Contains(err.Error(), "COS_SECRET_ID") {
		t.Fatalf("expected missing access key error, got %v", err)
	}
}

func TestS3CompatibleDerivesCOSRegion(t *testing.T) {
	backend, err := newS3CompatibleBackend("cos", map[string]any{
		"baseUrl":      "https://updates.example.com/",
		"endpoint":     "https://cos.ap-guangzhou.myqcloud.com",
		"bucket":       "myapp-1250000000",
		"accessKeyEnv": "COS_SECRET_ID",
		"secretKeyEnv": "COS_SECRET_KEY",
	}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s3 := backend.(*s3CompatibleBackend)
	if s3.region != "ap-guangzhou" {
		t.Fatalf("region=%q", s3.region)
	}
	if s3.forcePathStyle {
		t.Fatal("COS endpoint should default to virtual-hosted style")
	}
	rawURL, err := s3.objectURL("rup/index/app/stable.pb")
	if err != nil {
		t.Fatal(err)
	}
	wantHost := "myapp-1250000000.cos.ap-guangzhou.myqcloud.com"
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Host != wantHost || parsed.Path != "/rup/index/app/stable.pb" {
		t.Fatalf("object URL = %s", rawURL)
	}
}

func TestSignAWSV4StableShape(t *testing.T) {
	req, err := http.NewRequest(http.MethodPut, "https://bucket.example/key", bytes.NewReader([]byte("hi")))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	now := time.Date(2026, 8, 11, 8, 0, 0, 0, time.UTC)
	if err := signAWSV4(req, hashSHA256Hex([]byte("hi")), "us-east-1", "s3", "AKID", "SECRET", now); err != nil {
		t.Fatal(err)
	}
	if req.Header.Get("X-Amz-Date") != "20260811T080000Z" {
		t.Fatalf("X-Amz-Date=%q", req.Header.Get("X-Amz-Date"))
	}
	auth := req.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "AWS4-HMAC-SHA256 Credential=AKID/20260811/us-east-1/s3/aws4_request") {
		t.Fatalf("Authorization=%q", auth)
	}
	if !strings.Contains(auth, "SignedHeaders=") || !strings.Contains(auth, "Signature=") {
		t.Fatalf("Authorization missing parts: %q", auth)
	}
}

type fakeS3 struct {
	t         *testing.T
	dir       string
	accessKey string
	secretKey string
	region    string
}

func newFakeS3(t *testing.T, dir string, accessKey string, secretKey string, region string) http.Handler {
	return &fakeS3{t: t, dir: dir, accessKey: accessKey, secretKey: secretKey, region: region}
}

func (f *fakeS3) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	needsAuth := r.Method == http.MethodPut || r.Header.Get("Authorization") != ""
	if needsAuth {
		if err := f.verifySigV4(r); err != nil {
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}
	}
	path := strings.TrimPrefix(r.URL.Path, "/")
	if path == "" || strings.Contains(path, "..") {
		http.Error(w, "bad path", http.StatusBadRequest)
		return
	}
	target := filepath.Join(append([]string{f.dir}, strings.Split(path, "/")...)...)
	switch r.Method {
	case http.MethodPut:
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			f.t.Fatalf("mkdir: %v", err)
		}
		data, err := io.ReadAll(r.Body)
		if err != nil {
			f.t.Fatalf("read body: %v", err)
		}
		if err := os.WriteFile(target, data, 0o644); err != nil {
			f.t.Fatalf("write: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	case http.MethodGet, http.MethodHead:
		info, err := os.Stat(target)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodHead {
			w.Header().Set("Content-Length", strconv.FormatInt(info.Size(), 10))
			w.WriteHeader(http.StatusOK)
			return
		}
		http.ServeFile(w, r, target)
	default:
		http.Error(w, "method", http.StatusMethodNotAllowed)
	}
}

func (f *fakeS3) verifySigV4(r *http.Request) error {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, sigv4Algorithm+" ") {
		return errString("missing sigv4")
	}
	amzDate := r.Header.Get("X-Amz-Date")
	payloadHash := r.Header.Get("X-Amz-Content-Sha256")
	if amzDate == "" || payloadHash == "" {
		return errString("missing amz headers")
	}
	now, err := time.Parse("20060102T150405Z", amzDate)
	if err != nil {
		return err
	}
	signedHeaders := signedHeadersFromAuth(auth)
	if signedHeaders == "" {
		return errString("missing SignedHeaders")
	}

	// Transport may add unsigned headers (e.g. Accept-Encoding). Rebuild a
	// request that only carries the headers the client actually signed.
	clone := r.Clone(r.Context())
	if clone.URL.Host == "" {
		clone.URL.Host = r.Host
		clone.URL.Scheme = "http"
	}
	clone.Header = make(http.Header)
	for _, name := range strings.Split(signedHeaders, ";") {
		if name == "host" {
			clone.Header.Set("Host", r.Host)
			continue
		}
		values := r.Header.Values(name)
		if len(values) == 0 {
			return errString("missing signed header " + name)
		}
		for _, value := range values {
			clone.Header.Add(name, value)
		}
	}
	if r.Body != nil && r.Method != http.MethodGet && r.Method != http.MethodHead {
		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			return err
		}
		r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		clone.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		clone.ContentLength = int64(len(bodyBytes))
	}
	if err := signAWSV4(clone, payloadHash, f.region, "s3", f.accessKey, f.secretKey, now); err != nil {
		return err
	}
	if clone.Header.Get("Authorization") != auth {
		return errString("bad signature")
	}
	return nil
}

func signedHeadersFromAuth(auth string) string {
	const marker = "SignedHeaders="
	idx := strings.Index(auth, marker)
	if idx < 0 {
		return ""
	}
	rest := auth[idx+len(marker):]
	end := strings.Index(rest, ",")
	if end < 0 {
		return strings.TrimSpace(rest)
	}
	return strings.TrimSpace(rest[:end])
}

type errString string

func (e errString) Error() string { return string(e) }

func TestDualS3BackendsSamePointerBytes(t *testing.T) {
	accessKey := "AKIATEST"
	secretKey := "secret-test-key"
	t.Setenv("COS_SECRET_ID", accessKey)
	t.Setenv("COS_SECRET_KEY", secretKey)

	s1 := httptest.NewServer(newFakeS3(t, t.TempDir(), accessKey, secretKey, "us-east-1"))
	defer s1.Close()
	s2 := httptest.NewServer(newFakeS3(t, t.TempDir(), accessKey, secretKey, "us-west-1"))
	defer s2.Close()

	a, err := newS3CompatibleBackend("cos", map[string]any{
		"type": "s3-compatible", "endpoint": s1.URL, "bucket": "one", "prefix": "rup/",
		"baseUrl": s1.URL + "/one/rup/", "accessKeyEnv": "COS_SECRET_ID", "secretKeyEnv": "COS_SECRET_KEY",
		"region": "us-east-1", "forcePathStyle": true,
	}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	b, err := newS3CompatibleBackend("cos2", map[string]any{
		"type": "s3-compatible", "endpoint": s2.URL, "bucket": "two", "prefix": "rup/",
		"baseUrl": s2.URL + "/two/rup/", "accessKeyEnv": "COS_SECRET_ID", "secretKeyEnv": "COS_SECRET_KEY",
		"region": "us-west-1", "forcePathStyle": true,
	}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte("directory-pb-bytes")
	if _, err := a.PutPointer(payload, "directory/demo.pb"); err != nil {
		t.Fatal(err)
	}
	if _, err := b.PutPointer(payload, "directory/demo.pb"); err != nil {
		t.Fatal(err)
	}
	ga, err := a.Get("directory/demo.pb")
	if err != nil {
		t.Fatal(err)
	}
	gb, err := b.Get("directory/demo.pb")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(ga, gb) || !bytes.Equal(ga, payload) {
		t.Fatalf("mismatch %q %q", ga, gb)
	}
}
