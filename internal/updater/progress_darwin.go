//go:build darwin && cgo

package updater

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Cocoa

#import <Cocoa/Cocoa.h>
#include <stdlib.h>
#include <string.h>

extern void relkitProgressReady(void);
extern void relkitProgressButton(void);
extern void relkitProgressHelp(void);

// One window per worker process; every static below is touched only on the
// main thread.
static NSWindow *rkWindow;
static NSTextField *rkHeadline;
static NSTextField *rkDetail;
static NSProgressIndicator *rkBar;
static NSButton *rkButton;
static NSButton *rkHelp;

@interface RKProgressDelegate : NSObject <NSWindowDelegate>
@end

@implementation RKProgressDelegate
// The title-bar close button behaves like the button; it never closes an
// install that is already copying.
- (BOOL)windowShouldClose:(NSWindow *)sender {
	relkitProgressButton();
	return NO;
}
- (void)pressButton:(id)sender {
	relkitProgressButton();
}
- (void)pressHelp:(id)sender {
	relkitProgressHelp();
}
@end

static RKProgressDelegate *rkDelegate;

static NSString *rkString(const char *s) {
	NSString *v = s ? [NSString stringWithUTF8String:s] : nil;
	return v ? v : @"";
}

static NSTextField *rkLabel(NSRect frame, NSFont *font) {
	NSTextField *t = [NSTextField wrappingLabelWithString:@""];
	[t setFrame:frame];
	[t setFont:font];
	[t setSelectable:NO];
	return t;
}

// relkit_progress_run builds the window and spins the AppKit run loop until
// relkit_progress_close stops it. It must run on the main thread.
static void relkit_progress_run(const char *title, const char *headline, const char *button) {
	@autoreleasepool {
		[NSApplication sharedApplication];
		// Accessory: a window without a Dock icon or menu bar takeover.
		[NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];

		rkWindow = [[NSWindow alloc]
			initWithContentRect:NSMakeRect(0, 0, 440, 200)
			          styleMask:NSWindowStyleMaskTitled | NSWindowStyleMaskClosable | NSWindowStyleMaskMiniaturizable
			            backing:NSBackingStoreBuffered
			              defer:NO];
		[rkWindow setReleasedWhenClosed:NO];
		[rkWindow setTitle:rkString(title)];
		[rkWindow setLevel:NSFloatingWindowLevel];
		rkDelegate = [RKProgressDelegate new];
		[rkWindow setDelegate:rkDelegate];

		NSView *content = [rkWindow contentView];
		rkHeadline = rkLabel(NSMakeRect(20, 140, 400, 40), [NSFont boldSystemFontOfSize:13]);
		[rkHeadline setStringValue:rkString(headline)];
		rkBar = [[NSProgressIndicator alloc] initWithFrame:NSMakeRect(20, 116, 400, 16)];
		[rkBar setStyle:NSProgressIndicatorStyleBar];
		[rkBar setIndeterminate:NO];
		[rkBar setMinValue:0];
		[rkBar setMaxValue:1000];
		[rkBar setHidden:YES];
		rkDetail = rkLabel(NSMakeRect(20, 54, 400, 56), [NSFont systemFontOfSize:12]);
		rkButton = [NSButton buttonWithTitle:rkString(button) target:rkDelegate action:@selector(pressButton:)];
		[rkButton setFrame:NSMakeRect(328, 12, 92, 32)];
		rkHelp = [NSButton buttonWithTitle:@"" target:rkDelegate action:@selector(pressHelp:)];
		[rkHelp setFrame:NSMakeRect(228, 12, 92, 32)];
		[rkHelp setHidden:YES];
		for (NSView *v in @[ rkHeadline, rkBar, rkDetail, rkHelp, rkButton ]) {
			[content addSubview:v];
		}

		[rkWindow center];
		[rkWindow makeKeyAndOrderFront:nil];
		[NSApp activateIgnoringOtherApps:YES];
	}
	relkitProgressReady();
	[NSApp run];
	@autoreleasepool {
		rkWindow = nil;
		rkHeadline = nil;
		rkDetail = nil;
		rkBar = nil;
		rkButton = nil;
		rkHelp = nil;
		rkDelegate = nil;
	}
}

