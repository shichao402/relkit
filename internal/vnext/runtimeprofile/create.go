package runtimeprofile

import (
	"fmt"

	"github.com/shichao402/relkit/internal/envelope"
	"github.com/shichao402/relkit/internal/model"
)

// Create builds a Runtime Profile signed by the server root, without publishing
// it. Requirement 10.4: the one server root signs every document the server
// emits, so a profile carries exactly one signature and needs no separate
// Runtime Profile Root.
//
// Every input is a server fact: the product id comes from registration, the
// channels are the server channel set, the entry URLs come from the server's
// publish configuration. Nothing is read from the host project, which is what
// lets the server sign a product's first Profile the moment it is registered.
func Create(
	product string,
	entryURLs []string,
	profileVersion int64,
	signer envelope.Signer,
) (*Document, error) {
	if err := model.CheckIdentifier(product, "product"); err != nil {
		return nil, err
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
		Product:         product,
		ProfileVersion:  profileVersion,
		AllowedChannels: model.StandardChannels(),
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
