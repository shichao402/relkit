package main

// Tests for serving the rebuilt front end: the SPA shell must reach every page
// path, assets must keep their types, and the endpoints that are not the front
// end must not be swallowed by the catch-all.

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// appGet issues a GET through the real handler so routing order is what is
// under test, not a helper.
func appGet(t *testing.T, server *httptest.Server, path string) (*http.Response, string) {
	t.Helper()
	response, err := server.Client().Get(server.URL + path)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	t.Cleanup(func() { response.Body.Close() })
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return response, string(body)
}

func TestAppServesShellForDeepLinks(t *testing.T) {
	console, _ := newTestConsole(t)
	server := httptest.NewServer(console.handler())
	defer server.Close()

	// Every route the front end owns has to answer with the shell: a refresh on
	// a deep link is the normal way an operator reaches these pages.
	for _, path := range []string{
		"/-/admin/",
		"/-/admin/products",
		"/-/admin/products/loom",
		"/-/admin/users",
		"/-/admin/roles",
		"/-/admin/tokens",
		"/-/admin/files",
		"/-/admin/products/loom/tokens",
	} {
		response, body := appGet(t, server, path)
		if response.StatusCode != http.StatusOK {
			t.Errorf("GET %s = %d, want 200", path, response.StatusCode)
			continue
		}
		if !strings.Contains(body, `<div id="root">`) {
			t.Errorf("GET %s did not return the app shell", path)
		}
		if got := response.Header.Get("Cache-Control"); got != "no-store" {
			t.Errorf("GET %s Cache-Control = %q, want no-store", path, got)
		}
	}
}

func TestAppAssetsAreCacheableAndTyped(t *testing.T) {
	console, _ := newTestConsole(t)
	server := httptest.NewServer(console.handler())
	defer server.Close()

	_, shell := appGet(t, server, "/-/admin/")
	asset := assetPathFromShell(t, shell)

	response, _ := appGet(t, server, asset)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET %s = %d, want 200", asset, response.StatusCode)
	}
	if got := response.Header.Get("Cache-Control"); !strings.Contains(got, "immutable") {
		t.Errorf("asset Cache-Control = %q, want immutable", got)
	}
	contentType := response.Header.Get("Content-Type")
	if !strings.Contains(contentType, "javascript") && !strings.Contains(contentType, "css") {
		t.Errorf("asset Content-Type = %q, want a script or stylesheet type", contentType)
	}
}

// assetPathFromShell pulls the first hashed bundle reference out of the built
// index.html, so the test does not hardcode a build hash.
func assetPathFromShell(t *testing.T, shell string) string {
	t.Helper()
	const marker = `src="/-/admin/assets/`
	index := strings.Index(shell, marker)
	if index < 0 {
		t.Fatalf("built shell has no script reference:\n%s", shell)
	}
	rest := shell[index+len(`src="`):]
	end := strings.Index(rest, `"`)
	if end < 0 {
		t.Fatal("unterminated script reference in built shell")
	}
	return rest[:end]
}

func TestAppMissDoesNotSwallowOtherEndpoints(t *testing.T) {
	console, _ := newTestConsole(t)
	server := httptest.NewServer(console.handler())
	defer server.Close()

	// The health check and the API 404 stay JSON/text, never the app shell: a
	// monitor that received HTML would look healthy while parsing nonsense.
	response, body := appGet(t, server, "/-/health")
	if response.StatusCode != http.StatusOK || strings.TrimSpace(body) != "ok" {
		t.Errorf("health = %d %q", response.StatusCode, body)
	}
	if strings.Contains(body, "<div id=\"root\">") {
		t.Error("health check returned the app shell")
	}

	response, body = appGet(t, server, "/-/admin/api/nope")
	if response.StatusCode != http.StatusNotFound {
		t.Errorf("unknown API path = %d, want 404", response.StatusCode)
	}
	if strings.Contains(body, "<div id=\"root\">") {
		t.Error("unknown API path returned the app shell")
	}

	// A missing asset must 404 rather than fall back to the shell, or the
	// browser gets HTML where it expects JavaScript.
	response, _ = appGet(t, server, "/-/admin/assets/nope.js")
	if response.StatusCode != http.StatusNotFound {
		t.Errorf("missing asset = %d, want 404", response.StatusCode)
	}
}

func TestAppRejectsUnsafeMethods(t *testing.T) {
	console, _ := newTestConsole(t)
	server := httptest.NewServer(console.handler())
	defer server.Close()

	request, err := http.NewRequest(http.MethodPost, server.URL+"/-/admin/products", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer response.Body.Close()
	// The shell is read-only; a POST to a page path is a client bug, and the old
	// form handlers must not be reachable this way either.
	if response.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("POST to a page path = %d, want 405", response.StatusCode)
	}
}
