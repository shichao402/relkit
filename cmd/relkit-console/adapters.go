package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"go.firoyang.com/relkit/internal/httpx"
)

// Adapter is the console's single read path into storage facts. Every panel
// page renders from an adapter; the console never touches a store's files or
// a backend client directly. Writes are not part of this surface: publishing,
// token rotation and GC remain agent/store jobs, which is what keeps the
// console from becoming a second publish entry.
//
// The shape deliberately mirrors internal/backends: one small interface, one
// implementation per data plane, no type-switches at call sites.
type Adapter interface {
	// Name identifies the adapter in logs and on the panel.
	Name() string
	// ReadKey reads one RUP key (index/, manifest/, latest/, site/...).
	ReadKey(key string) ([]byte, error)
	// ReadDir lists one directory of the key space.
	ReadDir(dir string) ([]Entry, error)
	// ModTime reports when a key last changed, for freshness columns.
	// Implementations without that fact return the zero time.
	ModTime(key string) time.Time
}

// Entry is one directory listing row.
type Entry struct {
	Name  string
	IsDir bool
	Size  int64
	Mtime time.Time
}

// rootAdapter reads a release tree on this box through os.Root, exactly like
// relkit-store does today. It is the "same box, read-only" case: the console
// running next to relkit-store (or a store tree mounted read-only).
type rootAdapter struct {
	name string
	root *os.Root
}

func newRootAdapter(name, dir string) (*rootAdapter, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", dir, err)
	}
	return &rootAdapter{name: name, root: root}, nil
}

func (a *rootAdapter) Name() string { return a.name }

func (a *rootAdapter) ReadKey(key string) ([]byte, error) {
	return a.root.ReadFile(key)
}

func (a *rootAdapter) ReadDir(dir string) ([]Entry, error) {
	entries, err := fs.ReadDir(a.root.FS(), dir)
	if err != nil {
		return nil, err
	}
	out := make([]Entry, 0, len(entries))
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}
		out = append(out, Entry{
			Name:  entry.Name(),
			IsDir: entry.IsDir(),
			Size:  info.Size(),
			Mtime: info.ModTime(),
		})
	}
	return out, nil
}

func (a *rootAdapter) ModTime(key string) time.Time {
	file, err := a.root.Open(key)
	if err != nil {
		return time.Time{}
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}

var _ Adapter = (*rootAdapter)(nil)

// siteStatusView is what the panel shows about the last site rebuild: the
// dump's freshness plus per-sink deploy results. It reads agent state, which
// is co-located read-only input, not the release tree itself.
type siteStatusView struct {
	StateDir string
}

func (v siteStatusView) readStatus() (siteStatus, bool) {
	if v.StateDir == "" {
		return siteStatus{}, false
	}
	data, err := os.ReadFile(filepath.Join(v.StateDir, "site", "status.json"))
	if err != nil {
		return siteStatus{}, false
	}
	var status siteStatus
	if err := json.Unmarshal(data, &status); err != nil {
		return siteStatus{}, false
	}
	return status, true
}

// siteStatus mirrors internal/site.Status for display. The console reads the
// JSON; it does not import the site package to avoid a build dependency from
// the management plane onto the rebuild engine.
type siteStatus struct {
	At    string          `json:"at"`
	Sinks []sinkStatusRow `json:"sinks"`
}

type sinkStatusRow struct {
	Name         string `json:"name"`
	OK           bool   `json:"ok"`
	DeploymentID string `json:"deploymentId,omitempty"`
}

// storeEntryJSON mirrors relkit-store's /-/list/ row shape. Duplicating four
// fields beats importing the store package: the console must not depend on
// the storage plane's binary, only on its wire format (ADR 0016).
type storeEntryJSON struct {
	Name  string `json:"name"`
	IsDir bool   `json:"isDir"`
	Size  int64  `json:"size"`
	Mtime string `json:"mtime"`
}

// storeAdapter reads a remote relkit-store over HTTP (ADR 0016 step 5): the
// console box that is not co-located with the release tree. It is the read
// half of the same wire the backends package's relkit-compatible client
// speaks: plain GET on the tree, plus the store's /-/list/ JSON endpoint for
// directory listings, which carries name/isDir/size/mtime per row.
//
// ReadKey translates httpx.Get's "404 is (nil, nil)" into rootAdapter's
// "missing file is an error": scanProducts and friends treat an error as "no
// data", and a nil body would decode as garbage.
type storeAdapter struct {
	name    string
	baseURL string // absolute, no trailing slash
}

func newStoreAdapter(name, baseURL string) (*storeAdapter, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return nil, fmt.Errorf("parse store URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("store URL must be http(s), got %q", baseURL)
	}
	if parsed.Host == "" {
		return nil, fmt.Errorf("store URL is missing a host: %q", baseURL)
	}
	return &storeAdapter{
		name:    name,
		baseURL: strings.TrimRight(parsed.String(), "/"),
	}, nil
}

func (a *storeAdapter) Name() string { return a.name }

// keyNoCache mirrors the store's no-cache prefixes (and the relkit-compatible
// client's Get): pointers and indexes change under the console between
// renders, while artifacts under a versioned prefix are immutable.
func (a *storeAdapter) keyNoCache(key string) bool {
	for _, prefix := range []string{"index/", "fallback/", "directory/", "latest/", "site/", "browse/"} {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}

func (a *storeAdapter) ReadKey(key string) ([]byte, error) {
	body, err := httpx.Get(a.baseURL+"/"+key, 60*time.Second, a.keyNoCache(key))
	if err != nil {
		return nil, err
	}
	if body == nil {
		// 404: align with rootAdapter, where os.Root.ReadFile errors on a
		// missing file rather than returning nil.
		return nil, fmt.Errorf("GET %s: not found", key)
	}
	return body, nil
}

func (a *storeAdapter) ReadDir(dir string) ([]Entry, error) {
	target := a.baseURL + "/-/list/" + strings.TrimPrefix(dir, "./")
	body, err := httpx.Get(target, 60*time.Second, true)
	if err != nil {
		return nil, err
	}
	if body == nil {
		return nil, fmt.Errorf("list %s: not found", dir)
	}
	var rows []storeEntryJSON
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("list %s: decode: %w", dir, err)
	}
	entries := make([]Entry, 0, len(rows))
	for _, row := range rows {
		entry := Entry{Name: row.Name, IsDir: row.IsDir, Size: row.Size}
		if row.Mtime != "" {
			if parsed, err := time.Parse(time.RFC3339, row.Mtime); err == nil {
				entry.Mtime = parsed
			}
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func (a *storeAdapter) ModTime(key string) time.Time {
	// The listing rows carry mtime; a one-row directory query is the cheapest
	// authoritative source. Anything unparsable degrades to the zero time,
	// which the panel renders as "no freshness shown".
	dir, base := path.Split(key)
	entries, err := a.ReadDir(strings.TrimSuffix(dir, "/"))
	if err != nil {
		return time.Time{}
	}
	for _, entry := range entries {
		if entry.Name == base {
			return entry.Mtime
		}
	}
	return time.Time{}
}

var _ Adapter = (*storeAdapter)(nil)
