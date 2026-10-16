package ops

import (
	"fmt"

	"github.com/shichao402/relkit/internal/envelope"
	"github.com/shichao402/relkit/internal/vnext/document"
	"github.com/shichao402/relkit/internal/vnext/project"
	"github.com/shichao402/relkit/internal/vnext/runtimeprofile"
)

type CreateRuntimeProfileOptions struct {
	ProjectPath    string
	EntryURLs      []string
	ProfileVersion int64
	KeyPaths       []string
	Output         string
	Replace        bool
}

func CreateRuntimeProfile(options CreateRuntimeProfileOptions) (*runtimeprofile.Document, error) {
	product, err := project.Load(options.ProjectPath)
	if err != nil {
		return nil, err
	}
	if len(options.KeyPaths) == 0 {
		return nil, fmt.Errorf("at least one product signing key is required")
	}
	signers := make([]envelope.Signer, 0, len(options.KeyPaths))
	seen := map[string]bool{}
	for _, path := range options.KeyPaths {
		keyID, seed, err := loadSigningKey(path)
		if err != nil {
			return nil, err
		}
		if seen[keyID] {
			return nil, fmt.Errorf("duplicate product signing key %q", keyID)
		}
		seen[keyID] = true
		signers = append(signers, envelope.Signer{KeyID: keyID, Seed: seed})
	}
	value, err := runtimeprofile.Create(
		*product,
		options.EntryURLs,
		options.ProfileVersion,
		signers,
	)
	if err != nil {
		return nil, err
	}
	if options.Output == "" {
		return nil, fmt.Errorf("Runtime Profile output path is required")
	}
	if options.Replace {
		err = document.WriteAtomic(options.Output, value)
	} else {
		err = document.WriteNew(options.Output, value)
	}
	if err != nil {
		return nil, err
	}
	return value, nil
}
