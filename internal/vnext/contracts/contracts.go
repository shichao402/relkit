// Package contracts models the small set of versioned protocols that may
// vary independently of a Relkit release.
package contracts

import (
	"fmt"
	"sort"
)

type Domain string

const (
	DomainRUPSchemaSet    Domain = "rupSchemaSet"
	DomainPublishProtocol Domain = "publishProtocol"
	DomainUpdaterIPC      Domain = "updaterIpc"
	DomainArtifactSet     Domain = "artifactSet"
)

// Capability is either an identifier contract (RUP Schema Set and
// ArtifactSet) or an inclusive integer window (Publish Protocol and Updater
// IPC). Peer scopes the requirement and prevents unrelated capabilities from
// satisfying it accidentally.
type Capability struct {
	Domain Domain `json:"domain"`
	Peer   string `json:"peer"`
	ID     string `json:"id,omitempty"`
	Min    int    `json:"min,omitempty"`
	Max    int    `json:"max,omitempty"`
}

type Vector struct {
	Requires []Capability `json:"requires,omitempty"`
	Provides []Capability `json:"provides,omitempty"`
}

func (c Capability) Validate() error {
	if c.Peer == "" {
		return fmt.Errorf("%s capability has no peer", c.Domain)
	}
	switch c.Domain {
	case DomainRUPSchemaSet, DomainArtifactSet:
		if c.ID == "" {
			return fmt.Errorf("%s capability for peer %q has no id", c.Domain, c.Peer)
		}
		if c.Min != 0 || c.Max != 0 {
			return fmt.Errorf("%s capability for peer %q must use id, not a version window", c.Domain, c.Peer)
		}
	case DomainPublishProtocol, DomainUpdaterIPC:
		if c.ID != "" {
			return fmt.Errorf("%s capability for peer %q must use a version window, not id", c.Domain, c.Peer)
		}
		if c.Min <= 0 || c.Max < c.Min {
			return fmt.Errorf("%s capability for peer %q has invalid inclusive window [%d,%d]", c.Domain, c.Peer, c.Min, c.Max)
		}
	default:
		return fmt.Errorf("unknown contract domain %q", c.Domain)
	}
	return nil
}

func (v Vector) Validate() error {
	for _, group := range []struct {
		name string
		rows []Capability
	}{
		{name: "requires", rows: v.Requires},
		{name: "provides", rows: v.Provides},
	} {
		seen := map[string]struct{}{}
		for i, row := range group.rows {
			if err := row.Validate(); err != nil {
				return fmt.Errorf("%s[%d]: %w", group.name, i, err)
			}
			key := string(row.Domain) + "\x00" + row.Peer
			if _, ok := seen[key]; ok {
				return fmt.Errorf("%s contains duplicate %s capability for peer %q", group.name, row.Domain, row.Peer)
			}
			seen[key] = struct{}{}
		}
	}
	return nil
}

// Compatible reports whether two capabilities describe the same scoped
// contract and have a non-empty compatibility intersection.
func Compatible(requirement, provided Capability) bool {
	if requirement.Domain != provided.Domain || requirement.Peer != provided.Peer {
		return false
	}
	switch requirement.Domain {
	case DomainRUPSchemaSet, DomainArtifactSet:
		return requirement.ID != "" && requirement.ID == provided.ID
	case DomainPublishProtocol, DomainUpdaterIPC:
		return requirement.Min <= provided.Max && provided.Min <= requirement.Max
	default:
		return false
	}
}

// Satisfy checks every requirement against the provided vector. It returns a
// deterministic list of unsatisfied scoped contracts.
func Satisfy(requirements, provided []Capability) error {
	var missing []string
	for _, requirement := range requirements {
		found := false
		for _, candidate := range provided {
			if Compatible(requirement, candidate) {
				found = true
				break
			}
		}
		if !found {
			missing = append(missing, describe(requirement))
		}
	}
	if len(missing) == 0 {
		return nil
	}
	sort.Strings(missing)
	return fmt.Errorf("unsatisfied contracts: %v", missing)
}

func describe(c Capability) string {
	if c.ID != "" {
		return fmt.Sprintf("%s[%s]=%s", c.Domain, c.Peer, c.ID)
	}
	return fmt.Sprintf("%s[%s]=[%d,%d]", c.Domain, c.Peer, c.Min, c.Max)
}
