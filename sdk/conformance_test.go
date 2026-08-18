package sdk_test

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	rupv2 "cnb.cool/shichao402/relkit/api/rup/v2"
	"cnb.cool/shichao402/relkit/internal/testutil"
	"cnb.cool/shichao402/relkit/sdk"
)

func TestVersionSelectConformanceDelegated(t *testing.T) {
	// Full version-select fixtures live in internal/chain (legacy JSON → model).
	// Here we only assert the package is wired and fixtures are discoverable.
	root := filepath.Join(testutil.ConformanceRoot(t), "version-select")
	files, err := filepath.Glob(filepath.Join(root, "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no version-select fixtures")
	}
}

func TestThrottleAndStateStore(t *testing.T) {
	store := sdk.NewMemoryStateStore(nil)
	now := time.Now().UTC()
	st := &sdk.UpdateState{LastCheckAt: &now, LastResult: "success"}
	_ = store.Save(st)

	u := &sdk.Updater{
		Product: "demo", Channel: "stable", CurrentCode: 0,
		IndexURLs:   []string{"http://127.0.0.1:1/nope"},
		TrustedKeys: sdk.TrustedKeys{"k": make(ed25519.PublicKey, ed25519.PublicKeySize)},
		StateStore:  store,
		Policy:      sdk.DefaultPolicy(),
	}
	// Reload state from store
	loaded, _ := store.Load()
	u = &sdk.Updater{
		Product: "demo", Channel: "stable", CurrentCode: 0,
		IndexURLs:   []string{"http://127.0.0.1:1/nope"},
		TrustedKeys: sdk.TrustedKeys{"k": make(ed25519.PublicKey, ed25519.PublicKeySize)},
		StateStore:  sdk.NewMemoryStateStore(loaded),
		Policy:      sdk.DefaultPolicy(),
	}
	res := u.Check(context.Background())
	if !res.Throttled {
		t.Fatalf("expected throttled, got %+v", res)
	}
}

func TestAcceptsSequence(t *testing.T) {
	var seen int64 = 5
	if !sdk.AcceptsSequence(5, &seen) {
		t.Fatal("equal should accept")
	}
	if sdk.AcceptsSequence(4, &seen) {
		t.Fatal("older should refuse")
	}
	if !sdk.AcceptsSequence(6, &seen) {
		t.Fatal("newer should accept")
	}
}

func TestDownloadResume(t *testing.T) {
	payload := make([]byte, 200_000)
	for i := range payload {
		payload[i] = byte(i)
	}
	sum := sha256.Sum256(payload)
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	mux.HandleFunc("/blob", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Accept-Ranges", "bytes")
		rangeHdr := r.Header.Get("Range")
		if rangeHdr == "" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(payload)
			return
		}
		// bytes=N- or bytes=0-0 probe
		var start int
		if _, err := fmt.Sscanf(rangeHdr, "bytes=%d-", &start); err != nil {
			http.Error(w, "bad range", 400)
			return
		}
		if start == 0 && strings.HasSuffix(rangeHdr, "-0") {
			w.Header().Set("Content-Range", fmt.Sprintf("bytes 0-0/%d", len(payload)))
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write(payload[:1])
			return
		}
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, len(payload)-1, len(payload)))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(payload[start:])
	})

	art := &rupv2.Artifact{
		Id: "b", Filename: "b.bin", Size: int64(len(payload)),
		Sha256: hex.EncodeToString(sum[:]), Urls: []string{srv.URL + "/blob"},
	}
	dir := t.TempDir()
	dest := filepath.Join(dir, "out.bin")
	part := dest + ".part"
	if err := os.WriteFile(part, payload[:50_000], 0o644); err != nil {
		t.Fatal(err)
	}
	meta, _ := json.Marshal(map[string]any{
		"url": srv.URL + "/blob", "size": len(payload), "sha256": hex.EncodeToString(sum[:]),
	})
	_ = os.WriteFile(dest+".part.meta", meta, 0o644)

	verified, err := sdk.DownloadArtifact(context.Background(), &sdk.HTTPFetcher{}, art, dest, sdk.Policy{
		DownloadRetries: 2, DownloadWorkers: 1, DownloadChunkSize: 1 << 20,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(verified.Path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(payload) {
		t.Fatalf("len=%d want %d", len(got), len(payload))
	}
}