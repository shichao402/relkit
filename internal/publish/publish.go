package publish

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	rupv2 "cnb.cool/shichao402/relkit/api/rup/v2"
	"cnb.cool/shichao402/relkit/internal/backends"
	"cnb.cool/shichao402/relkit/internal/browse"
	"cnb.cool/shichao402/relkit/internal/chain"
	"cnb.cool/shichao402/relkit/internal/changelog"
	"cnb.cool/shichao402/relkit/internal/config"
	"cnb.cool/shichao402/relkit/internal/envelope"
	"cnb.cool/shichao402/relkit/internal/model"
	"cnb.cool/shichao402/relkit/internal/stage"
	"cnb.cool/shichao402/relkit/internal/webmeta"
)

type Error struct {
	Message string
}

func (e Error) Error() string {
	return e.Message
}

type Printer func(string)

func Run(cfg *config.Config, version string, to []string, dryRun bool, allowBackfill bool, allowPartial bool, printer Printer) (*model.IndexDocument, error) {
	if printer == nil {
		printer = func(string) {}
	}

	staged, err := stage.LoadStaged(cfg.Root, version)
	if err != nil {
		return nil, err
	}
	channel := staged.Channel
	mismatches := stage.VerifyStagedHashes(cfg, staged)
	if len(mismatches) > 0 {
		return nil, Error{Message: "staging tree no longer matches staged.pb:\n  " + strings.Join(mismatches, "\n  ") + "\nre-run 'relkit stage'"}
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
	var unwritable []string
	for _, backend := range openedBackends {
		if !backend.Writable() {
			unwritable = append(unwritable, backend.Name())
		}
	}
	if len(unwritable) > 0 {
		return nil, Error{Message: fmt.Sprintf("backend(s) %s are read-only and cannot be published to", strings.Join(unwritable, ", "))}
	}
	if !dryRun {
		for _, backend := range openedBackends {
			if preflighter, ok := backend.(backends.PublishPreflighter); ok {
				if err := preflighter.Preflight(); err != nil {
					return nil, Error{Message: fmt.Sprintf("publisher compatibility check failed for %s: %v", backend.Name(), err)}
				}
			}
		}
	}

	var signers []envelope.Signer
	if !dryRun {
		signers, err = cfg.LoadSigners()
		if err != nil {
			return nil, err
		}
	}

	trusted := trustedKeys(cfg, signers)
	if dryRun {
		if configured, err := cfg.TrustedPublicKeys(); err == nil {
			for keyID, publicKey := range configured {
				trusted[keyID] = publicKey
			}
		}
	}

	var descriptions []string
	for _, backend := range openedBackends {
		descriptions = append(descriptions, backend.Describe())
	}
	printer(fmt.Sprintf("publishing %s (code %d) to: %s", version, staged.Code, strings.Join(descriptions, ", ")))

	printer("reading current index...")
	existing, baseSequence, rawByBackend, err := readExistingIndexes(openedBackends, cfg, channel, trusted, printer)
	if err != nil {
		return nil, err
	}
	_ = rawByBackend

	if err := checkCodeMonotonic(existing, staged, allowBackfill); err != nil {
		return nil, err
	}

	provisionalNode := model.NewIndexNode(staged, strings.Repeat("0", 64), 0, []string{"https://placeholder.invalid/"}, "")
	merged := mergeNode(existing.Versions, provisionalNode)
	retained, pruned := applyRetainVersions(merged, cfg.RetainVersions)
	if len(pruned) > 0 {
		printer(fmt.Sprintf("  retainVersions=%d: dropping %s from index (orphans become GC-eligible after publish)",
			cfg.RetainVersions, formatVersionList(pruned)))
	}
	if err := changelog.CompactPriorNodes(retained, cfg.Changelog.URLTemplate, staged.Code); err != nil {
		return nil, Error{Message: err.Error()}
	}
	provisional, err := model.NewIndex(cfg.Product, channel, baseSequence+1, retained, model.MinSupportedPtr(existing), "")
	if err != nil {
		return nil, err
	}
	warnings, err := validateIndex(provisional, "the resulting index")
	if err != nil {
		return nil, err
	}
	for _, warning := range warnings {
		printer("  warning: " + warning)
	}

	path := chain.ResolveUpgradePath(provisional, 0)
	pathVersions := make([]string, 0, len(path))
	for _, node := range path {
		pathVersions = append(pathVersions, node.Version)
	}
	printer("  chain from a fresh install: " + strings.Join(pathVersions, " -> "))

	if dryRun {
		printer("")
		printer("dry run: validation passed, nothing uploaded")
		printer(fmt.Sprintf("would write sequence %d and these keys:", baseSequence+1))
		for _, artifact := range staged.Artifacts {
			printer("  " + model.ArtifactKey(cfg.Product, version, artifact.Filename))
		}
		printer("  " + model.ManifestKey(cfg.Product, version))
		printer("  " + model.IndexKey(cfg.Product, channel) + "  <- pointer, written last")
		if hasSiteCopy(cfg) {
			printer("  " + webmeta.SiteKey(cfg.Product) + "  <- site copy")
		}
		printer("  " + webmeta.LatestKey(cfg.Product, channel) + "  <- fixed latest links")
		printer("  " + browse.ProductKey(cfg.Product) + "  <- human index dump")
		printer("  " + browse.IndexKey() + "  <- human catalog page")
		printer("  " + browse.CatalogKey() + "  <- human catalog json")
		for _, sink := range OpenBrowseSinks(cfg, openedBackends) {
			printer("  " + sink.Name() + "  <- BrowseSink")
		}
		warnMissingSiteSink(cfg, openedBackends, printer)
		return nil, nil
	}

	printer("uploading artifacts...")
	directory := stage.ArtifactsDir(cfg.Root, version)
	urlsByArtifact := make(map[string][]string, len(staged.Artifacts))
	for _, artifact := range staged.Artifacts {
		urlsByArtifact[artifact.Id] = []string{}
	}
	for _, backend := range openedBackends {
		for _, artifact := range staged.Artifacts {
			key := model.ArtifactKey(cfg.Product, version, artifact.Filename)
			urls, err := backend.PutArtifact(filepath.Join(directory, artifact.Filename), key)
			if err != nil {
				return nil, err
			}
			urlsByArtifact[artifact.Id] = append(urlsByArtifact[artifact.Id], urls...)
			printer(fmt.Sprintf("  %-12s %s", backend.Name(), key))
		}
	}

	manifest, err := model.NewManifestFromStaged(staged, urlsByArtifact, "")
	if err != nil {
		return nil, err
	}
	manifestBytes, err := rupv2.MarshalManifest(manifest)
	if err != nil {
		return nil, err
	}
	manifestDigest := model.Sha256Bytes(manifestBytes)
	manifestSize := int64(len(manifestBytes))

	printer("uploading manifest...")
	manifestKey := model.ManifestKey(cfg.Product, version)
	var manifestURLs []string
	for _, backend := range openedBackends {
		urls, err := backend.PutImmutable(manifestBytes, manifestKey)
		if err != nil {
			return nil, err
		}
		manifestURLs = append(manifestURLs, urls...)
		printer(fmt.Sprintf("  %-12s %s", backend.Name(), manifestKey))
	}

	node := model.NewIndexNode(staged, manifestDigest, manifestSize, manifestURLs, "")
	mergedFinal, _ := applyRetainVersions(mergeNode(existing.Versions, node), cfg.RetainVersions)
	if err := changelog.CompactPriorNodes(mergedFinal, cfg.Changelog.URLTemplate, staged.Code); err != nil {
		return nil, Error{Message: err.Error()}
	}
	index, err := model.NewIndex(cfg.Product, channel, baseSequence+1, mergedFinal, model.MinSupportedPtr(existing), "")
	if err != nil {
		return nil, err
	}
	if _, err := validateIndex(index, "the final index"); err != nil {
		return nil, err
	}

	env, err := envelope.Seal(index, signers)
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
	var written []string
	var committed []backends.Backend
	var failures []string
	for _, backend := range openedBackends {
		if _, err := backend.PutPointer(envelopeBytes, indexKey); err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", backend.Name(), err))
			printer(fmt.Sprintf("  %-12s FAILED: %v", backend.Name(), err))
			continue
		}
		written = append(written, backend.Name())
		committed = append(committed, backend)
		printer(fmt.Sprintf("  %-12s %s", backend.Name(), indexKey))
	}

	// These documents are deliberately written after the signed index commit:
	// a fixed "latest" URL must never advertise a release that clients cannot
	// yet see. They are derived entirely from this publish, so the HTTP request
	// path only reads a pointer; it never scans index/manifest to calculate one.
	webFailures, err := writeWebPointers(cfg, staged, manifest, committed, printer)
	if err != nil {
		return nil, err
	}
	failures = append(failures, webFailures...)
	if len(failures) > 0 && !allowPartial {
		return nil, Error{Message: fmt.Sprintf("pointer write failed on %s (index committed on: %s). The signed release may already be live while a site/latest/browse pointer or Makers index is stale; re-run with --allow-backfill to finish, and use --allow-partial only to accept the divergence.", strings.Join(failures, ", "), chooseNone(strings.Join(written, ", ")))}
	}

	printer("")
	printer(fmt.Sprintf("published %s (code %d) on channel %s, sequence %d", version, staged.Code, channel, index.Sequence))
	printer(fmt.Sprintf("manifest sha256 %s (%d bytes)", manifestDigest, manifestSize))
	return index, nil
}

