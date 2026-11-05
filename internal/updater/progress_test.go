package updater

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	enginestate "github.com/shichao402/relkit/api/updater/enginestate/v1"
	updaterv2 "github.com/shichao402/relkit/api/updater/v2"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type fakeProgress struct {
	mu        sync.Mutex
	phases    []updaterv2.SessionPhase
	cancel    []bool
	lastRecv  int64
	lastTotal int64
	failed    *updaterv2.Error
	closed    bool
	cancelled chan struct{}
	onPhase   func(updaterv2.SessionPhase)
}

func newFakeProgress() *fakeProgress { return &fakeProgress{cancelled: make(chan struct{})} }

func (f *fakeProgress) SetPhase(p updaterv2.SessionPhase, cancelable bool) {
	f.mu.Lock()
	f.phases = append(f.phases, p)
	f.cancel = append(f.cancel, cancelable)
	hook := f.onPhase
	f.mu.Unlock()
	if hook != nil {
		hook(p)
	}
}

func (f *fakeProgress) SetDownload(r, total, _ int64) {
	f.mu.Lock()
	f.lastRecv, f.lastTotal = r, total
	f.mu.Unlock()
}

func (f *fakeProgress) Fail(err *updaterv2.Error) {
	f.mu.Lock()
	f.failed = err
	f.mu.Unlock()
}

func (f *fakeProgress) Close() {
	f.mu.Lock()
	f.closed = true
	f.mu.Unlock()
}

func (f *fakeProgress) Cancelled() <-chan struct{} { return f.cancelled }

func TestProgressTextLanguages(t *testing.T) {
	zh := progressText{spec: progressSpec{Product: "SvnMergeTool", Version: "1.2.3", Lang: "zh"}}
	en := progressText{spec: progressSpec{Product: "SvnMergeTool", Version: "1.2.3", Lang: "en"}}
	cases := []struct {
		phase  updaterv2.SessionPhase
		zh, en string
	}{
		{updaterv2.SessionPhase_SESSION_PHASE_WAITING_FOR_EXIT, "正在等待 SvnMergeTool 退出…", "Waiting for SvnMergeTool to exit…"},
		{updaterv2.SessionPhase_SESSION_PHASE_DOWNLOADING, "正在下载 1.2.3…", "Downloading 1.2.3…"},
		{updaterv2.SessionPhase_SESSION_PHASE_RELAUNCHING, "正在启动 SvnMergeTool…", "Starting SvnMergeTool…"},
	}
	for _, c := range cases {
		if got := zh.Phase(c.phase); got != c.zh {
			t.Errorf("zh %v = %q", c.phase, got)
		}
		if got := en.Phase(c.phase); got != c.en {
			t.Errorf("en %v = %q", c.phase, got)
		}
	}
	if zh.CancelButton() != "取消" || en.CloseButton() != "Close" {
		t.Fatal("button text")
	}
	if got := en.Download(1536, 3<<20, 1<<20); got != "1.5 KiB / 3.0 MiB  ·  1.0 MiB/s" {
		t.Fatalf("download line = %q", got)
	}
	for in, want := range map[string]string{"zh_CN.UTF-8": "zh", "zh-Hans": "zh", "en_US.UTF-8": "en", "": "en", "C": "en"} {
		if got := langFromLocale(in); got != want {
			t.Errorf("langFromLocale(%q) = %q", in, got)
		}
	}
}

func TestProgressTextFailureCarriesRecovery(t *testing.T) {
	txt := progressText{spec: progressSpec{Product: "P", Lang: "zh"}}
	err := &updaterv2.Error{
		Message: "network down",
		Recovery: &updaterv2.RecoveryHelp{
			Message: "请重新下载安装包",
			Links:   []*updaterv2.RecoveryLink{{Url: ""}, {Url: "https://example.com/dl"}},
		},
	}
	if got := txt.FailureDetail(err); got != "network down\r\n\r\n请重新下载安装包" {
		t.Fatalf("detail = %q", got)
	}
	label, url := txt.HelpURL(err)
	if label != "帮助" || url != "https://example.com/dl" {
		t.Fatalf("help = %q %q", label, url)
	}
}

