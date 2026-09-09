package updater

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	updaterv1 "cnb.cool/shichao402/relkit/api/updater/v1"
	"google.golang.org/protobuf/types/known/durationpb"
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
	if err := WriteFrame(&buf, ev); err != nil {
		t.Fatal(err)
	}
	got := &updaterv1.UpdaterEvent{}
	if err := ReadFrame(&buf, got); err != nil {
		t.Fatal(err)
	}
	if got.GetLog().GetMessage() != "hi" {
		t.Fatalf("%v", got)
	}
}

func TestNegotiateWindow(t *testing.T) {
	if negotiate(&updaterv1.ClientHello{IpcMin: 1, IpcMax: 1}) != nil {
		t.Fatal("current window")
	}
	if negotiate(&updaterv1.ClientHello{IpcMin: 2, IpcMax: 2}).Code != updaterv1.ErrorCode_ERROR_CODE_UPDATER_TOO_OLD {
		t.Fatal("too new host")
	}
	if negotiate(&updaterv1.ClientHello{IpcMin: 0, IpcMax: 0}).Code != updaterv1.ErrorCode_ERROR_CODE_UPDATER_TOO_NEW &&
		negotiate(&updaterv1.ClientHello{IpcMax: 0}).Code != updaterv1.ErrorCode_ERROR_CODE_PROTOCOL_MISMATCH {
		// ipc max 0 < IPCMin → too new (host older)
		got := negotiate(&updaterv1.ClientHello{IpcMin: 1, IpcMax: 0})
		if got == nil {
			t.Fatal("expected error")
		}
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
	if err == nil || err.Code != updaterv1.ErrorCode_ERROR_CODE_PROFILE_INVALID {
		t.Fatalf("code 0: %v", err)
	}
}

func TestFileSetRollback(t *testing.T) {
	root := t.TempDir()
	oldA := filepath.Join(root, "a.exe")
	oldB := filepath.Join(root, "b.exe")
	if err := os.WriteFile(oldA, []byte("old-a"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(oldB, []byte("old-b"), 0o755); err != nil {
		t.Fatal(err)
	}
	stage := t.TempDir()
	newA := filepath.Join(stage, "a.exe")
	newB := filepath.Join(stage, "b.exe")
	_ = os.WriteFile(newA, []byte("new-a"), 0o644)
	_ = os.WriteFile(newB, []byte("new-b"), 0o644)
	sess := &updaterv1.ApplySessionRecord{
		InstallRoot: root,
		StagedRoot:  stage,
		FileSet: []*updaterv1.FileSetEntry{
			{DestRelpath: "a.exe", ArtifactName: "a.exe"},
			{DestRelpath: "missing.exe", ArtifactName: "nope.exe"},
		},
	}
	plan := &updaterv1.UpdatePlan{Files: []*updaterv1.PlannedFile{
		{Name: "a.exe", LocalPath: newA},
		{Name: "b.exe", LocalPath: newB},
	}}
	if err := applyFileSet(sess, plan); err == nil {
		t.Fatal("expected missing artifact to fail")
	}
	got, _ := os.ReadFile(oldA)
	if string(got) != "old-a" {
		t.Fatalf("partial apply leaked: %s", got)
	}
}

func TestVersionedDirAtomicActive(t *testing.T) {
	root := t.TempDir()
	stage := t.TempDir()
	payload := filepath.Join(stage, "unpacked")
	_ = os.MkdirAll(filepath.Join(payload, "bin"), 0o755)
	_ = os.WriteFile(filepath.Join(payload, "bin", "app.exe"), []byte("n"), 0o644)
	// applyVersionedDir unpacks first file; give it a directory tree via non-zip local path dir
	plan := &updaterv1.UpdatePlan{
		Version: "1.0.0",
		Code:    2,
		Files:   []*updaterv1.PlannedFile{{Name: "app.exe", LocalPath: filepath.Join(payload, "bin", "app.exe")}},
	}
	sess := &updaterv1.ApplySessionRecord{
		InstallRoot:       root,
		StagedRoot:        stage,
		ExecutableRelpath: "bin/app.exe",
		Retain:            2,
	}
	if err := applyVersionedDir(sess, plan); err != nil {
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
	if err := ReadFrame(&buf, ev); err != nil {
		t.Fatal(err)
	}
	if ev.GetCapabilities() == nil {
		t.Fatal("first event must be capabilities")
	}
	ev2 := &updaterv1.UpdaterEvent{}
	if err := ReadFrame(&buf, ev2); err != nil {
		t.Fatal(err)
	}
	if ev2.GetFailed() == nil {
		t.Fatalf("expected hello failure, got %v", ev2)
	}
}
