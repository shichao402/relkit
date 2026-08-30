package main

import (
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path"
	"sort"
	"strings"
	"time"

	rupv2 "cnb.cool/shichao402/relkit/api/rup/v2"
	"cnb.cool/shichao402/relkit/internal/publishproto"
	"google.golang.org/protobuf/proto"
)

const healthPath = "/-/health"

func (c *config) handler() http.Handler {
	mux := http.NewServeMux()

	// Under /-/ so that it can never shadow a served file. RUP keys begin with
	// index/, manifest/ or artifact/, and a leading dash is not a legal
	// identifier, so the namespace is free.
	mux.HandleFunc(healthPath, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "application/protobuf")
		body, err := proto.Marshal(&rupv2.Health{
			Status:  "ok",
			Version: version,
		})
		if err != nil {
			http.Error(w, "health marshal failed", http.StatusInternalServerError)
			return
		}
		w.Write(body)
	})
	mux.HandleFunc(publishproto.PreflightPath, c.servePublishPreflight)

	mux.HandleFunc(productPathPrefix, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		c.serveProduct(w, r)
	})
	mux.HandleFunc(latestPathPrefix, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		c.serveLatest(w, r)
	})
	mux.HandleFunc(adminPath, c.serveAdmin)
	mux.HandleFunc(adminFilesPath, c.serveAdminFiles)

	mux.HandleFunc("/", c.serve)

	return c.withLogging(mux)
}

func (c *config) serve(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet, http.MethodHead:
		c.download(w, r)
	case http.MethodPut:
		c.upload(w, r)
	default:
		w.Header().Set("Allow", allowedMethods(c.uploadsEnabled()))
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func allowedMethods(uploads bool) string {
	if uploads {
		return "GET, HEAD, PUT"
	}
	return "GET, HEAD"
}

func (c *config) download(w http.ResponseWriter, r *http.Request) {
	name, ok := cleanKey(r.URL.Path)
	if !ok || hiddenServeKey(name, c.stats) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	// os.Root confines every operation to the served directory, including
	// through symlinks, which is the part a manual prefix check gets wrong.
	file, err := c.root.Open(name)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if info.IsDir() {
		// Directory browsing is for operators checking what is on the box.
		// Protocol clients still fetch only the signed index.
		if name != "." && !strings.HasSuffix(r.URL.Path, "/") {
			http.Redirect(w, r, r.URL.Path+"/", http.StatusMovedPermanently)
			return
		}
		if name == "." {
			if r.URL.Query().Has("files") {
				http.Redirect(w, r, adminFilesPath, http.StatusMovedPermanently)
				return
			}
			if c.serveExistingFile(w, r, "browse/index.html") {
				return
			}
			c.serveCatalogStub(w, r)
			return
		}
		if name == "browse" && c.serveExistingFile(w, r, "browse/index.html") {
			return
		}
		c.listDir(w, r, file, name)
		return
	}

	w.Header().Set("Cache-Control", c.cacheControl(name))
	if ct := contentType(name); ct != "" {
		w.Header().Set("Content-Type", ct)
	}

	if r.Method == http.MethodGet && strings.HasPrefix(name, "artifact/") &&
		countableRange(r.Header.Get("Range")) {
		c.stats.record(name)
	}

	// ServeContent handles Range, If-Range, If-Modified-Since and 206
	// responses. Passing the *os.File rather than a wrapper is what lets the
	// copy reach sendfile(2) on Linux.
	http.ServeContent(w, r, info.Name(), info.ModTime(), file)
}

func (c *config) serveExistingFile(w http.ResponseWriter, r *http.Request, name string) bool {
	file, err := c.root.Open(name)
	if err != nil {
		return false
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || info.IsDir() {
		return false
	}
	w.Header().Set("Cache-Control", "no-cache")
	if ct := contentType(name); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	http.ServeContent(w, r, info.Name(), info.ModTime(), file)
	return true
}

const catalogStubHTML = `<!DOCTYPE html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Releases</title>
<style>body{margin:0;font:16px/1.5 system-ui,sans-serif;padding:2rem 1rem;color:#1c1917;background:#f6f5f1}.wrap{max-width:40rem;margin:0 auto}.sub{color:#57534e}</style>
</head><body><div class="wrap">
<h1>Releases</h1>
<p class="sub">No published catalog yet. Protocol clients do not read this page.</p>
<p class="sub"><a href="/-/admin">Operator panel</a></p>
</div></body></html>
`

func (c *config) serveCatalogStub(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}
	w.Write([]byte(catalogStubHTML))
}

// listDir renders one directory: name, size, mtime, nothing else.
//
// This is the operator view of the raw key space, reached from the portal or
// by pasting a path into a browser. It stays deliberately dull -- no icons, no
// previews, no JavaScript -- because its job is to answer "is the file on the
// box" and nothing more.
func (c *config) listDir(w http.ResponseWriter, r *http.Request, dir *os.File, name string) {
	entries, err := dir.ReadDir(-1)
	if err != nil {
		http.Error(w, "cannot read directory", http.StatusInternalServerError)
		return
	}

	sort.Slice(entries, func(i, j int) bool {
		// Directories first, then alphabetical. Operators scan for product
		// folders more often than for individual files.
		if entries[i].IsDir() != entries[j].IsDir() {
			return entries[i].IsDir()
		}
		return entries[i].Name() < entries[j].Name()
	})

	display := "/"
	if name != "." {
		display = "/" + name + "/"
	}

	visible := 0
	for _, entry := range entries {
		if !hiddenServeKey(entryKey(name, entry.Name()), c.stats) {
			visible++
		}
	}

	page := &listingPage{
		pageChrome: pageChrome{
			Title:   display,
			Version: version,
			Note:    plural(visible, "entry", "entries"),
			Crumbs:  breadcrumbs(name),
		},
		Display: display,
		Count:   visible,
	}
	if name != "." {
		page.Parent = "../"
	}

	for _, entry := range entries {
		if hiddenServeKey(entryKey(name, entry.Name()), c.stats) {
			continue
		}
		row := listingEntry{
			Name: entry.Name(),
			Href: entry.Name(),
			Size: "-",
			Dir:  entry.IsDir(),
		}
		if row.Dir {
			row.Name += "/"
			row.Href += "/"
		}
		if name == "." {
			row.Href = "/" + row.Href
		}
		if info, err := entry.Info(); err == nil {
			if !row.Dir {
				row.Size = humanBytes(info.Size())
			}
			row.Mtime = info.ModTime().Format(stampLayout)
		}
		page.Entries = append(page.Entries, row)
	}

	c.renderPage(w, r, "listing", page)
}

// cacheControl distinguishes the mutable pointer from immutable content.
//
// This is the one thing a general-purpose file server cannot do for a release
// tree, because it has no way to know which path is which. Caching the index is
// what makes a finished release look "not published yet" for as long as the
// CDN or proxy holds it, while re-validating an artifact on every request
// wastes the one thing here that is actually large.
func (c *config) cacheControl(name string) string {
	for _, prefix := range c.noCache {
		if strings.HasPrefix(name, prefix) {
			return "no-cache, must-revalidate"
		}
	}
	for _, prefix := range c.immutable {
		if strings.HasPrefix(name, prefix) {
			return "public, max-age=31536000, immutable"
		}
	}
	if c.defaultMaxAge <= 0 {
		return "no-cache"
	}
	return "public, max-age=" + itoa(c.defaultMaxAge)
}

func contentType(name string) string {
	switch path.Ext(name) {
	case ".html":
		return "text/html; charset=utf-8"
	case ".pb":
		return "application/protobuf"
	case ".json":
		return "application/json"
	case ".zip":
		return "application/zip"
	case ".gz", ".tgz":
		return "application/gzip"
	case ".exe", ".dmg", ".apk", ".bin":
		return "application/octet-stream"
	}
	return ""
}

// cleanKey turns a request path into a relative key, rejecting anything that
// tries to climb out. os.Root would refuse these anyway; rejecting here keeps
// the traversal attempt out of the logs as a 404 rather than an open error.
//
// The serve root itself maps to "." so that GET / can list the release tree.
func entryKey(dir, name string) string {
	if dir == "." {
		return name
	}
	return path.Join(dir, name)
}

func cleanKey(urlPath string) (string, bool) {
	trimmed := strings.TrimPrefix(urlPath, "/")
	if trimmed == "" {
		return ".", true
	}
	cleaned := path.Clean(trimmed)
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", false
	}
	if strings.ContainsRune(cleaned, 0) {
		return "", false
	}
	// path.Clean("/") becomes ".", which would collapse "/foo/.." to the
	// serve root. That is intentional and safe inside os.Root.
	return cleaned, true
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits [20]byte
	i := len(digits)
	for n > 0 {
		i--
		digits[i] = byte('0' + n%10)
		n /= 10
	}
	return string(digits[i:])
}