func openBackends(cfg *config.Config, names []string) ([]backends.Backend, error) {
	result := make([]backends.Backend, 0, len(names))
	for _, name := range names {
		backend, err := backends.Create(name, cfg, cfg.Root)
		if err != nil {
			return nil, err
		}
		result = append(result, backend)
	}
	return result, nil
}

func hasSiteCopy(cfg *config.Config) bool {
	return cfg.Site.Title != "" || cfg.Site.Description != "" || cfg.Site.Homepage != ""
}

func writeWebPointers(
	cfg *config.Config,
	staged *model.StagedDocument,
	manifest *model.ManifestDocument,
	targets []backends.Backend,
	printer Printer,
) ([]string, error) {
	var documents []struct {
		label string
		key   string
		data  []byte
	}

	if hasSiteCopy(cfg) {
		data, err := webmeta.MarshalSite(webmeta.Site{
			Product:     cfg.Product,
			Title:       cfg.Site.Title,
			Description: cfg.Site.Description,
			Homepage:    cfg.Site.Homepage,
			UpdatedAt:   model.UTCNow(),
		})
		if err != nil {
			return nil, err
		}
		documents = append(documents, struct {
			label string
			key   string
			data  []byte
		}{"site", webmeta.SiteKey(cfg.Product), data})
	}

	// Every channel gets its own pointer: a beta link has to keep tracking beta,
	// and a page that only knew the default channel could not offer the others.
	latestDoc := webmeta.Latest{
		Product:     cfg.Product,
		Channel:     staged.Channel,
		Version:     staged.Version,
		Code:        staged.Code,
		PublishedAt: manifest.ReleasedAt,
		Artifacts:   webmeta.ArtifactsFromManifest(manifest),
	}
	data, err := webmeta.MarshalLatest(latestDoc)
	if err != nil {
		return nil, err
	}
	documents = append(documents, struct {
		label string
		key   string
		data  []byte
	}{"latest", webmeta.LatestKey(cfg.Product, staged.Channel), data})

	if len(targets) == 0 {
		return nil, nil
	}
	printer("writing web pointers...")
	var failures []string
	for _, backend := range targets {
		for _, document := range documents {
			if _, err := backend.PutPointer(document.data, document.key); err != nil {
				failures = append(failures, fmt.Sprintf("%s %s: %v", backend.Name(), document.label, err))
				printer(fmt.Sprintf("  %-12s %s FAILED: %v", backend.Name(), document.key, err))
				continue
			}
			printer(fmt.Sprintf("  %-12s %s", backend.Name(), document.key))
		}
	}

	var siteDoc *webmeta.Site
	if hasSiteCopy(cfg) {
		siteDoc = &webmeta.Site{
			Product:     cfg.Product,
			Title:       cfg.Site.Title,
			Description: cfg.Site.Description,
			Homepage:    cfg.Site.Homepage,
			UpdatedAt:   model.UTCNow(),
		}
	}
	catalog := browse.ApplyPublish(loadBrowseCatalog(cfg, targets), siteDoc, latestDoc, model.UTCNow())
	indexHTML, err := browse.RenderIndex(catalog)
	if err != nil {
		return nil, err
	}
	productHTML, err := browse.RenderProduct(browse.ProductPage(catalog, cfg.Product))
	if err != nil {
		return nil, err
	}
	catalogJSON, err := browse.MarshalCatalog(catalog)
	if err != nil {
		return nil, err
	}
	dump := map[string][]byte{
		browse.IndexKey():              indexHTML,
		browse.ProductKey(cfg.Product): productHTML,
		browse.CatalogKey():            catalogJSON,
	}
	if err := browse.WriteDump(cfg.Root, dump); err != nil {
		return nil, err
	}

	sinks := OpenBrowseSinks(cfg, targets)
	if len(sinks) > 0 {
		printer("deploying human index...")
	}
	for _, sink := range sinks {
		if err := sink.Deploy(dump); err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", sink.Name(), err))
			printer(fmt.Sprintf("  %-12s FAILED: %v", sink.Name(), err))
			continue
		}
		printer(fmt.Sprintf("  %-12s .relkit/browse", sink.Name()))
	}
	warnMissingSiteSink(cfg, targets, printer)
	return failures, nil
}

