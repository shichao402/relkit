package ops

import (
	"fmt"

	"github.com/shichao402/relkit/internal/vnext/catalog"
	"github.com/shichao402/relkit/internal/vnext/document"
)

type CatalogAuthority string

const (
	AuthorityProduction CatalogAuthority = "production"
	AuthorityCandidate  CatalogAuthority = "candidate"
)

type SignCatalogOptions struct {
	Input     string
	Output    string
	KeyPath   string
	Add       bool
	Authority CatalogAuthority
}

func SignCatalog(options SignCatalogOptions) (*catalog.Document, error) {
	var value catalog.Document
	if err := document.Load(options.Input, &value); err != nil {
		return nil, err
	}
	if options.Output == "" {
		return nil, fmt.Errorf("catalog output path is required")
	}
	if err := validateCatalogAuthority(value, options.Authority); err != nil {
		return nil, err
	}
	keyID, seed, err := loadSigningKey(options.KeyPath)
	if err != nil {
		return nil, err
	}
	if options.Add {
		if err := value.AddSignature(keyID, seed); err != nil {
			return nil, err
		}
	} else if err := value.Sign(keyID, seed); err != nil {
		return nil, err
	}
	if err := document.WriteAtomic(options.Output, value); err != nil {
		return nil, err
	}
	return &value, nil
}

type TransitionCatalogOptions struct {
	Input                   string
	Output                  string
	Distribution            string
	State                   catalog.State
	KeyPath                 string
	TrustKeyPaths           []string
	Authority               CatalogAuthority
	CertificationReportPath string
	ImpactPath              string
	FleetPath               string
	ProductionEscapeDigest  string
	ProductionEscapeCommit  string
}

func TransitionCatalog(options TransitionCatalogOptions) (*catalog.Document, error) {
	value, err := catalog.Load(options.Input)
	if err != nil {
		return nil, err
	}
	trusted, err := loadTrustKeys(options.TrustKeyPaths)
	if err != nil {
		return nil, err
	}
	if err := value.Verify(trusted); err != nil {
		return nil, fmt.Errorf("input catalog: %w", err)
	}
	if options.Distribution == "" {
		return nil, fmt.Errorf("distribution digest is required")
	}
	if options.Output == "" {
		return nil, fmt.Errorf("catalog output path is required")
	}
	var certifiedImpact *Impact
	if options.State == catalog.StateCertified {
		if options.CertificationReportPath == "" {
			return nil, fmt.Errorf("candidate certification report is required before certified transition")
		}
		if options.ImpactPath == "" {
			return nil, fmt.Errorf("candidate impact manifest is required before certified transition")
		}
		if options.FleetPath == "" {
			return nil, fmt.Errorf("fleet registration is required before certified transition")
		}
		if options.ProductionEscapeCommit == "" {
			return nil, fmt.Errorf("audited production escape source commit is required before certified transition")
		}
		var report CertificationReport
		if err := document.Load(options.CertificationReportPath, &report); err != nil {
			return nil, err
		}
		impact, err := LoadImpact(options.ImpactPath)
		if err != nil {
			return nil, err
		}
		if impact.CandidateDigest != options.Distribution {
			return nil, fmt.Errorf("impact manifest does not match candidate %s", options.Distribution)
		}
		if options.ProductionEscapeCommit != impact.CandidateCommit {
			return nil, fmt.Errorf("catalog transition source commit does not match the certified candidate")
		}
		if err := impact.ValidateAgainstCatalog(*value); err != nil {
			return nil, fmt.Errorf("impact manifest: %w", err)
		}
		fleet, err := LoadFleet(options.FleetPath)
		if err != nil {
			return nil, err
		}
		if err := report.ValidateForImpactAndFleet(*impact, *fleet, options.ProductionEscapeDigest); err != nil {
			return nil, err
		}
		certifiedImpact = impact
	}
	found := false
	for index := range value.Distributions {
		distribution := &value.Distributions[index]
		if distribution.Digest != options.Distribution {
			continue
		}
		if found {
			return nil, fmt.Errorf("catalog contains duplicate distribution digest %s", options.Distribution)
		}
		if err := catalog.ValidateTransition(distribution.State, options.State); err != nil {
			return nil, err
		}
		if options.State == catalog.StateCertified {
			if distribution.Commit != certifiedImpact.CandidateCommit {
				return nil, fmt.Errorf(
					"candidate commit %s does not match certified impact commit %s",
					distribution.Commit,
					certifiedImpact.CandidateCommit,
				)
			}
		}
		distribution.State = options.State
		found = true
	}
	if !found {
		return nil, fmt.Errorf("distribution %s is not present in catalog", options.Distribution)
	}
	value.Sequence++
	for index := range value.Distributions {
		distribution := &value.Distributions[index]
		if distribution.Digest == options.Distribution && options.State == catalog.StateCertified {
			distribution.CertificationSequence = value.Sequence
		}
	}
	if err := validateCatalogAuthority(*value, options.Authority); err != nil {
		return nil, err
	}
	keyID, seed, err := loadSigningKey(options.KeyPath)
	if err != nil {
		return nil, err
	}
	if err := value.Sign(keyID, seed); err != nil {
		return nil, err
	}
	if err := document.WriteAtomic(options.Output, value); err != nil {
		return nil, err
	}
	return value, nil
}

