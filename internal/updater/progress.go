package updater

import (
	"fmt"
	"os"
	"strings"

	enginestate "github.com/shichao402/relkit/api/updater/enginestate/v1"
	updaterv2 "github.com/shichao402/relkit/api/updater/v2"
)

// progressUI is the window the apply worker shows while the host is gone
// (ADR 0026 §6). Every method must be safe to call from the worker goroutine;
// implementations marshal onto their own UI thread.
type progressUI interface {
	// SetPhase switches the headline. cancelable decides whether the button
	// reads "Cancel" (and fires Cancelled) or is disabled.
	SetPhase(phase updaterv2.SessionPhase, cancelable bool)
	// SetDownload updates the progress bar during SESSION_PHASE_DOWNLOADING.
	SetDownload(received, total, bytesPerSecond int64)
	// Fail shows the error with a Close button and blocks until the user
	// dismisses it.
	Fail(err *updaterv2.Error)
	// Close removes the window. Safe to call more than once.
	Close()
	// Cancelled fires once when the user cancels a cancelable phase.
	Cancelled() <-chan struct{}
}

// progressSpec is what a window needs to render its text.
type progressSpec struct {
	Product string
	Version string
	Lang    string // "zh" or "en"
}

func progressSpecFor(sess *enginestate.ApplySessionRecord) progressSpec {
	name := strings.TrimSpace(sess.GetDisplayName())
	if name == "" {
		name = "the application"
	}
	return progressSpec{Product: name, Version: sess.GetTargetVersion(), Lang: uiLanguage()}
}

// openProgressUI picks the window for a session. The window only covers the
// stretch where the host is gone; a session that does not need the host to
// exit, or one whose host asked to draw its own UI, gets the no-op.
func (e *Engine) openProgressUI(sess *enginestate.ApplySessionRecord) progressUI {
	if sess.GetHideProgress() || !sess.GetRequiresHostExit() {
		return noopProgress{}
	}
	spec := progressSpecFor(sess)
	if e.newProgressUI != nil {
		return e.newProgressUI(spec)
	}
	ui, err := platformProgressUI(spec)
	if err != nil || ui == nil {
		return noopProgress{}
	}
	return ui
}

type noopProgress struct{}

func (noopProgress) SetPhase(updaterv2.SessionPhase, bool) {}
func (noopProgress) SetDownload(int64, int64, int64)       {}
func (noopProgress) Fail(*updaterv2.Error)                 {}
func (noopProgress) Close()                                {}
func (noopProgress) Cancelled() <-chan struct{}            { return nil }

// langFromLocale maps a POSIX/BCP-47 locale string to the two languages the
// window ships.
func langFromLocale(locale string) string {
	l := strings.ToLower(strings.TrimSpace(locale))
	if strings.HasPrefix(l, "zh") {
		return "zh"
	}
	return "en"
}

func envLocale() string {
	for _, k := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}

// progressText renders every string the window shows, so the copy is testable
// on any OS.
type progressText struct{ spec progressSpec }

func (t progressText) zh() bool { return t.spec.Lang == "zh" }

func (t progressText) Title() string {
	if t.zh() {
		return t.spec.Product + " 更新"
	}
	return "Updating " + t.spec.Product
}

func (t progressText) Phase(p updaterv2.SessionPhase) string {
	zh := t.zh()
	switch p {
	case updaterv2.SessionPhase_SESSION_PHASE_WAITING_FOR_EXIT:
		if zh {
			return "正在等待 " + t.spec.Product + " 退出…"
		}
		return "Waiting for " + t.spec.Product + " to exit…"
	case updaterv2.SessionPhase_SESSION_PHASE_DOWNLOADING:
		if zh {
			return "正在下载 " + t.version() + "…"
		}
		return "Downloading " + t.version() + "…"
	case updaterv2.SessionPhase_SESSION_PHASE_COPYING, updaterv2.SessionPhase_SESSION_PHASE_COMMITTING:
		if zh {
			return "正在安装 " + t.version() + "，请勿关闭电脑…"
		}
		return "Installing " + t.version() + ". Please keep the computer on…"
	case updaterv2.SessionPhase_SESSION_PHASE_RELAUNCHING:
		if zh {
			return "正在启动 " + t.spec.Product + "…"
		}
		return "Starting " + t.spec.Product + "…"
	case updaterv2.SessionPhase_SESSION_PHASE_COMPLETED:
		if zh {
			return "更新完成"
		}
		return "Update complete"
	}
	if zh {
		return "正在更新…"
	}
	return "Updating…"
}

func (t progressText) version() string {
	if t.spec.Version == "" {
		if t.zh() {
			return "新版本"
		}
		return "the new version"
	}
	return t.spec.Version
}

func (t progressText) Download(received, total, bps int64) string {
	s := humanBytes(received)
	if total > 0 {
		s += " / " + humanBytes(total)
	}
	if bps > 0 {
		s += "  ·  " + humanBytes(bps) + "/s"
	}
	return s
}

func (t progressText) FailureHeadline() string {
	if t.zh() {
		return "更新未完成"
	}
	return "The update did not finish"
}

func (t progressText) FailureDetail(err *updaterv2.Error) string {
	var parts []string
	if msg := strings.TrimSpace(err.GetMessage()); msg != "" {
		parts = append(parts, msg)
	}
	if m := strings.TrimSpace(err.GetRecovery().GetMessage()); m != "" {
		parts = append(parts, m)
	}
	return strings.Join(parts, "\r\n\r\n")
}

// HelpURL is the first recovery link, opened by the Help button.
func (t progressText) HelpURL(err *updaterv2.Error) (label, url string) {
	for _, l := range err.GetRecovery().GetLinks() {
		if u := strings.TrimSpace(l.GetUrl()); u != "" {
			label = strings.TrimSpace(l.GetLabel())
			if label == "" {
				if t.zh() {
					label = "帮助"
				} else {
					label = "Help"
				}
			}
			return label, u
		}
	}
	return "", ""
}

func (t progressText) CancelButton() string {
	if t.zh() {
		return "取消"
	}
	return "Cancel"
}

func (t progressText) CloseButton() string {
	if t.zh() {
		return "关闭"
	}
	return "Close"
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}
