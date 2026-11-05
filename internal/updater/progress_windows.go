//go:build windows

package updater

import (
	"errors"
	"runtime"
	"sync"
	"time"
	"unsafe"

	updaterv2 "github.com/shichao402/relkit/api/updater/v2"
	"golang.org/x/sys/windows"
)

// Win32 progress window for the apply worker (ADR 0026 §6). Plain user32 and
// comctl32 calls through x/sys/windows: no cgo, cross-compiles from any host.

var (
	modUser32   = windows.NewLazySystemDLL("user32.dll")
	modKernel32 = windows.NewLazySystemDLL("kernel32.dll")
	modComctl32 = windows.NewLazySystemDLL("comctl32.dll")
	modGdi32    = windows.NewLazySystemDLL("gdi32.dll")
	modShell32  = windows.NewLazySystemDLL("shell32.dll")

	procRegisterClassExW   = modUser32.NewProc("RegisterClassExW")
	procCreateWindowExW    = modUser32.NewProc("CreateWindowExW")
	procDefWindowProcW     = modUser32.NewProc("DefWindowProcW")
	procGetMessageW        = modUser32.NewProc("GetMessageW")
	procTranslateMessage   = modUser32.NewProc("TranslateMessage")
	procDispatchMessageW   = modUser32.NewProc("DispatchMessageW")
	procPostMessageW       = modUser32.NewProc("PostMessageW")
	procSendMessageW       = modUser32.NewProc("SendMessageW")
	procSetWindowTextW     = modUser32.NewProc("SetWindowTextW")
	procShowWindow         = modUser32.NewProc("ShowWindow")
	procUpdateWindow       = modUser32.NewProc("UpdateWindow")
	procDestroyWindow      = modUser32.NewProc("DestroyWindow")
	procPostQuitMessage    = modUser32.NewProc("PostQuitMessage")
	procEnableWindow       = modUser32.NewProc("EnableWindow")
	procGetSystemMetrics   = modUser32.NewProc("GetSystemMetrics")
	procLoadCursorW        = modUser32.NewProc("LoadCursorW")
	procSetForegroundWin   = modUser32.NewProc("SetForegroundWindow")
	procGetModuleHandleW   = modKernel32.NewProc("GetModuleHandleW")
	procGetUserDefUILang   = modKernel32.NewProc("GetUserDefaultUILanguage")
	procInitCommonControls = modComctl32.NewProc("InitCommonControlsEx")
	procGetStockObject     = modGdi32.NewProc("GetStockObject")
	procShellExecuteW      = modShell32.NewProc("ShellExecuteW")
)

