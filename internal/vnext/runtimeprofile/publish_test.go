package runtimeprofile

import (
	"crypto/ed25519"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	rupv2 "github.com/shichao402/relkit/api/rup/v2"
	"github.com/shichao402/relkit/internal/config"
	"github.com/shichao402/relkit/internal/envelope"
	"github.com/shichao402/relkit/internal/keys"
	"github.com/shichao402/relkit/internal/vnext/project"
	"github.com/shichao402/relkit/internal/vnext/trust"
)

func TestCreateBootstrapsSignedRuntimeProfile(t *testing.T) {
	seed := make([]byte, ed25519.SeedSize)
	for index := range seed {
		seed[index] = byte(index + 7)
	}
	signer := envelope.Signer{KeyID: "product-root", Seed: seed}
	fingerprint, err := trust.Fingerprint(signer.PublicKey())
	if err != nil {
		t.Fatal(err)
	}
	productDocument := project.Document{
		Schema:           project.Schema,
		Product:          "demo",
		Channels:         []string{"dev", "stable"},
		OperatorRef:      "prod",
		TrustFingerprint: "sha256:" + strings.Repeat("a", 64),
		Roles:            []project.Role{project.RolePublisher},
		ArtifactProducer: &project.Command{Command: []string{"fixture"}},
		RuntimeProfilePublication: &project.RuntimeProfilePublication{
			RootFingerprint: fingerprint,
			RUPSchemaSet:    "rup.v2",
			Recovery:        map[string]any{"message": "manual install"},
		},
		CapabilityFingerprint: project.CapabilityFingerprint{
			OSArch:   []string{"darwin-arm64"},
			Topology: "producer-only",
		},
	}

	profile, err := Create(
		productDocument,
		[]string{"https://updates.example/directory/demo.pb"},
		1,
		[]envelope.Signer{signer},
	)
	if err != nil {
		t.Fatal(err)
	}
	if profile.Product != "demo" || profile.ProfileVersion != 1 || profile.Digest == "" {
		t.Fatalf("incomplete Runtime Profile: %#v", profile)
	}
	if err := profile.Verify(map[string]ed25519.PublicKey{signer.KeyID: signer.PublicKey()}); err != nil {
		t.Fatalf("created Runtime Profile is not verifiable: %v", err)
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

	seed := make([]byte, ed25519.SeedSize)
	for index := range seed {
		seed[index] = byte(index + 3)
	}
	publicKey := ed25519.NewKeyFromSeed(seed).Public().(ed25519.PublicKey)
	fingerprint, err := trust.Fingerprint(publicKey)
	if err != nil {
		t.Fatal(err)
	}
	privateDocument := keys.PrivateKeyDocument("product-root", seed)
	privateBytes, err := rupv2.MarshalPrivateKey(&privateDocument)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	privatePath := filepath.Join(root, "product.private.pb")
	if err := os.WriteFile(privatePath, privateBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{
		Root:    root,
		Product: "demo",
		Signing: map[string]any{"keyId": "product-root", "privateKeyPath": privatePath},
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
	productDocument := project.Document{
		Schema:           project.Schema,
		Product:          "demo",
		Channels:         []string{"dev", "stable"},
		OperatorRef:      "prod",
		TrustFingerprint: "sha256:" + strings.Repeat("a", 64),
		Roles:            []project.Role{project.RolePublisher},
		ArtifactProducer: &project.Command{Command: []string{"fixture"}},
		RuntimeProfilePublication: &project.RuntimeProfilePublication{
			RootFingerprint: fingerprint,
			RUPSchemaSet:    "rup.v2",
		},
		CapabilityFingerprint: project.CapabilityFingerprint{
			OSArch:   []string{"darwin-arm64"},
			Topology: "producer-only",
		},
	}
	result, err := Publish(cfg, productDocument, PublishOptions{ProfileVersion: 3}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Profile.Digest == "" || result.Reference.Sha256 == "" || len(result.Reference.Urls) != 1 {
		t.Fatalf("incomplete result: %#v", result)
	}
	if _, ok := objects[result.Key]; !ok {
		t.Fatalf("Runtime Profile was not stored at %s", result.Key)
	}
}
