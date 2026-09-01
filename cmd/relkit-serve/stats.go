package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// statsFileName is the default counters file at the serve root.
//
// systemd only lets this process write the release tree (ReadWritePaths).
// Putting counters anywhere else would not survive a default install. The
// file is hidden from listings and refused on GET/PUT so it is not another
// publicly fetchable object and does not sit on the download hot path.
const statsFileName = ".relkit-serve-stats.json"

const statsPersistDebounce = 2 * time.Second

// downloadStats counts artifact downloads for the human-facing pages.
//
// Increments stay in memory. A debounced write copies the map to disk so a
// restart keeps the totals without a write on every GET.
type downloadStats struct {
	path     string
	serveKey string
	since    time.Time

	mu      sync.Mutex
	flushMu sync.Mutex
	counts  map[string]int64
	dirty   bool
	timer   *time.Timer
}

type persistedStats struct {
	Since  time.Time        `json:"since"`
	Counts map[string]int64 `json:"counts"`
}

func defaultStatsPath(rootPath string) string {
	return filepath.Join(rootPath, statsFileName)
}

func resolveStatsPath(rootPath, configured string) string {
	configured = strings.TrimSpace(configured)
	if configured == "" {
		return defaultStatsPath(rootPath)
	}
	if filepath.IsAbs(configured) {
		return filepath.Clean(configured)
	}
	return filepath.Join(rootPath, filepath.FromSlash(configured))
}

func statsServeKey(rootPath, statsPath string) string {
	rel, err := filepath.Rel(rootPath, statsPath)
	if err != nil {
		return ""
	}
	rel = filepath.ToSlash(rel)
	if rel == "." || rel == ".." || strings.HasPrefix(rel, "../") {
		return ""
	}
	return rel
}

func statsFileFrom(cfg *FileConfig) string {
	if cfg == nil {
		return ""
	}
	return cfg.StatsFile
}

func reservedServeKey(name string) bool {
	base := path.Base(name)
	return base == statsFileName || strings.HasPrefix(base, statsFileName+".") ||
		reservedAdminKey(name)
}

func hiddenServeKey(name string, s *downloadStats) bool {
	if reservedServeKey(name) {
		return true
	}
	if s == nil || s.serveKey == "" {
		return false
	}
	return name == s.serveKey || name == s.serveKey+".tmp~"
}

func newDownloadStats(path, rootPath string) *downloadStats {
	s := &downloadStats{path: path, since: time.Now(), counts: map[string]int64{}}
	if path == "" {
		return s
	}
	s.serveKey = statsServeKey(rootPath, path)
	if err := s.load(); err != nil {
		if os.IsNotExist(err) {
			s.dirty = true
		} else {
			log.Printf("WARNING: download stats %s: %v", path, err)
		}
	}
	return s
}

func (s *downloadStats) load() error {
	raw, err := os.ReadFile(s.path)
	if err != nil {
		return err
	}
	var doc persistedStats
	if err := json.Unmarshal(raw, &doc); err != nil {
		return fmt.Errorf("parse: %w", err)
	}
	if doc.Since.IsZero() {
		return fmt.Errorf("missing since")
	}
	counts := doc.Counts
	if counts == nil {
		counts = map[string]int64{}
	}
	s.since = doc.Since
	s.counts = counts
	s.dirty = false
	return nil
}

func (s *downloadStats) record(key string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.counts[key]++
	s.dirty = true
	s.scheduleLocked()
	s.mu.Unlock()
}

func (s *downloadStats) scheduleLocked() {
	if s.path == "" {
		return
	}
	if s.timer != nil {
		s.timer.Stop()
	}
	s.timer = time.AfterFunc(statsPersistDebounce, func() {
		if err := s.flush(); err != nil {
			log.Printf("WARNING: download stats: %v", err)
		}
	})
}

func (s *downloadStats) count(key string) int64 {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.counts[key]
}

func (s *downloadStats) totalUnder(prefix string) int64 {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var total int64
	for key, n := range s.counts {
		if strings.HasPrefix(key, prefix) {
			total += n
		}
	}
	return total
}

func (s *downloadStats) startedAt() string {
	if s == nil || s.since.IsZero() {
		return ""
	}
	return s.since.Format(stampLayout)
}

func (s *downloadStats) stop() {
	if s == nil {
		return
	}
	s.mu.Lock()
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	s.mu.Unlock()
	if err := s.flush(); err != nil {
		log.Printf("WARNING: download stats: %v", err)
	}
}

func (s *downloadStats) flush() error {
	if s == nil || s.path == "" {
		return nil
	}
	s.flushMu.Lock()
	defer s.flushMu.Unlock()

	s.mu.Lock()
	if !s.dirty {
		s.mu.Unlock()
		return nil
	}
	doc := persistedStats{
		Since:  s.since,
		Counts: make(map[string]int64, len(s.counts)),
	}
	for key, n := range s.counts {
		doc.Counts[key] = n
	}
	s.dirty = false
	s.mu.Unlock()

	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		s.markDirty()
		return err
	}
	raw = append(raw, '\n')

	tmp := s.path + ".tmp~"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		s.markDirty()
		return err
	}
	if err := os.Rename(tmp, s.path); err != nil {
		os.Remove(tmp)
		s.markDirty()
		return err
	}
	return nil
}

func (s *downloadStats) markDirty() {
	s.mu.Lock()
	s.dirty = true
	s.mu.Unlock()
}

// countableRange keeps a parallel download from counting as sixteen.
//
// Every downloader that splits a file still asks for one range beginning at
// zero, so counting only that request approximates one count per download
// without tracking connections or completion.
func countableRange(header string) bool {
	if header == "" {
		return true
	}
	return strings.HasPrefix(header, "bytes=0-")
}
