package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/shichao402/relkit/internal/payload"
	"github.com/shichao402/relkit/internal/vnext/artifactset"
	"github.com/shichao402/relkit/internal/vnext/document"
	"github.com/shichao402/relkit/internal/vnext/producer"
	"github.com/shichao402/relkit/internal/vnext/project"
	vnextrelease "github.com/shichao402/relkit/internal/vnext/release"
)

// defaultProjectPath is the project declaration a host repository commits.
//
// It is the only control-plane input a host still holds: the server address,
// the server root public key, and the product name. Everything that used to
// sit beside it (resolution, operator profile, bootstrap mapping) is gone with
// the host-picks-its-tool-version model (requirement 14.1).
const defaultProjectPath = "relkit.project.json"

func cmdPayload(args []string) error {
	if len(args) == 0 || args[0] != "build" {
		return fmt.Errorf("usage: relkit payload build --tree <directory> --out <payload.zip>")
	}
	tree := ""
	output := ""
	for i := 1; i < len(args); i++ {
		switch arg := args[i]; arg {
		case "--tree":
			i++
			tree = mustValue(args, i, "--tree")
		case "--out":
			i++
			output = mustValue(args, i, "--out")
		default:
			return fmt.Errorf("unexpected argument %q", arg)
		}
	}
	if tree == "" || output == "" {
		return fmt.Errorf("payload build requires --tree and --out")
	}
	table, err := payload.Build(output, payload.BuildOptions{Tree: tree})
	if err != nil {
		return err
	}
	fmt.Printf("built payload %s (%d files, %d scripts)\n", output, len(table.Files), len(table.Scripts))
	return nil
}

func cmdArtifacts(args []string) error {
	if len(args) > 0 && args[0] == "merge" {
		return cmdArtifactsMerge(args[1:])
	}
	if len(args) == 0 || args[0] != "produce" {
		return fmt.Errorf("usage: relkit artifacts <produce|merge> ...")
	}
	return cmdArtifactsProduce(args[1:])
}

// cmdArtifactsProduce runs the product's artifact-producer and writes the
// ArtifactSet it returned. The host no longer hands the producer a resolution:
// the build describes what it built for this product, version, channel, and
// target, and the server decides which relkit version consumes it.
func cmdArtifactsProduce(args []string) error {
	projectPath := defaultProjectPath
	version := ""
	channel := ""
	target := ""
	outputDir := ""
	outputPath := ""
	for i := 0; i < len(args); i++ {
		switch arg := args[i]; {
		case arg == "--project":
			i++
			projectPath = mustValue(args, i, "--project")
		case arg == "--version":
			i++
			version = mustValue(args, i, "--version")
		case arg == "--channel":
			i++
			channel = mustValue(args, i, "--channel")
		case arg == "--target":
			i++
			target = mustValue(args, i, "--target")
		case arg == "--out-dir":
			i++
			outputDir = mustValue(args, i, "--out-dir")
		case arg == "--out":
			i++
			outputPath = mustValue(args, i, "--out")
		case strings.HasPrefix(arg, "-"):
			return fmt.Errorf("unknown flag %q", arg)
		default:
			return fmt.Errorf("unexpected argument %q", arg)
		}
	}
	if version == "" || channel == "" || target == "" {
		return fmt.Errorf("--version, --channel, and --target are required")
	}
	productDocument, err := project.Load(projectPath)
	if err != nil {
		return err
	}
	root := filepath.Dir(projectPath)
	if root == "" {
		root = "."
	}
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	absoluteOutputDir := outputDir
	if absoluteOutputDir == "" {
		absoluteOutputDir = filepath.Join(absoluteRoot, ".relkit", "producer", version, target)
	} else if !filepath.IsAbs(absoluteOutputDir) {
		absoluteOutputDir = filepath.Join(absoluteRoot, absoluteOutputDir)
	}
	if outputPath == "" {
		outputPath = filepath.Join(absoluteOutputDir, vnextrelease.ArtifactSetFile)
	} else if !filepath.IsAbs(outputPath) {
		outputPath = filepath.Join(absoluteRoot, outputPath)
	}
	value, err := producer.Run(context.Background(), *productDocument, producer.Options{
		ProjectRoot:    absoluteRoot,
		OutputDir:      absoluteOutputDir,
		Version:        version,
		Channel:        channel,
		Target:         target,
		ProtectedPaths: []string{projectPath},
	})
	if err != nil {
		return err
	}
	if err := document.WriteNew(outputPath, value); err != nil {
		return err
	}
	fmt.Printf("produced %d artifact(s) for %s/%s\n", len(value.Artifacts), version, target)
	fmt.Println("wrote " + outputPath)
	return nil
}

