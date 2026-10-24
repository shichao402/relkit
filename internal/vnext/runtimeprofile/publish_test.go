package runtimeprofile

import (
	"crypto/ed25519"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/shichao402/relkit/internal/config"
	"github.com/shichao402/relkit/internal/envelope"
	"github.com/shichao402/relkit/internal/vnext/project"
)

func testSigner() envelope.Signer {
	seed := make([]byte, ed25519.SeedSize)
	for index := range seed {
		seed[index] = byte(index + 7)
	}
	return envelope.Signer{KeyID: "server-root", Seed: seed}
}

func testProduct() project.Document {
	signer := testSigner()
	return project.Document{
		Schema:        project.Schema,
		Product:       "demo",
		Server:        "https://updates.example",
		RootPublicKey: base64.StdEncoding.EncodeToString(signer.PublicKey()),
		Channels:      []string{"dev", "stable"},
	}
}

func TestCreateSignsWithTheServerRoot(t *testing.T) {
	signer := testSigner()
	profile, err := Create(
		testProduct(),
		[]string{"https://updates.example/directory/demo.pb"},
		1,
		signer,
	)
	if err != nil {
		t.Fatal(err)
	}
	if profile.Product != "demo" || profile.ProfileVersion != 1 || profile.Digest == "" {
		t.Fatalf("incomplete Runtime Profile: %#v", profile)
	}
	if len(profile.Signatures) != 1 || profile.Signatures[0].KeyID != signer.KeyID {
		t.Fatalf("Runtime Profile is not signed by exactly the server root: %#v", profile.Signatures)
	}
	if err := profile.Verify(map[string]ed25519.PublicKey{signer.KeyID: signer.PublicKey()}); err != nil {
		t.Fatalf("created Runtime Profile is not verifiable: %v", err)
	}
}

func TestCreateRequiresEntryURLs(t *testing.T) {
	if _, err := Create(testProduct(), nil, 1, testSigner()); err == nil ||
		!strings.Contains(err.Error(), "entry URLs") {
		t.Fatalf("expected a missing entry URL rejection, got %v", err)
	}
}

func TestPublishWritesAndReadsSignedRuntimeProfile(t *testing.T) {
	var mutex sync.Mutex
	objects := map[string][]byte{}
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		key := strings.TrimPrefix(request.URL.Path, "/")
		mutex.Lock()
		defer mutex.Unlock()
		switch request.Method {
		case http.MethodPut:
			data, _ := io.ReadAll(request.Body)
			objects[key] = data
			response.WriteHeader(http.StatusCreated)
		case http.MethodGet:
			data, ok := objects[key]
			if !ok {
				http.NotFound(response, request)
				return
			}
			_, _ = response.Write(data)
		default:
			response.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()
	t.Setenv("RELKIT_TEST_STORE_TOKEN", "token")

	signer := testSigner()
	root := t.TempDir()
	cfg := &config.Config{
		Root:      root,
		Product:   "demo",
		Raw:       map[string]any{},
		Backends: map[string]map[string]any{
			"serve": {
				"type":      "relkit-compatible",
				"baseUrl":   server.URL + "/",
				"uploadUrl": server.URL + "/",
				"tokenEnv":  "RELKIT_TEST_STORE_TOKEN",
			},
		},
		PublishTo: []string{"serve"},
		Directory: &config.DirectoryConfig{
			PublishTo: []string{"serve"},
			EntryURLs: []string{server.URL + "/directory/demo.pb"},
		},
	}
	cfg.SetRoot(&config.RootKey{KeyID: signer.KeyID, Seed: signer.Seed, PublicKey: signer.PublicKey()})

	result, err := Publish(cfg, testProduct(), PublishOptions{ProfileVersion: 3, Signer: signer}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Profile.Digest == "" || result.Reference.Sha256 == "" || len(result.Reference.Urls) != 1 {
		t.Fatalf("incomplete result: %#v", result)
	}
	if _, ok := objects[result.Key]; !ok {
		t.Fatalf("Runtime Profile was not stored at %s", result.Key)
	}
	if err := result.Profile.Verify(map[string]ed25519.PublicKey{signer.KeyID: signer.PublicKey()}); err != nil {
		t.Fatalf("published Runtime Profile is not verifiable: %v", err)
	}
}

func TestPublishRequiresTheServerRootSigner(t *testing.T) {
	cfg := &config.Config{
		Root:      t.TempDir(),
		Product:   "demo",
		Raw:       map[string]any{},
		PublishTo: []string{"serve"},
		Directory: &config.DirectoryConfig{
			PublishTo: []string{"serve"},
			EntryURLs: []string{"https://updates.example/directory/demo.pb"},
		},
	}
	if _, err := Publish(cfg, testProduct(), PublishOptions{ProfileVersion: 1}, nil); err == nil ||
		!strings.Contains(err.Error(), "server root signer") {
		t.Fatalf("expected a missing signer rejection, got %v", err)
	}
}
