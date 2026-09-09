package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"firoyang.com/relkit/internal/publishproto"
)

func TestCASCapabilityRejectsExpiredAndTamperedTickets(t *testing.T) {
	cfg, _ := newTestConfig(t, true)
	srv := newLocalServer(t, cfg)
	t.Cleanup(srv.Close)

	body := []byte("capability body")
	sum := sha256.Sum256(body)
	key := "cas/" + hex.EncodeToString(sum[:])
	baseReq, _ := http.NewRequest(http.MethodPut, srv.URL+"/"+key, nil)

	expired, err := cfg.signCASPutURL(baseReq, key, int64(len(body)), time.Now().Add(-time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	resp := putCapability(t, expired, body)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expired ticket status = %d, want 401", resp.StatusCode)
	}
	resp.Body.Close()

	valid, err := cfg.signCASPutURL(baseReq, key, int64(len(body)), time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	tampered, _ := url.Parse(valid)
	query := tampered.Query()
	query.Set("size", "1")
	tampered.RawQuery = query.Encode()
	resp = putCapability(t, tampered.String(), body)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("tampered ticket status = %d, want 401", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestCASMintReturnsAbsoluteURLAndRegistersLease(t *testing.T) {
	cfg, _ := newTestConfig(t, true)
	srv := newLocalServer(t, cfg)
	t.Cleanup(srv.Close)
	key := "cas/" + strings.Repeat("a", 64)
	req, _ := http.NewRequest(http.MethodPost, srv.URL+casUploadsPath,
		strings.NewReader(`{"key":"`+key+`","size":12,"ttl":60}`))
	req.Header.Set("Authorization", "Bearer "+testToken)
	req.Header.Set(publishproto.ProtocolHeader, "2")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !bytes.Contains(raw, []byte(`"url":"http`)) {
		t.Fatalf("mint status = %d body = %s", resp.StatusCode, raw)
	}
	if !cfg.gc.casProtected(key, time.Now().Add(-48*time.Hour), time.Now()) {
		t.Fatal("mint did not register a CAS lease")
	}
}

func TestCASCapabilityUploadAndServerSideCopy(t *testing.T) {
	cfg, dir := newTestConfig(t, true)
	srv := newLocalServer(t, cfg)
	t.Cleanup(srv.Close)

	body := []byte("promote me")
	sum := sha256.Sum256(body)
	key := "cas/" + hex.EncodeToString(sum[:])
	baseReq, _ := http.NewRequest(http.MethodPut, srv.URL+"/"+key, nil)
	signed, err := cfg.signCASPutURL(baseReq, key, int64(len(body)), time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	resp := putCapability(t, signed, body)
	if resp.StatusCode != http.StatusCreated {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("capability PUT status = %d: %s", resp.StatusCode, raw)
	}
	resp.Body.Close()

	dst := "artifact/app/1.0.0/app.zip"
	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/"+dst, strings.NewReader(""))
	req.Header.Set("Authorization", "Bearer "+testToken)
	req.Header.Set(publishproto.ProtocolHeader, "2")
	req.Header.Set(copySourceHeader, key)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusCreated {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("COPY status = %d: %s", resp.StatusCode, raw)
	}
	resp.Body.Close()
	got, err := cfg.root.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, body) {
		t.Fatalf("copied body = %q, want %q (root %s)", got, body, dir)
	}
}

func putCapability(t *testing.T, rawURL string, body []byte) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPut, rawURL, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}
