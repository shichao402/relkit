package verify

import (
	"bytes"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/shichao402/relkit/internal/backends"
	"github.com/shichao402/relkit/internal/chain"
	"github.com/shichao402/relkit/internal/config"
	"github.com/shichao402/relkit/internal/envelope"
	"github.com/shichao402/relkit/internal/httpx"
	"github.com/shichao402/relkit/internal/jsonio"
	"github.com/shichao402/relkit/internal/model"
)

type Error struct {
	Message string
}

func (e Error) Error() string {
	return e.Message
}

type Findings struct {
	Errors   []string
	Warnings []string
	Notes    []string
}

func (f *Findings) Error(message string) {
	f.Errors = append(f.Errors, message)
}

func (f *Findings) Warn(message string) {
	f.Warnings = append(f.Warnings, message)
}

func (f *Findings) Note(message string) {
	f.Notes = append(f.Notes, message)
}

func (f *Findings) OK() bool {
	return len(f.Errors) == 0
}

type Printer func(string)

func Run(cfg *config.Config, channel string, to []string, deep bool, printer Printer) (*Findings, error) {
	if printer == nil {
		printer = func(string) {}
	}

	resolvedChannel, err := cfg.ChannelOrDefault(channel)
	if err != nil {
		return nil, err
	}
	trusted, err := cfg.TrustedPublicKeys()
	if err != nil {
		return nil, err
	}

	targetNames := append([]string(nil), to...)
	if len(targetNames) == 0 {
		targetNames = append([]string(nil), cfg.PublishTo...)
	}
	findings := &Findings{}
	indexKey := model.IndexKey(cfg.Product, resolvedChannel)
	rawByBackend := map[string][]byte{}
	parsedByBackend := map[string]*model.IndexDocument{}

	for _, name := range targetNames {
		backend, err := backends.Create(name, cfg, cfg.Root)
		if err != nil {
			return nil, err
		}
		printer(backend.Describe())

		raw, err := backend.Get(indexKey)
		if err != nil {
			findings.Error(fmt.Sprintf("could not read the index from %s: %v", name, err))
			continue
		}
		rawByBackend[name] = raw
		if raw == nil {
			findings.Error(fmt.Sprintf("no index published on %s (%s)", name, indexKey))
			continue
		}

		var env model.Envelope
		if err := jsonio.LoadBytes(raw, &env); err != nil {
			findings.Error(fmt.Sprintf("index on %s is not valid JSON: %v", name, err))
			continue
		}

		index, err := envelope.OpenEnvelope(&env, trusted)
		if err != nil {
			findings.Error(fmt.Sprintf("signature check failed on %s: %v", name, err))
			continue
		}
		printer(fmt.Sprintf("    signature ok, sequence %d", index.Sequence))
		parsedByBackend[name] = index

		checkShape(index, findings)

		errors, warnings := chain.ValidateReachability(index)
		for _, code := range errors {
			stranded := chain.UnreachableStartCodes(index)
			detail := ""
			if len(stranded) > 0 {
				var parts []string
				for _, strandedCode := range stranded {
					parts = append(parts, strconv.Itoa(strandedCode))
				}
				detail = " (stranded start codes: " + strings.Join(parts, ", ") + ")"
			}
			findings.Error(fmt.Sprintf("reachability on %s: %s%s", name, code, detail))
		}
		for _, code := range warnings {
			findings.Warn(fmt.Sprintf("reachability on %s: %s", name, code))
		}
		if len(errors) == 0 {
			printer(fmt.Sprintf("    reachability ok (%d version(s))", len(index.Versions)))
		}

		checkManifests(backend, index, cfg.Product, findings, printer)

		if deep {
			if !backend.URLsAreLive() {
				findings.Note(fmt.Sprintf("--deep on %s checked stored bytes only; this backend's URLs are not expected to resolve yet", name))
			}
			checkArtifactsReachable(backend, index, cfg.Product, findings, printer)
		}
	}

	distinct := make([][]byte, 0, len(rawByBackend))
	for _, raw := range rawByBackend {
		if raw != nil {
			distinct = append(distinct, raw)
		}
	}
	if len(distinct) > 1 {
		allEqual := true
		for i := 1; i < len(distinct); i++ {
			if !bytes.Equal(distinct[0], distinct[i]) {
				allEqual = false
				break
			}
		}
		if !allEqual {
			var sequences []string
			for name, index := range parsedByBackend {
				sequences = append(sequences, fmt.Sprintf("%s=%d", name, index.Sequence))
			}
			slices.Sort(sequences)
			newest := 0
			for _, index := range parsedByBackend {
				if index.Sequence > newest {
					newest = index.Sequence
				}
			}
			var lagging []string
			for name, index := range parsedByBackend {
				if index.Sequence < newest {
					lagging = append(lagging, name)
				}
			}
			slices.Sort(lagging)
			laggingText := strings.Join(lagging, ", ")
			if laggingText == "" {
				laggingText = "none (same sequence, different bytes)"
			}
			findings.Error(fmt.Sprintf("backends are not byte-identical (%s); lagging: %s", strings.Join(sequences, ", "), laggingText))
		}
	}

	printer("")
	for _, message := range findings.Notes {
		printer("note:    " + message)
	}
	for _, message := range findings.Warnings {
		printer("warning: " + message)
	}
	for _, message := range findings.Errors {
		printer("error:   " + message)
	}
	if findings.OK() {
		printer("verify passed")
	} else {
		printer(fmt.Sprintf("verify FAILED with %d error(s)", len(findings.Errors)))
	}
	return findings, nil
}