// relkit_progress_update copies the strings now (the caller frees them right
// after) and applies them on the main thread.
static void relkit_progress_update(const char *headline, const char *detail, int showBar, int barPos,
		const char *button, int buttonEnabled, const char *helpLabel, int showHelp) {
	@autoreleasepool {
		NSString *h = rkString(headline);
		NSString *d = rkString(detail);
		NSString *b = rkString(button);
		NSString *l = rkString(helpLabel);
		dispatch_async(dispatch_get_main_queue(), ^{
			if (rkWindow == nil) {
				return;
			}
			[rkHeadline setStringValue:h];
			[rkDetail setStringValue:d];
			[rkBar setHidden:showBar ? NO : YES];
			[rkBar setDoubleValue:(double)barPos];
			[rkButton setTitle:b];
			[rkButton setEnabled:buttonEnabled ? YES : NO];
			[rkHelp setTitle:l];
			[rkHelp setHidden:showHelp ? NO : YES];
		});
	}
}

// relkit_progress_close removes the window and makes relkit_progress_run
// return. Safe from any thread and more than once.
static void relkit_progress_close(void) {
	dispatch_async(dispatch_get_main_queue(), ^{
		@autoreleasepool {
			if (rkWindow != nil) {
				[rkWindow orderOut:nil];
				[rkWindow close];
			}
			[NSApp stop:nil];
			// stop only takes effect after the next event; post one so the
			// loop notices now instead of on the next mouse move.
			NSEvent *wake = [NSEvent otherEventWithType:NSEventTypeApplicationDefined
			                                   location:NSZeroPoint
			                              modifierFlags:0
			                                  timestamp:0
			                               windowNumber:0
			                                    context:nil
			                                    subtype:0
			                                      data1:0
			                                      data2:0];
			[NSApp postEvent:wake atStart:YES];
		}
	});
}

static void relkit_open_url(const char *url) {
	@autoreleasepool {
		NSURL *u = [NSURL URLWithString:rkString(url)];
		if (u != nil) {
			[[NSWorkspace sharedWorkspace] openURL:u];
		}
	}
}

// relkit_preferred_language returns the first preferred UI language as a
// malloc'd string, or NULL.
static char *relkit_preferred_language(void) {
	@autoreleasepool {
		NSString *l = [[NSLocale preferredLanguages] firstObject];
		if (l == nil) {
			return NULL;
		}
		return strdup([l UTF8String]);
	}
}
*/
import "C"

import (
	"errors"
	"strings"
	"sync"
	"time"
	"unsafe"

	updaterv2 "github.com/shichao402/relkit/api/updater/v2"
	"github.com/shichao402/relkit/internal/mainthread"
)

// AppKit progress window for the apply worker (ADR 0026 §6). It needs cgo and
// the process main thread: cmd/relkit-updater runs the worker under
// mainthread.Run, and a darwin build without cgo falls back to the no-op in
// progress_other.go. Behaviour mirrors progress_windows.go.

const (
	macBarRange        = 1000
	macFailDismissWait = 30 * time.Minute
)

func uiLanguage() string {
	if p := C.relkit_preferred_language(); p != nil {
		defer C.free(unsafe.Pointer(p))
		if l := C.GoString(p); l != "" {
			return langFromLocale(l)
		}
	}
	return langFromLocale(envLocale())
}

// macProgress mirrors winProgress: worker-side methods mutate state and push
// it to the main thread; button callbacks arrive on the main thread.
type macProgress struct {
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

	ready      chan struct{}
	done       chan struct{}
	cancelled  chan struct{}
	dismissed  chan struct{}
	readyOnce  sync.Once
	cancelOnce sync.Once
	dismissOne sync.Once
	closeOnce  sync.Once
}

// Only one window exists per worker process; the exported callbacks find it here.
var (
	activeMacMu sync.Mutex
	activeMac   *macProgress
)

func currentMac() *macProgress {
	activeMacMu.Lock()
	defer activeMacMu.Unlock()
	return activeMac
}

func platformProgressUI(spec progressSpec) (progressUI, error) {
	w := &macProgress{
		text:      progressText{spec: spec},
		ready:     make(chan struct{}),
		done:      make(chan struct{}),
		cancelled: make(chan struct{}),
		dismissed: make(chan struct{}),
	}
	w.headline = w.text.Phase(updaterv2.SessionPhase_SESSION_PHASE_UNSPECIFIED)

	activeMacMu.Lock()
	if activeMac != nil {
		activeMacMu.Unlock()
		return nil, errors.New("a progress window is already open")
	}
	activeMac = w
	activeMacMu.Unlock()

	if !mainthread.Post(w.run) {
		w.forget()
		return nil, errors.New("no main-thread loop to host the progress window")
	}
	select {
	case <-w.ready:
		return w, nil
	case <-w.done:
		return nil, errors.New("progress window exited before it was shown")
	case <-time.After(5 * time.Second):
		// The run may still start later; a queued close stops it at once.
		C.relkit_progress_close()
		return nil, errors.New("progress window did not start")
	}
}

