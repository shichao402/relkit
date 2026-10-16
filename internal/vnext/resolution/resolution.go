// Package resolution selects and records one immutable Relkit distribution
// for an entire CI run.
package resolution

import (
	"crypto/ed25519"
	"fmt"
	"slices"
	"strings"

	"github.com/shichao402/relkit/internal/vnext/canonical"
	"github.com/shichao402/relkit/internal/vnext/catalog"
	"github.com/shichao402/relkit/internal/vnext/contracts"
	"github.com/shichao402/relkit/internal/vnext/document"
	"github.com/shichao402/relkit/internal/vnext/operator"
	"github.com/shichao402/relkit/internal/vnext/project"
	vnextregistry "github.com/shichao402/relkit/internal/vnext/registry"
	"github.com/shichao402/relkit/internal/vnext/runtimeprofile"
	"github.com/shichao402/relkit/internal/vnext/trust"
)

const Schema = "relkit.resolution/1"

type Identity string

const (
	IdentityProduction Identity = "production"
	IdentityCandidate  Identity = "candidate"
)

type OperatorProof struct {
	DeploymentID    string                   `json:"deploymentId"`
	ProfileDigest   string                   `json:"profileDigest"`
	Sequence        int64                    `json:"sequence"`
	SelectionPolicy operator.SelectionPolicy `json:"selectionPolicy"`
}

type SelectedDistribution struct {
	Version       string            `json:"version"`
	Commit        string            `json:"commit"`
	Digest        string            `json:"digest"`
	ModulePath    string            `json:"modulePath,omitempty"`
	ModuleH1      string            `json:"moduleH1,omitempty"`
	ReleaseAssets map[string]string `json:"releaseAssets,omitempty"`
}

type Component struct {
	Name     string   `json:"name"`
	Version  string   `json:"version"`
	Platform string   `json:"platform"`
	SHA256   string   `json:"sha256"`
	Source   string   `json:"source"`
	Locator  string   `json:"locator,omitempty"`
	Mirrors  []string `json:"mirrors,omitempty"`
}

type Document struct {
	Schema                string                 `json:"schema"`
	Identity              Identity               `json:"identity"`
	AuthorityFingerprint  string                 `json:"authorityFingerprint"`
	AllowedChannels       []string               `json:"allowedChannels,omitempty"`
	ProjectDigest         string                 `json:"projectDigest"`
	Operator              OperatorProof          `json:"operator"`
	CatalogSnapshotDigest string                 `json:"catalogSnapshotDigest"`
	RuntimeProfileDigests map[string]string      `json:"runtimeProfileDigests,omitempty"`
	SelectedDistribution  SelectedDistribution   `json:"selectedDistribution"`
	Components            []Component            `json:"components"`
	Contracts             []contracts.Capability `json:"contracts"`
	Digest                string                 `json:"digest"`
}

type unsignedDocument struct {
	Schema                string                 `json:"schema"`
	Identity              Identity               `json:"identity"`
	AuthorityFingerprint  string                 `json:"authorityFingerprint"`
	AllowedChannels       []string               `json:"allowedChannels,omitempty"`
	ProjectDigest         string                 `json:"projectDigest"`
	Operator              OperatorProof          `json:"operator"`
	CatalogSnapshotDigest string                 `json:"catalogSnapshotDigest"`
	RuntimeProfileDigests map[string]string      `json:"runtimeProfileDigests,omitempty"`
	SelectedDistribution  SelectedDistribution   `json:"selectedDistribution"`
	Components            []Component            `json:"components"`
	Contracts             []contracts.Capability `json:"contracts"`
}

type Request struct {
	Project                  project.Document
	Operator                 operator.Document
	Catalog                  catalog.Document
	TrustedDistributionKeys  map[string]ed25519.PublicKey
	TrustedCandidateKeys     map[string]ed25519.PublicKey
	CandidateRootFingerprint string
	RuntimeProfiles          map[string]runtimeprofile.Document
	TrustedRuntimeRoots      map[string]map[string]ed25519.PublicKey
	Identity                 Identity
	CandidateDigest          string
	MinimumProfileSequence   int64
	MinimumCatalogSequence   int64
}

