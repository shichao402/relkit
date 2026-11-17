//go:build windows

package updater

import (
	"errors"
	"path/filepath"
	"runtime"
	"sync"
	"time"
	"unsafe"

	updaterv2 "github.com/shichao402/relkit/api/updater/v2"
	"golang.org/x/sys/windows"
)

// Win32 progress window for the apply worker (ADR 0026 §6). Plain user32 and
// comctl32 calls through x/sys/windows: no cgo, cross-compiles from any host.
//
// The binary carries no manifest, so the window opts into the modern look at
// run time: a comctl32 v6 activation context (themed buttons, marquee bar), a
// system-DPI-aware UI thread (crisp text instead of a stretched bitmap), and
// the system message font (Segoe UI / Microsoft YaHei UI) scaled to that DPI.

var (
	modUser32   = windows.NewLazySystemDLL("user32.dll")
	modKernel32 = windows.NewLazySystemDLL("kernel32.dll")
	modComctl32 = windows.NewLazySystemDLL("comctl32.dll")
	modGdi32    = windows.NewLazySystemDLL("gdi32.dll")
	modShell32  = windows.NewLazySystemDLL("shell32.dll")

	procRegisterClassExW      = modUser32.NewProc("RegisterClassExW")
	procCreateWindowExW       = modUser32.NewProc("CreateWindowExW")
	procDefWindowProcW        = modUser32.NewProc("DefWindowProcW")
	procGetMessageW           = modUser32.NewProc("GetMessageW")
	procTranslateMessage      = modUser32.NewProc("TranslateMessage")
	procDispatchMessageW      = modUser32.NewProc("DispatchMessageW")
	procPostMessageW          = modUser32.NewProc("PostMessageW")
	procSendMessageW          = modUser32.NewProc("SendMessageW")
	procSetWindowTextW        = modUser32.NewProc("SetWindowTextW")
	procShowWindow            = modUser32.NewProc("ShowWindow")
	procUpdateWindow          = modUser32.NewProc("UpdateWindow")
	procDestroyWindow         = modUser32.NewProc("DestroyWindow")
	procPostQuitMessage       = modUser32.NewProc("PostQuitMessage")
	procEnableWindow          = modUser32.NewProc("EnableWindow")
	procMoveWindow            = modUser32.NewProc("MoveWindow")
	procSetWindowPos          = modUser32.NewProc("SetWindowPos")
	procGetSystemMetrics      = modUser32.NewProc("GetSystemMetrics")
	procLoadCursorW           = modUser32.NewProc("LoadCursorW")
	procSetForegroundWin      = modUser32.NewProc("SetForegroundWindow")
	procGetSysColor           = modUser32.NewProc("GetSysColor")
	procGetSysColorBrush      = modUser32.NewProc("GetSysColorBrush")
	procSystemParametersInfoW = modUser32.NewProc("SystemParametersInfoW")
	procAdjustWindowRectEx    = modUser32.NewProc("AdjustWindowRectEx")
	// Windows 10 1607+; each is probed with Find before use.
	procSetThreadDpiAwareness   = modUser32.NewProc("SetThreadDpiAwarenessContext")
	procGetDpiForSystem         = modUser32.NewProc("GetDpiForSystem")
	procSystemParametersInfoDpi = modUser32.NewProc("SystemParametersInfoForDpi")
	procAdjustWindowRectExDpi   = modUser32.NewProc("AdjustWindowRectExForDpi")

	procGetModuleHandleW   = modKernel32.NewProc("GetModuleHandleW")
	procGetUserDefUILang   = modKernel32.NewProc("GetUserDefaultUILanguage")
	procCreateActCtxW      = modKernel32.NewProc("CreateActCtxW")
	procActivateActCtx     = modKernel32.NewProc("ActivateActCtx")
	procDeactivateActCtx   = modKernel32.NewProc("DeactivateActCtx")
	procReleaseActCtx      = modKernel32.NewProc("ReleaseActCtx")
	procInitCommonControls = modComctl32.NewProc("InitCommonControlsEx")
	procGetStockObject     = modGdi32.NewProc("GetStockObject")
	procCreateFontIndirect = modGdi32.NewProc("CreateFontIndirectW")
	procDeleteObject       = modGdi32.NewProc("DeleteObject")
	procSetBkColor         = modGdi32.NewProc("SetBkColor")
	procSetTextColor       = modGdi32.NewProc("SetTextColor")
	procShellExecuteW      = modShell32.NewProc("ShellExecuteW")
)

