package agentadmin

import (
	"path/filepath"
	"sync"
	"testing"
)

func TestAllocateReleaseCodeIsMonotonicPersistedAndFloored(t *testing.T) {
	path := filepath.Join(t.TempDir(), "admin.json")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AddProduct("demo", "/srv/demo", ""); err != nil {
		t.Fatal(err)
	}

	// A product that published before the server took over numbering continues
	// above its history.
	code, err := store.AllocateReleaseCode("demo", 38)
	if err != nil || code != 39 {
		t.Fatalf("first claim=%d err=%v, want 39", code, err)
	}
	// The counter wins over a lower floor.
	code, err = store.AllocateReleaseCode("demo", 0)
	if err != nil || code != 40 {
		t.Fatalf("second claim=%d err=%v, want 40", code, err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	product, _ := reopened.Product("demo")
	if product.LastReleaseCode != 40 {
		t.Fatalf("persisted lastReleaseCode=%d, want 40", product.LastReleaseCode)
	}
	code, err = reopened.AllocateReleaseCode("demo", 0)
	if err != nil || code != 41 {
		t.Fatalf("claim after restart=%d err=%v, want 41", code, err)
	}

	if _, err := reopened.AllocateReleaseCode("missing", 0); err == nil {
		t.Fatal("claim for an unregistered product succeeded")
	}
	if _, err := reopened.AllocateReleaseCode("demo", -1); err == nil {
		t.Fatal("negative floor accepted")
	}
}

func TestAllocateReleaseCodeNeverRepeatsUnderConcurrency(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "admin.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AddProduct("demo", "/srv/demo", ""); err != nil {
		t.Fatal(err)
	}
	const claims = 32
	var wg sync.WaitGroup
	results := make(chan int64, claims)
	for range claims {
		wg.Add(1)
		go func() {
			defer wg.Done()
			code, err := store.AllocateReleaseCode("demo", 0)
			if err != nil {
				t.Error(err)
				return
			}
			results <- code
		}()
	}
	wg.Wait()
	close(results)
	seen := map[int64]bool{}
	for code := range results {
		if seen[code] {
			t.Fatalf("code %d handed out twice", code)
		}
		seen[code] = true
	}
	if len(seen) != claims {
		t.Fatalf("got %d distinct codes, want %d", len(seen), claims)
	}
}
