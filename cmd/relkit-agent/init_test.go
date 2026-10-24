package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shichao402/relkit/internal/agentadmin"
)

// TestInitBootstrapMintsOneShotCredential covers the only job `relkit-agent init`
// has left: mint the credential that creates the first operator (requirement
// 13.2). The credential stops working the moment an account exists, so the second
// call must be refused rather than silently rotate it.
func TestInitBootstrapMintsOneShotCredential(t *testing.T) {
	root := t.TempDir()
	cfgPath := filepath.Join(root, "agent.json")
	if err := os.WriteFile(cfgPath, []byte("{\n  \"stateDir\": \""+filepath.ToSlash(filepath.Join(root, "state"))+"\"\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := runInit(&out, []string{"-config", cfgPath, "-bootstrap"}); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	if !strings.Contains(text, "shown once") {
		t.Fatalf("bootstrap output does not warn that the value is shown once:\n%s", text)
	}
	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	store := cfg.admin
	if store == nil || !store.BootstrapLive() {
		t.Fatal("bootstrap is not live after the command reported success")
	}
	// Second call must be a refusal, not a silent rotation: rotating would
	// invalidate the value an operator already copied.
	if err := runInit(&bytes.Buffer{}, []string{"-config", cfgPath, "-bootstrap"}); err == nil {
		t.Fatal("bootstrap was minted twice; the first credential would be silently invalidated")
	}
	// Replacing it is an explicit, separate decision.
	out.Reset()
	if err := runInit(&out, []string{"-config", cfgPath, "-bootstrap", "-reset"}); err != nil {
		t.Fatalf("reset refused: %v", err)
	}
	if !strings.Contains(out.String(), "shown once") {
		t.Fatal("reset did not report the new credential")
	}
}

// TestInitRejectsRemovedProductFlags pins the removal itself. These spellings
// must not exist as deprecated no-ops: a script that still calls them has to fail
// loudly (requirement 11.3).
func TestInitRejectsRemovedProductFlags(t *testing.T) {
	root := t.TempDir()
	cfgPath := filepath.Join(root, "agent.json")
	if err := os.WriteFile(cfgPath, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"-config", cfgPath, "-product", "loom"},
		{"-config", cfgPath, "-product", "loom", "-token-only"},
		{"-config", cfgPath, "-product", "loom", "-share-with", "svn-auto-merge"},
		{"-config", cfgPath, "-list-products"},
		{"-config", cfgPath, "-product", "loom", "-remove"},
	} {
		if err := runInit(&bytes.Buffer{}, args); err == nil {
			t.Fatalf("init %v succeeded; the flag is supposed to be gone", args)
		}
	}
}

// TestLoadConfigRejectsLegacyProductFields covers the config-file half of the
// epoch rule: a server that still names products or upload tokens must refuse to
// start instead of authorizing nobody (requirements 11.3, 13.6).
func TestLoadConfigRejectsLegacyProductFields(t *testing.T) {
	cases := []string{
		`{"products": {"loom": {"root": "/srv/relkit/loom"}}}`,
		`{"uploadTokens": [{"file": "tokens/loom.token", "products": ["loom"]}]}`,
		`{"uploadToken": "instance-wide"}`,
		`{"uploadTokenFile": "token"}`,
	}
	for _, doc := range cases {
		root := t.TempDir()
		cfgPath := filepath.Join(root, "agent.json")
		if err := os.WriteFile(cfgPath, []byte(doc), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := LoadConfig(cfgPath)
		if err == nil {
			t.Fatalf("%s was accepted; the field is gone and must be an error", doc)
		}
		if !strings.Contains(err.Error(), "gone") {
			t.Fatalf("%s: error does not explain the removal: %v", doc, err)
		}
	}
}

// TestLoadConfigProjectsAdminProducts checks that registrations reach the request
// path with absolute roots and a defaulted profile: registration records what the
// operator typed, and the agent runs from wherever the service started.
func TestLoadConfigProjectsAdminProducts(t *testing.T) {
	root := t.TempDir()
	stateDir := filepath.Join(root, "state")
	productRoot := filepath.Join(root, "products-root", "loom")
	if err := os.MkdirAll(productRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	writeAdminDoc(t, filepath.Join(stateDir, "admin.json"), map[string]string{"loom": productRoot}, "tok", "loom")
	cfgPath := filepath.Join(root, "agent.json")
	if err := os.WriteFile(cfgPath, []byte("{\n  \"stateDir\": \""+filepath.ToSlash(stateDir)+"\"\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	product, ok := cfg.Products["loom"]
	if !ok {
		t.Fatalf("registration did not reach the runtime view: %+v", cfg.Products)
	}
	if product.Root != productRoot {
		t.Fatalf("root = %q, want %q", product.Root, productRoot)
	}
	wantProfile := filepath.Join(root, "products", "loom.json")
	if product.Profile != wantProfile {
		t.Fatalf("profile = %q, want %q", product.Profile, wantProfile)
	}
	if len(cfg.credentials) != 1 || !cfg.credentials[0].Allows("loom") {
		t.Fatalf("credentials = %+v", cfg.credentials)
	}
	if cfg.credentials[0].Allows("other") {
		t.Fatal("a single-product credential authorized another product")
	}
}

// TestAdminStateFileIsOptIn confirms the state file is only created when
// something changes. A read-only mirror must not grow a management document just
// because it started.
func TestAdminStateFileIsOptIn(t *testing.T) {
	root := t.TempDir()
	stateDir := filepath.Join(root, "state")
	cfgPath := filepath.Join(root, "agent.json")
	if err := os.WriteFile(cfgPath, []byte("{\n  \"stateDir\": \""+filepath.ToSlash(stateDir)+"\"\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(cfgPath); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(stateDir, "admin.json")); !os.IsNotExist(err) {
		t.Fatalf("loading the config created an admin document: %v", err)
	}
}

// TestAdminStoreRoundTripThroughConfig is the integration seam: a document
// written by the store is exactly what LoadConfig reads back, including roles and
// product assignments.
func TestAdminStoreRoundTripThroughConfig(t *testing.T) {
	root := t.TempDir()
	stateDir := filepath.Join(root, "state")
	cfgPath := filepath.Join(root, "agent.json")
	if err := os.WriteFile(cfgPath, []byte("{\n  \"stateDir\": \""+filepath.ToSlash(stateDir)+"\"\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cfg.admin.BootstrapToken(); err != nil {
		t.Fatal(err)
	}
	// A live bootstrap is not replaced by a second call: that would invalidate the
	// value an operator already has.
	if second, err := cfg.admin.BootstrapToken(); err == nil {
		t.Fatalf("a second bootstrap token was minted: %s", second)
	}
	// Reload through the store to prove the document survives a round trip.
	reopened, err := agentadmin.Open(cfg.admin.Path())
	if err != nil {
		t.Fatal(err)
	}
	if !reopened.BootstrapLive() {
		t.Fatal("bootstrap did not survive the reopen")
	}
	if err := json.NewEncoder(&bytes.Buffer{}).Encode(reopened.Products()); err != nil {
		t.Fatal(err)
	}
}
