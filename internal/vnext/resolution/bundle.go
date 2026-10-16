package resolution

import (
	"bytes"
	"crypto/ed25519"
	"fmt"

	"github.com/shichao402/relkit/internal/vnext/canonical"
	"github.com/shichao402/relkit/internal/vnext/catalog"
	"github.com/shichao402/relkit/internal/vnext/document"
	"github.com/shichao402/relkit/internal/vnext/operator"
	"github.com/shichao402/relkit/internal/vnext/project"
	"github.com/shichao402/relkit/internal/vnext/runtimeprofile"
	"github.com/shichao402/relkit/internal/vnext/trust"
)

const BundleSchema = "relkit.resolution-bundle/1"

type PublicKey struct {
	KeyID string `json:"keyId"`
	Key   []byte `json:"key"`
}

type Bundle struct {
	Schema            string                             `json:"schema"`
	Project           project.Document                   `json:"project"`
	Operator          operator.Document                  `json:"operator"`
	Catalog           catalog.Document                   `json:"catalog"`
	RuntimeProfiles   map[string]runtimeprofile.Document `json:"runtimeProfiles,omitempty"`
	DistributionRoots []PublicKey                        `json:"distributionRoots"`
	CandidateRoots    []PublicKey                        `json:"candidateRoots,omitempty"`
	RuntimeRoots      map[string][]PublicKey             `json:"runtimeRoots,omitempty"`
	Resolution        Document                           `json:"resolution"`
}

type BundlePolicy struct {
	DistributionRootFingerprint string
	CandidateRootFingerprint    string
	OperatorDeploymentID        string
	Identity                    Identity
}

func LoadBundle(path string) (*Bundle, error) {
	var value Bundle
	if err := document.Load(path, &value); err != nil {
		return nil, err
	}
	return &value, nil
}

func (b Bundle) Verify(policy BundlePolicy) (*Document, error) {
	if b.Schema != BundleSchema {
		return nil, fmt.Errorf("bundle schema is %q, expected %q", b.Schema, BundleSchema)
	}
	distributionRoots, err := publicKeyMap(b.DistributionRoots)
	if err != nil {
		return nil, fmt.Errorf("distribution roots: %w", err)
	}
	candidateRoots := map[string]ed25519.PublicKey{}
	if len(b.CandidateRoots) > 0 {
		candidateRoots, err = publicKeyMap(b.CandidateRoots)
		if err != nil {
			return nil, fmt.Errorf("candidate roots: %w", err)
		}
	}
	runtimeRoots := make(map[string]map[string]ed25519.PublicKey, len(b.RuntimeRoots))
	for product, keys := range b.RuntimeRoots {
		value, err := publicKeyMap(keys)
		if err != nil {
			return nil, fmt.Errorf("runtime roots for %q: %w", product, err)
		}
		runtimeRoots[product] = value
	}
	if policy.DistributionRootFingerprint != "" && b.Project.TrustFingerprint != policy.DistributionRootFingerprint {
		return nil, fmt.Errorf("project Distribution Root %s does not match agent policy %s", b.Project.TrustFingerprint, policy.DistributionRootFingerprint)
	}
	if policy.OperatorDeploymentID != "" && b.Operator.DeploymentID != policy.OperatorDeploymentID {
		return nil, fmt.Errorf("Operator deployment %q does not match agent policy %q", b.Operator.DeploymentID, policy.OperatorDeploymentID)
	}
	if policy.Identity != "" && b.Resolution.Identity != policy.Identity {
		return nil, fmt.Errorf("resolution identity %q does not match agent lane %q", b.Resolution.Identity, policy.Identity)
	}
	if b.Resolution.Identity == IdentityCandidate {
		if policy.CandidateRootFingerprint == "" {
			return nil, fmt.Errorf("candidate bundle policy has no Candidate Root fingerprint")
		}
		if b.Resolution.AuthorityFingerprint != policy.CandidateRootFingerprint {
			return nil, fmt.Errorf("candidate authority %s does not match agent policy %s", b.Resolution.AuthorityFingerprint, policy.CandidateRootFingerprint)
		}
	}
	request := Request{
		Project:                  b.Project,
		Operator:                 b.Operator,
		Catalog:                  b.Catalog,
		TrustedDistributionKeys:  distributionRoots,
		TrustedCandidateKeys:     candidateRoots,
		CandidateRootFingerprint: policy.CandidateRootFingerprint,
		RuntimeProfiles:          b.RuntimeProfiles,
		TrustedRuntimeRoots:      runtimeRoots,
		Identity:                 b.Resolution.Identity,
		MinimumProfileSequence:   b.Resolution.Operator.Sequence,
	}
	if b.Resolution.Identity == IdentityCandidate {
		request.CandidateDigest = b.Resolution.SelectedDistribution.Digest
	}
	computed, err := Resolve(request)
	if err != nil {
		return nil, err
	}
	if err := b.Resolution.Validate(); err != nil {
		return nil, err
	}
	if computed.Digest != b.Resolution.Digest {
		return nil, fmt.Errorf("bundled resolution digest %s does not match verified inputs %s", b.Resolution.Digest, computed.Digest)
	}
	left, err := canonical.Bytes(computed)
	if err != nil {
		return nil, err
	}
	right, err := canonical.Bytes(b.Resolution)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(left, right) {
		return nil, fmt.Errorf("bundled resolution fields differ from verified resolver output")
	}
	return computed, nil
}

func publicKeyMap(values []PublicKey) (map[string]ed25519.PublicKey, error) {
	if len(values) == 0 {
		return nil, fmt.Errorf("public key set must not be empty")
	}
	result := make(map[string]ed25519.PublicKey, len(values))
	for _, value := range values {
		if value.KeyID == "" || len(value.Key) != ed25519.PublicKeySize {
			return nil, fmt.Errorf("invalid public key entry %q", value.KeyID)
		}
		if _, exists := result[value.KeyID]; exists {
			return nil, fmt.Errorf("duplicate public key id %q", value.KeyID)
		}
		result[value.KeyID] = append(ed25519.PublicKey(nil), value.Key...)
	}
	return result, nil
}

func PublicKeys(values map[string]ed25519.PublicKey) []PublicKey {
	result := make([]PublicKey, 0, len(values))
	for keyID, key := range values {
		result = append(result, PublicKey{KeyID: keyID, Key: append([]byte(nil), key...)})
	}
	// Canonical JSON sorts object keys but not arrays. Keep bundle construction
	// deterministic regardless of map iteration.
	for index := 1; index < len(result); index++ {
		for position := index; position > 0 && result[position].KeyID < result[position-1].KeyID; position-- {
			result[position], result[position-1] = result[position-1], result[position]
		}
	}
	return result
}

func (b Bundle) DistributionRootFingerprints() ([]string, error) {
	keys, err := publicKeyMap(b.DistributionRoots)
	if err != nil {
		return nil, err
	}
	result := make([]string, 0, len(keys))
	for _, key := range keys {
		fingerprint, err := trust.Fingerprint(key)
		if err != nil {
			return nil, err
		}
		result = append(result, fingerprint)
	}
	return result, nil
}
