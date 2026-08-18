package publish

import (
	"testing"

	"cnb.cool/shichao402/relkit/internal/model"
)

func TestApplyRetainVersionsUnlimited(t *testing.T) {
	nodes := []*model.VersionNode{
		{Version: "1.0.0", Code: 100},
		{Version: "2.0.0", Code: 200},
	}
	retained, pruned := applyRetainVersions(nodes, 0)
	if len(retained) != 2 || len(pruned) != 0 {
		t.Fatalf("retain=0: retained=%d pruned=%d", len(retained), len(pruned))
	}
}

func TestApplyRetainVersionsKeepsHighestCodes(t *testing.T) {
	nodes := []*model.VersionNode{
		{Version: "1.0.0", Code: 100},
		{Version: "1.5.0", Code: 150},
		{Version: "2.0.0", Code: 200},
	}
	retained, pruned := applyRetainVersions(nodes, 1)
	if len(retained) != 1 || retained[0].Code != 200 {
		t.Fatalf("retain=1: got %+v", retained)
	}
	if len(pruned) != 2 {
		t.Fatalf("pruned = %d, want 2", len(pruned))
	}

	retained, pruned = applyRetainVersions(nodes, 2)
	if len(retained) != 2 || len(pruned) != 1 {
		t.Fatalf("retain=2: retained=%d pruned=%d", len(retained), len(pruned))
	}
	if retained[0].Code != 150 || retained[1].Code != 200 {
		t.Fatalf("retain=2 order: %+v", retained)
	}
	if pruned[0].Code != 100 {
		t.Fatalf("pruned code = %d, want 100", pruned[0].Code)
	}
}

func TestApplyRetainVersionsNoopWhenAlreadySmall(t *testing.T) {
	nodes := []*model.VersionNode{{Version: "1.0.0", Code: 100}}
	retained, pruned := applyRetainVersions(nodes, 3)
	if len(retained) != 1 || len(pruned) != 0 {
		t.Fatalf("retain=3 with 1 node: retained=%d pruned=%d", len(retained), len(pruned))
	}
}
