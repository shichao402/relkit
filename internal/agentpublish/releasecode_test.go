package agentpublish

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	rupv2 "github.com/shichao402/relkit/api/rup/v2"
	"github.com/shichao402/relkit/internal/stage"
)

func writeStagedCode(t *testing.T, root, version string, code int64) {
	t.Helper()
	data, err := rupv2.MarshalStaged(&rupv2.Staged{Version: version, Code: code, Channel: "dev"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(stage.StagingDir(root, version), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stage.StagedPath(root, version), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestRunRefusesStaleCodeBeforeUploading is the regression for a release that
// uploaded ~760 MB of CAS objects and only then learned its code was taken.
func TestRunRefusesStaleCodeBeforeUploading(t *testing.T) {
	root := t.TempDir()
	writeStagedCode(t, root, "1.0.0-dev.37+37", 37)
	var other []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/v1/release-code" {
			if r.URL.Query().Get("product") != "demo" || r.Header.Get("Authorization") != "Bearer tok" {
				t.Errorf("request=%s %s auth=%q", r.Method, r.URL, r.Header.Get("Authorization"))
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "product": "demo", "lastAllocated": 37, "highestPublished": 37})
			return
		}
		other = append(other, r.Method+" "+r.URL.Path)
		http.Error(w, "unexpected", http.StatusTeapot)
	}))
	defer server.Close()

	_, err := Run(t.Context(), Options{
		Root: root, Product: "demo", Version: "1.0.0-dev.37+37",
		AgentURL: server.URL, Token: "tok", Execute: true,
	})
	if err == nil || !strings.Contains(err.Error(), "not greater than the highest published code 37") {
		t.Fatalf("err=%v", err)
	}
	if len(other) != 0 {
		t.Fatalf("release touched %v before refusing the stale code", other)
	}
}

func TestCheckStagedCode(t *testing.T) {
	state := &ReleaseCodeState{Product: "demo", HighestPublished: 10}
	if err := CheckStagedCode(11, state, false); err != nil {
		t.Fatalf("fresh code refused: %v", err)
	}
	if err := CheckStagedCode(10, state, false); err == nil {
		t.Fatal("equal code accepted")
	}
	if err := CheckStagedCode(3, state, true); err != nil {
		t.Fatalf("backfill refused: %v", err)
	}
	if err := CheckStagedCode(3, nil, false); err != nil {
		t.Fatalf("missing state must not block: %v", err)
	}
}

func TestReleaseCodeOnOldAgentIsUnsupported(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	_, err := ReadReleaseCode(t.Context(), ReleaseCodeOptions{Product: "demo", AgentURL: server.URL, Token: "tok"})
	if !errors.Is(err, ErrReleaseCodeUnsupported) {
		t.Fatalf("err=%v", err)
	}
}

func TestReleaseCodeUnknownProductIsNotMistakenForOldAgent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"ok":false,"code":"release_code.unknown_product","error":"unknown product"}`))
	}))
	defer server.Close()
	_, err := ClaimReleaseCode(t.Context(), ReleaseCodeOptions{Product: "demo", AgentURL: server.URL, Token: "tok"})
	if err == nil || errors.Is(err, ErrReleaseCodeUnsupported) || !strings.Contains(err.Error(), "unknown product") {
		t.Fatalf("err=%v", err)
	}
}

func TestClaimReleaseCodeRejectsAnswerForAnotherProduct(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "product": "other", "code": 4})
	}))
	defer server.Close()
	_, err := ClaimReleaseCode(t.Context(), ReleaseCodeOptions{Product: "demo", AgentURL: server.URL, Token: "tok"})
	if err == nil || !strings.Contains(err.Error(), `"other"`) {
		t.Fatalf("err=%v", err)
	}
}
