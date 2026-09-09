package updater

import (
	"context"
	"encoding/hex"
	"os"
	"path/filepath"
	"time"

	rupv2 "go.firoyang.com/relkit/api/rup/v2"
	updaterv1 "go.firoyang.com/relkit/api/updater/v1"
	"go.firoyang.com/relkit/sdk"
)

func (e *Engine) handleDownload(ctx context.Context, req *updaterv1.UpdaterRequest, op *updaterv1.DownloadOp, st store) error {
	if op == nil || op.PlanId == "" {
		return e.emit(&updaterv1.UpdaterEvent{
			Kind: &updaterv1.UpdaterEvent_Download{Download: &updaterv1.DownloadResult{
				Kind: &updaterv1.DownloadResult_Failed{Failed: &updaterv1.Failed{
					Error: newError(updaterv1.ErrorCode_ERROR_CODE_PLAN_UNKNOWN, false, "planId required", nil),
				}},
			}},
		})
	}
	plan, err := st.loadPlan(op.PlanId)
	if err != nil {
		return e.emit(&updaterv1.UpdaterEvent{
			Kind: &updaterv1.UpdaterEvent_Download{Download: &updaterv1.DownloadResult{
				Kind: &updaterv1.DownloadResult_Failed{Failed: &updaterv1.Failed{
					Error: newError(updaterv1.ErrorCode_ERROR_CODE_PLAN_UNKNOWN, false, "unknown planId", nil),
				}},
			}},
		})
	}
	key, err := st.planKey()
	if err != nil || !verifyPlan(key, plan) {
		return e.emit(&updaterv1.UpdaterEvent{
			Kind: &updaterv1.UpdaterEvent_Download{Download: &updaterv1.DownloadResult{
				Kind: &updaterv1.DownloadResult_Failed{Failed: &updaterv1.Failed{
					Error: newError(updaterv1.ErrorCode_ERROR_CODE_PLAN_TAMPERED, false, "plan hmac mismatch", nil),
				}},
			}},
		})
	}
	if plan.ExpiresAt != nil && time.Now().After(plan.ExpiresAt.AsTime()) {
		return e.emit(&updaterv1.UpdaterEvent{
			Kind: &updaterv1.UpdaterEvent_Download{Download: &updaterv1.DownloadResult{
				Kind: &updaterv1.DownloadResult_Failed{Failed: &updaterv1.Failed{
					Error: newError(updaterv1.ErrorCode_ERROR_CODE_PLAN_EXPIRED, false, "plan expired", nil),
				}},
			}},
		})
	}
	destDir := filepath.Join(req.GetRuntime().GetDataDir(), "staging", plan.PlanId)
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return e.emit(&updaterv1.UpdaterEvent{
			Kind: &updaterv1.UpdaterEvent_Download{Download: &updaterv1.DownloadResult{
				Kind: &updaterv1.DownloadResult_Failed{Failed: &updaterv1.Failed{
					Error: newError(updaterv1.ErrorCode_ERROR_CODE_DISK, false, err.Error(), nil),
				}},
			}},
		})
	}
	var total int64
	for _, f := range plan.Files {
		if err := ValidateArtifactFilename(f.Name); err != nil {
			return e.emit(&updaterv1.UpdaterEvent{
				Kind: &updaterv1.UpdaterEvent_Download{Download: &updaterv1.DownloadResult{
					Kind: &updaterv1.DownloadResult_Failed{Failed: &updaterv1.Failed{
						Error: newError(updaterv1.ErrorCode_ERROR_CODE_SELECTOR_NO_MATCH, false, err.Error(), nil),
					}},
				}},
			})
		}
		dest := filepath.Join(destDir, filepath.Base(f.Name))
		art := &rupv2.Artifact{
			Filename: f.Name,
			Size:     f.Size,
			Sha256:   f.Sha256Hex,
			Urls:     f.Urls,
		}
		verified, err := sdk.DownloadArtifact(ctx, e.fetcher(), art, dest, sdk.DefaultPolicy(), func(p sdk.Progress) {
			_ = e.emit(&updaterv1.UpdaterEvent{
				Kind: &updaterv1.UpdaterEvent_Progress{Progress: &updaterv1.Progress{
					BytesReceived:  p.Received,
					BytesTotal:     p.Total,
					BytesPerSecond: p.BytesPerSecond,
				}},
			})
		})
		if err != nil {
			return e.emit(&updaterv1.UpdaterEvent{
				Kind: &updaterv1.UpdaterEvent_Download{Download: &updaterv1.DownloadResult{
					Kind: &updaterv1.DownloadResult_Failed{Failed: &updaterv1.Failed{
						Error: newError(updaterv1.ErrorCode_ERROR_CODE_NETWORK, true, err.Error(), nil),
					}},
				}},
			})
		}
		f.LocalPath = verified.Path
		f.Downloaded = true
		total += f.Size
		_ = hex.EncodeToString
	}
	if err := st.savePlan(plan); err != nil {
		return e.emit(&updaterv1.UpdaterEvent{
			Kind: &updaterv1.UpdaterEvent_Download{Download: &updaterv1.DownloadResult{
				Kind: &updaterv1.DownloadResult_Failed{Failed: &updaterv1.Failed{
					Error: newError(updaterv1.ErrorCode_ERROR_CODE_DISK, false, err.Error(), nil),
				}},
			}},
		})
	}
	// Re-HMAC after filling local paths.
	key, _ = st.planKey()
	plan.PlanHmac = hmacPlan(key, plan)
	_ = st.savePlan(plan)
	return e.emit(&updaterv1.UpdaterEvent{
		Kind: &updaterv1.UpdaterEvent_Download{Download: &updaterv1.DownloadResult{
			Kind: &updaterv1.DownloadResult_Downloaded{Downloaded: &updaterv1.Downloaded{
				PlanId: plan.PlanId,
				Bytes:  total,
			}},
		}},
	})
}
