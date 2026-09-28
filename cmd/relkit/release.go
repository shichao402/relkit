package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/shichao402/relkit/internal/releasegate"
)

// cmdRelease implements `relkit release [--execute]`: the publish-side gate
// for a tree that is already staged. It mirrors hostlib/release.py's
// cmd_release via_ci branch: version reconcile gate (lock drift, staged
// presence), incomplete-steps gate, stale staged cleanup, then agent
// publish (cas-put + POST /publish). Collection (packScript, drops,
// multi-platform aggregation) stays in the product CI's own scripts; this
// command only publishes what they staged.
//
// Execute is CI-only: it requires RELKIT_RELEASE_VIA_CI=1 and
// RELKIT_UPLOAD_TOKEN, mirroring the Python entry.
func cmdRelease(args []string) error {
	execute := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--execute":
			execute = true
		default:
			return fmt.Errorf("unknown flag %q for release", args[i])
		}
	}
	root := "."

	if execute && os.Getenv("RELKIT_RELEASE_VIA_CI") != "1" {
		return fmt.Errorf("release --execute requires RELKIT_RELEASE_VIA_CI=1 (CI holds the agent token; local shells must not publish)")
	}

	// Version: VERSION.json is the SSOT; the staged tree is keyed by it.
	version, err := releasegate.ReleaseVersionFor(root)
	if err != nil {
		return err
	}

	// The release gate: reconcile slice + incomplete steps.
	report, err := releasegate.Reconcile(root, version)
	if err != nil {
		return err
	}
	if len(report.Drift) > 0 {
		return fmt.Errorf("release refused: unresolved drift\n  %s", strings.Join(report.Drift, "\n  "))
	}
	if len(report.Unconfirmed) > 0 {
		return fmt.Errorf("release refused: cannot confirm remote state\n  %s", strings.Join(report.Unconfirmed, "\n  "))
	}

	viaCI := os.Getenv("RELKIT_RELEASE_VIA_CI") == "1"
	state, err := releasegate.LoadState(root)
	if err != nil {
		return err
	}
	if missing := releasegate.IncompleteSteps(state, viaCI, root); len(missing) > 0 {
		return fmt.Errorf("release refused: incomplete steps: %s", strings.Join(missing, ", "))
	}

	agent := releasegate.AgentBaseURL(root)
	if agent == "" {
		return fmt.Errorf("relkit.json agent.url is required for release; run relkit_host.py agent add (ops surface stays Python during the migration)")
	}
	fmt.Printf("relkit release %s execute=%t\n", version, execute)

	if execute {
		removed, err := releasegate.ClearStaleStagedTrees(root, version)
		if err != nil {
			return err
		}
		if len(removed) > 0 {
			fmt.Printf("removed stale staged caches: %s\n", strings.Join(removed, ", "))
		}
	}
	return releasegate.PublishViaAgent(root, version, execute)
}
