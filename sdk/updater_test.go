package sdk_test

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	rupv2 "github.com/shichao402/relkit/api/rup/v2"
	"github.com/shichao402/relkit/internal/envelope"
	"github.com/shichao402/relkit/internal/model"
	"github.com/shichao402/relkit/sdk"
)

func TestCheckAndDownload(t *testing.T) {
	dir := t.TempDir()
	artifactPath := filepath.Join(dir, "app.zip")
	if err := os.WriteFile(artifactPath, []byte("hello-rup-v2"), 0o644); err != nil {
		t.Fatal(err)
	}
	artSum := sha256.Sum256([]byte("hello-rup-v2"))
	artHex := hex.EncodeToString(artSum[:])

	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	manifest := &model.ManifestDocument{
		Schema:  rupv2.SchemaManifest,
		Product: "demo",
		Version: "1.0.0",
		Code:    100,
		Artifacts: []*model.ManifestArtifact{{
			Id:       "app",
			Filename: "app.zip",
			Size:     int64(len("hello-rup-v2")),
			Sha256:   artHex,
			Kind:     "archive",
			Urls:     []string{srv.URL + "/artifact/app.zip"},
		}},
	}
	manBytes, err := rupv2.MarshalManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	manSum := sha256.Sum256(manBytes)

	index := &model.IndexDocument{
		Schema:      rupv2.SchemaIndex,
		Product:     "demo",
		Channel:     "stable",
		Sequence:    1,
		GeneratedAt: "2026-08-01T00:00:00Z",
		Versions: []*model.VersionNode{{
			Version: "1.0.0",
			Code:    100,
			Manifest: &model.ManifestRef{
				Sha256: hex.EncodeToString(manSum[:]),
				Size:   int64(len(manBytes)),
				Urls:   []string{srv.URL + "/manifest/demo/1.0.0.pb"},
			},
		}},
	}

	_, seed, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	seed32 := seed.Seed()
	env, err := envelope.Seal(index, []envelope.Signer{{KeyID: "k1", Seed: seed32}})
	if err != nil {
		t.Fatal(err)
	}
	envBytes, err := rupv2.MarshalEnvelope(env)
	if err != nil {
		t.Fatal(err)
	}

	mux.HandleFunc("/index/demo/stable.pb", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/protobuf")
		_, _ = w.Write(envBytes)
	})
	mux.HandleFunc("/manifest/demo/1.0.0.pb", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/protobuf")
		_, _ = w.Write(manBytes)
	})
	mux.HandleFunc("/artifact/app.zip", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("hello-rup-v2"))
	})

	pub := ed25519.NewKeyFromSeed(seed32).Public().(ed25519.PublicKey)
	u := &sdk.Updater{
		Product:         "demo",
		Channel:         "stable",
		CurrentCode:     0,
		IndexURLs:       []string{srv.URL + "/index/demo/stable.pb"},
		TrustedKeys:     sdk.TrustedKeys{"k1": pub},
		ClientSelectors: map[string]string{},
	}

	result := u.Check(context.Background())
	if result.Err != nil {
		t.Fatalf("check: %v attempts=%v", result.Err, result.Attempts)
	}
	if result.Available == nil {
		t.Fatal("expected update")
	}
	dest := filepath.Join(dir, "out.zip")
	if err := u.Download(context.Background(), result.Available, dest); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "hello-rup-v2" {
		t.Fatalf("got %q", got)
	}
}
