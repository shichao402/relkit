package ops

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/shichao402/relkit/internal/vnext/artifactset"
	"github.com/shichao402/relkit/internal/vnext/project"
)

func TestArtifactSetPushPullIsDigestProtected(t *testing.T) {
	root := t.TempDir()
	projectPath := filepath.Join(root, "relkit.project.json")
	productDocument := project.Document{
		Schema: project.Schema, Product: "demo", Channels: []string{"dev"},
		OperatorRef: "fixture", TrustFingerprint: "sha256:" + strings.Repeat("a", 64),
		Roles:              []project.Role{project.RolePublisher},
		ArtifactProducer:   &project.Command{Command: []string{"fixture"}},
		SelectorDimensions: []string{"os", "arch"},
		CapabilityFingerprint: project.CapabilityFingerprint{
			OSArch: []string{"linux-amd64"}, Topology: "producer-only",
		},
	}
	rawProject, _ := json.Marshal(productDocument)
	if err := os.WriteFile(projectPath, rawProject, 0o644); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "source")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	content := []byte("immutable artifact")
	if err := os.WriteFile(filepath.Join(source, "demo.bin"), content, 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(content)
	value := artifactset.Document{
		Schema: artifactset.Schema, ResolutionDigest: "sha256:" + strings.Repeat("b", 64),
		Product: "demo", Version: "1.2.3", ReleaseCode: 12, Channel: "dev",
		Artifacts: []artifactset.Artifact{{
			ID: "demo", Path: "demo.bin", Kind: artifactset.KindBlob, Platform: "linux-amd64",
			Selectors: map[string]string{"os": "linux", "arch": "amd64"},
			SHA256:    "sha256:" + fmt.Sprintf("%x", sum[:]), Size: int64(len(content)),
		}},
	}
	rawSet, _ := json.Marshal(value)
	artifactSetPath := filepath.Join(source, artifactArchiveName)
	if err := os.WriteFile(artifactSetPath, rawSet, 0o644); err != nil {
		t.Fatal(err)
	}

	var mutex sync.Mutex
	var stored []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mutex.Lock()
		defer mutex.Unlock()
		switch r.Method {
		case http.MethodPut:
			username, password, ok := r.BasicAuth()
			if !ok || username != "writer" || password != "secret" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			stored, _ = io.ReadAll(r.Body)
			w.WriteHeader(http.StatusCreated)
		case http.MethodGet:
			if stored == nil {
				http.NotFound(w, r)
				return
			}
			_, _ = w.Write(stored)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()

	pushed, err := PushArtifactSet(t.Context(), PushArtifactSetOptions{
		ProjectPath: projectPath, ArtifactSetPath: artifactSetPath,
		URL: server.URL + "/demo.tar.gz", Username: "writer", Password: "secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(root, "pulled")
	pulled, err := PullArtifactSet(t.Context(), PullArtifactSetOptions{
		ProjectPath: projectPath, URL: server.URL + "/demo.tar.gz",
		ExpectedSHA256: pushed.ArchiveSHA256, OutputDirectory: destination,
	})
	if err != nil {
		t.Fatal(err)
	}
	if pulled.ArchiveSHA256 != pushed.ArchiveSHA256 {
		t.Fatalf("push=%+v pull=%+v", pushed, pulled)
	}
	if got, err := os.ReadFile(filepath.Join(destination, "demo.bin")); err != nil || string(got) != string(content) {
		t.Fatalf("pulled artifact=%q err=%v", got, err)
	}
	if _, err := PullArtifactSet(t.Context(), PullArtifactSetOptions{
		ProjectPath: projectPath, URL: server.URL + "/demo.tar.gz",
		ExpectedSHA256:  "sha256:" + strings.Repeat("c", 64),
		OutputDirectory: filepath.Join(root, "tampered"),
	}); err == nil || !strings.Contains(err.Error(), "digest") {
		t.Fatalf("expected digest rejection, got %v", err)
	}
}
