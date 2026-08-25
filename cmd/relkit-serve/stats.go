package main

import (
	"strings"
	"sync"
	"time"
)

// downloadStats counts artifact downloads for the human-facing pages.
//
// Deliberately in memory only. The one directory this process may write to is
// the release tree itself, and putting a counter file there would mean a write
// on the hot download path, a file that the listing exposes, and one more thing
// that can be corrupted while a publish is in flight. Counters therefore reset
// on restart, and every page that shows them says so.
type downloadStats struct {
	since  time.Time
	mu     sync.Mutex
	counts map[string]int64
}

func newDownloadStats() *downloadStats {
	return &downloadStats{since: time.Now(), counts: map[string]int64{}}
}

func (s *downloadStats) record(key string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.counts[key]++
	s.mu.Unlock()
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
