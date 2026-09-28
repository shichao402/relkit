// Package registry is the single source of truth for every relkit release
// component (ADR 0017 decision 6). It supersedes scripts/host/hostlib/facets.py.
// During the dual-implementation window this table mirrors facets.py exactly
// (same rows, same fields, same ordering); the migration completes by flipping
// Python consumers to a contract file generated from here, then deleting
// facets.py. Row order matters: deploy build --all and consume component
// selection iterate it.
package registry

import (
	"fmt"
	"path"
	"sort"
	"strings"
)

// Role classifies what kind of release component a row describes.
type Role string

const (
	// RoleHostBinary runs only on relkit infrastructure machines (store,
	// console, agent). Product repositories never consume them.
	RoleHostBinary Role = "host-binary"
	// RoleProductBinary lands inside the product tree at a fixed destination
	// (cli, updater).
	RoleProductBinary Role = "product-binary"
	// RoleProductTree is a zipped tree unpacked into the product tree
	// (SDKs, bindings, host-scripts).
	RoleProductTree Role = "product-tree"
)

// Probe names how install verifies an installed component.
type Probe string

const (
	ProbeFiles      Probe = "files"
	ProbeTreeSHA256 Probe = "tree-sha256"
)

// PackKind identifies how deploy packs a product-tree zip.
type PackKind string

const (
	PackTrackedTree PackKind = "tracked-tree"
	PackWorkingTree PackKind = "working-tree"
	PackListedFiles PackKind = "listed-files"
	PackGoDeps      PackKind = "go-deps"
)

// HostAPI marks which host integration surfaces a component carries.
const (
	HostAPIProtocol = "protocol"
	HostAPIFacade   = "facade"
)

// Targets lists every host target relkit ships binaries for.
var Targets = []string{
	"linux-amd64",
	"linux-arm64",
	"windows-amd64",
	"darwin-amd64",
	"darwin-arm64",
}

// PackConfig mirrors the Python facets.Component pack fields.
type PackConfig struct {
	Kind            PackKind
	Files           []string
	ExcludeParts    []string
	ExcludeSuffixes []string
	ExcludePaths    []string
	PackExtras      [][2]string
	GoEntrypoints   []string
	GoModule        string
}

// Component describes one release artifact family. Field set mirrors the
// Python facets.Component dataclass.
type Component struct {
	Name              string
	Role              Role
	BuildFlag         string
	Archive           string
	Source            string
	Destination       string
	Probe             Probe
	RequiredPaths     []string
	GoPackage         string
	BinaryPrefix      string
	Portable          bool
	DefaultConsume    bool
	Detect            []string
	UpdaterProcess    string
	InstallNames      map[string]string
	ImportSignals     []string
	WebviewProjection bool
	HostAPI           []string
	FacadePaths       []string
	Pack              PackConfig
}

