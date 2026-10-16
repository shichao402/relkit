package ops

import (
	"fmt"

	"github.com/shichao402/relkit/internal/vnext/document"
	"github.com/shichao402/relkit/internal/vnext/operator"
)

type SignOperatorOptions struct {
	Input   string
	Output  string
	KeyPath string
	Add     bool
}

func SignOperator(options SignOperatorOptions) (*operator.Document, error) {
	var value operator.Document
	if err := document.Load(options.Input, &value); err != nil {
		return nil, err
	}
	if options.Output == "" {
		return nil, fmt.Errorf("operator output path is required")
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
