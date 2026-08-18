package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	rupv2 "cnb.cool/shichao402/relkit/api/rup/v2"
	"cnb.cool/shichao402/relkit/internal/config"
	"cnb.cool/shichao402/relkit/internal/keys"
	"cnb.cool/shichao402/relkit/internal/stage"
)

func TestAgentStagedAndPublishDryRun(t *testing.T) {
	root := t.TempDir()
	stateDir := filepath.Join(root, "state")
	productRoot := filepath.Join(root, "product")
	if err := os.MkdirAll(productRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	seed, err := keys.GenerateSeed()
	if err != nil {
		t.Fatal(err)
	}
	pub := ed25519.NewKeyFromSeed(seed).Public().(ed25519.PublicKey)
	privDoc := keys.PrivateKeyDocument("k1", seed)
	privBytes, _ := rupv2.MarshalPrivateKey(&privDoc)
	privPath := filepath.Join(productRoot, "private.pb")
	if err := os.WriteFile(privPath, privBytes, 0o600); err != nil {
		t.Fatal(err)
	}

	cfgDoc := map[string]any{
		"product":        "demo",
		"defaultChannel": "stable",
		"channels":       []any{"stable"},
		"codeStrategy":   "explicit",
		"signing": map[string]any{
			"keyId":          "k1",
			"privateKeyPath": "private.pb",
			"publicKeys": []any{map[string]any{
				"keyId": "k1",
				"key":   base64.StdEncoding.EncodeToString(pub),
			}},
		},
		"backends": map[string]any{
			"local": map[string]any{
				"type":      "local",
				"outputDir": "dist",
				"baseUrl":   "https://example.invalid/rup/",
			},
		},
		"publishTo": []any{"local"},
	}
	cfgBytes, _ := json.MarshalIndent(cfgDoc, "", "  ")
	if err := os.WriteFile(filepath.Join(productRoot, config.ConfigName), cfgBytes, 0o644); err != nil {
		t.Fatal(err)
	}

	relkitCfg, err := config.Load(filepath.Join(productRoot, config.ConfigName))
	if err != nil {
		t.Fatal(err)
	}
	art := filepath.Join(productRoot, "app.bin")
	if err := os.WriteFile(art, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	code := 1
	if _, err := stage.Run(relkitCfg, "1.0.0", code, 0, []stage.AddSpec{{Path: art, PairsText: "id=app,kind=binary"}}, "stable", "", "", "", false, func(string) {}); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	stagedRoot := stage.StagingDir(productRoot, "1.0.0")
	err = filepath.Walk(stagedRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(stagedRoot, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		hdr, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		hdr.Name = filepath.ToSlash(rel)
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if info.Mode().IsRegular() {
			f, err := os.Open(path)
			if err != nil {
				return err
			}
			defer f.Close()
			_, err = io.Copy(tw, f)
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = tw.Close()
	_ = gz.Close()

	// wipe staged so upload recreates it
	_ = os.RemoveAll(stagedRoot)

	agentCfgPath := filepath.Join(root, "agent.json")
	agentDoc := map[string]any{
		"uploadToken": "test-token",
		"stateDir":    stateDir,
		"products": map[string]any{
			"demo": map[string]any{"root": productRoot},
		},
	}
	agentBytes, _ := json.MarshalIndent(agentDoc, "", "  ")
	if err := os.WriteFile(agentCfgPath, agentBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(agentCfgPath)
	if err != nil {
		t.Fatal(err)
	}
	srv := NewServer(cfg)
	mux := http.NewServeMux()
	mux.HandleFunc("/-/health", srv.handleHealth)
	mux.HandleFunc("/v1/staged/", srv.handleStaged)
	mux.HandleFunc("/v1/publish", srv.handlePublish)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	req, _ := http.NewRequest(http.MethodPut, ts.URL+"/v1/staged/demo/1.0.0", bytes.NewReader(buf.Bytes()))
	req.Header.Set("Authorization", "Bearer test-token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("staged status=%d body=%s", resp.StatusCode, body)
	}

	pubBody := `{"product":"demo","version":"1.0.0","dryRun":true}`
	req2, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/publish", bytes.NewReader([]byte(pubBody)))
	req2.Header.Set("Authorization", "Bearer test-token")
	req2.Header.Set("Content-Type", "application/json")
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	body2, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("publish status=%d body=%s", resp2.StatusCode, body2)
	}
}

func TestExtractRefusesTraversal(t *testing.T) {
	dir := t.TempDir()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	hdr := &tar.Header{Name: "../evil.txt", Mode: 0o644, Size: 4}
	_ = tw.WriteHeader(hdr)
	_, _ = tw.Write([]byte("evil"))
	_ = tw.Close()
	_ = gz.Close()
	arch := filepath.Join(dir, "bad.tgz")
	_ = os.WriteFile(arch, buf.Bytes(), 0o644)
	dest := filepath.Join(dir, "out")
	_ = os.MkdirAll(dest, 0o755)
	if err := extractTarGz(arch, dest, 100); err == nil {
		t.Fatal("expected traversal refusal")
	}
}

func TestExtractAllowsDotSlashRoot(t *testing.T) {
	dir := t.TempDir()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	_ = tw.WriteHeader(&tar.Header{Name: "./", Typeflag: tar.TypeDir, Mode: 0o755})
	payload := []byte("ok")
	_ = tw.WriteHeader(&tar.Header{Name: "./staged.pb", Mode: 0o644, Size: int64(len(payload))})
	_, _ = tw.Write(payload)
	_ = tw.Close()
	_ = gz.Close()
	arch := filepath.Join(dir, "ok.tgz")
	_ = os.WriteFile(arch, buf.Bytes(), 0o644)
	dest := filepath.Join(dir, "out")
	_ = os.MkdirAll(dest, 0o755)
	if err := extractTarGz(arch, dest, 100); err != nil {
		t.Fatalf("extract: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dest, "staged.pb"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "ok" {
		t.Fatalf("got %q", got)
	}
}
