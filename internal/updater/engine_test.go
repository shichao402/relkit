package updater

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	updaterv1 "go.firoyang.com/relkit/api/updater/v1"
	"go.firoyang.com/relkit/internal/ipc"
	"go.firoyang.com/relkit/internal/model"
	"go.firoyang.com/relkit/internal/payload"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestMapLegacyLastResult(t *testing.T) {
	cases := map[string]updaterv1.LastResult{
		"available":        updaterv1.LastResult_LAST_RESULT_UPDATE_AVAILABLE,
		"update-available": updaterv1.LastResult_LAST_RESULT_UPDATE_AVAILABLE,
		"fallback":         updaterv1.LastResult_LAST_RESULT_FALLBACK_REQUIRED,
		"success":          updaterv1.LastResult_LAST_RESULT_UP_TO_DATE,
		"up-to-date":       updaterv1.LastResult_LAST_RESULT_UP_TO_DATE,
		"failure":          updaterv1.LastResult_LAST_RESULT_FAILED,
		"error":            updaterv1.LastResult_LAST_RESULT_FAILED,
		"no-artifact":      updaterv1.LastResult_LAST_RESULT_FAILED,
		"mystery":          updaterv1.LastResult_LAST_RESULT_UNSPECIFIED,
	}
	for in, want := range cases {
		if got := MapLegacyLastResult(in); got != want {
			t.Errorf("%q: got %v want %v", in, got, want)
		}
	}
}

func TestMigrateLegacyJSONPreservesWatermark(t *testing.T) {
	dir := t.TempDir()
	seq := int64(9)
	dirSeq := int64(4)
	legacy := map[string]any{
		"lastResult":                "available",
		"lastSeenSequence":          seq,
		"lastSeenDirectorySequence": dirSeq,
		"skipped":                   []int{91},
	}
	raw, _ := json.Marshal(legacy)
	if err := os.WriteFile(filepath.Join(dir, LegacyStateJSON), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	st := store{dataDir: dir}
	got, err := st.loadState()
	if err != nil {
		t.Fatal(err)
	}
	if got.LastSeenSequence != seq || got.LastSeenDirectorySequence != dirSeq {
		t.Fatalf("watermarks dropped: %+v", got)
	}
	if got.LastResult != updaterv1.LastResult_LAST_RESULT_UPDATE_AVAILABLE {
		t.Fatalf("lastResult %v", got.LastResult)
	}
	if len(got.SkippedCodes) != 1 || got.SkippedCodes[0] != 91 {
		t.Fatalf("skipped %v", got.SkippedCodes)
	}
	if _, err := os.Stat(filepath.Join(dir, StateFileName)); err != nil {
		t.Fatal("state.pb not written")
	}
}

func TestClampHalfHourDoesNotBecomeZero(t *testing.T) {
	p := &updaterv1.CheckPolicy{AfterSuccess: durationpb.New(30 * time.Minute)}
	success, failure := clampPolicy(p)
	if success != 30*time.Minute {
		t.Fatalf("success=%v", success)
	}
	if failure < MinCheckInterval {
		t.Fatalf("failure clamped below min: %v", failure)
	}
	zero := &updaterv1.CheckPolicy{AfterSuccess: durationpb.New(0)}
	s, _ := clampPolicy(zero)
	if s != MinCheckInterval {
		t.Fatalf("zero must clamp to min, got %v", s)
	}
}

func TestValidateArtifactFilename(t *testing.T) {
	bad := []string{"/etc/passwd", `..\x`, "..", "foo/../bar", "CON", `C:\abs`}
	for _, n := range bad {
		if err := ValidateArtifactFilename(n); err == nil {
			t.Errorf("expected reject %q", n)
		}
	}
	if err := ValidateArtifactFilename("app.zip"); err != nil {
		t.Fatal(err)
	}
}

func TestFrameRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	ev := &updaterv1.UpdaterEvent{Kind: &updaterv1.UpdaterEvent_Log{Log: &updaterv1.Log{Message: "hi"}}}
	if err := ipc.WriteFrame(&buf, ev); err != nil {
		t.Fatal(err)
	}
	got := &updaterv1.UpdaterEvent{}
	if err := ipc.ReadFrame(&buf, got); err != nil {
		t.Fatal(err)
	}
	if got.GetLog().GetMessage() != "hi" {
		t.Fatalf("%v", got)
	}
}

