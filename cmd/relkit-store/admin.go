package main

import (
	"embed"
	"html/template"
	"log"
	"net/http"
	"path"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// Operator-panel authentication lives in relkit-console now. The store keeps
// only the pieces the storage plane still needs: the hidden-key rule (the
// admin state file sits in the tree and must never be listable or fetchable)
// and the shared template machinery for directory listings.
//
// The admin state file itself is owned by console; a store box that migrated
// from serve keeps the file untouched and hidden here the same way.

const (
	adminStateFileName = ".relkit-serve-admin.json"
	passwordCost       = bcrypt.DefaultCost
)

func reservedAdminKey(name string) bool {
	base := path.Base(name)
	return base == adminStateFileName ||
		base == "admin.json" ||
		strings.HasPrefix(base, adminStateFileName+".") ||
		strings.HasPrefix(base, "admin.json.")
}

// hiddenKey carries the hiding rule over the store boundary: the counters
// file, the CAS key and any admin state are internal objects, not servable
// or listable content.
func (c *config) hiddenKey(name string) bool {
	return hiddenServeKey(name, c.stats) || reservedAdminKey(name)
}

// ---- template machinery (shared shape with console's ui.go) ----

const stampLayout = "2006-01-02 15:04"

type crumb struct {
	Label string
	Href  string
}

type pageChrome struct {
	Title   string
	Version string
	Note    string
	Crumbs  []crumb
}

type listingEntry struct {
	Name  string
	Href  string
	Size  string
	Mtime string
	Dir   bool
}

type listingPage struct {
	pageChrome
	Display string
	Count   int
	Parent  string
	Entries []listingEntry
}

//go:embed templates/*.html
var templateFS embed.FS

// Directory-listing HTML lives in templates/, embedded so a store binary needs
// nothing beside it on disk.
var pageTemplates = template.Must(template.New("pages").ParseFS(templateFS, "templates/*.html"))

func (c *config) renderPage(w http.ResponseWriter, r *http.Request, name string, page any) {
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}
	if err := pageTemplates.ExecuteTemplate(w, name, page); err != nil {
		log.Printf("template %s: %v", name, err)
	}
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func breadcrumbs(name string) []crumb {
	if name == "." {
		return nil
	}
	parts := strings.Split(name, "/")
	crumbs := make([]crumb, 0, len(parts)+1)
	crumbs = append(crumbs, crumb{Label: "root", Href: "/"})
	for i, part := range parts {
		href := "/" + strings.Join(parts[:i+1], "/") + "/"
		crumbs = append(crumbs, crumb{Label: part, Href: href})
	}
	return crumbs
}

// hashPassword exists here only so config_test helpers can build fixtures; it
// is the same bcrypt call console uses.
func hashPassword(password string) ([]byte, error) {
	return bcrypt.GenerateFromPassword([]byte(password), passwordCost)
}
