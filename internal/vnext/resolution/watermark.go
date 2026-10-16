package resolution

import (
	"fmt"
	"os"

	"github.com/shichao402/relkit/internal/vnext/canonical"
	"github.com/shichao402/relkit/internal/vnext/document"
)

const WatermarkSchema = "relkit.control-plane-watermarks/1"

type Watermark struct {
	Sequence int64  `json:"sequence"`
	Digest   string `json:"digest"`
}

type Watermarks struct {
	Schema    string               `json:"schema"`
	Operators map[string]Watermark `json:"operators,omitempty"`
	Catalogs  map[string]Watermark `json:"catalogs,omitempty"`
}

func LoadWatermarks(path string) (*Watermarks, error) {
	value := &Watermarks{
		Schema:    WatermarkSchema,
		Operators: map[string]Watermark{},
		Catalogs:  map[string]Watermark{},
	}
	if err := document.Load(path, value); err != nil {
		if os.IsNotExist(err) {
			return value, nil
		}
		return nil, err
	}
	if value.Operators == nil {
		value.Operators = map[string]Watermark{}
	}
	if value.Catalogs == nil {
		value.Catalogs = map[string]Watermark{}
	}
	if err := value.Validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return value, nil
}

func (w Watermarks) Validate() error {
	if w.Schema != WatermarkSchema {
		return fmt.Errorf("schema is %q, expected %q", w.Schema, WatermarkSchema)
	}
	for key, point := range w.Operators {
		if key == "" {
			return fmt.Errorf("operator watermark has empty key")
		}
		if err := point.validate(); err != nil {
			return fmt.Errorf("operator %q: %w", key, err)
		}
	}
	for key, point := range w.Catalogs {
		if key == "" {
			return fmt.Errorf("catalog watermark has empty key")
		}
		if err := point.validate(); err != nil {
			return fmt.Errorf("catalog %q: %w", key, err)
		}
	}
	return nil
}

func (w Watermark) validate() error {
	if w.Sequence < 1 {
		return fmt.Errorf("sequence must be positive")
	}
	if err := canonical.ValidateDigest(w.Digest); err != nil {
		return fmt.Errorf("digest: %w", err)
	}
	return nil
}

func (w *Watermarks) AdvanceOperator(deploymentID string, sequence int64, digest string) error {
	if w.Operators == nil {
		w.Operators = map[string]Watermark{}
	}
	return advance(w.Operators, deploymentID, sequence, digest, "Operator Profile")
}

func (w *Watermarks) AdvanceCatalog(rootFingerprint string, sequence int64, digest string) error {
	if w.Catalogs == nil {
		w.Catalogs = map[string]Watermark{}
	}
	return advance(w.Catalogs, rootFingerprint, sequence, digest, "Distribution Catalog")
}

func advance(target map[string]Watermark, key string, sequence int64, digest string, label string) error {
	if key == "" {
		return fmt.Errorf("%s watermark key must not be empty", label)
	}
	next := Watermark{Sequence: sequence, Digest: digest}
	if err := next.validate(); err != nil {
		return fmt.Errorf("%s: %w", label, err)
	}
	if current, ok := target[key]; ok {
		if sequence < current.Sequence {
			return fmt.Errorf("%s replay: sequence %d is older than observed %d", label, sequence, current.Sequence)
		}
		if sequence == current.Sequence && digest != current.Digest {
			return fmt.Errorf("%s equivocation: sequence %d changed digest from %s to %s", label, sequence, current.Digest, digest)
		}
	}
	target[key] = next
	return nil
}

func (w Watermarks) Write(path string) error {
	if err := w.Validate(); err != nil {
		return err
	}
	return document.WriteAtomic(path, w)
}
