package backends

import (
	"strings"
	"testing"

	"firoyang.com/relkit/internal/config"
)

func TestStaticHTTPIsReadOnly(t *testing.T) {
	cfg := &config.Config{Backends: map[string]map[string]any{
		"mirror": {"type": "static-http", "baseUrl": "https://example.invalid/"},
	}}
	backend, err := Create("mirror", cfg, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if backend.Writable() {
		t.Fatal("static-http must be read-only")
	}
}

func TestStaticHTTPRejectsLegacyStageDir(t *testing.T) {
	cfg := &config.Config{Backends: map[string]map[string]any{
		"mirror": {
			"type": "static-http", "baseUrl": "https://example.invalid/",
			"stageDir": "out",
		},
	}}
	_, err := Create("mirror", cfg, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "relkit-compatible") {
		t.Fatalf("err=%v", err)
	}
}
