package stage

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	rupv2 "github.com/shichao402/relkit/api/rup/v2"
	"github.com/shichao402/relkit/internal/changelog"
	"github.com/shichao402/relkit/internal/config"
	"github.com/shichao402/relkit/internal/jsonio"
	"github.com/shichao402/relkit/internal/model"
	"github.com/shichao402/relkit/internal/selectors"
)

const stagingRoot = ".relkit/staged"

var reservedKeys = map[string]struct{}{
	"id":       {},
	"kind":     {},
	"filename": {},
}

type Error struct {
	Message string
}

func (e Error) Error() string {
	return e.Message
}

type AddSpec struct {
	Path      string
	PairsText string
}

type Printer func(string)

func StagingDir(root, version string) string {
	return filepath.Join(root, stagingRoot, version)
}

func StagedPath(root, version string) string {
	return filepath.Join(StagingDir(root, version), "staged.pb")
}

func ArtifactsDir(root, version string) string {
	return filepath.Join(StagingDir(root, version), "artifacts")
}

func LoadStaged(root, version string) (*model.StagedDocument, error) {
	path := StagedPath(root, version)
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return nil, Error{Message: fmt.Sprintf("no staged release for version %q (expected %s); run 'relkit stage' first", version, path)}
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

func ParseKeyValues(text string) (map[string]string, error) {
	result := map[string]string{}
	for _, rawPart := range strings.Split(text, ",") {
		part := strings.TrimSpace(rawPart)
		if part == "" {
			continue
		}
		if !strings.Contains(part, "=") {
			return nil, Error{Message: fmt.Sprintf("expected key=value pairs separated by commas, got %q", part)}
		}
		key, value, _ := strings.Cut(part, "=")
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" {
			return nil, Error{Message: fmt.Sprintf("empty key in %q", part)}
		}
		if _, exists := result[key]; exists {
			return nil, Error{Message: fmt.Sprintf("duplicate key %q", key)}
		}
		result[key] = value
	}
	return result, nil
}

func SplitControls(pairs map[string]string) (map[string]string, map[string]any, map[string]string, error) {
	controls := map[string]string{}
	meta := map[string]any{}
	selectorsMap := map[string]string{}
	for key, value := range pairs {
		if _, ok := reservedKeys[key]; ok {
			controls[key] = value
			continue
		}
		if strings.HasPrefix(key, "meta.") {
			metaKey := strings.TrimPrefix(key, "meta.")
			if metaKey == "" {
				return nil, nil, nil, Error{Message: fmt.Sprintf("meta key is empty in %q", key)}
			}
			meta[metaKey] = value
			continue
		}
		selectorsMap[key] = value
	}
	return controls, meta, selectorsMap, nil
}

func BuildArtifact(source string, pairs map[string]string, report *[]string) (*model.StagedArtifact, error) {
	sourcePath := source
	info, err := os.Stat(sourcePath)
	if err != nil || info.IsDir() {
		return nil, Error{Message: fmt.Sprintf("artifact not found: %s", sourcePath)}
	}

	controls, meta, selectorsMap, err := SplitControls(pairs)
	if err != nil {
		return nil, err
	}

	filename := filepath.Base(sourcePath)
	if value, ok := controls["filename"]; ok {
		filename = value
	}
	if err := model.CheckFilename(filename); err != nil {
		return nil, err
	}
	if err := model.CheckSelectors(selectorsMap, "selectors"); err != nil {
		return nil, err
	}

	kind := controls["kind"]
	if kind == "" {
		kind = model.InferKind(filename)
		*report = append(*report, fmt.Sprintf("  kind inferred as %-9s for %s", kind, filename))
	}

	artifactID := controls["id"]
	if artifactID == "" {
		artifactID, err = model.DefaultArtifactID(selectorsMap)
		if err != nil {
			return nil, err
		}
		*report = append(*report, fmt.Sprintf("  id derived as   %-9s for %s", artifactID, filename))
	}

	digest, size, err := model.Sha256File(sourcePath)
	if err != nil {
		return nil, err
	}

	return model.NewStagedArtifact(
		artifactID,
		filename,
		size,
		digest,
		kind,
		selectorsMap,
		nilIfEmpty(meta),
		sourcePath,
	)
}

