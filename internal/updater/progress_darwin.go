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

// Layout in points, laid out top-down in a flipped content view. The shape
// follows a macOS alert: app icon on the left, bold headline, then the bar
// and a quiet status line, buttons bottom-right.
#define RK_WIDTH      460.0
#define RK_MARGIN     20.0
#define RK_ICON       64.0
#define RK_ICON_GAP   16.0
#define RK_BAR_GAP    10.0
#define RK_BAR_H      16.0
#define RK_DETAIL_GAP 6.0
#define RK_FAIL_GAP   8.0
#define RK_ROW_GAP    16.0
#define RK_BUTTON_H   32.0
#define RK_BUTTON_MIN 84.0
#define RK_BUTTON_GAP 8.0
#define RK_BOTTOM     12.0
// A push button's frame is ~6pt wider than its bezel on each side.
#define RK_BEZEL_INSET 6.0

// One window per worker process; every static below is touched only on the
// main thread.
static NSWindow *rkWindow;
static NSImageView *rkIcon;
static NSTextField *rkHeadline;
static NSTextField *rkDetail;
static NSProgressIndicator *rkBar;
static NSButton *rkButton;
static NSButton *rkHelp;

@interface RKFlippedView : NSView
@end

@implementation RKFlippedView
- (BOOL)isFlipped {
	return YES;
}
@end

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

static NSTextField *rkLabel(NSFont *font, NSColor *color) {
	NSTextField *t = [NSTextField wrappingLabelWithString:@""];
	[t setFont:font];
	[t setTextColor:color];
	[t setSelectable:NO];
	return t;
}

// rkTextHeight is the height a label of width w needs for s; an empty string
// still takes one line so the layout does not jump.
static CGFloat rkTextHeight(NSTextField *label, NSString *s, CGFloat w, NSInteger maxLines) {
	NSFont *font = [label font];
	NSLayoutManager *lm = [NSLayoutManager new];
	CGFloat line = ceil([lm defaultLineHeightForFont:font]);
	if ([s length] == 0) {
		return line;
	}
	NSRect r = [s boundingRectWithSize:NSMakeSize(w - 4, CGFLOAT_MAX)
	                           options:NSStringDrawingUsesLineFragmentOrigin
	                        attributes:@{NSFontAttributeName : font}];
	CGFloat h = ceil(r.size.height);
	if (maxLines > 0 && h > line * maxLines) {
		h = line * maxLines;
	}
	return h < line ? line : h;
}

static CGFloat rkButtonWidth(NSButton *b) {
	[b sizeToFit];
	CGFloat w = [b frame].size.width;
	return w < RK_BUTTON_MIN ? RK_BUTTON_MIN : w;
}

// rkLayout places the controls for the progress or the failure screen and
// sizes the window to fit, keeping its top-left corner where it is.
static void rkLayout(BOOL failed, BOOL showHelp) {
	BOOL hasIcon = [rkIcon image] != nil;
	CGFloat x = hasIcon ? RK_MARGIN + RK_ICON + RK_ICON_GAP : RK_MARGIN;
	CGFloat tw = RK_WIDTH - x - RK_MARGIN;
	CGFloat y = RK_MARGIN;

	[rkIcon setHidden:!hasIcon];
	[rkIcon setFrame:NSMakeRect(RK_MARGIN, RK_MARGIN, RK_ICON, RK_ICON)];

	CGFloat hh = rkTextHeight(rkHeadline, [rkHeadline stringValue], tw, 3);
	[rkHeadline setFrame:NSMakeRect(x, y, tw, hh)];
	y += hh;

	if (failed) {
		y += RK_FAIL_GAP;
		[[rkDetail cell] setLineBreakMode:NSLineBreakByWordWrapping];
		[rkDetail setMaximumNumberOfLines:12];
		CGFloat dh = rkTextHeight(rkDetail, [rkDetail stringValue], tw, 12);
		[rkDetail setFrame:NSMakeRect(x, y, tw, dh)];
		y += dh;
	} else {
		y += RK_BAR_GAP;
		[rkBar setFrame:NSMakeRect(x, y, tw, RK_BAR_H)];
		y += RK_BAR_H + RK_DETAIL_GAP;
		[[rkDetail cell] setLineBreakMode:NSLineBreakByTruncatingTail];
		[rkDetail setMaximumNumberOfLines:1];
		CGFloat dh = rkTextHeight(rkDetail, @"", tw, 1);
		[rkDetail setFrame:NSMakeRect(x, y, tw, dh)];
		y += dh;
	}
	if (hasIcon && y < RK_MARGIN + RK_ICON) {
		y = RK_MARGIN + RK_ICON;
	}

	// The button row is kept even while no button shows, so the window does
	// not change height between phases.
	y += RK_ROW_GAP;
	CGFloat right = RK_WIDTH - RK_MARGIN + RK_BEZEL_INSET;
	CGFloat bw = rkButtonWidth(rkButton);
	[rkButton setFrame:NSMakeRect(right - bw, y, bw, RK_BUTTON_H)];
	if (showHelp) {
		CGFloat hw = rkButtonWidth(rkHelp);
		[rkHelp setFrame:NSMakeRect(right - bw - RK_BUTTON_GAP - hw, y, hw, RK_BUTTON_H)];
	}
	CGFloat height = y + RK_BUTTON_H + RK_BOTTOM;

	NSRect old = [rkWindow frame];
	NSRect next = [rkWindow frameRectForContentRect:NSMakeRect(0, 0, RK_WIDTH, height)];
	next.origin.x = old.origin.x;
	next.origin.y = old.origin.y + old.size.height - next.size.height;
	if (!NSEqualRects(old, next)) {
		[rkWindow setFrame:next display:YES animate:[rkWindow isVisible]];
	}
}