func TestOpenProgressUIOnlyWhileHostIsGone(t *testing.T) {
	fake := newFakeProgress()
	eng := &Engine{newProgressUI: func(progressSpec) progressUI { return fake }}
	if _, ok := eng.openProgressUI(&enginestate.ApplySessionRecord{RequiresHostExit: true, HideProgress: true}).(noopProgress); !ok {
		t.Fatal("hide_progress must suppress the window")
	}
	if _, ok := eng.openProgressUI(&enginestate.ApplySessionRecord{}).(noopProgress); !ok {
		t.Fatal("a host that keeps running must not get a window")
	}
	if eng.openProgressUI(&enginestate.ApplySessionRecord{RequiresHostExit: true}) != fake {
		t.Fatal("window expected when the host exits")
	}
}

func TestPhaseCancelableStopsAtCopy(t *testing.T) {
	for p, want := range map[updaterv2.SessionPhase]bool{
		updaterv2.SessionPhase_SESSION_PHASE_WAITING_FOR_EXIT: true,
		updaterv2.SessionPhase_SESSION_PHASE_DOWNLOADING:      true,
		updaterv2.SessionPhase_SESSION_PHASE_COPYING:          false,
		updaterv2.SessionPhase_SESSION_PHASE_COMMITTING:       false,
		updaterv2.SessionPhase_SESSION_PHASE_RELAUNCHING:      false,
	} {
		if phaseCancelable(p) != want {
			t.Errorf("phaseCancelable(%v) != %v", p, want)
		}
	}
}

// undownloadedApply accepts an apply for a plan that was never downloaded and
// returns the engine state the worker will run against.
func undownloadedApply(t *testing.T, op *updaterv2.ApplyOp) (string, store, *enginestate.UpdatePlan, string) {
	t.Helper()
	dataDir := t.TempDir()
	installRoot := t.TempDir()
	plan := mustPayloadPlan(t, t.TempDir(), "2.0.0", 2, map[string][]byte{"data/readme.txt": []byte("hello")})
	src := plan.Files[0].LocalPath
	raw, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeContent(w, r, "p.zip", time.Now(), bytes.NewReader(raw))
	}))
	t.Cleanup(srv.Close)
	plan.PlanId = "plan-undl"
	plan.ExpiresAt = timestamppb.New(time.Now().Add(time.Hour))
	plan.Files[0].LocalPath = ""
	plan.Files[0].Sha256Hex = hex.EncodeToString(sum[:])
	plan.Files[0].Urls = []string{srv.URL + "/p.zip"}
	st := store{dataDir: dataDir}
	key, err := st.planKey()
	if err != nil {
		t.Fatal(err)
	}
	plan.PlanHmac = hmacPlan(key, plan)
	if err := st.savePlan(plan); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var sessionID string
	eng := &Engine{Stdout: &stdout, LaunchWorker: func(_, id, _ string) error { sessionID = id; return nil }}
	req := &updaterv2.UpdaterRequest{
		Profile: &updaterv2.ClientProfile{
			ExpectedProduct: "demo",
			Recovery:        &updaterv2.RecoveryHelp{Message: "reinstall"},
		},
		Runtime: &updaterv2.Runtime{
			DataDir: dataDir,
			Install: &updaterv2.InstallSpec{Placement: updaterv2.Placement_PLACEMENT_IN_PLACE, InstallRoot: installRoot},
		},
	}
	if err := eng.handleApply(context.Background(), req, op, st); err != nil {
		t.Fatal(err)
	}
	events := decodeEvents(t, &stdout)
	acc := events[0].GetApply().GetAccepted()
	if acc == nil {
		t.Fatalf("apply of an undownloaded plan must be accepted, got %v", events)
	}
	if !acc.RequiresHostExit {
		t.Fatal("an undownloaded payload cannot be inspected, so the host must exit")
	}
	sess, err := st.loadSession(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if sess.DisplayName != "demo" || sess.GetRecovery().GetMessage() != "reinstall" {
		t.Fatalf("session window fields = %q %v", sess.DisplayName, sess.Recovery)
	}
	// The test process is not a host that will exit.
	sess.Pid = 0
	if err := st.saveSession(sess); err != nil {
		t.Fatal(err)
	}
	return dataDir, st, plan, sessionID
}

