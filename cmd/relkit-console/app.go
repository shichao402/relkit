package main

// Serving the rebuilt front end.
//
// The console used to answer every management path with server-rendered HTML.
// It now serves a single-page app: the shell for any path under /-/admin/ that
// is not an API call, and the built assets for /-/admin/assets/*. Old page URLs
// are real, bookmarkable paths, so a deep link and a refresh both have to land
// on the app shell rather than a 404.
//
// Requirement 7.3 keeps the old form URLs working: /-/admin/panel/products and
// friends stay valid entry points into the same app.

import (
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"

	"github.com/shichao402/relkit/cmd/relkit-console/internal/webui"
)

const (
	// assetsPrefix is where Vite puts hashed bundles. A miss here is a 404: an
	// asset request that falls back to index.html would hand the browser HTML
	// with a JavaScript content type.
	assetsPrefix = "/-/admin/assets/"

	// appPrefix is the front end's base path (vite `base`).
	appPrefix = "/-/admin/"
)

// serveApp serves the SPA shell and its assets. Everything the app owns lives
// under /-/admin/, so a path outside that prefix is not this handler's job.
func (c *console) serveApp() http.HandlerFunc {
	assets := webui.Assets()
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		// The shell must not be cached: it names the hashed bundle to load, and
		// a stale shell pins a bundle that no longer exists. Assets are hashed
		// and immutable, so they cache forever.
		if strings.HasPrefix(r.URL.Path, assetsPrefix) {
			name := strings.TrimPrefix(r.URL.Path, appPrefix)
			file, err := assets.Open(strings.TrimPrefix(name, "/"))
			if err != nil {
				http.NotFound(w, r)
				return
			}
			defer file.Close()
			if info, err := file.Stat(); err == nil && info.IsDir() {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			w.Header().Set("Content-Type", assetContentType(name))
			_, _ = io.Copy(w, file)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		index, err := fs.ReadFile(assets, "index.html")
		if err != nil {
			http.Error(w, "front end is not built into this binary", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(index)
	}
}

// assetContentType covers the extensions Vite emits. Serving them with a
// guessed type would break module loading, which browsers refuse to run when
// the type is not JavaScript.
func assetContentType(name string) string {
	switch path.Ext(name) {
	case ".js", ".mjs":
		return "text/javascript; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".svg":
		return "image/svg+xml"
	case ".woff2":
		return "font/woff2"
	case ".woff":
		return "font/woff"
	case ".json":
		return "application/json; charset=utf-8"
	case ".map":
		return "application/json; charset=utf-8"
	case ".ico":
		return "image/x-icon"
	case ".png":
		return "image/png"
	}
	return "application/octet-stream"
}

// isAppPath reports whether a request belongs to the front end rather than to
// an API route, an asset, a redirect endpoint or the health check. Only paths
// under /-/admin/ qualify: /-/latest/* and /-/health stay as they are, and
// control-plane reads are served by the store, not the console.
func isAppPath(p string) bool {
	if !strings.HasPrefix(p, appPrefix) {
		return false
	}
	if strings.HasPrefix(p, apiPrefix) || strings.HasPrefix(p, assetsPrefix) {
		return false
	}
	return true
}