func checkShape(index *model.IndexDocument, findings *Findings) {
	if index.Schema != model.SchemaIndex {
		findings.Error(fmt.Sprintf("index schema is %q, expected %q", index.Schema, model.SchemaIndex))
	}
	if index.Product == "" {
		findings.Error("index is missing required field \"product\"")
	}
	if index.Channel == "" {
		findings.Error("index is missing required field \"channel\"")
	}
	if index.Sequence == 0 {
		findings.Error("index is missing required field \"sequence\"")
	}
	if index.GeneratedAt == "" {
		findings.Error("index is missing required field \"generatedAt\"")
	}
	if len(index.Versions) == 0 {
		findings.Error("index.versions must be a non-empty array")
		return
	}

	for _, node := range index.Versions {
		if node.Version == "" {
			findings.Error("version node is missing \"version\"")
		}
		if node.Manifest.SHA256 != "" && (len(node.Manifest.SHA256) != 64 || node.Manifest.SHA256 != strings.ToLower(node.Manifest.SHA256)) {
			findings.Error(fmt.Sprintf("version %q has a malformed manifest sha256", node.Version))
		}
	}
}

func checkDeclaredURL(backend backends.Backend, key string, declaredURLs []string, what string, findings *Findings) *string {
	expected := backend.URLFor(key)
	if expected == nil {
		return nil
	}
	if !containsURL(declaredURLs, *expected) {
		findings.Error(fmt.Sprintf("%s does not list this backend's URL for %s\n           expects: %s\n           listed:  %s", what, key, *expected, strings.Join(declaredURLs, ", ")))
	}
	return expected
}

