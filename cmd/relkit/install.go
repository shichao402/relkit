package main

import (
	"fmt"
	"path/filepath"
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
		case arg == "--resolved-out":
			i++
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
