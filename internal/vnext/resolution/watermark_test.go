package resolution

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestWatermarksRejectReplayAndEquivocation(t *testing.T) {
	first := "sha256:" + strings.Repeat("a", 64)
	second := "sha256:" + strings.Repeat("b", 64)
	value := &Watermarks{Schema: WatermarkSchema}
	if err := value.AdvanceOperator("prod", 2, first); err != nil {
		t.Fatal(err)
	}
	if err := value.AdvanceOperator("prod", 1, first); err == nil || !strings.Contains(err.Error(), "replay") {
		t.Fatalf("unexpected replay result: %v", err)
	}
	if err := value.AdvanceOperator("prod", 2, second); err == nil || !strings.Contains(err.Error(), "equivocation") {
		t.Fatalf("unexpected equivocation result: %v", err)
	}
	if err := value.AdvanceOperator("prod", 3, second); err != nil {
		t.Fatal(err)
	}
}

func TestWatermarksRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "watermarks.json")
	digest := "sha256:" + strings.Repeat("c", 64)
	value := &Watermarks{Schema: WatermarkSchema}
	if err := value.AdvanceCatalog(digest, 4, digest); err != nil {
		t.Fatal(err)
	}
	if err := value.Write(path); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadWatermarks(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Catalogs[digest].Sequence != 4 {
		t.Fatalf("loaded watermark: %#v", loaded.Catalogs[digest])
	}
}
