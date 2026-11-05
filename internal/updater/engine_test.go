package updater

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	enginestate "github.com/shichao402/relkit/api/updater/enginestate/v1"
	updaterv2 "github.com/shichao402/relkit/api/updater/v2"
	"github.com/shichao402/relkit/internal/model"
	"github.com/shichao402/relkit/internal/payload"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestStateIsIsolatedByProductAndChannel(t *testing.T) {
	dir := t.TempDir()
	dev := store{dataDir: dir, product: "svn-auto-merge", channel: "dev"}
	stable := store{dataDir: dir, product: "svn-auto-merge", channel: "stable"}
	other := store{dataDir: dir, product: "other/product", channel: "dev"}

	if err := dev.saveState(&enginestate.PersistedState{LastSeenSequence: 93}); err != nil {
		t.Fatal(err)
	}
	if err := stable.saveState(&enginestate.PersistedState{LastSeenSequence: 21}); err != nil {
		t.Fatal(err)
	}
	if err := other.saveState(&enginestate.PersistedState{LastSeenSequence: 7}); err != nil {
		t.Fatal(err)
	}

	for name, tc := range map[string]struct {
		store store
		want  int64
	}{
		"dev":    {store: dev, want: 93},
		"stable": {store: stable, want: 21},
		"other":  {store: other, want: 7},
	} {
		got, err := tc.store.loadState()
		if err != nil {
			t.Fatal(err)
		}
		if got.LastSeenSequence != tc.want {
			t.Errorf("%s sequence=%d want=%d", name, got.LastSeenSequence, tc.want)
		}
	}
	if dev.statePath() == stable.statePath() {
		t.Fatal("channels share one state path")
	}
	if filepath.Base(other.statePath()) != "state-other_product-dev.pb" {
		t.Fatalf("unsafe product was not sanitized: %s", other.statePath())
	}
}

// The epoch deletes the legacy migration (requirement 3.4). An unattributed
// state.pb written by an old client must never leak into a scoped store.
func TestScopedStateIgnoresUnattributedStateFile(t *testing.T) {
	dir := t.TempDir()
	legacy := &enginestate.PersistedState{
		LastSeenSequence:          93,
		LastSeenDirectorySequence: 12,
		SkippedCodes:              []int64{150},
	}
	raw, err := proto.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, StateFileName), raw, 0o644); err != nil {
		t.Fatal(err)
	}

	scoped := store{dataDir: dir, product: "svn-auto-merge", channel: "stable"}
	got, err := scoped.loadState()
	if err != nil {
		t.Fatal(err)
	}
	if got.LastSeenSequence != 0 || got.LastSeenDirectorySequence != 0 ||
		len(got.SkippedCodes) != 0 {
		t.Fatalf("unattributed legacy state leaked into stable: %+v", got)
	}
	if _, err := os.Stat(scoped.statePath()); err != nil {
		t.Fatal("scoped state was not created")
	}
}

func TestClampHalfHourDoesNotBecomeZero(t *testing.T) {
	p := &updaterv2.CheckPolicy{AfterSuccess: durationpb.New(30 * time.Minute)}
	success, failure := clampPolicy(p)
	if success != 30*time.Minute {
		t.Fatalf("success=%v", success)
	}
	if failure < MinCheckInterval {
		t.Fatalf("failure clamped below min: %v", failure)
	}
	zero := &updaterv2.CheckPolicy{AfterSuccess: durationpb.New(0)}
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

func TestEventLineRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	ev := &updaterv2.UpdaterEvent{Kind: &updaterv2.UpdaterEvent_Log{Log: &updaterv2.Log{Message: "hi\nthere"}}}
	if err := WriteEvent(&buf, ev); err != nil {
		t.Fatal(err)
	}
	line := buf.String()
	if strings.Count(line, "\n") != 1 || !strings.HasSuffix(line, "\n") {
		t.Fatalf("event must be exactly one line, got %q", line)
	}
	events := decodeEvents(t, &buf)
	if len(events) != 1 || !proto.Equal(events[0], ev) {
		t.Fatalf("round trip = %v", events)
	}
}