func cmdArtifactsMerge(args []string) error {
	var inputs []string
	outputPath := ""
	for i := 0; i < len(args); i++ {
		switch arg := args[i]; {
		case arg == "--in":
			i++
			inputs = append(inputs, mustValue(args, i, "--in"))
		case arg == "--out":
			i++
			outputPath = mustValue(args, i, "--out")
		case strings.HasPrefix(arg, "-"):
			return fmt.Errorf("unknown flag %q", arg)
		default:
			return fmt.Errorf("unexpected argument %q", arg)
		}
	}
	if len(inputs) < 2 || outputPath == "" {
		return fmt.Errorf("artifacts merge requires at least two --in paths and --out")
	}
	documents := make([]artifactset.Document, 0, len(inputs))
	for _, path := range inputs {
		value, err := artifactset.Load(path)
		if err != nil {
			return err
		}
		documents = append(documents, *value)
	}
	merged, err := artifactset.Merge(documents...)
	if err != nil {
		return err
	}
	outputRoot := filepath.Dir(outputPath)
	if outputRoot == "." {
		return fmt.Errorf("--out must be inside a dedicated, new aggregate directory")
	}
	if _, err := os.Stat(outputRoot); err == nil {
		return fmt.Errorf("merge output directory already exists: %s", outputRoot)
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(outputRoot), 0o755); err != nil {
		return err
	}
	temporaryRoot, err := os.MkdirTemp(filepath.Dir(outputRoot), "."+filepath.Base(outputRoot)+".")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temporaryRoot)
	seenDestination := map[string]bool{}
	for index, value := range documents {
		sourceRoot := filepath.Dir(inputs[index])
		for _, artifact := range value.Artifacts {
			if seenDestination[artifact.Path] {
				return fmt.Errorf("merged artifacts contain duplicate relative path %q", artifact.Path)
			}
			seenDestination[artifact.Path] = true
			if err := copyArtifactCandidate(
				filepath.Join(sourceRoot, filepath.FromSlash(artifact.Path)),
				filepath.Join(temporaryRoot, filepath.FromSlash(artifact.Path)),
			); err != nil {
				return err
			}
		}
	}
	if err := merged.VerifyFiles(temporaryRoot, selectorDimensions(merged.Artifacts)); err != nil {
		return err
	}
	if err := document.Write(filepath.Join(temporaryRoot, filepath.Base(outputPath)), merged); err != nil {
		return err
	}
	if err := os.Rename(temporaryRoot, outputRoot); err != nil {
		return err
	}
	fmt.Printf("merged %d ArtifactSets into %s (%d artifacts)\n", len(inputs), outputPath, len(merged.Artifacts))
	return nil
}

func copyArtifactCandidate(source, destination string) error {
	if _, err := os.Stat(destination); err == nil {
		return fmt.Errorf("merge destination already exists: %s", destination)
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
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

func selectorDimensions(artifacts []artifactset.Artifact) []string {
	seen := map[string]bool{}
	var result []string
	for _, artifact := range artifacts {
		for dimension := range artifact.Selectors {
			if !seen[dimension] {
				seen[dimension] = true
				result = append(result, dimension)
			}
		}
	}
	sort.Strings(result)
	return result
}

// cmdStageVNext stages an immutable ArtifactSet staging tree.
//
// A stage no longer consumes a resolution or a candidate bundle: the server
// decides which relkit version is current, so the host only records what it
// built.
func cmdStageVNext(args []string) error {
	projectPath := defaultProjectPath
	artifactSetPath := ""
	code := 0
	minFrom := 0
	notesURL := ""
	for i := 0; i < len(args); i++ {
		switch arg := args[i]; {
		case arg == "--project":
			i++
			projectPath = mustValue(args, i, "--project")
		case arg == "--artifact-set":
			i++
			artifactSetPath = mustValue(args, i, "--artifact-set")
		case arg == "--code":
			i++
			value, err := strconv.Atoi(mustValue(args, i, "--code"))
			if err != nil {
				return fmt.Errorf("--code: %w", err)
			}
			if value <= 0 {
				return fmt.Errorf("--code must be positive")
			}
			code = value
		case arg == "--min-from":
			i++
			value, err := strconv.Atoi(mustValue(args, i, "--min-from"))
			if err != nil {
				return fmt.Errorf("--min-from: %w", err)
			}
			minFrom = value
		case arg == "--notes-url":
			i++
			notesURL = mustValue(args, i, "--notes-url")
		case strings.HasPrefix(arg, "-"):
			return fmt.Errorf("unknown flag %q", arg)
		default:
			return fmt.Errorf("unexpected argument %q", arg)
		}
	}
	if artifactSetPath == "" {
		return fmt.Errorf("stage requires --artifact-set")
	}
	productDocument, err := project.Load(projectPath)
	if err != nil {
		return err
	}
	artifacts, err := artifactset.Load(artifactSetPath)
	if err != nil {
		return err
	}
	staged, err := vnextrelease.Stage(*productDocument, *artifacts, vnextrelease.StageOptions{
		ProjectRoot:     filepath.Dir(projectPath),
		ArtifactSetPath: artifactSetPath,
		Code:            code,
		MinFrom:         minFrom,
		NotesURL:        notesURL,
		RelkitVersion:   version,
	})
	if err != nil {
		return err
	}
	fmt.Printf("staged %s (code %d, channel %s)\n", staged.Version, staged.Code, staged.Channel)
	return nil
}
