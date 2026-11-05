// Package build holds the repo-level inputs `relkit build` needs to stamp and
// describe cross-compiled binaries: version stamp, commit identity, file
// hashes, and the updater protocol read from its Go source of truth. The
// product-tree zip packing went with the SDKs and bindings (ADR 0026).
package build

import (
	"fmt"
	"os/exec"
	"strings"
)

// Identity is the commit/dirty pair a stamp derives from.
type Identity struct {
	Commit string
	Dirty  bool
}

// RepoContext carries repo-level inputs a build needs: where the repo is,
// the SSOT version number, and how to shell out to git and go.
type RepoContext struct {
	Root   string
	Number string // SSOT version number, e.g. "0.5.0"
}

// Stamp returns the version stamp: number + short commit (+ "-dirty").
func (c *RepoContext) Stamp() (string, error) {
	ident, err := c.Identity()
	if err != nil {
		return "", err
	}
	stamp := c.Number + "+" + ident.Commit[:12]
	if ident.Dirty {
		stamp += "-dirty"
	}
	return stamp, nil
}

// Identity returns the current commit and dirty state.
func (c *RepoContext) Identity() (*Identity, error) {
	commit, err := c.git("rev-parse", "HEAD")
	if err != nil {
		return nil, fmt.Errorf("git rev-parse HEAD: %w", err)
	}
	status, err := c.git("status", "--porcelain")
	if err != nil {
		return nil, fmt.Errorf("git status: %w", err)
	}
	return &Identity{Commit: strings.TrimSpace(commit), Dirty: strings.TrimSpace(status) != ""}, nil
}

func (c *RepoContext) git(args ...string) (string, error) {
	command := exec.Command("git", args...)
	command.Dir = c.Root
	out, err := command.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}
