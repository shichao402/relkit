package sdk

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"
)

// AcceptsSequence reports whether an index/directory/fallback sequence may be
// adopted (SPEC §12.4 / §16.2). Equal is accepted; strictly smaller is refused.
func AcceptsSequence(sequence int64, lastSeen *int64) bool {
	if lastSeen == nil {
		return true
	}
	return sequence >= *lastSeen
}

// SourceStat tracks outcomes for one candidate source key (URL or service:<id>).
type SourceStat struct {
	Successes            int        `json:"successes"`
	Failures             int        `json:"failures"`
	ConsecutiveFailures  int        `json:"consecutiveFailures"`
	LastBytesPerSecond   *int       `json:"lastBytesPerSecond,omitempty"`
	LastSuccessAt        *time.Time `json:"lastSuccessAt,omitempty"`
	LastFailureAt        *time.Time `json:"lastFailureAt,omitempty"`
}

// UpdateState is what the client remembers between runs, per (product, channel).
type UpdateState struct {
	LastCheckAt               *time.Time            `json:"lastCheckAt,omitempty"`
	LastResult                string                `json:"lastResult,omitempty"`
	LastSeenSequence          *int64                `json:"lastSeenSequence,omitempty"`
	LastSeenFallbackSequence  *int64                `json:"lastSeenFallbackSequence,omitempty"`
	LastSeenDirectorySequence *int64                `json:"lastSeenDirectorySequence,omitempty"`
	LastSuccessfulSourceKey   string                `json:"lastSuccessfulSourceKey,omitempty"`
	SourceStats               map[string]*SourceStat `json:"sourceStats,omitempty"`
	Skipped                   []int                 `json:"skipped,omitempty"`
}

func (s *UpdateState) ensureStats() {
	if s.SourceStats == nil {
		s.SourceStats = map[string]*SourceStat{}
	}
}

// ObserveSequence raises the index sequence high-water mark.
func (s *UpdateState) ObserveSequence(sequence int64) {
	if s.LastSeenSequence == nil || sequence > *s.LastSeenSequence {
		seq := sequence
		s.LastSeenSequence = &seq
	}
}

// ObserveFallbackSequence raises the fallback sequence high-water mark.
func (s *UpdateState) ObserveFallbackSequence(sequence int64) {
	if s.LastSeenFallbackSequence == nil || sequence > *s.LastSeenFallbackSequence {
		seq := sequence
		s.LastSeenFallbackSequence = &seq
	}
}

// ObserveDirectorySequence raises the directory sequence high-water mark.
func (s *UpdateState) ObserveDirectorySequence(sequence int64) {
	if s.LastSeenDirectorySequence == nil || sequence > *s.LastSeenDirectorySequence {
		seq := sequence
		s.LastSeenDirectorySequence = &seq
	}
}

// RecordSourceSuccess records a real successful attempt (SPEC §12.7).
func (s *UpdateState) RecordSourceSuccess(key string, bytesPerSecond int) {
	if key == "" {
		return
	}
	s.ensureStats()
	stat := s.SourceStats[key]
	if stat == nil {
		stat = &SourceStat{}
		s.SourceStats[key] = stat
	}
	stat.Successes++
	stat.ConsecutiveFailures = 0
	now := time.Now().UTC()
	stat.LastSuccessAt = &now
	if bytesPerSecond > 0 {
		bps := bytesPerSecond
		stat.LastBytesPerSecond = &bps
	}
	s.LastSuccessfulSourceKey = key
}

// RecordSourceFailure records a real failed attempt (SPEC §12.7).
func (s *UpdateState) RecordSourceFailure(key string) {
	if key == "" {
		return
	}
	s.ensureStats()
	stat := s.SourceStats[key]
	if stat == nil {
		stat = &SourceStat{}
		s.SourceStats[key] = stat
	}
	stat.Failures++
	stat.ConsecutiveFailures++
	now := time.Now().UTC()
	stat.LastFailureAt = &now
}

// IsSkipped reports whether code is in the user-skip list.
func (s *UpdateState) IsSkipped(code int) bool {
	for _, c := range s.Skipped {
		if c == code {
			return true
		}
	}
	return false
}

// Skip adds a code to the skip list (no-op for duplicates).
func (s *UpdateState) Skip(code int) {
	if s.IsSkipped(code) {
		return
	}
	s.Skipped = append(s.Skipped, code)
	sort.Ints(s.Skipped)
}

// StateStore loads and saves UpdateState.
type StateStore interface {
	Load() (*UpdateState, error)
	Save(*UpdateState) error
}

var sanitizeName = regexp.MustCompile(`[^A-Za-z0-9._-]`)

// FileStateStore keeps state in rup-state-<product>-<channel>.json.
type FileStateStore struct {
	Path string
}

// NewFileStateStore builds a store under dir for the given product/channel.
func NewFileStateStore(dir, product, channel string) *FileStateStore {
	name := "rup-state-" + sanitizeName.ReplaceAllString(product, "_") + "-" + sanitizeName.ReplaceAllString(channel, "_") + ".json"
	return &FileStateStore{Path: filepath.Join(dir, name)}
}

// Load returns empty state when the file is missing or corrupt.
func (s *FileStateStore) Load() (*UpdateState, error) {
	data, err := os.ReadFile(s.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return &UpdateState{}, nil
		}
		return &UpdateState{}, nil
	}
	var st UpdateState
	if err := json.Unmarshal(data, &st); err != nil {
		return &UpdateState{}, nil
	}
	if st.SourceStats == nil {
		st.SourceStats = map[string]*SourceStat{}
	}
	return &st, nil
}

// Save writes state atomically via a temp file rename.
func (s *FileStateStore) Save(state *UpdateState) error {
	if state == nil {
		state = &UpdateState{}
	}
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.Path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.Path)
}

// MemoryStateStore keeps state in memory (tests / hosts without a file).
type MemoryStateStore struct {
	state *UpdateState
}

// NewMemoryStateStore returns a store with optional initial state.
func NewMemoryStateStore(initial *UpdateState) *MemoryStateStore {
	if initial == nil {
		initial = &UpdateState{}
	}
	return &MemoryStateStore{state: initial}
}

// Load returns the in-memory state.
func (s *MemoryStateStore) Load() (*UpdateState, error) {
	if s.state == nil {
		s.state = &UpdateState{}
	}
	return s.state, nil
}

// Save replaces the in-memory state.
func (s *MemoryStateStore) Save(state *UpdateState) error {
	if state == nil {
		state = &UpdateState{}
	}
	s.state = state
	return nil
}
