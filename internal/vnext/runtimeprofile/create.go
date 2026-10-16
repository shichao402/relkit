package runtimeprofile

import (
	"fmt"

	"github.com/shichao402/relkit/internal/envelope"
	"github.com/shichao402/relkit/internal/vnext/project"
	"github.com/shichao402/relkit/internal/vnext/trust"
)

// Create builds a signed Runtime Profile without publishing it. This is the
// bootstrap path used before a self-subscribing product can produce its first
// vNext release.
func Create(
	product project.Document,
	entryURLs []string,
	profileVersion int64,
	signers []envelope.Signer,
) (*Document, error) {
	if product.RuntimeProfilePublication == nil {
		return nil, fmt.Errorf("project does not publish a Runtime Profile")
	}
	if len(entryURLs) == 0 {
		return nil, fmt.Errorf("Runtime Profile requires consumer-visible directory entry URLs")
	}
	for _, entryURL := range entryURLs {
		if entryURL == "" {
			return nil, fmt.Errorf("Runtime Profile directory entry URLs must not be empty")
		}
	}
	if profileVersion < 1 {
		return nil, fmt.Errorf("Runtime Profile version must be positive")
	}
	if len(signers) == 0 {
		return nil, fmt.Errorf("Runtime Profile requires at least one signer")
	}
	matchesPinnedRoot := false
	for _, signer := range signers {
		fingerprint, err := trust.Fingerprint(signer.PublicKey())
		if err != nil {
			return nil, err
		}
		if fingerprint == product.RuntimeProfilePublication.RootFingerprint {
			matchesPinnedRoot = true
		}
	}
	if !matchesPinnedRoot {
		return nil, fmt.Errorf(
			"none of the product signers match Runtime Profile root fingerprint %s",
			product.RuntimeProfilePublication.RootFingerprint,
		)
	}
	value := &Document{
		Schema:          Schema,
		Product:         product.Product,
		ProfileVersion:  profileVersion,
		AllowedChannels: append([]string(nil), product.Channels...),
		DirectoryBootstrap: DirectoryBootstrap{
			EntryURLs: append([]string(nil), entryURLs...),
			Recovery:  cloneAnyMap(product.RuntimeProfilePublication.Recovery),
		},
		RUPSchemaSet: product.RuntimeProfilePublication.RUPSchemaSet,
	}
	if err := value.Sign(signers[0].KeyID, signers[0].Seed); err != nil {
		return nil, err
	}
	for _, signer := range signers[1:] {
		if err := value.AddSignature(signer.KeyID, signer.Seed); err != nil {
			return nil, err
		}
	}
	return value, nil
}
