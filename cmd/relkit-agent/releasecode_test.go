package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/shichao402/relkit/internal/publishproto"
)

func (f *agentFixture) releaseCode(t *testing.T, method, token, product string) (int, releaseCodeState, []byte) {
	t.Helper()
	url := f.ts.URL + releaseCodePath
	var body io.Reader
	if method == http.MethodGet {
		url += "?product=" + product
	} else {
		body = bytes.NewReader([]byte(`{"product":"` + product + `"}`))
	}
	req, _ := http.NewRequest(method, url, body)
	req.Header.Set("Authorization", "Bearer "+token)
	publishproto.Apply(req.Header)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var state releaseCodeState
	_ = json.Unmarshal(raw, &state)
	return resp.StatusCode, state, raw
}

// TestReleaseCodeClaimsContinueAbovePublishedHistory covers ADR 0025 end to
// end: the fixture stages and publishes code 1 the way a host did before the
// server owned numbering, and claims then continue above it, persist, and are
// enforced by the publish-time check.
func TestReleaseCodeClaimsContinueAbovePublishedHistory(t *testing.T) {
	fx := makeVNextAgentFixture(t, vNextFixtureOptions{})

	status, state, raw := fx.releaseCode(t, http.MethodGet, fx.token, "demo")
	if status != http.StatusOK || state.HighestPublished != 0 || state.LastAllocated != 0 {
		t.Fatalf("empty read status=%d body=%s", status, raw)
	}

	if status, body := fx.putStaged(t, "demo", "1.0.0"); status != http.StatusCreated {
		t.Fatalf("staged status=%d body=%s", status, body)
	}
	if status, body := fx.publish(t, `{"product":"demo","version":"1.0.0"}`); status != http.StatusOK {
		t.Fatalf("publish status=%d body=%s", status, body)
	}

	status, state, raw = fx.releaseCode(t, http.MethodPost, fx.token, "demo")
	if status != http.StatusOK || state.Code != 2 || state.HighestPublished != 1 {
		t.Fatalf("first claim status=%d body=%s", status, raw)
	}
	status, state, raw = fx.releaseCode(t, http.MethodPost, fx.token, "demo")
	if status != http.StatusOK || state.Code != 3 {
		t.Fatalf("second claim status=%d body=%s", status, raw)
	}
	status, state, raw = fx.releaseCode(t, http.MethodGet, fx.token, "demo")
	if status != http.StatusOK || state.LastAllocated != 3 || state.Code != 0 {
		t.Fatalf("read after claims status=%d body=%s", status, raw)
	}
	product, _ := fx.cfg.admin.Product("demo")
	if product.LastReleaseCode != 3 {
		t.Fatalf("admin state lastReleaseCode=%d, want 3", product.LastReleaseCode)
	}

	// Republishing the same code is still refused, now with a stable error code
	// and a message that points at the claim command instead of a stale doc.
	status, body := fx.publish(t, `{"product":"demo","version":"1.0.0","idempotencyKey":"again"}`)
	if status == http.StatusOK {
		t.Fatalf("republish of code 1 succeeded: %s", body)
	}
	if !strings.Contains(string(body), "release.code_not_monotonic") || !strings.Contains(string(body), "release-code claim") {
		t.Fatalf("republish body=%s", body)
	}
}

func TestReleaseCodeRequiresTheProductToken(t *testing.T) {
	fx := makeVNextAgentFixture(t, vNextFixtureOptions{})
	if status, _, raw := fx.releaseCode(t, http.MethodPost, "wrong-token", "demo"); status != http.StatusUnauthorized {
		t.Fatalf("bad token status=%d body=%s", status, raw)
	}
	if status, _, raw := fx.releaseCode(t, http.MethodPost, fx.token, "other"); status != http.StatusForbidden {
		t.Fatalf("foreign product status=%d body=%s", status, raw)
	}
	req, _ := http.NewRequest(http.MethodDelete, fx.ts.URL+releaseCodePath, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("DELETE status=%d", resp.StatusCode)
	}
}
