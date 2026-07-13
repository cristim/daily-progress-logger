package ui

import qt "github.com/mappu/miqt/qt6"

// hoverReveal wires hover-reveal behaviour onto a row widget so that controls
// start hidden and appear only when the pointer enters the row or a button
// gains keyboard focus:
//
//   - controls is hidden at rest (SetVisible(false)) and shown when the pointer
//     enters row via OnEnterEvent.
//   - The leave handler uses a bounds check (MapFromGlobal + Rect.Contains) to
//     prevent spurious hiding when the pointer moves from the row background
//     onto a child widget: Qt fires LeaveEvent on the row in that case, but
//     the cursor is still within the row's geometry.
//   - Each button in focusButtons reveals controls on keyboard focus so
//     keyboard-only users can reach the buttons without a mouse. All elements
//     of focusButtons must be QToolButton instances; the helper casts via
//     UnsafePointer to reach OnFocusInEvent, which QAbstractButton does not
//     expose directly in the bindings.
//
// Call hoverReveal AFTER adding controls to row's layout so the row already
// owns the widget when events fire.
func hoverReveal(row *qt.QWidget, controls *qt.QWidget, focusButtons []*qt.QAbstractButton) {
	controls.SetVisible(false)

	showControls := func() { controls.SetVisible(true) }
	hideIfLeft := func() {
		local := row.MapFromGlobalWithQPoint(qt.QCursor_Pos())
		if !row.Rect().ContainsWithQPoint(local) {
			controls.SetVisible(false)
		}
	}

	row.OnEnterEvent(func(_ func(*qt.QEnterEvent), _ *qt.QEnterEvent) { showControls() })
	row.OnLeaveEvent(func(_ func(*qt.QEvent), _ *qt.QEvent) { hideIfLeft() })

	for _, ab := range focusButtons {
		// All buttons in this application are QToolButton instances; the unsafe
		// cast reaches OnFocusInEvent which is not exposed on QAbstractButton.
		btn := qt.UnsafeNewQToolButton(ab.UnsafePointer())
		btn.OnFocusInEvent(func(_ func(*qt.QFocusEvent), _ *qt.QFocusEvent) { showControls() })
	}
}

// newControlsContainer creates a zero-margin horizontal container for a row's
// action controls. It matches the inner layout discipline of newRowWidget so
// controls sit flush against the label with no extra padding.
func newControlsContainer() (*qt.QWidget, *qt.QHBoxLayout) {
	w := qt.NewQWidget2()
	l := qt.NewQHBoxLayout(w)
	l.SetContentsMargins(0, 0, 0, 0)
	return w, l
}
