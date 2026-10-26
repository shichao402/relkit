package main

import (
	"net/http"
	"strings"
	"testing"
)

func asSlice(t *testing.T, v any) []any {
	t.Helper()
	s, ok := v.([]any)
	if !ok {
		t.Fatalf("not a list: %#v", v)
	}
	return s
}

func asMap(t *testing.T, v any) map[string]any {
	t.Helper()
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("not an object: %#v", v)
	}
	return m
}

func TestAPIProductsMergeRegistrationAndTree(t *testing.T) {
	api, stub, dir := newAPITestDir(t)
	stub.addProduct("loom", "/data/relkit/loom")
	stub.addProduct("fresh", "/data/relkit/fresh")
	writeRelease(t, dir, "loom", "stable", "1.2.0", 12)
	writeRelease(t, dir, "orphan", "stable", "0.1.0", 1)
	api.login()

	status, body := api.call(http.MethodGet, apiPrefix+"products", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("products = %d %v", status, body)
	}
	byID := map[string]map[string]any{}
	for _, row := range asSlice(t, body["products"]) {
		m := asMap(t, row)
		byID[m["id"].(string)] = m
	}
	if len(byID) != 3 {
		t.Fatalf("want loom, fresh, orphan; got %v", byID)
	}
	if loom := byID["loom"]; loom["registered"] != true || loom["published"] != true {
		t.Fatalf("loom = %v", loom)
	}
	if fresh := byID["fresh"]; fresh["registered"] != true || fresh["published"] != false {
		t.Fatalf("a registration with nothing published must say so: %v", fresh)
	}
	if orphan := byID["orphan"]; orphan["registered"] != false || orphan["published"] != true {
		t.Fatalf("a tree with no registration must be flagged to an admin: %v", orphan)
	}
	ch := asMap(t, asSlice(t, byID["loom"]["channels"])[0])
	if ch["name"] != "stable" || ch["version"] != "1.2.0" {
		t.Fatalf("channel = %v", ch)
	}
}

func TestAPIOwnerDoesNotSeeUnregisteredTrees(t *testing.T) {
	api, stub, dir := newAPITestDir(t)
	stub.addUser("own", "password123", "owner", []string{"loom"})
	stub.addProduct("loom", "/data/relkit/loom")
	writeRelease(t, dir, "loom", "stable", "1.0.0", 1)
	writeRelease(t, dir, "secret", "stable", "1.0.0", 1)
	status, body := api.call(http.MethodPost, apiPrefix+"session/login",
		map[string]string{"username": "own", "password": "x"}, nil)
	if status != http.StatusOK {
		t.Fatalf("login = %d %v", status, body)
	}
	api.csrf, _ = body["csrf"].(string)
	_, body = api.call(http.MethodGet, apiPrefix+"products", nil, nil)
	for _, row := range asSlice(t, body["products"]) {
		if asMap(t, row)["id"] == "secret" {
			t.Fatalf("owner must not see a tree the agent did not list for them: %v", body)
		}
	}
}

func TestAPIProductDetail(t *testing.T) {
	api, stub, dir := newAPITestDir(t)
	stub.addProduct("loom", "/data/relkit/loom")
	writeRelease(t, dir, "loom", "stable", "1.2.0", 12)
	api.login()

	status, body := api.call(http.MethodGet, apiPrefix+"products/loom", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("detail = %d %v", status, body)
	}
	detail := asMap(t, asMap(t, body["product"])["detail"].([]any)[0])
	if detail["name"] != "stable" || detail["latestVersion"] != "1.2.0" {
		t.Fatalf("detail = %v", detail)
	}
	artifacts := asSlice(t, detail["artifacts"])
	if len(artifacts) != 1 || asMap(t, artifacts[0])["filename"] != "app.zip" {
		t.Fatalf("artifacts = %v", artifacts)
	}
	if status, body := api.call(http.MethodGet, apiPrefix+"products/missing", nil, nil); status != http.StatusNotFound {
		t.Fatalf("missing product = %d %v", status, body)
	}
	if status, _ := api.call(http.MethodGet, apiPrefix+"products/..%2Fetc", nil, nil); status != http.StatusNotFound {
		t.Fatalf("traversal = %d, want 404", status)
	}
}

