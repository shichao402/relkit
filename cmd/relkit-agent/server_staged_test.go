package main

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"net/http"
	"os"
	"testing"

	"github.com/shichao402/relkit/internal/stage"
	"github.com/shichao402/relkit/internal/vnext/project"
	vnextrelease "github.com/shichao402/relkit/internal/vnext/release"
)

// TestAgentRejectsProjectProductMismatch covers the check that replaced the
// retired release-policy.json: the staged project declaration, not a policy
// sidecar, must name the product in the route.
func TestAgentRejectsProjectProductMismatch(t *testing.T) {
	fx := newAgentFixture(t, agentFixtureOpts{
		patchProject: func(document *project.Document) {
			document.Product = "other"
		},
	})
	status, body := fx.putStaged(t, "demo", "1.0.0")
	if status != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", status, body)
	}
	if !bytes.Contains(body, []byte(`declares product "other"`)) {
		t.Fatalf("body=%s", body)
	}
}

// TestAgentRejectsForeignProjectRoot: a staged tree built against another
// server carries that server's rootPublicKey. Every document this agent emits
// is sealed with its own root, so publishing such a tree would hand hosts
// documents their embedded key cannot open. The upload is refused instead
// (requirements 10.7, 14.4).
func TestAgentRejectsForeignProjectRoot(t *testing.T) {
	foreign := make([]byte, ed25519.PublicKeySize)
	foreign[0] = 7
	fx := newAgentFixture(t, agentFixtureOpts{
		patchProject: func(document *project.Document) {
			document.RootPublicKey = base64.StdEncoding.EncodeToString(foreign)
		},
	})
	status, body := fx.putStaged(t, "demo", "1.0.0")
	if status != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", status, body)
	}
	if !bytes.Contains(body, []byte("does not match this server's root")) {
		t.Fatalf("body=%s", body)
	}
	if _, err := os.Stat(stage.StagingDir(fx.productRoot, "1.0.0")); !os.IsNotExist(err) {
		t.Fatalf("staged tree should be rejected and cleaned: %v", err)
	}
}

// TestAgentStagedRequiresProjectDeclaration is the successor of the retired
// "staged tree requires a resolution digest" test. The project declaration is
// the only provenance a staged tree carries now, so a tree without it is
// refused and the extracted directory is cleaned up.
func TestAgentStagedRequiresProjectDeclaration(t *testing.T) {
	fx := newAgentFixture(t, agentFixtureOpts{omitProject: true})
	status, body := fx.putStaged(t, "demo", "1.0.0")
	if status != http.StatusBadRequest {
		t.Fatalf("staged status=%d body=%s", status, body)
	}
	if !bytes.Contains(body, []byte(vnextrelease.ProjectFile)) {
		t.Fatalf("body=%s", body)
	}
	if _, err := os.Stat(stage.StagingDir(fx.productRoot, "1.0.0")); !os.IsNotExist(err) {
		t.Fatalf("staged tree should be rejected and cleaned: %v", err)
	}
}

// TestAgentStagedCarriesNoResolutionSidecar proves the epoch removal at the
// wire level: the staged tree the agent accepts and keeps holds exactly the
// project declaration, the ArtifactSet, staged.pb and the artifacts. A tree
// that still shipped a resolution sidecar would mean the old control plane is
// half alive.
func TestAgentStagedCarriesNoResolutionSidecar(t *testing.T) {
	fx := newAgentFixture(t, agentFixtureOpts{})
	status, body := fx.putStaged(t, "demo", "1.0.0")
	if status != http.StatusCreated {
		t.Fatalf("staged status=%d body=%s", status, body)
	}
	stagedRoot := stage.StagingDir(fx.productRoot, "1.0.0")
	for _, retired := range []string{"resolution.json", "resolution-bundle.json", "release-policy.json"} {
		if _, err := os.Stat(stagedRoot + string(os.PathSeparator) + retired); err == nil {
			t.Fatalf("staged tree still carries retired sidecar %s", retired)
		}
	}
	for _, required := range []string{"staged.pb", vnextrelease.ProjectFile, vnextrelease.ArtifactSetFile} {
		if _, err := os.Stat(stagedRoot + string(os.PathSeparator) + required); err != nil {
			t.Fatalf("staged tree is missing %s: %v", required, err)
		}
	}
}
