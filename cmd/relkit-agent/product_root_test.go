package main

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

type adminErrorBody struct {
	OK      bool   `json:"ok"`
	Code    string `json:"code"`
	Field   string `json:"field"`
	Message string `json:"message"`
}

func registerProduct(t *testing.T, fx *adminFixture, id, root string) (int, map[string]any, adminErrorBody) {
	t.Helper()
	status, body := fx.do(t, http.MethodPost, "/v1/admin/products", fx.admin, map[string]string{"id": id, "root": root})
	var ok map[string]any
	var bad adminErrorBody
	if status == http.StatusOK {
		if err := json.Unmarshal([]byte(body), &ok); err != nil {
			t.Fatalf("decode success body: %v (%s)", err, body)
		}
	} else if err := json.Unmarshal([]byte(body), &bad); err != nil {
		t.Fatalf("error body is not structured JSON: %v (%s)", err, body)
	}
	return status, ok, bad
}

func registeredRoot(t *testing.T, doc map[string]any) string {
	t.Helper()
	product, _ := doc["product"].(map[string]any)
	root, _ := product["root"].(string)
	if root == "" {
		t.Fatalf("response carries no product root: %v", doc)
	}
	return root
}

// TestProductRootParentFromConfig: the configured parent decides a blank root,
// not a path baked into the binary.
func TestProductRootParentFromConfig(t *testing.T) {
	fx := newAdminFixture(t)
	parent := filepath.Join(t.TempDir(), "products")
	fx.cfg.ProductRootParent = parent

	status, doc, bad := registerProduct(t, fx, "fresh", "")
	if status != http.StatusOK {
		t.Fatalf("status=%d err=%+v", status, bad)
	}
	want := filepath.Join(parent, "fresh")
	if got := registeredRoot(t, doc); got != want {
		t.Fatalf("root=%q want %q", got, want)
	}
	if info, err := os.Stat(want); err != nil || !info.IsDir() {
		t.Fatalf("product root was not created: %v", err)
	}
}

// TestProductRootParentInferred: with nothing configured, a blank root lands
// next to the products already registered on this machine.
func TestProductRootParentInferred(t *testing.T) {
	fx := newAdminFixture(t)
	existing, ok := fx.cfg.admin.Product(fx.product)
	if !ok {
		t.Fatal("fixture product missing")
	}
	status, doc, bad := registerProduct(t, fx, "sibling", "")
	if status != http.StatusOK {
		t.Fatalf("status=%d err=%+v", status, bad)
	}
	want := filepath.Join(filepath.Dir(existing.Root), "sibling")
	if got := registeredRoot(t, doc); got != want {
		t.Fatalf("root=%q want %q", got, want)
	}
}

// TestProductRootParentNotInferable: scattered products give no convention,
// so a blank root is refused with a message pointing at the fix instead of
// trying some path that may not be writable.
func TestProductRootParentNotInferable(t *testing.T) {
	fx := newAdminFixture(t)
	if _, err := fx.cfg.admin.AddProduct("elsewhere", filepath.Join(t.TempDir(), "x", "elsewhere"), ""); err != nil {
		t.Fatal(err)
	}
	status, _, bad := registerProduct(t, fx, "orphan", "")
	if status != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", status)
	}
	if bad.Code != "product_root_required" || bad.Field != "root" || bad.Message == "" {
		t.Fatalf("unexpected error %+v", bad)
	}
	if _, ok := fx.cfg.admin.Product("orphan"); ok {
		t.Fatal("refused registration still left a product behind")
	}
}

// TestProductRootUnwritable: a root the service user cannot create comes back
// as a field error naming the path, and nothing is registered.
func TestProductRootUnwritable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory permission bits are not enforced on windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	fx := newAdminFixture(t)
	locked := filepath.Join(t.TempDir(), "locked")
	if err := os.MkdirAll(locked, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })
	target := filepath.Join(locked, "nope")

	status, _, bad := registerProduct(t, fx, "nope", target)
	if status != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", status)
	}
	if bad.Code != "product_root_unwritable" || bad.Field != "root" {
		t.Fatalf("unexpected error %+v", bad)
	}
	if _, ok := fx.cfg.admin.Product("nope"); ok {
		t.Fatal("unwritable root still registered the product")
	}
}

// TestProductIDCheckedBeforePath: the id is validated before it is joined into
// a filesystem path, so a crafted id cannot create directories elsewhere.
func TestProductIDCheckedBeforePath(t *testing.T) {
	fx := newAdminFixture(t)
	parent := filepath.Join(t.TempDir(), "products")
	fx.cfg.ProductRootParent = parent
	status, _, bad := registerProduct(t, fx, "../escape", "")
	if status != http.StatusBadRequest || bad.Code != "invalid_product_id" {
		t.Fatalf("status=%d err=%+v", status, bad)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(parent), "escape")); !os.IsNotExist(err) {
		t.Fatalf("crafted id created a directory outside the parent: %v", err)
	}
}

// TestAdminSettingsExposesRootParent: the panel reads the real parent instead
// of printing a placeholder that is only true on some other machine.
func TestAdminSettingsExposesRootParent(t *testing.T) {
	fx := newAdminFixture(t)
	if status, _ := fx.do(t, http.MethodGet, "/v1/admin/settings", "", nil); status != http.StatusUnauthorized {
		t.Fatalf("anonymous settings status=%d want 401", status)
	}
	fx.cfg.ProductRootParent = "/data/relkit"
	status, body := fx.do(t, http.MethodGet, "/v1/admin/settings", fx.owner, nil)
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%s", status, body)
	}
	var doc struct {
		Parent string `json:"productRootParent"`
		Source string `json:"productRootParentSource"`
	}
	if err := json.Unmarshal([]byte(body), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Parent != "/data/relkit" || doc.Source != "config" {
		t.Fatalf("settings=%+v", doc)
	}
}

// TestLoadConfigProductRootParent: relative values resolve against the config
// file, like every other path in it.
func TestLoadConfigProductRootParent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.json")
	if err := os.WriteFile(path, []byte(`{"addr":"127.0.0.1:0","productRootParent":"trees"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, "trees"); cfg.ProductRootParent != want {
		t.Fatalf("ProductRootParent=%q want %q", cfg.ProductRootParent, want)
	}
}