const (
	wsOverlapped     = 0x00000000
	wsCaption        = 0x00C00000
	wsSysMenu        = 0x00080000
	wsMinimizeBox    = 0x00020000
	wsVisible        = 0x10000000
	wsChild          = 0x40000000
	wsTabStop        = 0x00010000
	wsExDlgModalFrm  = 0x00000001
	wsExTopmost      = 0x00000008
	wsExAppWindow    = 0x00040000
	ssEndEllipsis    = 0x00004000
	ssNoPrefix       = 0x00000080
	pbsMarquee       = 0x00000008
	frameStyle       = wsOverlapped | wsCaption | wsSysMenu | wsMinimizeBox
	frameExStyle     = wsExTopmost | wsExAppWindow | wsExDlgModalFrm
	swpNoMove        = 0x0002
	swpNoZOrder      = 0x0004
	swpNoActivate    = 0x0010
	spiGetNCMetrics  = 0x0029
	actCtxResName    = 0x0008
	actCtxAsmDir     = 0x0004
	shell32Manifest  = 124         // shell32.dll's RT_MANIFEST naming comctl32 v6
	dpiSystemAware   = ^uintptr(1) // DPI_AWARENESS_CONTEXT_SYSTEM_AWARE (-2)
	baseDPI          = 96
	headlineColorRef = 0x00993300 // RGB(0x00,0x33,0x99), the task dialog heading blue

	wmDestroy        = 0x0002
	wmClose          = 0x0010
	wmSetFont        = 0x0030
	wmCommand        = 0x0111
	wmCtlColorStatic = 0x0138
	wmApp            = 0x8000
	wmUser           = 0x0400

	wmAppRefresh = wmApp + 1
	wmAppClose   = wmApp + 2

	pbmSetPos     = wmUser + 2
	pbmSetRange32 = wmUser + 6
	pbmSetMarquee = wmUser + 10

	swHide       = 0
	swShowNormal = 1
	swShow       = 5

	colorWindow     = 5
	colorWindowText = 8
	idcArrow        = 32512
	smCxScreen      = 0
	smCyScreen      = 1
	iccProgress     = 0x00000020
	defaultGUIFont  = 17
	langChinese     = 0x04
	idButton        = 101
	idHelp          = 102
	barRange        = 1000
	failDismissWait = 30 * time.Minute
)

// Layout in 96-DPI units; scaled by the system DPI at creation.
const (
	layMargin     = 20
	layWidth      = 440 // client width
	layHeadlineY  = 18
	layHeadlineH  = 28
	layBarY       = 56
	layBarH       = 14
	layDetailY    = 78
	layDetailH    = 20
	layButtonW    = 88
	layButtonH    = 28
	layButtonGap  = 8
	layBottom     = 16
	layFailDetail = 76 // detail height on the failure screen (bar hidden)
)

type wndClassEx struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   windows.Handle
	Icon       windows.Handle
	Cursor     windows.Handle
	Background windows.Handle
	MenuName   *uint16
	ClassName  *uint16
	IconSm     windows.Handle
}

type winMsg struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	PtX     int32
	PtY     int32
	Private uint32
}

type initCommonControlsEx struct {
	Size uint32
	ICC  uint32
}

type winRect struct{ Left, Top, Right, Bottom int32 }

type logFontW struct {
	Height, Width, Escapement, Orientation, Weight int32
	Italic, Underline, StrikeOut, CharSet          byte
	OutPrecision, ClipPrecision, Quality, PitchFam byte
	FaceName                                       [32]uint16
}

