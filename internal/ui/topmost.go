package ui

import qt "github.com/mappu/miqt/qt6"

// pinDialogEverywhere makes a check-in dialog impossible to miss: it floats
// above every other window and (on macOS, via pinAcrossSpaces) appears on
// whichever desktop Space / monitor the user is currently looking at, instead
// of opening behind the active app or on a Space the user has scrolled away
// from. The dialog stays modal, so the app is already blocked until answered;
// this only guarantees it is visible while it waits.
//
// The stay-on-top flag is set before the dialog is shown (changing window
// flags after Show can require a re-show on some platforms); pinAcrossSpaces
// is applied after Show in run(), once the native window exists.
func pinDialogOnTop(dialog *qt.QDialog) {
	dialog.SetWindowFlag2(qt.WindowStaysOnTopHint, true)
}
