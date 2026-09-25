package main

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func httptestGet(srv *Server, target string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", target, nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	return rec
}

func writeAgentConfig(t *testing.T, site map[string]any) string {
	t.Helper()
	dir := t.TempDir()
	raw := map[string]any{
		"addr":      "127.0.0.1:8787",
		"stateDir":  filepath.Join(dir, "state"),
		"products":  map[string]any{"demo": map[string]any{"root": dir}},
		"site":      site,
	}
	data, _ := json.Marshal(raw)
	path := filepath.Join(dir, "relkit-agent.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadConfigExpandsLegacySiteMakers(t *testing.T) {
	path := writeAgentConfig(t, map[string]any{
		"makers": map[string]any{"projectId": "p1"},
	})
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Site.Sinks) != 1 || cfg.Site.Sinks[0].Type != "makers" || cfg.Site.Sinks[0].ProjectID != "p1" {
		t.Fatalf("sinks=%+v", cfg.Site.Sinks)
	}
	if cfg.Site.Sinks[0].Region != "china" || cfg.Site.Sinks[0].TokenEnv != "EDGEONE_PAGES_API_TOKEN" {
		t.Fatalf("defaults not applied: %+v", cfg.Site.Sinks[0])
	}
}

func TestLoadConfigRejectsMakersPlusSinks(t *testing.T) {
	path := writeAgentConfig(t, map[string]any{
		"makers": map[string]any{"projectId": "p1"},
		"sinks":  []any{map[string]any{"type": "directory", "path": "/srv/site"}},
	})
	_, err := LoadConfig(path)
	if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("expected mutual-exclusion error, got %v", err)
	}
}

func TestLoadConfigParsesSinks(t *testing.T) {
	path := writeAgentConfig(t, map[string]any{
		"sinks": []any{
			map[string]any{"type": "makers", "projectId": "relkit-updates-index"},
			map[string]any{"type": "backend", "backend": "intranet-serve"},
			map[string]any{"type": "directory", "path": "/srv/relkit-site"},
		},
	})
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Site.Sinks) != 3 {
		t.Fatalf("sinks=%+v", cfg.Site.Sinks)
	}
	for i, want := range []string{"makers", "backend", "directory"} {
		if cfg.Site.Sinks[i].Type != want {
			t.Fatalf("sink %d = %+v, want type %s", i, cfg.Site.Sinks[i], want)
		}
	}
}

func TestLoadConfigRejectsBadSink(t *testing.T) {
	path := writeAgentConfig(t, map[string]any{
		"sinks": []any{map[string]any{"type": "directory"}},
	})
	_, err := LoadConfig(path)
	if err == nil || !strings.Contains(err.Error(), "path is required") {
		t.Fatalf("expected sink validation error, got %v", err)
	}
}

func TestSiteStatusReportsSinks(t *testing.T) {
	t.Setenv("PAGES_SECRET", "must-not-leak")
	cfg, err := LoadConfig(writeAgentConfig(t, map[string]any{
		"sinks": []any{
			map[string]any{"type": "makers", "projectId": "makers-demo", "tokenEnv": "PAGES_SECRET", "region": "china"},
			map[string]any{"type": "directory", "path": "/srv/relkit-site"},
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	srv := NewServer(cfg)
	rec := httptestGet(srv, "/-/site")
	body := rec.Body.String()
	if !strings.Contains(body, `"sinks": 2`) || !strings.Contains(body, `"tokenPresent": true`) {
		t.Fatalf("body=%s", body)
	}
	if strings.Contains(body, "must-not-leak") {
		t.Fatalf("site status leaked token: %s", body)
	}
}
