package publish

import (
	"fmt"
	"strings"

	rupv2 "github.com/shichao402/relkit/api/rup/v2"
	"github.com/shichao402/relkit/internal/backends"
	"github.com/shichao402/relkit/internal/config"
	"github.com/shichao402/relkit/internal/envelope"
	"github.com/shichao402/relkit/internal/model"
	"github.com/shichao402/relkit/internal/webmeta"
)

// Unpublish removes one version from a channel's signed index and re-projects
// the human-facing web documents. It never touches CAS blobs (the store's own
// GC reclaims manifest/artifact orphans once no index references them) and
// never reuses the deleted code: publish's monotonic gate only counts what the
// index still lists, so the removed version must not be re-issued at the same
// code (ADR: dirty versions are deleted outright, gaps are skipped, only
// higher codes are ever issued).
//
// The flow mirrors publish.Run's commit discipline: read and verify every
// backend's index, remove the node, validate reachability, seal with the
// product's signing key, commit the pointer, then rewrite the derived web
// documents. Failures before the pointer commit leave the index untouched;
// failures after it leave the index committed and report the divergence the
// same way publish does.
func Unpublish(cfg *config.Config, version string, to []string, dryRun bool, printer Printer) (*model.IndexDocument, error) {
	if printer == nil {
		printer = func(string) {}
	}
	if strings.TrimSpace(version) == "" {
		return nil, Error{Message: "version is required"}
	}

	targetNames := append([]string(nil), to...)
	if len(targetNames) == 0 {
		targetNames = append([]string(nil), cfg.PublishTo...)
	}
	if len(targetNames) == 0 {
		return nil, Error{Message: "no target backends; set publishTo or pass --to"}
	}

	openedBackends, err := openBackends(cfg, targetNames)
	if err != nil {
		return nil, err
	}
	return unpublishOnBackends(cfg, version, openedBackends, dryRun, printer)
}