func TestReadRequestIsStrict(t *testing.T) {
	good := `{"protocol":5,"profile":{"bootstrap":"{}","expectedProduct":"p"},"runtime":{"channel":"dev","dataDir":"x"},"status":{}}`
	req, err := ReadRequest(strings.NewReader(good))
	if err != nil {
		t.Fatalf("valid request rejected: %v", err)
	}
	if req.GetProtocol() != 5 || req.GetStatus() == nil || req.GetProfile().GetExpectedProduct() != "p" {
		t.Fatalf("decoded = %v", req)
	}
	// proto field names are accepted too; that is standard ProtoJSON.
	if _, err := ReadRequest(strings.NewReader(`{"protocol":5,"profile":{"expected_product":"p"}}`)); err != nil {
		t.Fatalf("proto names must decode: %v", err)
	}
	for name, raw := range map[string]string{
		"unknown top-level":  `{"protocol":5,"hello":{"ipcMin":4}}`,
		"removed field":      `{"protocol":5,"profile":{"product":"p"}}`,
		"removed op":         `{"protocol":5,"cancel":{}}`,
		"wrong type":         `{"protocol":"five"}`,
		"length-prefixed v1": "\x00\x00\x00\x02{}",
		"empty":              ``,
	} {
		if _, err := ReadRequest(strings.NewReader(raw)); err == nil {
			t.Fatalf("%s: must be rejected", name)
		}
	}
}

func TestRejectRequestAnswersOnStdout(t *testing.T) {
	var buf bytes.Buffer
	eng := &Engine{Version: "test", Stdout: &buf}
	_, cause := ReadRequest(strings.NewReader(`{"hello":{}}`))
	if err := eng.RejectRequest(cause); err != nil {
		t.Fatal(err)
	}
	events := decodeEvents(t, &buf)
	if len(events) != 2 || events[0].GetCapabilities() == nil {
		t.Fatalf("want capabilities + failure, got %v", events)
	}
	if got := events[1].GetFailed().GetError().GetCode(); got != updaterv2.ErrorCode_ERROR_CODE_PROTOCOL_MISMATCH {
		t.Fatalf("code = %v", got)
	}
}

func TestRuntimeValidation(t *testing.T) {
	opened := &openedProfile{Product: "p", EntryURLs: []string{"https://e.invalid/"}}
	if err := validateRuntime(&openedProfile{Product: "p"}, &updaterv2.Runtime{Channel: "dev", CurrentCode: 1, DataDir: "x"}); err == nil ||
		err.Code != updaterv2.ErrorCode_ERROR_CODE_PROFILE_INVALID {
		t.Fatalf("entry urls required: %v", err)
	}
	if err := validateRuntime(opened, &updaterv2.Runtime{CurrentCode: 1, DataDir: "x"}); err == nil {
		t.Fatal("channel required")
	}
	if err := validateRuntime(opened, &updaterv2.Runtime{Channel: "dev", CurrentCode: 1}); err == nil {
		t.Fatal("dataDir required")
	}
	if err := validateRuntime(opened, &updaterv2.Runtime{Channel: "dev", CurrentCode: 0, DataDir: "x"}); err != nil {
		t.Fatalf("code 0 is a valid pre-release baseline: %v", err)
	}
}

