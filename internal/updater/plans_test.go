package updater

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	rupv2 "github.com/shichao402/relkit/api/rup/v2"
	enginestate "github.com/shichao402/relkit/api/updater/enginestate/v1"
	updaterv2 "github.com/shichao402/relkit/api/updater/v2"
	"github.com/shichao402/relkit/internal/inprocess"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func planCheckResult(code int64, sha string, urls ...string) inprocess.CheckResult {
	return inprocess.CheckResult{Available: &inprocess.UpdateAvailable{
		Target:   &rupv2.VersionNode{Code: code, Version: "1.0.0+" + itoa(code)},
		Sequence: 1000 + code,
		Artifact: &rupv2.Artifact{
			Filename: "app.relkit-payload.zip",
			Size:     4,
			Sha256:   sha,
			Urls:     urls,
			Kind:     rupv2.ArtifactKind_ARTIFACT_KIND_PAYLOAD,
		},
	}}
}

func planTestStore(t *testing.T, channel string) store {
	t.Helper()
	st := store{dataDir: t.TempDir(), product: "demo", channel: channel}
	if err := st.ensure(); err != nil {
		t.Fatal(err)
	}
	return st
}

func buildTestPlan(t *testing.T, st store, result inprocess.CheckResult) *enginestate.UpdatePlan {
	t.Helper()
	e := &Engine{}
	plan, err := e.buildPlan(st, &openedProfile{Product: st.product}, &updaterv2.Runtime{Channel: st.channel}, result)
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func planFileCount(t *testing.T, st store) int {
	t.Helper()
	ids, err := listDir(st.plansDir())
	if err != nil {
		t.Fatal(err)
	}
	return len(ids)
}

const shaA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
const shaB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

// A host polling check must get the same plan, not a new one per poll.
func TestRepeatedCheckReusesPlanAndKeepsDownload(t *testing.T) {
	st := planTestStore(t, "stable")
	first := buildTestPlan(t, st, planCheckResult(7, shaA, "https://a/1"))

	staged := st.planStagingDir(first.PlanId)
	if err := os.MkdirAll(staged, 0o755); err != nil {
		t.Fatal(err)
	}
	local := filepath.Join(staged, "app.relkit-payload.zip")
	if err := os.WriteFile(local, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	first.Files[0].Downloaded = true
	first.Files[0].LocalPath = local
	key, _ := st.planKey()
	first.ExpiresAt = timestamppb.New(time.Now().Add(time.Hour))
	first.PlanHmac = hmacPlan(key, first)
	if err := st.savePlan(first); err != nil {
		t.Fatal(err)
	}

	second := buildTestPlan(t, st, planCheckResult(7, shaA, "https://b/1"))
	if second.PlanId != first.PlanId {
		t.Fatalf("plan not reused: %s then %s", first.PlanId, second.PlanId)
	}
	if n := planFileCount(t, st); n != 1 {
		t.Fatalf("plans on disk = %d, want 1", n)
	}
	got, err := st.loadPlan(second.PlanId)
	if err != nil {
		t.Fatal(err)
	}
	if !verifyPlan(key, got) {
		t.Fatal("reused plan is not resealed")
	}
	if !planDownloaded(got) {
		t.Fatal("reused plan lost its download")
	}
	if u := got.Files[0].Urls; len(u) != 1 || u[0] != "https://b/1" {
		t.Fatalf("urls not refreshed: %v", u)
	}
	if !got.ExpiresAt.AsTime().After(time.Now().Add(PlanTTL - time.Minute)) {
		t.Fatalf("expiry not refreshed: %v", got.ExpiresAt.AsTime())
	}
}

func TestReuseForgetsFilesThatVanished(t *testing.T) {
	st := planTestStore(t, "stable")
	first := buildTestPlan(t, st, planCheckResult(7, shaA))
	first.Files[0].Downloaded = true
	first.Files[0].LocalPath = filepath.Join(st.planStagingDir(first.PlanId), "gone.zip")
	key, _ := st.planKey()
	first.PlanHmac = hmacPlan(key, first)
	if err := st.savePlan(first); err != nil {
		t.Fatal(err)
	}
	second := buildTestPlan(t, st, planCheckResult(7, shaA))
	if second.PlanId != first.PlanId {
		t.Fatal("plan not reused")
	}
	if f := second.Files[0]; f.Downloaded || f.LocalPath != "" {
		t.Fatalf("stale download state kept: %+v", f)
	}
}

func TestPlanIsNotReusedAcrossTargets(t *testing.T) {
	st := planTestStore(t, "stable")
	base := buildTestPlan(t, st, planCheckResult(7, shaA))
	if p := buildTestPlan(t, st, planCheckResult(8, shaA)); p.PlanId == base.PlanId {
		t.Fatal("reused across codes")
	}
	if p := buildTestPlan(t, st, planCheckResult(7, shaB)); p.PlanId == base.PlanId {
		t.Fatal("reused across bytes")
	}
	other := st
	other.channel = "dev"
	if p := buildTestPlan(t, other, planCheckResult(7, shaA)); p.PlanId == base.PlanId {
		t.Fatal("reused across channels")
	}
	other = st
	other.product = "other"
	if p := buildTestPlan(t, other, planCheckResult(7, shaA)); p.PlanId == base.PlanId {
		t.Fatal("reused across products")
	}
}

func TestExpiredOrTamperedPlanIsNotReused(t *testing.T) {
	st := planTestStore(t, "stable")
	key, _ := st.planKey()

	expired := buildTestPlan(t, st, planCheckResult(7, shaA))
	expired.ExpiresAt = timestamppb.New(time.Now().Add(-time.Minute))
	expired.PlanHmac = hmacPlan(key, expired)
	if err := st.savePlan(expired); err != nil {
		t.Fatal(err)
	}
	fresh := buildTestPlan(t, st, planCheckResult(7, shaA))
	if fresh.PlanId == expired.PlanId {
		t.Fatal("expired plan reused")
	}

	fresh.Mandatory = !fresh.Mandatory // break the seal
	if err := st.savePlan(fresh); err != nil {
		t.Fatal(err)
	}
	if p := buildTestPlan(t, st, planCheckResult(7, shaA)); p.PlanId == fresh.PlanId {
		t.Fatal("tampered plan reused")
	}
}

func sweepPlan(t *testing.T, st store, expiresIn time.Duration) string {
	t.Helper()
	key, _ := st.planKey()
	p := &enginestate.UpdatePlan{
		PlanId:    newID("plan"),
		Product:   st.product,
		Channel:   st.channel,
		CreatedAt: timestamppb.Now(),
		ExpiresAt: timestamppb.New(time.Now().Add(expiresIn)),
	}
	p.PlanHmac = hmacPlan(key, p)
	if err := st.savePlan(p); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(st.planStagingDir(p.PlanId), 0o755); err != nil {
		t.Fatal(err)
	}
	return p.PlanId
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func TestSweepReclaimsExpiredAndOrphanedStaging(t *testing.T) {
	st := planTestStore(t, "stable")
	live := sweepPlan(t, st, time.Hour)
	expired := sweepPlan(t, st, -time.Hour)
	ancient := sweepPlan(t, st, -(planRecordRetention + time.Hour))
	orphan := filepath.Join(st.stagingDir(), "plan-orphan")
	if err := os.MkdirAll(orphan, 0o755); err != nil {
		t.Fatal(err)
	}
	staleTmp := filepath.Join(st.plansDir(), "plan-x.pb.tmp")
	if err := os.WriteFile(staleTmp, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-2 * PlanTTL)
	_ = os.Chtimes(staleTmp, old, old)

	st.sweep(time.Now())

	if !exists(st.planPath(live)) || !exists(st.planStagingDir(live)) {
		t.Fatal("live plan swept")
	}
	if exists(st.planStagingDir(expired)) {
		t.Fatal("expired plan kept its staging")
	}
	if !exists(st.planPath(expired)) {
		t.Fatal("expired plan record dropped before retention")
	}
	if exists(st.planPath(ancient)) || exists(st.planStagingDir(ancient)) {
		t.Fatal("plan past retention kept")
	}
	if exists(orphan) {
		t.Fatal("orphan staging kept")
	}
	if exists(staleTmp) {
		t.Fatal("stale temp file kept")
	}
}

func TestSweepSparesWhatALiveSessionUses(t *testing.T) {
	st := planTestStore(t, "stable")
	expired := sweepPlan(t, st, -(planRecordRetention + time.Hour))
	now := time.Now()
	if err := st.saveSession(&enginestate.ApplySessionRecord{
		SessionId:   "sess-live",
		PlanId:      expired,
		Phase:       updaterv2.SessionPhase_SESSION_PHASE_COPYING,
		StartedAt:   timestamppb.New(now.Add(-time.Hour)),
		HeartbeatAt: timestamppb.New(now.Add(-time.Minute)),
		StagedRoot:  st.planStagingDir(expired),
	}); err != nil {
		t.Fatal(err)
	}
	st.sweep(now)
	if !exists(st.planPath(expired)) || !exists(st.planStagingDir(expired)) {
		t.Fatal("swept files of a live session")
	}

	// A worker silent for longer than PlanTTL no longer protects anything.
	st.sweep(now.Add(PlanTTL + time.Hour))
	if exists(st.planStagingDir(expired)) {
		t.Fatal("dead session still protects staging")
	}
}

func TestSweepKeepsNewestSessionAndDropsOldFinishedOnes(t *testing.T) {
	st := planTestStore(t, "stable")
	now := time.Now()
	save := func(id string, phase updaterv2.SessionPhase, age time.Duration) {
		t.Helper()
		at := timestamppb.New(now.Add(-age))
		if err := st.saveSession(&enginestate.ApplySessionRecord{
			SessionId: id, Phase: phase, StartedAt: at, HeartbeatAt: at,
		}); err != nil {
			t.Fatal(err)
		}
	}
	save("sess-old", updaterv2.SessionPhase_SESSION_PHASE_COMPLETED, 3*sessionRetention)
	save("sess-recent", updaterv2.SessionPhase_SESSION_PHASE_NEEDS_ATTENTION, time.Hour)
	save("sess-newest-old", updaterv2.SessionPhase_SESSION_PHASE_COMPLETED, 2*sessionRetention)

	// Make the newest record old too, to prove newest is kept regardless of age.
	st.sweep(now.Add(10 * sessionRetention))
	if exists(st.sessionPath("sess-old")) {
		t.Fatal("old finished session kept")
	}
	if exists(st.sessionPath("sess-newest-old")) {
		t.Fatal("non-newest finished session kept")
	}
	if !exists(st.sessionPath("sess-recent")) {
		t.Fatal("newest session dropped")
	}
}