func TestWorkerDownloadsThenInstallsWithProgress(t *testing.T) {
	dataDir, st, plan, sessionID := undownloadedApply(t, &updaterv2.ApplyOp{PlanId: "plan-undl"})
	fake := newFakeProgress()
	eng := &Engine{newProgressUI: func(progressSpec) progressUI { return fake }}
	if err := eng.RunWorker(dataDir, sessionID); err != nil {
		t.Fatal(err)
	}
	sess, _ := st.loadSession(sessionID)
	if sess.Phase != updaterv2.SessionPhase_SESSION_PHASE_COMPLETED {
		t.Fatalf("phase = %v err = %v", sess.Phase, sess.Error)
	}
	want := []updaterv2.SessionPhase{
		updaterv2.SessionPhase_SESSION_PHASE_WAITING_FOR_EXIT,
		updaterv2.SessionPhase_SESSION_PHASE_DOWNLOADING,
		updaterv2.SessionPhase_SESSION_PHASE_COPYING,
	}
	if len(fake.phases) != len(want) {
		t.Fatalf("phases = %v", fake.phases)
	}
	for i := range want {
		if fake.phases[i] != want[i] {
			t.Fatalf("phases = %v", fake.phases)
		}
	}
	if fake.cancel[2] {
		t.Fatal("copying must not be cancelable")
	}
	if fake.lastTotal != plan.Files[0].Size || fake.lastRecv != plan.Files[0].Size {
		t.Fatalf("download progress = %d/%d", fake.lastRecv, fake.lastTotal)
	}
	if !fake.closed || fake.failed != nil {
		t.Fatal("window must close cleanly")
	}
	reloaded, _ := st.loadPlan(plan.PlanId)
	key, _ := st.planKey()
	if !planDownloaded(reloaded) || !verifyPlan(key, reloaded) {
		t.Fatal("worker must record and re-seal the download")
	}
	if got, err := os.ReadFile(filepath.Join(sess.InstallRoot, "data", "readme.txt")); err != nil || string(got) != "hello" {
		t.Fatalf("installed file = %q %v", got, err)
	}
}

func TestWorkerCancelBeforeCopyRelaunchesOldHost(t *testing.T) {
	dataDir, st, _, sessionID := undownloadedApply(t, &updaterv2.ApplyOp{PlanId: "plan-undl"})
	fake := newFakeProgress()
	fake.onPhase = func(p updaterv2.SessionPhase) {
		if p == updaterv2.SessionPhase_SESSION_PHASE_DOWNLOADING {
			close(fake.cancelled)
			time.Sleep(50 * time.Millisecond) // let the cancel propagate
		}
	}
	var relaunched bool
	orig := relaunchHost
	relaunchHost = func(*enginestate.ApplySessionRecord) { relaunched = true }
	t.Cleanup(func() { relaunchHost = orig })

	eng := &Engine{newProgressUI: func(progressSpec) progressUI { return fake }}
	if err := eng.RunWorker(dataDir, sessionID); err == nil {
		t.Fatal("cancelled worker must report an error")
	}
	sess, _ := st.loadSession(sessionID)
	if sess.GetError().GetCode() != updaterv2.ErrorCode_ERROR_CODE_CANCELED {
		t.Fatalf("error = %v", sess.Error)
	}
	if sess.GetError().GetRecovery().GetMessage() != "reinstall" {
		t.Fatal("worker errors must carry the host recovery help")
	}
	if fake.failed != nil {
		t.Fatal("a user cancel is not shown as a failure")
	}
	if !relaunched {
		t.Fatal("old host must be relaunched after a cancel")
	}
	if entries, _ := os.ReadDir(sess.InstallRoot); len(entries) != 0 {
		names := []string{}
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("install root touched: %v", names)
	}
}
