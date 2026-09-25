package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
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
// relkit-serve does today. It is the "same box, read-only" case: the console
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