func validateCatalogAuthority(value catalog.Document, authority CatalogAuthority) error {
	switch authority {
	case AuthorityProduction:
		return nil
	case AuthorityCandidate:
		for _, distribution := range value.Distributions {
			if distribution.State != catalog.StateCandidate && distribution.State != catalog.StateRevoked {
				return fmt.Errorf("candidate authority cannot sign distribution %s in state %s", distribution.Digest, distribution.State)
			}
		}
		return nil
	default:
		return fmt.Errorf("catalog authority must be %q or %q", AuthorityProduction, AuthorityCandidate)
	}
}

type PutDistributionOptions struct {
	CatalogPath   string
	Distribution  string
	Output        string
	KeyPath       string
	TrustKeyPaths []string
	Authority     CatalogAuthority
}

func PutDistribution(options PutDistributionOptions) (*catalog.Document, error) {
	var distribution catalog.Distribution
	if err := document.Load(options.Distribution, &distribution); err != nil {
		return nil, err
	}
	identityDigest, err := distribution.IdentityDigest()
	if err != nil {
		return nil, err
	}
	if distribution.Digest != identityDigest {
		return nil, fmt.Errorf(
			"distribution digest %s does not match immutable identity %s",
			distribution.Digest,
			identityDigest,
		)
	}
	var value catalog.Document
	if options.CatalogPath == "" {
		value = catalog.Document{
			Schema:        catalog.Schema,
			Sequence:      1,
			Distributions: []catalog.Distribution{distribution},
		}
	} else {
		current, err := catalog.Load(options.CatalogPath)
		if err != nil {
			return nil, err
		}
		trusted, err := loadTrustKeys(options.TrustKeyPaths)
		if err != nil {
			return nil, err
		}
		if err := current.Verify(trusted); err != nil {
			return nil, fmt.Errorf("input catalog: %w", err)
		}
		for _, existing := range current.Distributions {
			if existing.Version == distribution.Version {
				return nil, fmt.Errorf("distribution version %s already exists; immutable corrections require a new version", distribution.Version)
			}
			if existing.Digest == distribution.Digest {
				return nil, fmt.Errorf("distribution digest %s already exists", distribution.Digest)
			}
		}
		value = *current
		value.Sequence++
		value.Distributions = append(value.Distributions, distribution)
	}
	if err := validateCatalogAuthority(value, options.Authority); err != nil {
		return nil, err
	}
	keyID, seed, err := loadSigningKey(options.KeyPath)
	if err != nil {
		return nil, err
	}
	if err := value.Sign(keyID, seed); err != nil {
		return nil, err
	}
	if options.Output == "" {
		return nil, fmt.Errorf("catalog output path is required")
	}
	// The Catalog is a mutable signed pointer. Distribution assets are
	// immutable and are never overwritten by this operation.
	if err := document.WriteAtomic(options.Output, value); err != nil {
		return nil, err
	}
	return &value, nil
}
