package ui

import (
	qt "github.com/mappu/miqt/qt6"

	"github.com/cristim/daily-progress-logger/internal/store"
)

// choice describes one option in a choiceSelector.
type choice struct {
	id         int
	icon       qt.QStyle__StandardPixmap
	customIcon *qt.QIcon // overrides icon when non-nil
	tooltip    string
}

// choiceSelector is a row of mutually exclusive icon buttons where each
// button corresponds to a choice. It is the generic foundation for
// stateSelector (plan item states) and the week-review action selector.
type choiceSelector struct {
	widget *qt.QWidget
	group  *qt.QButtonGroup
}

// newChoiceSelector builds a row of exclusive icon QToolButtons from choices,
// with the button matching initialID pre-checked.
func newChoiceSelector(choices []choice, initialID int) *choiceSelector {
	widget := qt.NewQWidget2()
	layout := qt.NewQHBoxLayout(widget)
	layout.SetContentsMargins(0, 0, 0, 0)
	layout.SetSpacing(2)

	group := qt.NewQButtonGroup()
	group.SetExclusive(true)

	for _, c := range choices {
		button := qt.NewQToolButton2()
		if c.customIcon != nil {
			button.SetIcon(c.customIcon)
		} else {
			button.SetIcon(standardIcon(c.icon))
		}
		button.SetToolButtonStyle(qt.ToolButtonIconOnly)
		button.SetCheckable(true)
		button.SetToolTip(c.tooltip)
		button.SetAccessibleName(c.tooltip)
		if c.id == initialID {
			button.SetChecked(true)
		}
		group.AddButton2(button.QAbstractButton, c.id)
		layout.AddWidget(button.QWidget)
	}

	widget.SetStyleSheet(
		`QToolButton { padding: 4px 6px; border: 1px solid transparent; border-radius: 5px; }` +
			` QToolButton:checked { background-color: palette(highlight);` +
			` color: palette(highlighted-text); border-color: palette(highlight); }`,
	)

	return &choiceSelector{widget: widget, group: group}
}

// stateSelector is a row of three mutually exclusive icon buttons for an
// item's state: Done / Not done / Postpone to next week. It is shared by
// the evening check-in dialog and the main window's plan list.
type stateSelector struct {
	widget *qt.QWidget
	group  *qt.QButtonGroup
}

// newStateSelector returns a stateSelector pre-set to initial, built as a
// thin wrapper over newChoiceSelector.
func newStateSelector(initial store.ItemState) *stateSelector {
	cs := newChoiceSelector([]choice{
		{id: int(store.StateDone), icon: qt.QStyle__SP_DialogApplyButton, tooltip: "Done"},
		{id: int(store.StateTodo), icon: qt.QStyle__SP_DialogCancelButton, tooltip: "Not done (keep as an open todo)"},
		{id: int(store.StatePostponed), customIcon: postponeIcon(), tooltip: "Postpone to next week"},
	}, int(initial))
	return &stateSelector{widget: cs.widget, group: cs.group}
}

// postponeIcon draws a right-pointing chevron in a visible mid-gray on a
// 16x16 transparent pixmap. SP_ArrowForward is nearly invisible as an
// unchecked button in dark mode; this custom glyph is always legible.
func postponeIcon() *qt.QIcon {
	const size = 16
	pixmap := qt.NewQPixmap2(size, size)
	pixmap.FillWithFillColor(qt.NewQColor11(0, 0, 0, 0))
	painter := qt.NewQPainter2(pixmap.QPaintDevice)
	painter.SetRenderHint(qt.QPainter__Antialiasing)
	pen := qt.NewQPen3(qt.NewQColor3(140, 140, 140))
	pen.SetWidth(2)
	painter.SetPenWithPen(pen)
	// Right-pointing chevron: two lines meeting at the right tip.
	painter.DrawLine2(3, 4, 12, 8)
	painter.DrawLine2(3, 12, 12, 8)
	painter.End()
	return qt.NewQIcon2(pixmap)
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
