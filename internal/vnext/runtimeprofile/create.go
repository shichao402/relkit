package runtimeprofile

import (
	"fmt"

	"github.com/shichao402/relkit/internal/envelope"
	"github.com/shichao402/relkit/internal/vnext/project"
)

// Create builds a Runtime Profile signed by the server root, without publishing
// it. Requirement 10.4: the one server root signs every document the server
// emits, so a profile carries exactly one signature and needs no separate
// Runtime Profile Root.
func Create(
	product project.Document,
	entryURLs []string,
	profileVersion int64,
	signer envelope.Signer,
) (*Document, error) {
	if err := product.Validate(); err != nil {
		return nil, fmt.Errorf("project: %w", err)
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
	if signer.KeyID == "" {
		return nil, fmt.Errorf("Runtime Profile requires the server root signer")
	}
	value := &Document{
		Schema:          Schema,
		Product:         product.Product,
		ProfileVersion:  profileVersion,
		AllowedChannels: append([]string(nil), product.Channels...),
		DirectoryBootstrap: DirectoryBootstrap{
			EntryURLs: append([]string(nil), entryURLs...),
		},
		RUPSchemaSet: SchemaSet,
	}
	if err := value.Sign(signer.KeyID, signer.Seed); err != nil {
		return nil, err
	}
	return value, nil
}
