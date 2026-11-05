package updater

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"time"

	enginestate "github.com/shichao402/relkit/api/updater/enginestate/v1"
	updaterv2 "github.com/shichao402/relkit/api/updater/v2"
	"github.com/shichao402/relkit/internal/sdk"
	"google.golang.org/protobuf/encoding/protojson"
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
	// newProgressUI is injectable for tests; production uses the platform
	// window (ADR 0026 §6).
	newProgressUI func(progressSpec) progressUI
}

func (e *Engine) writer() io.Writer {
	if e.Stdout != nil {
		return e.Stdout
	}
	return os.Stdout
}

func (e *Engine) emit(ev *updaterv2.UpdaterEvent) error {
	return WriteEvent(e.writer(), ev)
}

// WriteEvent writes one UpdaterEvent as a single ProtoJSON line (ADR 0026).
// protojson never emits raw newlines in single-line mode, so "\n" is a safe
// record separator.
func WriteEvent(w io.Writer, ev *updaterv2.UpdaterEvent) error {
	raw, err := protojson.Marshal(ev)
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	_, err = w.Write(raw)
	return err
}

// MaxRequestBytes bounds the stdin request. A bootstrap is a few KiB; anything
// near this limit is a host bug, not a real request.
const MaxRequestBytes = 32 << 20

