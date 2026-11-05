package updater

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	enginestate "github.com/shichao402/relkit/api/updater/enginestate/v1"
	updaterv2 "github.com/shichao402/relkit/api/updater/v2"
)

// Plan lifecycle in dataDir.
//
// A plan is the engine's sealed answer to one check: plans/<planId>.pb plus,
// once downloaded, staging/<planId>/. Hosts poll check on a timer, so the
// engine must not mint a fresh plan (and a fresh download) for an answer it
// already gave. Two rules keep dataDir bounded:
//
//  1. Reuse. A check that resolves to the same bytes for the same product,
//     channel and code returns the live plan it issued before, refreshed with
//     the latest index facts and a new expiry. Its planId and staged files
//     stay valid, so a host holding the planId across rounds (Loom launcher
//     waiting for the editor to close) keeps a usable plan as long as it keeps
//     checking.
//  2. Sweep. Staged bytes of an expired plan are useless (download and apply
//     refuse it), so they go as soon as the plan expires. The plan record
//     itself lingers one more PlanTTL so a late download/apply still gets
//     PLAN_EXPIRED rather than PLAN_UNKNOWN. Staging directories without a
//     plan record are orphans. Anything a live apply session points at is
//     never touched.

// planRecordRetention is how long an expired plan record is kept so late
// requests see PLAN_EXPIRED instead of PLAN_UNKNOWN.
const planRecordRetention = PlanTTL

// sessionRetention is how long a finished session record is kept. The newest
// session is always kept: status reports it.
const sessionRetention = PlanTTL

// stagingDirName is dataDir/staging, where staging/<planId>/ holds a plan's
// downloaded files, the copied worker and the extracted payload.
const stagingDirName = "staging"

func (s store) stagingDir() string { return filepath.Join(s.dataDir, stagingDirName) }

func (s store) planStagingDir(planID string) string {
	return filepath.Join(s.stagingDir(), planID)
}

// samePlanFiles reports whether two plans deliver the same bytes. URLs are
// mirrors of those bytes and do not take part.
func samePlanFiles(a, b []*enginestate.PlannedFile) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].GetName() != b[i].GetName() ||
			a[i].GetSize() != b[i].GetSize() ||
			!strings.EqualFold(a[i].GetSha256Hex(), b[i].GetSha256Hex()) ||
			a[i].GetKind() != b[i].GetKind() {
			return false
		}
	}
	return true
}

// reusablePlan returns the live, intact plan that already answers want
// (same product, channel, code and bytes), or nil. When several qualify the
// one with the most bytes already on disk wins, then the newest.
func (s store) reusablePlan(key []byte, want *enginestate.UpdatePlan, now time.Time) *enginestate.UpdatePlan {
	ids, err := listDir(s.plansDir())
	if err != nil {
		return nil
	}
	var best *enginestate.UpdatePlan
	bestReady := -1
	for _, id := range ids {
		p, err := s.loadPlan(id)
		if err != nil || p.GetPlanId() != id || !verifyPlan(key, p) {
			continue
		}
		if p.ExpiresAt == nil || !now.Before(p.ExpiresAt.AsTime()) {
			continue
		}
		if p.Product != want.Product || p.Channel != want.Channel || p.Code != want.Code ||
			!samePlanFiles(p.Files, want.Files) {
			continue
		}
		ready := 0
		for _, f := range p.Files {
			if planFileReady(f) {
				ready++
			}
		}
		if best == nil || ready > bestReady ||
			(ready == bestReady && p.GetCreatedAt().AsTime().After(best.GetCreatedAt().AsTime())) {
			best, bestReady = p, ready
		}
	}
	return best
}

// refreshPlan carries the facts of a new check into a reused plan while
// keeping its identity and download state.
func refreshPlan(old, fresh *enginestate.UpdatePlan) {
	old.Version = fresh.Version
	old.Sequence = fresh.Sequence
	old.Mandatory = fresh.Mandatory
	old.RemainingHops = fresh.RemainingHops
	old.ReleaseNotesMarkdown = fresh.ReleaseNotesMarkdown
	old.ReleaseNotesUrl = fresh.ReleaseNotesUrl
	old.ExpiresAt = fresh.ExpiresAt
	for i, f := range old.Files {
		f.Urls = append([]string{}, fresh.Files[i].GetUrls()...)
		if f.Downloaded && !planFileReady(f) {
			f.Downloaded = false
			f.LocalPath = ""
		}
	}
}

