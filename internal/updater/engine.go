package updater

import (
	"context"
	"crypto/ed25519"
	"io"
	"os"

	updaterv1 "github.com/shichao402/relkit/api/updater/v1"
	"github.com/shichao402/relkit/internal/ipc"
	"github.com/shichao402/relkit/internal/vnext/runtimebootstrap"
	"github.com/shichao402/relkit/internal/sdk"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Engine is the unique updater implementation.
type Engine struct {
	Version string
	Fetcher sdk.Fetcher
	Now     func() timestamppb.Timestamp
	Stdout  io.Writer
	// LaunchWorker is injectable for tests. Production copies the running
	// updater into staging and starts it independently of the host install tree.
	LaunchWorker func(dataDir, sessionID, stagedRoot string) error
}

func (e *Engine) writer() io.Writer {
	if e.Stdout != nil {
		return e.Stdout
	}
	return os.Stdout
}

func (e *Engine) emit(ev *updaterv1.UpdaterEvent) error {
	return ipc.WriteFrame(e.writer(), ev)
}

// PublicCapabilities is the handshake payload for a host that has not opened a
// profile yet (the --capabilities probe).
func (e *Engine) PublicCapabilities() *updaterv1.Capabilities {
	return e.capabilities(nil)
}

func (e *Engine) capabilities(report *updaterv1.HandshakeReport) *updaterv1.Capabilities {
	ver := e.Version
	if ver == "" {
		ver = "dev"
	}
	return &updaterv1.Capabilities{
		Ipc: IPCCurrent,
		Operations: []updaterv1.Operation{
			updaterv1.Operation_OPERATION_CHECK,
			updaterv1.Operation_OPERATION_SKIP,
			updaterv1.Operation_OPERATION_DOWNLOAD,
			updaterv1.Operation_OPERATION_APPLY,
			updaterv1.Operation_OPERATION_STATUS,
			updaterv1.Operation_OPERATION_CLEANUP,
			updaterv1.Operation_OPERATION_CANCEL,
			updaterv1.Operation_OPERATION_LIST_INSTALLED,
			updaterv1.Operation_OPERATION_SWITCH_ACTIVE,
			updaterv1.Operation_OPERATION_ROLLBACK,
			updaterv1.Operation_OPERATION_DELETE_INSTALLED,
		},
		Placements: []updaterv1.Placement{
			updaterv1.Placement_PLACEMENT_IN_PLACE,
			updaterv1.Placement_PLACEMENT_LIBRARY,
		},
		MinCheckInterval: minCheckDurationPB(),
		PlanTtl:          planTTLDurationPB(),
		EngineVersion:    ver,
		Handshake:        report,
	}
}

func negotiate(hello *updaterv1.ClientHello) *updaterv1.Error {
	if hello == nil || hello.IpcMin == 0 && hello.IpcMax == 0 {
		return newError(updaterv1.ErrorCode_ERROR_CODE_PROTOCOL_MISMATCH, false, "missing ClientHello", nil)
	}
	if hello.IpcMax < IPCMin {
		return newError(updaterv1.ErrorCode_ERROR_CODE_UPDATER_TOO_NEW, false,
			"host IPC window is older than the engine baseline (window 4); this epoch does not migrate hosts", nil)
	}
	if hello.IpcMin > IPCMax {
		return newError(updaterv1.ErrorCode_ERROR_CODE_UPDATER_TOO_OLD, false,
			"host IPC window is newer than this engine; update the updater sidecar", nil)
	}
	return nil
}

// resolveBootstrap validates the Runtime Bootstrap a host forwarded verbatim
// and returns the profile the engine will actually use plus the handshake
// report the host reads product/channels from (requirement 3.8).
//
// The engine is the single parser: schema, key encoding, fingerprint, product
// consistency, channel membership and the profile signature all get checked
// here. Hosts that still build a ClientProfile without a bootstrap keep
// working, but then nothing is reported back.
func resolveBootstrap(profile *updaterv1.ClientProfile, channel string) (*updaterv1.ClientProfile, *updaterv1.HandshakeReport, *updaterv1.Error) {
	raw := profile.GetBootstrap()
	if raw == "" {
		if profile.GetExpectedProduct() != "" && profile.GetExpectedProduct() != profile.GetProduct() {
			return nil, nil, newError(updaterv1.ErrorCode_ERROR_CODE_PROFILE_INVALID, false,
				"expectedProduct does not match profile.product", nil)
		}
		if profile.GetProduct() == "" {
			return nil, nil, newError(updaterv1.ErrorCode_ERROR_CODE_PROFILE_INVALID, false, "product required", nil)
		}
		if err := channelAllowed(profile.GetAllowedChannels(), channel); err != nil {
			return nil, nil, err
		}
		return profile, nil, nil
	}

	var doc runtimebootstrap.Document
	parsed, perr := decodeBootstrap([]byte(raw))
	if perr != nil {
		return nil, nil, newError(updaterv1.ErrorCode_ERROR_CODE_PROFILE_INVALID, false, perr.Error(), nil)
	}
	doc = *parsed
	if err := doc.Validate(); err != nil {
		return nil, nil, newError(updaterv1.ErrorCode_ERROR_CODE_PROFILE_INVALID, false,
			"runtime bootstrap: "+err.Error(), nil)
	}
	expected := profile.GetExpectedProduct()
	if expected == "" {
		expected = profile.GetProduct()
	}
	if expected != "" && expected != doc.Product {
		return nil, nil, newError(updaterv1.ErrorCode_ERROR_CODE_PROFILE_INVALID, false,
			"bootstrap belongs to product "+doc.Product+", host expected "+expected, nil)
	}
	if err := channelAllowed(doc.Profile.AllowedChannels, channel); err != nil {
		return nil, nil, err
	}
	disagreement := func(field, fromHost, fromBootstrap string) *updaterv1.Error {
		return newError(updaterv1.ErrorCode_ERROR_CODE_PROFILE_INVALID, false,
			field+" disagrees with the runtime bootstrap ("+fromHost+" vs "+fromBootstrap+")", nil)
	}
	if p := profile.GetProduct(); p != "" && p != doc.Product {
		return nil, nil, disagreement("profile.product", p, doc.Product)
	}
	if hosts := profile.GetAllowedChannels(); len(hosts) > 0 && !sameStrings(hosts, doc.Profile.AllowedChannels) {
		return nil, nil, disagreement("profile.allowedChannels", join(hosts), join(doc.Profile.AllowedChannels))
	}
	if hosts := profile.GetEntryUrls(); len(hosts) > 0 && !sameStrings(hosts, doc.Profile.DirectoryBootstrap.EntryURLs) {
		return nil, nil, disagreement("profile.entryUrls", join(hosts), join(doc.Profile.DirectoryBootstrap.EntryURLs))
	}

	pub, err := doc.PublicKey()
	if err != nil {
		return nil, nil, newError(updaterv1.ErrorCode_ERROR_CODE_PROFILE_INVALID, false, err.Error(), nil)
	}
	keys := append([]*updaterv1.TrustedKey(nil), profile.GetTrustedKeys()...)
	if !hasKey(keys, doc.RootKeyID, pub) {
		keys = append(keys, &updaterv1.TrustedKey{KeyId: doc.RootKeyID, PublicKey: pub})
	}

	resolved := &updaterv1.ClientProfile{
		Product:         doc.Product,
		AllowedChannels: append([]string(nil), doc.Profile.AllowedChannels...),
		EntryUrls:       append([]string(nil), doc.Profile.DirectoryBootstrap.EntryURLs...),
		IndexUrls:       append([]string(nil), profile.GetIndexUrls()...),
		FallbackUrls:    append([]string(nil), profile.GetFallbackUrls()...),
		TrustedKeys:     keys,
		Recovery:        profile.GetRecovery(),
	}
	report := &updaterv1.HandshakeReport{
		Product:         doc.Product,
		AllowedChannels: append([]string(nil), doc.Profile.AllowedChannels...),
		DefaultChannel:  doc.Profile.AllowedChannels[0],
		RootKeyId:       doc.RootKeyID,
		RootFingerprint: doc.RootFingerprint,
	}
	return resolved, report, nil
}

func channelAllowed(allowed []string, channel string) *updaterv1.Error {
	if len(allowed) == 0 || channel == "" {
		return nil
	}
	for _, c := range allowed {
		if c == channel {
			return nil
		}
	}
	return newError(updaterv1.ErrorCode_ERROR_CODE_CHANNEL_NOT_ALLOWED, false,
		"channel "+channel+" is not in allowedChannels", nil)
}

func hasKey(keys []*updaterv1.TrustedKey, keyID string, pub ed25519.PublicKey) bool {
	for _, k := range keys {
		if k != nil && k.GetKeyId() == keyID && ed25519.PublicKey(k.GetPublicKey()).Equal(pub) {
			return true
		}
	}
	return false
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func join(values []string) string {
	out := ""
	for i, v := range values {
		if i > 0 {
			out += ","
		}
		out += v
	}
	return out
}

func validateOpen(profile *updaterv1.ClientProfile, runtime *updaterv1.Runtime) *updaterv1.Error {
	if profile == nil || runtime == nil {
		return newError(updaterv1.ErrorCode_ERROR_CODE_PROFILE_INVALID, false, "profile and runtime required", nil)
	}
	if profile.Product == "" {
		return newError(updaterv1.ErrorCode_ERROR_CODE_PROFILE_INVALID, false, "product required", nil)
	}
	if len(profile.TrustedKeys) == 0 {
		return newError(updaterv1.ErrorCode_ERROR_CODE_PROFILE_INVALID, false, "trustedKeys required", nil)
	}
	if len(profile.EntryUrls) == 0 && len(profile.IndexUrls) == 0 {
		return newError(updaterv1.ErrorCode_ERROR_CODE_PROFILE_INVALID, false, "entryUrls or indexUrls required", nil)
	}
	if runtime.CurrentCode < 0 {
		return newError(updaterv1.ErrorCode_ERROR_CODE_PROFILE_INVALID, false, "currentCode must not be negative", nil)
	}
	if runtime.Channel == "" {
		return newError(updaterv1.ErrorCode_ERROR_CODE_PROFILE_INVALID, false, "channel required", nil)
	}
	if err := channelAllowed(profile.AllowedChannels, runtime.Channel); err != nil {
		return err
	}
	if runtime.DataDir == "" {
		return newError(updaterv1.ErrorCode_ERROR_CODE_PROFILE_INVALID, false, "dataDir required", nil)
	}
	if inst := runtime.Install; inst != nil {
		if inst.Placement == updaterv1.Placement_PLACEMENT_UNSPECIFIED {
			return newError(updaterv1.ErrorCode_ERROR_CODE_LAYOUT_UNSUPPORTED, false, "placement required", nil)
		}
	}
	return nil
}

func trustedKeys(profile *updaterv1.ClientProfile) sdk.TrustedKeys {
	out := sdk.TrustedKeys{}
	for _, k := range profile.TrustedKeys {
		if k == nil || len(k.PublicKey) != ed25519.PublicKeySize {
			continue
		}
		out[k.KeyId] = ed25519.PublicKey(k.PublicKey)
	}
	return out
}

// HandleRequest processes one UpdaterRequest and writes events to stdout.
func (e *Engine) HandleRequest(ctx context.Context, req *updaterv1.UpdaterRequest) error {
	// Negotiation happens before anything else so an incompatible host gets one
	// clear error instead of a partially useful capabilities frame.
	if nego := negotiate(req.GetHello()); nego != nil {
		if err := e.emit(&updaterv1.UpdaterEvent{
			Kind: &updaterv1.UpdaterEvent_Capabilities{Capabilities: e.capabilities(nil)},
		}); err != nil {
			return err
		}
		return e.emit(failedEvent(nego.Code, nego.Retryable, nego.Message, nil))
	}

	profile, report, berr := resolveBootstrap(req.GetProfile(), req.GetRuntime().GetChannel())
	if berr != nil {
		if err := e.emit(&updaterv1.UpdaterEvent{
			Kind: &updaterv1.UpdaterEvent_Capabilities{Capabilities: e.capabilities(nil)},
		}); err != nil {
			return err
		}
		return e.emit(failedEvent(berr.Code, berr.Retryable, berr.Message, nil))
	}
	if err := e.emit(&updaterv1.UpdaterEvent{
		Kind: &updaterv1.UpdaterEvent_Capabilities{Capabilities: e.capabilities(report)},
	}); err != nil {
		return err
	}
	if verr := validateOpen(profile, req.GetRuntime()); verr != nil {
		if !opToleratesIncompleteProfile(req) {
			return e.emit(failedEvent(verr.Code, verr.Retryable, verr.Message, nil))
		}
		if req.GetRuntime() == nil || req.GetRuntime().DataDir == "" {
			return e.emit(failedEvent(verr.Code, verr.Retryable, verr.Message, nil))
		}
	}

	st := store{
		dataDir: req.GetRuntime().GetDataDir(),
		product: profile.GetProduct(),
		channel: req.GetRuntime().GetChannel(),
	}
	// Downstream handlers read the profile straight off the request, so the
	// resolved one (bootstrap-derived) replaces whatever the host sent.
	req.Profile = profile
	switch op := req.GetOp().(type) {
	case *updaterv1.UpdaterRequest_Check:
		return e.handleCheck(ctx, req, op.Check, st)
	case *updaterv1.UpdaterRequest_Skip:
		return e.handleSkip(ctx, req, op.Skip, st)
	case *updaterv1.UpdaterRequest_Download:
		return e.handleDownload(ctx, req, op.Download, st)
	case *updaterv1.UpdaterRequest_Apply:
		return e.handleApply(ctx, req, op.Apply, st)
	case *updaterv1.UpdaterRequest_Status:
		return e.handleStatus(req, st)
	case *updaterv1.UpdaterRequest_Cleanup:
		return e.handleCleanup(req, st)
	case *updaterv1.UpdaterRequest_Cancel:
		return e.emit(&updaterv1.UpdaterEvent{
			Kind: &updaterv1.UpdaterEvent_Result{Result: &updaterv1.Result{Kind: &updaterv1.Result_Ok{Ok: &updaterv1.Ok{}}}},
		})
	case *updaterv1.UpdaterRequest_ListInstalled:
		return e.handleListInstalled(req)
	case *updaterv1.UpdaterRequest_SwitchActive:
		return e.handleSwitchActive(req, op.SwitchActive)
	case *updaterv1.UpdaterRequest_Rollback:
		return e.handleRollback(req)
	case *updaterv1.UpdaterRequest_DeleteInstalled:
		return e.handleDeleteInstalled(req, op.DeleteInstalled)
	default:
		return e.emit(failedEvent(updaterv1.ErrorCode_ERROR_CODE_PROFILE_INVALID, false, "missing operation", nil))
	}
}

// opToleratesIncompleteProfile reports whether an op can run with only a
// dataDir, which is what a host needs after a crash or when it just wants the
// installed inventory.
func opToleratesIncompleteProfile(req *updaterv1.UpdaterRequest) bool {
	switch req.GetOp().(type) {
	case *updaterv1.UpdaterRequest_Status,
		*updaterv1.UpdaterRequest_Cleanup,
		*updaterv1.UpdaterRequest_ListInstalled:
		return true
	}
	return false
}

func (e *Engine) handleStatus(req *updaterv1.UpdaterRequest, st store) error {
	state, err := st.loadState()
	if err != nil {
		return e.emit(failedEvent(updaterv1.ErrorCode_ERROR_CODE_DISK, false, err.Error(), nil))
	}
	snap := &updaterv1.StatusSnapshot{
		LastResult:       state.LastResult,
		LastSeenSequence: state.LastSeenSequence,
		SkippedCodes:     state.SkippedCodes,
		LastCheckAt:      state.LastCheckAt,
		Sidecar: &updaterv1.SidecarInfo{
			Ipc:     IPCCurrent,
			Version: e.Version,
		},
	}
	ok, next := shouldCheck(lastCheckTime(state), state.LastResult, nil, false, timestamppb.Now().AsTime())
	if !ok {
		snap.NextAllowedAt = timestamppb.New(next)
	}
	if sess, _ := st.latestSession(); sess != nil {
		snap.ActiveSession = &updaterv1.SessionView{
			SessionId: sess.SessionId,
			PlanId:    sess.PlanId,
			Phase:     sess.Phase,
			StartedAt: sess.StartedAt,
			Error:     sess.Error,
		}
	}
	_ = req
	return e.emit(&updaterv1.UpdaterEvent{Kind: &updaterv1.UpdaterEvent_Status{Status: snap}})
}

func (e *Engine) handleCleanup(req *updaterv1.UpdaterRequest, st store) error {
	sess, err := st.latestSession()
	if err != nil {
		return e.emit(failedEvent(updaterv1.ErrorCode_ERROR_CODE_DISK, false, err.Error(), nil))
	}
	if sess != nil {
		switch sess.Phase {
		case updaterv1.SessionPhase_SESSION_PHASE_COMPLETED, updaterv1.SessionPhase_SESSION_PHASE_ROLLED_BACK:
			if sess.StagedRoot != "" {
				_ = os.RemoveAll(sess.StagedRoot)
			}
		}
	}
	_ = req
	return e.emit(&updaterv1.UpdaterEvent{
		Kind: &updaterv1.UpdaterEvent_Result{Result: &updaterv1.Result{Kind: &updaterv1.Result_Ok{Ok: &updaterv1.Ok{}}}},
	})
}

func recoveryFrom(profile *updaterv1.ClientProfile) *updaterv1.RecoveryHelp {
	if profile == nil {
		return nil
	}
	return profile.Recovery
}

func (e *Engine) fetcher() sdk.Fetcher {
	if e.Fetcher != nil {
		return e.Fetcher
	}
	return &sdk.HTTPFetcher{}
}
