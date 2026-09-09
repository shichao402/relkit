package casput

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	rupv2 "go.firoyang.com/relkit/api/rup/v2"
	"go.firoyang.com/relkit/internal/model"
	"go.firoyang.com/relkit/internal/stage"
)

func TestPutUploadsCASAndThinStaged(t *testing.T) {
	root := t.TempDir()
	version := "1.0.0"
	payload := []byte("hello")
	digest := model.Sha256Bytes(payload)
	if err := os.MkdirAll(stage.ArtifactsDir(root, version), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stage.ArtifactsDir(root, version), "app.bin"), payload, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stage.ArtifactsDir(root, version), "app-copy.bin"), payload, 0o644); err != nil {
		t.Fatal(err)
	}
	artifact, err := model.NewStagedArtifact("app", "app.bin", int64(len(payload)), digest, "binary", map[string]string{"os": "windows"}, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	copyArtifact, err := model.NewStagedArtifact("app-copy", "app-copy.bin", int64(len(payload)), digest, "binary", map[string]string{"os": "linux"}, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := model.NewStagedDocument("demo", version, 1, 0, "stable", []*model.StagedArtifact{artifact, copyArtifact}, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	data, err := rupv2.MarshalStaged(doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stage.StagedPath(root, version), data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stage.ReleasePolicyPath(root, version), []byte(`{"product":"demo"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/cas/credentials":
			var request credentialRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Error(err)
			}
			if len(request.Blobs) != 1 {
				t.Errorf("credential blobs=%v", request.Blobs)
			}
			writeTestJSON(w, map[string]any{
				"uploads": []any{map[string]any{
					"sha256": digest, "size": len(payload),
					"requests": []any{map[string]any{"method": "PUT", "url": server.URL + "/cas-upload"}},
				}},
			})
		case r.Method == http.MethodPut && r.URL.Path == "/cas-upload":
			got, _ := io.ReadAll(r.Body)
			if !bytes.Equal(got, payload) {
				t.Errorf("CAS body=%q", got)
			}
			w.WriteHeader(http.StatusCreated)
		case r.Method == http.MethodPut && r.URL.Path == "/v1/staged/demo/1.0.0":
			assertThinArchive(t, r.Body)
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"sha256":"ok"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	result, err := Put(context.Background(), Options{
		Root: root, Product: "demo", Version: version, URL: server.URL, Token: "token",
		HTTPClient: server.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Uploaded != 1 || result.Skipped != 0 || result.StagedSHA256 == "" {
		t.Fatalf("result=%+v", result)
	}
	if err := os.WriteFile(filepath.Join(stage.ArtifactsDir(root, version), "app.bin"), []byte("HELLO"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = Put(context.Background(), Options{
		Root: root, Product: "demo", Version: version, URL: server.URL, Token: "token",
		HTTPClient: server.Client(),
	})
	if err == nil || !strings.Contains(err.Error(), "no longer matches staged.pb") {
		t.Fatalf("changed artifact err=%v", err)
	}
}

func TestUploadAllDoesNotFollowRedirect(t *testing.T) {
	source := filepath.Join(t.TempDir(), "blob")
	if err := os.WriteFile(source, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	targetHits := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/target" {
			targetHits++
			w.WriteHeader(http.StatusCreated)
			return
		}
		http.Redirect(w, r, "/target", http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	digest := model.Sha256Bytes([]byte("hello"))
	err := uploadAll(context.Background(), server.Client(), Options{URL: server.URL, Concurrency: 1}, []upload{{
		SHA256: digest, Size: 5, Requests: []uploadRequest{{Method: "PUT", URL: server.URL + "/redirect"}},
	}}, map[string]string{digest: source})
	if err == nil || !strings.Contains(err.Error(), "HTTP 307") {
		t.Fatalf("err=%v", err)
	}
	if targetHits != 0 {
		t.Fatalf("redirect target hits=%d", targetHits)
	}
}

func TestUploadOneRetriesCOSUserNetworkTooSlow(t *testing.T) {
	source := filepath.Join(t.TempDir(), "blob")
	payload := []byte("hello")
	if err := os.WriteFile(source, payload, 0o644); err != nil {
		t.Fatal(err)
	}
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		got, _ := io.ReadAll(r.Body)
		if !bytes.Equal(got, payload) {
			t.Errorf("attempt %d body=%q", attempts, got)
		}
		if attempts == 1 {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, "<Error><Code>UserNetworkTooSlow</Code></Error>")
			return
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	var logs []string
	item := upload{
		SHA256: model.Sha256Bytes(payload),
		Size:   int64(len(payload)),
		Requests: []uploadRequest{{
			Method: http.MethodPut,
			URL:    server.URL,
		}},
	}
	err := uploadOne(context.Background(), server.Client(), source, item, func(line string) {
		logs = append(logs, line)
	})
	if err != nil {
		t.Fatal(err)
	}
	if attempts != 2 {
		t.Fatalf("attempts=%d", attempts)
	}
	if len(logs) != 1 || !strings.Contains(logs[0], "attempt 1/4") {
		t.Fatalf("logs=%v", logs)
	}
}

func TestRetryableUploadResponse(t *testing.T) {
	if !retryableUploadResponse(http.StatusBadRequest, "<Code>UserNetworkTooSlow</Code>") {
		t.Fatal("COS slow-network response should be retryable")
	}
	if !retryableUploadResponse(http.StatusServiceUnavailable, "") {
		t.Fatal("503 should be retryable")
	}
	if retryableUploadResponse(http.StatusBadRequest, "<Code>InvalidArgument</Code>") {
		t.Fatal("ordinary 400 must not be retried")
	}
	if retryableUploadResponse(http.StatusTemporaryRedirect, "") {
		t.Fatal("redirect must not be retried")
	}
}

func TestUploadOneRejectsRelativeURL(t *testing.T) {
	_, err := normalizeUpload(upload{SHA256: "aa", Size: 1, Requests: []uploadRequest{{URL: "/cas-upload"}}})
	if err == nil || !strings.Contains(err.Error(), "absolute") {
		t.Fatalf("err=%v", err)
	}
}

func TestRedactTransportErrorHidesSignedQuery(t *testing.T) {
	err := redactErr(errors.New(`Put "https://bucket.example/cas/x?X-Amz-Signature=secret&sig=also-secret": timeout`))
	if strings.Contains(err.Error(), "secret") || !strings.Contains(err.Error(), "REDACTED") {
		t.Fatalf("redacted error = %q", err)
	}
}

func assertThinArchive(t *testing.T, body io.Reader) {
	t.Helper()
	zr, err := gzip.NewReader(body)
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(zr)
	var names []string
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, header.Name)
		if strings.HasPrefix(header.Name, "artifacts") {
			t.Fatalf("thin archive contains %q", header.Name)
		}
	}
	if strings.Join(names, ",") != "staged.pb,release-policy.json" {
		t.Fatalf("archive names=%v", names)
	}
}

func writeTestJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}
