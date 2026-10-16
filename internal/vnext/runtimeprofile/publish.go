package runtimeprofile

import (
	"bytes"
	"fmt"
	"slices"
	"strings"

	"github.com/shichao402/relkit/internal/backends"
	"github.com/shichao402/relkit/internal/config"
	"github.com/shichao402/relkit/internal/model"
	"github.com/shichao402/relkit/internal/vnext/canonical"
	"github.com/shichao402/relkit/internal/vnext/project"
)

type PublishOptions struct {
	To             []string
	ProfileVersion int64
	DryRun         bool
}

type PublishResult struct {
	Profile   Document
	Reference model.ManifestRef
	Key       string
}

type Printer func(string)

func Publish(
	cfg *config.Config,
	product project.Document,
	options PublishOptions,
	printer Printer,
) (*PublishResult, error) {
	if printer == nil {
		printer = func(string) {}
	}
	if product.RuntimeProfilePublication == nil {
		return nil, fmt.Errorf("project does not publish a Runtime Profile")
	}
	if cfg.Directory == nil || len(cfg.Directory.EntryURLs) == 0 {
		return nil, fmt.Errorf("Runtime Profile publication requires consumer-visible directory.entryUrls")
	}
	signers, err := cfg.LoadSigners()
	if err != nil {
		return nil, err
	}
	value, err := Create(product, cfg.Directory.EntryURLs, options.ProfileVersion, signers)
	if err != nil {
		return nil, err
	}
	raw, err := canonical.Bytes(*value)
	if err != nil {
		return nil, err
	}
	rawDigest := model.Sha256Bytes(raw)
	key := fmt.Sprintf("runtime-profile/%s/%s.json", product.Product, rawDigest)
	result := &PublishResult{
		Profile: *value,
		Key:     key,
		Reference: model.ManifestRef{
			Sha256: rawDigest,
			Size:   int64(len(raw)),
		},
	}
	targetNames := append([]string(nil), options.To...)
	if len(targetNames) == 0 && cfg.Directory != nil {
		targetNames = append(targetNames, cfg.Directory.PublishTo...)
	}
	if len(targetNames) == 0 {
		targetNames = append(targetNames, cfg.PublishTo...)
	}
	if len(targetNames) == 0 {
		return nil, fmt.Errorf("Runtime Profile has no publication backends")
	}
	if options.DryRun {
		printer(fmt.Sprintf("would publish Runtime Profile %s at %s", value.Digest, key))
		return result, nil
	}
	for _, name := range targetNames {
		backend, err := backends.Create(name, cfg, cfg.Root)
		if err != nil {
			return nil, err
		}
		if !backend.Writable() {
			return nil, fmt.Errorf("Runtime Profile backend %q is read-only", name)
		}
		urls, err := backend.PutImmutable(raw, key)
		if err != nil {
			return nil, fmt.Errorf("release.runtime_profile_write_failed: backend %s: %w", name, err)
		}
		stored, err := backend.Get(key)
		if err != nil || !bytes.Equal(stored, raw) {
			return nil, fmt.Errorf("release.runtime_profile_verify_failed: backend %s did not return published bytes", name)
		}
		result.Reference.Urls = append(result.Reference.Urls, urls...)
		if expected := backend.URLFor(key); expected != nil && !slices.Contains(result.Reference.Urls, *expected) {
			result.Reference.Urls = append(result.Reference.Urls, *expected)
		}
		printer(fmt.Sprintf("  %-12s %s", name, key))
	}
	slices.Sort(result.Reference.Urls)
	result.Reference.Urls = slices.Compact(result.Reference.Urls)
	if len(result.Reference.Urls) == 0 {
		return nil, fmt.Errorf("release.runtime_profile_no_url: published Runtime Profile has no consumer-visible URL")
	}
	printer(fmt.Sprintf("published Runtime Profile %s (%s)", value.Digest, strings.Join(result.Reference.Urls, ", ")))
	return result, nil
}

func cloneAnyMap(source map[string]any) map[string]any {
	if len(source) == 0 {
		return nil
	}
	result := make(map[string]any, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}