// rkApply pushes one state snapshot into the controls; main thread only.
static void rkApply(NSString *headline, NSString *detail, BOOL showBar, double barPos, BOOL busy,
		NSString *button, BOOL buttonEnabled, BOOL showButton, BOOL failed, NSString *helpLabel, BOOL showHelp) {
	[rkHeadline setStringValue:headline];
	[rkDetail setStringValue:detail];
	if (failed) {
		// Selectable so the error can be copied into a support request.
		[rkDetail setSelectable:YES];
		[rkDetail setFont:[NSFont systemFontOfSize:[NSFont systemFontSize] - 1]];
		[rkDetail setTextColor:[NSColor labelColor]];
	} else {
		[rkDetail setSelectable:NO];
		[rkDetail setFont:[NSFont monospacedDigitSystemFontOfSize:[NSFont smallSystemFontSize]
		                                                    weight:NSFontWeightRegular]];
		[rkDetail setTextColor:[NSColor secondaryLabelColor]];
	}

	// Determinate while downloading, an indeterminate bar while waiting or
	// installing so the window never looks stuck.
	BOOL marquee = busy && !showBar && !failed;
	BOOL barVisible = (showBar || marquee) && !failed;
	if (marquee) {
		if (![rkBar isIndeterminate]) {
			[rkBar setIndeterminate:YES];
		}
		[rkBar startAnimation:nil];
	} else {
		[rkBar stopAnimation:nil];
		if ([rkBar isIndeterminate]) {
			[rkBar setIndeterminate:NO];
		}
		[rkBar setDoubleValue:barPos];
	}
	[rkBar setHidden:!barVisible];

	[rkButton setTitle:button];
	[rkButton setEnabled:buttonEnabled];
	[rkButton setHidden:!showButton];
	// Return confirms Close on the failure screen; Escape cancels otherwise.
	[rkButton setKeyEquivalent:failed ? @"\r" : @"\033"];
	[rkHelp setTitle:helpLabel];
	[rkHelp setHidden:!showHelp];

	rkLayout(failed, showHelp);
}