type nonClientMetricsW struct {
	Size                                   uint32
	BorderWidth, ScrollWidth, ScrollHeight int32
	CaptionWidth, CaptionHeight            int32
	CaptionFont                            logFontW
	SmCaptionWidth, SmCaptionHeight        int32
	SmCaptionFont                          logFontW
	MenuWidth, MenuHeight                  int32
	MenuFont, StatusFont, MessageFont      logFontW
	PaddedBorderWidth                      int32
}

type actCtxW struct {
	Size                  uint32
	Flags                 uint32
	Source                *uint16
	ProcessorArchitecture uint16
	LangID                uint16
	AssemblyDirectory     *uint16
	ResourceName          uintptr
	ApplicationName       *uint16
	Module                windows.Handle
}

func uiLanguage() string {
	id, _, _ := procGetUserDefUILang.Call()
	if id&0x3ff == langChinese {
		return "zh"
	}
	if l := envLocale(); l != "" {
		return langFromLocale(l)
	}
	return "en"
}

// winProgress owns one top-level window on a dedicated, locked OS thread.
// Worker-side methods only mutate state and post a refresh message.
type winProgress struct {
	text progressText

	mu         sync.Mutex
	headline   string
	detail     string
	showBar    bool
	busy       bool // indeterminate phase: marquee bar
	barPos     int
	cancelable bool
	cancelling bool
	failed     bool
	helpLabel  string
	helpURL    string

	// UI-thread only.
	hwnd, headlineWnd, detailWnd, barWnd, marqueeWnd, buttonWnd, helpWnd uintptr
	dpi                                                                  int
	bodyFont, headFont                                                   uintptr
	failLayout, marqueeOn                                                bool

	done       chan struct{}
	cancelled  chan struct{}
	dismissed  chan struct{}
	cancelOnce sync.Once
	dismissOne sync.Once
	closeOnce  sync.Once
}

// Only one window exists per worker process; the window procedure finds it here.
var (
	activeWinMu sync.Mutex
	activeWin   *winProgress
	wndProcPtr  = windows.NewCallback(progressWndProc)
)

func platformProgressUI(spec progressSpec) (progressUI, error) {
	w := &winProgress{
		text:      progressText{spec: spec},
		busy:      true,
		done:      make(chan struct{}),
		cancelled: make(chan struct{}),
		dismissed: make(chan struct{}),
	}
	w.headline = w.text.Phase(updaterv2.SessionPhase_SESSION_PHASE_UNSPECIFIED)
	ready := make(chan error, 1)
	go w.loop(ready)
	select {
	case err := <-ready:
		if err != nil {
			return nil, err
		}
	case <-time.After(5 * time.Second):
		return nil, errors.New("progress window did not start")
	}
	return w, nil
}

func utf16(s string) uintptr {
	p, err := windows.UTF16PtrFromString(s)
	if err != nil {
		p, _ = windows.UTF16PtrFromString("")
	}
	return uintptr(unsafe.Pointer(p))
}

// enableVisualStyles activates comctl32 v6 on this thread through the
// manifest shell32.dll ships, the usual route for a binary without one.
// The returned func undoes it; on failure the classic controls still work.
func enableVisualStyles() func() {
	if procCreateActCtxW.Find() != nil {
		return func() {}
	}
	sysDir, err := windows.GetSystemDirectory()
	if err != nil {
		return func() {}
	}
	src, _ := windows.UTF16PtrFromString(filepath.Join(sysDir, "shell32.dll"))
	dir, _ := windows.UTF16PtrFromString(sysDir)
	ctx := actCtxW{
		Flags:             actCtxResName | actCtxAsmDir,
		Source:            src,
		AssemblyDirectory: dir,
		ResourceName:      shell32Manifest,
	}
	ctx.Size = uint32(unsafe.Sizeof(ctx))
	h, _, _ := procCreateActCtxW.Call(uintptr(unsafe.Pointer(&ctx)))
	if h == 0 || h == ^uintptr(0) {
		return func() {}
	}
	var cookie uintptr
	if ok, _, _ := procActivateActCtx.Call(h, uintptr(unsafe.Pointer(&cookie))); ok == 0 {
		_, _, _ = procReleaseActCtx.Call(h)
		return func() {}
	}
	return func() {
		_, _, _ = procDeactivateActCtx.Call(0, cookie)
		_, _, _ = procReleaseActCtx.Call(h)
	}
}

