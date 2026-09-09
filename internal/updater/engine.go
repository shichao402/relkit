package updater

import (
	"context"
	"crypto/ed25519"
	"io"
	"os"

	updaterv1 "go.firoyang.com/relkit/api/updater/v1"
	"go.firoyang.com/relkit/sdk"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Engine is the unique updater implementation.
type Engine struct {
	Version string
	Fetcher sdk.Fetcher
	Now     func() timestamppb.Timestamp
	Stdout  io.Writer
}

func (e *Engine) writer() io.Writer {
	if e.Stdout != nil {
		return e.Stdout
	}
	return os.Stdout
}

func (e *Engine) emit(ev *updaterv1.UpdaterEvent) error {
	return WriteFrame(e.writer(), ev)
}

// PublicCapabilities is the handshake payload.
func (e *Engine) PublicCapabilities() *updaterv1.Capabilities {
	return e.capabilities()
}

func (e *Engine) capabilities() *updaterv1.Capabilities {
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
			updaterv1.Operation_OPERATION_SCHEDULER,
		},
		Layouts: []updaterv1.Layout{
			updaterv1.Layout_LAYOUT_WHOLE_ROOT,
			updaterv1.Layout_LAYOUT_VERSIONED_DIR,
			updaterv1.Layout_LAYOUT_FILE_SET,
		},
		MinCheckInterval: minCheckDurationPB(),
		PlanTtl:          planTTLDurationPB(),
		EngineVersion:    ver,
	}
}

func negotiate(hello *updaterv1.ClientHello) *updaterv1.Error {
	if hello == nil || hello.IpcMin == 0 && hello.IpcMax == 0 {
		return newError(updaterv1.ErrorCode_ERROR_CODE_PROTOCOL_MISMATCH, false, "missing ClientHello", nil)
	}
	if hello.IpcMax < IPCMin {
		return newError(updaterv1.ErrorCode_ERROR_CODE_UPDATER_TOO_NEW, false, "host IPC window is older than engine", nil)
	}
	if hello.IpcMin > IPCMax {
		return newError(updaterv1.ErrorCode_ERROR_CODE_UPDATER_TOO_OLD, false, "host IPC window is newer than engine", nil)
	}
	return nil
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
	if runtime.CurrentCode == 0 {
		return newError(updaterv1.ErrorCode_ERROR_CODE_PROFILE_INVALID, false, "currentCode must not be 0", nil)
	}
	if runtime.Channel == "" {
		return newError(updaterv1.ErrorCode_ERROR_CODE_PROFILE_INVALID, false, "channel required", nil)
	}
	allowed := false
	if len(profile.AllowedChannels) == 0 {
		allowed = true
	}
	for _, c := range profile.AllowedChannels {
		if c == runtime.Channel {
			allowed = true
			break
		}
	}
	if !allowed {
		return newError(updaterv1.ErrorCode_ERROR_CODE_CHANNEL_NOT_ALLOWED, false, "channel not in allowedChannels", nil)
	}
	if runtime.DataDir == "" {
		return newError(updaterv1.ErrorCode_ERROR_CODE_PROFILE_INVALID, false, "dataDir required", nil)
	}
	if inst := runtime.Install; inst != nil {
		if inst.Layout == updaterv1.Layout_LAYOUT_UNSPECIFIED {
			return newError(updaterv1.ErrorCode_ERROR_CODE_LAYOUT_UNSUPPORTED, false, "layout required", nil)
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
	if err := e.emit(&updaterv1.UpdaterEvent{
		Kind: &updaterv1.UpdaterEvent_Capabilities{Capabilities: e.capabilities()},
	}); err != nil {
		return err
	}
	if nego := negotiate(req.GetHello()); nego != nil {
		return e.emit(failedEvent(nego.Code, nego.Retryable, nego.Message, nil))
	}
	if verr := validateOpen(req.GetProfile(), req.GetRuntime()); verr != nil && req.GetStatus() == nil {
		// status after crash still needs dataDir; allow status with runtime only
		if req.GetOp() == nil {
			return e.emit(failedEvent(verr.Code, verr.Retryable, verr.Message, nil))
		}
		switch req.GetOp().(type) {
		case *updaterv1.UpdaterRequest_Status, *updaterv1.UpdaterRequest_Cleanup:
			if req.GetRuntime() == nil || req.GetRuntime().DataDir == "" {
				return e.emit(failedEvent(verr.Code, verr.Retryable, verr.Message, nil))
			}
		default:
			return e.emit(failedEvent(verr.Code, verr.Retryable, verr.Message, nil))
		}
	}

	st := store{dataDir: req.GetRuntime().GetDataDir()}
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
	default:
		return e.emit(failedEvent(updaterv1.ErrorCode_ERROR_CODE_PROFILE_INVALID, false, "missing operation", nil))
	}
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

func layoutRequiresHostExit(layout updaterv1.Layout) bool {
	return layout == updaterv1.Layout_LAYOUT_WHOLE_ROOT || layout == updaterv1.Layout_LAYOUT_VERSIONED_DIR
}

func (e *Engine) fetcher() sdk.Fetcher {
	if e.Fetcher != nil {
		return e.Fetcher
	}
	return &sdk.HTTPFetcher{}
}
