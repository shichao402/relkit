package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"cnb.cool/shichao402/relkit/internal/config"
	"cnb.cool/shichao402/relkit/internal/model"
)

func TestCASCredentialsUploadAndSkip(t *testing.T) {
	fx := newAgentFixture(t, agentFixtureOpts{})
	payload := []byte("hello")
	digest := model.Sha256Bytes(payload)

	doc := requestCASCredentials(t, fx, `{"product":"demo","blobs":[{"sha256":"`+digest+`","size":5},{"sha256":"`+digest+`","size":5}]}`)
	if len(doc.Uploads) != 1 {
		t.Fatalf("uploads=%v", doc.Uploads)
	}
	putCAS(t, doc.Uploads[0], payload, http.StatusCreated)

	doc = requestCASCredentials(t, fx, `{"product":"demo","blobs":[{"sha256":"`+digest+`","size":5}]}`)
	if len(doc.Uploads) != 0 {
		t.Fatalf("cas hit should return no uploads: %v", doc.Uploads)
	}
}

func TestCASPutRejectsWrongHash(t *testing.T) {
	fx := newAgentFixture(t, agentFixtureOpts{})
	digest := strings.Repeat("a", 64)
	doc := requestCASCredentials(t, fx, `{"product":"demo","blobs":[{"sha256":"`+digest+`","size":5}]}`)
	putCAS(t, doc.Uploads[0], []byte("hello"), http.StatusBadRequest)
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
					"type":    "static-http",
					"baseUrl": "https://example.invalid/rup/",
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

func TestCASCredentialsAllowsMultiplePublishTargets(t *testing.T) {
	mirror := startFakeRelkitServe(t, "serve-token")
	fx := newAgentFixture(t, agentFixtureOpts{
		patchProfile: func(profile *config.PublishProfile) {
			profile.Backends["mirror"] = mirror.backendConfig()
			profile.PublishTo = []string{"serve", "mirror"}
		},
	})
	digest := strings.Repeat("a", 64)
	doc := requestCASCredentials(t, fx, `{"product":"demo","blobs":[{"sha256":"`+digest+`","size":1}]}`)
	if len(doc.Uploads) != 1 {
		t.Fatalf("uploads=%v", doc.Uploads)
	}
	if !strings.Contains(doc.Uploads[0].Requests[0].URL, "/cas/") {
		t.Fatalf("url=%s", doc.Uploads[0].Requests[0].URL)
	}
}

func TestThinStagedPublishesFromCASWithoutLocalArtifact(t *testing.T) {
	fx := newAgentFixture(t, agentFixtureOpts{})
	payload := []byte("hello")
	digest := model.Sha256Bytes(payload)
	doc := requestCASCredentials(t, fx, `{"product":"demo","blobs":[{"sha256":"`+digest+`","size":5}]}`)
	putCAS(t, doc.Uploads[0], payload, http.StatusCreated)

	thin := stripArtifactsFromTar(t, fx.tarball)
	req, _ := http.NewRequest(http.MethodPut, fx.ts.URL+"/v1/staged/demo/1.0.0", bytes.NewReader(thin))
	req.Header.Set("Authorization", "Bearer "+fx.token)
	resp, err := http.DefaultClient.Do(req)
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
	got := fx.ingest.get("artifact/demo/1.0.0/app.bin")
	if !bytes.Equal(got, payload) {
		t.Fatalf("artifact=%q", got)
	}
}

func TestThinStagedMaterializesSecondBackend(t *testing.T) {
	mirror := startFakeRelkitServe(t, "serve-token")
	fx := newAgentFixture(t, agentFixtureOpts{
		patchProfile: func(profile *config.PublishProfile) {
			profile.Backends["mirror"] = mirror.backendConfig()
			profile.PublishTo = []string{"serve", "mirror"}
		},
	})
	payload := []byte("hello")
	digest := model.Sha256Bytes(payload)
	doc := requestCASCredentials(t, fx, `{"product":"demo","blobs":[{"sha256":"`+digest+`","size":5}]}`)
	putCAS(t, doc.Uploads[0], payload, http.StatusCreated)

	req, _ := http.NewRequest(http.MethodPut, fx.ts.URL+"/v1/staged/demo/1.0.0", bytes.NewReader(stripArtifactsFromTar(t, fx.tarball)))
	req.Header.Set("Authorization", "Bearer "+fx.token)
	resp, err := http.DefaultClient.Do(req)
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
	if !bytes.Contains(body, []byte("materialized from serve")) {
		t.Fatalf("publish log=%s", body)
	}
	if !bytes.Equal(fx.ingest.get("artifact/demo/1.0.0/app.bin"), payload) {
		t.Fatal("ingest missing artifact")
	}
	if !bytes.Equal(mirror.get("artifact/demo/1.0.0/app.bin"), payload) {
		t.Fatal("mirror missing artifact")
	}
	casKey, _ := model.CasKey(digest)
	if len(mirror.get(casKey)) != 0 {
		t.Fatal("mirror should not store cas/")
	}
}

func putCAS(t *testing.T, doc casUploadDocument, payload []byte, want int) {
	t.Helper()
	if len(doc.Requests) == 0 {
		t.Fatal("no requests")
	}
	req, _ := http.NewRequest(http.MethodPut, doc.Requests[0].URL, bytes.NewReader(payload))
	for key, value := range doc.Requests[0].Headers {
		req.Header.Set(key, value)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != want {
		t.Fatalf("PUT status=%d want %d", resp.StatusCode, want)
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