func Resolve(request Request) (*Document, error) {
	if err := request.Project.Validate(); err != nil {
		return nil, fmt.Errorf("project: %w", err)
	}
	identity := request.Identity
	if identity == "" {
		identity = IdentityProduction
	}
	authorityFingerprint := request.Project.TrustFingerprint
	trustedKeys := request.TrustedDistributionKeys
	if identity == IdentityCandidate {
		if request.CandidateRootFingerprint == "" {
			return nil, fmt.Errorf("candidate identity requires an independently pinned Candidate Root fingerprint")
		}
		authorityFingerprint = request.CandidateRootFingerprint
		trustedKeys = request.TrustedCandidateKeys
	}
	trustedRoot, err := matchingRootKeys(authorityFingerprint, trustedKeys)
	if err != nil {
		return nil, err
	}
	if err := request.Operator.Verify(trustedRoot); err != nil {
		return nil, fmt.Errorf("operator profile: %w", err)
	}
	if err := request.Catalog.Verify(trustedRoot); err != nil {
		return nil, fmt.Errorf("distribution catalog: %w", err)
	}
	if request.Operator.Sequence < request.MinimumProfileSequence {
		return nil, fmt.Errorf("operator profile sequence %d is older than required %d", request.Operator.Sequence, request.MinimumProfileSequence)
	}
	if request.Catalog.Sequence < request.MinimumCatalogSequence {
		return nil, fmt.Errorf("catalog sequence %d is older than required %d", request.Catalog.Sequence, request.MinimumCatalogSequence)
	}
	runtimeProfileDigests, err := verifyRuntimeProfiles(request)
	if err != nil {
		return nil, err
	}
	selected, err := selectDistribution(request)
	if err != nil {
		return nil, err
	}
	if err := compatible(request.Project, request.Operator, selected); err != nil {
		return nil, fmt.Errorf("distribution %s is incompatible: %w", selected.Version, err)
	}
	components, err := selectComponents(request.Project, request.Operator, selected)
	if err != nil {
		return nil, err
	}
	if err := verifyRuntimeProfileContracts(request.RuntimeProfiles, components); err != nil {
		return nil, err
	}
	projectDigest, err := request.Project.Digest()
	if err != nil {
		return nil, err
	}
	operatorDigest, err := request.Operator.Digest()
	if err != nil {
		return nil, err
	}
	catalogDigest, err := request.Catalog.Digest()
	if err != nil {
		return nil, err
	}
	allowedChannels := append([]string(nil), request.Project.Channels...)
	if identity == IdentityCandidate {
		allowedChannels = slices.DeleteFunc(allowedChannels, func(channel string) bool {
			return channel == "stable"
		})
	}
	value := &Document{
		Schema:               Schema,
		Identity:             identity,
		AuthorityFingerprint: authorityFingerprint,
		AllowedChannels:      allowedChannels,
		ProjectDigest:        projectDigest,
		Operator: OperatorProof{
			DeploymentID:    request.Operator.DeploymentID,
			ProfileDigest:   operatorDigest,
			Sequence:        request.Operator.Sequence,
			SelectionPolicy: request.Operator.SelectionPolicy,
		},
		CatalogSnapshotDigest: catalogDigest,
		RuntimeProfileDigests: runtimeProfileDigests,
		SelectedDistribution: SelectedDistribution{
			Version:       selected.Version,
			Commit:        selected.Commit,
			Digest:        selected.Digest,
			ModulePath:    selected.ModulePath,
			ModuleH1:      selected.ModuleH1,
			ReleaseAssets: cloneMap(selected.ReleaseAssets),
		},
		Components: components,
		Contracts:  append([]contracts.Capability(nil), selected.Provides...),
	}
	if err := value.Finalize(); err != nil {
		return nil, err
	}
	return value, nil
}