func loadBrowseCatalog(cfg *config.Config, targets []backends.Backend) *browse.Catalog {
	if dumped := browse.ReadDumpCatalog(cfg.Root); dumped != nil {
		return dumped
	}
	for _, sink := range OpenBrowseSinks(cfg, targets) {
		if doc := sink.LoadCatalog(); doc != nil {
			return doc
		}
	}
	return nil
}

func trustedKeys(cfg *config.Config, signers []envelope.Signer) map[string]ed25519.PublicKey {
	trusted := map[string]ed25519.PublicKey{}
	if configured, err := cfg.TrustedPublicKeys(); err == nil {
		for keyID, publicKey := range configured {
			trusted[keyID] = publicKey
		}
	}
	for _, signer := range signers {
		trusted[signer.KeyID] = signer.PublicKey()
	}
	return trusted
}

func checkClientsCanVerify(cfg *config.Config, env *model.Envelope, channel string) error {
	trusted, err := cfg.TrustedPublicKeys()
	if err != nil || len(trusted) == 0 {
		return nil
	}

	reason := ""
	if _, err := envelope.OpenEnvelope(env, trusted); err != nil {
		reason = err.Error()
	} else if !envelope.AcceptEnvelope(env, trusted, cfg.Product, channel) {
		reason = fmt.Sprintf("the envelope does not bind to product %q channel %q", cfg.Product, channel)
	}
	if reason == "" {
		return nil
	}

	signedBy := make([]string, 0, len(env.Signatures))
	for _, signature := range env.Signatures {
		if signature == nil {
			continue
		}
		signedBy = append(signedBy, signature.KeyId)
	}
	slices.Sort(signedBy)
	trustedIDs := make([]string, 0, len(trusted))
	for keyID := range trusted {
		trustedIDs = append(trustedIDs, keyID)
	}
	slices.Sort(trustedIDs)

	return Error{Message: fmt.Sprintf("the index was signed, but the configured public keys reject it, so every client would too: %s\n  signed by: %s\n  trusted:   %s\nMake signing.keyId name a key that appears in signing.publicKeys, and sign with that key's private half. Nothing was published.", reason, strings.Join(signedBy, ", "), strings.Join(trustedIDs, ", "))}
}

