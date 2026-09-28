package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/shichao402/relkit/internal/consume"
)

// cmdConsumeUpgrade implements `relkit upgrade vX.Y.Z`: rewrite the lock from
// an immutable release (manifest.json + SHA256SUMS) and install. Mirrors
// relkit_host.py upgrade for the product CI surface; the consume/3 lock is
// rebuilt whole, never patched, so any prior schema upgrades in one step.
func cmdConsumeUpgrade(args []string) error {
	root := "."
	release := ""
	finalize := false

	nonFlags := []string{}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--project-root":
			i++
			root = mustValue(args, i, "--project-root")
		case arg == "--finalize":
			finalize = true
		case strings.HasPrefix(arg, "-"):
			return fmt.Errorf("unknown flag %q", arg)
		default:
			nonFlags = append(nonFlags, arg)
		}
	}
	if len(nonFlags) != 1 {
		return fmt.Errorf("usage: relkit upgrade vX.Y.Z [--project-root DIR] [--finalize]")
	}
	release = nonFlags[0]
	_ = finalize

	lockPath := filepath.Join(root, "scripts", "relkit.lock.json")

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

	var previous *consume.Lock
	if prev, err := consume.LoadLock(lockPath); err == nil {
		previous = prev
	}
	lock, err := consume.UpgradeLock(release, manifest, sums, previous)
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
