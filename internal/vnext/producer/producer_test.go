package producer

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shichao402/relkit/internal/vnext/artifactset"
	"github.com/shichao402/relkit/internal/vnext/contracts"
	"github.com/shichao402/relkit/internal/vnext/operator"
	"github.com/shichao402/relkit/internal/vnext/project"
	"github.com/shichao402/relkit/internal/vnext/resolution"
)

func TestArtifactProducerHelperProcess(t *testing.T) {
	if os.Getenv("RELKIT_PRODUCER_HELPER") != "1" {
		return
	}
	var input Input
	if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
		os.Exit(81)
	}
	content := []byte("verified candidate")
	if err := os.WriteFile(filepath.Join(input.OutputDirectory, "demo.zip"), content, 0o644); err != nil {
		os.Exit(82)
	}
	sum := sha256.Sum256(content)
	output := artifactset.Document{
		Schema:           artifactset.Schema,
		ResolutionDigest: input.ResolutionDigest,
		Product:          input.Product,
		Version:          input.Version,
		Channel:          input.Channel,
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

func TestRunVerifiesProducerOutput(t *testing.T) {
	root := t.TempDir()
	productDocument, resolved := producerFixture(t, root)
	output, err := Run(context.Background(), productDocument, resolved, Options{
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
}

func TestCandidateCannotProduceStableArtifacts(t *testing.T) {
	root := t.TempDir()
	productDocument, resolved := producerFixture(t, root)
	resolved.Identity = resolution.IdentityCandidate
	resolved.AllowedChannels = []string{"dev"}
	if err := resolved.Finalize(); err != nil {
		t.Fatal(err)
	}
	_, err := Run(context.Background(), productDocument, resolved, Options{
		ProjectRoot: root,
		Version:     "1.2.3",
		Channel:     "stable",
		Target:      "darwin-arm64",
	})
	if err == nil || !strings.Contains(err.Error(), "does not allow channel") {
		t.Fatalf("unexpected result: %v", err)
	}
}

func producerFixture(t *testing.T, root string) (project.Document, resolution.Document) {
	t.Helper()
	controlDigest := "sha256:" + strings.Repeat("a", 64)
	productDocument := project.Document{
		Schema:           project.Schema,
		Product:          "demo",
		Channels:         []string{"dev", "stable"},
		OperatorRef:      "prod",
		TrustFingerprint: controlDigest,
		Roles:            []project.Role{project.RolePublisher},
		ArtifactProducer: &project.Command{
			Command:         []string{os.Args[0], "-test.run=TestArtifactProducerHelperProcess"},
			ExtensionSchema: "relkit.extension/1",
			EnvAllowlist:    []string{"RELKIT_PRODUCER_HELPER"},
			Permissions:     []string{"read-workspace", "write-out-dir", "exec-self"},
		},
		SelectorDimensions: []string{"audience"},
		CapabilityFingerprint: project.CapabilityFingerprint{
			OSArch:   []string{"darwin-arm64"},
			Topology: "producer-only",
		},
	}
	t.Setenv("RELKIT_PRODUCER_HELPER", "1")
	projectDigest, err := productDocument.Digest()
	if err != nil {
		t.Fatal(err)
	}
	resolved := resolution.Document{
		Schema:               resolution.Schema,
		Identity:             resolution.IdentityProduction,
		AuthorityFingerprint: controlDigest,
		AllowedChannels:      []string{"dev", "stable"},
		ProjectDigest:        projectDigest,
		Operator: resolution.OperatorProof{
			DeploymentID:    "prod",
			ProfileDigest:   controlDigest,
			Sequence:        1,
			SelectionPolicy: operator.SelectionExact,
		},
		CatalogSnapshotDigest: controlDigest,
		SelectedDistribution: resolution.SelectedDistribution{
			Version: "v1.0.0",
			Commit:  strings.Repeat("b", 40),
			Digest:  controlDigest,
		},
		Components: []resolution.Component{
			{Name: "cli", Version: "v1.0.0", Platform: "darwin-arm64", SHA256: controlDigest, Source: "release-asset", Locator: "fixture"},
		},
		Contracts: []contracts.Capability{
			{Domain: contracts.DomainArtifactSet, Peer: "producer", ID: artifactset.Schema},
		},
	}
	if err := resolved.Finalize(); err != nil {
		t.Fatal(err)
	}
	return productDocument, resolved
}
