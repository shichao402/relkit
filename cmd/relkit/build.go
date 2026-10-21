package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/shichao402/relkit/internal/build"
	"github.com/shichao402/relkit/internal/publishproto"
	"github.com/shichao402/relkit/internal/vnext/registry"
)

// cmdBuild implements `relkit build`: cross-compile host/product binaries and
// pack product-tree zips, writing the relkit.release/1 manifest. The component
// table and asset naming rules come from the single vNext registry; the
// retired internal/registry table and scripts/host/release-contract.json are
// gone (ADR 0019 §1).
func cmdBuild(args []string) error {
	root := "."
	outDir := "dist"
	anyOS := ""
	anyArch := ""
	all := false
	checkTag := ""
	selected := map[string]bool{}
	registryDocument, err := registry.Default()
	if err != nil {
		return err
	}

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--project-root":
			i++
			root = mustValue(args, i, "--project-root")
		case arg == "--out":
			i++
			outDir = mustValue(args, i, "--out")
		case arg == "--os":
			i++
			anyOS = mustValue(args, i, "--os")
		case arg == "--arch":
			i++
			anyArch = mustValue(args, i, "--arch")
		case arg == "--check-tag":
			i++
			checkTag = mustValue(args, i, "--check-tag")
		case arg == "--all":
			all = true
		case strings.HasPrefix(arg, "--"):
			flag := strings.TrimPrefix(arg, "--")
			if _, ok := registryDocument.ByBuildFlag(flag); !ok {
				return fmt.Errorf("unknown flag %q", arg)
			}
			selected[flag] = true
		default:
			return fmt.Errorf("unknown argument %q", arg)
		}
	}

	outPath := filepath.FromSlash(outDir)
	if !filepath.IsAbs(outPath) {
		outPath = filepath.Join(root, outPath)
	}
	if !filepath.IsAbs(outPath) {
		abs, err := filepath.Abs(outPath)
		if err != nil {
			return err
		}
		outPath = abs
	}
	if err := os.MkdirAll(outPath, 0o755); err != nil {
		return err
	}

	binaryRows := registryDocument.InRoles(registry.RoleInfrastructureBinary, registry.RoleProductBinary)
	var targets []string
	var treeRows []registry.Component
	treeCandidates := registryDocument.InRoles(registry.RoleFacade, registry.RoleProjection)
	if all {
		for _, row := range binaryRows {
			targets = append(targets, row.Name)
		}
		treeRows = treeCandidates
	} else {
		for _, row := range binaryRows {
			if selected[row.Build.Flag] {
				targets = append(targets, row.Name)
			}
		}
		for _, row := range treeCandidates {
			if selected[row.Build.Flag] {
				treeRows = append(treeRows, row)
			}
		}
		if len(targets) == 0 && len(treeRows) == 0 {
			targets = []string{"store", "agent"}
		}
	}

	platforms := [][2]string{}
	for _, target := range registryDocument.BuildTargets {
		parts := strings.SplitN(target, "-", 2)
		platforms = append(platforms, [2]string{parts[0], parts[1]})
	}
	if anyOS != "" {
		filtered := platforms[:0]
		for _, p := range platforms {
			if p[0] == anyOS {
				filtered = append(filtered, p)
			}
		}
		platforms = filtered
	}
	if anyArch != "" {
		filtered := platforms[:0]
		for _, p := range platforms {
			if p[1] == anyArch {
				filtered = append(filtered, p)
			}
		}
		platforms = filtered
	}
	if len(platforms) == 0 {
		return fmt.Errorf("no build platforms left after --os/--arch filters")
	}

	number, err := ssotNumber(root)
	if err != nil {
		return err
	}
	if checkTag != "" {
		expected := "v" + number
		if checkTag != expected {
			return fmt.Errorf("git tag %s does not match VERSION.json %s", checkTag, expected)
		}
		fmt.Println(expected)
	}
	ctx := &build.RepoContext{Root: root, Number: number}
	stamp, err := ctx.Stamp()
	if err != nil {
		return err
	}
	fmt.Printf("build %s\n", stamp)

	// The retired scripts/host/release-contract.json is gone (ADR 0019 §1).
	// The protocol and updater IPC windows now come from their Go sources of
	// truth, which is where the contract file used to be pinned from anyway.
	ipcMin, ipcMax, err := build.UpdateIPCWindow(root)
	if err != nil {
		return err
	}
	if publishproto.Min <= 0 || publishproto.Max < publishproto.Min {
		return fmt.Errorf("internal/publishproto declares an invalid protocol window [%d,%d]", publishproto.Min, publishproto.Max)
	}
	if ipcMin <= 0 || ipcMax < ipcMin {
		return fmt.Errorf("internal/updater declares an invalid IPC window [%d,%d]", ipcMin, ipcMax)
	}

	type builtArtifact struct {
		Component string `json:"component"`
		OS        string `json:"os"`
		Arch      string `json:"arch"`
		Path      string `json:"path"`
		SHA256    string `json:"sha256"`
	}
	var built []builtArtifact

	for _, name := range targets {
		row, ok := registryDocument.ByName(name)
		if !ok {
			return fmt.Errorf("component %q is not registered", name)
		}
		ldflags := "-s -w -X main.version=" + stamp
		for _, platform := range platforms {
			osName, arch := platform[0], platform[1]
			binaryName := fmt.Sprintf("%s-%s-%s", row.Build.BinaryPrefix, osName, arch)
			if osName == "windows" {
				binaryName += ".exe"
			}
			destination := filepath.Join(outPath, binaryName)
			fmt.Printf("  %s\n", binaryName)
			if err := goBuildCross(root, row.Build.GoPackage, destination, ldflags, osName, arch); err != nil {
				return err
			}
			digest, err := build.FileSHA256(destination)
			if err != nil {
				return err
			}
			built = append(built, builtArtifact{Component: name, OS: osName, Arch: arch, Path: binaryName, SHA256: digest})
		}
	}

	for _, row := range treeRows {
		entries, err := ctx.ComponentEntries(registryDocument, row.Name)
		if err != nil {
			return err
		}
		archive := filepath.Join(outPath, row.Build.Archive)
		if err := build.WriteDeterministicZip(archive, entries); err != nil {
			return err
		}
		digest, err := build.FileSHA256(archive)
		if err != nil {
			return err
		}
		built = append(built, builtArtifact{Component: row.Name, OS: "any", Arch: "any", Path: row.Build.Archive, SHA256: digest})
	}

	ident, err := ctx.Identity()
	if err != nil {
		return err
	}
	manifest := map[string]any{
		"schema":        "relkit.release/1",
		"version":       stamp,
		"commit":        ident.Commit,
		"dirty":         ident.Dirty,
		"minProtocol":   publishproto.Min,
		"maxProtocol":   publishproto.Max,
		"minUpdaterIpc": ipcMin,
		"maxUpdaterIpc": ipcMax,
		"builtAt":       time.Now().UTC().Format("2006-01-02T15:04:05Z"),
		"artifacts":     built,
	}
	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(outPath, "manifest.json"), append(manifestData, '\n'), 0o644); err != nil {
		return err
	}
	fmt.Printf("output in %s\n", filepath.ToSlash(outPath))
	return nil
}