func checkManifests(backend backends.Backend, index *model.IndexDocument, product string, findings *Findings, printer Printer) {
	nodes := append([]model.VersionNode(nil), index.Versions...)
	slices.SortFunc(nodes, func(a, b model.VersionNode) int { return a.Code - b.Code })
	for _, node := range nodes {
		key := model.ManifestKey(product, node.Version)
		checkDeclaredURL(backend, key, node.Manifest.URLs, "index entry for "+node.Version, findings)

		raw, err := backend.Get(key)
		if err != nil {
			findings.Error(fmt.Sprintf("manifest missing on %s: %s", backend.Name(), key))
			continue
		}
		if raw == nil {
			findings.Error(fmt.Sprintf("manifest missing on %s: %s", backend.Name(), key))
			continue
		}

		actualDigest := model.Sha256Bytes(raw)
		if actualDigest != node.Manifest.SHA256 || int64(len(raw)) != node.Manifest.Size {
			findings.Error(fmt.Sprintf("manifest-digest-mismatch for %s on %s: index records %s (%d bytes), stored manifest is %s (%d bytes)", node.Version, backend.Name(), prefix(node.Manifest.SHA256), node.Manifest.Size, prefix(actualDigest), len(raw)))
			continue
		}

		var manifest model.ManifestDocument
		if err := jsonio.LoadBytes(raw, &manifest); err != nil {
			findings.Error(fmt.Sprintf("manifest %s is not valid JSON: %v", node.Version, err))
			continue
		}
		if manifest.Product != product {
			findings.Error(fmt.Sprintf("manifest %s has product %q, expected %q", node.Version, manifest.Product, product))
		}
		if manifest.Version != node.Version {
			findings.Error(fmt.Sprintf("manifest %s declares version %q", node.Version, manifest.Version))
		}
		if manifest.Code != node.Code {
			findings.Error(fmt.Sprintf("manifest %s declares code %d, index says %d", node.Version, manifest.Code, node.Code))
		}
		printer(fmt.Sprintf("    %-12s manifest ok (%d artifact(s))", node.Version, len(manifest.Artifacts)))
	}
}

func checkArtifactsReachable(backend backends.Backend, index *model.IndexDocument, product string, findings *Findings, printer Printer) {
	nodes := append([]model.VersionNode(nil), index.Versions...)
	slices.SortFunc(nodes, func(a, b model.VersionNode) int { return a.Code - b.Code })
	for _, node := range nodes {
		raw, err := backend.Get(model.ManifestKey(product, node.Version))
		if err != nil || raw == nil {
			continue
		}

		var manifest model.ManifestDocument
		if err := jsonio.LoadBytes(raw, &manifest); err != nil {
			continue
		}
		for _, artifact := range manifest.Artifacts {
			key := model.ArtifactKey(product, node.Version, artifact.Filename)
			url := checkDeclaredURL(backend, key, artifact.URLs, fmt.Sprintf("manifest %s, artifact %s", node.Version, artifact.ID), findings)
			if !backend.URLsAreLive() {
				stored, err := backend.Get(key)
				if err != nil || stored == nil {
					findings.Error(fmt.Sprintf("artifact missing on %s: %s", backend.Name(), key))
				} else if int64(len(stored)) != artifact.Size {
					findings.Error(fmt.Sprintf("artifact %s on %s is %d bytes, manifest says %d", key, backend.Name(), len(stored), artifact.Size))
				} else {
					printer(fmt.Sprintf("    %-12s %s ok (stored bytes)", node.Version, artifact.ID))
				}
				continue
			}
			if url == nil {
				continue
			}
			ok, size, note := backend.Probe(*url)
			if !ok {
				findings.Error(fmt.Sprintf("artifact not downloadable: %s\n           %s", *url, note))
			} else if !httpx.SizeMatches(artifact.Size, size) {
				findings.Error(fmt.Sprintf("artifact %s is %d bytes at the URL, manifest says %d\n           %s", artifact.ID, valueOrZero(size), artifact.Size, *url))
			} else {
				sizeText := ""
				if size != nil {
					sizeText = fmt.Sprintf(", %d bytes", *size)
				}
				printer(fmt.Sprintf("    %-12s %s ok (%s%s)", node.Version, artifact.ID, note, sizeText))
			}
		}
	}
}

func containsURL(urls []string, want string) bool {
	for _, url := range urls {
		if url == want {
			return true
		}
	}
	return false
}

func prefix(value string) string {
	if len(value) > 12 {
		return value[:12]
	}
	return value
}

func valueOrZero(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}
