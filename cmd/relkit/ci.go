package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/shichao402/relkit/internal/model"
	"github.com/shichao402/relkit/internal/releasegate"
	"github.com/shichao402/relkit/internal/stage"
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

// ciReleaseArtifacts is the relkit.release-artifacts/2 manifest written by
// the product's release.packScript. Schema /1 (single install/payload pair)
// is still accepted: loadReleaseArtifactsManifest normalizes it into one
// selector group, so downstream staging logic never branches on schema.
type ciReleaseArtifacts struct {
	Schema   string           `json:"schema"`
	Version  string           `json:"version"`
	Groups   []ciSelectorGroup `json:"selectorGroups"`
	Archives []ciArchiveRef   `json:"archives"`
}

// ciSelectorGroup is one selector profile: a full-install artifact plus its
// matching update payloads. Each group expands to its own --install/--payload
// argv pair when staging.
type ciSelectorGroup struct {
	Selectors map[string]string `json:"selectors"`
	Install   ciArtifactRef     `json:"install"`
	Payloads  []ciPayloadRef    `json:"payloads"`
}

// ciArtifactRef names one install-track file. Kind mirrors the stage engine
// (installer/binary/blob); Filename is the download-side archive name and
// only the manifest /2 path sets it explicitly.
type ciArtifactRef struct {
	Path     string            `json:"path"`
	Kind     string            `json:"kind"`
	Filename string            `json:"filename,omitempty"`
	Selectors map[string]string `json:"selectors,omitempty"`
}

// ciPayloadRef names one payload-track directory. Payloads are always kind
// payload with apply=relkit-payload, so only path and selectors remain.
type ciPayloadRef struct {
	Path     string            `json:"path"`
	Filename string            `json:"filename,omitempty"`
	Selectors map[string]string `json:"selectors,omitempty"`
}

type ciArchiveRef struct {
	Path string `json:"path"`
	Role string `json:"role"`
}