func TestAPIRegisterProductReturnsPlaintextOnceAndNeverInListings(t *testing.T) {
	api, _, _ := newAPITestDir(t)
	api.login()

	status, body := api.call(http.MethodPost, apiPrefix+"products",
		map[string]string{"id": "newprod", "root": "", "label": "ci"}, nil)
	if status != http.StatusOK {
		t.Fatalf("register = %d %v", status, body)
	}
	plain, _ := body["plaintext"].(string)
	if plain == "" {
		t.Fatalf("registration must hand back the one-time plaintext: %v", body)
	}

	for _, path := range []string{"products", "products/newprod", "products/newprod/tokens", "session", "users"} {
		_, got := api.call(http.MethodGet, apiPrefix+path, nil, nil)
		if strings.Contains(fmtJSON(got), plain) {
			t.Fatalf("GET %s leaked the plaintext token", path)
		}
	}
}

func fmtJSON(v any) string {
	var b strings.Builder
	var walk func(x any)
	walk = func(x any) {
		switch val := x.(type) {
		case string:
			b.WriteString(val)
			b.WriteByte('\n')
		case map[string]any:
			for k, v := range val {
				b.WriteString(k)
				b.WriteByte('\n')
				walk(v)
			}
		case []any:
			for _, v := range val {
				walk(v)
			}
		}
	}
	walk(v)
	return b.String()
}

func TestAPIRegisterProductRelaysStructuredAgentError(t *testing.T) {
	api, stub, _ := newAPITestDir(t)
	stub.mu.Lock()
	stub.refuse = ""
	stub.mu.Unlock()
	api.login()
	status, body := api.call(http.MethodPost, apiPrefix+"products", map[string]string{"id": " "}, nil)
	if status != http.StatusBadRequest || body["message"] == "" || body["ok"] != false {
		t.Fatalf("empty id = %d %v", status, body)
	}
}

func TestAPIRegisterProductRejectsUnknownFields(t *testing.T) {
	api, _, _ := newAPITestDir(t)
	api.login()
	status, body := api.call(http.MethodPost, apiPrefix+"products",
		map[string]string{"id": "x", "rooot": "/typo"}, nil)
	if status != http.StatusBadRequest || body["code"] != "bad_request" {
		t.Fatalf("misspelled key = %d %v", status, body)
	}
}

func TestAPIDeleteProduct(t *testing.T) {
	api, stub, _ := newAPITestDir(t)
	stub.addProduct("gone", "/data/relkit/gone")
	api.login()
	status, body := api.call(http.MethodDelete, apiPrefix+"products/gone", nil, nil)
	if status != http.StatusOK || body["ok"] != true {
		t.Fatalf("delete = %d %v", status, body)
	}
	_, body = api.call(http.MethodGet, apiPrefix+"products", nil, nil)
	for _, row := range asSlice(t, body["products"]) {
		if asMap(t, row)["id"] == "gone" {
			t.Fatalf("product still listed after delete: %v", body)
		}
	}
}

// A preview must reach the agent (only it can say what retirement would drop)
// and must leave the product exactly where it was.
func TestAPIDeleteProductDryRunChangesNothing(t *testing.T) {
	api, stub, _ := newAPITestDir(t)
	stub.addProduct("kept", "/data/relkit/kept")
	api.login()

	status, body := api.call(http.MethodDelete, apiPrefix+"products/kept?dryRun=1", nil, nil)
	if status != http.StatusOK || body["ok"] != true {
		t.Fatalf("dry run = %d %v", status, body)
	}
	if body["dryRun"] != true {
		t.Fatalf("a preview must say so, otherwise a caller cannot tell it apart: %v", body)
	}
	if body["removedTokens"] != float64(0) {
		t.Fatalf("a preview removed tokens: %v", body)
	}

	stub.mu.Lock()
	previews := append([]string(nil), stub.dryRuns...)
	stillThere := 0
	for _, product := range stub.products {
		if product.ID == "kept" {
			stillThere++
		}
	}
	stub.mu.Unlock()
	if len(previews) != 1 || previews[0] != "kept" {
		t.Fatalf("the panel did not forward the flag to the agent: %v", previews)
	}
	if stillThere != 1 {
		t.Fatalf("a preview removed the product registration")
	}

	// The product is still usable, which is the point of previewing.
	_, body = api.call(http.MethodGet, apiPrefix+"products", nil, nil)
	found := false
	for _, row := range asSlice(t, body["products"]) {
		if asMap(t, row)["id"] == "kept" {
			found = true
		}
	}
	if !found {
		t.Fatalf("product missing after a preview: %v", body)
	}
}