// ReadRequest reads stdin to EOF and decodes it strictly: unknown fields are
// rejected so a host built against a different schema fails loudly instead of
// silently dropping data (ADR 0026, decision 4).
func ReadRequest(r io.Reader) (*updaterv2.UpdaterRequest, error) {
	raw, err := io.ReadAll(io.LimitReader(r, MaxRequestBytes+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > MaxRequestBytes {
		return nil, errRequestTooLarge
	}
	req := &updaterv2.UpdaterRequest{}
	if err := (protojson.UnmarshalOptions{DiscardUnknown: false}).Unmarshal(raw, req); err != nil {
		return nil, err
	}
	return req, nil
}

type requestError string

func (e requestError) Error() string { return string(e) }

const errRequestTooLarge = requestError("request exceeds 32 MiB")

// RejectRequest is what the engine says when stdin did not decode: the public
// capabilities line followed by PROTOCOL_MISMATCH.
func (e *Engine) RejectRequest(cause error) error {
	if err := e.emit(&updaterv2.UpdaterEvent{
		Kind: &updaterv2.UpdaterEvent_Capabilities{Capabilities: e.capabilities(nil)},
	}); err != nil {
		return err
	}
	return e.emit(failedEvent(updaterv2.ErrorCode_ERROR_CODE_PROTOCOL_MISMATCH, false,
		"request is not a valid relkit.updater.v2.UpdaterRequest: "+cause.Error(), nil))
}

// PublicCapabilities is the handshake payload for a host that has not opened a
// profile yet (the --capabilities probe).
func (e *Engine) PublicCapabilities() *updaterv2.Capabilities {
	return e.capabilities(nil)
}

func (e *Engine) capabilities(report *updaterv2.HandshakeReport) *updaterv2.Capabilities {
	ver := e.Version
	if ver == "" {
		ver = "dev"
	}
	return &updaterv2.Capabilities{
		Protocol: Protocol,
		Operations: []updaterv2.Operation{
			updaterv2.Operation_OPERATION_CHECK,
			updaterv2.Operation_OPERATION_SKIP,
			updaterv2.Operation_OPERATION_DOWNLOAD,
			updaterv2.Operation_OPERATION_APPLY,
			updaterv2.Operation_OPERATION_STATUS,
			updaterv2.Operation_OPERATION_CLEANUP,
			updaterv2.Operation_OPERATION_LIST_INSTALLED,
			updaterv2.Operation_OPERATION_SWITCH_ACTIVE,
			updaterv2.Operation_OPERATION_ROLLBACK,
			updaterv2.Operation_OPERATION_DELETE_INSTALLED,
		},
		Placements: []updaterv2.Placement{
			updaterv2.Placement_PLACEMENT_IN_PLACE,
			updaterv2.Placement_PLACEMENT_LIBRARY,
		},
		MinCheckInterval: minCheckDurationPB(),
		PlanTtl:          planTTLDurationPB(),
		EngineVersion:    ver,
		Handshake:        report,
	}
}

// checkProtocol compares the host's declared protocol with the engine's.
// There is exactly one protocol per engine build; there is no window.
func checkProtocol(host uint32) *updaterv2.Error {
	switch {
	case host == 0:
		return newError(updaterv2.ErrorCode_ERROR_CODE_PROTOCOL_MISMATCH, false, "request.protocol required", nil)
	case host > Protocol:
		return newError(updaterv2.ErrorCode_ERROR_CODE_UPDATER_TOO_OLD, false,
			"host speaks a newer updater protocol than this engine; update the relkit-updater sidecar", nil)
	case host < Protocol:
		return newError(updaterv2.ErrorCode_ERROR_CODE_UPDATER_TOO_NEW, false,
			"host speaks an older updater protocol than this engine; regenerate host types from `relkit-updater proto`", nil)
	}
	return nil
}

// openedProfile is what the engine actually works with once the Runtime
// Bootstrap has been verified. It never travels on the wire: hosts only send
// the bootstrap verbatim and read the HandshakeReport back.
type openedProfile struct {
	Product         string
	AllowedChannels []string
	EntryURLs       []string
	TrustedKeys     sdk.TrustedKeys
	Recovery        *updaterv2.RecoveryHelp
}

// openProfile validates the Runtime Bootstrap the host forwarded verbatim and
// returns the opened profile plus the handshake report (requirement 3.8).
//
// The engine is the single parser: schema, key encoding, fingerprint, product
// consistency, channel membership and the profile signature are all checked
// here. bootstrap and expected_product are both required.
func openProfile(profile *updaterv2.ClientProfile, channel string) (*openedProfile, *updaterv2.HandshakeReport, *updaterv2.Error) {
	raw := profile.GetBootstrap()
	if raw == "" {
		return nil, nil, newError(updaterv2.ErrorCode_ERROR_CODE_PROFILE_INVALID, false, "profile.bootstrap required", nil)
	}
	expected := profile.GetExpectedProduct()
	if expected == "" {
		return nil, nil, newError(updaterv2.ErrorCode_ERROR_CODE_PROFILE_INVALID, false, "profile.expectedProduct required", nil)
	}
	parsed, perr := decodeBootstrap([]byte(raw))
	if perr != nil {
		return nil, nil, newError(updaterv2.ErrorCode_ERROR_CODE_PROFILE_INVALID, false, perr.Error(), nil)
	}
	doc := *parsed
	if err := doc.Validate(); err != nil {
		return nil, nil, newError(updaterv2.ErrorCode_ERROR_CODE_PROFILE_INVALID, false,
			"runtime bootstrap: "+err.Error(), nil)
	}
	if expected != doc.Product {
		return nil, nil, newError(updaterv2.ErrorCode_ERROR_CODE_PROFILE_INVALID, false,
			"bootstrap belongs to product "+doc.Product+", host expected "+expected, nil)
	}
	if err := channelAllowed(doc.Profile.AllowedChannels, channel); err != nil {
		return nil, nil, err
	}
	pub, err := doc.PublicKey()
	if err != nil {
		return nil, nil, newError(updaterv2.ErrorCode_ERROR_CODE_PROFILE_INVALID, false, err.Error(), nil)
	}
	opened := &openedProfile{
		Product:         doc.Product,
		AllowedChannels: append([]string(nil), doc.Profile.AllowedChannels...),
		EntryURLs:       append([]string(nil), doc.Profile.DirectoryBootstrap.EntryURLs...),
		TrustedKeys:     sdk.TrustedKeys{doc.RootKeyID: pub},
		Recovery:        profile.GetRecovery(),
	}
	report := &updaterv2.HandshakeReport{
		Product:         doc.Product,
		AllowedChannels: append([]string(nil), doc.Profile.AllowedChannels...),
		DefaultChannel:  doc.Profile.AllowedChannels[0],
		RootKeyId:       doc.RootKeyID,
		RootFingerprint: doc.RootFingerprint,
	}
	return opened, report, nil
}

func channelAllowed(allowed []string, channel string) *updaterv2.Error {
	if len(allowed) == 0 || channel == "" {
		return nil
	}
	for _, c := range allowed {
		if c == channel {
			return nil
		}
	}
	return newError(updaterv2.ErrorCode_ERROR_CODE_CHANNEL_NOT_ALLOWED, false,
		"channel "+channel+" is not in allowedChannels", nil)
}

func validateRuntime(profile *openedProfile, runtime *updaterv2.Runtime) *updaterv2.Error {
	if runtime == nil {
		return newError(updaterv2.ErrorCode_ERROR_CODE_PROFILE_INVALID, false, "runtime required", nil)
	}
	if len(profile.EntryURLs) == 0 {
		return newError(updaterv2.ErrorCode_ERROR_CODE_PROFILE_INVALID, false, "bootstrap carries no entryUrls", nil)
	}
	if runtime.CurrentCode < 0 {
		return newError(updaterv2.ErrorCode_ERROR_CODE_PROFILE_INVALID, false, "currentCode must not be negative", nil)
	}
	if runtime.Channel == "" {
		return newError(updaterv2.ErrorCode_ERROR_CODE_PROFILE_INVALID, false, "channel required", nil)
	}
	if runtime.DataDir == "" {
		return newError(updaterv2.ErrorCode_ERROR_CODE_PROFILE_INVALID, false, "dataDir required", nil)
	}
	if inst := runtime.Install; inst != nil {
		if inst.Placement == updaterv2.Placement_PLACEMENT_UNSPECIFIED {
			return newError(updaterv2.ErrorCode_ERROR_CODE_LAYOUT_UNSUPPORTED, false, "placement required", nil)
		}
	}
	return nil
}

// HandleRequest processes one UpdaterRequest and writes events to stdout.
func (e *Engine) HandleRequest(ctx context.Context, req *updaterv2.UpdaterRequest) error {
	// The protocol check comes first so an incompatible host gets one clear
	// error instead of a partially useful capabilities line.
	if perr := checkProtocol(req.GetProtocol()); perr != nil {
		if err := e.emit(&updaterv2.UpdaterEvent{
			Kind: &updaterv2.UpdaterEvent_Capabilities{Capabilities: e.capabilities(nil)},
		}); err != nil {
			return err
		}
		return e.emit(failedEvent(perr.Code, perr.Retryable, perr.Message, nil))
	}

	profile, report, berr := openProfile(req.GetProfile(), req.GetRuntime().GetChannel())
	if berr != nil {
		if err := e.emit(&updaterv2.UpdaterEvent{
			Kind: &updaterv2.UpdaterEvent_Capabilities{Capabilities: e.capabilities(nil)},
		}); err != nil {
			return err
		}
		return e.emit(failedEvent(berr.Code, berr.Retryable, berr.Message, nil))
	}
	if err := e.emit(&updaterv2.UpdaterEvent{
		Kind: &updaterv2.UpdaterEvent_Capabilities{Capabilities: e.capabilities(report)},
	}); err != nil {
		return err
	}
	if verr := validateRuntime(profile, req.GetRuntime()); verr != nil {
		if !opToleratesIncompleteRuntime(req) || req.GetRuntime().GetDataDir() == "" {
			return e.emit(failedEvent(verr.Code, verr.Retryable, verr.Message, nil))
		}
	}

	st := store{
		dataDir: req.GetRuntime().GetDataDir(),
		product: profile.Product,
		channel: req.GetRuntime().GetChannel(),
	}
	switch op := req.GetOp().(type) {
	case *updaterv2.UpdaterRequest_Check:
		return e.handleCheck(ctx, req, profile, op.Check, st)
	case *updaterv2.UpdaterRequest_Skip:
		return e.handleSkip(ctx, req, op.Skip, st)
	case *updaterv2.UpdaterRequest_Download:
		return e.handleDownload(ctx, req, op.Download, st)
	case *updaterv2.UpdaterRequest_Apply:
		return e.handleApply(ctx, req, op.Apply, st)
	case *updaterv2.UpdaterRequest_Status:
		return e.handleStatus(req, st)
	case *updaterv2.UpdaterRequest_Cleanup:
		return e.handleCleanup(req, st)
	case *updaterv2.UpdaterRequest_ListInstalled:
		return e.handleListInstalled(req)
	case *updaterv2.UpdaterRequest_SwitchActive:
		return e.handleSwitchActive(req, op.SwitchActive)
	case *updaterv2.UpdaterRequest_Rollback:
		return e.handleRollback(req)
	case *updaterv2.UpdaterRequest_DeleteInstalled:
		return e.handleDeleteInstalled(req, op.DeleteInstalled)
	default:
		return e.emit(failedEvent(updaterv2.ErrorCode_ERROR_CODE_PROFILE_INVALID, false, "missing operation", nil))
	}
}

// opToleratesIncompleteRuntime reports whether an op can run with only a
// dataDir, which is what a host needs after a crash or when it just wants the
// installed inventory.
func opToleratesIncompleteRuntime(req *updaterv2.UpdaterRequest) bool {
	switch req.GetOp().(type) {
	case *updaterv2.UpdaterRequest_Status,
		*updaterv2.UpdaterRequest_Cleanup,
		*updaterv2.UpdaterRequest_ListInstalled:
		return true
	}
	return false
}

func (e *Engine) handleStatus(req *updaterv2.UpdaterRequest, st store) error {
	state, err := st.loadState()
	if err != nil {
		return e.emit(failedEvent(updaterv2.ErrorCode_ERROR_CODE_DISK, false, err.Error(), nil))
	}
	snap := &updaterv2.StatusSnapshot{
		LastResult:       state.LastResult,
		LastSeenSequence: state.LastSeenSequence,
		SkippedCodes:     state.SkippedCodes,
		LastCheckAt:      state.LastCheckAt,
		Sidecar: &updaterv2.SidecarInfo{
			Protocol: Protocol,
			Version:  e.Version,
		},
	}
	ok, next := shouldCheck(lastCheckTime(state), state.LastResult, nil, false, timestamppb.Now().AsTime())
	if !ok {
		snap.NextAllowedAt = timestamppb.New(next)
	}
	if sess, _ := st.latestSession(); sess != nil {
		snap.ActiveSession = &updaterv2.SessionView{
			SessionId: sess.SessionId,
			PlanId:    sess.PlanId,
			Phase:     sess.Phase,
			StartedAt: sess.StartedAt,
			Error:     sess.Error,
		}
		snap.HostMustExit = hostMustExit(sess, time.Now())
	}
	_ = req
	return e.emit(&updaterv2.UpdaterEvent{Kind: &updaterv2.UpdaterEvent_Status{Status: snap}})
}

// workerGrace covers the gap between handleApply returning and the worker
// taking over the apply lock, and any heartbeat the worker just wrote.
const workerGrace = 15 * time.Second

// hostMustExit is the startup handoff rule (StatusSnapshot.host_must_exit).
// The engine owns it so hosts never encode phase semantics, and it fails open:
// a worker that is gone never keeps a host from starting.
func hostMustExit(sess *enginestate.ApplySessionRecord, now time.Time) bool {
	if sess == nil || !sess.GetRequiresHostExit() {
		return false
	}
	switch sess.GetPhase() {
	case updaterv2.SessionPhase_SESSION_PHASE_WAITING_FOR_EXIT,
		updaterv2.SessionPhase_SESSION_PHASE_DOWNLOADING,
		updaterv2.SessionPhase_SESSION_PHASE_COPYING,
		updaterv2.SessionPhase_SESSION_PHASE_COMMITTING:
	default:
		return false
	}
	if hb := sess.GetHeartbeatAt(); hb != nil && now.Sub(hb.AsTime()) < workerGrace {
		return true
	}
	raw, err := os.ReadFile(sessionLockPath(sess))
	if err != nil {
		return false
	}
	var lock applyLockFile
	if json.Unmarshal(raw, &lock) != nil {
		return false
	}
	return lock.SessionID == sess.GetSessionId() && lock.Pid > 0 && processAlive(lock.Pid)
}

func (e *Engine) handleCleanup(req *updaterv2.UpdaterRequest, st store) error {
	sess, err := st.latestSession()
	if err != nil {
		return e.emit(failedEvent(updaterv2.ErrorCode_ERROR_CODE_DISK, false, err.Error(), nil))
	}
	if sess != nil {
		switch sess.Phase {
		case updaterv2.SessionPhase_SESSION_PHASE_COMPLETED, updaterv2.SessionPhase_SESSION_PHASE_ROLLED_BACK:
			if sess.StagedRoot != "" {
				_ = os.RemoveAll(sess.StagedRoot)
			}
		}
	}
	_ = req
	return e.emit(&updaterv2.UpdaterEvent{
		Kind: &updaterv2.UpdaterEvent_Result{Result: &updaterv2.Result{Kind: &updaterv2.Result_Ok{Ok: &updaterv2.Ok{}}}},
	})
}

func recoveryFrom(profile *openedProfile) *updaterv2.RecoveryHelp {
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
