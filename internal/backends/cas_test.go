package backends

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.firoyang.com/relkit/internal/model"
)

type memStore struct {
	name string
	kind string
	obj  map[string][]byte
}

func newMem(name string) *memStore {
	return &memStore{name: name, kind: "mem", obj: map[string][]byte{}}
}

func (m *memStore) Name() string                  { return m.name }
func (m *memStore) Type() string                  { return m.kind }
func (m *memStore) Describe() string              { return m.name }
func (m *memStore) URLsAreLive() bool             { return true }
func (m *memStore) Writable() bool                { return true }
func (m *memStore) HostsBrowse() bool             { return false }
func (m *memStore) URLFor(key string) *string     { u := "http://mem/" + key; return &u }
func (m *memStore) Probe(string) (bool, *int64, string) {
	return false, nil, ""
}
func (m *memStore) PutArtifact(localPath, key string) ([]string, error) {
	data, err := os.ReadFile(localPath)
	if err != nil {
		return nil, err
	}
	m.obj[key] = data
	return []string{*m.URLFor(key)}, nil
}
func (m *memStore) PutImmutable(data []byte, key string) ([]string, error) {
	m.obj[key] = append([]byte(nil), data...)
	return []string{*m.URLFor(key)}, nil
}
func (m *memStore) PutPointer(data []byte, key string) ([]string, error) {
	return m.PutImmutable(data, key)
}
func (m *memStore) Get(key string) ([]byte, error) { return m.obj[key], nil }
func (m *memStore) Head(key string) (int64, bool, error) {
	data, ok := m.obj[key]
	if !ok {
		return 0, false, nil
	}
	return int64(len(data)), true, nil
}
func (m *memStore) Promote(srcKey, dstKey string) ([]string, error) {
	data, ok := m.obj[srcKey]
	if !ok {
		return nil, os.ErrNotExist
	}
	m.obj[dstKey] = append([]byte(nil), data...)
	return []string{*m.URLFor(dstKey)}, nil
}

func TestPutArtifactCASSkipsSecondUpload(t *testing.T) {
	backend := newMem("disk")
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
	backend := newMem("disk")
	_, _, err := PutArtifactCAS(backend, filepath.Join(t.TempDir(), "missing.bin"), "artifact/demo/1/app.bin", strings.Repeat("a", 64), 5)
	if err == nil || !strings.Contains(err.Error(), "CAS miss") {
		t.Fatalf("err=%v", err)
	}
}

func TestMaterializeCopiesFromIngestCASWithoutLocalFile(t *testing.T) {
	ingest := newMem("disk")
	mirror := newMem("mirror")
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
	urls, err := Materialize(ingest, mirror, "artifact/demo/1.0.0/app.bin", digest, size, filepath.Join(t.TempDir(), "missing.bin"))
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
