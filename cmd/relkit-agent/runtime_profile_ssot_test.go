package main

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"testing"

	"github.com/shichao402/relkit/internal/model"
	"github.com/shichao402/relkit/internal/publishproto"
	"github.com/shichao402/relkit/internal/vnext/runtimeprofile"
)

// TestAdminRegistrationSignsFirstRuntimeProfile is ADR 0024: registering a
// product is the whole of what a host needs before `relkit bootstrap build`.
// The server derives the product's publish config from its own defaults and
// signs the first Runtime Profile in the same request.
func TestAdminRegistrationSignsFirstRuntimeProfile(t *testing.T) {
	fx := newAdminFixtureWith(t, agentFixtureOpts{omitProfile: true, publishDefaults: true})
	status, body := fx.do(t, http.MethodPost, "/v1/admin/products", fx.admin, map[string]any{
		"id": "fresh", "root": t.TempDir(),
	})
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%s", status, body)
	}
	var resp struct {
		ProfileMissing bool `json:"profileMissing"`
		RuntimeProfile struct {
			Signed            bool   `json:"signed"`
			DirectorySequence int64  `json:"directorySequence"`
			Error             string `json:"error"`
		} `json:"runtimeProfile"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.RuntimeProfile.Signed || resp.RuntimeProfile.DirectorySequence < 1 {
		t.Fatalf("registration did not sign the first profile: %s", body)
	}
	if resp.ProfileMissing {
		t.Fatalf("publish defaults should make a per-product profile unnecessary: %s", body)
	}
	if !fx.ingest.hasPrefix("runtime-profile/fresh/") {
		t.Fatal("Runtime Profile object was not published")
	}
	if len(fx.ingest.get("directory/fresh.pb")) == 0 {
		t.Fatal("directory pointer was not published")
	}
}

// TestAdminRegistrationWithoutPublishDefaultsStillRegisters: signing is a
// consequence of registering, not a precondition. A server without publish
// defaults registers the product and reports why nothing was signed.
func TestAdminRegistrationWithoutPublishDefaultsStillRegisters(t *testing.T) {
	fx := newAdminFixtureWith(t, agentFixtureOpts{omitProfile: true})
	status, body := fx.do(t, http.MethodPost, "/v1/admin/products", fx.admin, map[string]any{
		"id": "fresh", "root": t.TempDir(),
	})
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%s", status, body)
	}
	var resp struct {
		ProfileMissing bool `json:"profileMissing"`
		RuntimeProfile struct {
			Signed bool   `json:"signed"`
			Error  string `json:"error"`
		} `json:"runtimeProfile"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.RuntimeProfile.Signed || resp.RuntimeProfile.Error == "" || !resp.ProfileMissing {
		t.Fatalf("expected an unsigned, profile-missing registration: %s", body)
	}
	if fx.ingest.hasPrefix("runtime-profile/fresh/") {
		t.Fatal("nothing should have been published")
	}
}

// TestEnsureRuntimeProfilesSignsOnlyMissing: startup reconciliation signs a
// product that has no directory and leaves one that has a directory alone.
func TestEnsureRuntimeProfilesSignsOnlyMissing(t *testing.T) {
	fx := newAgentFixture(t, agentFixtureOpts{omitProfile: true, publishDefaults: true})
	srv := NewServer(fx.cfg)
	var lines []string
	srv.ensureRuntimeProfiles(func(line string) { lines = append(lines, line) })
	first := fx.ingest.get("directory/demo.pb")
	if len(first) == 0 {
		t.Fatalf("missing product was not signed; log=%v", lines)
	}
	lines = nil
	srv.ensureRuntimeProfiles(func(line string) { lines = append(lines, line) })
	if !bytes.Equal(first, fx.ingest.get("directory/demo.pb")) {
		t.Fatal("a product with a directory was re-signed")
	}
	if len(lines) != 0 {
		t.Fatalf("a skipped product should log nothing, got %v", lines)
	}
}

// TestRuntimeProfileWithoutProject: every input is a server fact, so the
// re-sign endpoint needs only the product id, and the signed Profile carries
// the server's channel set.
func TestRuntimeProfileWithoutProject(t *testing.T) {
	fx := newAgentFixture(t, agentFixtureOpts{omitProfile: true, publishDefaults: true})
	payload, _ := json.Marshal(runtimeProfileRequest{Product: "demo"})
	req, _ := http.NewRequest(http.MethodPost, fx.ts.URL+"/v1/runtime-profile", bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+fx.token)
	req.Header.Set("Content-Type", "application/json")
	publishproto.Apply(req.Header)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.StatusCode, raw)
	}
	var result struct {
		Profile runtimeprofile.Document `json:"profile"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(result.Profile.AllowedChannels, model.StandardChannels()) {
		t.Fatalf("allowedChannels=%v", result.Profile.AllowedChannels)
	}
	if err := result.Profile.Verify(map[string]ed25519.PublicKey{fx.rootKeyID: fx.rootPublic()}); err != nil {
		t.Fatalf("profile signature: %v", err)
	}
}

// TestPerProductProfileFileWinsOverDefaults: an existing per-product file is
// still honoured, so servers configured before ADR 0024 keep their behaviour.
func TestPerProductProfileFileWinsOverDefaults(t *testing.T) {
	fx := newAgentFixture(t, agentFixtureOpts{publishDefaults: true})
	pc, ok := NewServer(fx.cfg).product("demo")
	if !ok {
		t.Fatal("demo not registered")
	}
	if pc.Publish == nil {
		t.Fatal("defaults should still be derived for the product")
	}
	_, source, err := pc.loadPublishProfile()
	if err != nil {
		t.Fatal(err)
	}
	if source != fx.profilePath {
		t.Fatalf("source=%q want the per-product file %q", source, fx.profilePath)
	}
}
