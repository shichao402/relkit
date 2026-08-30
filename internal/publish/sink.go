package publish

import (
	"fmt"

	"cnb.cool/shichao402/relkit/internal/backends"
	"cnb.cool/shichao402/relkit/internal/browse"
	"cnb.cool/shichao402/relkit/internal/config"
	"cnb.cool/shichao402/relkit/internal/makers"
)

// BrowseSink deploys the human-facing dump. Protocol clients never read it.
// Implementations are selected from config and Backend.HostsBrowse, never
// from Backend.Type() strings in publish.Run.
type BrowseSink interface {
	Name() string
	Deploy(dump map[string][]byte) error
	LoadCatalog() *browse.Catalog
}

type dataPlaneSink struct {
	backend backends.Backend
}

func (s dataPlaneSink) Name() string {
	return s.backend.Name()
}

func (s dataPlaneSink) Deploy(dump map[string][]byte) error {
	for key, data := range dump {
		if _, err := s.backend.PutPointer(data, key); err != nil {
			return fmt.Errorf("%s: %w", key, err)
		}
	}
	return nil
}

func (s dataPlaneSink) LoadCatalog() *browse.Catalog {
	raw, err := s.backend.Get(browse.CatalogKey())
	if err != nil || len(raw) == 0 {
		return nil
	}
	doc, err := browse.UnmarshalCatalog(raw)
	if err != nil {
		return nil
	}
	return doc
}

type makersSink struct {
	root   string
	makers *config.MakersConfig
}

func (s makersSink) Name() string {
	return "makers:" + s.makers.ProjectID
}

func (s makersSink) Deploy(map[string][]byte) error {
	return makers.DeployDump(s.root, s.makers)
}

func (s makersSink) LoadCatalog() *browse.Catalog {
	return nil
}

// OpenBrowseSinks builds human-index destinations for this publish run.
// HostsBrowse backends get a data-plane sink. site.makers is added only when
// some target cannot host browse/ (so --to local skips Makers).
func OpenBrowseSinks(cfg *config.Config, targets []backends.Backend) []BrowseSink {
	var sinks []BrowseSink
	if cfg == nil {
		return sinks
	}
	for _, backend := range targets {
		if backend.HostsBrowse() {
			sinks = append(sinks, dataPlaneSink{backend: backend})
		}
	}
	if cfg.Site.Makers != nil && cfg.Site.Makers.ProjectID != "" && needsExternalSite(targets) {
		sinks = append(sinks, makersSink{root: cfg.Root, makers: cfg.Site.Makers})
	}
	return sinks
}

func needsExternalSite(targets []backends.Backend) bool {
	for _, backend := range targets {
		if !backend.HostsBrowse() {
			return true
		}
	}
	return false
}

func warnMissingSiteSink(cfg *config.Config, targets []backends.Backend, printer Printer) {
	if !needsExternalSite(targets) {
		return
	}
	for _, sink := range OpenBrowseSinks(cfg, targets) {
		if _, ok := sink.(makersSink); ok {
			return
		}
	}
	printer("  warning: this publish writes a protocol-only backend with no BrowseSink (site.makers or other site hosting); human index will not be deployed")
}
