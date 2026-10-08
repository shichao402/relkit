package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/shichao402/relkit/internal/consume"
	"github.com/shichao402/relkit/internal/registry"
)

// cmdConsumeInstall implements `relkit install`: download and place every
// lock-pinned artifact the detected stack consumes. Mirrors relkit_host.py
// cmd_install + relkit_consume.py install for the product CI surface.
func cmdConsumeInstall(args []string) error {
	lockPath := ""
	root := ""
	target := "host"
	resolvedOut := ""
	var components []string
	var sdks []string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--lock":
			i++
			lockPath = mustValue(args, i, "--lock")
		case arg == "--project-root":
			i++
			root = mustValue(args, i, "--project-root")
		case arg == "--target":
			i++
			target = mustValue(args, i, "--target")
		case arg == "--component":
			components = append(components, mustValue(args, i, "--component"))
		case arg == "--sdk":
			i++
			sdks = append(sdks, mustValue(args, i, "--sdk"))
		case arg == "--resolved-out":
			resolvedOut = mustValue(args, i, "--resolved-out")
		default:
			return fmt.Errorf("unknown flag %q", arg)
		}
	}

	if root == "" {
		root = "."
	}
	if lockPath == "" {
		lockPath = filepath.Join(root, "scripts", "relkit.lock.json")
	}

	lock, err := consume.LoadLock(lockPath)
	if err != nil {
		return err
	}
	if len(sdks) > 0 {
		// --sdk declares the materialization scope (issue #27): validate
		// against the registry's UpdaterProcess names, then persist into
		// the lock so check/status stay in the declared scope without the
		// flag — the lock is the single source of truth for scope, not the
		// command line that happened to run install.
		normalized, err := consume.ValidateSdks(sdks)
		if err != nil {
			return err
		}
		lock.Sdks = normalized
		if err := lock.WriteLock(lockPath); err != nil {
			return fmt.Errorf("persisting sdks declaration into %s: %w", filepath.ToSlash(lockPath), err)
		}
		fmt.Printf("relkit: sdks %s recorded in %s\n", strings.Join(normalized, ","), filepath.ToSlash(lockPath))
	}
	lock, err = resolveFollowLatest(root, lock, lockPath)
	if err != nil {
		return err
	}
	if target == "host" {
		target, err = consume.HostTarget()
		if err != nil {
			return err
		}
	}

	names, err := consumeComponentsFor(root, components, lock)
	if err != nil {
		return err
	}

	// A release that predates an SDK attachment simply has nothing to
	// install; the lock stays the single source of truth either way.
	pinned := lock.PinnedComponents()
	var installable []string
	for _, name := range names {
		if strings.HasPrefix(name, "sdk-") && pinned != nil && !pinned[name] {
			fmt.Printf("relkit: %s is not pinned by the lock; skipping\n", name)
			continue
		}
		installable = append(installable, name)
	}

	dl := &consume.HTTPDownloader{}
	resolved := consume.ResolvedArtifacts{
		Schema:    lock.Schema,
		Release:   lock.Release,
		Commit:    lock.Commit,
		Target:    target,
		Artifacts: map[string]string{},
	}
	for _, name := range installable {
		if name == "updater" && lock.Schema == consume.SchemaV3 && !pinned[name] {
			// ADR 0017 decision 7: the updater exits the artifacts block on
			// consume/3; install it through the module channel instead —
			// unless an identical, working binary is already placed (an
			// injected env bundle pre-provisions it, mirroring how
			// checkModuleChannelUpdater verifies: exists + --version probe).
			if updaterAlreadyPlaced(root, target) {
				continue
			}
			if err := installUpdaterViaModule(root, lock); err != nil {
				return err
			}
			continue
		}
		spec, err := lock.ArtifactSpecFor(name, target)
		if err != nil {
			return err
		}
		resolved.Artifacts[name] = spec.SHA256

		if name == "host-scripts" {
			if err := installHostScripts(root, lock, spec); err != nil {
				return err
			}
			continue
		}
		artifact, err := consume.DownloadArtifact(root, name, spec, dl)
		if err != nil {
			return err
		}
		row := registry.ByName[name]
		switch row.Role {
		case registry.RoleProductTree:
			destination := filepath.Join(root, filepath.FromSlash(row.Destination))
			if err := consume.ExtractTree(artifact, destination, spec.SHA256, name); err != nil {
				return err
			}
			if name == "sdk-rust" {
				consume.ClearOrphanRelkitProto(root)
			}
		default:
			if _, err := consume.InstallBinary(root, name, target, artifact, spec.SHA256); err != nil {
				return err
			}
		}
	}
	if resolvedOut != "" {
		if err := consume.WriteResolved(resolvedOut, &resolved); err != nil {
			return err
		}
	}
	fmt.Printf("relkit consume: install complete release=%s target=%s\n", lock.Release, target)
	return nil
}

