package ops

import (
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/shichao402/relkit/internal/vnext/catalog"
	"github.com/shichao402/relkit/internal/vnext/document"
)

func TestMirrorDistributionUploadsContentAddressedAssetsAndIsIdempotent(t *testing.T) {
	first := []byte("first immutable component")
	second := []byte("second immutable component")
	firstDigest := testMirrorDigest(first)
	secondDigest := testMirrorDigest(second)

	var mutex sync.Mutex
	stored := map[string][]byte{}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodGet && strings.HasPrefix(request.URL.Path, "/source/"):
			switch request.URL.Path {
			case "/source/first":
				_, _ = writer.Write(first)
			case "/source/second":
				_, _ = writer.Write(second)
			default:
				http.NotFound(writer, request)
			}
		case strings.HasPrefix(request.URL.Path, "/mirror/blobs/"):
			mutex.Lock()
			defer mutex.Unlock()
			switch request.Method {
			case http.MethodPut:
				username, password, ok := request.BasicAuth()
				if !ok || username != "writer" || password != "secret" {
					http.Error(writer, "unauthorized", http.StatusUnauthorized)
					return
				}
				if request.ContentLength < 1 {
					http.Error(writer, "content length required", http.StatusLengthRequired)
					return
				}
				if request.Header.Get("If-None-Match") != "*" || request.Header.Get("X-BKREPO-EXPIRES") != "0" {
					http.Error(writer, "immutable upload headers required", http.StatusBadRequest)
					return
				}
				if _, exists := stored[request.URL.Path]; exists {
					writer.WriteHeader(http.StatusConflict)
					return
				}
				stored[request.URL.Path], _ = io.ReadAll(request.Body)
				writer.WriteHeader(http.StatusCreated)
			case http.MethodGet:
				value, ok := stored[request.URL.Path]
				if !ok {
					http.NotFound(writer, request)
					return
				}
				_, _ = writer.Write(value)
			default:
				writer.WriteHeader(http.StatusMethodNotAllowed)
			}
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	distribution := mirrorFixtureDistribution(t, server.URL, firstDigest, secondDigest)
	path := filepath.Join(t.TempDir(), "distribution.json")
	if err := document.Write(path, distribution); err != nil {
		t.Fatal(err)
	}
	options := MirrorDistributionOptions{
		DistributionPath: path, MirrorRoot: server.URL + "/mirror",
		Username: "writer", Password: "secret",
	}
	result, err := MirrorDistribution(t.Context(), options)
	if err != nil {
		t.Fatal(err)
	}
	if result.Schema != "relkit.ops-mirror-result/1" || len(result.Assets) != 2 {
		t.Fatalf("result=%+v", result)
	}
	for _, asset := range result.Assets {
		if asset.Status != "uploaded" {
			t.Fatalf("first sync status=%q", asset.Status)
		}
		if !strings.HasSuffix(asset.MirrorURL, "/blobs/"+strings.TrimPrefix(asset.Digest, "sha256:")) {
			t.Fatalf("mirror URL=%q", asset.MirrorURL)
		}
	}

	repeated, err := MirrorDistribution(t.Context(), options)
	if err != nil {
		t.Fatal(err)
	}
	for _, asset := range repeated.Assets {
		if asset.Status != "present" {
			t.Fatalf("repeated sync status=%q", asset.Status)
		}
	}
}

func TestMirrorDistributionRejectsSourceDigestMismatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodGet && request.URL.Path == "/source/first" {
			_, _ = writer.Write([]byte("tampered"))
			return
		}
		t.Fatalf("unexpected request %s %s", request.Method, request.URL.Path)
	}))
	defer server.Close()

	firstDigest := testMirrorDigest([]byte("expected"))
	distribution := mirrorFixtureDistribution(t, server.URL, firstDigest, "")
	path := filepath.Join(t.TempDir(), "distribution.json")
	if err := document.Write(path, distribution); err != nil {
		t.Fatal(err)
	}
	_, err := MirrorDistribution(t.Context(), MirrorDistributionOptions{
		DistributionPath: path, MirrorRoot: server.URL + "/mirror",
		Username: "writer", Password: "secret",
	})
	if err == nil || !strings.Contains(err.Error(), "digest is") {
		t.Fatalf("expected digest mismatch, got %v", err)
	}
}

func TestMirrorDistributionRejectsChangedIdentity(t *testing.T) {
	content := []byte("component")
	distribution := mirrorFixtureDistribution(t, "https://source.invalid", testMirrorDigest(content), "")
	distribution.Commit = "changed"
	path := filepath.Join(t.TempDir(), "distribution.json")
	if err := document.Write(path, distribution); err != nil {
		t.Fatal(err)
	}
	_, err := MirrorDistribution(t.Context(), MirrorDistributionOptions{
		DistributionPath: path, MirrorRoot: "https://mirror.invalid",
		Username: "writer", Password: "secret",
	})
	if err == nil || !strings.Contains(err.Error(), "distribution identity") {
		t.Fatalf("expected identity rejection, got %v", err)
	}
}

func TestMirrorDistributionRejectsCredentialsInMirrorURL(t *testing.T) {
	content := []byte("component")
	distribution := mirrorFixtureDistribution(t, "https://source.invalid", testMirrorDigest(content), "")
	path := filepath.Join(t.TempDir(), "distribution.json")
	if err := document.Write(path, distribution); err != nil {
		t.Fatal(err)
	}
	_, err := MirrorDistribution(t.Context(), MirrorDistributionOptions{
		DistributionPath: path, MirrorRoot: "https://writer:secret@mirror.invalid/relkit",
		Username: "writer", Password: "secret",
	})
	if err == nil || !strings.Contains(err.Error(), "without credentials") {
		t.Fatalf("expected unsafe mirror URL rejection, got %v", err)
	}
}

func mirrorFixtureDistribution(t *testing.T, sourceRoot, firstDigest, secondDigest string) catalog.Distribution {
	t.Helper()
	components := []catalog.Component{{
		Name: "cli", Version: "v1.2.3", Platform: "linux-amd64", SHA256: firstDigest,
		Source: catalog.Source{Kind: "release-asset", URL: sourceRoot + "/source/first"},
	}}
	if secondDigest != "" {
		components = append(components, catalog.Component{
			Name: "updater", Version: "v1.2.3", Platform: "linux-amd64", SHA256: secondDigest,
			Source: catalog.Source{Kind: "release-asset", URL: sourceRoot + "/source/second"},
		})
	}
	value := catalog.Distribution{
		Version: "v1.2.3", Commit: strings.Repeat("a", 40), State: catalog.StateCandidate,
		Components: components,
	}
	digest, err := value.IdentityDigest()
	if err != nil {
		t.Fatal(err)
	}
	value.Digest = digest
	return value
}

func testMirrorDigest(value []byte) string {
	sum := sha256.Sum256(value)
	return "sha256:" + fmt.Sprintf("%x", sum[:])
}
