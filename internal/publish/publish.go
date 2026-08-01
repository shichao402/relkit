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

	rupv2 "github.com/shichao402/relkit/api/rup/v2"
	"github.com/shichao402/relkit/internal/backends"
	"github.com/shichao402/relkit/internal/chain"
	"github.com/shichao402/relkit/internal/config"
	"github.com/shichao402/relkit/internal/envelope"
	"github.com/shichao402/relkit/internal/model"
	"github.com/shichao402/relkit/internal/stage"
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
	provisional, err := model.NewIndex(cfg.Product, channel, baseSequence+1, mergeNode(existing.Versions, provisionalNode), model.MinSupportedPtr(existing), "")
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
	index, err := model.NewIndex(cfg.Product, channel, baseSequence+1, mergeNode(existing.Versions, node), model.MinSupportedPtr(existing), "")
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
	var failures []string
	for _, backend := range openedBackends {
		if _, err := backend.PutPointer(envelopeBytes, indexKey); err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", backend.Name(), err))
			printer(fmt.Sprintf("  %-12s FAILED: %v", backend.Name(), err))
			continue
		}
		written = append(written, backend.Name())
		printer(fmt.Sprintf("  %-12s %s", backend.Name(), indexKey))
	}
	if len(failures) > 0 && !allowPartial {
		return nil, Error{Message: fmt.Sprintf("pointer write failed on %s (succeeded on: %s). Clients reading a lagging backend see an older sequence and treat it as 'no update' rather than an error, so this is not an outage; re-run to finish.", strings.Join(failures, ", "), chooseNone(strings.Join(written, ", ")))}
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
