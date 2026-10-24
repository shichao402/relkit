// Package producer runs the only product-build extension the release core
// accepts and verifies its ArtifactSet before any staging occurs.
package producer

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/shichao402/relkit/internal/vnext/artifactset"
	"github.com/shichao402/relkit/internal/vnext/extension"
	"github.com/shichao402/relkit/internal/vnext/project"
)

const InputSchema = "relkit.artifact-producer-input/2"

// Input is the JSON document handed to the product's artifact-producer on
// stdin. It no longer carries a resolution digest: the server, not the host
// repository, decides which relkit version runs (requirement 12.1).
type Input struct {
	Schema          string `json:"schema"`
	Product         string `json:"product"`
	Version         string `json:"version"`
	Channel         string `json:"channel"`
	Target          string `json:"target"`
	OutputDirectory string `json:"outputDirectory"`
}

type Options struct {
	ProjectRoot    string
	OutputDir      string
	Version        string
	Channel        string
	Target         string
	ProtectedPaths []string
}

func Run(
	ctx context.Context,
	product project.Document,
	options Options,
) (*artifactset.Document, error) {
	if err := product.Validate(); err != nil {
		return nil, fmt.Errorf("project: %w", err)
	}
	if product.ArtifactProducer == nil {
		return nil, fmt.Errorf("project has no artifactProducer")
	}
	if options.Version == "" || options.Channel == "" || options.Target == "" {
		return nil, fmt.Errorf("version, channel, and target are required")
	}
	if len(product.Channels) != 0 && !slices.Contains(product.Channels, options.Channel) {
		return nil, fmt.Errorf("project does not declare channel %q", options.Channel)
	}
	if len(product.SelectorDimensions) != 0 && !slices.Contains(product.SelectorDimensions, "os-arch") {
		return nil, fmt.Errorf("project selectorDimensions must include os-arch for targeted builds")
	}
	root := options.ProjectRoot
	if root == "" {
		root = "."
	}
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	outputDir := options.OutputDir
	if outputDir == "" {
		outputDir = filepath.Join(absoluteRoot, ".relkit", "producer", options.Version, options.Target)
	} else if !filepath.IsAbs(outputDir) {
		outputDir = filepath.Join(absoluteRoot, outputDir)
	}
	if err := requireInside(absoluteRoot, outputDir); err != nil {
		return nil, err
	}
	if err := requireEmptyDirectory(outputDir); err != nil {
		return nil, err
	}
	protected, err := snapshot(options.ProtectedPaths)
	if err != nil {
		return nil, err
	}
	command := product.ArtifactProducer
	spec := extension.Spec{
		Schema:           extension.Schema,
		Kind:             extension.KindArtifactProducer,
		Command:          append([]string(nil), command.Command...),
		StdinSchema:      InputSchema,
		StdoutSchema:     artifactset.Schema,
		TimeoutSeconds:   command.TimeoutSeconds,
		WorkingDirectory: absoluteRoot,
		EnvAllowlist:     append([]string(nil), command.EnvAllowlist...),
		Permissions:      append([]string(nil), command.Permissions...),
	}
	input := Input{
		Schema:          InputSchema,
		Product:         product.Product,
		Version:         options.Version,
		Channel:         options.Channel,
		Target:          options.Target,
		OutputDirectory: outputDir,
	}
	var output artifactset.Document
	runErr := extension.Run(ctx, spec, input, &output)
	if err := verifyProtected(protected); err != nil {
		return nil, err
	}
	if runErr != nil {
		return nil, runErr
	}
	if output.Product != product.Product ||
		output.Version != options.Version ||
		output.Channel != options.Channel {
		return nil, fmt.Errorf("artifact producer returned a different product/version/channel")
	}
	if err := output.VerifyFiles(outputDir, product.SelectorDimensions); err != nil {
		return nil, err
	}
	return &output, nil
}

func requireInside(root, candidate string) error {
	relative, err := filepath.Rel(root, candidate)
	if err != nil {
		return err
	}
	if relative == ".." || filepath.IsAbs(relative) || len(relative) >= 3 && relative[:3] == ".."+string(filepath.Separator) {
		return fmt.Errorf("producer output directory %s is outside project root %s", candidate, root)
	}
	return nil
}

func requireEmptyDirectory(path string) error {
	entries, err := os.ReadDir(path)
	if err != nil {
		if os.IsNotExist(err) {
			return os.MkdirAll(path, 0o755)
		}
		return err
	}
	if len(entries) != 0 {
		return fmt.Errorf("producer output directory %s is not empty", path)
	}
	return nil
}

type protectedSnapshot struct {
	Path   string
	Digest [sha256.Size]byte
}

func snapshot(paths []string) ([]protectedSnapshot, error) {
	result := make([]protectedSnapshot, 0, len(paths))
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("protect %s: %w", path, err)
		}
		result = append(result, protectedSnapshot{Path: path, Digest: sha256.Sum256(data)})
	}
	return result, nil
}

func verifyProtected(snapshots []protectedSnapshot) error {
	for _, snapshot := range snapshots {
		data, err := os.ReadFile(snapshot.Path)
		if err != nil {
			return fmt.Errorf("extension modified protected input %s: %w", snapshot.Path, err)
		}
		if sha256.Sum256(data) != snapshot.Digest {
			return fmt.Errorf("extension modified protected input %s", snapshot.Path)
		}
	}
	return nil
}
