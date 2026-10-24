// Package artifactset owns the versioned producer output consumed by the
// vNext release core.
package artifactset

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/shichao402/relkit/internal/model"
	"github.com/shichao402/relkit/internal/vnext/canonical"
	"github.com/shichao402/relkit/internal/vnext/document"
)

const Schema = "relkit.artifact-set/1"

type Kind string

const (
	KindInstaller Kind = "installer"
	KindPayload   Kind = "payload"
	KindBlob      Kind = "blob"
	KindMetadata  Kind = "metadata"
	KindOther     Kind = "other"
)

type Relations struct {
	InstallOf string `json:"installOf,omitempty"`
	PayloadOf string `json:"payloadOf,omitempty"`
}

type Artifact struct {
	ID        string            `json:"id"`
	Path      string            `json:"path"`
	Kind      Kind              `json:"kind"`
	Platform  string            `json:"platform,omitempty"`
	Selectors map[string]string `json:"selectors,omitempty"`
	SHA256    string            `json:"sha256"`
	Size      int64             `json:"size"`
	Relations Relations         `json:"relations,omitempty"`
}

type MergeIdentity struct {
	Product string `json:"product"`
	Version string `json:"version"`
	Channel string `json:"channel"`
}

type Document struct {
	Schema        string         `json:"schema"`
	Product       string         `json:"product"`
	Version       string         `json:"version"`
	ReleaseCode   int            `json:"releaseCode"`
	Channel       string         `json:"channel"`
	Changelog     string         `json:"changelog,omitempty"`
	Display       map[string]any `json:"display,omitempty"`
	MergeIdentity *MergeIdentity `json:"mergeIdentity,omitempty"`
	Artifacts     []Artifact     `json:"artifacts"`
}

func Load(path string) (*Document, error) {
	var value Document
	if err := document.Load(path, &value); err != nil {
		return nil, err
	}
	if err := value.Validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &value, nil
}

