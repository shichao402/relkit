// Package release implements the vNext release path from verified ArtifactSet
// to the existing frozen RUP wire documents.
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
	"github.com/shichao402/relkit/internal/vnext/resolution"
)

const (
	ResolutionFile       = "resolution.json"
	ResolutionBundleFile = "resolution-bundle.json"
	ProjectFile          = "relkit.project.json"
	ArtifactSetFile      = "artifact-set.json"
)

type StageOptions struct {
	ProjectRoot              string
	ArtifactSetPath          string
	ResolutionBundlePath     string
	CandidateRootFingerprint string
	Code                     int
	MinFrom                  int
	NotesURL                 string
}

func Stage(
	product project.Document,
	resolved resolution.Document,
	artifacts artifactset.Document,
	options StageOptions,
) (*model.StagedDocument, error) {
	if err := product.Validate(); err != nil {
		return nil, fmt.Errorf("project: %w", err)
	}
	if err := resolved.Validate(); err != nil {
		return nil, fmt.Errorf("resolution: %w", err)
	}
	if options.ResolutionBundlePath == "" {
		return nil, fmt.Errorf("resolution bundle path is required")
	}
	bundle, err := resolution.LoadBundle(options.ResolutionBundlePath)
	if err != nil {
		return nil, err
	}
	verifiedResolution, err := bundle.Verify(resolution.BundlePolicy{
		DistributionRootFingerprint: product.TrustFingerprint,
		CandidateRootFingerprint:    options.CandidateRootFingerprint,
		OperatorDeploymentID:        resolved.Operator.DeploymentID,
		Identity:                    resolved.Identity,
	})
	if err != nil {
		return nil, fmt.Errorf("resolution bundle: %w", err)
	}
	if verifiedResolution.Digest != resolved.Digest {
		return nil, fmt.Errorf("resolution bundle digest does not match resolution")
	}
	if err := artifacts.Validate(); err != nil {
		return nil, fmt.Errorf("artifact set: %w", err)
	}
	projectDigest, err := product.Digest()
	if err != nil {
		return nil, err
	}
	if resolved.ProjectDigest != projectDigest {
		return nil, fmt.Errorf("resolution belongs to a different project document")
	}
	if artifacts.Product != product.Product || artifacts.ResolutionDigest != resolved.Digest {
		return nil, fmt.Errorf("ArtifactSet product/resolution does not match the selected project and resolution")
	}
	if !contains(resolved.AllowedChannels, artifacts.Channel) {
		return nil, fmt.Errorf("resolution identity %s does not allow channel %q", resolved.Identity, artifacts.Channel)
	}
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
		options.Code,
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
	staged.ResolutionDigest = resolved.Digest
	staged.OperatorDeploymentId = resolved.Operator.DeploymentID
	staged.ResolutionIdentity = string(resolved.Identity)

	finalDirectory := legacystage.StagingDir(absoluteRoot, artifacts.Version)
	if _, err := os.Stat(finalDirectory); err == nil {
		return nil, fmt.Errorf("staging tree %s already exists; vNext staged releases are immutable", finalDirectory)
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
	if err := document.Write(filepath.Join(temporary, ResolutionFile), resolved); err != nil {
		return nil, err
	}
	if err := document.Write(filepath.Join(temporary, ResolutionBundleFile), bundle); err != nil {
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
