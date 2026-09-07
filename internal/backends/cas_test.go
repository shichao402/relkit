package backends

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"cnb.cool/shichao402/relkit/internal/model"
)

func TestPutArtifactCASLocalSkipsSecondUpload(t *testing.T) {
	root := t.TempDir()
	backend, err := newLocalBackend("disk", map[string]any{
		"type":      "local",
		"baseUrl":   "http://127.0.0.1/rup/",
		"outputDir": filepath.Join(root, "out"),
	}, root)
	if err != nil {
		t.Fatal(err)
	}

	payload := bytes.Repeat([]byte("blob"), 32)
	src := filepath.Join(t.TempDir(), "app.bin")
	if err := os.WriteFile(src, payload, 0o644); err != nil {
		t.Fatal(err)
	}
	digest, size, err := model.Sha256File(src)
	if err != nil {
		t.Fatal(err)
	}

	urls, skipped, err := PutArtifactCAS(backend, src, "artifact/demo/1.0.0/app.bin", digest, size)
	if err != nil {
		t.Fatal(err)
	}
	if skipped {
		t.Fatal("first put must upload")
	}
	if len(urls) != 1 {
		t.Fatalf("urls=%v", urls)
	}

	if err := os.Remove(src); err != nil {
		t.Fatal(err)
	}
	urls, skipped, err = PutArtifactCAS(backend, src, "artifact/demo/2.0.0/app.bin", digest, size)
	if err != nil {
		t.Fatal(err)
	}
	if !skipped {
		t.Fatal("second put must skip upload when cas exists")
	}

	got, err := backend.Get("artifact/demo/2.0.0/app.bin")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("promoted bytes mismatch")
	}
}

func TestPutArtifactCASFallsBackWithoutIngest(t *testing.T) {
	backend := typeOnlyNoIngest{}
	urls, skipped, err := PutArtifactCAS(backend, "missing.bin", "artifact/x", "aa", 1)
	if err != nil {
		t.Fatal(err)
	}
	if skipped {
		t.Fatal("non-ingest backend cannot skip")
	}
	if len(urls) != 0 {
		t.Fatalf("urls=%v", urls)
	}
}

type typeOnlyNoIngest struct{}

func (typeOnlyNoIngest) Name() string                                 { return "stub" }
func (typeOnlyNoIngest) Type() string                                 { return "stub" }
func (typeOnlyNoIngest) Describe() string                             { return "stub" }
func (typeOnlyNoIngest) URLsAreLive() bool                            { return false }
func (typeOnlyNoIngest) Writable() bool                               { return true }
func (typeOnlyNoIngest) HostsBrowse() bool                            { return false }
func (typeOnlyNoIngest) PutArtifact(string, string) ([]string, error) { return nil, nil }
func (typeOnlyNoIngest) PutImmutable([]byte, string) ([]string, error) {
	return nil, nil
}
func (typeOnlyNoIngest) PutPointer([]byte, string) ([]string, error) { return nil, nil }
func (typeOnlyNoIngest) Get(string) ([]byte, error)                  { return nil, nil }
func (typeOnlyNoIngest) URLFor(string) *string                       { return nil }
func (typeOnlyNoIngest) Probe(string) (bool, *int64, string)         { return false, nil, "" }