func (d Document) Validate() error {
	if d.Schema != Schema {
		return fmt.Errorf("schema is %q, expected %q", d.Schema, Schema)
	}
	if d.Product == "" || d.Version == "" || d.Channel == "" {
		return fmt.Errorf("product, version, and channel must not be empty")
	}
	if d.ReleaseCode <= 0 {
		return fmt.Errorf("releaseCode must be positive")
	}
	if len(d.Artifacts) == 0 {
		return fmt.Errorf("artifacts must not be empty")
	}
	if d.MergeIdentity != nil {
		if d.MergeIdentity.Product != d.Product ||
			d.MergeIdentity.Version != d.Version ||
			d.MergeIdentity.Channel != d.Channel {
			return fmt.Errorf("mergeIdentity does not match document identity")
		}
	}
	seen := map[string]bool{}
	artifactsByID := map[string]Artifact{}
	for index, artifact := range d.Artifacts {
		if artifact.ID == "" {
			return fmt.Errorf("artifacts[%d].id must not be empty", index)
		}
		if err := model.CheckIdentifier(artifact.ID, fmt.Sprintf("artifacts[%d].id", index)); err != nil {
			return err
		}
		if artifactsByID[artifact.ID].ID != "" {
			return fmt.Errorf("duplicate artifact id %q", artifact.ID)
		}
		artifactsByID[artifact.ID] = artifact
		if artifact.Path == "" {
			return fmt.Errorf("artifacts[%d].path must not be empty", index)
		}
		clean := path.Clean(artifact.Path)
		if clean == "." || clean != artifact.Path || path.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, "../") || strings.Contains(artifact.Path, `\`) {
			return fmt.Errorf("artifacts[%d].path %q is not a normalized relative slash path", index, artifact.Path)
		}
		switch artifact.Kind {
		case KindInstaller, KindPayload, KindBlob, KindMetadata, KindOther:
		default:
			return fmt.Errorf("artifacts[%d] has unknown kind %q", index, artifact.Kind)
		}
		if err := canonical.ValidateDigest(artifact.SHA256); err != nil {
			return fmt.Errorf("artifacts[%d].sha256: %w", index, err)
		}
		if artifact.Size < 0 {
			return fmt.Errorf("artifacts[%d].size must not be negative", index)
		}
		key := artifact.Platform + "\x00" + artifact.Path
		if seen[key] {
			return fmt.Errorf("duplicate artifact path %q for platform %q", artifact.Path, artifact.Platform)
		}
		seen[key] = true
		selectorNames := map[string]bool{}
		for name, value := range artifact.Selectors {
			if name == "" || value == "" || selectorNames[name] {
				return fmt.Errorf("artifact %q has an empty or duplicate selector", artifact.Path)
			}
			selectorNames[name] = true
		}
		if artifact.Relations.InstallOf != "" && artifact.Relations.PayloadOf != "" {
			return fmt.Errorf("artifact %q cannot be both installOf and payloadOf", artifact.Path)
		}
	}
	for _, artifact := range d.Artifacts {
		for relation, target := range map[string]string{
			"installOf": artifact.Relations.InstallOf,
			"payloadOf": artifact.Relations.PayloadOf,
		} {
			if target == "" {
				continue
			}
			if target == artifact.ID {
				return fmt.Errorf("artifact %q relation %s cannot reference itself", artifact.Path, relation)
			}
			targetArtifact, ok := artifactsByID[target]
			if !ok {
				return fmt.Errorf("artifact %q relation %s references missing artifact id %q", artifact.Path, relation, target)
			}
			if targetArtifact.Platform != artifact.Platform {
				return fmt.Errorf("artifact %q relation %s crosses platforms", artifact.Path, relation)
			}
		}
	}
	return nil
}

// VerifyFiles checks that producer output is entirely inside outputRoot and
// that every declared byte count and digest matches the candidate file.
func (d Document) VerifyFiles(outputRoot string, selectorDimensions []string) error {
	if err := d.Validate(); err != nil {
		return err
	}
	absoluteRoot, err := filepath.Abs(outputRoot)
	if err != nil {
		return err
	}
	allowedSelectors := map[string]bool{}
	for _, dimension := range selectorDimensions {
		allowedSelectors[dimension] = true
	}
	for _, artifact := range d.Artifacts {
		for dimension := range artifact.Selectors {
			if !allowedSelectors[dimension] {
				return fmt.Errorf("artifact %q uses undeclared selector dimension %q", artifact.Path, dimension)
			}
		}
		candidate := filepath.Join(absoluteRoot, filepath.FromSlash(artifact.Path))
		relative, err := filepath.Rel(absoluteRoot, candidate)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return fmt.Errorf("artifact %q escapes producer output directory", artifact.Path)
		}
		file, err := os.Open(candidate)
		if err != nil {
			return fmt.Errorf("artifact %q: %w", artifact.Path, err)
		}
		hash := sha256.New()
		size, copyErr := io.Copy(hash, file)
		closeErr := file.Close()
		if copyErr != nil {
			return fmt.Errorf("artifact %q: %w", artifact.Path, copyErr)
		}
		if closeErr != nil {
			return fmt.Errorf("artifact %q: %w", artifact.Path, closeErr)
		}
		if size != artifact.Size {
			return fmt.Errorf("artifact %q size is %d, producer declared %d", artifact.Path, size, artifact.Size)
		}
		digest := canonical.DigestPrefix + fmt.Sprintf("%x", hash.Sum(nil))
		if digest != artifact.SHA256 {
			return fmt.Errorf("artifact %q digest is %s, producer declared %s", artifact.Path, digest, artifact.SHA256)
		}
	}
	return nil
}


func Merge(documents ...Document) (*Document, error) {
	if len(documents) == 0 {
		return nil, fmt.Errorf("at least one artifact set is required")
	}
	for index := range documents {
		if err := documents[index].Validate(); err != nil {
			return nil, fmt.Errorf("artifact set %d: %w", index, err)
		}
	}
	base := documents[0]
	if len(documents) > 1 && base.MergeIdentity == nil {
		return nil, fmt.Errorf("multi-platform merge requires mergeIdentity")
	}
	baseDisplayDigest, err := canonical.Digest(base.Display)
	if err != nil {
		return nil, err
	}
	result := base
	result.Artifacts = nil
	for index, candidate := range documents {
		if candidate.Product != base.Product ||
			candidate.Version != base.Version ||
			candidate.ReleaseCode != base.ReleaseCode ||
			candidate.Channel != base.Channel {
			return nil, fmt.Errorf("artifact set %d has a different product/version/releaseCode/channel identity", index)
		}
		if len(documents) > 1 && candidate.MergeIdentity == nil {
			return nil, fmt.Errorf("artifact set %d is missing mergeIdentity", index)
		}
		if candidate.Changelog != base.Changelog {
			return nil, fmt.Errorf("artifact set %d has different changelog metadata", index)
		}
		candidateDisplayDigest, err := canonical.Digest(candidate.Display)
		if err != nil {
			return nil, err
		}
		if candidateDisplayDigest != baseDisplayDigest {
			return nil, fmt.Errorf("artifact set %d has different display metadata", index)
		}
		result.Artifacts = append(result.Artifacts, candidate.Artifacts...)
	}
	slices.SortStableFunc(result.Artifacts, func(left, right Artifact) int {
		if left.Platform != right.Platform {
			if left.Platform < right.Platform {
				return -1
			}
			return 1
		}
		if left.Path < right.Path {
			return -1
		}
		if left.Path > right.Path {
			return 1
		}
		return 0
	})
	if err := result.Validate(); err != nil {
		return nil, err
	}
	return &result, nil
}