func unpublishOnBackends(cfg *config.Config, version string, openedBackends []backends.Backend, dryRun bool, printer Printer) (*model.IndexDocument, error) {
	for _, backend := range openedBackends {
		if !backend.Writable() {
			return nil, Error{Message: fmt.Sprintf("backend %q is read-only and cannot be unpublished from", backend.Name())}
		}
	}

	channel, err := cfg.ChannelOrDefault("")
	if err != nil {
		return nil, err
	}

	// The signing key is required even for a dry run: OpenEnvelope verifies the
	// current index, and the trust set includes the signer's public half (same
	// discipline as publish.Run, which needs signers for the same read).
	signers, err := cfg.LoadSigners()
	if err != nil {
		return nil, err
	}
	trusted := trustedKeys(cfg, signers)

	printer(fmt.Sprintf("unpublishing %s from channel %s", version, channel))
	printer("reading current index...")
	existing, baseSequence, _, err := readExistingIndexes(openedBackends, cfg, channel, trusted, printer)
	if err != nil {
		return nil, err
	}
	if len(existing.Versions) == 0 {
		return nil, Error{Message: fmt.Sprintf("channel %s has no published versions to remove", channel)}
	}

	var removed *model.VersionNode
	kept := make([]*model.VersionNode, 0, len(existing.Versions))
	for _, node := range existing.Versions {
		if node == nil {
			continue
		}
		if node.Version == version {
			if removed != nil {
				return nil, Error{Message: fmt.Sprintf("index lists %q more than once; the index is corrupt, run 'relkit verify'", version)}
			}
			removed = node
			continue
		}
		kept = append(kept, node)
	}
	if removed == nil {
		return nil, Error{Message: fmt.Sprintf("version %q is not in the %s index (%s)", version, channel, formatVersionList(existing.Versions))}
	}
	if len(kept) == 0 {
		return nil, Error{Message: fmt.Sprintf("refusing to remove %q: it is the only version in the %s index; publish a replacement first", version, channel)}
	}

	if dryRun {
		printer("")
		printer("dry run: validation passed, nothing removed")
		printer(fmt.Sprintf("would write sequence %d without %s (code %d), leaving %d version(s)",
			baseSequence+1, version, removed.Code, len(kept)))
		return nil, nil
	}

	rebuilt, err := model.NewIndex(cfg.Product, channel, baseSequence+1, kept, model.MinSupportedPtr(existing), "")
	if err != nil {
		return nil, err
	}
	warnings, err := validateIndex(rebuilt, "the resulting index")
	if err != nil {
		return nil, err
	}
	for _, warning := range warnings {
		printer("  warning: " + warning)
	}

	env, err := envelope.Seal(rebuilt, signers)
	if err != nil {
		return nil, err
	}
	envelopeBytes, err := rupv2.MarshalEnvelope(env)
	if err != nil {
		return nil, err
	}
	if err := checkClientsCanVerify(cfg, env, channel); err != nil {
		return nil, err
	}

	printer("writing pointer (commit point)...")
	indexKey := model.IndexKey(cfg.Product, channel)
	var committed []backends.Backend
	var failures []string
	for _, backend := range openedBackends {
		if _, err := backend.PutPointer(envelopeBytes, indexKey); err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", backend.Name(), err))
			printer(fmt.Sprintf("  %-12s FAILED: %v", backend.Name(), err))
			continue
		}
		committed = append(committed, backend)
		printer(fmt.Sprintf("  %-12s %s", backend.Name(), indexKey))
	}
	if len(committed) == 0 {
		return nil, Error{Message: fmt.Sprintf("pointer write failed on every backend (%s); the index still lists %q, nothing was removed", strings.Join(failures, ", "), version)}
	}

	// The per-version release detail is an immutable document keyed by version.
	// With the node gone no future publish rewrites it, and the relkit-store GC
	// never scans the release/ prefix, so unpublish deletes it itself to keep
	// the web history consistent with the signed index.
	failures = append(failures, deleteReleaseDoc(cfg, channel, version, committed, printer)...)

	webFailures, err := rewriteWebPointersAfterRemoval(cfg, rebuilt, channel, committed, printer)
	failures = append(failures, webFailures...)
	if err != nil {
		return nil, err
	}
	if len(failures) > 0 {
		printer("")
		printer("WARNING: the index was committed without " + version + " but some web documents failed:")
		for _, failure := range failures {
			printer("  " + failure)
		}
		printer("re-run to retry; the committed index stays authoritative")
		return rebuilt, nil
	}

	printer("")
	printer(fmt.Sprintf("unpublished %s (code %d) from channel %s, sequence %d; %d version(s) remain",
		version, removed.Code, channel, rebuilt.Sequence, len(rebuilt.Versions)))
	printer("store GC reclaims the orphaned manifest and artifacts on its next round")
	return rebuilt, nil
}