type recorder struct {
	http.ResponseWriter
	status int
	bytes  int64
}

func (r *recorder) WriteHeader(code int) {
	if r.status == 0 {
		r.status = code
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *recorder) Write(p []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	n, err := r.ResponseWriter.Write(p)
	r.bytes += int64(n)
	return n, err
}

// ReadFrom keeps the zero-copy path available.
//
// net/http hands the response body to sendfile(2) only when the ResponseWriter
// implements io.ReaderFrom. Wrapping a ResponseWriter to count bytes therefore
// silently disables it and routes every byte of every download through user
// space -- a large regression that no test would notice, since the bytes are
// still correct. Forwarding ReadFrom keeps both the counter and the fast path.
func (r *recorder) ReadFrom(src io.Reader) (int64, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	rf, ok := r.ResponseWriter.(io.ReaderFrom)
	if !ok {
		n, err := io.Copy(r.ResponseWriter, src)
		r.bytes += n
		return n, err
	}
	n, err := rf.ReadFrom(src)
	r.bytes += n
	return n, err
}

// Unwrap lets http.NewResponseController reach the real ResponseWriter.
func (r *recorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

func (c *config) withLogging(next http.Handler) http.Handler {
	if !c.logRequests {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &recorder{ResponseWriter: w}
		next.ServeHTTP(rec, r)

		// Range is logged because it is the signal that a client is downloading
		// in parallel, and its absence on a slow transfer is the first thing to
		// check when someone reports that downloads are slow.
		extra := ""
		if rng := r.Header.Get("Range"); rng != "" {
			extra = " range=" + rng
		}
		log.Printf("%s %s %s %d %s %s%s",
			clientIP(r), r.Method, r.URL.Path, rec.status,
			humanBytes(rec.bytes), time.Since(start).Round(time.Millisecond), extra)
	})
}

func clientIP(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}
