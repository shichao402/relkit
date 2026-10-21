package ops

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/shichao402/relkit/internal/publishproto"
	"github.com/shichao402/relkit/internal/vnext/artifactset"
	"github.com/shichao402/relkit/internal/vnext/canonical"
	"github.com/shichao402/relkit/internal/vnext/catalog"
	"github.com/shichao402/relkit/internal/vnext/contracts"
	"github.com/shichao402/relkit/internal/vnext/document"
	vnextregistry "github.com/shichao402/relkit/internal/vnext/registry"
)

type releaseManifestArtifact struct {
	Component string `json:"component"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
	Path      string `json:"path"`
	SHA256    string `json:"sha256"`
}

type releaseManifest struct {
	Schema            string                    `json:"schema"`
	Version           string                    `json:"version"`
	Commit            string                    `json:"commit"`
	Dirty             bool                      `json:"dirty"`
	MinProtocol       int                       `json:"minProtocol"`
	MaxProtocol       int                       `json:"maxProtocol"`
	MinUpdaterIPC     int                       `json:"minUpdaterIpc"`
	MaxUpdaterIPC     int                       `json:"maxUpdaterIpc"`
	BuiltAt           string                    `json:"builtAt"`
	HostScriptsSHA256 string                    `json:"hostScriptsSha256,omitempty"`
	Artifacts         []releaseManifestArtifact `json:"artifacts"`
}

type BuildDistributionOptions struct {
	Version      string
	ManifestPath string
	Assets       string
	SourceBase   string
	Output       string
}

// BuildDistribution converts one immutable Relkit release into the complete
// vNext distribution row consumed by catalog put. It derives components from
// the declarative registry and refuses incomplete release assets.
func BuildDistribution(options BuildDistributionOptions) (*catalog.Distribution, error) {
	if options.Version == "" || options.ManifestPath == "" || options.Assets == "" ||
		options.SourceBase == "" || options.Output == "" {
		return nil, fmt.Errorf("version, manifest, assets, source-base, and output are required")
	}
	if _, err := catalog.CompareVersions(options.Version, options.Version); err != nil {
		return nil, fmt.Errorf("version: %w", err)
	}
	sourceBase, err := immutableSourceBase(options.SourceBase)
	if err != nil {
		return nil, err
	}
	var manifest releaseManifest
	if err := document.Load(options.ManifestPath, &manifest); err != nil {
		return nil, err
	}
	if manifest.Schema != "relkit.release/1" {
		return nil, fmt.Errorf("release manifest schema is %q, expected relkit.release/1", manifest.Schema)
	}
	if manifest.Dirty {
		return nil, fmt.Errorf("release manifest is dirty")
	}
	if !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(manifest.Commit) {
		return nil, fmt.Errorf("release manifest commit must be a 40-character lowercase hex digest")
	}
	number := strings.TrimPrefix(options.Version, "v")
	if index := strings.IndexAny(number, "-+"); index >= 0 {
		number = number[:index]
	}
	if !strings.HasPrefix(manifest.Version, number+"+") {
		return nil, fmt.Errorf("release manifest version %q does not match distribution %q", manifest.Version, options.Version)
	}
	if manifest.MinUpdaterIPC <= 0 || manifest.MaxUpdaterIPC < manifest.MinUpdaterIPC {
		return nil, fmt.Errorf("release manifest has invalid updater IPC window [%d,%d]", manifest.MinUpdaterIPC, manifest.MaxUpdaterIPC)
	}

	assetsRoot, err := filepath.Abs(options.Assets)
	if err != nil {
		return nil, err
	}
	releaseAssets := map[string]string{}
	manifestRows := map[string]releaseManifestArtifact{}
	for index, artifact := range manifest.Artifacts {
		if artifact.Component == "" || artifact.OS == "" || artifact.Arch == "" || artifact.Path == "" {
			return nil, fmt.Errorf("release manifest artifacts[%d] is incomplete", index)
		}
		if artifact.Path != filepath.Base(artifact.Path) {
			return nil, fmt.Errorf("release manifest artifact path %q is not a file name", artifact.Path)
		}
		key := artifact.Component + "\x00" + artifact.OS + "-" + artifact.Arch
		if _, exists := manifestRows[key]; exists {
			return nil, fmt.Errorf("release manifest contains duplicate component target %s/%s-%s", artifact.Component, artifact.OS, artifact.Arch)
		}
		manifestRows[key] = artifact
		digest, err := verifyReleaseAsset(assetsRoot, artifact.Path, artifact.SHA256)
		if err != nil {
			return nil, err
		}
		releaseAssets[artifact.Path] = digest
	}

	registry, err := vnextregistry.Default()
	if err != nil {
		return nil, err
	}
	components := make([]catalog.Component, 0)
	for _, definition := range registry.Components {
		for _, target := range definition.Targets {
			assetName, manifestTarget, err := definition.ReleaseAssetName(target)
			if err != nil {
				return nil, fmt.Errorf("component %s/%s: %w", definition.Name, target, err)
			}
			expectedHex := ""
			if row, exists := manifestRows[definition.Name+"\x00"+manifestTarget]; exists {
				if row.Path != assetName {
					return nil, fmt.Errorf("component %s/%s manifest path is %q, expected %q", definition.Name, target, row.Path, assetName)
				}
				expectedHex = row.SHA256
			} else if target != "darwin-universal" {
				return nil, fmt.Errorf("release manifest lacks required component %s/%s", definition.Name, target)
			}
			digest, err := verifyReleaseAsset(assetsRoot, assetName, expectedHex)
			if err != nil {
				return nil, fmt.Errorf("component %s/%s: %w", definition.Name, target, err)
			}
			releaseAssets[assetName] = digest
			components = append(components, catalog.Component{
				Name:     definition.Name,
				Version:  options.Version,
				Platform: target,
				SHA256:   digest,
				Source: catalog.Source{
					Kind: string(vnextregistry.TransportReleaseAsset),
					URL:  sourceBase + "/" + url.PathEscape(assetName),
				},
				Requires: append([]contracts.Capability(nil), definition.Requires...),
				Provides: append([]contracts.Capability(nil), definition.Provides...),
			})
		}
	}

	for _, metadata := range []string{"manifest.json"} {
		path := filepath.Join(assetsRoot, metadata)
		if info, statErr := os.Stat(path); statErr == nil && info.Mode().IsRegular() {
			digest, digestErr := fileDigest(path)
			if digestErr != nil {
				return nil, digestErr
			}
			releaseAssets[metadata] = digest
		} else if statErr != nil && !os.IsNotExist(statErr) {
			return nil, statErr
		}
	}
	if err := verifyChecksumFile(assetsRoot, releaseAssets); err != nil {
		return nil, err
	}

	sort.Slice(components, func(left, right int) bool {
		if components[left].Name == components[right].Name {
			return components[left].Platform < components[right].Platform
		}
		return components[left].Name < components[right].Name
	})
	value := &catalog.Distribution{
		Version:       options.Version,
		Commit:        manifest.Commit,
		ReleaseAssets: releaseAssets,
		State:         catalog.StateCandidate,
		Components:    components,
		Requires: []contracts.Capability{{
			Domain: contracts.DomainPublishProtocol,
			Peer:   "agent",
			Min:    publishproto.Min,
			Max:    publishproto.Max,
		}},
		Provides: []contracts.Capability{
			{Domain: contracts.DomainPublishProtocol, Peer: "agent", Min: publishproto.Min, Max: publishproto.Max},
			{Domain: contracts.DomainArtifactSet, Peer: "producer", ID: artifactset.Schema},
			{Domain: contracts.DomainUpdaterIPC, Peer: "facade", Min: manifest.MinUpdaterIPC, Max: manifest.MaxUpdaterIPC},
			{Domain: contracts.DomainRUPSchemaSet, Peer: "directory", ID: "rup.v2"},
		},
	}
	value.Digest, err = value.IdentityDigest()
	if err != nil {
		return nil, err
	}
	if err := document.WriteNew(options.Output, value); err != nil {
		return nil, err
	}
	return value, nil
}

func immutableSourceBase(raw string) (string, error) {
	value, err := url.Parse(strings.TrimRight(raw, "/"))
	if err != nil {
		return "", err
	}
	if value.Scheme != "https" || value.Host == "" || value.RawQuery != "" || value.Fragment != "" {
		return "", fmt.Errorf("source-base must be an immutable https URL without query or fragment")
	}
	return value.String(), nil
}

func verifyReleaseAsset(root, name, expectedHex string) (string, error) {
	path := filepath.Join(root, name)
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("release asset %s: %w", name, err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("release asset %s is not a regular file", name)
	}
	digest, err := fileDigest(path)
	if err != nil {
		return "", err
	}
	if expectedHex != "" {
		expected := canonical.DigestPrefix + strings.ToLower(strings.TrimPrefix(expectedHex, canonical.DigestPrefix))
		if err := canonical.ValidateDigest(expected); err != nil {
			return "", fmt.Errorf("release manifest digest for %s: %w", name, err)
		}
		if digest != expected {
			return "", fmt.Errorf("release asset %s digest is %s, expected %s", name, digest, expected)
		}
	}
	return digest, nil
}

func fileDigest(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}
	return canonical.DigestPrefix + hex.EncodeToString(hasher.Sum(nil)), nil
}

func verifyChecksumFile(root string, assets map[string]string) error {
	raw, err := os.ReadFile(filepath.Join(root, "SHA256SUMS"))
	if os.IsNotExist(err) {
		return fmt.Errorf("release assets do not contain SHA256SUMS")
	}
	if err != nil {
		return err
	}
	checksums := map[string]string{}
	for lineNumber, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			return fmt.Errorf("SHA256SUMS line %d is malformed", lineNumber+1)
		}
		name := strings.TrimPrefix(fields[1], "*")
		if name != filepath.Base(name) {
			return fmt.Errorf("SHA256SUMS line %d has unsafe file name %q", lineNumber+1, name)
		}
		digest := canonical.DigestPrefix + strings.ToLower(fields[0])
		if err := canonical.ValidateDigest(digest); err != nil {
			return fmt.Errorf("SHA256SUMS line %d: %w", lineNumber+1, err)
		}
		if _, exists := checksums[name]; exists {
			return fmt.Errorf("SHA256SUMS contains duplicate file %q", name)
		}
		checksums[name] = digest
	}
	for name, digest := range assets {
		if name == "SHA256SUMS" {
			continue
		}
		expected, ok := checksums[name]
		if !ok {
			return fmt.Errorf("SHA256SUMS does not cover release asset %s", name)
		}
		if expected != digest {
			return fmt.Errorf("SHA256SUMS digest for %s is %s, calculated %s", name, expected, digest)
		}
	}
	return nil
}