// installUpdaterViaModule installs the updater through the module channel
// (ADR 0017 decision 7): go install the pinned module version, then place the
// built binary under the registry install name. GOPROXY stays untouched so
// the unified entry (mirrors.tencent.com/go/) set by the environment wins;
// only when nothing is configured do we seed the unified default.
func installUpdaterViaModule(root string, lock *consume.Lock) error {
	module := "github.com/shichao402/relkit/cmd/relkit-updater"
	version := lock.Source.Version
	if lock.Source != nil && lock.Source.Module != "" {
		module = lock.Source.Module + "/cmd/relkit-updater"
	}
	if version == "" {
		return fmt.Errorf("consume/3 lock has no source.version; cannot install updater via module channel")
	}

	command := exec.Command("go", "install", module+"@"+version)
	command.Dir = root
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if strings.TrimSpace(os.Getenv("GOPROXY")) == "" {
		command.Env = append(os.Environ(), "GOPROXY=https://mirrors.tencent.com/go/")
	}
	fmt.Printf("relkit: installing updater via module channel: go install %s@%s\n", module, version)
	if err := command.Run(); err != nil {
		return fmt.Errorf("go install %s@%s failed: %w", module, version, err)
	}

	// go install drops the binary into GOBIN/GOPATH/bin; place it under the
	// registry install name inside the product tree (tools/bin).
	row := registry.ByName["updater"]
	installName, err := row.InstallName(hostTargetString())
	if err != nil {
		return err
	}
	destination := filepath.Join(root, filepath.FromSlash(row.Destination), installName)
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	built := filepath.Join(goBinDir(), updaterBinaryName())
	if _, err := os.Stat(built); err != nil {
		return fmt.Errorf("go install succeeded but %s is missing", built)
	}
	if err := copyFile(built, destination); err != nil {
		return err
	}
	fmt.Printf("relkit: updater placed at %s\n", filepath.ToSlash(destination))
	return nil
}

// updaterAlreadyPlaced reports whether a working updater binary already
// sits under the registry install name: exists + the same --version probe
// checkModuleChannelUpdater gates on. An injected env bundle (relkit-env/1)
// pre-provisions the updater so hostile-network build machines never fall
// back to the go module channel; a missing or broken binary still installs
// through the module channel as before.
func updaterAlreadyPlaced(root, target string) bool {
	row := registry.ByName["updater"]
	installName, err := row.InstallName(target)
	if err != nil {
		return false
	}
	placed := filepath.Join(root, filepath.FromSlash(row.Destination), installName)
	if _, statErr := os.Stat(placed); statErr != nil {
		return false
	}
	if probeErr := consume.SmokeTest(placed); probeErr != nil {
		fmt.Printf("relkit: placed updater failed probe (%v); reinstalling via module channel\n", probeErr)
		return false
	}
	fmt.Printf("relkit: updater already installed at %s; skipping module channel\n", filepath.ToSlash(placed))
	return true
}

func hostTargetString() string {
	return runtime.GOOS + "-" + runtime.GOARCH
}

func updaterBinaryName() string {
	if runtime.GOOS == "windows" {
		return "relkit-updater.exe"
	}
	return "relkit-updater"
}

// goBinDir resolves GOBIN or GOPATH/bin (where go install places binaries).
func goBinDir() string {
	if gobin := strings.TrimSpace(os.Getenv("GOBIN")); gobin != "" {
		return gobin
	}
	gopath := os.Getenv("GOPATH")
	if gopath == "" {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, "go", "bin")
		}
		return ""
	}
	return filepath.Join(strings.Split(gopath, string(os.PathListSeparator))[0], "bin")
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	tmp := dst + ".tmp"
	if err := os.WriteFile(tmp, data, 0o755); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}
