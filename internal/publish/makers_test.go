package publish

import (
	"strings"
	"testing"

	"cnb.cool/shichao402/relkit/internal/backends"
	"cnb.cool/shichao402/relkit/internal/config"
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
	return b.kind == "local" || b.kind == "http-put"
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
	local := []backends.Backend{typeOnlyBackend{kind: "local"}}
	httpPut := []backends.Backend{typeOnlyBackend{kind: "http-put"}}
	mixed := []backends.Backend{typeOnlyBackend{kind: "local"}, typeOnlyBackend{kind: "s3-compatible"}}

	if !hasSinkPrefix(OpenBrowseSinks(makers, s3), "makers:") {
		t.Fatal("protocol-only backend with site.makers should open MakersSink")
	}
	if hasSinkPrefix(OpenBrowseSinks(makers, local), "makers:") {
		t.Fatal("--to local should skip Makers even when site.makers is set")
	}
	if len(OpenBrowseSinks(makers, local)) != 1 || OpenBrowseSinks(makers, local)[0].Name() != "local" {
		t.Fatal("local should open a data-plane BrowseSink")
	}
	if len(OpenBrowseSinks(makers, httpPut)) != 1 || OpenBrowseSinks(makers, httpPut)[0].Name() != "http-put" {
		t.Fatal("http-put should host browse/ with the full dump")
	}
	sinks := OpenBrowseSinks(makers, mixed)
	if !hasSinkPrefix(sinks, "makers:") {
		t.Fatal("mixed publish including a protocol-only backend should deploy Makers")
	}
	if !hasSinkPrefix(sinks, "local") {
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
	warnMissingSiteSink(cfg, []backends.Backend{typeOnlyBackend{kind: "local"}}, printer)
	if len(lines) != 0 {
		t.Fatalf("local should not warn, got %v", lines)
	}
}