func TestAPITokenLifecycle(t *testing.T) {
	api, stub, _ := newAPITestDir(t)
	first := stub.addProduct("loom", "/data/relkit/loom")
	api.login()

	status, body := api.call(http.MethodPost, apiPrefix+"products/loom/tokens", map[string]string{"label": "ci2"}, nil)
	if status != http.StatusOK || body["plaintext"] == "" {
		t.Fatalf("issue = %d %v", status, body)
	}

	status, body = api.call(http.MethodPost, apiPrefix+"tokens/"+first.ID+"/rotate", nil, nil)
	if status != http.StatusOK || !strings.HasPrefix(body["plaintext"].(string), "plain-rotated") {
		t.Fatalf("rotate = %d %v", status, body)
	}

	status, body = api.call(http.MethodGet, apiPrefix+"products/loom/tokens", nil, nil)
	if status != http.StatusOK || len(asSlice(t, body["tokens"])) != 2 {
		t.Fatalf("list = %d %v", status, body)
	}
	if strings.Contains(fmtJSON(body), "plain-") {
		t.Fatalf("token listing leaked a plaintext: %v", body)
	}

	status, body = api.call(http.MethodPost, apiPrefix+"tokens/"+first.ID+"-r/revoke", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("revoke = %d %v", status, body)
	}
	if _, has := body["plaintext"]; has {
		t.Fatalf("a revocation issues nothing, so it must not return a plaintext: %v", body)
	}
	if status, _ := api.call(http.MethodPost, apiPrefix+"tokens/"+first.ID+"/explode", nil, nil); status != http.StatusNotFound {
		t.Fatalf("unknown action = %d", status)
	}
	if status, body := api.call(http.MethodPost, apiPrefix+"tokens/nope/rotate", nil, nil); status != http.StatusNotFound {
		t.Fatalf("unknown token = %d %v", status, body)
	}
}

func TestAPIGetNeverTriggersWrites(t *testing.T) {
	api, stub, _ := newAPITestDir(t)
	first := stub.addProduct("loom", "/data/relkit/loom")
	api.login()
	for _, path := range []string{"tokens/" + first.ID + "/rotate", "tokens/" + first.ID + "/revoke"} {
		status, body := api.call(http.MethodGet, apiPrefix+path, nil, nil)
		if status != http.StatusMethodNotAllowed {
			t.Fatalf("GET %s = %d %v: a replayed GET must never rotate or revoke", path, status, body)
		}
	}
	_, body := api.call(http.MethodGet, apiPrefix+"products/loom/tokens", nil, nil)
	tok := asMap(t, asSlice(t, body["tokens"])[0])
	if tok["id"] != first.ID || tok["rotatedAt"] != nil && tok["rotatedAt"] != "" {
		t.Fatalf("token changed after GETs: %v", tok)
	}
}

