package updater

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	updaterv1 "go.firoyang.com/relkit/api/updater/v1"
	"go.firoyang.com/relkit/sdk"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type store struct {
	dataDir string
}

func (s store) statePath() string { return filepath.Join(s.dataDir, StateFileName) }
func (s store) jsonPath() string  { return filepath.Join(s.dataDir, LegacyStateJSON) }
func (s store) plansDir() string  { return filepath.Join(s.dataDir, PlansDirName) }
func (s store) sessionsDir() string {
	return filepath.Join(s.dataDir, SessionsDirName)
}

func (s store) ensure() error {
	for _, d := range []string{s.dataDir, s.plansDir(), s.sessionsDir()} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func atomicWrite(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (s store) loadState() (*updaterv1.PersistedState, error) {
	if err := s.ensure(); err != nil {
		return nil, err
	}
	if raw, err := os.ReadFile(s.statePath()); err == nil {
		st := &updaterv1.PersistedState{}
		if err := proto.Unmarshal(raw, st); err != nil {
			return nil, err
		}
		return st, nil
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	st, err := s.migrateLegacyJSON()
	if err != nil {
		return nil, err
	}
	if err := s.saveState(st); err != nil {
		return nil, err
	}
	return st, nil
}

func (s store) migrateLegacyJSON() (*updaterv1.PersistedState, error) {
	st := &updaterv1.PersistedState{}
	raw, err := os.ReadFile(s.jsonPath())
	if err != nil {
		return st, nil
	}
	var legacy sdk.UpdateState
	if json.Unmarshal(raw, &legacy) != nil {
		return st, nil
	}
	if legacy.LastCheckAt != nil {
		st.LastCheckAt = timestamppb.New(legacy.LastCheckAt.UTC())
	}
	st.LastResult = MapLegacyLastResult(legacy.LastResult)
	if legacy.LastSeenSequence != nil {
		st.LastSeenSequence = *legacy.LastSeenSequence
	}
	if legacy.LastSeenDirectorySequence != nil {
		st.LastSeenDirectorySequence = *legacy.LastSeenDirectorySequence
	}
	if legacy.LastSeenFallbackSequence != nil {
		st.LastSeenFallbackSequence = *legacy.LastSeenFallbackSequence
	}
	for _, c := range legacy.Skipped {
		st.SkippedCodes = append(st.SkippedCodes, int64(c))
	}
	return st, nil
}

func (s store) saveState(st *updaterv1.PersistedState) error {
	raw, err := proto.Marshal(st)
	if err != nil {
		return err
	}
	return atomicWrite(s.statePath(), raw)
}

func (s store) planPath(id string) string {
	return filepath.Join(s.plansDir(), id+".pb")
}

func (s store) savePlan(p *updaterv1.UpdatePlan) error {
	raw, err := proto.Marshal(p)
	if err != nil {
		return err
	}
	return atomicWrite(s.planPath(p.PlanId), raw)
}

func (s store) loadPlan(id string) (*updaterv1.UpdatePlan, error) {
	raw, err := os.ReadFile(s.planPath(id))
	if err != nil {
		return nil, err
	}
	p := &updaterv1.UpdatePlan{}
	if err := proto.Unmarshal(raw, p); err != nil {
		return nil, err
	}
	return p, nil
}

func (s store) sessionPath(id string) string {
	return filepath.Join(s.sessionsDir(), id+".pb")
}

func (s store) saveSession(sess *updaterv1.ApplySessionRecord) error {
	raw, err := proto.Marshal(sess)
	if err != nil {
		return err
	}
	return atomicWrite(s.sessionPath(sess.SessionId), raw)
}

func (s store) loadSession(id string) (*updaterv1.ApplySessionRecord, error) {
	raw, err := os.ReadFile(s.sessionPath(id))
	if err != nil {
		return nil, err
	}
	sess := &updaterv1.ApplySessionRecord{}
	if err := proto.Unmarshal(raw, sess); err != nil {
		return nil, err
	}
	return sess, nil
}

func (s store) latestSession() (*updaterv1.ApplySessionRecord, error) {
	ents, err := os.ReadDir(s.sessionsDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var best *updaterv1.ApplySessionRecord
	for _, e := range ents {
		if e.IsDir() || filepath.Ext(e.Name()) != ".pb" {
			continue
		}
		sess, err := s.loadSession(e.Name()[:len(e.Name())-3])
		if err != nil {
			continue
		}
		if best == nil || sess.GetStartedAt().AsTime().After(best.GetStartedAt().AsTime()) {
			best = sess
		}
	}
	return best, nil
}

func (s store) planKey() ([]byte, error) {
	path := filepath.Join(s.dataDir, PlanKeyFileName)
	if raw, err := os.ReadFile(path); err == nil && len(raw) == 32 {
		return raw, nil
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	if err := atomicWrite(path, key); err != nil {
		return nil, err
	}
	return key, nil
}

func skippedSet(st *updaterv1.PersistedState) map[int64]struct{} {
	out := map[int64]struct{}{}
	if st == nil {
		return out
	}
	for _, c := range st.SkippedCodes {
		out[c] = struct{}{}
	}
	return out
}

func lastCheckTime(st *updaterv1.PersistedState) *time.Time {
	if st == nil || st.LastCheckAt == nil {
		return nil
	}
	t := st.LastCheckAt.AsTime()
	return &t
}

func newID(prefix string) string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return fmt.Sprintf("%s-%x", prefix, b[:])
}
