package ui

import (
	"fmt"
	"strings"
	"time"

	qt "github.com/mappu/miqt/qt6"

	"github.com/cristim/daily-progress-logger/internal/store"
)

// dialogResult is the user's verdict on a check-in dialog.
type dialogResult int

const (
	// dialogCanceled: skip this check-in for the rest of the day.
	dialogCanceled dialogResult = iota
	// dialogAccepted: answers were applied.
	dialogAccepted
	// dialogSnoozed: ask again in an hour.
	dialogSnoozed
)

// snoozeCode is the custom QDialog result code for "Postpone 1h" (0 and 1
// are taken by QDialog__Rejected and QDialog__Accepted).
const snoozeCode = 2

// dialogSpec is a built check-in dialog plus the action applying its answers
// once accepted. Building and running are separate so dialogs can also be
// rendered offscreen (see App.GrabScreenshots).
type dialogSpec struct {
	dialog *qt.QDialog
	apply  func() error
}

// run shows the dialog modally and applies the answers if accepted.
func (s *dialogSpec) run() (dialogResult, error) {
	switch s.dialog.Exec() {
	case int(qt.QDialog__Accepted):
		return dialogAccepted, s.apply()
	case snoozeCode:
		return dialogSnoozed, nil
	default:
		return dialogCanceled, nil
	}
}

func (a *App) runMorningDialog() (dialogResult, error) {
	spec, err := a.buildMorningDialog(time.Now())
	if err != nil {
		return dialogCanceled, err
	}
	return spec.run()
}

func (a *App) runEveningDialog() (dialogResult, error) {
	spec, err := a.buildEveningDialog(time.Now())
	if err != nil {
		return dialogCanceled, err
	}
	return spec.run()
}

func (a *App) runWeekReviewDialog(week store.WeekID) (dialogResult, error) {
	spec, err := a.buildWeekReviewDialog(week)
	if err != nil {
		return dialogCanceled, err
	}
	return spec.run()
}

// buildMorningDialog asks what the user plans to work on today, offering
// carry-over candidates from earlier in the week and the backlog.
func (a *App) buildMorningDialog(today time.Time) (*dialogSpec, error) {
	candidates, err := a.store.MorningCandidates(today)
	if err != nil {
		return nil, err
	}

	dialog := qt.NewQDialog(a.window.win.QWidget)
	dialog.SetWindowTitle("Morning Check-in")
	layout := qt.NewQVBoxLayout(dialog.QWidget)

	layout.AddWidget(qt.NewQLabel3("<b>What are you planning to work on today?</b>").QWidget)
	editor := qt.NewQPlainTextEdit2()
	editor.SetPlaceholderText("One task per line…")
	layout.AddWidget(editor.QWidget)

	var candidateList *qt.QListWidget
	if len(candidates) > 0 {
		layout.AddWidget(qt.NewQLabel3("Carry over open items (from this week and the backlog):").QWidget)
		candidateList = qt.NewQListWidget2()
		for _, c := range candidates {
			text := c.Text
			if c.FromBacklog {
				text += "  (backlog)"
			}
			item := qt.NewQListWidgetItem2(text)
			item.SetFlags(qt.ItemIsSelectable | qt.ItemIsEnabled | qt.ItemIsUserCheckable)
			item.SetCheckState(qt.Checked)
			candidateList.AddItemWithItem(item)
		}
		layout.AddWidget(candidateList.QWidget)
	}
	attachButtons(dialog, layout)

	apply := func() error {
		var adopted []store.Candidate
		if candidateList != nil {
			for i, c := range candidates {
				if candidateList.Item(i).CheckState() == qt.Checked {
					adopted = append(adopted, c)
				}
			}
		}
		return a.store.ApplyMorning(today, splitLines(editor.ToPlainText()), adopted)
	}
	return &dialogSpec{dialog: dialog, apply: apply}, nil
}

