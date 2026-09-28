package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/shichao402/relkit/internal/releasegate"
)

// ciReleaseOptions carries the parsed `relkit ci release` arguments.
type ciReleaseOptions struct {
	channel string
	execute bool
}

// cmdCI implements `relkit ci <sub>`: the CI-facing product release entry.
// Only `release` exists today, mirroring relkit_host.py's ci subcommand.
func cmdCI(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: relkit ci release --channel <dev|stable> [--execute]")
	}
	switch args[0] {
	case "release":
		rest := args[1:]
		opts := ciReleaseOptions{}
		for i := 0; i < len(rest); i++ {
			switch rest[i] {
			case "--channel":
				i++
				opts.channel = mustValue(rest, i, "--channel")
			case "--execute":
				opts.execute = true
			default:
				return fmt.Errorf("unknown flag %q for ci release", rest[i])
			}
		}
		if opts.channel == "" {
			return fmt.Errorf("ci release requires --channel")
		}
		return cmdCIRelease(&opts)
	default:
		return fmt.Errorf("unknown ci subcommand %q", args[0])
	}
}

// ciReleaseArtifacts is the relkit.release-artifacts/1 manifest written by the
// product's release.packScript.
type ciReleaseArtifacts struct {
	Schema   string `json:"schema"`
	Version  string `json:"version"`
	Install  ciArtifactRef
	Payload  ciArtifactRef
	Archives []ciArchiveRef
}

type ciArtifactRef struct {
	Path      string `json:"path"`
	Kind      string `json:"kind"`
	Selectors string `json:"selectors"`
}

type ciArchiveRef struct {
	Path string `json:"path"`
	Role string `json:"role"`
}

// cmdCIRelease implements `relkit ci release`: install → pack → stage →
// simulate → fake → publish. Execute is CI-only (RELKIT_RELEASE_VIA_CI=1 and
// RELKIT_UPLOAD_TOKEN both required), mirroring the Python entry.
func cmdCIRelease(opts *ciReleaseOptions) error {
	if err := resolveCIChannel(opts.channel); err != nil {
		return err
	}
	if opts.execute && os.Getenv("RELKIT_RELEASE_VIA_CI") != "1" {
		return fmt.Errorf("ci release --execute requires RELKIT_RELEASE_VIA_CI=1 (CI holds the agent token; local shells must not publish)")
	}
	if opts.execute && strings.TrimSpace(os.Getenv("RELKIT_UPLOAD_TOKEN")) == "" {
		return fmt.Errorf("ci release --execute requires RELKIT_UPLOAD_TOKEN (product agent Bearer injected by CI secrets)")
	}

	fmt.Printf("relkit ci release channel=%s execute=%t\n", opts.channel, opts.execute)
	if err := cmdConsumeInstall([]string{}); err != nil {
		return err
	}
	version, err := projectVersionForRelkit(".")
	if err != nil {
		return err
	}
	pack, err := releasePackConfig(".")
	if err != nil {
		return err
	}
	if err := runReleasePackScript(".", pack.script); err != nil {
		return err
	}
	artifacts, err := loadReleaseArtifactsManifest(".", pack.manifest, version)
	if err != nil {
		return err
	}

	stageArgs := []string{
		"stage", version,
		"--channel", opts.channel,
		"--install", artifacts.Install.Path,
		"kind=" + artifacts.Install.Kind + "," + artifacts.Install.Selectors,
		"--payload", artifacts.Payload.Path, artifacts.Payload.Selectors,
	}
	fmt.Printf("staging install=%s payload=%s archives=%d (ci-only, not staged)\n",
		artifacts.Install.Path, artifacts.Payload.Path, len(artifacts.Archives))
	if err := dispatch(stageArgs, ""); err != nil {
		return err
	}
	if err := dispatch([]string{"simulate", "--with-staged", version, "--from", "all"}, ""); err != nil {
		return err
	}
	if err := cmdFakeVerify(version); err != nil {
		return err
	}
	if !opts.execute {
		fmt.Println("ci release dry-run complete (install → pack → stage → simulate → fake); pass --execute with RELKIT_RELEASE_VIA_CI=1 to publish")
		return nil
	}
	// The release gate: unresolved onboarding steps block the publish. The
	// state file stays Python-written during migration; Go only reads it.
	state, err := releasegate.LoadState(".")
	if err != nil {
		return err
	}
	if missing := releasegate.IncompleteSteps(state, true, "."); len(missing) > 0 {
		return fmt.Errorf("release refused: incomplete steps: %s", strings.Join(missing, ", "))
	}
	// Agent mode is the only publish path from CI: cas-put + POST /publish,
	// signing key stays on the publish host.
	if removed, err := releasegate.ClearStaleStagedTrees(".", version); err != nil {
		return err
	} else if len(removed) > 0 {
		fmt.Printf("removed stale staged caches: %s\n", strings.Join(removed, ", "))
	}
	return releasegate.PublishViaAgent(".", version, true)
}

