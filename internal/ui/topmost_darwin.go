//go:build darwin

package ui

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>
#include <stdint.h>

// pinViewWindowAcrossSpaces makes the NSWindow backing the given NSView show on
// every Space (virtual desktop) and float over other apps' full-screen Spaces,
// so a check-in that opens while the user is on another desktop still appears in
// front of them rather than staying hidden on the Space where it was created.
static void pinViewWindowAcrossSpaces(uintptr_t viewPtr) {
    if (viewPtr == 0) {
        return;
    }
    NSView *view = (__bridge NSView *)(void *)viewPtr;
    NSWindow *window = [view window];
    if (window == nil) {
        return;
    }
    [window setCollectionBehavior:
        NSWindowCollectionBehaviorCanJoinAllSpaces |
        NSWindowCollectionBehaviorFullScreenAuxiliary];
}
// pinViewWindowAcrossSpaces needs a live NSWindow, so call it after Show.
*/
import "C"

import qt "github.com/mappu/miqt/qt6"

// pinAcrossSpaces pins the shown dialog to every macOS Space. WinId() returns
// the dialog's NSView*; the native window must already exist (call after Show).
func pinAcrossSpaces(dialog *qt.QDialog) {
	winID := dialog.WinId()
	if winID == 0 {
		return
	}
	C.pinViewWindowAcrossSpaces(C.uintptr_t(winID))
}
