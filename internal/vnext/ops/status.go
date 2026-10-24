package ops

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/shichao402/relkit/internal/vnext/document"
	"github.com/shichao402/relkit/internal/vnext/operator"
)

type Status struct {
	Schema               string            `json:"schema"`
	OK                   bool              `json:"ok"`
	Operator             string            `json:"operator"`
	ProfileDigest        string            `json:"profileDigest"`
	Profile              operator.Document `json:"profile"`
	CatalogDigest        string            `json:"catalogDigest"`
	CatalogSequence      int64             `json:"catalogSequence"`
	CatalogCurrent       string            `json:"catalogCurrent"`
	UpgradeAvailable     bool              `json:"upgradeAvailable"`
	IncompleteOperations []OperationState  `json:"incompleteOperations,omitempty"`
}

func ReadStatus(inventory Inventory) (*Status, error) {
	profile, value, err := loadTrustedState(inventory)
	if err != nil {
		return nil, operationError("operator.control_plane_invalid", "status", "Operator control plane is not trusted", nil, err)
	}
	profileDigest, err := profile.Digest()
	if err != nil {
		return nil, err
	}
	catalogDigest, err := value.Digest()
	if err != nil {
		return nil, err
	}
	current, err := value.CurrentRelease()
	if err != nil {
		return nil, err
	}
	incomplete, err := readIncompleteOperations(inventory.StateDirectory)
	if err != nil {
		return nil, err
	}
	status := &Status{
		Schema:               "relkit.ops-status/2",
		OK:                   len(incomplete) == 0,
		Operator:             inventory.Operator,
		ProfileDigest:        profileDigest,
		Profile:              *profile,
		CatalogDigest:        catalogDigest,
		CatalogSequence:      value.Sequence,
		CatalogCurrent:       current.Version,
		IncompleteOperations: incomplete,
	}
	status.UpgradeAvailable = current.Version != profile.ServerRelease.Version ||
		current.Commit != profile.ServerRelease.Commit
	return status, nil
}

func readIncompleteOperations(stateDirectory string) ([]OperationState, error) {
	directory := filepath.Join(stateDirectory, "operations")
	entries, err := os.ReadDir(directory)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var result []OperationState
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		var state OperationState
		path := filepath.Join(directory, entry.Name())
		if err := loadOperationState(path, &state); err != nil {
			// Staged profile documents share this directory and end in
			// .profile.json. Ignore those, but fail closed for journal damage.
			if filepath.Ext(strings.TrimSuffix(entry.Name(), ".json")) == ".profile" {
				continue
			}
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		if state.Status != "complete" {
			result = append(result, state)
		}
	}
	sort.Slice(result, func(left, right int) bool {
		return result[left].StartedAt < result[right].StartedAt
	})
	return result, nil
}

func loadOperationState(path string, state *OperationState) error {
	if err := document.Load(path, state); err != nil {
		return err
	}
	if state.Schema != OperationStateSchema || state.OperationID == "" || state.Status == "" {
		return fmt.Errorf("invalid operation state")
	}
	return nil
}
