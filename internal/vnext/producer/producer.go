// Package producer runs the only product-build extension accepted by the
// vNext release core and verifies its ArtifactSet before any staging occurs.
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
	"github.com/shichao402/relkit/internal/vnext/resolution"
	"github.com/shichao402/relkit/internal/vnext/runtimebootstrap"
)

const InputSchema = "relkit.artifact-producer-input/1"

type Input struct {
	Schema            string                               `json:"schema"`
	ResolutionDigest  string                               `json:"resolutionDigest"`
	Product           string                               `json:"product"`
	Version           string                               `json:"version"`
	Channel           string                               `json:"channel"`
	Target            string                               `json:"target"`
	OutputDirectory   string                               `json:"outputDirectory"`
	RuntimeBootstraps map[string]runtimebootstrap.Document `json:"runtimeBootstraps,omitempty"`
}

type Options struct {
	ProjectRoot       string
	OutputDir         string
	Version           string
	Channel           string
	Target            string
	ProtectedPaths    []string
	RuntimeBootstraps map[string]runtimebootstrap.Document
}

func Run(
	ctx context.Context,
	product project.Document,
	resolved resolution.Document,
	options Options,
) (*artifactset.Document, error) {
	if err := product.Validate(); err != nil {
		return nil, fmt.Errorf("project: %w", err)
	}
	if err := resolved.Validate(); err != nil {
		return nil, fmt.Errorf("resolution: %w", err)
	}
	projectDigest, err := product.Digest()
	if err != nil {
		return nil, err
	}
	if resolved.ProjectDigest != projectDigest {
		return nil, fmt.Errorf("resolution belongs to project digest %s, current project is %s", resolved.ProjectDigest, projectDigest)
	}
	if product.ArtifactProducer == nil {
		return nil, fmt.Errorf("project has no artifactProducer")
	}
	if options.Version == "" || options.Channel == "" || options.Target == "" {
		return nil, fmt.Errorf("version, channel, and target are required")
	}
	if !slices.Contains(resolved.AllowedChannels, options.Channel) {
		return nil, fmt.Errorf("resolution identity %s does not allow channel %q", resolved.Identity, options.Channel)
	}
	if !slices.Contains(product.CapabilityFingerprint.OSArch, options.Target) {
		return nil, fmt.Errorf("target %q is not declared by the project", options.Target)
	}
	if err := validateRuntimeBootstraps(product, resolved, options.RuntimeBootstraps); err != nil {
		return nil, err
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
		Schema:            InputSchema,
		ResolutionDigest:  resolved.Digest,
		Product:           product.Product,
		Version:           options.Version,
		Channel:           options.Channel,
		Target:            options.Target,
		OutputDirectory:   outputDir,
		RuntimeBootstraps: options.RuntimeBootstraps,
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
		output.Channel != options.Channel ||
		output.ResolutionDigest != resolved.Digest {
		return nil, fmt.Errorf("artifact producer returned a different product/version/channel/resolution identity")
	}
	if err := output.VerifyFiles(outputDir, product.SelectorDimensions); err != nil {
		return nil, err
	}
	return &output, nil
}

func validateRuntimeBootstraps(
	product project.Document,
	resolved resolution.Document,
	values map[string]runtimebootstrap.Document,
) error {
	declared := make(map[string]project.Subscription, len(product.RuntimeProfileSubscriptions))
	for _, subscription := range product.RuntimeProfileSubscriptions {
		declared[subscription.ProductRef] = subscription
		value, ok := values[subscription.ProductRef]
		if !ok {
			return fmt.Errorf("runtime bootstrap %q is required by the project", subscription.ProductRef)
		}
		if err := value.Validate(); err != nil {
			return fmt.Errorf("runtime bootstrap %q: %w", subscription.ProductRef, err)
		}
		if value.ResolutionDigest != resolved.Digest {
			return fmt.Errorf("runtime bootstrap %q belongs to resolution %s, current resolution is %s", subscription.ProductRef, value.ResolutionDigest, resolved.Digest)
		}
		if value.Profile.Product != subscription.ProductRef {
			return fmt.Errorf("runtime bootstrap %q embeds product %q", subscription.ProductRef, value.Profile.Product)
		}
		if value.RootFingerprint != subscription.RootFingerprint {
			return fmt.Errorf("runtime bootstrap %q root fingerprint does not match project", subscription.ProductRef)
		}
		if digest := resolved.RuntimeProfileDigests[subscription.ProductRef]; digest != value.ProfileDigest {
			return fmt.Errorf("runtime bootstrap %q profile digest %s does not match resolution %s", subscription.ProductRef, value.ProfileDigest, digest)
		}
	}
	for productRef := range values {
		if _, ok := declared[productRef]; !ok {
			return fmt.Errorf("runtime bootstrap %q was provided but is not declared by the project", productRef)
		}
	}
	return nil
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
