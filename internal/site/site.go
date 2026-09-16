// Package site rebuilds the human-facing static site from authoritative
// data-plane documents. Product publishing never reads or writes rendered
// catalog files.
package site

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"go.firoyang.com/relkit/internal/backends"
	"go.firoyang.com/relkit/internal/browse"
	"go.firoyang.com/relkit/internal/config"
	"go.firoyang.com/relkit/internal/makers"
	"go.firoyang.com/relkit/internal/webmeta"
)

type Config struct {
	Makers   *makers.Config
	StateDir string
}

type Product struct {
	ID      string
	Root    string
	Profile string
}

type Printer func(string)

var createBackend = backends.Create

type sink interface {
	Name() string
	Deploy(map[string][]byte) error
}

type backendSink struct{ backend backends.Backend }

func (s backendSink) Name() string { return s.backend.Describe() }
func (s backendSink) Deploy(dump map[string][]byte) error {
	keys := make([]string, 0, len(dump))
	for key := range dump {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if _, err := s.backend.PutPointer(dump[key], key); err != nil {
			return fmt.Errorf("%s: %w", key, err)
		}
	}
	return nil
}

type makersSink struct{ cfg *makers.Config }

func (s makersSink) Name() string { return "makers:" + s.cfg.ProjectID }
func (s makersSink) Deploy(dump map[string][]byte) error {
	return makers.DeployDump(dump, s.cfg)
}

// Rebuild collects every configured product and deploys one complete dump.
func Rebuild(siteCfg Config, products []Product, printer Printer) (bool, error) {
	if printer == nil {
		printer = func(string) {}
	}
	inputs, sinks, err := collect(products, siteCfg.Makers, printer)
	if err != nil {
		return false, err
	}
	if len(inputs) == 0 {
		return false, fmt.Errorf("site rebuild found no published product data")
	}
	if len(sinks) == 0 {
		return false, fmt.Errorf("site rebuild has no destination (configure agent site.makers or a HostsBrowse backend)")
	}
	dump, err := browse.Build(inputs)
	if err != nil {
		return false, err
	}
	sum := hashDump(dump, sinks)
	statePath := filepath.Join(siteCfg.StateDir, "site", "dump.sha256")
	if previous, err := os.ReadFile(statePath); err == nil && strings.TrimSpace(string(previous)) == sum {
		printer("site rebuild: unchanged; deployment skipped")
		return false, nil
	}
	for _, dest := range sinks {
		printer("site rebuild: deploying " + dest.Name())
		if err := dest.Deploy(dump); err != nil {
			return false, fmt.Errorf("%s: %w", dest.Name(), err)
		}
	}
	if err := os.MkdirAll(filepath.Dir(statePath), 0o755); err != nil {
		return false, err
	}
	if err := os.WriteFile(statePath, []byte(sum+"\n"), 0o644); err != nil {
		return false, err
	}
	return true, nil
}

func collect(products []Product, makersCfg *makers.Config, printer Printer) ([]browse.ProductData, []sink, error) {
	sorted := append([]Product(nil), products...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })
	var inputs []browse.ProductData
	var sinks []sink
	seenSink := map[string]bool{}
	if makersCfg != nil && makersCfg.ProjectID != "" {
		sinks = append(sinks, makersSink{cfg: makersCfg})
		seenSink["makers:"+makersCfg.ProjectID] = true
	}
	for _, product := range sorted {
		profile, err := config.LoadPublishProfile(product.Profile)
		if err != nil {
			return nil, nil, fmt.Errorf("%s profile: %w", product.ID, err)
		}
		cfg := &config.Config{
			Root: product.Root, Product: product.ID,
			Backends: profile.Backends, PublishTo: profile.PublishTo,
		}
		var sources []backends.Backend
		for _, name := range profile.PublishTo {
			backend, err := createBackend(name, cfg, product.Root)
			if err != nil {
				return nil, nil, fmt.Errorf("%s backend %s: %w", product.ID, name, err)
			}
			sources = append(sources, backend)
			if backend.HostsBrowse() {
				key := backend.Type() + ":" + backend.Describe()
				if !seenSink[key] {
					seenSink[key] = true
					sinks = append(sinks, backendSink{backend: backend})
				}
			}
		}
		siteRaw, err := firstGet(sources, webmeta.SiteKey(product.ID))
		if err != nil {
			return nil, nil, fmt.Errorf("%s site: %w", product.ID, err)
		}
		if len(siteRaw) == 0 {
			printer("site rebuild: skip " + product.ID + " (no site document)")
			continue
		}
		siteDoc, err := webmeta.UnmarshalSite(siteRaw)
		if err != nil {
			return nil, nil, fmt.Errorf("%s site: %w", product.ID, err)
		}
		if siteDoc.Product != product.ID {
			return nil, nil, fmt.Errorf("%s site document names product %q", product.ID, siteDoc.Product)
		}
		channels := siteDoc.Channels
		if len(channels) == 0 {
			channels = []string{"stable"} // compatibility with relkit.site/1 writers before channels
		}
		input := browse.ProductData{Site: siteDoc}
		for _, channel := range channels {
			raw, err := firstGet(sources, webmeta.LatestKey(product.ID, channel))
			if err != nil {
				return nil, nil, fmt.Errorf("%s latest/%s: %w", product.ID, channel, err)
			}
			if len(raw) == 0 {
				printer(fmt.Sprintf("site rebuild: %s has no latest/%s", product.ID, channel))
				continue
			}
			doc, err := webmeta.UnmarshalLatest(raw)
			if err != nil {
				return nil, nil, fmt.Errorf("%s latest/%s: %w", product.ID, channel, err)
			}
			if doc.Product != product.ID || doc.Channel != channel {
				return nil, nil, fmt.Errorf("%s latest/%s document identity mismatch", product.ID, channel)
			}
			input.Latests = append(input.Latests, *doc)
		}
		if len(input.Latests) > 0 {
			inputs = append(inputs, input)
		}
	}
	return inputs, sinks, nil
}

func firstGet(sources []backends.Backend, key string) ([]byte, error) {
	var failures []string
	for _, source := range sources {
		data, err := source.Get(key)
		if err == nil && len(data) > 0 {
			return data, nil
		}
		if err != nil {
			failures = append(failures, source.Name()+": "+err.Error())
		}
	}
	if len(failures) == len(sources) && len(failures) > 0 {
		return nil, fmt.Errorf("%s", strings.Join(failures, "; "))
	}
	return nil, nil
}

func hashDump(dump map[string][]byte, sinks []sink) string {
	keys := make([]string, 0, len(dump))
	for key := range dump {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	hash := sha256.New()
	names := make([]string, 0, len(sinks))
	for _, dest := range sinks {
		names = append(names, dest.Name())
	}
	sort.Strings(names)
	for _, name := range names {
		hash.Write([]byte("sink:" + name))
		hash.Write([]byte{0})
	}
	for _, key := range keys {
		hash.Write([]byte(key))
		hash.Write([]byte{0})
		hash.Write(dump[key])
		hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil))
}
