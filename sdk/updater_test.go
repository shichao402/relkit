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

	rupv2 "cnb.cool/shichao402/relkit/api/rup/v2"
	"cnb.cool/shichao402/relkit/internal/envelope"
	"cnb.cool/shichao402/relkit/sdk"
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

	manifest := &rupv2.Manifest{
		Schema:  rupv2.SchemaManifest,
		Product: "demo",
		Version: "1.0.0",
		Code:    100,
		Artifacts: []*rupv2.Artifact{{
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

	index := &rupv2.Index{
		Schema:      rupv2.SchemaIndex,
		Product:     "demo",
		Channel:     "stable",
		Sequence:    1,
		GeneratedAt: "2026-08-01T00:00:00Z",
		Versions: []*rupv2.VersionNode{{
			Version: "1.0.0",
			Code:    100,
			Manifest: &rupv2.DigestRef{
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
	store := sdk.NewMemoryStateStore(nil)
	u := &sdk.Updater{
		Product:         "demo",
		Channel:         "stable",
		CurrentCode:     0,
		IndexURLs:       []string{srv.URL + "/index/demo/stable.pb"},
		TrustedKeys:     sdk.TrustedKeys{"k1": pub},
		ClientSelectors: map[string]string{},
		StateStore:      store,
		Policy:          sdk.Policy{AfterSuccess: 0, AfterFailure: 0}, // still uses defaults via policy()
	}
	// Force bypass throttle by clearing last check via fresh store
	result := u.CheckForce(context.Background(), true)
	if result.Err != nil {
		t.Fatalf("check: %v attempts=%v", result.Err, result.Attempts)
	}
	if result.Available == nil {
		t.Fatalf("expected available update, got %+v", result)
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

func TestDirectoryBootstrap(t *testing.T) {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	_, seed, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	seed32 := seed.Seed()
	pub := ed25519.NewKeyFromSeed(seed32).Public().(ed25519.PublicKey)

	payload := []byte("artifact-bytes")
	artSum := sha256.Sum256(payload)
	manifest := &rupv2.Manifest{
		Schema:  rupv2.SchemaManifest,
		Product: "demo",
		Version: "2.0.0",
		Code:    200,
		Artifacts: []*rupv2.Artifact{{
			Id: "app", Filename: "app.bin", Size: int64(len(payload)),
			Sha256: hex.EncodeToString(artSum[:]), Kind: "bin",
			Urls: []string{srv.URL + "/a.bin"},
		}},
	}
	manBytes, _ := rupv2.MarshalManifest(manifest)
	manSum := sha256.Sum256(manBytes)
	index := &rupv2.Index{
		Schema: rupv2.SchemaIndex, Product: "demo", Channel: "stable", Sequence: 3,
		GeneratedAt: "2026-08-01T00:00:00Z",
		Versions: []*rupv2.VersionNode{{
			Version: "2.0.0", Code: 200,
			Manifest: &rupv2.DigestRef{Sha256: hex.EncodeToString(manSum[:]), Size: int64(len(manBytes)), Urls: []string{srv.URL + "/m.pb"}},
		}},
	}
	indexEnv, _ := envelope.Seal(index, []envelope.Signer{{KeyID: "k1", Seed: seed32}})
	indexBytes, _ := rupv2.MarshalEnvelope(indexEnv)

	dirDoc := &rupv2.UpdateDirectory{
		Schema: rupv2.SchemaDirectory, Product: "demo", DirectorySequence: 1,
		UpdatedAt: "2026-08-01T00:00:00Z",
		Services: []*rupv2.DirectoryService{{
			Id: "primary", Priority: 10, IndexUrl: srv.URL + "/index.pb", Channel: "stable",
		}},
	}
	dirEnv, err := envelope.SealDirectory(dirDoc, []envelope.Signer{{KeyID: "k1", Seed: seed32}})
	if err != nil {
		t.Fatal(err)
	}
	dirBytes, _ := rupv2.MarshalEnvelope(dirEnv)

	mux.HandleFunc("/directory.pb", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(dirBytes) })
	mux.HandleFunc("/index.pb", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(indexBytes) })
	mux.HandleFunc("/m.pb", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(manBytes) })
	mux.HandleFunc("/a.bin", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(payload) })

	u := &sdk.Updater{
		Product: "demo", Channel: "stable", CurrentCode: 0,
		EntryURLs:   []string{srv.URL + "/directory.pb"},
		TrustedKeys: sdk.TrustedKeys{"k1": pub},
		StateStore:  sdk.NewMemoryStateStore(nil),
	}
	result := u.CheckForce(context.Background(), true)
	if result.Err != nil || result.Available == nil {
		t.Fatalf("check: err=%v available=%v attempts=%v", result.Err, result.Available, result.Attempts)
	}
	if result.Available.Target.Code != 200 {
		t.Fatalf("code=%d", result.Available.Target.Code)
	}
}

func TestSemverCode(t *testing.T) {
	code, err := sdk.SemverCode("v1.13.25")
	if err != nil {
		t.Fatal(err)
	}
	if code != 1_013_025 {
		t.Fatalf("got %d", code)
	}
}