func (w *macProgress) forget() {
	activeMacMu.Lock()
	if activeMac == w {
		activeMac = nil
	}
	activeMacMu.Unlock()
}

// run executes on the main thread and returns when the window is closed.
func (w *macProgress) run() {
	defer close(w.done)
	defer w.forget()
	defer w.dismiss()
	title := C.CString(w.text.Title())
	headline := C.CString(w.headline)
	button := C.CString(w.text.CancelButton())
	defer C.free(unsafe.Pointer(title))
	defer C.free(unsafe.Pointer(headline))
	defer C.free(unsafe.Pointer(button))
	C.relkit_progress_run(title, headline, button)
}

func (w *macProgress) onReady() {
	w.readyOnce.Do(func() { close(w.ready) })
	w.refresh()
}

// refresh pushes the current state to the window; callable from any thread.
func (w *macProgress) refresh() {
	w.mu.Lock()
	headline, detail := w.headline, strings.ReplaceAll(w.detail, "\r\n", "\n")
	showBar, pos := w.showBar, w.barPos
	cancelable, failed := w.cancelable, w.failed
	helpLabel, helpURL := w.helpLabel, w.helpURL
	w.mu.Unlock()

	label := w.text.CancelButton()
	if failed {
		label = w.text.CloseButton()
	}
	cs := []*C.char{C.CString(headline), C.CString(detail), C.CString(label), C.CString(helpLabel)}
	defer func() {
		for _, s := range cs {
			C.free(unsafe.Pointer(s))
		}
	}()
	C.relkit_progress_update(cs[0], cs[1], cbool(showBar), C.int(pos),
		cs[2], cbool(cancelable || failed), cs[3], cbool(failed && helpURL != ""))
}

func cbool(b bool) C.int {
	if b {
		return 1
	}
	return 0
}

// onButton runs on the main thread for the button and the title-bar close.
func (w *macProgress) onButton() {
	w.mu.Lock()
	failed, cancelable := w.failed, w.cancelable
	if cancelable && !failed {
		w.cancelable = false
	}
	w.mu.Unlock()
	switch {
	case failed:
		C.relkit_progress_close()
	case cancelable:
		w.cancelOnce.Do(func() { close(w.cancelled) })
		w.refresh()
	}
}

func (w *macProgress) openHelp() {
	w.mu.Lock()
	url := w.helpURL
	w.mu.Unlock()
	if url == "" {
		return
	}
	cu := C.CString(url)
	defer C.free(unsafe.Pointer(cu))
	C.relkit_open_url(cu)
}

func (w *macProgress) dismiss() { w.dismissOne.Do(func() { close(w.dismissed) }) }

func (w *macProgress) isDone() bool {
	select {
	case <-w.done:
		return true
	default:
		return false
	}
}

func (w *macProgress) SetPhase(phase updaterv2.SessionPhase, cancelable bool) {
	w.mu.Lock()
	w.headline = w.text.Phase(phase)
	w.detail = ""
	w.showBar = phase == updaterv2.SessionPhase_SESSION_PHASE_DOWNLOADING
	w.barPos = 0
	w.cancelable = cancelable
	w.mu.Unlock()
	if !w.isDone() {
		w.refresh()
	}
}

func (w *macProgress) SetDownload(received, total, bps int64) {
	w.mu.Lock()
	w.showBar = true
	if total > 0 {
		w.barPos = int(received * macBarRange / total)
	}
	w.detail = w.text.Download(received, total, bps)
	w.mu.Unlock()
	if !w.isDone() {
		w.refresh()
	}
}

func (w *macProgress) Fail(err *updaterv2.Error) {
	w.mu.Lock()
	w.failed = true
	w.cancelable = false
	w.showBar = false
	w.headline = w.text.FailureHeadline()
	w.detail = w.text.FailureDetail(err)
	w.helpLabel, w.helpURL = w.text.HelpURL(err)
	w.mu.Unlock()
	if w.isDone() {
		return
	}
	w.refresh()
	select {
	case <-w.dismissed:
	case <-w.done:
	case <-time.After(macFailDismissWait):
	}
}

func (w *macProgress) Close() {
	w.closeOnce.Do(func() {
		if w.isDone() {
			return
		}
		C.relkit_progress_close()
		select {
		case <-w.done:
		case <-time.After(3 * time.Second):
		}
	})
}

func (w *macProgress) Cancelled() <-chan struct{} { return w.cancelled }
