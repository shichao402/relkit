package onboard

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"cnb.cool/shichao402/relkit/internal/config"
	"cnb.cool/shichao402/relkit/internal/jsonio"
)

const (
	SchemaContract   = "relkit.client-contract/1"
	ContractFileName = "client-contract.json"
)

// ClientContract is the host-embeddable snapshot: product, keys, entryUrls, recovery.
type ClientContract struct {
	Schema         string                 `json:"schema"`
	Product        string                 `json:"product"`
	DefaultChannel string                 `json:"defaultChannel"`
	PublicKeys     []any                  `json:"publicKeys"`
	EntryURLs      []string               `json:"entryUrls"`
	Recovery       *config.RecoveryConfig `json:"recovery"`
}

func ContractPath(hostRoot string) string {
	return filepath.Join(RelkitDir(hostRoot), ContractFileName)
}

func BuildClientContract(cfg *config.Config) (*ClientContract, error) {
	if cfg == nil {
		return nil, fmt.Errorf("relkit.json is not loaded")
	}
	if cfg.Recovery == nil {
		return nil, fmt.Errorf("recovery is required for the client contract")
	}
	rawKeys, _ := cfg.Signing["publicKeys"].([]any)
	if len(rawKeys) == 0 {
		return nil, fmt.Errorf("signing.publicKeys is required for the client contract")
	}
	entry := []string{}
	if cfg.Directory != nil {
		entry = append(entry, cfg.Directory.EntryURLs...)
	}
	if len(entry) == 0 {
		return nil, fmt.Errorf("directory.entryUrls is required for the client contract")
	}
	return &ClientContract{
		Schema:         SchemaContract,
		Product:        cfg.Product,
		DefaultChannel: cfg.DefaultChannel,
		PublicKeys:     rawKeys,
		EntryURLs:      entry,
		Recovery:       cfg.Recovery,
	}, nil
}

func WriteClientContract(hostRoot string, cfg *config.Config) error {
	doc, err := BuildClientContract(cfg)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(RelkitDir(hostRoot), 0o755); err != nil {
		return err
	}
	raw, err := jsonio.MarshalPretty(doc)
	if err != nil {
		return err
	}
	return os.WriteFile(ContractPath(hostRoot), raw, 0o644)
}

func ClientContractMatches(hostRoot string, cfg *config.Config) (bool, string) {
	want, err := BuildClientContract(cfg)
	if err != nil {
		return false, err.Error()
	}
	data, err := os.ReadFile(ContractPath(hostRoot))
	if err != nil {
		return false, err.Error()
	}
	wantRaw, err := jsonio.MarshalPretty(want)
	if err != nil {
		return false, err.Error()
	}
	var got, wantNorm any
	if err := json.Unmarshal(data, &got); err != nil {
		return false, err.Error()
	}
	if err := json.Unmarshal(wantRaw, &wantNorm); err != nil {
		return false, err.Error()
	}
	gotRaw, _ := json.Marshal(got)
	normRaw, _ := json.Marshal(wantNorm)
	if !bytes.Equal(gotRaw, normRaw) {
		return false, "client-contract.json does not match relkit.json"
	}
	return true, "client-contract.json matches relkit.json"
}
