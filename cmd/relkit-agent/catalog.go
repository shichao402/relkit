package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/shichao402/relkit/internal/vnext/catalog"
	"github.com/shichao402/relkit/internal/vnext/tools"
)

// The Catalog is the second half of tool distribution: the tool manifest says
// "here are the files for this version", the Catalog says "this is the version
// the server serves right now" (requirement 12.6). Both are signed by the same
// root and both are rendered from one upload, so they cannot drift.
const catalogFile = "catalog.json"

// catalogPath is where the server keeps the one Catalog it serves. It lives in
// the state directory because it is server state, not tool bytes.
func (s *Server) catalogPath() string {
	return filepath.Join(s.cfg.StateDir, catalogFile)
}

// handleCatalog serves the signed Catalog. Host CI reads it after the tool has
// started, so the whole tool path stays on one origin (requirement 12.2).
func (s *Server) handleCatalog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	path := s.catalogPath()
	if info, err := os.Stat(path); err != nil || info.IsDir() {
		http.Error(w, "no catalog published", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	// A Catalog is mutable by design: publishing a new tool version moves
	// `current`. Revalidation keeps CI from pinning an older one.
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFile(w, r, path)
}

// writeCatalog folds one published tool manifest into the Catalog and marks its
// version current. Older releases stay listed so a host that pinned a version
// can still fetch the exact files it pinned; only `current` moves.
func (s *Server) writeCatalog(manifest tools.Document) error {
	if s.cfg.Root == nil {
		return fmt.Errorf("server root is not configured")
	}
	path := s.catalogPath()
	document := catalog.Document{Schema: catalog.Schema, Sequence: 1}
	if existing, err := catalog.Load(path); err == nil {
		document = *existing
		document.Sequence++
	}
	release := manifest.CatalogRelease()
	replaced := false
	for index := range document.Releases {
		if document.Releases[index].Version == release.Version {
			document.Releases[index] = release
			replaced = true
			break
		}
	}
	if !replaced {
		document.Releases = append(document.Releases, release)
	}
	// A server has one toolchain, so the version just published is the one
	// hosts get. Nothing else may claim `current`.
	document.Current = release.Version
	if err := document.Sign(s.cfg.Root.KeyID, s.cfg.Root.Seed); err != nil {
		return err
	}
	data, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	// Write-then-rename: a host reading mid-write must never see a truncated
	// Catalog, because a truncated signature reads as "tampered".
	temp, err := os.CreateTemp(filepath.Dir(path), ".catalog-*.json")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	if _, err := temp.Write(append(data, '\n')); err != nil {
		temp.Close()
		os.Remove(tempName)
		return err
	}
	if err := temp.Close(); err != nil {
		os.Remove(tempName)
		return err
	}
	if err := os.Chmod(tempName, 0o644); err != nil {
		os.Remove(tempName)
		return err
	}
	return os.Rename(tempName, path)
}

// toolComponentFromFile is the fallback name for an upload that predates the
// component field: the registry names a binary <prefix>-<os>-<arch>, so the
// leading prefix is the component.
func toolComponentFromFile(name string) string {
	base := strings.TrimSuffix(name, ".exe")
	if index := strings.Index(base, "-windows-"); index > 0 {
		return strings.TrimPrefix(base[:index], "relkit-")
	}
	if index := strings.Index(base, "-darwin-"); index > 0 {
		return strings.TrimPrefix(base[:index], "relkit-")
	}
	if index := strings.Index(base, "-linux-"); index > 0 {
		return strings.TrimPrefix(base[:index], "relkit-")
	}
	return strings.TrimSuffix(base, filepath.Ext(base))
}