// systemDPI makes this thread system-DPI aware and reports that DPI, plus a
// func restoring the thread's previous awareness. Older Windows stay at 96 and
// let the compositor scale the window.
func systemDPI() (int, func()) {
	if procSetThreadDpiAwareness.Find() != nil || procGetDpiForSystem.Find() != nil {
		return baseDPI, func() {}
	}
	prev, _, _ := procSetThreadDpiAwareness.Call(dpiSystemAware)
	if prev == 0 {
		return baseDPI, func() {}
	}
	restore := func() { _, _, _ = procSetThreadDpiAwareness.Call(prev) }
	d, _, _ := procGetDpiForSystem.Call()
	if d == 0 {
		return baseDPI, restore
	}
	return int(d), restore
}

func (w *winProgress) px(v int) int { return v * w.dpi / baseDPI }

// createFonts builds the body font from the system message font and a larger
// headline from the same face. Falls back to DEFAULT_GUI_FONT.
func (w *winProgress) createFonts() {
	var ncm nonClientMetricsW
	ncm.Size = uint32(unsafe.Sizeof(ncm))
	ok := uintptr(0)
	if procSystemParametersInfoDpi.Find() == nil {
		ok, _, _ = procSystemParametersInfoDpi.Call(spiGetNCMetrics, uintptr(ncm.Size), uintptr(unsafe.Pointer(&ncm)), 0, uintptr(w.dpi))
	}
	if ok == 0 {
		ok, _, _ = procSystemParametersInfoW.Call(spiGetNCMetrics, uintptr(ncm.Size), uintptr(unsafe.Pointer(&ncm)), 0)
	}
	if ok != 0 {
		body := ncm.MessageFont
		w.bodyFont, _, _ = procCreateFontIndirect.Call(uintptr(unsafe.Pointer(&body)))
		head := ncm.MessageFont
		head.Height = head.Height * 4 / 3
		w.headFont, _, _ = procCreateFontIndirect.Call(uintptr(unsafe.Pointer(&head)))
	}
	if w.bodyFont == 0 {
		w.bodyFont, _, _ = procGetStockObject.Call(defaultGUIFont)
	}
	if w.headFont == 0 {
		w.headFont = w.bodyFont
	}
}

func (w *winProgress) deleteFonts() {
	stock, _, _ := procGetStockObject.Call(defaultGUIFont)
	if w.headFont != 0 && w.headFont != w.bodyFont && w.headFont != stock {
		_, _, _ = procDeleteObject.Call(w.headFont)
	}
	if w.bodyFont != 0 && w.bodyFont != stock {
		_, _, _ = procDeleteObject.Call(w.bodyFont)
	}
}

// clientHeight is the client area height for the current screen, 96-DPI units.
func clientHeight(failed bool) int {
	if failed {
		return layHeadlineY + layHeadlineH + 8 + layFailDetail + 8 + layButtonH + layBottom
	}
	return layDetailY + layDetailH + 8 + layButtonH + layBottom
}

// frameSize converts a client size (device pixels) to the window size.
func (w *winProgress) frameSize(cw, ch int) (int, int) {
	r := winRect{Right: int32(cw), Bottom: int32(ch)}
	if procAdjustWindowRectExDpi.Find() == nil {
		_, _, _ = procAdjustWindowRectExDpi.Call(uintptr(unsafe.Pointer(&r)), frameStyle, 0, frameExStyle, uintptr(w.dpi))
	} else {
		_, _, _ = procAdjustWindowRectEx.Call(uintptr(unsafe.Pointer(&r)), frameStyle, 0, frameExStyle)
	}
	return int(r.Right - r.Left), int(r.Bottom - r.Top)
}

