package updater

import (
	"context"
	"os"
	"path/filepath"
	"time"

	rupv2 "github.com/shichao402/relkit/api/rup/v2"
	enginestate "github.com/shichao402/relkit/api/updater/enginestate/v1"
	updaterv2 "github.com/shichao402/relkit/api/updater/v2"
	"github.com/shichao402/relkit/internal/sdk"
)

func (e *Engine) handleDownload(ctx context.Context, req *updaterv2.UpdaterRequest, op *updaterv2.DownloadOp, st store) error {
	if op == nil || op.PlanId == "" {
		return e.emit(&updaterv2.UpdaterEvent{
			Kind: &updaterv2.UpdaterEvent_Download{Download: &updaterv2.DownloadResult{
				Kind: &updaterv2.DownloadResult_Failed{Failed: &updaterv2.Failed{
					Error: newError(updaterv2.ErrorCode_ERROR_CODE_PLAN_UNKNOWN, false, "planId required", nil),
				}},
			}},
		})
	}
	plan, err := st.loadPlan(op.PlanId)
	if err != nil {
		return e.emit(&updaterv2.UpdaterEvent{
			Kind: &updaterv2.UpdaterEvent_Download{Download: &updaterv2.DownloadResult{
				Kind: &updaterv2.DownloadResult_Failed{Failed: &updaterv2.Failed{
					Error: newError(updaterv2.ErrorCode_ERROR_CODE_PLAN_UNKNOWN, false, "unknown planId", nil),
				}},
			}},
		})
	}
	key, err := st.planKey()
	if err != nil || !verifyPlan(key, plan) {
		return e.emit(&updaterv2.UpdaterEvent{
			Kind: &updaterv2.UpdaterEvent_Download{Download: &updaterv2.DownloadResult{
				Kind: &updaterv2.DownloadResult_Failed{Failed: &updaterv2.Failed{
					Error: newError(updaterv2.ErrorCode_ERROR_CODE_PLAN_TAMPERED, false, "plan hmac mismatch", nil),
				}},
			}},
		})
	}
	if plan.ExpiresAt != nil && time.Now().After(plan.ExpiresAt.AsTime()) {
		return e.emit(&updaterv2.UpdaterEvent{
			Kind: &updaterv2.UpdaterEvent_Download{Download: &updaterv2.DownloadResult{
				Kind: &updaterv2.DownloadResult_Failed{Failed: &updaterv2.Failed{
					Error: newError(updaterv2.ErrorCode_ERROR_CODE_PLAN_EXPIRED, false, "plan expired", nil),
				}},
			}},
		})
	}
	destDir := filepath.Join(req.GetRuntime().GetDataDir(), "staging", plan.PlanId)
	total, derr := e.downloadPlanFiles(ctx, st, plan, destDir, func(_ int, p sdk.Progress) {
		_ = e.emit(&updaterv2.UpdaterEvent{
			Kind: &updaterv2.UpdaterEvent_Progress{Progress: &updaterv2.Progress{
				BytesReceived:  p.Received,
				BytesTotal:     p.Total,
				BytesPerSecond: p.BytesPerSecond,
			}},
		})
	})
	if derr != nil {
		return e.emit(&updaterv2.UpdaterEvent{
			Kind: &updaterv2.UpdaterEvent_Download{Download: &updaterv2.DownloadResult{
				Kind: &updaterv2.DownloadResult_Failed{Failed: &updaterv2.Failed{Error: derr}},
			}},
		})
	}
	return e.emit(&updaterv2.UpdaterEvent{
		Kind: &updaterv2.UpdaterEvent_Download{Download: &updaterv2.DownloadResult{
			Kind: &updaterv2.DownloadResult_Downloaded{Downloaded: &updaterv2.Downloaded{
				PlanId: plan.PlanId,
				Bytes:  total,
			}},
		}},
	})
}

// downloadPlanFiles fetches and verifies every file of plan into destDir,
// records the local paths and re-seals the plan. It is shared by the download
// op and by the apply worker, which downloads a plan that apply accepted
// before it was fetched (ADR 0026 §6). Files already downloaded and still the
// right size are skipped. onProgress receives per-file progress tagged with
// the file index.
func (e *Engine) downloadPlanFiles(
	ctx context.Context,
	st store,
	plan *enginestate.UpdatePlan,
	destDir string,
	onProgress func(int, sdk.Progress),
) (int64, *updaterv2.Error) {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return 0, newError(updaterv2.ErrorCode_ERROR_CODE_DISK, false, err.Error(), nil)
	}
	var total int64
	for i, f := range plan.Files {
		if err := ValidateArtifactFilename(f.Name); err != nil {
			return total, newError(updaterv2.ErrorCode_ERROR_CODE_SELECTOR_NO_MATCH, false, err.Error(), nil)
		}
		if planFileReady(f) {
			total += f.Size
			continue
		}
		dest := filepath.Join(destDir, filepath.Base(f.Name))
		art := &rupv2.Artifact{
			Filename: f.Name,
			Size:     f.Size,
			Sha256:   f.Sha256Hex,
			Urls:     f.Urls,
		}
		verified, err := sdk.DownloadArtifact(ctx, e.fetcher(), art, dest, sdk.DefaultPolicy(), func(p sdk.Progress) {
			if onProgress != nil {
				onProgress(i, p)
			}
		})
		if err != nil {
			if ctx.Err() != nil {
				return total, newError(updaterv2.ErrorCode_ERROR_CODE_CANCELED, true, "download cancelled", nil)
			}
			return total, newError(updaterv2.ErrorCode_ERROR_CODE_NETWORK, true, err.Error(), nil)
		}
		f.LocalPath = verified.Path
		f.Downloaded = true
		total += f.Size
	}
	key, err := st.planKey()
	if err != nil {
		return total, newError(updaterv2.ErrorCode_ERROR_CODE_DISK, false, err.Error(), nil)
	}
	// Re-seal after filling local paths.
	plan.PlanHmac = hmacPlan(key, plan)
	if err := st.savePlan(plan); err != nil {
		return total, newError(updaterv2.ErrorCode_ERROR_CODE_DISK, false, err.Error(), nil)
	}
	return total, nil
}

// planFileReady reports whether a plan file is on disk at its declared size.
func planFileReady(f *enginestate.PlannedFile) bool {
	if f == nil || !f.Downloaded || f.LocalPath == "" {
		return false
	}
	fi, err := os.Stat(f.LocalPath)
	return err == nil && fi.Size() == f.Size
}

// planDownloaded reports whether every file of the plan is ready.
func planDownloaded(plan *enginestate.UpdatePlan) bool {
	for _, f := range plan.GetFiles() {
		if !planFileReady(f) {
			return false
		}
	}
	return true
}
