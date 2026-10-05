package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/shichao402/relkit/internal/consume"
)

// resolveFollowLatest materializes the follow-latest lock form: when the
// lock says "latest", ask GitHub for the newest release, rebuild the pinned
// lock in memory from that release's manifest + SHA256SUMS, and persist the
// resolution under .relkit/cache so product CI can pass a concrete version
// to the mirror-sync pipeline (which needs a vX.Y.Z to sync).
//
// The on-disk lock is never rewritten by this path: "latest" stays "latest"
// on disk (the user's intent), while the resolved marker records what a
// given run actually consumed. Breaking changes surface as install failures
// with the resolved version in the message — the product then adjusts per
// the new release's requirements (P4 principle: errors are the signal).
func resolveFollowLatest(root string, lock *consume.Lock, lockPath string) (*consume.Lock, error) {
	if !consume.IsLatest(lock.Release) && !(lock.Source != nil && consume.IsLatest(lock.Source.Version)) {
		return lock, nil
	}

	tag, err := consume.ResolveLatestTag()
	if err != nil {
		return nil, fmt.Errorf("lock follows latest but resolution failed: %w", err)
	}

	base := consume.ReleaseBase(tag)
	manifest, err := consume.ReleaseManifestFor(base)
	if err != nil {
		return nil, fmt.Errorf("latest %s: %w", tag, err)
	}
	sumsText, err := consume.HTTPGet(base + "/SHA256SUMS")
	if err != nil {
		return nil, fmt.Errorf("latest %s: %w", tag, err)
	}
	sums := consume.ParseSHA256SUMS(sumsText)

	hostless := lock.Source != nil && lock.Source.Module != "" &&
		(lock.HostScriptsSHA256 == "")
	resolved, err := consume.UpgradeLock(tag, manifest, sums, lock, hostless)
	if err != nil {
		return nil, fmt.Errorf("latest %s: %w", tag, err)
	}
	// UpgradeLock pins Source.Version = tag; a follow-latest lock keeps its
	// module source pinned to "latest" so subsequent runs re-resolve.
	if lock.Source != nil {
		resolved.Source.Version = lock.Source.Version
	}
	// Prepend the mirrors URL for every resolved artifact: the sync
	// pipeline keeps the partition current, and DownloadArtifact walks the
	// URLs in order, so GitHub-unreachable pools fall through to mirrors
	// without special-casing. The sha256 pin stays the single truth either
	// way, so a stale mirrors copy self-rejects.
	consume.PrependMirrorURLs(resolved, tag)

	markerPath := filepath.Join(root, ".relkit", "cache", "resolved-latest.json")
	marker := resolvedLatestMarker{
		Schema:  "relkit.resolved-latest/1",
		Release: tag,
		Commit:  resolved.Commit,
	}
	if err := writeResolvedLatestMarker(markerPath, &marker); err != nil {
		return nil, err
	}
	fmt.Printf("relkit: follow-latest resolved to %s (marker %s)\n", tag, filepath.ToSlash(markerPath))
	return resolved, nil
}

type resolvedLatestMarker struct {
	Schema  string `json:"schema"`
	Release string `json:"release"`
	Commit  string `json:"commit"`
}

func writeResolvedLatestMarker(path string, marker *resolvedLatestMarker) error {
	data, err := json.MarshalIndent(marker, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// cmdConsumeLatest implements `relkit latest`: print the concrete release
// tag a follow-latest lock would consume right now (redirect -> API ->
// mirrors listing chain). This is the delegated resolution entry for
// product CI scripts: the Sync-stage version reader and the mirror-first
// installer both call this instead of reimplementing the chain. With
// --project-root the resolution also refreshes the resolved-latest marker
// (same artifact install/check materialize), so a clean checkout can
// resolve once here and every later consumer reads the marker.
func cmdConsumeLatest(args []string) error {
	root := "."
	asJSON := false

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--project-root":
			i++
			root = mustValue(args, i, "--project-root")
		case arg == "--json":
			asJSON = true
		case strings.HasPrefix(arg, "-"):
			return fmt.Errorf("unknown flag %q", arg)
		default:
			return fmt.Errorf("unexpected argument %q", arg)
		}
	}

	tag, err := consume.ResolveLatestTag()
	if err != nil {
		return fmt.Errorf("latest resolution failed: %w", err)
	}

	commit := ""
	if asJSON {
		base := consume.ReleaseBase(tag)
		manifest, err := consume.ReleaseManifestFor(base)
		if err != nil {
			return fmt.Errorf("latest %s: %w", tag, err)
		}
		commit = manifest.Commit
	}

	// Refresh the marker when a project root is given: the Sync stage runs
	// this before any install (clean checkout has no marker yet), and the
	// mirror-first installer relies on the marker for the concrete
	// partition. Keeping the write here means one resolution per build,
	// shared by every consumer in that workspace.
	if root != "" {
		markerPath := filepath.Join(root, ".relkit", "cache", "resolved-latest.json")
		marker := resolvedLatestMarker{
			Schema:  "relkit.resolved-latest/1",
			Release: tag,
			Commit:  commit,
		}
		if err := writeResolvedLatestMarker(markerPath, &marker); err != nil {
			return err
		}
	}

	if asJSON {
		out := resolvedLatestMarker{
			Schema:  "relkit.resolved-latest/1",
			Release: tag,
			Commit:  commit,
		}
		encoded, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(encoded))
		return nil
	}
	fmt.Println(tag)
	return nil
}
