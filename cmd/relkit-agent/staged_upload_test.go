package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"cnb.cool/shichao402/relkit/internal/stage"
	"cnb.cool/shichao402/relkit/internal/stagedput"
)

func TestMultipartStagedUploadAndPublish(t *testing.T) {
	fx := newAgentFixture(t, agentFixtureOpts{tinyParts: true})
	path := writeTarball(t, fx.tarball)

	result, err := stagedput.Put(context.Background(), stagedput.Options{
		URL:         fx.ts.URL,
		Token:       fx.token,
		Product:     "demo",
		Version:     "1.0.0",
		File:        path,
		PartSize:    32,
		Concurrency: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Bytes != int64(len(fx.tarball)) {
		t.Fatalf("bytes=%d want %d", result.Bytes, len(fx.tarball))
	}
	if _, err := stage.LoadStaged(fx.productRoot, "1.0.0"); err != nil {
		t.Fatal(err)
	}
	status, body := fx.publish(t, `{"product":"demo","version":"1.0.0","dryRun":true,"stagedSha256":"`+result.SHA256+`"}`)
	if status != http.StatusOK {
		t.Fatalf("publish status=%d body=%s", status, body)
	}
}

func TestMultipartResumeSkipsReceivedParts(t *testing.T) {
	fx := newAgentFixture(t, agentFixtureOpts{tinyParts: true})
	path := writeTarball(t, fx.tarball)
	sum := sha256.Sum256(fx.tarball)

	create, _ := json.Marshal(map[string]any{
		"bytes":    len(fx.tarball),
		"sha256":   hex.EncodeToString(sum[:]),
		"partSize": 32,
	})
	req, _ := http.NewRequest(http.MethodPost, fx.ts.URL+"/v1/staged/demo/1.0.0/uploads", bytes.NewReader(create))
	req.Header.Set("Authorization", "Bearer "+fx.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", resp.StatusCode, body)
	}
	var session stagedput.Session
	if err := json.Unmarshal(body, &session); err != nil {
		t.Fatal(err)
	}
	if session.PartCount < 2 {
		t.Fatalf("need multiple parts, got %d (tarball %d bytes)", session.PartCount, len(fx.tarball))
	}

	part0 := fx.tarball[:session.PartSize]
	put, _ := http.NewRequest(http.MethodPut, fx.ts.URL+"/v1/staged/demo/1.0.0/uploads/"+session.ID+"/parts/0", bytes.NewReader(part0))
	put.Header.Set("Authorization", "Bearer "+fx.token)
	putResp, err := http.DefaultClient.Do(put)
	if err != nil {
		t.Fatal(err)
	}
	putBody, _ := io.ReadAll(putResp.Body)
	putResp.Body.Close()
	if putResp.StatusCode != http.StatusCreated {
		t.Fatalf("part0 status=%d body=%s", putResp.StatusCode, putBody)
	}

	if _, err := stagedput.Put(context.Background(), stagedput.Options{
		URL:         fx.ts.URL,
		Token:       fx.token,
		Product:     "demo",
		Version:     "1.0.0",
		File:        path,
		PartSize:    32,
		Concurrency: 3,
	}); err != nil {
		t.Fatal(err)
	}
}

func TestMultipartRejectsWrongSHA(t *testing.T) {
	fx := newAgentFixture(t, agentFixtureOpts{tinyParts: true})
	create, _ := json.Marshal(map[string]any{
		"bytes":  len(fx.tarball),
		"sha256": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	})
	req, _ := http.NewRequest(http.MethodPost, fx.ts.URL+"/v1/staged/demo/1.0.0/uploads", bytes.NewReader(create))
	req.Header.Set("Authorization", "Bearer "+fx.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var session stagedput.Session
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	for i := 0; i < session.PartCount; i++ {
		start := int(session.PartSize) * i
		end := start + int(session.PartSize)
		if end > len(fx.tarball) {
			end = len(fx.tarball)
		}
		put, _ := http.NewRequest(http.MethodPut, fx.ts.URL+"/v1/staged/demo/1.0.0/uploads/"+session.ID+"/parts/"+strconv.Itoa(i), bytes.NewReader(fx.tarball[start:end]))
		put.Header.Set("Authorization", "Bearer "+fx.token)
		putResp, err := http.DefaultClient.Do(put)
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, putResp.Body)
		putResp.Body.Close()
		if putResp.StatusCode != http.StatusCreated && putResp.StatusCode != http.StatusOK {
			t.Fatalf("part %d status=%d", i, putResp.StatusCode)
		}
	}
	complete, _ := http.NewRequest(http.MethodPost, fx.ts.URL+"/v1/staged/demo/1.0.0/uploads/"+session.ID+"/complete", http.NoBody)
	complete.Header.Set("Authorization", "Bearer "+fx.token)
	done, err := http.DefaultClient.Do(complete)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(done.Body)
	done.Body.Close()
	if done.StatusCode != http.StatusBadRequest {
		t.Fatalf("complete status=%d body=%s", done.StatusCode, body)
	}
}

func TestLoadConfigUploadTuning(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.json")
	if err := os.WriteFile(path, []byte(`{
  "partSize": "16MiB",
  "minPartSize": "2MiB",
  "maxPartSize": "32MiB",
  "maxPartConcurrency": 12,
  "uploadTTL": "6h",
  "products": {}
}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PartSize != 16<<20 || cfg.MinPartSize != 2<<20 || cfg.MaxPartSize != 32<<20 {
		t.Fatalf("sizes part=%d min=%d max=%d", cfg.PartSize, cfg.MinPartSize, cfg.MaxPartSize)
	}
	if cfg.MaxPartConcurrency != 12 {
		t.Fatalf("concurrency %d", cfg.MaxPartConcurrency)
	}
}

func writeTarball(t *testing.T, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "staged.tar.gz")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
