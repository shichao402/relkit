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
	profile, err := config.ExtractPublishProfile(relkitCfg)
	if err != nil {
		t.Fatal(err)
	}
	profilePath := filepath.Join(root, "products", "demo.json")
	profileBytes, _ := json.MarshalIndent(profile, "", "  ")
	if err := os.MkdirAll(filepath.Dir(profilePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(profilePath, profileBytes, 0o600); err != nil {
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
			"demo": map[string]any{"root": productRoot, "profile": profilePath},
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
	mux.HandleFunc("/v1/drop/", srv.handleDrop)
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

func TestAgentDropPutGetHead(t *testing.T) {
	root := t.TempDir()
	productRoot := filepath.Join(root, "product")
	if err := os.MkdirAll(productRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	agentCfgPath := filepath.Join(root, "agent.json")
	agentDoc := map[string]any{
		"uploadToken": "test-token",
		"stateDir":    filepath.Join(root, "state"),
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
	mux.HandleFunc("/v1/drop/", srv.handleDrop)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	payload := []byte("macos-zip-bytes")
	url := ts.URL + "/v1/drop/demo/0.2.0+105/SvnAutoMerge_macos_0.2.0+105.zip"
	req, _ := http.NewRequest(http.MethodPut, url, bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer test-token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("drop put status=%d body=%s", resp.StatusCode, body)
	}

	head, err := http.NewRequest(http.MethodHead, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	head.Header.Set("Authorization", "Bearer test-token")
	respH, err := http.DefaultClient.Do(head)
	if err != nil {
		t.Fatal(err)
	}
	respH.Body.Close()
	if respH.StatusCode != http.StatusOK {
		t.Fatalf("drop head status=%d", respH.StatusCode)
	}

	get, _ := http.NewRequest(http.MethodGet, url, nil)
	get.Header.Set("Authorization", "Bearer test-token")
	respG, err := http.DefaultClient.Do(get)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(respG.Body)
	respG.Body.Close()
	if respG.StatusCode != http.StatusOK {
		t.Fatalf("drop get status=%d", respG.StatusCode)
	}
	if string(got) != string(payload) {
		t.Fatalf("drop get body=%q", got)
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

type agentFixture struct {
	productRoot string
	profilePath string
	legacyPath  string
	ts          *httptest.Server
	token       string
	tarball     []byte
}

func newAgentFixture(t *testing.T, opts agentFixtureOpts) *agentFixture {
	t.Helper()
	if opts.product == "" {
		opts.product = "demo"
	}
	if opts.version == "" {
		opts.version = "1.0.0"
	}
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
		"product":        opts.product,
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
		"changelog": map[string]any{
			"file":        "CHANGELOG.md",
			"urlTemplate": "https://example.invalid/notes/{version}",
		},
	}
	cfgBytes, _ := json.MarshalIndent(cfgDoc, "", "  ")
	legacyPath := filepath.Join(productRoot, config.ConfigName)
	if err := os.WriteFile(legacyPath, cfgBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(productRoot, "CHANGELOG.md"), []byte("## "+opts.version+"\n\n- notes\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	relkitCfg, err := config.Load(legacyPath)
	if err != nil {
		t.Fatal(err)
	}
	art := filepath.Join(productRoot, "app.bin")
	if err := os.WriteFile(art, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := stage.Run(relkitCfg, opts.version, 1, 0, []stage.AddSpec{{Path: art, PairsText: "id=app,kind=binary"}}, "stable", "", "", "", false, func(string) {}); err != nil {
		t.Fatal(err)
	}

	if opts.patchPolicy != nil {
		policyPath := stage.ReleasePolicyPath(productRoot, opts.version)
		data, err := os.ReadFile(policyPath)
		if err != nil {
			t.Fatal(err)
		}
		var policy map[string]any
		if err := json.Unmarshal(data, &policy); err != nil {
			t.Fatal(err)
		}
		opts.patchPolicy(policy)
		out, err := json.MarshalIndent(policy, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(policyPath, append(out, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if opts.omitPolicy {
		if err := os.Remove(stage.ReleasePolicyPath(productRoot, opts.version)); err != nil {
			t.Fatal(err)
		}
	}

	tarball, err := tarStaging(stage.StagingDir(productRoot, opts.version))
	if err != nil {
		t.Fatal(err)
	}
	_ = os.RemoveAll(stage.StagingDir(productRoot, opts.version))

	profilePath := filepath.Join(root, "products", opts.product+".json")
	if !opts.omitProfile {
		profile, err := config.ExtractPublishProfile(relkitCfg)
		if err != nil {
			t.Fatal(err)
		}
		if opts.patchProfile != nil {
			opts.patchProfile(profile)
		}
		profileBytes, _ := json.MarshalIndent(profile, "", "  ")
		if err := os.MkdirAll(filepath.Dir(profilePath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(profilePath, profileBytes, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	productCfg := map[string]any{"root": productRoot}
	if !opts.omitProfile {
		productCfg["profile"] = profilePath
	}
	agentCfgPath := filepath.Join(root, "agent.json")
	agentDoc := map[string]any{
		"uploadToken": "test-token",
		"stateDir":    stateDir,
		"products": map[string]any{
			opts.product: productCfg,
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
	mux.HandleFunc("/v1/drop/", srv.handleDrop)
	mux.HandleFunc("/v1/staged/", srv.handleStaged)
	mux.HandleFunc("/v1/publish", srv.handlePublish)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	return &agentFixture{
		productRoot: productRoot,
		profilePath: profilePath,
		legacyPath:  legacyPath,
		ts:          ts,
		token:       "test-token",
		tarball:     tarball,
	}
}

type agentFixtureOpts struct {
	product      string
	version      string
	omitPolicy   bool
	omitProfile  bool
	patchPolicy  func(map[string]any)
	patchProfile func(*config.PublishProfile)
}

func tarStaging(stagedRoot string) ([]byte, error) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	err := filepath.Walk(stagedRoot, func(path string, info os.FileInfo, err error) error {
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
	_ = tw.Close()
	_ = gz.Close()
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (f *agentFixture) putStaged(t *testing.T, product, version string) (int, []byte) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPut, f.ts.URL+"/v1/staged/"+product+"/"+version, bytes.NewReader(f.tarball))
	req.Header.Set("Authorization", "Bearer "+f.token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, body
}

func (f *agentFixture) publish(t *testing.T, body string) (int, []byte) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, f.ts.URL+"/v1/publish", bytes.NewReader([]byte(body)))
	req.Header.Set("Authorization", "Bearer "+f.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, out
}

func TestAgentRejectsReleasePolicyProductMismatch(t *testing.T) {
	fx := newAgentFixture(t, agentFixtureOpts{
		patchPolicy: func(policy map[string]any) {
			policy["product"] = "other"
		},
	})
	status, body := fx.putStaged(t, "demo", "1.0.0")
	if status != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", status, body)
	}
	if !bytes.Contains(body, []byte("does not match route product")) {
		t.Fatalf("body=%s", body)
	}
}

func TestAgentPublishStagedSHA256MatchAndMismatch(t *testing.T) {
	fx := newAgentFixture(t, agentFixtureOpts{})
	status, body := fx.putStaged(t, "demo", "1.0.0")
	if status != http.StatusCreated {
		t.Fatalf("staged status=%d body=%s", status, body)
	}
	var stagedResp map[string]any
	if err := json.Unmarshal(body, &stagedResp); err != nil {
		t.Fatal(err)
	}
	sum, _ := stagedResp["sha256"].(string)
	if sum == "" {
		t.Fatalf("missing sha256: %s", body)
	}

	status, body = fx.publish(t, `{"product":"demo","version":"1.0.0","dryRun":true,"stagedSha256":"`+sum+`"}`)
	if status != http.StatusOK {
		t.Fatalf("matching sha status=%d body=%s", status, body)
	}

	status, body = fx.publish(t, `{"product":"demo","version":"1.0.0","dryRun":true,"stagedSha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`)
	if status != http.StatusConflict {
		t.Fatalf("mismatch status=%d body=%s", status, body)
	}
	if !bytes.Contains(body, []byte("staged sha256 mismatch")) {
		t.Fatalf("body=%s", body)
	}

	status, body = fx.publish(t, `{"product":"demo","version":"1.0.0","dryRun":true,"stagedSha256":"short"}`)
	if status != http.StatusConflict {
		t.Fatalf("short sha status=%d body=%s", status, body)
	}
}

func TestAgentPublishRequiresProfileWhenPolicyPresent(t *testing.T) {
	fx := newAgentFixture(t, agentFixtureOpts{omitProfile: true})
	status, body := fx.putStaged(t, "demo", "1.0.0")
	if status != http.StatusCreated {
		t.Fatalf("staged status=%d body=%s", status, body)
	}
	status, body = fx.publish(t, `{"product":"demo","version":"1.0.0","dryRun":true}`)
	if status == http.StatusOK {
		t.Fatalf("expected missing profile error, body=%s", body)
	}
	if !bytes.Contains(body, []byte("profile")) {
		t.Fatalf("body=%s", body)
	}
}

func TestAgentPublishRequiresPolicy(t *testing.T) {
	fx := newAgentFixture(t, agentFixtureOpts{omitPolicy: true})
	status, body := fx.putStaged(t, "demo", "1.0.0")
	if status != http.StatusCreated {
		t.Fatalf("staged status=%d body=%s", status, body)
	}
	if _, err := os.Stat(stage.ReleasePolicyPath(fx.productRoot, "1.0.0")); !os.IsNotExist(err) {
		t.Fatalf("policy should be absent: %v", err)
	}
	if _, err := os.Stat(fx.legacyPath); err != nil {
		t.Fatalf("leftover product-root config should still exist: %v", err)
	}
	status, body = fx.publish(t, `{"product":"demo","version":"1.0.0","dryRun":true}`)
	if status == http.StatusOK {
		t.Fatalf("expected missing policy error, body=%s", body)
	}
	if !bytes.Contains(body, []byte("release-policy.json")) {
		t.Fatalf("body=%s", body)
	}
}

func TestAgentPublishRejectsPolicyProfileConflicts(t *testing.T) {
	t.Run("product", func(t *testing.T) {
		fx := newAgentFixture(t, agentFixtureOpts{
			patchProfile: func(profile *config.PublishProfile) {
				profile.Product = "other"
			},
		})
		status, body := fx.putStaged(t, "demo", "1.0.0")
		if status != http.StatusCreated {
			t.Fatalf("staged status=%d body=%s", status, body)
		}
		status, body = fx.publish(t, `{"product":"demo","version":"1.0.0","dryRun":true}`)
		if status == http.StatusOK {
			t.Fatalf("expected product mismatch, body=%s", body)
		}
		if !bytes.Contains(body, []byte("product")) {
			t.Fatalf("body=%s", body)
		}
	})
	t.Run("keyId", func(t *testing.T) {
		fx := newAgentFixture(t, agentFixtureOpts{
			patchProfile: func(profile *config.PublishProfile) {
				profile.Signing.KeyID = "k2"
			},
		})
		status, body := fx.putStaged(t, "demo", "1.0.0")
		if status != http.StatusCreated {
			t.Fatalf("staged status=%d body=%s", status, body)
		}
		status, body = fx.publish(t, `{"product":"demo","version":"1.0.0","dryRun":true}`)
		if status == http.StatusOK {
			t.Fatalf("expected keyId mismatch, body=%s", body)
		}
		if !bytes.Contains(body, []byte("keyId")) {
			t.Fatalf("body=%s", body)
		}
	})
}
