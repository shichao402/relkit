package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/shichao402/relkit/internal/consume"
)

// intWindowLike builds the protocol windows for the follow-latest lock. The
// values are placeholders: resolution replaces them wholesale, so they only
// need to be structurally valid.
func intWindowLike(min, max int) consume.IntWindow {
	return consume.IntWindow{Min: min, Max: max}
}

// cmdConsumeUpgrade implements `relkit upgrade vX.Y.Z`: rewrite the lock from
// an immutable release (manifest.json + SHA256SUMS) and install. Mirrors
// relkit_host.py upgrade for the product CI surface; the consume/3 lock is
// rebuilt whole, never patched, so any prior schema upgrades in one step.
func cmdConsumeUpgrade(args []string) error {
	root := "."
	release := ""
	finalize := false
	noHostScripts := false

	nonFlags := []string{}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--project-root":
			i++
			root = mustValue(args, i, "--project-root")
		case arg == "--finalize":
			finalize = true
		case arg == "--no-host-scripts":
			noHostScripts = true
		case strings.HasPrefix(arg, "-"):
			return fmt.Errorf("unknown flag %q", arg)
		default:
			nonFlags = append(nonFlags, arg)
		}
	}
	if len(nonFlags) != 1 {
		return fmt.Errorf("usage: relkit upgrade vX.Y.Z [--project-root DIR] [--finalize] [--no-host-scripts]")
	}
	release = nonFlags[0]
	_ = finalize

	lockPath := filepath.Join(root, "scripts", "relkit.lock.json")

	if strings.ToLower(strings.TrimSpace(release)) == consume.LatestKeyword {
		// Follow-latest form (P4): the lock records intent, not a version.
		// install/check/status resolve at run time; a breaking release
		// surfaces as an install error the product then adapts to.
		previous, _ := consume.LoadLock(lockPath)
		hostless := true
		if _, err := os.Stat(filepath.Join(root, "scripts", "host")); err == nil {
			hostless = false
		}
		if previous != nil && previous.Schema == consume.SchemaV2 {
			hostless = false
		}
		follow := &consume.Lock{
			Schema: consume.SchemaV3,
			Release: consume.LatestKeyword,
			Commit: "",
			Source: &consume.SourceBlock{
				Module:  "github.com/shichao402/relkit",
				Version: consume.LatestKeyword,
			},
			Protocol:   intWindowLike(2, 2),
			UpdaterIPC: intWindowLike(3, 3),
			Artifacts:  map[string]json.RawMessage{},
		}
		if !hostless {
			if previous != nil && previous.HostScriptsSHA256 != "" {
				follow.HostScriptsSHA256 = previous.HostScriptsSHA256
			}
		}
		if err := follow.WriteLock(lockPath); err != nil {
			return err
		}
		fmt.Printf("wrote %s in follow-latest form (resolved at install/check time)\n", filepath.ToSlash(lockPath))
		return cmdConsumeInstall([]string{"--project-root", root})
	}

	base := consume.ReleaseBase(release)
	manifest, err := consume.ReleaseManifestFor(base)
	if err != nil {
		return err
	}
	sumsText, err := consume.HTTPGet(base + "/SHA256SUMS")
	if err != nil {
		return err
	}
	sums := consume.ParseSHA256SUMS(sumsText)

	// Hostless form (ADR 0017 phase-3 target): a product that retired
	// scripts/host gets a lock without hostScriptsSha256 or a host-scripts
	// artifact row. Auto-detected from the missing tree; --no-host-scripts
	// forces it for products keeping the tree around untracked.
	if !noHostScripts {
		if _, err := os.Stat(filepath.Join(root, "scripts", "host")); os.IsNotExist(err) {
			noHostScripts = true
		}
	}

	var previous *consume.Lock
	if prev, err := consume.LoadLock(lockPath); err == nil {
		previous = prev
	}
	lock, err := consume.UpgradeLock(release, manifest, sums, previous, noHostScripts)
	if err != nil {
		return err
	}
	previousSchema := "none"
	if previous != nil {
		previousSchema = previous.Schema
	}
	if err := lock.WriteLock(lockPath); err != nil {
		return err
	}
	if previousSchema != lock.Schema {
		fmt.Printf("rewrote %s from %s to %s\n", filepath.ToSlash(lockPath), previousSchema, lock.Schema)
	}
	fmt.Printf("updated %s to %s\n", filepath.ToSlash(lockPath), release)

	// After the lock lands, run the install so scripts/host and pinned
	// artifacts follow the new release. Finalize semantics (re-entering the
	// freshly installed host scripts) is a Python-process concern; the Go CLI
	// is a single static binary, so install-once suffices.
	return cmdConsumeInstall([]string{"--project-root", root})
}