func TestWaitForHostExit(t *testing.T) {
	if err := waitForHostExit(context.Background(), 0, time.Second, nil); err != nil {
		t.Fatal(err)
	}
	if err := waitForHostExit(context.Background(), os.Getpid(), time.Millisecond, nil); err == nil {
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
	plan := &enginestate.UpdatePlan{
		PlanId:    "plan-test",
		Version:   "1.0.0",
		Code:      1,
		ExpiresAt: timestamppb.New(time.Now().Add(time.Hour)),
		Files: []*enginestate.PlannedFile{{
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
	req := &updaterv2.UpdaterRequest{
		Runtime: &updaterv2.Runtime{
			DataDir: dataDir,
			Install: &updaterv2.InstallSpec{
				Placement:   updaterv2.Placement_PLACEMENT_LIBRARY,
				InstallRoot: installRoot,
			},
		},
	}
	if err := eng.handleApply(
		context.Background(),
		req,
		&updaterv2.ApplyOp{PlanId: plan.PlanId},
		st,
	); err != nil {
		t.Fatal(err)
	}
	if !launched {
		t.Fatal("apply accepted without starting worker")
	}
	events := decodeEvents(t, &stdout)
	if len(events) != 1 {
		t.Fatalf("want one event, got %v", events)
	}
	ev := events[0]
	if ev.GetApply().GetAccepted() == nil {
		t.Fatalf("expected accepted result, got %v", ev)
	}
}

func mustPayloadPlan(t *testing.T, stage, version string, code int64, files map[string][]byte) *enginestate.UpdatePlan {
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
	return &enginestate.UpdatePlan{
		Version: version,
		Code:    code,
		Files: []*enginestate.PlannedFile{{
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
	if err := saveBaseline(filepath.Join(dataDir, baselineFileName), &enginestate.Baseline{
		Schema: baselineSchema,
		Files: []*enginestate.BaselineEntry{
			{Relpath: "stale.txt", Sha256: oldDigest},
			{Relpath: "remove.txt", Sha256: oldDigest},
		},
	}); err != nil {
		t.Fatal(err)
	}
	sess := &enginestate.ApplySessionRecord{
		InstallRoot: root,
		StagedRoot:  stage,
	}
	content := &applyContent{
		table:     &enginestate.FileTable{Schema: payload.Schema},
		filesRoot: filesRoot,
	}
	if err := applyContentTable(dataDir, sess, &enginestate.UpdatePlan{Code: 2}, content, root, true); err != nil {
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
	sess := &enginestate.ApplySessionRecord{
		InstallRoot:       root,
		StagedRoot:        stage,
		Placement:         updaterv2.Placement_PLACEMENT_LIBRARY,
		ExecutableRelpath: "bin/app.exe",
		Library:           &updaterv2.LibraryPolicy{Retain: 2},
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

func TestHandleRequestRequiresBootstrap(t *testing.T) {
	var buf bytes.Buffer
	eng := &Engine{Version: "test", Stdout: &buf}
	req := &updaterv2.UpdaterRequest{
		Protocol: Protocol,
		Profile:  &updaterv2.ClientProfile{ExpectedProduct: "p"},
		Runtime:  &updaterv2.Runtime{Channel: "stable", CurrentCode: 1, DataDir: t.TempDir()},
		Op:       &updaterv2.UpdaterRequest_Status{Status: &updaterv2.StatusOp{}},
	}
	if err := eng.HandleRequest(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	events := decodeEvents(t, &buf)
	if len(events) != 2 || events[0].GetCapabilities() == nil {
		t.Fatalf("want capabilities + failure, got %v", events)
	}
	if got := events[1].GetFailed().GetError().GetCode(); got != updaterv2.ErrorCode_ERROR_CODE_PROFILE_INVALID {
		t.Fatalf("code = %v", got)
	}
}

func TestInstallOnlyLeavesActive(t *testing.T) {
	root := t.TempDir()
	stage := t.TempDir()
	first := mustPayloadPlan(t, stage, "1.0.0+1", 1, map[string][]byte{
		"bin/app.exe": []byte("a"),
	})
	sess := &enginestate.ApplySessionRecord{
		InstallRoot:       root,
		StagedRoot:        stage,
		Placement:         updaterv2.Placement_PLACEMENT_LIBRARY,
		ExecutableRelpath: "bin/app.exe",
		Library:           &updaterv2.LibraryPolicy{Retain: 2},
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
	sess := &enginestate.ApplySessionRecord{
		InstallRoot:       root,
		StagedRoot:        stage,
		Placement:         updaterv2.Placement_PLACEMENT_LIBRARY,
		ExecutableRelpath: "Loom Editor.app/Contents/MacOS/loom",
		Library:           &updaterv2.LibraryPolicy{Retain: 2},
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

func TestWithEngineDefaults(t *testing.T) {
	cases := []struct {
		name string
		host map[string]string
		want map[string]string
	}{
		{
			name: "empty host gets engine platform truth",
			host: nil,
			want: map[string]string{
				"os": runtime.GOOS, "arch": runtime.GOARCH,
				"apply": "relkit-payload",
			},
		},
		{
			name: "host platform values are kept verbatim",
			host: map[string]string{"os": "macos", "arch": "x86_64"},
			want: map[string]string{
				"os": "macos", "arch": "x86_64",
				"apply": "relkit-payload",
			},
		},
		{
			name: "publisher vocabulary survives alongside injections",
			host: map[string]string{"arch": "x64", "component": "console"},
			want: map[string]string{
				"arch": "x64", "component": "console",
				"os": runtime.GOOS, "apply": "relkit-payload",
			},
		},
	}
	for _, tc := range cases {
		got := withEngineDefaults(tc.host)
		if len(got) != len(tc.want) {
			t.Fatalf("%s: got %v want %v", tc.name, got, tc.want)
		}
		for key, want := range tc.want {
			if got[key] != want {
				t.Errorf("%s: %s=%q want %q", tc.name, key, got[key], want)
			}
		}
		// The input map must never be mutated: hosts may reuse it.
		if tc.host != nil {
			if _, mutated := tc.host["apply"]; mutated {
				t.Errorf("%s: input map mutated", tc.name)
			}
		}
	}
}