func readExistingIndexes(openedBackends []backends.Backend, cfg *config.Config, channel string, trusted map[string]ed25519.PublicKey, printer Printer) (*model.IndexDocument, int, map[string][]byte, error) {
	key := model.IndexKey(cfg.Product, channel)
	legacyKey := path.Join("index", cfg.Product, channel+".json")
	rawByBackend := map[string][]byte{}
	parsed := map[string]*model.IndexDocument{}

	for _, backend := range openedBackends {
		raw, err := backend.Get(key)
		if err != nil {
			return nil, 0, nil, Error{Message: fmt.Sprintf("could not read the current index from backend %q: %v", backend.Name(), err)}
		}
		rawByBackend[backend.Name()] = raw
		if raw == nil {
			printer(fmt.Sprintf("  %-12s no existing protobuf index", backend.Name()))
			continue
		}

		env, err := rupv2.UnmarshalEnvelope(raw)
		if err != nil {
			return nil, 0, nil, Error{Message: fmt.Sprintf("index on backend %q is not valid protobuf: %v", backend.Name(), err)}
		}
		index, err := envelope.OpenEnvelope(env, trusted)
		if err != nil {
			return nil, 0, nil, Error{Message: fmt.Sprintf("index on backend %q failed verification: %v", backend.Name(), err)}
		}
		if index.Product != cfg.Product || index.Channel != channel {
			return nil, 0, nil, Error{Message: fmt.Sprintf("index on backend %q is for %s/%s, expected %s/%s", backend.Name(), index.Product, index.Channel, cfg.Product, channel)}
		}
		parsed[backend.Name()] = index
		printer(fmt.Sprintf("  %-12s sequence %d, %d version(s)", backend.Name(), index.Sequence, len(index.Versions)))
	}

	legacyFloor := 0
	for _, backend := range openedBackends {
		legacyRaw, err := backend.Get(legacyKey)
		if err != nil {
			return nil, 0, nil, Error{Message: fmt.Sprintf("could not read legacy index from backend %q: %v", backend.Name(), err)}
		}
		if legacyRaw == nil {
			continue
		}
		seq, err := legacyV1Sequence(legacyRaw, trusted, cfg.Product, channel)
		if err != nil {
			printer(fmt.Sprintf("  %-12s ignoring legacy %s: %v", backend.Name(), legacyKey, err))
			continue
		}
		if seq > legacyFloor {
			legacyFloor = seq
		}
		printer(fmt.Sprintf("  %-12s legacy v1 sequence floor %d (%s)", backend.Name(), seq, legacyKey))
	}

	if len(parsed) == 0 {
		if legacyFloor > 0 {
			printer(fmt.Sprintf("  using legacy v1 sequence floor %d (no protobuf index yet)", legacyFloor))
			return model.EmptyIndex(cfg.Product, channel), legacyFloor, rawByBackend, nil
		}
		printer("  first release on this channel")
		return model.EmptyIndex(cfg.Product, channel), 0, rawByBackend, nil
	}

	var distinct [][]byte
	for _, raw := range rawByBackend {
		if raw != nil {
			distinct = append(distinct, raw)
		}
	}
	for i := 1; i < len(distinct); i++ {
		if !bytes.Equal(distinct[0], distinct[i]) {
			var sequences []string
			for name, index := range parsed {
				sequences = append(sequences, fmt.Sprintf("%s=%d", name, index.Sequence))
			}
			slices.Sort(sequences)
			return nil, 0, nil, Error{Message: fmt.Sprintf("backends disagree on the current index (%s); run 'relkit verify' and resolve the divergence before publishing", strings.Join(sequences, ", "))}
		}
	}

	var newest *model.IndexDocument
	for _, index := range parsed {
		if newest == nil || index.Sequence > newest.Sequence {
			newest = index
		}
	}
	baseSequence := int(newest.Sequence)
	if legacyFloor > baseSequence {
		printer(fmt.Sprintf("  elevating sequence floor from legacy v1: %d -> %d", baseSequence, legacyFloor))
		baseSequence = legacyFloor
	}
	return newest, baseSequence, rawByBackend, nil
}

