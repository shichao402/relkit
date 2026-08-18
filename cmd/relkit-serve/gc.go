package main

import (
	"fmt"
	"io/fs"
	"log"
	"net/url"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	rupv2 "cnb.cool/shichao402/relkit/api/rup/v2"
)

const (
	defaultGCInterval = time.Hour
	defaultGCDebounce = 2 * time.Second
)

// gcState schedules orphan cleanup. The server never rewrites index files; it
// only deletes manifest/ and artifact/ objects that no index still references.
type gcState struct {
	enabled  bool
	interval time.Duration
	debounce time.Duration

	runMu     sync.Mutex
	schedMu   sync.Mutex
	debouncer *time.Timer
}

func newGCState(enabled bool, interval, debounce time.Duration) *gcState {
	if debounce <= 0 {
		debounce = defaultGCDebounce
	}
	return &gcState{
		enabled:  enabled,
		interval: interval,
		debounce: debounce,
	}
}

func (c *config) scheduleGC() {
	if c.gc == nil || !c.gc.enabled {
		return
	}
	c.gc.schedMu.Lock()
	defer c.gc.schedMu.Unlock()
	if c.gc.debouncer != nil {
		c.gc.debouncer.Stop()
	}
	c.gc.debouncer = time.AfterFunc(c.gc.debounce, func() {
		c.runGC("index-put")
	})
}

func (c *config) startGCLoop(stop <-chan struct{}) {
	if c.gc == nil || !c.gc.enabled || c.gc.interval <= 0 {
		return
	}
	ticker := time.NewTicker(c.gc.interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				c.runGC("timer")
			}
		}
	}()
}

func (c *config) stopGC() {
	if c.gc == nil {
		return
	}
	c.gc.schedMu.Lock()
	if c.gc.debouncer != nil {
		c.gc.debouncer.Stop()
		c.gc.debouncer = nil
	}
	c.gc.schedMu.Unlock()
}

func (c *config) runGC(reason string) {
	if c.gc == nil || !c.gc.enabled {
		return
	}
	if !c.gc.runMu.TryLock() {
		log.Printf("gc: skipped (%s), already running", reason)
		return
	}
	defer c.gc.runMu.Unlock()

	result, err := c.gcOnce()
	if err != nil {
		log.Printf("gc: aborted (%s): %v", reason, err)
		return
	}
	log.Printf("gc: %s removed %d file(s), %d dir(s); kept %d live object(s)",
		reason, result.filesRemoved, result.dirsRemoved, result.live)
}

type gcResult struct {
	live         int
	filesRemoved int
	dirsRemoved  int
}

func (c *config) gcOnce() (gcResult, error) {
	indexes, err := listFilesUnder(c.root, "index")
	if err != nil {
		return gcResult{}, err
	}
	if len(indexes) == 0 {
		return gcResult{}, fmt.Errorf("no index files found")
	}

	live := map[string]struct{}{}
	parsed := 0
	var parseErrors []string
	for _, name := range indexes {
		if strings.HasSuffix(name, ".tmp~") {
			continue
		}
		manifestURLs, err := readIndexManifestURLs(c.root, name)
		if err != nil {
			parseErrors = append(parseErrors, fmt.Sprintf("%s: %v", name, err))
			continue
		}
		parsed++
		for _, raw := range manifestURLs {
			if key, ok := localKeyFromURL(raw); ok {
				live[key] = struct{}{}
			}
		}
	}
	if parsed == 0 {
		return gcResult{}, fmt.Errorf("all %d index file(s) failed to parse (%s)",
			len(indexes), strings.Join(parseErrors, "; "))
	}
	if len(live) == 0 {
		return gcResult{}, fmt.Errorf("parsed %d index file(s) but found no local manifest references", parsed)
	}

	// Expand each live local manifest into its artifact keys. A missing or
	// broken referenced manifest aborts the round so we never delete the
	// artifacts a still-published index expects.
	manifestKeys := make([]string, 0, len(live))
	for key := range live {
		if strings.HasPrefix(key, "manifest/") {
			manifestKeys = append(manifestKeys, key)
		}
	}
	for _, key := range manifestKeys {
		artifactURLs, err := readManifestArtifactURLs(c.root, key)
		if err != nil {
			return gcResult{}, fmt.Errorf("referenced manifest %s: %w", key, err)
		}
		for _, raw := range artifactURLs {
			if art, ok := localKeyFromURL(raw); ok {
				live[art] = struct{}{}
			}
		}
	}

	var result gcResult
	result.live = len(live)

	for _, prefix := range []string{"manifest", "artifact"} {
		files, err := listFilesUnder(c.root, prefix)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return result, err
		}
		for _, name := range files {
			if strings.HasSuffix(name, ".tmp~") {
				if err := c.root.Remove(name); err == nil {
					result.filesRemoved++
				}
				continue
			}
			if _, keep := live[name]; keep {
				continue
			}
			if err := c.root.Remove(name); err != nil {
				log.Printf("gc: remove %s: %v", name, err)
				continue
			}
			result.filesRemoved++
		}
		removed, err := removeEmptyDirs(c.root, prefix)
		if err != nil && !os.IsNotExist(err) {
			return result, err
		}
		result.dirsRemoved += removed
	}
	return result, nil
}

