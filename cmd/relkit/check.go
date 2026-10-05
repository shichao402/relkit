package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/shichao402/relkit/internal/consume"
	"github.com/shichao402/relkit/internal/registry"
)

// cmdConsumeCheck implements `relkit check`: verify every consume component
// against the lock without downloading anything. Mirrors relkit_consume.py's
// check subcommand (the product CI surface; the existing publish-side
// `relkit verify` command is unrelated and stays).
func cmdConsumeCheck(args []string) error {
	lockPath := ""
	root := ""
	target := "host"
	var components []string

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
			i++
			components = append(components, mustValue(args, i, "--component"))
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

	failed := false
	for _, name := range names {
		if !registry.HasComponent(name) {
			return fmt.Errorf("unknown component %q", name)
		}
		result := consume.CheckInstalled(root, lock, name, target)
		if result.OK {
			fmt.Printf("relkit consume: verified %s\n", name)
			continue
		}
		failed = true
		fmt.Printf("relkit consume: FAILED %s (%s)\n", name, result.Detail)
	}
	fmt.Printf("relkit consume: check complete release=%s target=%s\n", lock.Release, target)
	if failed {
		return exitCodeError{code: 1}
	}
	return nil
}

// installHostScripts installs the host-scripts tree with the
// already-matches-lock shortcut from the Python consumer.
func installHostScripts(root string, lock *consume.Lock, spec *consume.ArtifactSpec) error {
	pinned := lock.HostScriptsSHA256
	if !isSHA256Hex(pinned) {
		return fmt.Errorf("lock has no valid hostScriptsSha256")
	}
	hostDir := filepath.Join(root, "scripts", "host")
	if actual, err := consume.TreeSHA256(hostDir); err == nil && actual == strings.ToLower(pinned) {
		fmt.Fprintln(os.Stderr, "relkit consume: host-scripts already match lock; skipping download")
		return nil
	}
	artifact, err := consume.DownloadArtifact(root, "host-scripts", spec, &consume.HTTPDownloader{})
	if err != nil {
		return err
	}
	return consume.InstallHostScripts(artifact, hostDir, pinned)
}

func isSHA256Hex(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, ch := range strings.ToLower(value) {
		if !((ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f')) {
			return false
		}
	}
	return true
}