// Components lists every release component, mirroring facets.COMPONENTS.
var Components = []Component{
	{
		Name:         "store",
		Role:         RoleHostBinary,
		BuildFlag:    "store",
		GoPackage:    "./cmd/relkit-store",
		BinaryPrefix: "relkit-store",
		Probe:        ProbeFiles,
	},
	{
		Name:         "console",
		Role:         RoleHostBinary,
		BuildFlag:    "console",
		GoPackage:    "./cmd/relkit-console",
		BinaryPrefix: "relkit-console",
		Probe:        ProbeFiles,
	},
	{
		Name:         "agent",
		Role:         RoleHostBinary,
		BuildFlag:    "agent",
		GoPackage:    "./cmd/relkit-agent",
		BinaryPrefix: "relkit-agent",
		Probe:        ProbeFiles,
	},
	{
		Name:           "cli",
		Role:           RoleProductBinary,
		BuildFlag:      "cli",
		GoPackage:      "./cmd/relkit",
		BinaryPrefix:   "relkit",
		Destination:    "tools/bin",
		Probe:          ProbeFiles,
		DefaultConsume: true,
		InstallNames: map[string]string{
			"windows-amd64": "relkit.exe",
			"linux-amd64":   "relkit-linux-amd64",
			"*":             "relkit",
		},
	},
	{
		Name:           "updater",
		Role:           RoleProductBinary,
		BuildFlag:      "updater",
		GoPackage:      "./cmd/relkit-updater",
		BinaryPrefix:   "relkit-updater",
		Destination:    "tools/bin",
		Probe:          ProbeFiles,
		DefaultConsume: true,
		InstallNames:   map[string]string{"*": "relkit-updater{exe}"},
	},
	{
		Name:        "host-scripts",
		Role:        RoleProductTree,
		BuildFlag:   "host-scripts",
		Archive:     "relkit-host-scripts.zip",
		Source:      "scripts/host",
		Destination: "scripts/host",
		Probe:       ProbeTreeSHA256,
		RequiredPaths: []string{
			"relkit_host.py",
			"relkit_consume.py",
			"hostlib/__init__.py",
			"hostlib/const.py",
			"hostlib/state.py",
			"hostlib/ssh.py",
			"hostlib/inspect.py",
			"hostlib/onboard.py",
			"hostlib/reconcile.py",
			"hostlib/remote.py",
			"hostlib/release.py",
			"hostlib/retrospect.py",
			"hostlib/facets.py",
			"hostlib/digest.py",
			"hostlib/gates.py",
			"hostlib/runtime.py",
		},
		Portable: true,
		Pack: PackConfig{
			Kind:         PackWorkingTree,
			ExcludeParts: []string{"__pycache__"},
		},
	},
	{
		Name:           "sdk-dart",
		Role:           RoleProductTree,
		BuildFlag:      "dart-sdk",
		Archive:        "relkit-sdk-dart.zip",
		Source:         "sdk/dart",
		Destination:    "third_party/relkit/sdk/dart",
		Probe:          ProbeFiles,
		RequiredPaths:  []string{"pubspec.yaml", "lib"},
		Portable:       true,
		DefaultConsume: true,
		Detect:         []string{"pubspec.yaml"},
		UpdaterProcess: "dart",
		ImportSignals:  []string{"package:rup_client/"},
		HostAPI:        []string{"protocol", "facade"},
		FacadePaths:    []string{"lib/src/updater_facade.dart"},
		Pack: PackConfig{
			Kind: PackTrackedTree,
			ExcludePaths: []string{
				"lib/src/updater.dart",
				"lib/src/scheduler.dart",
				"example/",
				"test/",
			},
		},
	},
	{
		Name:           "sdk-go",
		Role:           RoleProductTree,
		BuildFlag:      "go-sdk",
		Archive:        "relkit-sdk-go.zip",
		Source:         ".",
		Destination:    "third_party/relkit",
		Probe:          ProbeFiles,
		RequiredPaths:  []string{"go.mod", "sdk", "api/updater/v1"},
		Portable:       true,
		Detect:         []string{"go.mod"},
		UpdaterProcess: "go",
		ImportSignals:  []string{"github.com/shichao402/relkit/sdk"},
		HostAPI:        []string{"protocol", "facade"},
		FacadePaths:    []string{"sdk/updaterfacade/facade.go"},
		Pack: PackConfig{
			Kind:            PackGoDeps,
			Files:           []string{"go.mod", "go.sum"},
			ExcludeSuffixes: []string{"_test.go"},
			GoEntrypoints:   []string{"./sdk", "./sdk/updaterfacade"},
			GoModule:        "github.com/shichao402/relkit",
		},
	},
	{
		Name:           "sdk-rust",
		Role:           RoleProductTree,
		BuildFlag:      "rust-sdk",
		Archive:        "relkit-sdk-rust.zip",
		Source:         "sdk/rust",
		Destination:    "third_party/relkit/sdk/rust",
		Probe:          ProbeFiles,
		RequiredPaths:  []string{"Cargo.toml", "src/lib.rs", "proto/updater/v1/updater.proto"},
		Portable:       true,
		Detect:         []string{"Cargo.toml", "**/src-tauri/Cargo.toml"},
		UpdaterProcess: "rust",
		ImportSignals:  []string{"relkit_updater", "relkit-updater"},
		HostAPI:        []string{"facade"},
		FacadePaths:    []string{"src/lib.rs"},
		Pack: PackConfig{
			Kind:       PackTrackedTree,
			PackExtras: [][2]string{{"proto/updater/v1/updater.proto", "proto/updater/v1/updater.proto"}},
		},
	},
	{
		Name:              "bindings-ts",
		Role:              RoleProductTree,
		BuildFlag:         "bindings-ts",
		Archive:           "relkit-bindings-ts.zip",
		Source:            "bindings/ts",
		Destination:       "third_party/relkit/bindings/ts",
		Probe:             ProbeFiles,
		RequiredPaths:     []string{"package.json", "dist/updater_pb.js", "dist/updater_pb.d.ts"},
		Portable:          true,
		Detect:            []string{"package.json", "**/package.json"},
		UpdaterProcess:    "node",
		ImportSignals:     []string{"third_party/relkit/bindings/ts", "@relkit/updater-bindings"},
		WebviewProjection: true,
		HostAPI:           []string{"facade"},
		FacadePaths:       []string{"dist/updater_pb.js"},
		Pack: PackConfig{
			Kind: PackListedFiles,
			Files: []string{
				"package.json",
				"README.md",
				"dist/updater_pb.js",
				"dist/updater_pb.d.ts",
			},
		},
	},
}

