package backends

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
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

func TestPutArtifactCASReportsThinStagedMiss(t *testing.T) {
	root := t.TempDir()
	backend, err := newLocalBackend("disk", map[string]any{
		"type": "local", "baseUrl": "http://127.0.0.1/rup/", "outputDir": filepath.Join(root, "out"),
	}, root)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = PutArtifactCAS(backend, filepath.Join(root, "missing.bin"), "artifact/demo/1/app.bin", strings.Repeat("a", 64), 5)
	if err == nil || !strings.Contains(err.Error(), "CAS miss") {
		t.Fatalf("err=%v", err)
	}
}

func TestMaterializeCopiesFromIngestCASWithoutLocalFile(t *testing.T) {
	root := t.TempDir()
	ingest, err := newLocalBackend("disk", map[string]any{
		"type": "local", "baseUrl": "http://127.0.0.1/rup/", "outputDir": filepath.Join(root, "ingest"),
	}, root)
	if err != nil {
		t.Fatal(err)
	}
	mirror, err := newLocalBackend("mirror", map[string]any{
		"type": "local", "baseUrl": "http://127.0.0.1/mirror/", "outputDir": filepath.Join(root, "mirror"),
	}, root)
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte("hello")
	src := filepath.Join(t.TempDir(), "app.bin")
	if err := os.WriteFile(src, payload, 0o644); err != nil {
		t.Fatal(err)
	}
	digest, size, err := model.Sha256File(src)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := PutArtifactCAS(ingest, src, "artifact/demo/1.0.0/app.bin", digest, size); err != nil {
		t.Fatal(err)
	}
	urls, err := Materialize(ingest, mirror, "artifact/demo/1.0.0/app.bin", digest, size, filepath.Join(root, "missing.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if len(urls) != 1 {
		t.Fatalf("urls=%v", urls)
	}
	got, err := mirror.Get("artifact/demo/1.0.0/app.bin")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("mirror=%q", got)
	}
	casKey, _ := model.CasKey(digest)
	if data, _ := mirror.Get(casKey); len(data) != 0 {
		t.Fatal("mirror must not receive cas/")
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