// dispatch routes an internal command line through the same switch main()
// uses, so ci release drives the exact same code paths as the CLI surface.
func dispatch(argv []string, configPath string) error {
	if len(argv) == 0 {
		return fmt.Errorf("dispatch: empty argv")
	}
	switch argv[0] {
	case "stage":
		return cmdStage(argv[1:], configPath)
	case "simulate":
		return cmdSimulate(argv[1:], configPath)
	default:
		return fmt.Errorf("dispatch: unsupported command %q", argv[0])
	}
}

// projectVersionForRelkit reads VERSION.json (v prefix normalized away).
func projectVersionForRelkit(root string) (string, error) {
	for _, name := range []string{"VERSION.json", "version.json"} {
		path := filepath.Join(root, name)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var doc struct {
			Version string `json:"version"`
		}
		if err := json.Unmarshal(data, &doc); err != nil {
			continue
		}
		raw := strings.TrimSpace(doc.Version)
		if raw != "" {
			return strings.TrimPrefix(raw, "v"), nil
		}
	}
	return "", fmt.Errorf("VERSION.json / VERSION is required for ci release")
}

// releasePackConfig reads relkit.json release.packScript + release.manifest.
type releasePack struct {
	script   string
	manifest string
}

func releasePackConfig(root string) (*releasePack, error) {
	path := filepath.Join(root, "relkit.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("relkit.json not found; run keys gen --execute first")
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("relkit.json is not valid JSON: %v", err)
	}
	release, _ := config["release"].(map[string]any)
	script, _ := release["packScript"].(string)
	if strings.TrimSpace(script) == "" {
		return nil, fmt.Errorf("relkit.json must declare release.packScript for ci release")
	}
	manifest, _ := release["manifest"].(string)
	if strings.TrimSpace(manifest) == "" {
		manifest = "dist/release-artifacts.json"
	}
	return &releasePack{script: script, manifest: manifest}, nil
}

// runReleasePackScript executes the product pack script by extension.
func runReleasePackScript(root, script string) error {
	scriptPath := filepath.Join(root, script)
	if _, err := os.Stat(scriptPath); err != nil {
		return fmt.Errorf("release.packScript is missing: %s", script)
	}
	var command *exec.Cmd
	switch strings.ToLower(filepath.Ext(script)) {
	case ".mjs", ".js", ".cjs":
		node := os.Getenv("RELKIT_NODE")
		if node == "" {
			node = "node"
		}
		command = exec.Command(node, scriptPath)
	case ".py":
		python := os.Getenv("RELKIT_PYTHON")
		if python == "" {
			python = "python"
		}
		command = exec.Command(python, scriptPath)
	case ".ps1":
		command = exec.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", scriptPath)
	case ".cmd", ".bat":
		command = exec.Command("cmd.exe", "/c", scriptPath)
	default:
		return fmt.Errorf("unsupported release.packScript type %s; use .mjs/.js/.py/.ps1/.cmd", filepath.Ext(script))
	}
	command.Dir = root
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	fmt.Printf("> %s\n", command.String())
	if err := command.Run(); err != nil {
		return fmt.Errorf("release.packScript failed: %w", err)
	}
	return nil
}