// ciArtifactRefWire is the /1 wire form of one artifact reference: selectors
// are a comma-joined string. Only the loader consumes it; everything past
// loadReleaseArtifactsManifest works with structured maps.
type ciArtifactRefWire struct {
	Path      string `json:"path"`
	Kind      string `json:"kind"`
	Selectors string `json:"selectors"`
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

	stageArgs := []string{"stage", version, "--channel", opts.channel}
	for _, group := range artifacts.Groups {
		stageArgs = append(stageArgs, "--install", group.Install.Path)
		stageArgs = append(stageArgs, installPairsText(group)...)
		for _, payload := range group.Payloads {
			stageArgs = append(stageArgs, "--payload", payload.Path)
			stageArgs = append(stageArgs, payloadPairsText(payload)...)
		}
	}
	fmt.Printf("staging groups=%d archives=%d (ci-only, not staged)\n",
		len(artifacts.Groups), len(artifacts.Archives))
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

// loadReleaseArtifactsManifest reads and validates the release manifest
// written by packScript. Schema /2 (selectorGroups) is the structured form:
// selectors are maps and id/kind/filename are explicit fields. Schema /1
// (single install/payload with comma-joined selector strings) is accepted
// for compatibility and normalized into one selector group.
func loadReleaseArtifactsManifest(root, manifestPath, expectedVersion string) (*ciReleaseArtifacts, error) {
	path := filepath.Join(root, manifestPath)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("release manifest missing after packScript: %s", manifestPath)
	}
	var raw struct {
		Schema         string            `json:"schema"`
		Version        string            `json:"version"`
		SelectorGroups []json.RawMessage `json:"selectorGroups"`
		Install        *ciArtifactRefWire `json:"install"`
		Payload        *ciArtifactRefWire `json:"payload"`
		Archives       []ciArchiveRef    `json:"archives"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("release manifest is not readable JSON: %v", err)
	}
	if raw.Schema != "relkit.release-artifacts/2" && raw.Schema != "relkit.release-artifacts/1" {
		return nil, fmt.Errorf("release manifest schema must be 'relkit.release-artifacts/2' (or /1)")
	}
	if strings.TrimSpace(raw.Version) != expectedVersion {
		return nil, fmt.Errorf("release manifest version %q does not match project version %q", raw.Version, expectedVersion)
	}

	var groups []ciSelectorGroup
	if raw.Schema == "relkit.release-artifacts/2" {
		if len(raw.SelectorGroups) == 0 {
			return nil, fmt.Errorf("selectorGroups must contain at least one group")
		}
		for i, entry := range raw.SelectorGroups {
			group, err := decodeSelectorGroup(entry)
			if err != nil {
				return nil, fmt.Errorf("selectorGroups[%d]: %v", i, err)
			}
			groups = append(groups, group)
		}
	} else {
		if raw.Install == nil || raw.Payload == nil {
			return nil, fmt.Errorf("schema /1 requires both install and payload")
		}
		installSelectors, err := parseSelectorsString(raw.Install.Selectors, "install.selectors")
		if err != nil {
			return nil, err
		}
		payloadSelectors, err := parseSelectorsString(raw.Payload.Selectors, "payload.selectors")
		if err != nil {
			return nil, err
		}
		groups = []ciSelectorGroup{{
			Selectors: installSelectors,
			Install:   ciArtifactRef{Path: raw.Install.Path, Kind: raw.Install.Kind},
			Payloads:  []ciPayloadRef{{Path: raw.Payload.Path, Selectors: payloadSelectors}},
		}}
	}

	artifacts := &ciReleaseArtifacts{
		Schema:   raw.Schema,
		Version:  raw.Version,
		Groups:   groups,
		Archives: raw.Archives,
	}
	if err := validateGroups(root, artifacts); err != nil {
		return nil, err
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
	return artifacts, nil
}

// decodeSelectorGroup unmarshals one selectorGroups entry. The wire format
// keeps manifest-level fields (id/kind/filename) explicit so no string
// parsing survives past this function.
func decodeSelectorGroup(entry json.RawMessage) (ciSelectorGroup, error) {
	var group struct {
		Selectors map[string]string `json:"selectors"`
		Install   *ciArtifactRef    `json:"install"`
		Payloads  []ciPayloadRef    `json:"payloads"`
	}
	if err := json.Unmarshal(entry, &group); err != nil {
		return ciSelectorGroup{}, err
	}
	if len(group.Selectors) == 0 {
		return ciSelectorGroup{}, fmt.Errorf("selectors must be a non-empty object")
	}
	if group.Install == nil {
		return ciSelectorGroup{}, fmt.Errorf("install is required in every group")
	}
	if len(group.Payloads) == 0 {
		return ciSelectorGroup{}, fmt.Errorf("payloads must contain at least one payload directory")
	}
	for j, payload := range group.Payloads {
		if len(payload.Selectors) == 0 {
			return ciSelectorGroup{}, fmt.Errorf("payloads[%d].selectors must be a non-empty object", j)
		}
	}
	return ciSelectorGroup{
		Selectors: group.Selectors,
		Install:   *group.Install,
		Payloads:  group.Payloads,
	}, nil
}

// parseSelectorsString decodes the /1 comma-joined selector form
// ("os=windows,arch=amd64") into a map. Only used on the /1 compatibility
// path; /2 manifests carry structured maps.
func parseSelectorsString(text, what string) (map[string]string, error) {
	pairs, err := stage.ParseKeyValues(strings.TrimSpace(text))
	if err != nil {
		return nil, fmt.Errorf("%s: %v", what, err)
	}
	if len(pairs) == 0 {
		return nil, fmt.Errorf("%s must be non-empty", what)
	}
	return pairs, nil
}

// validateGroups enforces the invariants every group must hold regardless of
// schema version: group selectors non-empty and well-formed, install present
// on disk with a stage-recognized kind, payload directories present, and
// selector keys shared between install and its payloads.
func validateGroups(root string, artifacts *ciReleaseArtifacts) error {
	if len(artifacts.Groups) == 0 {
		return fmt.Errorf("selectorGroups must contain at least one group")
	}
	for i, group := range artifacts.Groups {
		if err := model.CheckSelectors(group.Selectors, fmt.Sprintf("selectorGroups[%d].selectors", i)); err != nil {
			return err
		}
		switch group.Install.Kind {
		case "installer", "binary", "blob":
		default:
			return fmt.Errorf("selectorGroups[%d].install.kind must be installer, binary, or blob; got %q", i, group.Install.Kind)
		}
		installPath := filepath.Join(root, group.Install.Path)
		if info, err := os.Stat(installPath); err != nil || info.Size() == 0 {
			return fmt.Errorf("selectorGroups[%d].install missing or empty: %s", i, group.Install.Path)
		}
		for j, payload := range group.Payloads {
			if err := model.CheckSelectors(payload.Selectors, fmt.Sprintf("selectorGroups[%d].payloads[%d].selectors", i, j)); err != nil {
				return err
			}
			payloadPath := filepath.Join(root, payload.Path)
			if info, err := os.Stat(payloadPath); err != nil || !info.IsDir() {
				return fmt.Errorf("selectorGroups[%d].payloads[%d] path must be a directory: %s", i, j, payload.Path)
			}
		}
	}
	return nil
}

// installPairsText renders one group's install pairs for the stage argv. The
// stage engine still accepts k=v text (hand-written CLI), and ci release is
// an in-process caller of that same engine, so the text form is assembled
// here without a subprocess boundary.
func installPairsText(group ciSelectorGroup) []string {
	pairs := make([]string, 0, len(group.Selectors)+3)
	pairs = append(pairs, "kind="+group.Install.Kind)
	if group.Install.Filename != "" {
		pairs = append(pairs, "filename="+group.Install.Filename)
	}
	for _, key := range sortedKeys(group.Selectors) {
		pairs = append(pairs, key+"="+group.Selectors[key])
	}
	return []string{strings.Join(pairs, ",")}
}

// payloadPairsText renders one payload's pairs for the stage argv.
func payloadPairsText(payload ciPayloadRef) []string {
	pairs := make([]string, 0, len(payload.Selectors)+3)
	if payload.Filename != "" {
		pairs = append(pairs, "filename="+payload.Filename)
	}
	for _, key := range sortedKeys(payload.Selectors) {
		pairs = append(pairs, key+"="+payload.Selectors[key])
	}
	return []string{strings.Join(pairs, ",")}
}

// sortedKeys returns map keys in sorted order for deterministic argv.
func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
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