func TestAPIUsersAndRoles(t *testing.T) {
	api, stub, _ := newAPITestDir(t)
	stub.addProduct("loom", "/data/relkit/loom")
	api.login()

	status, body := api.call(http.MethodPost, apiPrefix+"users",
		map[string]any{"username": "dev", "password": "password123", "role": "owner", "products": []string{"loom", "loom", " "}}, nil)
	if status != http.StatusOK {
		t.Fatalf("add user = %d %v", status, body)
	}
	if prods := asSlice(t, asMap(t, body["user"])["products"]); len(prods) != 1 {
		t.Fatalf("products must be de-duplicated and trimmed: %v", prods)
	}

	status, body = api.call(http.MethodGet, apiPrefix+"roles", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("roles = %d %v", status, body)
	}
	counts := map[string]float64{}
	for _, r := range asSlice(t, body["roles"]) {
		m := asMap(t, r)
		counts[m["name"].(string)] = m["members"].(float64)
	}
	if counts["admin"] != 1 || counts["owner"] != 1 {
		t.Fatalf("role counts = %v", counts)
	}

	if status, body := api.call(http.MethodPost, apiPrefix+"users/dev/password", map[string]string{"password": "newpassword1"}, nil); status != http.StatusOK {
		t.Fatalf("password = %d %v", status, body)
	}
	if status, body := api.call(http.MethodPost, apiPrefix+"users/dev/role", map[string]any{"role": "owner", "products": []string{}}, nil); status != http.StatusBadRequest || body["message"] == "" {
		t.Fatalf("an owner with no products must be refused with the agent's reason: %d %v", status, body)
	}
	if status, body := api.call(http.MethodPut, apiPrefix+"users/dev/products", map[string]any{"products": []string{"loom"}}, nil); status != http.StatusOK {
		t.Fatalf("set products = %d %v", status, body)
	}
	if status, body := api.call(http.MethodDelete, apiPrefix+"users/dev", nil, nil); status != http.StatusOK {
		t.Fatalf("delete user = %d %v", status, body)
	}
}

func TestAPIAgentRefusalIsRelayedVerbatim(t *testing.T) {
	api, stub, _ := newAPITestDir(t)
	api.login()
	stub.mu.Lock()
	stub.refuse = "forbidden: maintenance window"
	stub.mu.Unlock()
	status, body := api.call(http.MethodGet, apiPrefix+"users", nil, nil)
	if status != http.StatusForbidden || body["message"] != "forbidden: maintenance window" {
		t.Fatalf("refusal = %d %v", status, body)
	}
}

func TestAPIFilesAdminOnlyAndHidesInternalKeys(t *testing.T) {
	api, stub, dir := newAPITestDir(t)
	writeRelease(t, dir, "loom", "stable", "1.0.0", 1)
	writeFile(t, dir, ".relkit-serve-admin.json", []byte("{}"))
	api.login()

	status, body := api.call(http.MethodGet, apiPrefix+"files", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("files = %d %v", status, body)
	}
	names := fmtJSON(body)
	if !strings.Contains(names, "index") || strings.Contains(names, "admin.json") || strings.Contains(names, adminStateFileName) {
		t.Fatalf("listing = %v", body)
	}
	if status, body := api.call(http.MethodGet, apiPrefix+"files?dir=index/loom", nil, nil); status != http.StatusOK || !strings.Contains(fmtJSON(body), "stable.pb") {
		t.Fatalf("subdir = %d %v", status, body)
	}
	for _, bad := range []string{"..", "../..", "index/../..", "/etc"} {
		status, _ := api.call(http.MethodGet, apiPrefix+"files?dir="+bad, nil, nil)
		if status == http.StatusOK {
			t.Fatalf("dir=%q must not list anything outside the tree", bad)
		}
	}

	stub.addUser("own", "password123", "owner", []string{"loom"})
	other := newAPIClient(t, api.srv)
	if status, body := other.call(http.MethodPost, apiPrefix+"session/login",
		map[string]string{"username": "own", "password": "x"}, nil); status != http.StatusOK {
		t.Fatalf("owner login = %d %v", status, body)
	}
	if status, _ := other.call(http.MethodGet, apiPrefix+"files", nil, nil); status != http.StatusForbidden {
		t.Fatalf("an owner browsing the whole tree = %d, want 403", status)
	}
}

func TestAPISettingsRelaysRealRootParent(t *testing.T) {
	api, _, _ := newAPITestDir(t)
	api.login()
	status, body := api.call(http.MethodGet, apiPrefix+"settings", nil, nil)
	if status != http.StatusOK || asMap(t, body["settings"])["productRootParent"] != "/data/relkit" {
		t.Fatalf("settings = %d %v", status, body)
	}
}