var ByName = func() map[string]Component {
	m := make(map[string]Component, len(Components))
	for _, row := range Components {
		m[row.Name] = row
	}
	return m
}()

// All returns every component row in registry order.
func All() []Component {
	out := make([]Component, len(Components))
	copy(out, Components)
	return out
}

// InRole returns components with the given role, in registry order.
func InRole(role Role) []Component {
	var out []Component
	for i := range Components {
		if Components[i].Role == role {
			out = append(out, Components[i])
		}
	}
	return out
}

// ProductComponents matches facets.product_components(): product-binary rows
// first, then product-tree rows, both in registry order.
func ProductComponents() []Component {
	out := make([]Component, 0, len(Components))
	out = append(out, InRole(RoleProductBinary)...)
	out = append(out, InRole(RoleProductTree)...)
	return out
}

// DefaultComponents matches facets.default_components().
func DefaultComponents() []Component {
	var out []Component
	for i := range Components {
		if Components[i].DefaultConsume {
			out = append(out, Components[i])
		}
	}
	return out
}

// HasComponent reports whether name exists.
func HasComponent(name string) bool {
	_, ok := ByName[name]
	return ok
}

// InstallName resolves the file name a product-binary lands under for target.
func (c *Component) InstallName(target string) (string, error) {
	if c.Role != RoleProductBinary {
		return "", fmt.Errorf("%s is not a binary component", c.Name)
	}
	if v, ok := c.InstallNames[target]; ok {
		return strings.ReplaceAll(v, "{exe}", exeSuffix(target)), nil
	}
	if v, ok := c.InstallNames["*"]; ok {
		return strings.ReplaceAll(v, "{exe}", exeSuffix(target)), nil
	}
	return "", fmt.Errorf("%s has no install name for %s", c.Name, target)
}

func exeSuffix(target string) string {
	if strings.HasPrefix(target, "windows-") {
		return ".exe"
	}
	return ""
}

// ArtifactFilename returns the release attachment name for this component at
// target. Product-tree rows return their archive name for every target.
func (c *Component) ArtifactFilename(target string) string {
	if c.Role == RoleProductTree {
		return c.Archive
	}
	name := fmt.Sprintf("%s-%s", c.BinaryPrefix, target)
	if strings.HasPrefix(target, "windows-") {
		return name + ".exe"
	}
	return name
}

// DetectedComponents mirrors facets.detected_components(): product-tree rows
// whose Detect pattern matches any signal, in registry order.
// Python's PurePosixPath.match() semantics: "**/name" matches name at any
// depth; "name" matches only at the root; a multi-segment pattern matches
// the trailing path segments of the signal.
func DetectedComponents(signals []string) []string {
	var names []string
	for i := range Components {
		row := &Components[i]
		if row.Role != RoleProductTree || len(row.Detect) == 0 {
			continue
		}
		matched := false
		for _, pattern := range row.Detect {
			for _, signal := range signals {
				if detectMatch(pattern, signal) {
					matched = true
					break
				}
			}
			if matched {
				break
			}
		}
		if matched {
			names = append(names, row.Name)
		}
	}
	return names
}

// detectMatch implements PurePosixPath.match semantics for the detect
// patterns used by the registry: matching is right-anchored ("Cargo.toml"
// matches "src-tauri/Cargo.toml"), "*" spans one segment, and a "**"
// segment absorbs every signal segment to its left. Patterns with more
// segments than the signal never match, mirroring pathlib.
func detectMatch(pattern, signal string) bool {
	patParts := strings.Split(pattern, "/")
	sigParts := strings.Split(signal, "/")
	if len(patParts) > len(sigParts) {
		return false
	}
	// Match from the tail; a "**" segment absorbs every remaining leading
	// signal segment (including zero).
	pi := len(patParts) - 1
	si := len(sigParts) - 1
	for pi >= 0 && si >= 0 {
		if patParts[pi] == "**" {
			return true
		}
		ok, err := path.Match(patParts[pi], sigParts[si])
		if err != nil || !ok {
			return false
		}
		pi--
		si--
	}
	// The pattern consumed all its segments against the signal tail; extra
	// leading signal segments are fine (right-anchored match). The length
	// guard above already rejected the pattern-longer case, so landing
	// here means the tail matched.
	return pi < 0
}