// ssotDocument is the parsed relkit.version/1 VERSION.json.
type ssotDocument struct {
	Version string
	Number  string
	Build   int
	Tag     string
}

// loadSSOTDocument parses and validates VERSION.json (mirrors
// parse_ssot_document in scripts/deploy/relkit_ops.py).
func loadSSOTDocument(path string) (*ssotDocument, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("missing VERSION.json; it is the relkit version SSOT")
	}
	var doc struct {
		Schema  string `json:"schema"`
		Version string `json:"version"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("VERSION.json is not valid JSON: %v", err)
	}
	if doc.Schema != "relkit.version/1" {
		return nil, fmt.Errorf("VERSION.json schema must be relkit.version/1")
	}
	match := regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+)\+(\d+)$`).FindStringSubmatch(strings.TrimSpace(doc.Version))
	if match == nil {
		return nil, fmt.Errorf("VERSION.json version must be x.y.z+build")
	}
	return &ssotDocument{
		Version: strings.TrimSpace(doc.Version),
		Number:  match[1] + "." + match[2] + "." + match[3],
		Build:   mustAtoi(match[4]),
		Tag:     "v" + match[1] + "." + match[2] + "." + match[3],
	}, nil
}

// ssotNumber reads the relkit.version/1 SSOT document and returns the x.y.z
// number (mirrors parse_ssot_document in scripts/deploy/relkit_ops.py).
func ssotNumber(root string) (string, error) {
	doc, err := loadSSOTDocument(filepath.Join(root, "VERSION.json"))
	if err != nil {
		return "", err
	}
	return doc.Number, nil
}

func mustAtoi(value string) int {
	n, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}
	return n
}

// goBuildCross runs go build with GOOS/GOARCH pinned and CGO off.
func goBuildCross(root, pkg, dest, ldflags, osName, arch string) error {
	command := exec.Command("go", "build", "-trimpath", "-ldflags", ldflags, "-o", dest, pkg)
	command.Dir = root
	command.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS="+osName, "GOARCH="+arch)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	return command.Run()
}

func intOf(v any) int {
	switch value := v.(type) {
	case int:
		return value
	case float64:
		return int(value)
	case json.Number:
		n, _ := value.Int64()
		return int(n)
	}
	return 0
}