const (
	wsOverlapped  = 0x00000000
	wsCaption     = 0x00C00000
	wsSysMenu     = 0x00080000
	wsMinimizeBox = 0x00020000
	wsVisible     = 0x10000000
	wsChild       = 0x40000000
	wsTabStop     = 0x00010000
	wsExTopmost   = 0x00000008
	wsExAppWindow = 0x00040000

	wmDestroy = 0x0002
	wmClose   = 0x0010
	wmSetFont = 0x0030
	wmCommand = 0x0111
	wmApp     = 0x8000
	wmUser    = 0x0400

	wmAppRefresh = wmApp + 1
	wmAppClose   = wmApp + 2

	pbmSetPos     = wmUser + 2
	pbmSetRange32 = wmUser + 6

	swHide       = 0
	swShowNormal = 1
	swShow       = 5

	colorBtnFace    = 15
	idcArrow        = 32512
	smCxScreen      = 0
	smCyScreen      = 1
	iccProgress     = 0x00000020
	defaultGUIFont  = 17
	langChinese     = 0x04
	idButton        = 101
	idHelp          = 102
	barRange        = 1000
	clientWidth     = 440
	clientHeight    = 230
	failDismissWait = 30 * time.Minute
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
	barPos     int
	cancelable bool
	failed     bool
	helpLabel  string
	helpURL    string

	hwnd, headlineWnd, detailWnd, barWnd, buttonWnd, helpWnd uintptr

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

func (w *winProgress) loop(ready chan<- error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer close(w.done)

	icc := initCommonControlsEx{Size: uint32(unsafe.Sizeof(initCommonControlsEx{})), ICC: iccProgress}
	_, _, _ = procInitCommonControls.Call(uintptr(unsafe.Pointer(&icc)))

	inst, _, _ := procGetModuleHandleW.Call(0)
	cursor, _, _ := procLoadCursorW.Call(0, idcArrow)
	className, _ := windows.UTF16PtrFromString("RelkitUpdaterProgress")
	wc := wndClassEx{
		WndProc:    wndProcPtr,
		Instance:   windows.Handle(inst),
		Cursor:     windows.Handle(cursor),
		Background: windows.Handle(colorBtnFace + 1),
		ClassName:  className,
	}
	wc.Size = uint32(unsafe.Sizeof(wc))
	// A second registration in the same process fails harmlessly with
	// ERROR_CLASS_ALREADY_EXISTS; CreateWindowEx below is the real check.
	_, _, _ = procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))

	activeWinMu.Lock()
	activeWin = w
	activeWinMu.Unlock()

	sw, _, _ := procGetSystemMetrics.Call(smCxScreen)
	sh, _, _ := procGetSystemMetrics.Call(smCyScreen)
	x := (int(sw) - clientWidth) / 2
	y := (int(sh) - clientHeight) / 2
	hwnd, _, cerr := procCreateWindowExW.Call(
		wsExTopmost|wsExAppWindow,
		uintptr(unsafe.Pointer(className)),
		utf16(w.text.Title()),
		wsOverlapped|wsCaption|wsSysMenu|wsMinimizeBox,
		uintptr(x), uintptr(y), clientWidth, clientHeight,
		0, 0, inst, 0,
	)
	if hwnd == 0 {
		ready <- cerr
		return
	}
	w.hwnd = hwnd
	font, _, _ := procGetStockObject.Call(defaultGUIFont)
	child := func(class, text string, style uintptr, x, y, cx, cy int, id uintptr) uintptr {
		h, _, _ := procCreateWindowExW.Call(0, utf16(class), utf16(text),
			wsChild|wsVisible|style, uintptr(x), uintptr(y), uintptr(cx), uintptr(cy), hwnd, id, inst, 0)
		if h != 0 && font != 0 {
			_, _, _ = procSendMessageW.Call(h, wmSetFont, font, 1)
		}
		return h
	}
	w.headlineWnd = child("STATIC", w.headline, 0, 20, 18, 392, 36, 0)
	w.barWnd = child("msctls_progress32", "", 0, 20, 60, 392, 18, 0)
	w.detailWnd = child("STATIC", "", 0, 20, 86, 392, 64, 0)
	w.helpWnd = child("BUTTON", "", wsTabStop, 208, 154, 96, 28, idHelp)
	w.buttonWnd = child("BUTTON", w.text.CancelButton(), wsTabStop, 316, 154, 96, 28, idButton)
	_, _, _ = procSendMessageW.Call(w.barWnd, pbmSetRange32, 0, barRange)

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

// refresh runs on the UI thread and pushes state into the controls.
func (w *winProgress) refresh() {
	w.mu.Lock()
	headline, detail := w.headline, w.detail
	showBar, pos := w.showBar, w.barPos
	cancelable, failed := w.cancelable, w.failed
	helpLabel, helpURL := w.helpLabel, w.helpURL
	w.mu.Unlock()

	_, _, _ = procSetWindowTextW.Call(w.headlineWnd, utf16(headline))
	_, _, _ = procSetWindowTextW.Call(w.detailWnd, utf16(detail))
	if showBar {
		_, _, _ = procShowWindow.Call(w.barWnd, swShow)
		_, _, _ = procSendMessageW.Call(w.barWnd, pbmSetPos, uintptr(pos), 0)
	} else {
		_, _, _ = procShowWindow.Call(w.barWnd, swHide)
	}
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
	if failed && helpURL != "" {
		_, _, _ = procSetWindowTextW.Call(w.helpWnd, utf16(helpLabel))
		_, _, _ = procShowWindow.Call(w.helpWnd, swShow)
	} else {
		_, _, _ = procShowWindow.Call(w.helpWnd, swHide)
	}
}

func (w *winProgress) onButton() {
	w.mu.Lock()
	failed, cancelable := w.failed, w.cancelable
	if cancelable && !failed {
		w.cancelable = false
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
	w.barPos = 0
	w.cancelable = cancelable
	w.mu.Unlock()
	w.post(wmAppRefresh)
}

func (w *winProgress) SetDownload(received, total, bps int64) {
	w.mu.Lock()
	w.showBar = true
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
	w.showBar = false
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
