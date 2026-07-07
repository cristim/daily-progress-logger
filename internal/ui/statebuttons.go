package ui

import (
	qt "github.com/mappu/miqt/qt6"

	"github.com/cristim/daily-progress-logger/internal/store"
)

// stateSelector is a row of three mutually exclusive icon buttons for an
// item's state: Done / Not done / Postpone to next week. It is shared by
// the evening check-in dialog and the main window's plan list.
type stateSelector struct {
	widget *qt.QWidget
	group  *qt.QButtonGroup
}

func newStateSelector(initial store.ItemState) *stateSelector {
	widget := qt.NewQWidget2()
	layout := qt.NewQHBoxLayout(widget)
	layout.SetContentsMargins(0, 0, 0, 0)
	layout.SetSpacing(2)

	group := qt.NewQButtonGroup()
	group.SetExclusive(true)

	add := func(state store.ItemState, icon qt.QStyle__StandardPixmap, tooltip string) {
		button := qt.NewQToolButton2()
		button.SetIcon(standardIcon(icon))
		button.SetToolButtonStyle(qt.ToolButtonIconOnly)
		button.SetCheckable(true)
		button.SetToolTip(tooltip)
		if state == initial {
			button.SetChecked(true)
		}
		group.AddButton2(button.QAbstractButton, int(state))
		layout.AddWidget(button.QWidget)
	}
	add(store.StateDone, qt.QStyle__SP_DialogApplyButton, "Done")
	add(store.StateTodo, qt.QStyle__SP_DialogCancelButton, "Not done (keep as an open todo)")
	add(store.StatePostponed, qt.QStyle__SP_ArrowForward, "Postpone to next week")

	widget.SetStyleSheet(`QToolButton { padding: 4px 6px; border: 1px solid transparent; border-radius: 5px; } QToolButton:checked { background-color: palette(highlight); color: palette(highlighted-text); border-color: palette(highlight); }`)

	return &stateSelector{widget: widget, group: group}
}

// state returns the currently selected item state.
func (s *stateSelector) state() store.ItemState {
	return store.ItemState(s.group.CheckedId())
}

// onChanged registers a handler invoked when the user picks a state.
func (s *stateSelector) onChanged(handler func(store.ItemState)) {
	s.group.OnIdClicked(func(id int) { handler(store.ItemState(id)) })
}

// standardIcon fetches one of the platform style's built-in icons.
func standardIcon(pixmap qt.QStyle__StandardPixmap) *qt.QIcon {
	return qt.QApplication_Style().StandardIcon(pixmap, nil, nil)
}
