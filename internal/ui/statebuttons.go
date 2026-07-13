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
	label      string // when non-empty, button shows text caption instead of an icon
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
		switch {
		case c.label != "":
			// Text-caption mode: show the label text, no icon.
			button.SetText(c.label)
			button.SetToolButtonStyle(qt.ToolButtonTextOnly)
		case c.customIcon != nil:
			button.SetIcon(c.customIcon)
			button.SetToolButtonStyle(qt.ToolButtonIconOnly)
		default:
			button.SetIcon(standardIcon(c.icon))
			button.SetToolButtonStyle(qt.ToolButtonIconOnly)
		}
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

// stateSelector is a row of two mutually exclusive icon buttons for an item's
// completion state: Done / Not done. Deferring an item (next day, next week,
// backlog) is handled by separate action buttons, not this selector. It is
// used by the main window's plan list.
type stateSelector struct {
	widget *qt.QWidget
	group  *qt.QButtonGroup
}

// newStateSelector returns a stateSelector pre-set to initial, built as a
// thin wrapper over newChoiceSelector. An item in the postponed state leaves
// neither button checked (postpone is no longer a selectable state here).
func newStateSelector(initial store.ItemState) *stateSelector {
	cs := newChoiceSelector([]choice{
		{id: int(store.StateDone), label: "Done", tooltip: "Done"},
		{id: int(store.StateTodo), label: "Not done", tooltip: "Not done (keep as an open todo)"},
	}, int(initial))
	return &stateSelector{widget: cs.widget, group: cs.group}
}

// eveningChoices are the per-item outcome buttons in the evening check-in:
// Done, Not done, and the three defer targets. The button ids are
// store.EveningAction values so the checked group id maps straight back to an
// action. Caption wording matches the main window's task-row action buttons for
// consistency ("Next day", "Next week", "Backlog").
func eveningChoices() []choice {
	return []choice{
		{id: int(store.EveningActionDone), label: "Done", tooltip: "Done"},
		{id: int(store.EveningActionTodo), label: "Not done", tooltip: "Not done (keep as an open todo)"},
		{id: int(store.EveningActionNextDay), label: "Next day", tooltip: "Postpone to the next day"},
		{id: int(store.EveningActionNextWeek), label: "Next week", tooltip: "Postpone to next week"},
		{id: int(store.EveningActionBacklog), label: "Backlog", tooltip: "Move to the cross-week backlog"},
	}
}

// postponeIcon draws a right-pointing chevron in a visible mid-gray on a
// 16x16 transparent pixmap. SP_ArrowForward is nearly invisible as an
// unchecked button in dark mode; this custom glyph is always legible.
// Used by the backlog dialog's "Move to next week" button.
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

// onChanged registers a handler invoked when the user picks a state.
func (s *stateSelector) onChanged(handler func(store.ItemState)) {
	s.group.OnIdClicked(func(id int) { handler(store.ItemState(id)) })
}

// standardIcon fetches one of the platform style's built-in icons.
func standardIcon(pixmap qt.QStyle__StandardPixmap) *qt.QIcon {
	return qt.QApplication_Style().StandardIcon(pixmap, nil, nil)
}
