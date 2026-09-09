package backends

import (
	"bytes"
	"encoding/hex"
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

	"firoyang.com/relkit/internal/model"
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

func TestPutArtifactCASS3SkipsSecondUpload(t *testing.T) {
	store := t.TempDir()
	accessKey := "AKIATEST"
	secretKey := "secret-test-key"
	fake := newFakeS3(t, store, accessKey, secretKey, "us-east-1")
	server := httptest.NewServer(fake)
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

	payload := bytes.Repeat([]byte("y"), 256)
	src := filepath.Join(t.TempDir(), "app.zip")
	if err := os.WriteFile(src, payload, 0o644); err != nil {
		t.Fatal(err)
	}
	digest, size, err := model.Sha256File(src)
	if err != nil {
		t.Fatal(err)
	}

	if _, skipped, err := PutArtifactCAS(backend, src, "artifact/demo/1.0.0/app.zip", digest, size); err != nil {
		t.Fatal(err)
	} else if skipped {
		t.Fatal("first put must upload")
	}
	firstBytes := fake.bytesUploaded
	if firstBytes != size {
		t.Fatalf("uploaded %d, want %d", firstBytes, size)
	}

	if err := os.Remove(src); err != nil {
		t.Fatal(err)
	}
	if _, skipped, err := PutArtifactCAS(backend, src, "artifact/demo/2.0.0/app.zip", digest, size); err != nil {
		t.Fatal(err)
	} else if !skipped {
		t.Fatal("second put must skip upload")
	}
	if fake.bytesUploaded != firstBytes {
		t.Fatalf("cas hit still uploaded %d extra bytes", fake.bytesUploaded-firstBytes)
	}

	got, err := backend.Get("artifact/demo/2.0.0/app.zip")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatal("promoted object mismatch")
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

func TestPresignS3RequestStableShape(t *testing.T) {
	req, err := http.NewRequest(http.MethodPut, "https://bucket.example/cas/abc", nil)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 7, 8, 0, 0, 0, time.UTC)
	if err := PresignS3Request(req, "ap-guangzhou", "AKID", "SECRET", now, time.Hour); err != nil {
		t.Fatal(err)
	}
	query := req.URL.Query()
	if query.Get("X-Amz-Algorithm") != "AWS4-HMAC-SHA256" ||
		query.Get("X-Amz-Credential") != "AKID/20260907/ap-guangzhou/s3/aws4_request" ||
		query.Get("X-Amz-Date") != "20260907T080000Z" ||
		query.Get("X-Amz-Expires") != "3600" ||
		query.Get("X-Amz-SignedHeaders") != "host" ||
		len(query.Get("X-Amz-Signature")) != 64 {
		t.Fatalf("unexpected presign query: %s", req.URL.RawQuery)
	}
	if req.Header.Get("Authorization") != "" {
		t.Fatal("presigned request must not carry Authorization header")
	}
}

func TestS3AuthorizeCASUploadUsesUnsignedPayload(t *testing.T) {
	t.Setenv("TEST_S3_ACCESS", "AKID")
	t.Setenv("TEST_S3_SECRET", "SECRET")
	backendAny, err := newS3CompatibleBackend("cos", map[string]any{
		"type": "s3-compatible", "baseUrl": "https://download.example/rup/",
		"endpoint": "https://cos.ap-guangzhou.myqcloud.com", "bucket": "bucket-1251882798",
		"accessKeyEnv": "TEST_S3_ACCESS", "secretKeyEnv": "TEST_S3_SECRET",
		"region": "ap-guangzhou", "casCredentials": "presign",
	}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	backend := backendAny.(*s3CompatibleBackend)
	digest := strings.Repeat("a", 64)
	upload, err := backend.AuthorizeCASUpload(CASUploadRequest{
		Key:  "cas/" + digest,
		Size: 123,
		TTL:  time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	if upload.Requests[0].Headers["X-Amz-Content-Sha256"] != unsignedPayload {
		t.Fatalf("headers=%v", upload.Requests[0].Headers)
	}
	parsed, err := url.Parse(upload.Requests[0].URL)
	if err != nil {
		t.Fatal(err)
	}
	signedHeaders := parsed.Query().Get("X-Amz-SignedHeaders")
	if !strings.Contains(signedHeaders, "x-amz-content-sha256") || !strings.Contains(signedHeaders, "content-type") {
		t.Fatalf("signed headers=%q", signedHeaders)
	}
	got := parsed.Query().Get("X-Amz-Signature")
	want := cosQueryAuthSignature(http.MethodPut, parsed, upload.Requests[0].Headers, "ap-guangzhou", "SECRET")
	if got != want {
		t.Fatalf("presign does not match COS query-auth: got %s want %s", got, want)
	}

	wrong := map[string]string{
		"Content-Type":         upload.Requests[0].Headers["Content-Type"],
		"X-Amz-Content-Sha256": digest,
	}
	if cosQueryAuthSignature(http.MethodPut, parsed, wrong, "ap-guangzhou", "SECRET") == got {
		t.Fatal("COS would accept a PUT that binds the object sha256 as x-amz-content-sha256")
	}
}

func TestS3IgnoresCasCredentialsField(t *testing.T) {
	t.Setenv("TEST_S3_ACCESS", "AKID")
	t.Setenv("TEST_S3_SECRET", "SECRET")
	backendAny, err := newS3CompatibleBackend("cos", map[string]any{
		"type": "s3-compatible", "baseUrl": "https://download.example/rup/",
		"endpoint": "https://cos.ap-guangzhou.myqcloud.com", "bucket": "bucket-1251882798",
		"accessKeyEnv": "TEST_S3_ACCESS", "secretKeyEnv": "TEST_S3_SECRET",
		"region": "ap-guangzhou", "casCredentials": "sts",
	}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	backend := backendAny.(*s3CompatibleBackend)
	digest := strings.Repeat("a", 64)
	upload, err := backend.AuthorizeCASUpload(CASUploadRequest{Key: "cas/" + digest, Size: 1, TTL: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(upload.Requests[0].URL, "X-Amz-Signature") {
		t.Fatalf("expected presigned URL, got %s", upload.Requests[0].URL)
	}
}

func headerValueCI(headers map[string]string, name string) string {
	for key, value := range headers {
		if strings.EqualFold(key, name) {
			return value
		}
	}
	return ""
}

// cosQueryAuthSignature rebuilds a Tencent COS query-auth signature: signed
// headers come from the PUT, HashedPayload is always UNSIGNED-PAYLOAD.
func cosQueryAuthSignature(method string, u *url.URL, headers map[string]string, region, secretKey string) string {
	query := u.Query()
	amzDate := query.Get("X-Amz-Date")
	signedHeaders := query.Get("X-Amz-SignedHeaders")
	query.Del("X-Amz-Signature")

	signed := make(http.Header)
	for _, name := range strings.Split(signedHeaders, ";") {
		if name == "host" {
			signed.Set("Host", u.Host)
			continue
		}
		if value := headerValueCI(headers, name); value != "" {
			signed.Set(name, value)
		}
	}
	_, canonicalHeaders := canonicalHeaderBlock(signed)
	canonicalRequest := strings.Join([]string{
		method,
		canonicalURI(u),
		canonicalQuery(query),
		canonicalHeaders,
		signedHeaders,
		unsignedPayload,
	}, "\n")
	dateStamp := amzDate[:8]
	credentialScope := strings.Join([]string{dateStamp, region, "s3", "aws4_request"}, "/")
	stringToSign := strings.Join([]string{
		sigv4Algorithm,
		amzDate,
		credentialScope,
		hashSHA256Hex([]byte(canonicalRequest)),
	}, "\n")
	return hex.EncodeToString(hmacSHA256(deriveSigningKey(secretKey, dateStamp, region, "s3"), stringToSign))
}

type fakeS3 struct {
	t             *testing.T
	dir           string
	accessKey     string
	secretKey     string
	region        string
	bytesUploaded int64
}

func newFakeS3(t *testing.T, dir string, accessKey string, secretKey string, region string) *fakeS3 {
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
		if src := r.Header.Get("x-amz-copy-source"); src != "" {
			decoded, err := url.PathUnescape(src)
			if err != nil {
				http.Error(w, "bad copy-source", http.StatusBadRequest)
				return
			}
			decoded = strings.TrimPrefix(decoded, "/")
			source := filepath.Join(append([]string{f.dir}, strings.Split(decoded, "/")...)...)
			data, err := os.ReadFile(source)
			if err != nil {
				http.NotFound(w, r)
				return
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				f.t.Fatalf("mkdir: %v", err)
			}
			if err := os.WriteFile(target, data, 0o644); err != nil {
				f.t.Fatalf("write: %v", err)
			}
			w.WriteHeader(http.StatusOK)
			return
		}
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
		f.bytesUploaded += int64(len(data))
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
	case http.MethodDelete:
		if err := os.Remove(target); err != nil {
			if os.IsNotExist(err) {
				http.NotFound(w, r)
				return
			}
			f.t.Fatalf("delete: %v", err)
		}
		w.WriteHeader(http.StatusNoContent)
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