func (w *winProgress) loop(ready chan<- error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer close(w.done)

	undoStyles := enableVisualStyles()
	defer undoStyles()
	var restoreDPI func()
	w.dpi, restoreDPI = systemDPI()
	defer restoreDPI()

	icc := initCommonControlsEx{Size: uint32(unsafe.Sizeof(initCommonControlsEx{})), ICC: iccProgress}
	_, _, _ = procInitCommonControls.Call(uintptr(unsafe.Pointer(&icc)))

	inst, _, _ := procGetModuleHandleW.Call(0)
	cursor, _, _ := procLoadCursorW.Call(0, idcArrow)
	className, _ := windows.UTF16PtrFromString("RelkitUpdaterProgress")
	wc := wndClassEx{
		WndProc:    wndProcPtr,
		Instance:   windows.Handle(inst),
		Cursor:     windows.Handle(cursor),
		Background: windows.Handle(colorWindow + 1),
		ClassName:  className,
	}
	wc.Size = uint32(unsafe.Sizeof(wc))
	// A second registration in the same process fails harmlessly with
	// ERROR_CLASS_ALREADY_EXISTS; CreateWindowEx below is the real check.
	_, _, _ = procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))

	activeWinMu.Lock()
	activeWin = w
	activeWinMu.Unlock()

	w.createFonts()
	defer w.deleteFonts()

	fw, fh := w.frameSize(w.px(layWidth), w.px(clientHeight(false)))
	sw, _, _ := procGetSystemMetrics.Call(smCxScreen)
	sh, _, _ := procGetSystemMetrics.Call(smCyScreen)
	x := (int(sw) - fw) / 2
	y := (int(sh) - fh) / 2
	hwnd, _, cerr := procCreateWindowExW.Call(
		frameExStyle,
		uintptr(unsafe.Pointer(className)),
		utf16(w.text.Title()),
		frameStyle,
		uintptr(x), uintptr(y), uintptr(fw), uintptr(fh),
		0, 0, inst, 0,
	)
	if hwnd == 0 {
		ready <- cerr
		return
	}
	w.hwnd = hwnd
	child := func(class, text string, style uintptr, font uintptr, id uintptr) uintptr {
		h, _, _ := procCreateWindowExW.Call(0, utf16(class), utf16(text),
			wsChild|wsVisible|style, 0, 0, 0, 0, hwnd, id, inst, 0)
		if h != 0 && font != 0 {
			_, _, _ = procSendMessageW.Call(h, wmSetFont, font, 1)
		}
		return h
	}
	w.headlineWnd = child("STATIC", w.headline, ssNoPrefix|ssEndEllipsis, w.headFont, 0)
	w.barWnd = child("msctls_progress32", "", 0, 0, 0)
	w.marqueeWnd = child("msctls_progress32", "", pbsMarquee, 0, 0)
	w.detailWnd = child("STATIC", "", ssNoPrefix, w.bodyFont, 0)
	w.helpWnd = child("BUTTON", "", wsTabStop, w.bodyFont, idHelp)
	w.buttonWnd = child("BUTTON", w.text.CancelButton(), wsTabStop, w.bodyFont, idButton)
	_, _, _ = procSendMessageW.Call(w.barWnd, pbmSetRange32, 0, barRange)
	w.layout(false)

	w.refresh()
	_, _, _ = procShowWindow.Call(hwnd, swShow)
	_, _, _ = procUpdateWindow.Call(hwnd)
	_, _, _ = procSetForegroundWin.Call(hwnd)
	ready <- nil

	var m winMsg
	for {
		r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		_, _, _ = procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		_, _, _ = procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
	activeWinMu.Lock()
	if activeWin == w {
		activeWin = nil
	}
	activeWinMu.Unlock()
}