// rewriteWebPointersAfterRemoval re-projects site/, latest/ and channel/ from
// the rebuilt index so the human-facing history stops listing the removed
// version. latest/ is projected from the new head node's manifest (an
// immutable document, so the read cannot race a concurrent publish).
func rewriteWebPointersAfterRemoval(
	cfg *config.Config,
	rebuilt *model.IndexDocument,
	channel string,
	targets []backends.Backend,
	printer Printer,
) ([]string, error) {
	var failures []string
	newHead := highestCodeNode(rebuilt)
	if newHead == nil {
		return nil, Error{Message: "the rebuilt index has no versions"}
	}

	if hasSiteCopy(cfg) {
		data, err := webmeta.MarshalSite(webmeta.Site{
			Product:     cfg.Product,
			Title:       cfg.Site.Title,
			Description: cfg.Site.Description,
			Homepage:    cfg.Site.Homepage,
			Channels:    append([]string(nil), cfg.Channels...),
			UpdatedAt:   model.UTCNow(),
		})
		if err != nil {
			return nil, err
		}
		failures = append(failures, putWebDoc(targets, webmeta.SiteKey(cfg.Product), data, true, printer, "site")...)
	}

	latestDoc, err := latestFromNode(cfg, channel, newHead, targets)
	if err != nil {
		failures = append(failures, "latest: "+err.Error())
	} else {
		latestData, err := webmeta.MarshalLatest(*latestDoc)
		if err != nil {
			failures = append(failures, "latest: "+err.Error())
		} else {
			failures = append(failures, putWebDoc(targets, webmeta.LatestKey(cfg.Product, channel), latestData, true, printer, "latest")...)
		}
	}

	channelDoc, err := channelFromIndex(rebuilt, model.UTCNow())
	if err != nil {
		failures = append(failures, "channel: "+err.Error())
	} else {
		channelData, err := webmeta.MarshalChannel(channelDoc)
		if err != nil {
			failures = append(failures, "channel: "+err.Error())
		} else {
			failures = append(failures, putWebDoc(targets, webmeta.ChannelKey(cfg.Product, channel), channelData, true, printer, "channel")...)
		}
	}

	return failures, nil
}

// latestFromNode builds the latest/ document for the head node by reading its
// manifest from the first backend that has it. A missing manifest is reported
// loudly but does not abort: the committed index stays authoritative either way.
func latestFromNode(cfg *config.Config, channel string, node *model.VersionNode, targets []backends.Backend) (*webmeta.Latest, error) {
	manifestKey := model.ManifestKey(cfg.Product, node.Version)
	var manifest *rupv2.Manifest
	for _, backend := range targets {
		raw, err := backend.Get(manifestKey)
		if err != nil || raw == nil {
			continue
		}
		doc, err := rupv2.UnmarshalManifest(raw)
		if err != nil {
			continue
		}
		manifest = doc
		break
	}
	if manifest == nil {
		return nil, Error{Message: fmt.Sprintf("could not read manifest %s for the new head %s; latest/ keeps pointing at the previous publish until the next one rewrites it", manifestKey, node.Version)}
	}
	return &webmeta.Latest{
		Product:     cfg.Product,
		Channel:     channel,
		Version:     node.Version,
		Code:        node.Code,
		PublishedAt: node.ReleasedAt,
		Artifacts:   webmeta.ArtifactsFromManifest(manifest),
	}, nil
}

// deleteReleaseDoc removes the immutable release detail document for the
// unpublished version on every backend that supports deletion.
func deleteReleaseDoc(cfg *config.Config, channel, version string, targets []backends.Backend, printer Printer) []string {
	var failures []string
	key := webmeta.ReleaseKey(cfg.Product, channel, version)
	for _, backend := range targets {
		deleter, ok := backend.(backends.Deleter)
		if !ok {
			failures = append(failures, fmt.Sprintf("%s release: backend does not support delete", backend.Name()))
			continue
		}
		if err := deleter.Delete(key); err != nil {
			failures = append(failures, fmt.Sprintf("%s release: %v", backend.Name(), err))
			continue
		}
		printer(fmt.Sprintf("  %-12s deleted %s", backend.Name(), key))
	}
	return failures
}

func putWebDoc(targets []backends.Backend, key string, data []byte, pointer bool, printer Printer, label string) []string {
	var failures []string
	for _, backend := range targets {
		var err error
		if pointer {
			_, err = backend.PutPointer(data, key)
		} else {
			_, err = backend.PutImmutable(data, key)
		}
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s %s: %v", backend.Name(), label, err))
			continue
		}
		printer(fmt.Sprintf("  %-12s %s", backend.Name(), key))
	}
	return failures
}

func highestCodeNode(index *model.IndexDocument) *model.VersionNode {
	var best *model.VersionNode
	for _, node := range index.Versions {
		if node == nil {
			continue
		}
		if best == nil || node.Code > best.Code || (node.Code == best.Code && node.Version > best.Version) {
			best = node
		}
	}
	return best
}
