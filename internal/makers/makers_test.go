package makers

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestUnwrapPagesBodyDataResponse(t *testing.T) {
	raw := []byte(`{"Code":0,"Message":"ok","Data":{"Response":{"Bucket":"tmp","Region":"ap-guangzhou","TargetPath":"upload/abc"}}}`)
	got, err := unwrapPagesBody(raw)
	if err != nil {
		t.Fatal(err)
	}
	var body tempToken
	if err := json.Unmarshal(got, &body); err != nil {
		t.Fatal(err)
	}
	if body.Bucket != "tmp" || body.TargetPath != "upload/abc" {
		t.Fatalf("unwrapped %+v", body)
	}
}

func TestUnwrapPagesBodyBareResponse(t *testing.T) {
	raw := []byte(`{"Response":{"DeploymentId":"dep-1"}}`)
	got, err := unwrapPagesBody(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), `"dep-1"`) {
		t.Fatalf("got %s", got)
	}
}

func TestUnwrapPagesBodyNonZeroCode(t *testing.T) {
	_, err := unwrapPagesBody([]byte(`{"Code":401,"Message":"unauthorized"}`))
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("got %v", err)
	}
}

func TestListDumpFilesKeepsHTMLAndCatalog(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("index.html", "<html></html>")
	write("demo.html", "<html>demo</html>")
	write("catalog.json", `{"schema":"relkit.browse-catalog/1"}`)
	write("README.md", "no")
	write("notes.txt", "no")

	files, err := listDumpFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, file := range files {
		got[file.Rel] = true
	}
	if !got["index.html"] || !got["demo.html"] || !got["catalog.json"] {
		t.Fatalf("files=%v", got)
	}
	if got["README.md"] || got["notes.txt"] {
		t.Fatalf("should skip extras: %v", got)
	}
}

func TestDeployDirUsesWrappedTempToken(t *testing.T) {
	var uploaded []Object
	var createBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			http.Error(w, "auth", http.StatusUnauthorized)
			return
		}
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		var req map[string]any
		if err := json.Unmarshal(raw, &req); err != nil {
			t.Fatal(err)
		}
		switch req["Action"] {
		case "DescribePagesCosTempToken":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"Code":0,"Data":{"Response":{"Bucket":"pages-tmp","Region":"ap-guangzhou","TargetPath":"upload/abc","Credentials":{"TmpSecretId":"AKIATMP","TmpSecretKey":"secret","Token":"session"}}}}`))
		case "CreatePagesDeployment":
			if err := json.Unmarshal(raw, &createBody); err != nil {
				t.Fatal(err)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"Response":{"DeploymentId":"dep-9"}}`))
		default:
			http.Error(w, "unknown action", http.StatusBadRequest)
		}
	}))
	t.Cleanup(server.Close)

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<h1>hi</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "catalog.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}

	client := &Client{
		Token:   "test-token",
		BaseURL: server.URL,
		PutObject: func(obj Object) error {
			uploaded = append(uploaded, obj)
			return nil
		},
	}
	result, err := client.DeployDir("makers-test", dir)
	if err != nil {
		t.Fatal(err)
	}
	if result.DeploymentID != "dep-9" || result.TempBucketPath != "upload/abc" {
		t.Fatalf("result=%+v", result)
	}
	if len(uploaded) != 2 {
		t.Fatalf("uploaded %d files: %+v", len(uploaded), uploaded)
	}
	for _, obj := range uploaded {
		if !strings.HasPrefix(obj.Key, "upload/abc/") {
			t.Fatalf("key %q missing prefix", obj.Key)
		}
		if obj.Token != "session" || obj.SecretID != "AKIATMP" {
			t.Fatalf("sts not forwarded: %+v", obj)
		}
	}
	if createBody["ViaMeta"] != "Upload" || createBody["DistType"] != "Folder" {
		t.Fatalf("create payload %+v", createBody)
	}
	if createBody["TempBucketPath"] != "upload/abc" {
		t.Fatalf("TempBucketPath=%v", createBody["TempBucketPath"])
	}
}

func TestAPIBaseURL(t *testing.T) {
	if APIBaseURL("china") != chinaAPI {
		t.Fatal(APIBaseURL("china"))
	}
	if APIBaseURL("global") != globalAPI {
		t.Fatal(APIBaseURL("global"))
	}
	if APIBaseURL("") != chinaAPI {
		t.Fatal(APIBaseURL(""))
	}
}

func TestPutCOSSignsSecurityToken(t *testing.T) {
	var got http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if !bytes.Equal(body, []byte("hello")) {
			t.Fatalf("body=%q", body)
		}
		got = r.Header.Clone()
		if r.URL.Path != "/pages-tmp/upload/abc/index.html" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	client := &Client{
		Token:      "unused",
		COSBaseURL: server.URL,
		Now:        func() time.Time { return time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC) },
	}
	err := client.putCOS(Object{
		Bucket:      "pages-tmp",
		Region:      "ap-guangzhou",
		Key:         "upload/abc/index.html",
		ContentType: "text/html; charset=utf-8",
		Body:        []byte("hello"),
		SecretID:    "AKIATMP",
		SecretKey:   "secret",
		Token:       "session-token",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Get("X-Amz-Security-Token") != "session-token" {
		t.Fatalf("security token header=%q", got.Get("X-Amz-Security-Token"))
	}
	if !strings.Contains(got.Get("Authorization"), "AWS4-HMAC-SHA256") {
		t.Fatalf("Authorization=%q", got.Get("Authorization"))
	}
	if !strings.Contains(got.Get("Authorization"), "x-amz-security-token") {
		t.Fatalf("token header was not signed: %q", got.Get("Authorization"))
	}
}
