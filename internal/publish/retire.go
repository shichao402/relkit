package publish

import (
	"fmt"
	"slices"
	"strings"

	"github.com/shichao402/relkit/internal/backends"
	"github.com/shichao402/relkit/internal/config"
	"github.com/shichao402/relkit/internal/model"
	"github.com/shichao402/relkit/internal/webmeta"
)

// Retire is the whole-product retirement: the same operation the panel's
// "delete product" button performs (requirements 8.3, 13.5). It removes every
// published version from every declared channel, then the documents that make
// the product visible: the signed index pointer, latest/ and channel/ pointers,
// site/, the per-version release details, and the signed UpdateDirectory.
//
// Unpublish refuses to remove a channel's last version because a client holding
// that index would lose its update path. Retirement is the deliberate exception:
// the product is going away, so the index itself must be deleted rather than
// rewritten without its last node.
//
// CAS blobs, artifacts and manifests are not deleted here. They are immutable
// and unreferenced once every index is gone; relkit-store's own GC reclaims
// them, exactly as it does after a normal unpublish.
func Retire(cfg *config.Config, to []string, dryRun bool, printer Printer) (*RetireResult, error) {
	if printer == nil {
		printer = func(string) {}
	}
	if strings.TrimSpace(cfg.Product) == "" {
		return nil, Error{Message: "product is required"}
	}

	targetNames := append([]string(nil), to...)
	if len(targetNames) == 0 {
		targetNames = append([]string(nil), cfg.PublishTo...)
	}
	if len(targetNames) == 0 {
		return nil, Error{Message: "no target backends; set publishTo or pass --to"}
	}

	opened, err := openBackends(cfg, targetNames)
	if err != nil {
		return nil, err
	}
	for _, backend := range opened {
		if !backend.Writable() {
			return nil, Error{Message: fmt.Sprintf("backend %q is read-only and cannot be retired from", backend.Name())}
		}
		if _, ok := backend.(backends.Deleter); !ok {
			return nil, Error{Message: fmt.Sprintf("backend %q does not support delete, so the product cannot be retired", backend.Name())}
		}
	}

	signers, err := cfg.Signers()
	if err != nil {
		return nil, err
	}
	trusted := trustedKeys(cfg, signers)

	result := &RetireResult{Product: cfg.Product, DryRun: dryRun}
	channels := append([]string(nil), cfg.Channels...)
	if len(channels) == 0 {
		channels = []string{cfg.DefaultChannel}
	}
	for _, channel := range channels {
		printer(fmt.Sprintf("retiring %s from channel %s", cfg.Product, channel))
		existing, _, _, err := readExistingIndexes(opened, cfg, channel, trusted, printer)
		if err != nil {
			return nil, err
		}
		if len(existing.Versions) == 0 {
			printer("  channel has no published versions")
			continue
		}
		versions := make([]string, 0, len(existing.Versions))
		for _, node := range existing.Versions {
			if node == nil {
				continue
			}
			versions = append(versions, node.Version)
		}
		result.Channels = append(result.Channels, channel)
		for _, version := range versions {
			result.Versions = append(result.Versions, channel+"/"+version)
		}
		if dryRun {
			printer(fmt.Sprintf("  would delete index, latest, channel, and %d release document(s)", len(versions)))
			continue
		}
		// Release details first: they are immutable documents no future publish
		// rewrites, so nothing else will clean them up.
		for _, version := range versions {
			result.deleteOnAll(opened, webmeta.ReleaseKey(cfg.Product, channel, version), "release", printer)
		}
		// Then the pointers that make the channel readable. Deleting the index
		// pointer is the commit point: once it is gone, no client can resolve
		// this product on this channel.
		result.deleteOnAll(opened, model.IndexKey(cfg.Product, channel), "index", printer)
		result.deleteOnAll(opened, webmeta.LatestKey(cfg.Product, channel), "latest", printer)
		result.deleteOnAll(opened, webmeta.ChannelKey(cfg.Product, channel), "channel", printer)
	}

	if dryRun {
		printer("  would delete site, directory, and every remaining product document")
		result.sortKeys()
		return result, nil
	}

	// Product-level documents. site/ is what the human page reads, directory/ is
	// the signed bootstrap list; both must go, or the product keeps advertising
	// itself with no versions behind it.
	result.deleteOnAll(opened, webmeta.SiteKey(cfg.Product), "site", printer)
	result.deleteOnAll(opened, model.DirectoryKey(cfg.Product), "directory", printer)
	result.sortKeys()

	printer("")
	printer(fmt.Sprintf("retired %s: %d channel(s), %d version(s), %d document(s)",
		cfg.Product, len(result.Channels), len(result.Versions), len(result.Deleted)))
	if len(result.Failures) > 0 {
		printer("WARNING: some documents could not be deleted:")
		for _, failure := range result.Failures {
			printer("  " + failure)
		}
		printer("re-run retirement to retry; deleting is idempotent")
	}
	printer("store GC reclaims the orphaned manifests, artifacts and CAS blobs on its next round")
	return result, nil
}

// RetireResult is what the panel and the CLI report back.
type RetireResult struct {
	Product  string   `json:"product"`
	DryRun   bool     `json:"dryRun"`
	Channels []string `json:"channels,omitempty"`
	// Versions is "channel/version" for every retired node.
	Versions []string `json:"versions,omitempty"`
	// Deleted is every document key actually removed, sorted.
	Deleted []string `json:"deleted,omitempty"`
	// Failures lists per-backend delete failures. The operation is idempotent,
	// so a rerun retries exactly what is left.
	Failures []string `json:"failures,omitempty"`
}

func (r *RetireResult) deleteOnAll(targets []backends.Backend, key, label string, printer Printer) {
	if r == nil {
		return
	}
	for _, backend := range targets {
		deleter, ok := backend.(backends.Deleter)
		if !ok {
			r.Failures = append(r.Failures, fmt.Sprintf("%s %s: backend does not support delete", backend.Name(), label))
			continue
		}
		if err := deleter.Delete(key); err != nil {
			r.Failures = append(r.Failures, fmt.Sprintf("%s %s: %v", backend.Name(), label, err))
			printer(fmt.Sprintf("  %-12s FAILED %s: %v", backend.Name(), key, err))
			continue
		}
		printer(fmt.Sprintf("  %-12s deleted %s", backend.Name(), key))
	}
	if !slices.Contains(r.Deleted, key) {
		r.Deleted = append(r.Deleted, key)
	}
}

func (r *RetireResult) sortKeys() {
	slices.Sort(r.Deleted)
	slices.Sort(r.Versions)
}
