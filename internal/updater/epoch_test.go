package updater

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	enginestate "github.com/shichao402/relkit/api/updater/enginestate/v1"
	updaterv2 "github.com/shichao402/relkit/api/updater/v2"
	"github.com/shichao402/relkit/internal/vnext/runtimebootstrap"
	"github.com/shichao402/relkit/internal/vnext/runtimeprofile"
	"github.com/shichao402/relkit/internal/vnext/trust"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// installedRoot builds a LIBRARY install root with the given codes installed
// and active.json pointing at activeCode.
func installedRoot(t *testing.T, codes []int64, activeCode int64) string {
	t.Helper()
	root := t.TempDir()
	for _, code := range codes {
		dir := filepath.Join(root, VersionsDirName, versionID(code))
		if err := os.MkdirAll(filepath.Join(dir, "bin"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "bin", "app.exe"), []byte("bin"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := writeVersionMeta(dir, versionMeta{
			Code:       code,
			Version:    versionID(code),
			Path:       filepath.ToSlash(filepath.Join(VersionsDirName, versionID(code))),
			Executable: "bin/app.exe",
		}); err != nil {
			t.Fatal(err)
		}
	}
	if activeCode != 0 {
		if err := writeActive(root, activePointer{
			Code:       activeCode,
			Version:    versionID(activeCode),
			Path:       filepath.ToSlash(filepath.Join(VersionsDirName, versionID(activeCode))),
			Executable: "bin/app.exe",
		}); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func versionID(code int64) string {
	return "1.0." + itoa(code)
}

func itoa(code int64) string {
	if code == 0 {
		return "0"
	}
	var digits []byte
	for code > 0 {
		digits = append([]byte{byte('0' + code%10)}, digits...)
		code /= 10
	}
	return string(digits)
}

func deleteReq(root string, op *updaterv2.DeleteInstalledOp) *updaterv2.UpdaterRequest {
	return &updaterv2.UpdaterRequest{
		Protocol: Protocol,
		Runtime: &updaterv2.Runtime{
			Channel: "dev",
			DataDir: filepath.Join(root, ".data"),
			Install: &updaterv2.InstallSpec{Placement: updaterv2.Placement_PLACEMENT_LIBRARY, InstallRoot: root},
		},
		Op: &updaterv2.UpdaterRequest_DeleteInstalled{DeleteInstalled: op},
	}
}

func decodeEvents(t *testing.T, buf *bytes.Buffer) []*updaterv2.UpdaterEvent {
	t.Helper()
	var out []*updaterv2.UpdaterEvent
	sc := bufio.NewScanner(buf)
	sc.Buffer(make([]byte, 0, 64<<10), MaxRequestBytes)
	for sc.Scan() {
		ev := &updaterv2.UpdaterEvent{}
		// Strict on purpose: the engine must never emit a field the schema
		// does not know.
		if err := protojson.Unmarshal(sc.Bytes(), ev); err != nil {
			t.Fatalf("event line %q: %v", sc.Text(), err)
		}
		out = append(out, ev)
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestDeleteInstalledRemovesInactiveVersion(t *testing.T) {
	root := installedRoot(t, []int64{2, 3}, 3)
	eng := &Engine{Version: "test", Stdout: &bytes.Buffer{}}
	if err := deleteInstalled(root, 2, nil); err != nil {
		t.Fatal(err)
	}
	remaining := listInstalled(root).GetVersions()
	if len(remaining) != 1 || remaining[0].Code != 3 {
		t.Fatalf("remaining versions = %+v", remaining)
	}
	if _, err := os.Stat(filepath.Join(root, VersionsDirName, versionID(2))); !os.IsNotExist(err) {
		t.Fatal("version 2 directory still present")
	}
	if entries, _ := os.ReadDir(filepath.Join(root, TrashDirName)); len(entries) != 0 {
		t.Fatalf("trash not emptied: %d entries", len(entries))
	}
	_ = eng
}

func TestDeleteInstalledRefusesActiveVersion(t *testing.T) {
	root := installedRoot(t, []int64{2, 3}, 3)
	err := deleteInstalled(root, 3, nil)
	if err == nil || !strings.Contains(err.Error(), "active") {
		t.Fatalf("active delete must be refused, got %v", err)
	}
	if len(listInstalled(root).GetVersions()) != 2 {
		t.Fatal("active version was deleted anyway")
	}
}

func TestDeleteInstalledRefusesCodeHostIsRunning(t *testing.T) {
	root := installedRoot(t, []int64{2, 3}, 3)
	err := deleteInstalled(root, 2, []int64{1, 2})
	if err == nil || !strings.Contains(err.Error(), "in use") {
		t.Fatalf("in-use delete must be refused, got %v", err)
	}
	if len(listInstalled(root).GetVersions()) != 2 {
		t.Fatal("in-use version was deleted anyway")
	}
}

func TestDeleteInstalledUnknownCode(t *testing.T) {
	root := installedRoot(t, []int64{3}, 3)
	if err := deleteInstalled(root, 9, nil); err == nil {
		t.Fatal("unknown code must be refused")
	}
}

func TestDeleteInstalledSharesApplyLock(t *testing.T) {
	root := installedRoot(t, []int64{2, 3}, 3)
	if err := acquireApplyLock(root, "apply-running"); err != nil {
		t.Fatal(err)
	}
	defer releaseApplyLock(root)
	var buf bytes.Buffer
	eng := &Engine{Version: "test", Stdout: &buf}
	if err := eng.handleDeleteInstalled(deleteReq(root, &updaterv2.DeleteInstalledOp{Code: 2}), &updaterv2.DeleteInstalledOp{Code: 2}); err != nil {
		t.Fatal(err)
	}
	events := decodeEvents(t, &buf)
	if len(events) == 0 {
		t.Fatal("no event emitted")
	}
	last := events[len(events)-1]
	if last.GetFailed() == nil || last.GetFailed().GetError().GetCode() != updaterv2.ErrorCode_ERROR_CODE_OCCUPIED {
		t.Fatalf("delete during apply must report OCCUPIED, got %v", last)
	}
	if len(listInstalled(root).GetVersions()) != 2 {
		t.Fatal("version was deleted while an apply held the lock")
	}
}

func TestDeleteInstalledRejectsInPlaceInstall(t *testing.T) {
	root := installedRoot(t, []int64{2, 3}, 3)
	req := deleteReq(root, &updaterv2.DeleteInstalledOp{Code: 2})
	req.Runtime.Install.Placement = updaterv2.Placement_PLACEMENT_IN_PLACE
	var buf bytes.Buffer
	eng := &Engine{Version: "test", Stdout: &buf}
	if err := eng.handleDeleteInstalled(req, &updaterv2.DeleteInstalledOp{Code: 2}); err != nil {
		t.Fatal(err)
	}
	events := decodeEvents(t, &buf)
	if len(events) == 0 {
		t.Fatal("no event emitted")
	}
	last := events[len(events)-1]
	if last.GetFailed().GetError().GetCode() != updaterv2.ErrorCode_ERROR_CODE_LAYOUT_UNSUPPORTED {
		t.Fatalf("IN_PLACE delete must be rejected, got %v", last)
	}
}

func TestCapabilitiesAdvertiseDeleteInstalledAndNoScheduler(t *testing.T) {
	eng := &Engine{Version: "test"}
	caps := eng.PublicCapabilities()
	hasDelete := false
	for _, op := range caps.Operations {
		if op == updaterv2.Operation_OPERATION_DELETE_INSTALLED {
			hasDelete = true
		}
		if op.String() == "OPERATION_SCHEDULER" {
			t.Fatal("scheduler must not be advertised (requirement 3.3)")
		}
	}
	if !hasDelete {
		t.Fatal("deleteInstalled must be advertised")
	}
	if caps.Protocol != 5 || Protocol != 5 {
		t.Fatalf("capabilities protocol=%d engine=%d want 5", caps.Protocol, Protocol)
	}
	for _, op := range caps.Operations {
		if op == 7 {
			t.Fatal("cancel (7) was removed in protocol 5 and must not be advertised")
		}
	}
}

func TestCapabilitiesOnlyProbeCarriesNoHandshake(t *testing.T) {
	caps := (&Engine{Version: "test"}).PublicCapabilities()
	if caps.GetHandshake() != nil {
		t.Fatal("--capabilities probe must not claim a validated profile")
	}
}

// bootstrapDocument builds a real Runtime Bootstrap the same way the publisher
// does, so the fixture can never drift from the generator (requirement 3.9).
func bootstrapDocument(t *testing.T, product string) (string, ed25519.PublicKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	profile := runtimeprofile.Document{
		Schema:          runtimeprofile.Schema,
		Product:         product,
		ProfileVersion:  7,
		AllowedChannels: []string{"dev", "stable"},
		DirectoryBootstrap: runtimeprofile.DirectoryBootstrap{
			EntryURLs: []string{"https://update.example.invalid/"},
		},
		RUPSchemaSet: runtimeprofile.SchemaSet,
	}
	if err := profile.Sign("root-key", priv.Seed()); err != nil {
		t.Fatal(err)
	}
	doc, err := runtimebootstrap.New("root-key", pub, product, profile)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw), pub
}

func TestOpenProfileDerivesProfileAndReport(t *testing.T) {
	raw, pub := bootstrapDocument(t, "loom")
	profile := &updaterv2.ClientProfile{Bootstrap: raw, ExpectedProduct: "loom"}
	resolved, report, err := openProfile(profile, "dev")
	if err != nil {
		t.Fatalf("resolveBootstrap: %v", err)
	}
	if resolved.Product != "loom" {
		t.Fatalf("product = %q", resolved.Product)
	}
	if len(resolved.EntryURLs) != 1 || resolved.EntryURLs[0] != "https://update.example.invalid/" {
		t.Fatalf("entryUrls = %v", resolved.EntryURLs)
	}
	if len(resolved.TrustedKeys) != 1 {
		t.Fatalf("trustedKeys = %v", resolved.TrustedKeys)
	}
	if !resolved.TrustedKeys["root-key"].Equal(pub) {
		t.Fatal("embedded server root was not adopted as the trusted key")
	}
	if report == nil {
		t.Fatal("no handshake report")
	}
	if report.Product != "loom" || report.DefaultChannel != "dev" || report.RootKeyId != "root-key" {
		t.Fatalf("report = %+v", report)
	}
	if len(report.AllowedChannels) != 2 {
		t.Fatalf("allowedChannels = %v", report.AllowedChannels)
	}
}

func TestOpenProfileRejectsProductMismatch(t *testing.T) {
	raw, _ := bootstrapDocument(t, "loom")
	_, _, err := openProfile(&updaterv2.ClientProfile{Bootstrap: raw, ExpectedProduct: "loom-launcher"}, "dev")
	if err == nil || err.Code != updaterv2.ErrorCode_ERROR_CODE_PROFILE_INVALID {
		t.Fatalf("product mismatch must be PROFILE_INVALID, got %v", err)
	}
}

func TestOpenProfileRejectsChannelOutsideProfile(t *testing.T) {
	raw, _ := bootstrapDocument(t, "loom")
	_, _, err := openProfile(&updaterv2.ClientProfile{Bootstrap: raw, ExpectedProduct: "loom"}, "canary")
	if err == nil || err.Code != updaterv2.ErrorCode_ERROR_CODE_CHANNEL_NOT_ALLOWED {
		t.Fatalf("channel must be refused, got %v", err)
	}
}

// The 2026-10-17 accident: rootPublicKey shipped as an array of numbers. The
// typed document must reject it loudly instead of hashing garbage.
func TestOpenProfileRejectsNumericPublicKey(t *testing.T) {
	raw, _ := bootstrapDocument(t, "loom")
	var doc map[string]any
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatal(err)
	}
	doc["rootPublicKey"] = []int{115, 115, 104}
	broken, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	_, _, rerr := openProfile(&updaterv2.ClientProfile{Bootstrap: string(broken), ExpectedProduct: "loom"}, "dev")
	if rerr == nil {
		t.Fatal("numeric public key must be rejected")
	}
	if rerr.Code != updaterv2.ErrorCode_ERROR_CODE_PROFILE_INVALID {
		t.Fatalf("code = %v", rerr.Code)
	}
}

func TestOpenProfileRejectsTamperedProfile(t *testing.T) {
	raw, _ := bootstrapDocument(t, "loom")
	var doc map[string]any
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatal(err)
	}
	profile := doc["profile"].(map[string]any)
	profile["allowedChannels"] = []string{"dev", "stable", "canary"}
	tampered, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	_, _, rerr := openProfile(&updaterv2.ClientProfile{Bootstrap: string(tampered), ExpectedProduct: "loom"}, "dev")
	if rerr == nil {
		t.Fatal("profile signature must be verified")
	}
}

func TestOpenProfileRequiresBootstrapAndExpectedProduct(t *testing.T) {
	raw, _ := bootstrapDocument(t, "loom")
	for name, profile := range map[string]*updaterv2.ClientProfile{
		"nil":               nil,
		"no bootstrap":      {ExpectedProduct: "loom"},
		"no expected":       {Bootstrap: raw},
		"bootstrap is junk": {Bootstrap: "{}", ExpectedProduct: "loom"},
	} {
		_, report, err := openProfile(profile, "dev")
		if err == nil || err.Code != updaterv2.ErrorCode_ERROR_CODE_PROFILE_INVALID {
			t.Fatalf("%s: want PROFILE_INVALID, got %v", name, err)
		}
		if report != nil {
			t.Fatalf("%s: rejected profile must not produce a report", name)
		}
	}
}

func TestHandshakeCarriesReportToHost(t *testing.T) {
	raw, _ := bootstrapDocument(t, "loom")
	var buf bytes.Buffer
	eng := &Engine{Version: "test", Stdout: &buf}
	req := &updaterv2.UpdaterRequest{
		Protocol: Protocol,
		Profile:  &updaterv2.ClientProfile{Bootstrap: raw, ExpectedProduct: "loom"},
		Runtime:  &updaterv2.Runtime{Channel: "dev", DataDir: t.TempDir()},
		Op:       &updaterv2.UpdaterRequest_Status{Status: &updaterv2.StatusOp{}},
	}
	if err := eng.HandleRequest(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	events := decodeEvents(t, &buf)
	if len(events) == 0 || events[0].GetCapabilities() == nil {
		t.Fatal("first event must be capabilities")
	}
	report := events[0].GetCapabilities().GetHandshake()
	if report == nil {
		t.Fatal("validated bootstrap must be reported back")
	}
	if report.Product != "loom" || report.DefaultChannel != "dev" {
		t.Fatalf("report = %+v", report)
	}
}

func TestHandshakeRejectsProtocolBeforeCapabilities(t *testing.T) {
	raw, _ := bootstrapDocument(t, "loom")
	for _, tc := range []struct {
		protocol uint32
		want     updaterv2.ErrorCode
	}{
		{0, updaterv2.ErrorCode_ERROR_CODE_PROTOCOL_MISMATCH},
		{4, updaterv2.ErrorCode_ERROR_CODE_UPDATER_TOO_NEW},
		{6, updaterv2.ErrorCode_ERROR_CODE_UPDATER_TOO_OLD},
	} {
		var buf bytes.Buffer
		eng := &Engine{Version: "test", Stdout: &buf}
		req := &updaterv2.UpdaterRequest{
			Protocol: tc.protocol,
			Profile:  &updaterv2.ClientProfile{Bootstrap: raw, ExpectedProduct: "loom"},
			Runtime:  &updaterv2.Runtime{Channel: "dev", DataDir: t.TempDir()},
			Op:       &updaterv2.UpdaterRequest_Status{Status: &updaterv2.StatusOp{}},
		}
		if err := eng.HandleRequest(context.Background(), req); err != nil {
			t.Fatal(err)
		}
		events := decodeEvents(t, &buf)
		if len(events) != 2 {
			t.Fatalf("protocol %d: want capabilities + failure, got %d events", tc.protocol, len(events))
		}
		if events[0].GetCapabilities().GetHandshake() != nil {
			t.Fatalf("protocol %d: rejected host must not get a handshake report", tc.protocol)
		}
		if got := events[1].GetFailed().GetError().GetCode(); got != tc.want {
			t.Fatalf("protocol %d: code = %v want %v", tc.protocol, got, tc.want)
		}
	}
}

func TestClassifyApplyDistinguishesPayloadAndInstaller(t *testing.T) {
	payload := &enginestate.UpdatePlan{Files: []*enginestate.PlannedFile{{Name: "app-payload.zip", Kind: "payload"}}}
	d, rel, err := classifyApply(payload)
	if err != nil || d != updaterv2.ApplyDisposition_APPLY_DISPOSITION_INTERNAL || rel != "" {
		t.Fatalf("payload: %v %q %v", d, rel, err)
	}
	installer := &enginestate.UpdatePlan{Files: []*enginestate.PlannedFile{{Name: "setup.exe", Kind: "installer"}}}
	d, rel, err = classifyApply(installer)
	if err != nil || d != updaterv2.ApplyDisposition_APPLY_DISPOSITION_FULL_INSTALL || rel != "setup.exe" {
		t.Fatalf("installer: %v %q %v", d, rel, err)
	}
	unknown := &enginestate.UpdatePlan{Files: []*enginestate.PlannedFile{{Name: "x.bin", Kind: "blob"}}}
	if _, _, err := classifyApply(unknown); err == nil {
		t.Fatal("blob must not be silently installed")
	}
	if _, _, err := classifyApply(nil); err == nil {
		t.Fatal("nil plan must be rejected")
	}
}

func TestFullInstallRefusedOnlyOnDarwin(t *testing.T) {
	for _, goos := range []string{"darwin"} {
		if !fullInstallRefused(goos) {
			t.Fatalf("%s must refuse installer handoff (requirement 3.11)", goos)
		}
	}
	for _, goos := range []string{"windows", "linux", "freebsd"} {
		if fullInstallRefused(goos) {
			t.Fatalf("%s must accept installer handoff", goos)
		}
	}
}

func TestFullInstallGateUsesRunningPlatform(t *testing.T) {
	want := runtime.GOOS == "darwin"
	if got := fullInstallRefused(runtime.GOOS); got != want {
		t.Fatalf("fullInstallRefused(%q) = %v", runtime.GOOS, got)
	}
}

func TestInstallerExitClassification(t *testing.T) {
	if !isUserCancelledExit(1) || !isUserCancelledExit(2) {
		t.Fatal("wizard cancellation codes must be recognised")
	}
	for _, code := range []int{0, 3, 5, 1603} {
		if isUserCancelledExit(code) {
			t.Fatalf("exit %d must be treated as a failure, not a cancel", code)
		}
	}
}

func TestRunFullInstallMissingInstaller(t *testing.T) {
	sess := &enginestate.ApplySessionRecord{StagedRoot: t.TempDir(), InstallerRelpath: "setup.exe"}
	err := runFullInstall(sess, filepath.Join(sess.StagedRoot, "setup.exe"))
	if err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("missing installer must fail, got %v", err)
	}
}

func TestStatusReportsFullInstallFailure(t *testing.T) {
	dataDir := t.TempDir()
	root := t.TempDir()
	st := store{dataDir: dataDir}
	sess := &enginestate.ApplySessionRecord{
		SessionId:        "sess-full",
		PlanId:           "plan-full",
		Phase:            updaterv2.SessionPhase_SESSION_PHASE_NEEDS_ATTENTION,
		StartedAt:        timestamppb.Now(),
		InstallRoot:      root,
		StagedRoot:       filepath.Join(dataDir, "staging", "plan-full"),
		Disposition:      updaterv2.ApplyDisposition_APPLY_DISPOSITION_FULL_INSTALL,
		InstallerRelpath: "setup.exe",
		Error:            newError(updaterv2.ErrorCode_ERROR_CODE_INSTALLER_FAILED, false, "installer exited with code 5", nil),
	}
	if err := st.saveSession(sess); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	eng := &Engine{Version: "test", Stdout: &buf}
	if err := eng.handleStatus(&updaterv2.UpdaterRequest{}, st); err != nil {
		t.Fatal(err)
	}
	events := decodeEvents(t, &buf)
	if len(events) != 1 || events[0].GetStatus() == nil {
		t.Fatalf("want one status event, got %v", events)
	}
	active := events[0].GetStatus().GetActiveSession()
	if active == nil || active.GetPhase() != updaterv2.SessionPhase_SESSION_PHASE_NEEDS_ATTENTION {
		t.Fatalf("active session = %+v", active)
	}
	if active.GetError().GetCode() != updaterv2.ErrorCode_ERROR_CODE_INSTALLER_FAILED {
		t.Fatalf("error code = %v", active.GetError().GetCode())
	}
}

func TestNoCompatibilitySessionFileIsWritten(t *testing.T) {
	root := t.TempDir()
	dataDir := t.TempDir()
	st := store{dataDir: dataDir}
	sess := &enginestate.ApplySessionRecord{
		SessionId:   "sess-1",
		Phase:       updaterv2.SessionPhase_SESSION_PHASE_COMPLETED,
		InstallRoot: root,
		Placement:   updaterv2.Placement_PLACEMENT_LIBRARY,
	}
	if err := st.saveSession(sess); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{root, dataDir} {
		if _, err := os.Stat(filepath.Join(dir, "update_apply.json")); !os.IsNotExist(err) {
			t.Fatalf("%s still gets the legacy session file", dir)
		}
	}
}

func TestValidateArtifactFilenameRejectsInstallerTraversal(t *testing.T) {
	plan := &enginestate.UpdatePlan{Files: []*enginestate.PlannedFile{{
		Name:        "../setup.exe",
		Kind:        "installer",
		DestRelpath: "../setup.exe",
	}}}
	if _, _, err := classifyApply(plan); err == nil {
		t.Fatal("path traversal in installer relpath must be rejected")
	}
}

func TestTrustSSHPublicKeyLineIsWhatBootstrapEmbeds(t *testing.T) {
	raw, pub := bootstrapDocument(t, "svn-auto-merge")
	var doc runtimebootstrap.Document
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatal(err)
	}
	parsed, err := trust.ParsePublicKey(doc.RootPublicKey)
	if err != nil {
		t.Fatal(err)
	}
	if !parsed.Equal(pub) {
		t.Fatal("bootstrap embeds a different key than the one it was built from")
	}
}
