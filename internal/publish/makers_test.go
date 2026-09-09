package publish

import (
	"strings"
	"testing"

	"go.firoyang.com/relkit/internal/backends"
	"go.firoyang.com/relkit/internal/config"
)

type typeOnlyBackend struct {
	kind string
}

func (b typeOnlyBackend) Name() string      { return b.kind }
func (b typeOnlyBackend) Type() string      { return b.kind }
func (b typeOnlyBackend) Describe() string  { return b.kind }
func (b typeOnlyBackend) URLsAreLive() bool { return true }
func (b typeOnlyBackend) Writable() bool    { return true }
func (b typeOnlyBackend) HostsBrowse() bool {
	return b.kind == "relkit-compatible"
}
func (b typeOnlyBackend) PutArtifact(string, string) ([]string, error) {
	return nil, nil
}
func (b typeOnlyBackend) PutImmutable([]byte, string) ([]string, error) {
	return nil, nil
}
func (b typeOnlyBackend) PutPointer([]byte, string) ([]string, error) { return nil, nil }
func (b typeOnlyBackend) Get(string) ([]byte, error)                  { return nil, nil }
func (b typeOnlyBackend) URLFor(string) *string                       { return nil }
func (b typeOnlyBackend) Probe(string) (bool, *int64, string)         { return false, nil, "" }

func sinkNames(sinks []BrowseSink) []string {
	names := make([]string, 0, len(sinks))
	for _, sink := range sinks {
		names = append(names, sink.Name())
	}
	return names
}

func hasSinkPrefix(sinks []BrowseSink, prefix string) bool {
	for _, name := range sinkNames(sinks) {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func TestOpenBrowseSinks(t *testing.T) {
	makers := &config.Config{Site: config.SiteConfig{Makers: &config.MakersConfig{ProjectID: "makers-test"}}}
	s3 := []backends.Backend{typeOnlyBackend{kind: "s3-compatible"}}
	serve := []backends.Backend{typeOnlyBackend{kind: "relkit-compatible"}}
	mixed := []backends.Backend{typeOnlyBackend{kind: "relkit-compatible"}, typeOnlyBackend{kind: "s3-compatible"}}

	if !hasSinkPrefix(OpenBrowseSinks(makers, s3), "makers:") {
		t.Fatal("protocol-only backend with site.makers should open MakersSink")
	}
	if hasSinkPrefix(OpenBrowseSinks(makers, serve), "makers:") {
		t.Fatal("--to relkit-compatible should skip Makers even when site.makers is set")
	}
	if len(OpenBrowseSinks(makers, serve)) != 1 || OpenBrowseSinks(makers, serve)[0].Name() != "relkit-compatible" {
		t.Fatal("relkit-compatible should open a data-plane BrowseSink")
	}
	sinks := OpenBrowseSinks(makers, mixed)
	if !hasSinkPrefix(sinks, "makers:") {
		t.Fatal("mixed publish including a protocol-only backend should deploy Makers")
	}
	if !hasSinkPrefix(sinks, "relkit-compatible") {
		t.Fatal("mixed publish should still write browse/ on the HostsBrowse backend")
	}
	if hasSinkPrefix(OpenBrowseSinks(&config.Config{}, s3), "makers:") {
		t.Fatal("protocol-only backend without site.makers should not open MakersSink")
	}
}

func TestWarnMissingSiteSink(t *testing.T) {
	var lines []string
	printer := func(line string) { lines = append(lines, line) }
	cfg := &config.Config{}
	warnMissingSiteSink(cfg, []backends.Backend{typeOnlyBackend{kind: "s3-compatible"}}, printer)
	if len(lines) != 1 || !strings.Contains(lines[0], "BrowseSink") {
		t.Fatalf("lines=%v", lines)
	}
	lines = nil
	warnMissingSiteSink(cfg, []backends.Backend{typeOnlyBackend{kind: "relkit-compatible"}}, printer)
	if len(lines) != 0 {
		t.Fatalf("relkit-compatible should not warn, got %v", lines)
	}
}
