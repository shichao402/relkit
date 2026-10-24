// Package release implements the release path from a verified ArtifactSet to
// the frozen RUP wire documents the agent publishes.
//
// A stage no longer consumes a host-side resolution or a candidate bundle: the
// server decides which relkit version is current (requirement 12.1), so staging
// records only what the host actually built — its project declaration, the
// ArtifactSet, and the resulting staged document.
package release

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	rupv2 "github.com/shichao402/relkit/api/rup/v2"
	"github.com/shichao402/relkit/internal/model"
	legacystage "github.com/shichao402/relkit/internal/stage"
	"github.com/shichao402/relkit/internal/vnext/artifactset"
	"github.com/shichao402/relkit/internal/vnext/document"
	"github.com/shichao402/relkit/internal/vnext/project"
)

const (
	ProjectFile     = "relkit.project.json"
	ArtifactSetFile = "artifact-set.json"
)

type StageOptions struct {
	ProjectRoot string
	// ArtifactSetPath is the artifact-set.json the producer wrote. Its directory
	// is the root the ArtifactSet paths are resolved against.
	ArtifactSetPath string
	Code            int
	MinFrom         int
	NotesURL        string
	// RelkitVersion is the relkit component version the server currently marks
	// as current. It is recorded for diagnostics only; the host does not pick
	// it, the server does.
	RelkitVersion string
}

// Stage writes an immutable staging tree for the ArtifactSet produced from the
// given project document.
func Stage(
	product project.Document,
	artifacts artifactset.Document,
	options StageOptions,
) (*model.StagedDocument, error) {
	if err := product.Validate(); err != nil {
		return nil, fmt.Errorf("project: %w", err)
	}
	if err := artifacts.Validate(); err != nil {
		return nil, fmt.Errorf("artifact set: %w", err)
	}
	if artifacts.Product != product.Product {
		return nil, fmt.Errorf("ArtifactSet product %q does not match project product %q", artifacts.Product, product.Product)
	}
	if len(product.Channels) != 0 && !contains(product.Channels, artifacts.Channel) {
		return nil, fmt.Errorf("project does not declare channel %q", artifacts.Channel)
	}
	if options.Code != 0 && options.Code != artifacts.ReleaseCode {
		return nil, fmt.Errorf("stage code %d does not match ArtifactSet releaseCode %d", options.Code, artifacts.ReleaseCode)
	}
	releaseCode := artifacts.ReleaseCode
	root := options.ProjectRoot
	if root == "" {
		root = "."
	}
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	artifactRoot := filepath.Dir(options.ArtifactSetPath)
	if !filepath.IsAbs(artifactRoot) {
		artifactRoot, err = filepath.Abs(artifactRoot)
		if err != nil {
			return nil, err
		}
	}
	if err := artifacts.VerifyFiles(artifactRoot, product.SelectorDimensions); err != nil {
		return nil, err
	}
	stagedArtifacts := make([]*model.StagedArtifact, 0, len(artifacts.Artifacts))
	filenames := map[string]bool{}
	for _, artifact := range artifacts.Artifacts {
		filename := filepath.Base(filepath.FromSlash(artifact.Path))
		if filenames[filename] {
			return nil, fmt.Errorf("ArtifactSet paths collapse to duplicate staged filename %q", filename)
		}
		filenames[filename] = true
		selectors := cloneMap(artifact.Selectors)
		if artifact.Kind == artifactset.KindPayload {
			if value := selectors["apply"]; value != "" && value != "relkit-payload" {
				return nil, fmt.Errorf("payload %q reserves selector apply=relkit-payload", artifact.ID)
			}
			selectors["apply"] = "relkit-payload"
		}
		meta := map[string]any{}
		if artifact.Relations.InstallOf != "" {
			meta["relkit.installOf"] = artifact.Relations.InstallOf
		}
		if artifact.Relations.PayloadOf != "" {
			meta["relkit.payloadOf"] = artifact.Relations.PayloadOf
		}
		kind := string(artifact.Kind)
		switch artifact.Kind {
		case artifactset.KindInstaller, artifactset.KindPayload, artifactset.KindBlob:
		case artifactset.KindMetadata, artifactset.KindOther:
			meta["relkit.artifactKind"] = kind
			kind = "blob"
		default:
			return nil, fmt.Errorf("unsupported ArtifactSet kind %q", artifact.Kind)
		}
		value, err := model.NewStagedArtifact(
			artifact.ID,
			filename,
			artifact.Size,
			strings.TrimPrefix(artifact.SHA256, "sha256:"),
			kind,
			selectors,
			meta,
			filepath.Join(artifactRoot, filepath.FromSlash(artifact.Path)),
		)
		if err != nil {
			return nil, err
		}
		stagedArtifacts = append(stagedArtifacts, value)
	}
	staged, err := model.NewStagedDocument(
		product.Product,
		artifacts.Version,
		releaseCode,
		options.MinFrom,
		artifacts.Channel,
		stagedArtifacts,
		artifacts.Changelog,
		options.NotesURL,
		"",
	)
	if err != nil {
		return nil, err
	}

	finalDirectory := legacystage.StagingDir(absoluteRoot, artifacts.Version)
	if _, err := os.Stat(finalDirectory); err == nil {
		return nil, fmt.Errorf("staging tree %s already exists; staged releases are immutable", finalDirectory)
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	parent := filepath.Dir(finalDirectory)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return nil, err
	}
	temporary, err := os.MkdirTemp(parent, "."+filepath.Base(finalDirectory)+".")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(temporary)
	artifactDirectory := filepath.Join(temporary, "artifacts")
	if err := os.MkdirAll(artifactDirectory, 0o755); err != nil {
		return nil, err
	}
	for _, artifact := range staged.Artifacts {
		if err := copyFile(artifact.SourcePath, filepath.Join(artifactDirectory, artifact.Filename)); err != nil {
			return nil, err
		}
	}
	stagedBytes, err := rupv2.MarshalStaged(staged)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(temporary, "staged.pb"), stagedBytes, 0o644); err != nil {
		return nil, err
	}
	if err := document.Write(filepath.Join(temporary, ProjectFile), product); err != nil {
		return nil, err
	}
	if err := document.Write(filepath.Join(temporary, ArtifactSetFile), artifacts); err != nil {
		return nil, err
	}
	if err := os.Rename(temporary, finalDirectory); err != nil {
		return nil, err
	}
	return staged, nil
}

func copyFile(source, destination string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func cloneMap(source map[string]string) map[string]string {
	result := make(map[string]string, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