// layout places the controls for the progress or the failure screen and
// resizes the window to fit, keeping its top-left corner.
func (w *winProgress) layout(failed bool) {
	w.failLayout = failed
	move := func(h uintptr, x, y, cx, cy int) {
		_, _, _ = procMoveWindow.Call(h, uintptr(w.px(x)), uintptr(w.px(y)), uintptr(w.px(cx)), uintptr(w.px(cy)), 1)
	}
	inner := layWidth - 2*layMargin
	move(w.headlineWnd, layMargin, layHeadlineY, inner, layHeadlineH)
	move(w.barWnd, layMargin, layBarY, inner, layBarH)
	move(w.marqueeWnd, layMargin, layBarY, inner, layBarH)
	buttonY := layDetailY + layDetailH + 8
	if failed {
		detailY := layHeadlineY + layHeadlineH + 8
		move(w.detailWnd, layMargin, detailY, inner, layFailDetail)
		buttonY = detailY + layFailDetail + 8
	} else {
		move(w.detailWnd, layMargin, layDetailY, inner, layDetailH)
	}
	right := layWidth - layMargin - layButtonW
	move(w.buttonWnd, right, buttonY, layButtonW, layButtonH)
	move(w.helpWnd, right-layButtonGap-layButtonW, buttonY, layButtonW, layButtonH)

	fw, fh := w.frameSize(w.px(layWidth), w.px(clientHeight(failed)))
	_, _, _ = procSetWindowPos.Call(w.hwnd, 0, 0, 0, uintptr(fw), uintptr(fh), swpNoMove|swpNoZOrder|swpNoActivate)
}

func progressWndProc(hwnd, msg, wparam, lparam uintptr) uintptr {
	activeWinMu.Lock()
	w := activeWin
	activeWinMu.Unlock()
	if w != nil && w.hwnd == hwnd {
		switch msg {
		case wmAppRefresh:
			w.refresh()
			return 0
		case wmAppClose:
			_, _, _ = procDestroyWindow.Call(hwnd)
			return 0
		case wmCtlColorStatic:
			// Labels sit on the white client area like a task dialog; the
			// headline takes the task dialog's heading blue.
			bg, _, _ := procGetSysColor.Call(colorWindow)
			_, _, _ = procSetBkColor.Call(wparam, bg)
			if lparam == w.headlineWnd {
				_, _, _ = procSetTextColor.Call(wparam, headlineColorRef)
			} else {
				fg, _, _ := procGetSysColor.Call(colorWindowText)
				_, _, _ = procSetTextColor.Call(wparam, fg)
			}
			brush, _, _ := procGetSysColorBrush.Call(colorWindow)
			return brush
		case wmCommand:
			switch wparam & 0xffff {
			case idButton:
				w.onButton()
			case idHelp:
				w.openHelp()
			}
			return 0
		case wmClose:
			// The title-bar X behaves like the button; it never closes an
			// install that is already copying.
			w.onButton()
			return 0
		case wmDestroy:
			w.dismiss()
			_, _, _ = procPostQuitMessage.Call(0)
			return 0
		}
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, msg, wparam, lparam)
	return r
}

func showIf(h uintptr, visible bool) {
	cmd := uintptr(swHide)
	if visible {
		cmd = swShow
	}
	_, _, _ = procShowWindow.Call(h, cmd)
}

