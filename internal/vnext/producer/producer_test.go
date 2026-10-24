package producer

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shichao402/relkit/internal/vnext/artifactset"
	"github.com/shichao402/relkit/internal/vnext/project"
)

func TestArtifactProducerHelperProcess(t *testing.T) {
	if os.Getenv("RELKIT_PRODUCER_HELPER") != "1" {
		return
	}
	var input Input
	if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
		os.Exit(81)
	}
	// The producer input no longer carries a resolution: it describes what the
	// build produced, nothing about which relkit version consumes it.
	if input.Schema != InputSchema {
		os.Exit(84)
	}
	content := []byte("verified candidate")
	if err := os.WriteFile(filepath.Join(input.OutputDirectory, "demo.zip"), content, 0o644); err != nil {
		os.Exit(82)
	}
	sum := sha256.Sum256(content)
	output := artifactset.Document{
		Schema:      artifactset.Schema,
		Product:     input.Product,
		Version:     input.Version,
		ReleaseCode: 12,
		Channel:     input.Channel,
		Artifacts: []artifactset.Artifact{
			{
				ID:        "desktop-payload",
				Path:      "demo.zip",
				Kind:      artifactset.KindPayload,
				Platform:  input.Target,
				Selectors: map[string]string{"audience": "desktop"},
				SHA256:    "sha256:" + fmt.Sprintf("%x", sum[:]),
				Size:      int64(len(content)),
			},
		},
	}
	if err := json.NewEncoder(os.Stdout).Encode(output); err != nil {
		os.Exit(83)
	}
	os.Exit(0)
}

func producerFixture(t *testing.T, root string) project.Document {
	t.Helper()
	productDocument := project.Document{
		Schema:             project.Schema,
		Product:            "demo",
		Server:             "https://updates.example",
		RootPublicKey:      base64.StdEncoding.EncodeToString(make([]byte, 32)),
		Channels:           []string{"dev", "stable"},
		SelectorDimensions: []string{"audience", "os-arch"},
		ArtifactProducer: &project.Command{
			Command:         []string{os.Args[0], "-test.run=TestArtifactProducerHelperProcess"},
			ExtensionSchema: "relkit.extension/1",
			EnvAllowlist:    []string{"RELKIT_PRODUCER_HELPER"},
			Permissions:     []string{"read-workspace", "write-out-dir", "exec-self"},
		},
	}
	if err := productDocument.Validate(); err != nil {
		t.Fatal(err)
	}
	t.Setenv("RELKIT_PRODUCER_HELPER", "1")
	return productDocument
}

func TestRunVerifiesProducerOutput(t *testing.T) {
	root := t.TempDir()
	productDocument := producerFixture(t, root)
	output, err := Run(context.Background(), productDocument, Options{
		ProjectRoot: root,
		Version:     "1.2.3",
		Channel:     "dev",
		Target:      "darwin-arm64",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(output.Artifacts) != 1 || output.Artifacts[0].Path != "demo.zip" {
		t.Fatalf("unexpected ArtifactSet: %#v", output)
	}
	if output.Channel != "dev" || output.Version != "1.2.3" {
		t.Fatalf("unexpected ArtifactSet identity: %#v", output)
	}
}

func TestRunRejectsUndeclaredChannel(t *testing.T) {
	root := t.TempDir()
	productDocument := producerFixture(t, root)
	productDocument.Channels = []string{"dev"}
	_, err := Run(context.Background(), productDocument, Options{
		ProjectRoot: root,
		Version:     "1.2.3",
		Channel:     "stable",
		Target:      "darwin-arm64",
	})
	if err == nil || !strings.Contains(err.Error(), "does not declare channel") {
		t.Fatalf("unexpected result: %v", err)
	}
}

func TestRunRequiresTheProducerCommand(t *testing.T) {
	root := t.TempDir()
	productDocument := producerFixture(t, root)
	productDocument.ArtifactProducer = nil
	if _, err := Run(context.Background(), productDocument, Options{
		ProjectRoot: root,
		Version:     "1.2.3",
		Channel:     "dev",
		Target:      "darwin-arm64",
	}); err == nil || !strings.Contains(err.Error(), "no artifactProducer") {
		t.Fatalf("unexpected result: %v", err)
	}
}

func TestRunRejectsEscapeFromProjectRoot(t *testing.T) {
	root := t.TempDir()
	productDocument := producerFixture(t, root)
	outside := filepath.Join(filepath.Dir(root), "outside")
	if _, err := Run(context.Background(), productDocument, Options{
		ProjectRoot: root,
		OutputDir:   outside,
		Version:     "1.2.3",
		Channel:     "dev",
		Target:      "darwin-arm64",
	}); err == nil || !strings.Contains(err.Error(), "outside project root") {
		t.Fatalf("unexpected result: %v", err)
	}
}