// sessionIsLive reports whether a session may still use its plan and staged
// files: it is in a working phase and its worker has shown signs of life
// within PlanTTL. A worker that died mid-phase stops protecting its files
// once that window passes.
func sessionIsLive(sess *enginestate.ApplySessionRecord, now time.Time) bool {
	switch sess.GetPhase() {
	case updaterv2.SessionPhase_SESSION_PHASE_WAITING_FOR_EXIT,
		updaterv2.SessionPhase_SESSION_PHASE_DOWNLOADING,
		updaterv2.SessionPhase_SESSION_PHASE_COPYING,
		updaterv2.SessionPhase_SESSION_PHASE_COMMITTING,
		updaterv2.SessionPhase_SESSION_PHASE_RELAUNCHING:
	default:
		return false
	}
	return sessionLastSign(sess).After(now.Add(-PlanTTL))
}

func sessionLastSign(sess *enginestate.ApplySessionRecord) time.Time {
	if hb := sess.GetHeartbeatAt(); hb != nil {
		return hb.AsTime()
	}
	return sess.GetStartedAt().AsTime()
}

// sweep reclaims what no request can use any more: staged bytes of expired
// or unknown plans, expired plan records past their retention, stale temp
// files, and finished session records past their retention. It is best
// effort; whatever cannot be removed now (a file still open on Windows) is
// retried on the next sweep.
func (s store) sweep(now time.Time) {
	protectedPlans := map[string]struct{}{}
	protectedStaging := map[string]struct{}{}
	s.sweepSessions(now, func(sess *enginestate.ApplySessionRecord) {
		protectedPlans[sess.GetPlanId()] = struct{}{}
		if sess.GetStagedRoot() != "" {
			protectedStaging[filepath.Clean(sess.GetStagedRoot())] = struct{}{}
		}
	})
	isProtected := func(planID string) bool {
		if _, ok := protectedPlans[planID]; ok {
			return true
		}
		_, ok := protectedStaging[filepath.Clean(s.planStagingDir(planID))]
		return ok
	}

	known := map[string]struct{}{}
	if planEnts, err := os.ReadDir(s.plansDir()); err == nil {
		for _, pe := range planEnts {
			if pe.IsDir() {
				continue
			}
			name := pe.Name()
			path := filepath.Join(s.plansDir(), name)
			if strings.HasSuffix(name, ".tmp") {
				if fi, err := os.Stat(path); err == nil && now.Sub(fi.ModTime()) > PlanTTL {
					_ = os.Remove(path)
				}
				continue
			}
			id, ok := strings.CutSuffix(name, ".pb")
			if !ok {
				continue
			}
			p, err := s.loadPlan(id)
			if err != nil {
				// Unreadable records answer PLAN_UNKNOWN already.
				if !isProtected(id) {
					_ = os.Remove(path)
				}
				continue
			}
			known[id] = struct{}{}
			if isProtected(id) || p.ExpiresAt == nil {
				continue
			}
			expiry := p.ExpiresAt.AsTime()
			if now.Before(expiry) {
				continue
			}
			_ = os.RemoveAll(s.planStagingDir(id))
			if now.Sub(expiry) > planRecordRetention {
				_ = os.Remove(path)
				delete(known, id)
			}
		}
	}

	ents, err := os.ReadDir(s.stagingDir())
	if err != nil {
		return
	}
	for _, ent := range ents {
		id := ent.Name()
		if _, ok := known[id]; ok || isProtected(id) {
			continue
		}
		_ = os.RemoveAll(filepath.Join(s.stagingDir(), id))
	}
}

// sweepSessions drops finished session records past sessionRetention, keeps
// the newest record whatever its age, and calls live for every session that
// still owns its plan.
func (s store) sweepSessions(now time.Time, live func(*enginestate.ApplySessionRecord)) {
	ents, err := os.ReadDir(s.sessionsDir())
	if err != nil {
		return
	}
	type rec struct {
		path string
		sess *enginestate.ApplySessionRecord
	}
	var all []rec
	var newest *enginestate.ApplySessionRecord
	for _, ent := range ents {
		id, ok := strings.CutSuffix(ent.Name(), ".pb")
		if ent.IsDir() || !ok {
			continue
		}
		sess, err := s.loadSession(id)
		if err != nil {
			continue
		}
		all = append(all, rec{path: filepath.Join(s.sessionsDir(), ent.Name()), sess: sess})
		if newest == nil || sess.GetStartedAt().AsTime().After(newest.GetStartedAt().AsTime()) {
			newest = sess
		}
	}
	for _, r := range all {
		if sessionIsLive(r.sess, now) {
			live(r.sess)
			continue
		}
		if r.sess == newest {
			continue
		}
		if now.Sub(sessionLastSign(r.sess)) > sessionRetention {
			_ = os.Remove(r.path)
		}
	}
}
