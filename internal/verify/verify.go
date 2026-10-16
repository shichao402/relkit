package verify

import (
	"bytes"
	"crypto/ed25519"
	"fmt"
	"slices"
	"strconv"
	"strings"

	rupv2 "github.com/shichao402/relkit/api/rup/v2"
	"github.com/shichao402/relkit/internal/backends"
	"github.com/shichao402/relkit/internal/chain"
	"github.com/shichao402/relkit/internal/config"
	"github.com/shichao402/relkit/internal/envelope"
	"github.com/shichao402/relkit/internal/httpx"
	"github.com/shichao402/relkit/internal/model"
	vnextdocument "github.com/shichao402/relkit/internal/vnext/document"
	"github.com/shichao402/relkit/internal/vnext/runtimeprofile"
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

type ReleasedOptions struct {
	Channel                  string
	To                       []string
	Deep                     bool
	Version                  string
	ResolutionDigest         string
	OperatorDeploymentID     string
	MinimumIndexSequence     int64
	MinimumDirectorySequence int64
	RuntimeProfileSHA256     string
	RuntimeProfileDigest     string
}

// RunReleased re-reads the committed release from consumer-visible backend
// pointers. It is intentionally independent from the publish write path.
func RunReleased(cfg *config.Config, options ReleasedOptions, printer Printer) (*Findings, error) {
	findings, err := Run(cfg, options.Channel, options.To, options.Deep, printer)
	if err != nil {
		return nil, err
	}
	if options.Version == "" {
		return nil, Error{Message: "released verification requires an expected version"}
	}
	channel, err := cfg.ChannelOrDefault(options.Channel)
	if err != nil {
		return nil, err
	}
	trusted, err := cfg.TrustedPublicKeys()
	if err != nil {
		return nil, err
	}
	targetNames := append([]string(nil), options.To...)
	if len(targetNames) == 0 {
		targetNames = append(targetNames, cfg.PublishTo...)
	}
	for _, name := range targetNames {
		backend, err := backends.Create(name, cfg, cfg.Root)
		if err != nil {
			return nil, err
		}
		raw, getErr := backend.Get(model.IndexKey(cfg.Product, channel))
		if getErr != nil || raw == nil {
			findings.Error(fmt.Sprintf("released index is unavailable on %s", name))
			continue
		}
		env, openErr := rupv2.UnmarshalEnvelope(raw)
		if openErr != nil {
			findings.Error(fmt.Sprintf("released index on %s is invalid: %v", name, openErr))
			continue
		}
		index, openErr := envelope.OpenEnvelope(env, trusted)
		if openErr != nil {
			findings.Error(fmt.Sprintf("released index on %s is untrusted: %v", name, openErr))
			continue
		}
		if index.Sequence < options.MinimumIndexSequence {
			findings.Error(fmt.Sprintf("released index sequence on %s is %d, below required %d", name, index.Sequence, options.MinimumIndexSequence))
		}
		var selected *model.VersionNode
		for _, node := range index.Versions {
			if node != nil && node.Version == options.Version {
				selected = node
				break
			}
		}
		if selected == nil {
			findings.Error(fmt.Sprintf("version %s is absent from released index on %s", options.Version, name))
			continue
		}
		checkProvenance(
			"index",
			name,
			selected.ResolutionDigest,
			selected.OperatorDeploymentId,
			selected.ResolutionIdentity,
			options,
			findings,
		)
		manifestRaw, getErr := backend.Get(model.ManifestKey(cfg.Product, options.Version))
		if getErr != nil || manifestRaw == nil {
			findings.Error(fmt.Sprintf("released manifest %s is unavailable on %s", options.Version, name))
			continue
		}
		manifest, parseErr := rupv2.UnmarshalManifest(manifestRaw)
		if parseErr != nil {
			findings.Error(fmt.Sprintf("released manifest %s on %s is invalid: %v", options.Version, name, parseErr))
			continue
		}
		checkProvenance(
			"manifest",
			name,
			manifest.ResolutionDigest,
			manifest.OperatorDeploymentId,
			manifest.ResolutionIdentity,
			options,
			findings,
		)
	}
	verifyReleasedDirectory(cfg, options, trusted, findings, printer)
	if findings.OK() {
		printer(fmt.Sprintf("released verification passed for %s at resolution %s", options.Version, options.ResolutionDigest))
	}
	return findings, nil
}

func checkProvenance(
	documentName string,
	backendName string,
	resolutionDigest string,
	deploymentID string,
	identity string,
	options ReleasedOptions,
	findings *Findings,
) {
	if options.ResolutionDigest != "" && resolutionDigest != options.ResolutionDigest {
		findings.Error(fmt.Sprintf("%s provenance on %s has resolution %q, expected %q", documentName, backendName, resolutionDigest, options.ResolutionDigest))
	}
	if options.OperatorDeploymentID != "" && deploymentID != options.OperatorDeploymentID {
		findings.Error(fmt.Sprintf("%s provenance on %s has Operator %q, expected %q", documentName, backendName, deploymentID, options.OperatorDeploymentID))
	}
	if options.ResolutionDigest != "" && identity == "" {
		findings.Error(fmt.Sprintf("%s provenance on %s has no resolution identity", documentName, backendName))
	}
}

func verifyReleasedDirectory(
	cfg *config.Config,
	options ReleasedOptions,
	trusted map[string]ed25519.PublicKey,
	findings *Findings,
	printer Printer,
) {
	if cfg.Directory == nil {
		findings.Error("released verification requires directory publication configuration")
		return
	}
	names := append([]string(nil), cfg.Directory.PublishTo...)
	if len(names) == 0 {
		names = append(names, options.To...)
	}
	if len(names) == 0 {
		names = append(names, cfg.PublishTo...)
	}
	key := model.DirectoryKey(cfg.Product)
	var reference []byte
	for _, name := range names {
		backend, err := backends.Create(name, cfg, cfg.Root)
		if err != nil {
			findings.Error(fmt.Sprintf("directory backend %s: %v", name, err))
			continue
		}
		raw, err := backend.Get(key)
		if err != nil || raw == nil {
			findings.Error(fmt.Sprintf("released directory is unavailable on %s", name))
			continue
		}
		if reference == nil {
			reference = append([]byte(nil), raw...)
		} else if !bytes.Equal(reference, raw) {
			findings.Error(fmt.Sprintf("released directory is not byte-identical on backend %s", name))
		}
		env, err := rupv2.UnmarshalEnvelope(raw)
		if err != nil {
			findings.Error(fmt.Sprintf("released directory on %s is invalid: %v", name, err))
			continue
		}
		value, err := envelope.OpenDirectoryEnvelope(env, trusted)
		if err != nil {
			findings.Error(fmt.Sprintf("released directory on %s is untrusted: %v", name, err))
			continue
		}
		if value.Product != cfg.Product {
			findings.Error(fmt.Sprintf("released directory on %s has product %q", name, value.Product))
		}
		if value.DirectorySequence < options.MinimumDirectorySequence {
			findings.Error(fmt.Sprintf("released directory sequence on %s is %d, below required %d", name, value.DirectorySequence, options.MinimumDirectorySequence))
		}
		if options.RuntimeProfileSHA256 != "" {
			if value.RuntimeProfile == nil {
				findings.Error(fmt.Sprintf("released directory on %s has no Runtime Profile", name))
			} else {
				verifyRuntimeProfileReference(cfg.Product, backend, value.RuntimeProfile, options, trusted, findings)
			}
		}
		printer(fmt.Sprintf("    directory on %-12s sequence %d", name, value.DirectorySequence))
	}
}

func verifyRuntimeProfileReference(
	product string,
	backend backends.Backend,
	reference *model.ManifestRef,
	options ReleasedOptions,
	trusted map[string]ed25519.PublicKey,
	findings *Findings,
) {
	if reference.Sha256 != options.RuntimeProfileSHA256 {
		findings.Error(fmt.Sprintf("Runtime Profile reference on %s has sha256 %s, expected %s", backend.Name(), reference.Sha256, options.RuntimeProfileSHA256))
		return
	}
	key := fmt.Sprintf("runtime-profile/%s/%s.json", product, reference.Sha256)
	if expectedURL := backend.URLFor(key); expectedURL != nil && !containsURL(reference.Urls, *expectedURL) {
		findings.Error(fmt.Sprintf("Runtime Profile reference does not list %s URL %s", backend.Name(), *expectedURL))
	}
	raw, err := backend.Get(key)
	if err != nil || raw == nil {
		findings.Error(fmt.Sprintf("Runtime Profile is unavailable on %s at %s", backend.Name(), key))
		return
	}
	if digest := model.Sha256Bytes(raw); digest != reference.Sha256 || int64(len(raw)) != reference.Size {
		findings.Error(fmt.Sprintf("Runtime Profile bytes on %s do not match directory reference", backend.Name()))
		return
	}
	var profile runtimeprofile.Document
	if err := vnextdocument.Decode(raw, &profile); err != nil {
		findings.Error(fmt.Sprintf("Runtime Profile on %s is invalid JSON: %v", backend.Name(), err))
		return
	}
	if err := profile.Verify(trusted); err != nil {
		findings.Error(fmt.Sprintf("Runtime Profile on %s is untrusted: %v", backend.Name(), err))
		return
	}
	if options.RuntimeProfileDigest != "" && profile.Digest != options.RuntimeProfileDigest {
		findings.Error(fmt.Sprintf("Runtime Profile on %s has digest %s, expected %s", backend.Name(), profile.Digest, options.RuntimeProfileDigest))
	}
}

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

		env, err := rupv2.UnmarshalEnvelope(raw)
		if err != nil {
			findings.Error(fmt.Sprintf("index on %s is not valid protobuf: %v", name, err))
			continue
		}

		index, err := envelope.OpenEnvelope(env, trusted)
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
			var newest int64
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
		if node == nil {
			findings.Error("index contains an empty version node")
			continue
		}
		if node.Version == "" {
			findings.Error("version node is missing \"version\"")
		}
		if node.Manifest == nil {
			findings.Error(fmt.Sprintf("version %q is missing manifest", node.Version))
			continue
		}
		if node.Manifest.Sha256 != "" && (len(node.Manifest.Sha256) != 64 || node.Manifest.Sha256 != strings.ToLower(node.Manifest.Sha256)) {
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
	nodes := append([]*model.VersionNode(nil), index.Versions...)
	slices.SortFunc(nodes, compareVersionNodes)
	for _, node := range nodes {
		if node == nil || node.Manifest == nil {
			continue
		}
		key := model.ManifestKey(product, node.Version)
		checkDeclaredURL(backend, key, node.Manifest.Urls, "index entry for "+node.Version, findings)

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
		if actualDigest != node.Manifest.Sha256 || int64(len(raw)) != node.Manifest.Size {
			findings.Error(fmt.Sprintf("manifest-digest-mismatch for %s on %s: index records %s (%d bytes), stored manifest is %s (%d bytes)", node.Version, backend.Name(), prefix(node.Manifest.Sha256), node.Manifest.Size, prefix(actualDigest), len(raw)))
			continue
		}

		manifest, err := rupv2.UnmarshalManifest(raw)
		if err != nil {
			findings.Error(fmt.Sprintf("manifest %s is not valid protobuf: %v", node.Version, err))
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
	nodes := append([]*model.VersionNode(nil), index.Versions...)
	slices.SortFunc(nodes, compareVersionNodes)
	for _, node := range nodes {
		if node == nil {
			continue
		}
		raw, err := backend.Get(model.ManifestKey(product, node.Version))
		if err != nil || raw == nil {
			continue
		}

		manifest, err := rupv2.UnmarshalManifest(raw)
		if err != nil {
			continue
		}
		for _, artifact := range manifest.Artifacts {
			if artifact == nil {
				continue
			}
			key := model.ArtifactKey(product, node.Version, artifact.Filename)
			url := checkDeclaredURL(backend, key, artifact.Urls, fmt.Sprintf("manifest %s, artifact %s", node.Version, artifact.Id), findings)
			if !backend.URLsAreLive() {
				stored, err := backend.Get(key)
				if err != nil || stored == nil {
					findings.Error(fmt.Sprintf("artifact missing on %s: %s", backend.Name(), key))
				} else if int64(len(stored)) != artifact.Size {
					findings.Error(fmt.Sprintf("artifact %s on %s is %d bytes, manifest says %d", key, backend.Name(), len(stored), artifact.Size))
				} else {
					printer(fmt.Sprintf("    %-12s %s ok (stored bytes)", node.Version, artifact.Id))
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
				findings.Error(fmt.Sprintf("artifact %s is %d bytes at the URL, manifest says %d\n           %s", artifact.Id, valueOrZero(size), artifact.Size, *url))
			} else {
				sizeText := ""
				if size != nil {
					sizeText = fmt.Sprintf(", %d bytes", *size)
				}
				printer(fmt.Sprintf("    %-12s %s ok (%s%s)", node.Version, artifact.Id, note, sizeText))
			}
		}
	}
}

func compareVersionNodes(a, b *model.VersionNode) int {
	switch {
	case a == nil && b == nil:
		return 0
	case a == nil:
		return 1
	case b == nil:
		return -1
	case a.Code < b.Code:
		return -1
	case a.Code > b.Code:
		return 1
	default:
		return 0
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