func verifyRuntimeProfiles(request Request) (map[string]string, error) {
	expected := make(map[string]project.Subscription, len(request.Project.RuntimeProfileSubscriptions))
	for _, subscription := range request.Project.RuntimeProfileSubscriptions {
		expected[subscription.ProductRef] = subscription
	}
	for product := range request.RuntimeProfiles {
		if _, ok := expected[product]; !ok {
			return nil, fmt.Errorf("runtime profile %q was injected but is not declared by the project", product)
		}
	}
	digests := make(map[string]string, len(expected))
	for product, subscription := range expected {
		profile, ok := request.RuntimeProfiles[product]
		if !ok {
			return nil, fmt.Errorf("runtime profile %q is required but was not injected", product)
		}
		if profile.Product != product {
			return nil, fmt.Errorf("runtime profile %q declares product %q", product, profile.Product)
		}
		trusted, err := matchingRootKeys(subscription.RootFingerprint, request.TrustedRuntimeRoots[product])
		if err != nil {
			return nil, fmt.Errorf("runtime profile %q: %w", product, err)
		}
		if err := profile.Verify(trusted); err != nil {
			return nil, fmt.Errorf("runtime profile %q: %w", product, err)
		}
		digests[product] = profile.Digest
	}
	if len(digests) == 0 {
		return nil, nil
	}
	return digests, nil
}

func verifyRuntimeProfileContracts(profiles map[string]runtimeprofile.Document, components []Component) error {
	if len(profiles) == 0 {
		return nil
	}
	hasUpdater := false
	for _, component := range components {
		if component.Name == "updater" {
			hasUpdater = true
			break
		}
	}
	if !hasUpdater {
		return fmt.Errorf("runtime profile subscriptions require the updater component")
	}
	registry, err := vnextregistry.Default()
	if err != nil {
		return err
	}
	updater, ok := registry.ByName("updater")
	if !ok {
		return fmt.Errorf("component registry has no updater")
	}
	for product, profile := range profiles {
		requirement := contracts.Capability{
			Domain: contracts.DomainRUPSchemaSet,
			Peer:   "directory",
			ID:     profile.RUPSchemaSet,
		}
		matched := false
		for _, supported := range updater.Requires {
			if contracts.Compatible(requirement, supported) {
				matched = true
				break
			}
		}
		if !matched {
			return fmt.Errorf("runtime profile %q uses unsupported RUP Schema Set %q", product, profile.RUPSchemaSet)
		}
	}
	return nil
}

func Load(path string) (*Document, error) {
	var value Document
	if err := document.Load(path, &value); err != nil {
		return nil, err
	}
	if err := value.Validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &value, nil
}

func (d *Document) Finalize() error {
	if d == nil {
		return fmt.Errorf("nil resolution")
	}
	d.Digest = ""
	if err := d.validateUnsigned(); err != nil {
		return err
	}
	digest, err := canonical.Digest(d.unsigned())
	if err != nil {
		return err
	}
	d.Digest = digest
	return nil
}

func (d Document) Validate() error {
	if err := d.validateUnsigned(); err != nil {
		return err
	}
	if err := canonical.ValidateDigest(d.Digest); err != nil {
		return fmt.Errorf("digest: %w", err)
	}
	expected, err := canonical.Digest(d.unsigned())
	if err != nil {
		return err
	}
	if d.Digest != expected {
		return fmt.Errorf("resolution digest mismatch: got %s, expected %s", d.Digest, expected)
	}
	return nil
}