// legacyV1Sequence reads a discarded rup.envelope/1 JSON document solely to keep
// the sequence watermark monotonic across the v1→v2 cutover. Versions are not
// imported (their manifests remain JSON and are not usable by v2 clients).
func legacyV1Sequence(raw []byte, trusted map[string]ed25519.PublicKey, expectProduct, expectChannel string) (int, error) {
	var env struct {
		Schema     string `json:"schema"`
		Payload    string `json:"payload"`
		Signatures []struct {
			KeyID string `json:"keyId"`
			Alg   string `json:"alg"`
			Sig   string `json:"sig"`
		} `json:"signatures"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return 0, fmt.Errorf("not JSON: %w", err)
	}
	if env.Schema != "rup.envelope/1" {
		return 0, fmt.Errorf("unexpected schema %q", env.Schema)
	}
	payload, err := base64.StdEncoding.DecodeString(env.Payload)
	if err != nil {
		return 0, fmt.Errorf("payload is not base64: %w", err)
	}

	verified := false
	for _, entry := range env.Signatures {
		if entry.Alg != "ed25519" {
			continue
		}
		publicKey, ok := trusted[entry.KeyID]
		if !ok {
			continue
		}
		sig, err := base64.StdEncoding.DecodeString(entry.Sig)
		if err != nil {
			continue
		}
		if ed25519.Verify(publicKey, payload, sig) {
			verified = true
			break
		}
	}
	if !verified {
		return 0, fmt.Errorf("no trusted key verified the legacy envelope")
	}

	var index struct {
		Product  string `json:"product"`
		Channel  string `json:"channel"`
		Sequence int64  `json:"sequence"`
	}
	if err := json.Unmarshal(payload, &index); err != nil {
		return 0, fmt.Errorf("payload is not a JSON index: %w", err)
	}
	if index.Product != expectProduct || index.Channel != expectChannel {
		return 0, fmt.Errorf("legacy index is for %s/%s", index.Product, index.Channel)
	}
	if index.Sequence < 1 {
		return 0, fmt.Errorf("legacy sequence %d is invalid", index.Sequence)
	}
	return int(index.Sequence), nil
}

func checkCodeMonotonic(existing *model.IndexDocument, staged *model.StagedDocument, allowBackfill bool) error {
	if len(existing.Versions) == 0 {
		return nil
	}
	highest := existing.Versions[0].Code
	for _, version := range existing.Versions[1:] {
		if version.Code > highest {
			highest = version.Code
		}
	}
	if staged.Code > highest {
		return nil
	}

	message := fmt.Sprintf("new code %d is not greater than the highest existing code %d; this usually means the code source reset (see CLI.md 4.2)", staged.Code, highest)
	if !allowBackfill {
		return Error{Message: message + "; pass --allow-backfill only if you are deliberately backfilling a historical version"}
	}
	return nil
}

func mergeNode(existing []*model.VersionNode, node *model.VersionNode) []*model.VersionNode {
	versions := make([]*model.VersionNode, 0, len(existing)+1)
	for _, version := range existing {
		if version.Code != node.Code {
			versions = append(versions, version)
		}
	}
	versions = append(versions, node)
	return versions
}

// applyRetainVersions keeps at most keep highest-code nodes. keep <= 0 means
// unlimited (full history). Returns the retained slice and the dropped nodes.
func applyRetainVersions(versions []*model.VersionNode, keep int) (retained, pruned []*model.VersionNode) {
	if keep <= 0 || len(versions) <= keep {
		out := make([]*model.VersionNode, len(versions))
		copy(out, versions)
		return out, nil
	}

	ordered := make([]*model.VersionNode, 0, len(versions))
	for _, version := range versions {
		if version != nil {
			ordered = append(ordered, version)
		}
	}
	slices.SortFunc(ordered, func(a, b *model.VersionNode) int {
		if a.Code != b.Code {
			if a.Code > b.Code {
				return -1
			}
			return 1
		}
		return strings.Compare(a.Version, b.Version)
	})

	pruned = append([]*model.VersionNode(nil), ordered[keep:]...)
	kept := append([]*model.VersionNode(nil), ordered[:keep]...)
	// Ascending code order matches the usual publish-append history shape.
	slices.SortFunc(kept, func(a, b *model.VersionNode) int {
		if a.Code != b.Code {
			if a.Code < b.Code {
				return -1
			}
			return 1
		}
		return strings.Compare(a.Version, b.Version)
	})
	return kept, pruned
}

func formatVersionList(nodes []*model.VersionNode) string {
	parts := make([]string, 0, len(nodes))
	for _, node := range nodes {
		if node == nil {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s (code %d)", node.Version, node.Code))
	}
	return strings.Join(parts, ", ")
}

func validateIndex(index *model.IndexDocument, what string) ([]string, error) {
	errors, warnings := chain.ValidateReachability(index)
	if len(errors) > 0 {
		stranded := chain.UnreachableStartCodes(index)
		detail := ""
		if len(stranded) > 0 {
			var codes []string
			for _, code := range stranded {
				codes = append(codes, strconv.Itoa(code))
			}
			detail = "; these start codes cannot reach the newest version: " + strings.Join(codes, ", ")
		}
		return nil, Error{Message: fmt.Sprintf("%s failed reachability validation: %s%s", what, strings.Join(errors, ", "), detail)}
	}
	return warnings, nil
}

func chooseNone(value string) string {
	if value == "" {
		return "none"
	}
	return value
}