// IsValid validates the component table, mirroring facets.validate_components.
// It runs once in the package init below.
func IsValid() error {
	seen := map[string]bool{}
	flags := map[string]bool{}
	for i := range Components {
		row := &Components[i]
		if row.Name == "" || row.Role == "" || row.BuildFlag == "" || row.Probe == "" {
			return fmt.Errorf("component %q missing required fields", row.Name)
		}
		switch row.Role {
		case RoleHostBinary, RoleProductBinary:
			if row.GoPackage == "" || row.BinaryPrefix == "" {
				return fmt.Errorf("component %q missing goPackage/binaryPrefix", row.Name)
			}
		case RoleProductTree:
			if row.Archive == "" || row.Source == "" || row.Destination == "" || len(row.RequiredPaths) == 0 {
				return fmt.Errorf("component %q missing tree fields", row.Name)
			}
		default:
			return fmt.Errorf("component %q invalid role %q", row.Name, row.Role)
		}
		switch row.Probe {
		case ProbeFiles, ProbeTreeSHA256:
		default:
			return fmt.Errorf("component %q invalid probe %q", row.Name, row.Probe)
		}
		if seen[row.Name] || flags[row.BuildFlag] {
			return fmt.Errorf("duplicate component name/build flag %q", row.Name)
		}
		if row.Portable != (row.Role == RoleProductTree) {
			return fmt.Errorf("component %q portable disagrees with role", row.Name)
		}
		if row.Role != RoleProductTree {
			if row.Pack.Kind != "" || len(row.Pack.Files) > 0 || len(row.Pack.ExcludeParts) > 0 ||
				len(row.Pack.ExcludeSuffixes) > 0 || len(row.Pack.ExcludePaths) > 0 ||
				len(row.Pack.GoEntrypoints) > 0 || row.Pack.GoModule != "" ||
				len(row.HostAPI) > 0 || len(row.FacadePaths) > 0 || row.UpdaterProcess != "" {
				return fmt.Errorf("component %q pack fields only apply to product-tree", row.Name)
			}
		} else {
			kind := row.Pack.Kind
			if kind == "" {
				kind = PackTrackedTree
			}
			switch kind {
			case PackTrackedTree, PackWorkingTree:
			case PackListedFiles:
				if len(row.Pack.Files) == 0 {
					return fmt.Errorf("component %q listed-files pack requires files", row.Name)
				}
			case PackGoDeps:
				if len(row.Pack.GoEntrypoints) == 0 || row.Pack.GoModule == "" {
					return fmt.Errorf("component %q go-deps pack requires entrypoints and module", row.Name)
				}
			default:
				return fmt.Errorf("component %q invalid pack kind %q", row.Name, kind)
			}
			if kind != PackGoDeps && (len(row.Pack.GoEntrypoints) > 0 || row.Pack.GoModule != "") {
				return fmt.Errorf("component %q go-deps fields require pack=go-deps", row.Name)
			}
			if kind != PackListedFiles && kind != PackGoDeps && len(row.Pack.Files) > 0 {
				return fmt.Errorf("component %q pack files require listed-files or go-deps", row.Name)
			}
			if kind != PackWorkingTree && len(row.Pack.ExcludeParts) > 0 {
				return fmt.Errorf("component %q excludeParts require working-tree", row.Name)
			}
			if kind != PackGoDeps && len(row.Pack.ExcludeSuffixes) > 0 {
				return fmt.Errorf("component %q excludeSuffixes require go-deps", row.Name)
			}
			if kind != PackTrackedTree && kind != PackWorkingTree && len(row.Pack.ExcludePaths) > 0 {
				return fmt.Errorf("component %q excludePaths require tracked-tree or working-tree", row.Name)
			}
			if row.UpdaterProcess != "" {
				for _, api := range row.HostAPI {
					if api != HostAPIProtocol && api != HostAPIFacade {
						return fmt.Errorf("component %q invalid hostApi %q", row.Name, api)
					}
				}
				hasFacade := false
				for _, api := range row.HostAPI {
					if api == HostAPIFacade {
						hasFacade = true
					}
				}
				if !hasFacade {
					return fmt.Errorf("component %q updaterProcess requires hostApi facade", row.Name)
				}
				if len(row.FacadePaths) == 0 {
					return fmt.Errorf("component %q updaterProcess requires facadePaths", row.Name)
				}
			} else if len(row.HostAPI) > 0 || len(row.FacadePaths) > 0 {
				return fmt.Errorf("component %q hostApi/facadePaths require updaterProcess", row.Name)
			}
		}
		seen[row.Name] = true
		flags[row.BuildFlag] = true
	}
	return nil
}

func init() {
	if err := IsValid(); err != nil {
		panic("registry: " + err.Error())
	}
}

// SortedNames returns component names sorted alphabetically.
func SortedNames() []string {
	names := make([]string, 0, len(Components))
	for _, row := range Components {
		names = append(names, row.Name)
	}
	sort.Strings(names)
	return names
}