func (d Document) validateUnsigned() error {
	if d.Schema != Schema {
		return fmt.Errorf("schema is %q, expected %q", d.Schema, Schema)
	}
	switch d.Identity {
	case IdentityProduction, IdentityCandidate:
	default:
		return fmt.Errorf("unknown identity %q", d.Identity)
	}
	if err := canonical.ValidateDigest(d.AuthorityFingerprint); err != nil {
		return fmt.Errorf("authorityFingerprint: %w", err)
	}
	if d.Identity == IdentityCandidate && slices.Contains(d.AllowedChannels, "stable") {
		return fmt.Errorf("candidate resolution must not allow stable")
	}
	for name, digest := range map[string]string{
		"projectDigest":         d.ProjectDigest,
		"profileDigest":         d.Operator.ProfileDigest,
		"catalogSnapshotDigest": d.CatalogSnapshotDigest,
		"distributionDigest":    d.SelectedDistribution.Digest,
	} {
		if err := canonical.ValidateDigest(digest); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	if d.Operator.DeploymentID == "" || d.Operator.Sequence < 1 {
		return fmt.Errorf("operator proof is incomplete")
	}
	if d.SelectedDistribution.Version == "" || d.SelectedDistribution.Commit == "" {
		return fmt.Errorf("selected distribution is incomplete")
	}
	if (d.SelectedDistribution.ModulePath == "") != (d.SelectedDistribution.ModuleH1 == "") {
		return fmt.Errorf("selected distribution must declare modulePath and moduleH1 together")
	}
	if len(d.Components) == 0 {
		return fmt.Errorf("components must not be empty")
	}
	componentKeys := map[string]bool{}
	for _, component := range d.Components {
		if component.Name == "" || component.Version == "" || component.Platform == "" || component.Source == "" {
			return fmt.Errorf("component row is incomplete")
		}
		key := component.Name + "\x00" + component.Platform
		if componentKeys[key] {
			return fmt.Errorf("duplicate component %s for %s", component.Name, component.Platform)
		}
		componentKeys[key] = true
		if err := canonical.ValidateDigest(component.SHA256); err != nil {
			return fmt.Errorf("component %s/%s: %w", component.Name, component.Platform, err)
		}
		seenMirrors := map[string]bool{}
		for _, mirror := range component.Mirrors {
			if mirror == "" || seenMirrors[mirror] {
				return fmt.Errorf("component %s/%s has an empty or duplicate mirror", component.Name, component.Platform)
			}
			seenMirrors[mirror] = true
		}
	}
	if err := (contracts.Vector{Provides: d.Contracts}).Validate(); err != nil {
		return fmt.Errorf("contracts: %w", err)
	}
	return nil
}

func (d Document) unsigned() unsignedDocument {
	return unsignedDocument{
		Schema:                d.Schema,
		Identity:              d.Identity,
		AuthorityFingerprint:  d.AuthorityFingerprint,
		AllowedChannels:       d.AllowedChannels,
		ProjectDigest:         d.ProjectDigest,
		Operator:              d.Operator,
		CatalogSnapshotDigest: d.CatalogSnapshotDigest,
		RuntimeProfileDigests: d.RuntimeProfileDigests,
		SelectedDistribution:  d.SelectedDistribution,
		Components:            d.Components,
		Contracts:             d.Contracts,
	}
}

func selectDistribution(request Request) (catalog.Distribution, error) {
	if request.Identity == IdentityCandidate {
		if request.CandidateDigest == "" {
			return catalog.Distribution{}, fmt.Errorf("candidate resolution requires candidateDigest")
		}
		for _, distribution := range request.Catalog.Distributions {
			if distribution.State == catalog.StateCandidate && distribution.Digest == request.CandidateDigest {
				return distribution, nil
			}
		}
		return catalog.Distribution{}, fmt.Errorf("candidate distribution %s was not found", request.CandidateDigest)
	}
	if request.Identity != "" && request.Identity != IdentityProduction {
		return catalog.Distribution{}, fmt.Errorf("unknown resolution identity %q", request.Identity)
	}

	switch request.Operator.SelectionPolicy {
	case operator.SelectionExact:
		for _, distribution := range request.Catalog.Distributions {
			if distribution.Version != request.Operator.ServerRelease.Version ||
				distribution.Digest != request.Operator.ServerRelease.DistributionDigest ||
				distribution.Commit != request.Operator.ServerRelease.Commit {
				continue
			}
			if distribution.State == catalog.StateCandidate || distribution.State == catalog.StateRevoked {
				return catalog.Distribution{}, fmt.Errorf("exact server distribution %s has unusable state %s", distribution.Version, distribution.State)
			}
			return distribution, nil
		}
		return catalog.Distribution{}, fmt.Errorf("exact server distribution %s (%s) was not found", request.Operator.ServerRelease.Version, request.Operator.ServerRelease.DistributionDigest)
	case operator.SelectionLatestCompatible:
		candidates, err := request.Catalog.Production()
		if err != nil {
			return catalog.Distribution{}, err
		}
		var rejected []string
		for _, distribution := range candidates {
			if err := compatible(request.Project, request.Operator, distribution); err == nil {
				return distribution, nil
			} else {
				rejected = append(rejected, fmt.Sprintf("%s: %v", distribution.Version, err))
			}
		}
		return catalog.Distribution{}, fmt.Errorf("no certified compatible distribution: %v", rejected)
	default:
		return catalog.Distribution{}, fmt.Errorf("unknown operator selectionPolicy %q", request.Operator.SelectionPolicy)
	}
}

func compatible(product project.Document, target operator.Document, distribution catalog.Distribution) error {
	if err := contracts.Satisfy(distribution.Requires, target.Provides); err != nil {
		return err
	}
	available := append([]contracts.Capability(nil), distribution.Provides...)
	available = append(available, target.Provides...)
	return contracts.Satisfy(product.CapabilityFingerprint.RequiredContracts, available)
}

func selectComponents(product project.Document, operatorProfile operator.Document, distribution catalog.Distribution) ([]Component, error) {
	platforms := map[string]bool{"any": true, "all": true}
	for _, platform := range product.CapabilityFingerprint.OSArch {
		platforms[platform] = true
	}
	updaterHost := false
	for _, role := range product.Roles {
		if role == project.RoleUpdaterHost {
			updaterHost = true
		}
	}
	componentRegistry, err := vnextregistry.Default()
	if err != nil {
		return nil, err
	}
	required := map[string]bool{}
	targets := product.CapabilityFingerprint.OSArch
	if len(targets) == 0 {
		targets = []string{"all"}
	}
	for _, platform := range targets {
		names, err := componentRegistry.Required(
			platform,
			product.CapabilityFingerprint.Facades,
			product.CapabilityFingerprint.Projections,
			updaterHost,
		)
		if err != nil {
			return nil, err
		}
		for _, name := range names {
			definition, _ := componentRegistry.ByName(name)
			required[requiredComponentKey(definition, platform)] = true
		}
	}

	var result []Component
	found := map[string]bool{}
	for _, component := range distribution.Components {
		definition, registered := componentRegistry.ByName(component.Name)
		if !registered {
			continue
		}
		key := requiredComponentKey(definition, component.Platform)
		if !platforms[component.Platform] && component.Platform != "all" && component.Platform != "any" {
			continue
		}
		if !required[key] {
			continue
		}
		if component.Source.Kind != string(definition.Transport) {
			return nil, fmt.Errorf(
				"distribution %s component %s/%s uses transport %q, registry requires %q",
				distribution.Version, component.Name, component.Platform, component.Source.Kind, definition.Transport,
			)
		}
		locator := component.Source.URL
		if locator == "" {
			locator = component.Source.Module
		}
		result = append(result, Component{
			Name:     component.Name,
			Version:  component.Version,
			Platform: component.Platform,
			SHA256:   component.SHA256,
			Source:   component.Source.Kind,
			Locator:  locator,
			Mirrors:  mirrorLocators(operatorProfile.Sources.Mirrors, component.SHA256),
		})
		found[key] = true
	}
	var missing []string
	for key := range required {
		if !found[key] {
			name, platform, _ := strings.Cut(key, "\x00")
			missing = append(missing, name+"/"+platform)
		}
	}
	if len(missing) > 0 {
		slices.Sort(missing)
		return nil, fmt.Errorf("distribution %s lacks required components %v for declared platforms", distribution.Version, missing)
	}
	return result, nil
}

func requiredComponentKey(definition vnextregistry.Component, platform string) string {
	if slices.Contains(definition.Targets, "all") {
		return definition.Name + "\x00all"
	}
	return definition.Name + "\x00" + platform
}

func mirrorLocators(roots []string, digest string) []string {
	hash := strings.TrimPrefix(digest, canonical.DigestPrefix)
	result := make([]string, 0, len(roots))
	for _, root := range roots {
		if root == "" {
			continue
		}
		result = append(result, strings.TrimRight(root, "/")+"/blobs/"+hash)
	}
	return result
}

func matchingRootKeys(expected string, trusted map[string]ed25519.PublicKey) (map[string]ed25519.PublicKey, error) {
	matching := map[string]ed25519.PublicKey{}
	for keyID, publicKey := range trusted {
		fingerprint, err := trust.Fingerprint(publicKey)
		if err != nil {
			return nil, fmt.Errorf("trusted key %q: %w", keyID, err)
		}
		if fingerprint == expected {
			matching[keyID] = publicKey
		}
	}
	if len(matching) == 0 {
		return nil, fmt.Errorf("project trustFingerprint %s does not match any trusted Distribution Root", expected)
	}
	return matching, nil
}

func cloneMap[T any](source map[string]T) map[string]T {
	if len(source) == 0 {
		return nil
	}
	result := make(map[string]T, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}