// relkit_progress_run builds the window and spins the AppKit run loop until
// relkit_progress_close stops it. It must run on the main thread.
static void relkit_progress_run(const char *title, const char *headline, const char *button, const char *appBundle) {
	@autoreleasepool {
		[NSApplication sharedApplication];
		// Accessory: a window without a Dock icon or menu bar takeover.
		[NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];

		rkWindow = [[NSWindow alloc]
			initWithContentRect:NSMakeRect(0, 0, RK_WIDTH, 140)
			          styleMask:NSWindowStyleMaskTitled | NSWindowStyleMaskClosable | NSWindowStyleMaskMiniaturizable
			            backing:NSBackingStoreBuffered
			              defer:NO];
		[rkWindow setReleasedWhenClosed:NO];
		[rkWindow setTitle:rkString(title)];
		[rkWindow setLevel:NSFloatingWindowLevel];
		rkDelegate = [RKProgressDelegate new];
		[rkWindow setDelegate:rkDelegate];

		RKFlippedView *content = [[RKFlippedView alloc] initWithFrame:NSMakeRect(0, 0, RK_WIDTH, 140)];
		[rkWindow setContentView:content];

		rkIcon = [NSImageView imageViewWithImage:[NSImage new]];
		[rkIcon setImage:nil];
		[rkIcon setImageScaling:NSImageScaleProportionallyUpOrDown];
		NSString *bundle = rkString(appBundle);
		if ([bundle length] > 0) {
			NSImage *img = [[NSWorkspace sharedWorkspace] iconForFile:bundle];
			[img setSize:NSMakeSize(RK_ICON, RK_ICON)];
			[rkIcon setImage:img];
		}

		rkHeadline = rkLabel([NSFont boldSystemFontOfSize:[NSFont systemFontSize]], [NSColor labelColor]);
		[rkHeadline setStringValue:rkString(headline)];
		rkBar = [[NSProgressIndicator alloc] initWithFrame:NSZeroRect];
		[rkBar setStyle:NSProgressIndicatorStyleBar];
		[rkBar setIndeterminate:YES];
		[rkBar setMinValue:0];
		[rkBar setMaxValue:1000];
		rkDetail = rkLabel([NSFont monospacedDigitSystemFontOfSize:[NSFont smallSystemFontSize] weight:NSFontWeightRegular],
			[NSColor secondaryLabelColor]);
		rkButton = [NSButton buttonWithTitle:rkString(button) target:rkDelegate action:@selector(pressButton:)];
		[rkButton setKeyEquivalent:@"\033"];
		rkHelp = [NSButton buttonWithTitle:@"" target:rkDelegate action:@selector(pressHelp:)];
		[rkHelp setHidden:YES];
		for (NSView *v in @[ rkIcon, rkHeadline, rkBar, rkDetail, rkHelp, rkButton ]) {
			[content addSubview:v];
		}
		[rkBar startAnimation:nil];

		rkLayout(NO, NO);
		[rkWindow center];
		[rkWindow makeKeyAndOrderFront:nil];
		[NSApp activateIgnoringOtherApps:YES];
	}
	relkitProgressReady();
	[NSApp run];
	@autoreleasepool {
		rkWindow = nil;
		rkIcon = nil;
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
static void relkit_progress_update(const char *headline, const char *detail, int showBar, int barPos, int busy,
		const char *button, int buttonEnabled, int showButton, int failed, const char *helpLabel, int showHelp) {
	@autoreleasepool {
		NSString *h = rkString(headline);
		NSString *d = rkString(detail);
		NSString *b = rkString(button);
		NSString *l = rkString(helpLabel);
		dispatch_async(dispatch_get_main_queue(), ^{
			if (rkWindow == nil) {
				return;
			}
			rkApply(h, d, showBar != 0, (double)barPos, busy != 0, b, buttonEnabled != 0, showButton != 0,
				failed != 0, l, showHelp != 0);
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
	busy       bool // indeterminate phase: animated bar
	barPos     int
	cancelable bool
	cancelling bool
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
		busy:      true,
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
	bundle := C.CString(w.text.spec.AppBundle)
	defer C.free(unsafe.Pointer(title))
	defer C.free(unsafe.Pointer(headline))
	defer C.free(unsafe.Pointer(button))
	defer C.free(unsafe.Pointer(bundle))
	C.relkit_progress_run(title, headline, button, bundle)
}

func (w *macProgress) onReady() {
	w.readyOnce.Do(func() { close(w.ready) })
	w.refresh()
}

// refresh pushes the current state to the window; callable from any thread.
func (w *macProgress) refresh() {
	w.mu.Lock()
	headline, detail := w.headline, strings.ReplaceAll(w.detail, "\r\n", "\n")
	showBar, busy, pos := w.showBar, w.busy, w.barPos
	cancelable, cancelling, failed := w.cancelable, w.cancelling, w.failed
	helpLabel, helpURL := w.helpLabel, w.helpURL
	w.mu.Unlock()

	// The button only appears when it does something; it stays, disabled,
	// between a Cancel click and the worker acting on it.
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
	C.relkit_progress_update(cs[0], cs[1], cbool(showBar && !failed), C.int(pos), cbool(busy),
		cs[2], cbool(cancelable || failed), cbool(cancelable || cancelling || failed), cbool(failed),
		cs[3], cbool(failed && helpURL != ""))
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
		w.cancelling = true
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
	w.busy = !w.showBar && phase != updaterv2.SessionPhase_SESSION_PHASE_COMPLETED
	w.barPos = 0
	w.cancelable = cancelable
	w.cancelling = false
	w.mu.Unlock()
	if !w.isDone() {
		w.refresh()
	}
}

func (w *macProgress) SetDownload(received, total, bps int64) {
	w.mu.Lock()
	w.showBar = true
	w.busy = false
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
	w.cancelling = false
	w.showBar = false
	w.busy = false
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