// buildEveningDialog asks what happened to each planned item and what else
// was accomplished.
func (a *App) buildEveningDialog(today time.Time) (*dialogSpec, error) {
	daily, _, err := a.store.LoadDaily(today)
	if err != nil {
		return nil, err
	}
	var plan []store.Item
	if daily != nil {
		plan = daily.Plan
	}

	dialog := qt.NewQDialog(a.window.win.QWidget)
	dialog.SetWindowTitle("Evening Check-in")
	layout := qt.NewQVBoxLayout(dialog.QWidget)

	layout.AddWidget(qt.NewQLabel3("<b>How did today go?</b>").QWidget)

	selectors := make([]*stateSelector, len(plan))
	if len(plan) > 0 {
		for i, item := range plan {
			row := qt.NewQHBoxLayout2()
			label := qt.NewQLabel3(item.Text)
			selector := newStateSelector(item.State)
			selectors[i] = selector
			row.AddWidget2(label.QWidget, 1)
			row.AddWidget(selector.widget)
			layout.AddLayout(row.QLayout)
		}
	} else {
		layout.AddWidget(qt.NewQLabel3("(No plan was recorded for today.)").QWidget)
	}

	layout.AddWidget(qt.NewQLabel3("Anything else you accomplished?").QWidget)
	editor := qt.NewQPlainTextEdit2()
	editor.SetPlaceholderText("One accomplishment per line…")
	layout.AddWidget(editor.QWidget)
	attachButtons(dialog, layout)

	apply := func() error {
		states := make([]store.ItemState, len(plan))
		for i, selector := range selectors {
			states[i] = selector.state()
		}
		return a.store.ApplyEvening(today, states, splitLines(editor.ToPlainText()))
	}
	return &dialogSpec{dialog: dialog, apply: apply}, nil
}

// buildWeekReviewDialog triages the given week's leftover items.
func (a *App) buildWeekReviewDialog(week store.WeekID) (*dialogSpec, error) {
	items, err := a.store.WeekReviewCandidates(week)
	if err != nil {
		return nil, err
	}

	dialog := qt.NewQDialog(a.window.win.QWidget)
	dialog.SetWindowTitle(fmt.Sprintf("Week Review: %s", week))
	layout := qt.NewQVBoxLayout(dialog.QWidget)

	title := fmt.Sprintf("<b>Starting a new week.</b> These items from %s are still open. Are they still relevant?", week)
	if len(items) == 0 {
		title = fmt.Sprintf("<b>Starting a new week.</b> Nothing left open from %s. Great job!", week)
	}
	label := qt.NewQLabel3(title)
	label.SetWordWrap(true)
	layout.AddWidget(label.QWidget)

	combos := make([]*qt.QComboBox, len(items))
	for i, text := range items {
		row := qt.NewQHBoxLayout2()
		itemLabel := qt.NewQLabel3(text)
		combo := qt.NewQComboBox2()
		combo.AddItems([]string{"Keep this week", "Postpone to next week", "Drop"})
		combos[i] = combo
		row.AddWidget2(itemLabel.QWidget, 1)
		row.AddWidget(combo.QWidget)
		layout.AddLayout(row.QLayout)
	}
	attachButtons(dialog, layout)

	apply := func() error {
		decisions := make([]store.ReviewDecision, len(items))
		for i, text := range items {
			decisions[i] = store.ReviewDecision{
				Text:   text,
				Action: actionForComboIndex(combos[i].CurrentIndex()),
			}
		}
		return a.store.ApplyWeekReview(week, decisions)
	}
	return &dialogSpec{dialog: dialog, apply: apply}, nil
}

// attachButtons appends an OK / Postpone 1h / Cancel button box wired to
// the dialog. "Postpone 1h" snoozes the check-in; Cancel skips it for the
// rest of the day.
func attachButtons(dialog *qt.QDialog, layout *qt.QVBoxLayout) {
	buttons := qt.NewQDialogButtonBox4(qt.QDialogButtonBox__Ok | qt.QDialogButtonBox__Cancel)
	snooze := buttons.AddButton2("Postpone 1h", qt.QDialogButtonBox__ActionRole)
	snooze.SetToolTip("Ask again in an hour")
	snooze.OnClicked(func() { dialog.Done(snoozeCode) })
	buttons.OnAccepted(dialog.Accept)
	buttons.OnRejected(dialog.Reject)
	layout.AddWidget(buttons.QWidget)
}

func actionForComboIndex(index int) store.ReviewAction {
	switch index {
	case 1:
		return store.ReviewPostpone
	case 2:
		return store.ReviewDrop
	default:
		return store.ReviewKeep
	}
}

// splitLines turns textarea content into trimmed, non-empty lines.
func splitLines(text string) []string {
	var lines []string
	for line := range strings.SplitSeq(text, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}
