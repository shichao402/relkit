package verify

import (
	"testing"
)

// A base-URL change (host rename) leaves every already-published manifest
// declaring the URL that was in force when it was signed. The release commit
// gate only strict-checks the version it is releasing; older drift must be a
// warning, otherwise the channel can never publish again until retainVersions
// prunes the old nodes.
func TestRunOptionsStrictScope(t *testing.T) {
	all := runOptions{}
	if !all.strict("0.1.0") || !all.strict("0.2.0") {
		t.Fatal("an empty strict set must hold every version to the current backend URLs")
	}

	gate := runOptions{strictVersions: map[string]bool{"0.2.2+88": true}}
	if !gate.strict("0.2.2+88") {
		t.Fatal("the version being released must be strict-checked")
	}
	if gate.strict("0.2.2+87") {
		t.Fatal("an already-published version must not be strict-checked by the commit gate")
	}
}

func TestCheckDeclaredURLStrictness(t *testing.T) {
	backend := &fakeBackend{name: "intranet", base: "https://update-internal.example/"}
	key := "manifest/loom/0.2.2+87.pb"

	strict := &Findings{}
	checkDeclaredURL(backend, key, []string{"https://update.example/"}, "index entry", true, strict)
	if strict.OK() {
		t.Fatal("strict mode must fail when the declared URL does not match")
	}

	relaxed := &Findings{}
	checkDeclaredURL(backend, key, []string{"https://update.example/"}, "index entry", false, relaxed)
	if !relaxed.OK() {
		t.Fatalf("relaxed mode must not fail the run, got errors: %v", relaxed.Errors)
	}
	if len(relaxed.Warnings) != 1 {
		t.Fatalf("relaxed mode must still report the drift, got %d warning(s)", len(relaxed.Warnings))
	}

	matching := &Findings{}
	checkDeclaredURL(backend, key, []string{"https://update-internal.example/manifest/loom/0.2.2+87.pb"}, "index entry", true, matching)
	if !matching.OK() || len(matching.Warnings) != 0 {
		t.Fatalf("a matching declared URL must be silent, got errors=%v warnings=%v", matching.Errors, matching.Warnings)
	}
}

type fakeBackend struct {
	name string
	base string
}

func (b *fakeBackend) Name() string                                  { return b.name }
func (b *fakeBackend) Type() string                                  { return "relkit-compatible" }
func (b *fakeBackend) Describe() string                              { return b.name }
func (b *fakeBackend) URLsAreLive() bool                             { return true }
func (b *fakeBackend) Writable() bool                                { return true }
func (b *fakeBackend) HostsBrowse() bool                             { return false }
func (b *fakeBackend) Get(string) ([]byte, error)                    { return nil, nil }
func (b *fakeBackend) Probe(string) (bool, *int64, string)           { return true, nil, "" }
func (b *fakeBackend) PutArtifact(string, string) ([]string, error)  { return nil, nil }
func (b *fakeBackend) PutImmutable([]byte, string) ([]string, error) { return nil, nil }
func (b *fakeBackend) PutPointer([]byte, string) ([]string, error)   { return nil, nil }

func (b *fakeBackend) URLFor(key string) *string {
	value := b.base + key
	return &value
}