func Run(cfg *config.Config, version string, code, minFrom int, adds []AddSpec, channel string, notes string, notesFile string, notesURL string, useLink bool, printer Printer) (*model.StagedDocument, error) {
	if printer == nil {
		printer = func(string) {}
	}

	resolvedChannel, err := cfg.ChannelOrDefault(channel)
	if err != nil {
		return nil, err
	}

	notes, notesURL, err = changelog.ResolveNotes(
		changelog.Config{File: cfg.Changelog.File, URLTemplate: cfg.Changelog.URLTemplate},
		cfg.Root,
		version,
		notes,
		notesFile,
		notesURL,
	)
	if err != nil {
		return nil, Error{Message: err.Error()}
	}

	if len(adds) == 0 {
		return nil, Error{Message: "at least one --add is required"}
	}

	var report []string
	artifacts := make([]*model.StagedArtifact, 0, len(adds))
	for _, add := range adds {
		pairs := map[string]string{}
		if add.PairsText != "" {
			pairs, err = ParseKeyValues(add.PairsText)
			if err != nil {
				return nil, err
			}
		}
		artifact, err := BuildArtifact(add.Path, pairs, &report)
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, artifact)
	}

	if duplicateIDs := findDuplicateIDs(artifacts); len(duplicateIDs) > 0 {
		return nil, Error{Message: fmt.Sprintf("artifact ids must be unique within a release; duplicated: %s", strings.Join(duplicateIDs, ", "))}
	}

	if duplicates := selectors.FindDuplicateSelectors(artifacts); len(duplicates) > 0 {
		first := duplicates[0]
		return nil, Error{Message: fmt.Sprintf("artifacts %q and %q declare identical selectors %v, so one of them could never be chosen (SPEC.md 11)", first.FirstID, first.SecondID, first.Selectors)}
	}

	staged, err := model.NewStagedDocument(cfg.Product, version, code, minFrom, resolvedChannel, artifacts, notes, notesURL, "")
	if err != nil {
		return nil, err
	}

	if notes != "" {
		printer(fmt.Sprintf("notes: %d chars", len(notes)))
	}
	if notesURL != "" {
		printer("notesUrl: " + notesURL)
	}

	targetDir := ArtifactsDir(cfg.Root, version)
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return nil, err
	}
	for _, artifact := range staged.Artifacts {
		target := filepath.Join(targetDir, artifact.Filename)
		if err := materialize(artifact.SourcePath, target, useLink); err != nil {
			return nil, err
		}
	}

	data, err := rupv2.MarshalStaged(staged)
	if err != nil {
		return nil, err
	}
	if err := jsonio.WritePath(StagedPath(cfg.Root, version), data); err != nil {
		return nil, err
	}

	for _, line := range report {
		printer(line)
	}
	printer(fmt.Sprintf("staged %s (code %d, channel %s, minFrom %d)", version, code, resolvedChannel, minFrom))
	for _, artifact := range staged.Artifacts {
		hashPrefix := artifact.Sha256
		if len(hashPrefix) > 12 {
			hashPrefix = hashPrefix[:12]
		}
		printer(fmt.Sprintf("  %-12s %-40s %12d bytes  %s", artifact.Id, artifact.Filename, artifact.Size, hashPrefix))
	}
	printer("staging tree: " + StagingDir(cfg.Root, version))
	return staged, nil
}

func VerifyStagedHashes(cfg *config.Config, staged *model.StagedDocument) []string {
	var mismatches []string
	directory := ArtifactsDir(cfg.Root, staged.Version)
	for _, artifact := range staged.Artifacts {
		path := filepath.Join(directory, artifact.Filename)
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
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
		}
	}
	return mismatches
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