// refresh runs on the UI thread and pushes state into the controls.
func (w *winProgress) refresh() {
	w.mu.Lock()
	headline, detail := w.headline, w.detail
	showBar, busy, pos := w.showBar, w.busy, w.barPos
	cancelable, cancelling, failed := w.cancelable, w.cancelling, w.failed
	helpLabel, helpURL := w.helpLabel, w.helpURL
	w.mu.Unlock()

	if failed != w.failLayout {
		w.layout(failed)
	}
	_, _, _ = procSetWindowTextW.Call(w.headlineWnd, utf16(headline))
	_, _, _ = procSetWindowTextW.Call(w.detailWnd, utf16(detail))

	showBar = showBar && !failed
	marquee := busy && !showBar && !failed
	showIf(w.barWnd, showBar)
	if showBar {
		_, _, _ = procSendMessageW.Call(w.barWnd, pbmSetPos, uintptr(pos), 0)
	}
	if marquee != w.marqueeOn {
		on := uintptr(0)
		if marquee {
			on = 1
		}
		_, _, _ = procSendMessageW.Call(w.marqueeWnd, pbmSetMarquee, on, 30)
		w.marqueeOn = marquee
	}
	showIf(w.marqueeWnd, marquee)

	// The button only appears when it does something; a greyed-out Cancel
	// during the install read as broken. It stays, disabled, between a
	// Cancel click and the worker acting on it.
	label := w.text.CancelButton()
	if failed {
		label = w.text.CloseButton()
	}
	_, _, _ = procSetWindowTextW.Call(w.buttonWnd, utf16(label))
	enable := uintptr(0)
	if cancelable || failed {
		enable = 1
	}
	_, _, _ = procEnableWindow.Call(w.buttonWnd, enable)
	showIf(w.buttonWnd, cancelable || cancelling || failed)
	if failed && helpURL != "" {
		_, _, _ = procSetWindowTextW.Call(w.helpWnd, utf16(helpLabel))
		showIf(w.helpWnd, true)
	} else {
		showIf(w.helpWnd, false)
	}
}

func (w *winProgress) onButton() {
	w.mu.Lock()
	failed, cancelable := w.failed, w.cancelable
	if cancelable && !failed {
		w.cancelable = false
		w.cancelling = true
	}
	w.mu.Unlock()
	switch {
	case failed:
		_, _, _ = procDestroyWindow.Call(w.hwnd)
	case cancelable:
		w.cancelOnce.Do(func() { close(w.cancelled) })
		w.refresh()
	}
}

func (w *winProgress) openHelp() {
	w.mu.Lock()
	url := w.helpURL
	w.mu.Unlock()
	if url == "" {
		return
	}
	_, _, _ = procShellExecuteW.Call(w.hwnd, utf16("open"), utf16(url), 0, 0, swShowNormal)
}

func (w *winProgress) dismiss() { w.dismissOne.Do(func() { close(w.dismissed) }) }

func (w *winProgress) post(msg uintptr) {
	_, _, _ = procPostMessageW.Call(w.hwnd, msg, 0, 0)
}

func (w *winProgress) SetPhase(phase updaterv2.SessionPhase, cancelable bool) {
	w.mu.Lock()
	w.headline = w.text.Phase(phase)
	w.detail = ""
	w.showBar = phase == updaterv2.SessionPhase_SESSION_PHASE_DOWNLOADING
	w.busy = !w.showBar && phase != updaterv2.SessionPhase_SESSION_PHASE_COMPLETED
	w.barPos = 0
	w.cancelable = cancelable
	w.cancelling = false
	w.mu.Unlock()
	w.post(wmAppRefresh)
}

func (w *winProgress) SetDownload(received, total, bps int64) {
	w.mu.Lock()
	w.showBar = true
	w.busy = false
	if total > 0 {
		w.barPos = int(received * barRange / total)
	}
	w.detail = w.text.Download(received, total, bps)
	w.mu.Unlock()
	w.post(wmAppRefresh)
}

func (w *winProgress) Fail(err *updaterv2.Error) {
	w.mu.Lock()
	w.failed = true
	w.cancelable = false
	w.cancelling = false
	w.showBar = false
	w.busy = false
	w.headline = w.text.FailureHeadline()
	w.detail = w.text.FailureDetail(err)
	w.helpLabel, w.helpURL = w.text.HelpURL(err)
	w.mu.Unlock()
	w.post(wmAppRefresh)
	select {
	case <-w.dismissed:
	case <-w.done:
	case <-time.After(failDismissWait):
	}
}

func (w *winProgress) Close() {
	w.closeOnce.Do(func() {
		w.post(wmAppClose)
		select {
		case <-w.done:
		case <-time.After(3 * time.Second):
		}
	})
}

func (w *winProgress) Cancelled() <-chan struct{} { return w.cancelled }