func listFilesUnder(root *os.Root, prefix string) ([]string, error) {
	if _, err := root.Stat(prefix); err != nil {
		return nil, err
	}
	var out []string
	err := fs.WalkDir(root.FS(), prefix, func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		out = append(out, name)
		return nil
	})
	return out, err
}

func removeEmptyDirs(root *os.Root, prefix string) (int, error) {
	if _, err := root.Stat(prefix); err != nil {
		return 0, err
	}
	var dirs []string
	err := fs.WalkDir(root.FS(), prefix, func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && name != prefix {
			dirs = append(dirs, name)
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	removed := 0
	for i := len(dirs) - 1; i >= 0; i-- {
		name := dirs[i]
		entries, err := fs.ReadDir(root.FS(), name)
		if err != nil || len(entries) > 0 {
			continue
		}
		if err := root.Remove(name); err != nil {
			continue
		}
		removed++
	}
	return removed, nil
}

func localKeyFromURL(raw string) (string, bool) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", false
	}
	key := strings.TrimPrefix(path.Clean(u.Path), "/")
	if key == "" || key == "." || key == ".." || strings.HasPrefix(key, "../") {
		return "", false
	}
	if strings.HasPrefix(key, "manifest/") || strings.HasPrefix(key, "artifact/") {
		return key, true
	}
	return "", false
}

func readIndexManifestURLs(root *os.Root, name string) ([]string, error) {
	raw, err := root.ReadFile(name)
	if err != nil {
		return nil, err
	}
	env, err := rupv2.UnmarshalEnvelope(raw)
	if err != nil {
		return nil, fmt.Errorf("envelope protobuf: %w", err)
	}
	if env.Schema != rupv2.SchemaEnvelope {
		return nil, fmt.Errorf("unexpected envelope schema %q", env.Schema)
	}
	index, err := rupv2.UnmarshalIndex(env.Payload)
	if err != nil {
		return nil, fmt.Errorf("index payload: %w", err)
	}
	if len(index.Versions) == 0 {
		return nil, fmt.Errorf("index has no versions")
	}
	var urls []string
	for _, v := range index.Versions {
		if v == nil || v.Manifest == nil {
			continue
		}
		urls = append(urls, v.Manifest.Urls...)
	}
	if len(urls) == 0 {
		return nil, fmt.Errorf("index versions have no manifest urls")
	}
	return urls, nil
}

func readManifestArtifactURLs(root *os.Root, name string) ([]string, error) {
	raw, err := root.ReadFile(name)
	if err != nil {
		return nil, err
	}
	doc, err := rupv2.UnmarshalManifest(raw)
	if err != nil {
		return nil, fmt.Errorf("manifest protobuf: %w", err)
	}
	if len(doc.Artifacts) == 0 {
		return nil, fmt.Errorf("manifest has no artifacts")
	}
	var urls []string
	for _, a := range doc.Artifacts {
		if a == nil {
			continue
		}
		urls = append(urls, a.Urls...)
	}
	if len(urls) == 0 {
		return nil, fmt.Errorf("manifest artifacts have no urls")
	}
	return urls, nil
}
