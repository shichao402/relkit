package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cnb.cool/shichao402/relkit/internal/config"
	"cnb.cool/shichao402/relkit/internal/model"
)

func TestCASCredentialsLocalUploadAndSkip(t *testing.T) {
	fx := newAgentFixture(t, agentFixtureOpts{})
	payload := []byte("hello")
	digest := model.Sha256Bytes(payload)

	doc := requestCASCredentials(t, fx, `{"product":"demo","blobs":[{"sha256":"`+digest+`","size":5},{"sha256":"`+digest+`","size":5}]}`)
	if len(doc.Uploads) != 1 {
		t.Fatalf("uploads=%v", doc.Uploads)
	}
	upload := doc.Uploads[0]
	req, _ := http.NewRequest(http.MethodPut, fx.ts.URL+upload.PutURL, bytes.NewReader(payload))
	for key, value := range upload.Headers {
		req.Header.Set(key, value)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("PUT status=%d", resp.StatusCode)
	}

	doc = requestCASCredentials(t, fx, `{"product":"demo","blobs":[{"sha256":"`+digest+`","size":5}]}`)
	if len(doc.Uploads) != 0 {
		t.Fatalf("cas hit should return no uploads: %v", doc.Uploads)
	}
}

func TestCASPutRejectsWrongHash(t *testing.T) {
	fx := newAgentFixture(t, agentFixtureOpts{})
	digest := strings.Repeat("a", 64)
	req, _ := http.NewRequest(http.MethodPut, fx.ts.URL+"/v1/cas/demo/"+digest+"?size=5", bytes.NewReader([]byte("hello")))
	req.Header.Set("Authorization", "Bearer "+fx.token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status=%d body=%s", resp.StatusCode, body)
	}
}

func TestCASCredentialsRequiresProductToken(t *testing.T) {
	fx := newAgentFixture(t, agentFixtureOpts{})
	req, _ := http.NewRequest(http.MethodPost, fx.ts.URL+"/v1/cas/credentials", strings.NewReader(`{"product":"demo","blobs":[{"sha256":"`+strings.Repeat("a", 64)+`","size":1}]}`))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status=%d", resp.StatusCode)
	}
}

func TestCASCredentialsRejectsProfileWithoutIngest(t *testing.T) {
	t.Setenv("STATIC_TOKEN", "token")
	fx := newAgentFixture(t, agentFixtureOpts{
		patchProfile: func(profile *config.PublishProfile) {
			profile.Backends = map[string]map[string]any{
				"static": {
					"type":     "static-http",
					"baseUrl":  "https://example.invalid/rup/",
					"stageDir": "mirror",
				},
			}
			profile.PublishTo = []string{"static"}
		},
	})
	req, _ := http.NewRequest(http.MethodPost, fx.ts.URL+"/v1/cas/credentials", strings.NewReader(`{"product":"demo","blobs":[{"sha256":"`+strings.Repeat("a", 64)+`","size":1}]}`))
	req.Header.Set("Authorization", "Bearer "+fx.token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status=%d body=%s", resp.StatusCode, body)
	}
}

func TestCASCredentialsRejectsMultiplePublishTargetsUntilMaterialize(t *testing.T) {
	fx := newAgentFixture(t, agentFixtureOpts{
		patchProfile: func(profile *config.PublishProfile) {
			profile.Backends["mirror"] = map[string]any{
				"type": "static-http", "baseUrl": "https://example.invalid/mirror/", "stageDir": "mirror",
			}
			profile.PublishTo = []string{"local", "mirror"}
		},
	})
	req, _ := http.NewRequest(http.MethodPost, fx.ts.URL+"/v1/cas/credentials", strings.NewReader(`{"product":"demo","blobs":[{"sha256":"`+strings.Repeat("a", 64)+`","size":1}]}`))
	req.Header.Set("Authorization", "Bearer "+fx.token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusBadRequest || !bytes.Contains(body, []byte("exactly one publishTo")) {
		t.Fatalf("status=%d body=%s", resp.StatusCode, body)
	}
}

func TestThinStagedPublishesFromCASWithoutLocalArtifact(t *testing.T) {
	fx := newAgentFixture(t, agentFixtureOpts{})
	payload := []byte("hello")
	digest := model.Sha256Bytes(payload)
	doc := requestCASCredentials(t, fx, `{"product":"demo","blobs":[{"sha256":"`+digest+`","size":5}]}`)
	upload := doc.Uploads[0]
	put, _ := http.NewRequest(http.MethodPut, fx.ts.URL+upload.PutURL, bytes.NewReader(payload))
	for key, value := range upload.Headers {
		put.Header.Set(key, value)
	}
	resp, err := http.DefaultClient.Do(put)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("CAS PUT status=%d", resp.StatusCode)
	}

	thin := stripArtifactsFromTar(t, fx.tarball)
	req, _ := http.NewRequest(http.MethodPut, fx.ts.URL+"/v1/staged/demo/1.0.0", bytes.NewReader(thin))
	req.Header.Set("Authorization", "Bearer "+fx.token)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("thin staged status=%d body=%s", resp.StatusCode, body)
	}

	status, body := fx.publish(t, `{"product":"demo","version":"1.0.0"}`)
	if status != http.StatusOK {
		t.Fatalf("publish status=%d body=%s", status, body)
	}
	got, err := os.ReadFile(filepath.Join(fx.productRoot, "dist", "artifact", "demo", "1.0.0", "app.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("artifact=%q", got)
	}
}

func requestCASCredentials(t *testing.T, fx *agentFixture, body string) casCredentialResponse {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, fx.ts.URL+"/v1/cas/credentials", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+fx.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("credentials status=%d body=%s", resp.StatusCode, data)
	}
	var doc casCredentialResponse
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	return doc
}

func stripArtifactsFromTar(t *testing.T, source []byte) []byte {
	t.Helper()
	zr, err := gzip.NewReader(bytes.NewReader(source))
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(zr)
	var out bytes.Buffer
	zw := gzip.NewWriter(&out)
	tw := tar.NewWriter(zw)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if hdr.Name == "artifacts" || strings.HasPrefix(hdr.Name, "artifacts/") {
			continue
		}
		copy := *hdr
		if err := tw.WriteHeader(&copy); err != nil {
			t.Fatal(err)
		}
		if _, err := io.Copy(tw, tr); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}