// loadReleaseArtifactsManifest reads and validates the relkit.release-artifacts/1
// manifest written by packScript.
func loadReleaseArtifactsManifest(root, manifestPath, expectedVersion string) (*ciReleaseArtifacts, error) {
	path := filepath.Join(root, manifestPath)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("release manifest missing after packScript: %s", manifestPath)
	}
	var artifacts struct {
		Schema   string         `json:"schema"`
		Version  string         `json:"version"`
		Install  ciArtifactRef  `json:"install"`
		Payload  ciArtifactRef  `json:"payload"`
		Archives []ciArchiveRef `json:"archives"`
	}
	if err := json.Unmarshal(data, &artifacts); err != nil {
		return nil, fmt.Errorf("release manifest is not readable JSON: %v", err)
	}
	if artifacts.Schema != "relkit.release-artifacts/1" {
		return nil, fmt.Errorf("release manifest schema must be 'relkit.release-artifacts/1'")
	}
	if strings.TrimSpace(artifacts.Version) != expectedVersion {
		return nil, fmt.Errorf("release manifest version %q does not match project version %q", artifacts.Version, expectedVersion)
	}
	if artifacts.Install.Kind != "installer" {
		return nil, fmt.Errorf("install.kind must be \"installer\" for the full-install track")
	}
	if strings.TrimSpace(artifacts.Install.Selectors) == "" || strings.TrimSpace(artifacts.Payload.Selectors) == "" {
		return nil, fmt.Errorf("install/payload selectors must be non-empty")
	}
	installPath := filepath.Join(root, artifacts.Install.Path)
	if info, err := os.Stat(installPath); err != nil || info.Size() == 0 {
		return nil, fmt.Errorf("install artifact missing or empty: %s", artifacts.Install.Path)
	}
	payloadPath := filepath.Join(root, artifacts.Payload.Path)
	if info, err := os.Stat(payloadPath); err != nil || !info.IsDir() {
		return nil, fmt.Errorf("payload path must be a directory: %s", artifacts.Payload.Path)
	}
	for i, archive := range artifacts.Archives {
		role := strings.TrimSpace(archive.Role)
		if role == "" {
			role = "ci-only"
		}
		if role != "ci-only" {
			return nil, fmt.Errorf("archives[%d].role must be ci-only (archives are never staged)", i)
		}
		archivePath := filepath.Join(root, archive.Path)
		if info, err := os.Stat(archivePath); err != nil || info.Size() == 0 {
			return nil, fmt.Errorf("archives[%d] missing or empty: %s", i, archive.Path)
		}
	}
	return &ciReleaseArtifacts{
		Schema:   artifacts.Schema,
		Version:  artifacts.Version,
		Install:  artifacts.Install,
		Payload:  artifacts.Payload,
		Archives: artifacts.Archives,
	}, nil
}

// resolveCIChannel validates the channel against relkit.json defaults.
func resolveCIChannel(explicit string) error {
	data, err := os.ReadFile(filepath.Join(".", "relkit.json"))
	if err != nil {
		return fmt.Errorf("relkit.json not found; run keys gen --execute first")
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("relkit.json is not valid JSON: %v", err)
	}
	allowed := []string{"stable", "dev"}
	if channels, ok := config["channels"].([]any); ok {
		allowed = nil
		for _, item := range channels {
			if s := strings.TrimSpace(fmt.Sprint(item)); s != "" {
				allowed = append(allowed, s)
			}
		}
	}
	channel := strings.TrimSpace(explicit)
	if channel == "" {
		channel = strings.TrimSpace(os.Getenv("RUP_CHANNEL"))
	}
	for _, a := range allowed {
		if channel == a {
			return nil
		}
	}
	return fmt.Errorf("--channel must be one of %s; got %s", strings.Join(allowed, ", "), channel)
}

// cmdFakeVerify implements `relkit fake verify [--version X.Y.Z]`: stage a
// dummy zip when no staged tree exists, then simulate. The onboarding state
// file write stays Python-side until the release batch; here we only gate.
func cmdFakeVerify(version string) error {
	resolved := strings.TrimPrefix(strings.TrimSpace(version), "v")
	if resolved == "" {
		v, err := projectVersionForRelkit(".")
		if err != nil {
			return fmt.Errorf("fake verify needs a version")
		}
		resolved = v
	}
	staged := filepath.Join(".", ".relkit", "cache", "staged", resolved, "staged.pb")
	if _, err := os.Stat(staged); err != nil {
		return fmt.Errorf("no staged tree for %s; run relkit stage first (dummy staging lands with the release batch)", resolved)
	}
	return dispatch([]string{"simulate", "--with-staged", resolved, "--from", "all"}, "")
}