func TestNegotiateWindow(t *testing.T) {
	if negotiate(&updaterv1.ClientHello{IpcMin: 1, IpcMax: 1}).Code != updaterv1.ErrorCode_ERROR_CODE_UPDATER_TOO_NEW {
		t.Fatal("window 1 host must be rejected")
	}
	if negotiate(&updaterv1.ClientHello{IpcMin: 2, IpcMax: 2}).Code != updaterv1.ErrorCode_ERROR_CODE_UPDATER_TOO_NEW {
		t.Fatal("window 2 host must be rejected")
	}
	if negotiate(&updaterv1.ClientHello{IpcMin: 3, IpcMax: 3}) != nil {
		t.Fatal("window 3 host")
	}
	got := negotiate(&updaterv1.ClientHello{IpcMin: 1, IpcMax: 0})
	if got == nil {
		t.Fatal("expected error")
	}
}

func TestProfileValidation(t *testing.T) {
	err := validateOpen(&updaterv1.ClientProfile{Product: "p"}, &updaterv1.Runtime{Channel: "stable", CurrentCode: 1, DataDir: "x"})
	if err == nil || err.Code != updaterv1.ErrorCode_ERROR_CODE_PROFILE_INVALID {
		t.Fatalf("keys+urls required: %v", err)
	}
	err = validateOpen(&updaterv1.ClientProfile{
		Product: "p", IndexUrls: []string{"https://e.invalid/i"},
		TrustedKeys:     []*updaterv1.TrustedKey{{KeyId: "k", PublicKey: bytes.Repeat([]byte{1}, 32)}},
		AllowedChannels: []string{"stable"},
	}, &updaterv1.Runtime{Channel: "dev", CurrentCode: 1, DataDir: "x"})
	if err == nil || err.Code != updaterv1.ErrorCode_ERROR_CODE_CHANNEL_NOT_ALLOWED {
		t.Fatalf("got %v", err)
	}
	err = validateOpen(&updaterv1.ClientProfile{
		Product: "p", IndexUrls: []string{"https://e.invalid/i"},
		TrustedKeys: []*updaterv1.TrustedKey{{KeyId: "k", PublicKey: bytes.Repeat([]byte{1}, 32)}},
	}, &updaterv1.Runtime{Channel: "dev", CurrentCode: 0, DataDir: "x"})
	if err != nil {
		t.Fatalf("code 0 is a valid pre-release baseline: %v", err)
	}
}

func TestWaitForHostExit(t *testing.T) {
	if err := waitForHostExit(0, time.Second, nil); err != nil {
		t.Fatal(err)
	}
	if err := waitForHostExit(os.Getpid(), time.Millisecond, nil); err == nil {
		t.Fatal("live process must time out")
	}
}

