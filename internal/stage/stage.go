package stage

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	rupv2 "github.com/shichao402/relkit/api/rup/v2"
	"github.com/shichao402/relkit/internal/config"
	"github.com/shichao402/relkit/internal/model"
	"github.com/shichao402/relkit/internal/payload"
)

const stagingRoot = ".relkit/cache/staged"

type Error struct {
	Message string
}

func (e Error) Error() string {
	return e.Message
}

// StagingDir is the per-version staged tree under the product root. It is the
// data-plane layout shared by `relkit stage`, the agent's staged upload, and
// the publisher; the legacy release-policy.json sidecar is retired
// (ADR 0019 §1), so the directory no longer carries repository policy.
func StagingDir(root, version string) string {
	return filepath.Join(root, stagingRoot, version)
}

func StagedPath(root, version string) string {
	return filepath.Join(StagingDir(root, version), "staged.pb")
}

// StagedVersions lists versions that have a staging directory under root, most
// recently written first.
func StagedVersions(root string) ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(root, stagingRoot))
	if err != nil {
		return nil, err
	}
	type staged struct {
		name     string
		modified time.Time
	}
	found := make([]staged, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		found = append(found, staged{name: entry.Name(), modified: info.ModTime()})
	}
	sort.Slice(found, func(i, j int) bool { return found[i].modified.After(found[j].modified) })
	names := make([]string, 0, len(found))
	for _, item := range found {
		names = append(names, item.name)
	}
	return names, nil
}

func ArtifactsDir(root, version string) string {
	return filepath.Join(StagingDir(root, version), "artifacts")
}

func LoadStaged(root, version string) (*model.StagedDocument, error) {
	path := StagedPath(root, version)
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return nil, Error{Message: fmt.Sprintf("no staged release for version %q (expected %s); run 'relkit stage --artifact-set' first", version, path)}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	staged, err := rupv2.UnmarshalStaged(data)
	if err != nil {
		return nil, err
	}
	return staged, nil
}

func VerifyStagedHashes(cfg *config.Config, staged *model.StagedDocument) []string {
	return verifyStagedHashes(cfg, staged, false)
}

// VerifyPresentStagedHashes verifies every artifact that is present locally,
// while allowing a thin staged tree whose bytes were uploaded to CAS first.
func VerifyPresentStagedHashes(cfg *config.Config, staged *model.StagedDocument) []string {
	return verifyStagedHashes(cfg, staged, true)
}

func verifyStagedHashes(cfg *config.Config, staged *model.StagedDocument, allowMissing bool) []string {
	var mismatches []string
	directory := ArtifactsDir(cfg.Root, staged.Version)
	for _, artifact := range staged.Artifacts {
		path := filepath.Join(directory, artifact.Filename)
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			if allowMissing && os.IsNotExist(err) {
				continue
			}
			mismatches = append(mismatches, fmt.Sprintf("%s: missing from staging tree", artifact.Filename))
			continue
		}
		digest, size, err := model.Sha256File(path)
		if err != nil {
			mismatches = append(mismatches, fmt.Sprintf("%s: %v", artifact.Filename, err))
			continue
		}
		if digest != artifact.Sha256 || size != artifact.Size {
			actualPrefix := digest
			if len(actualPrefix) > 12 {
				actualPrefix = actualPrefix[:12]
			}
			expectedPrefix := artifact.Sha256
			if len(expectedPrefix) > 12 {
				expectedPrefix = expectedPrefix[:12]
			}
			mismatches = append(mismatches, fmt.Sprintf("%s: staged tree has %s (%d bytes), staged.pb records %s (%d bytes)", artifact.Filename, actualPrefix, size, expectedPrefix, artifact.Size))
			continue
		}
		if artifact.Kind == rupv2.ArtifactKind_ARTIFACT_KIND_PAYLOAD {
			if _, err := payload.Validate(path); err != nil {
				mismatches = append(mismatches, fmt.Sprintf("%s: invalid payload: %v", artifact.Filename, err))
			}
		}
	}
	return mismatches
}

func requireInstallCounterparts(artifacts []*model.StagedArtifact) error {
	for _, artifact := range artifacts {
		if artifact == nil || artifact.Kind != rupv2.ArtifactKind_ARTIFACT_KIND_PAYLOAD {
			continue
		}
		want := model.SelectorsToMap(artifact.Selectors)
		delete(want, "apply")
		found := false
		for _, candidate := range artifacts {
			if candidate == nil || candidate.Kind == rupv2.ArtifactKind_ARTIFACT_KIND_PAYLOAD {
				continue
			}
			if mapsEqual(want, model.SelectorsToMap(candidate.Selectors)) {
				found = true
				break
			}
		}
		if !found {
			return Error{Message: fmt.Sprintf("payload %q requires a full-install artifact with the same selectors (excluding apply=relkit-payload)", artifact.Id)}
		}
	}
	return nil
}

func mapsEqual(left, right map[string]string) bool {
	if len(left) != len(right) {
		return false
	}
	for key, value := range left {
		if right[key] != value {
			return false
		}
	}
	return true
}

func materialize(source, target string, useLink bool) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	_ = os.Remove(target)
	if useLink {
		if err := os.Link(source, target); err == nil {
			return nil
		}
	}
	src, err := os.Open(source)
	if err != nil {
		return err
	}
	defer src.Close()
	dst, err := os.Create(target)
	if err != nil {
		return err
	}
	defer dst.Close()
	_, err = io.Copy(dst, src)
	return err
}

func nilIfEmpty(meta map[string]any) map[string]any {
	if len(meta) == 0 {
		return nil
	}
	return meta
}

func findDuplicateIDs(artifacts []*model.StagedArtifact) []string {
	counts := make(map[string]int, len(artifacts))
	for _, artifact := range artifacts {
		if artifact == nil {
			continue
		}
		counts[artifact.Id]++
	}
	var duplicates []string
	for id, count := range counts {
		if count > 1 {
			duplicates = append(duplicates, id)
		}
	}
	sort.Strings(duplicates)
	return duplicates
}

func findDuplicateFilenames(artifacts []*model.StagedArtifact) []string {
	counts := make(map[string]int, len(artifacts))
	for _, artifact := range artifacts {
		if artifact == nil {
			continue
		}
		counts[artifact.Filename]++
	}
	var duplicates []string
	for filename, count := range counts {
		if count > 1 {
			duplicates = append(duplicates, filename)
		}
	}
	sort.Strings(duplicates)
	return duplicates
}
