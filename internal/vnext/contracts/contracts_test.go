package contracts

import (
	"strings"
	"testing"
)

func TestCompatibleUsesScopedInclusiveIntersection(t *testing.T) {
	requirement := Capability{Domain: DomainPublishProtocol, Peer: "agent", Min: 2, Max: 3}
	if !Compatible(requirement, Capability{Domain: DomainPublishProtocol, Peer: "agent", Min: 3, Max: 4}) {
		t.Fatal("inclusive boundary should intersect")
	}
	if Compatible(requirement, Capability{Domain: DomainPublishProtocol, Peer: "store", Min: 3, Max: 4}) {
		t.Fatal("different peer must not satisfy requirement")
	}
	if Compatible(requirement, Capability{Domain: DomainPublishProtocol, Peer: "agent", Min: 4, Max: 5}) {
		t.Fatal("disjoint windows must not be compatible")
	}
}

func TestCompatibleIdentifierContracts(t *testing.T) {
	requirement := Capability{Domain: DomainRUPSchemaSet, Peer: "directory", ID: "rup.v2"}
	if !Compatible(requirement, Capability{Domain: DomainRUPSchemaSet, Peer: "directory", ID: "rup.v2"}) {
		t.Fatal("matching schema set should be compatible")
	}
	if Compatible(requirement, Capability{Domain: DomainRUPSchemaSet, Peer: "directory", ID: "rup.v3"}) {
		t.Fatal("different schema sets must not be compatible")
	}
}

func TestVectorRejectsBareAndDuplicateCapabilities(t *testing.T) {
	vector := Vector{
		Requires: []Capability{
			{Domain: DomainUpdaterIPC, Min: 3, Max: 3},
		},
	}
	if err := vector.Validate(); err == nil || !strings.Contains(err.Error(), "no peer") {
		t.Fatalf("expected peer validation error, got %v", err)
	}

	vector = Vector{
		Provides: []Capability{
			{Domain: DomainArtifactSet, Peer: "producer", ID: "relkit.artifact-set/1"},
			{Domain: DomainArtifactSet, Peer: "producer", ID: "relkit.artifact-set/1"},
		},
	}
	if err := vector.Validate(); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("expected duplicate validation error, got %v", err)
	}
}

func TestSatisfyReportsMissingDeterministically(t *testing.T) {
	requirements := []Capability{
		{Domain: DomainUpdaterIPC, Peer: "facade", Min: 3, Max: 3},
		{Domain: DomainRUPSchemaSet, Peer: "directory", ID: "rup.v2"},
	}
	provided := []Capability{
		{Domain: DomainUpdaterIPC, Peer: "facade", Min: 3, Max: 3},
	}
	err := Satisfy(requirements, provided)
	if err == nil || !strings.Contains(err.Error(), "rupSchemaSet[directory]=rup.v2") {
		t.Fatalf("unexpected error: %v", err)
	}
}