func TestHandleApplyStartsWorkerBeforeAccepting(t *testing.T) {
	dataDir := t.TempDir()
	installRoot := t.TempDir()
	tree := t.TempDir()
	_ = os.WriteFile(filepath.Join(tree, "app.exe"), []byte("bin"), 0o644)
	artifact := filepath.Join(t.TempDir(), "app-payload.zip")
	if _, err := payload.Build(artifact, payload.BuildOptions{Tree: tree}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(artifact)
	if err != nil {
		t.Fatal(err)
	}
	st := store{dataDir: dataDir}
	key, err := st.planKey()
	if err != nil {
		t.Fatal(err)
	}
	plan := &updaterv1.UpdatePlan{
		PlanId:    "plan-test",
		Version:   "1.0.0",
		Code:      1,
		ExpiresAt: timestamppb.New(time.Now().Add(time.Hour)),
		Files: []*updaterv1.PlannedFile{{
			Name:       "app-payload.zip",
			Kind:       "payload",
			Size:       info.Size(),
			LocalPath:  artifact,
			Downloaded: true,
		}},
	}
	plan.PlanHmac = hmacPlan(key, plan)
	if err := st.savePlan(plan); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	var launched bool
	eng := &Engine{
		Stdout: &stdout,
		LaunchWorker: func(gotDataDir, sessionID, stagedRoot string) error {
			launched = true
			if gotDataDir != dataDir || sessionID == "" ||
				stagedRoot != filepath.Join(dataDir, "staging", plan.PlanId) {
				t.Fatalf("unexpected worker args: %q %q %q", gotDataDir, sessionID, stagedRoot)
			}
			return nil
		},
	}
	req := &updaterv1.UpdaterRequest{
		Runtime: &updaterv1.Runtime{
			DataDir: dataDir,
			Install: &updaterv1.InstallSpec{
				Placement:   updaterv1.Placement_PLACEMENT_LIBRARY,
				InstallRoot: installRoot,
			},
		},
	}
	if err := eng.handleApply(
		context.Background(),
		req,
		&updaterv1.ApplyOp{PlanId: plan.PlanId},
		st,
	); err != nil {
		t.Fatal(err)
	}
	if !launched {
		t.Fatal("apply accepted without starting worker")
	}
	ev := &updaterv1.UpdaterEvent{}
	if err := ipc.ReadFrame(&stdout, ev); err != nil {
		t.Fatal(err)
	}
	if ev.GetApply().GetAccepted() == nil {
		t.Fatalf("expected accepted result, got %v", ev)
	}
}

func mustPayloadPlan(t *testing.T, stage, version string, code int64, files map[string][]byte) *updaterv1.UpdatePlan {
	t.Helper()
	tree := filepath.Join(stage, "tree-"+version)
	for rel, body := range files {
		path := filepath.Join(tree, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		mode := os.FileMode(0o644)
		if strings.HasSuffix(rel, "loom") || strings.HasSuffix(rel, ".exe") {
			mode = 0o755
		}
		if err := os.WriteFile(path, body, mode); err != nil {
			t.Fatal(err)
		}
	}
	zipPath := filepath.Join(stage, version+"-payload.zip")
	if _, err := payload.Build(zipPath, payload.BuildOptions{Tree: tree}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	return &updaterv1.UpdatePlan{
		Version: version,
		Code:    code,
		Files: []*updaterv1.PlannedFile{{
			Name:      filepath.Base(zipPath),
			Kind:      "payload",
			Size:      info.Size(),
			LocalPath: zipPath,
		}},
	}
}

func TestBaselineDeletionProtectsModifiedFiles(t *testing.T) {
	root := t.TempDir()
	dataDir := t.TempDir()
	stage := t.TempDir()
	filesRoot := filepath.Join(stage, "files")
	_ = os.MkdirAll(filesRoot, 0o755)
	_ = os.WriteFile(filepath.Join(root, "stale.txt"), []byte("user-edited"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "remove.txt"), []byte("old"), 0o644)
	oldDigest := model.Sha256Bytes([]byte("old"))
	if err := saveBaseline(filepath.Join(dataDir, baselineFileName), &updaterv1.Baseline{
		Schema: baselineSchema,
		Files: []*updaterv1.BaselineEntry{
			{Relpath: "stale.txt", Sha256: oldDigest},
			{Relpath: "remove.txt", Sha256: oldDigest},
		},
	}); err != nil {
		t.Fatal(err)
	}
	sess := &updaterv1.ApplySessionRecord{
		InstallRoot: root,
		StagedRoot:  stage,
	}
	content := &applyContent{
		table:     &updaterv1.FileTable{Schema: payload.Schema},
		filesRoot: filesRoot,
	}
	if err := applyContentTable(dataDir, sess, &updaterv1.UpdatePlan{Code: 2}, content, root, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "remove.txt")); !os.IsNotExist(err) {
		t.Fatal("unmodified stale file was not deleted")
	}
	if got, _ := os.ReadFile(filepath.Join(root, "stale.txt")); string(got) != "user-edited" {
		t.Fatal("modified stale file was deleted")
	}
}

func TestVersionedDirAtomicActive(t *testing.T) {
	root := t.TempDir()
	stage := t.TempDir()
	plan := mustPayloadPlan(t, stage, "1.0.0", 2, map[string][]byte{
		"bin/app.exe": []byte("n"),
	})
	sess := &updaterv1.ApplySessionRecord{
		InstallRoot:       root,
		StagedRoot:        stage,
		Placement:         updaterv1.Placement_PLACEMENT_LIBRARY,
		ExecutableRelpath: "bin/app.exe",
		Library:           &updaterv1.LibraryPolicy{Retain: 2},
	}
	if err := applyPlan(t.TempDir(), sess, plan); err != nil {
		t.Fatal(err)
	}
	ptr := readActive(root)
	if ptr == nil || ptr.Code != 2 {
		t.Fatalf("active %+v", ptr)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(ptr.Path))); err != nil {
		t.Fatal(err)
	}
}

func TestHandleRequestMissingHello(t *testing.T) {
	var buf bytes.Buffer
	eng := &Engine{Version: "test", Stdout: &buf}
	req := &updaterv1.UpdaterRequest{
		Profile: &updaterv1.ClientProfile{
			Product:     "p",
			IndexUrls:   []string{"https://e.invalid/i"},
			TrustedKeys: []*updaterv1.TrustedKey{{KeyId: "k", PublicKey: bytes.Repeat([]byte{1}, 32)}},
		},
		Runtime: &updaterv1.Runtime{Channel: "stable", CurrentCode: 1, DataDir: t.TempDir()},
		Op:      &updaterv1.UpdaterRequest_Status{Status: &updaterv1.StatusOp{}},
	}
	if err := eng.HandleRequest(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	ev := &updaterv1.UpdaterEvent{}
	if err := ipc.ReadFrame(&buf, ev); err != nil {
		t.Fatal(err)
	}
	if ev.GetCapabilities() == nil {
		t.Fatal("first event must be capabilities")
	}
	ev2 := &updaterv1.UpdaterEvent{}
	if err := ipc.ReadFrame(&buf, ev2); err != nil {
		t.Fatal(err)
	}
	if ev2.GetFailed() == nil {
		t.Fatalf("expected hello failure, got %v", ev2)
	}
}

func TestInstallOnlyLeavesActive(t *testing.T) {
	root := t.TempDir()
	stage := t.TempDir()
	first := mustPayloadPlan(t, stage, "1.0.0+1", 1, map[string][]byte{
		"bin/app.exe": []byte("a"),
	})
	sess := &updaterv1.ApplySessionRecord{
		InstallRoot:       root,
		StagedRoot:        stage,
		Placement:         updaterv1.Placement_PLACEMENT_LIBRARY,
		ExecutableRelpath: "bin/app.exe",
		Library:           &updaterv1.LibraryPolicy{Retain: 2},
	}
	if err := applyPlan(t.TempDir(), sess, first); err != nil {
		t.Fatal(err)
	}
	second := mustPayloadPlan(t, stage, "1.0.1+2", 2, map[string][]byte{
		"bin/app.exe": []byte("b"),
	})
	sess.InstallOnly = true
	if err := applyPlan(t.TempDir(), sess, second); err != nil {
		t.Fatal(err)
	}
	ptr := readActive(root)
	if ptr == nil || ptr.Code != 1 {
		t.Fatalf("active moved: %+v", ptr)
	}
	list := listInstalled(root)
	if len(list.Versions) != 2 {
		t.Fatalf("want 2 installed, got %d", len(list.Versions))
	}
	if err := switchActive(root, 2); err != nil {
		t.Fatal(err)
	}
	if readActive(root).Code != 2 {
		t.Fatal("switch failed")
	}
	if err := rollbackActive(root); err != nil {
		t.Fatal(err)
	}
	if readActive(root).Code != 1 {
		t.Fatal("rollback failed")
	}
}

func TestPruneKeepsReservedCodes(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "versions")
	for _, name := range []string{"0.1.0+1", "0.1.1+2", "0.1.2+3"} {
		dir := filepath.Join(base, name)
		_ = os.MkdirAll(dir, 0o755)
	}
	_ = writeVersionMeta(filepath.Join(base, "0.1.0+1"), versionMeta{Code: 1, Path: "versions/0.1.0+1"})
	_ = writeVersionMeta(filepath.Join(base, "0.1.1+2"), versionMeta{Code: 2, Path: "versions/0.1.1+2"})
	_ = writeVersionMeta(filepath.Join(base, "0.1.2+3"), versionMeta{Code: 3, Path: "versions/0.1.2+3"})
	pruneVersions(root, activePointer{Code: 3, Path: "versions/0.1.2+3"}, 2, []int64{1}, "versions/0.1.2+3")
	if _, err := os.Stat(filepath.Join(base, "0.1.0+1")); err != nil {
		t.Fatal("reserved code 1 was pruned")
	}
	if _, err := os.Stat(filepath.Join(base, "0.1.1+2")); err == nil {
		t.Fatal("unreserved previous should be pruned when retain=2 already has current+reserved")
	}
}

func TestVersionedDirKeepsAppBundle(t *testing.T) {
	root := t.TempDir()
	stage := t.TempDir()
	plan := mustPayloadPlan(t, stage, "0.2.2+21", 21, map[string][]byte{
		"Loom Editor.app/Contents/MacOS/loom": []byte("bin"),
	})
	sess := &updaterv1.ApplySessionRecord{
		InstallRoot:       root,
		StagedRoot:        stage,
		Placement:         updaterv1.Placement_PLACEMENT_LIBRARY,
		ExecutableRelpath: "Loom Editor.app/Contents/MacOS/loom",
		Library:           &updaterv1.LibraryPolicy{Retain: 2},
	}
	if err := applyPlan(t.TempDir(), sess, plan); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(root, "versions", "0.2.2+21", "Loom Editor.app", "Contents", "MacOS", "loom")
	if _, err := os.Stat(dest); err != nil {
		t.Fatalf("bundle not preserved: %v", err)
	}
}

func TestApplyLockRejectsSecondHolder(t *testing.T) {
	root := t.TempDir()
	if err := acquireApplyLock(root, "a"); err != nil {
		t.Fatal(err)
	}
	if err := acquireApplyLock(root, "b"); err == nil {
		t.Fatal("second lock should fail")
	}
	releaseApplyLock(root)
	if err := acquireApplyLock(root, "b"); err != nil {
		t.Fatal(err)
	}
}
