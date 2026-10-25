package main

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRetireLeftoverNote pins what the operator is told after a retirement.
// Retirement never deletes the tree, so the only defence against empty
// directories piling up beside live ones is saying out loud what was left. The
// three cases are the three states a tree can be in when the registration goes.
func TestRetireLeftoverNote(t *testing.T) {
	cases := []struct {
		name string
		root func(t *testing.T) string
		want string
	}{
		{
			name: "empty tree says it is empty and safe to remove",
			root: func(t *testing.T) string { return t.TempDir() },
			want: "is still on disk and empty",
		},
		{
			name: "non-empty tree says it was left untouched",
			root: func(t *testing.T) string {
				dir := t.TempDir()
				if err := os.WriteFile(filepath.Join(dir, "relkit.json"), []byte("{}"), 0o644); err != nil {
					t.Fatal(err)
				}
				return dir
			},
			want: "is still on disk with 1 item(s); it was left untouched",
		},
		{
			name: "missing tree says nothing",
			root: func(t *testing.T) string { return filepath.Join(t.TempDir(), "already-gone") },
			want: "",
		},
		{
			name: "unset root says nothing",
			root: func(t *testing.T) string { return "" },
			want: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := retireLeftoverNote(tc.root(t))
			if tc.want == "" {
				if got != "" {
					t.Fatalf("note = %q, want empty", got)
				}
				return
			}
			if !strings.Contains(got, tc.want) {
				t.Fatalf("note = %q, want it to contain %q", got, tc.want)
			}
		})
	}
}

// TestAdminRetireProductReportsLeftoverTree checks the note actually reaches the
// operator through the management API, on both a real run and the dry run that
// previews it. A note that only exists inside a helper is worth nothing.
func TestAdminRetireProductReportsLeftoverTree(t *testing.T) {
	fx := newAdminFixture(t)
	root := t.TempDir()
	status, body := fx.do(t, http.MethodPost, "/v1/admin/products", fx.admin, map[string]any{
		"id": "leaky", "root": root,
	})
	if status != http.StatusOK {
		t.Fatalf("register status=%d body=%s", status, body)
	}

	status, body = fx.do(t, http.MethodDelete, "/v1/admin/products/leaky?dryRun=1", fx.admin, nil)
	if status != http.StatusOK {
		t.Fatalf("dry run status=%d body=%s", status, body)
	}
	if !strings.Contains(body, "is still on disk and empty") {
		t.Fatalf("dry run did not name the leftover tree: %s", body)
	}
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("dry run removed the tree: %v", err)
	}

	status, body = fx.do(t, http.MethodDelete, "/v1/admin/products/leaky", fx.admin, nil)
	if status != http.StatusOK {
		t.Fatalf("retire status=%d body=%s", status, body)
	}
	if !strings.Contains(body, "is still on disk and empty") {
		t.Fatalf("retirement did not name the leftover tree: %s", body)
	}
	// Naming it must not turn into deleting it: the directory is the operator's.
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("retirement deleted the tree, which is not its job: %v", err)
	}
	if _, ok := fx.cfg.admin.Product("leaky"); ok {
		t.Fatal("registration survived retirement")
	}
}

// TestRetireRemovesProductTokens is the claim from the 2026-10-25 acceptance run
// put under test. That run read two tokens on a freshly registered product and
// concluded a retired product had left an orphan behind. It had not: registering
// a product mints its first token, and RemoveProduct drops every token for the
// product. This test pins the real behaviour so the misreading cannot come back.
func TestRetireRemovesProductTokens(t *testing.T) {
	fx := newAdminFixture(t)
	root := t.TempDir()
	status, body := fx.do(t, http.MethodPost, "/v1/admin/products", fx.admin, map[string]any{
		"id": "counted", "root": root,
	})
	if status != http.StatusOK {
		t.Fatalf("register status=%d body=%s", status, body)
	}
	// Registration itself issued one token; a second one makes the count
	// unambiguous.
	if status, body := fx.do(t, http.MethodPost, "/v1/admin/products/counted/tokens", fx.admin,
		map[string]any{"label": "second"}); status != http.StatusOK {
		t.Fatalf("issue second status=%d body=%s", status, body)
	}
	if got := len(fx.cfg.admin.Tokens("counted")); got != 2 {
		t.Fatalf("tokens before retirement = %d, want 2 (one from registration)", got)
	}

	status, body = fx.do(t, http.MethodDelete, "/v1/admin/products/counted", fx.admin, nil)
	if status != http.StatusOK {
		t.Fatalf("retire status=%d body=%s", status, body)
	}
	if got := len(fx.cfg.admin.Tokens("counted")); got != 0 {
		t.Fatalf("retirement left %d token(s) behind", got)
	}
	if got := len(fx.cfg.admin.Tokens("")); got == 0 {
		t.Fatal("retirement removed every token, including other products'")
	}
}
