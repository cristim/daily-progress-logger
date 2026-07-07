package ui

import (
	"fmt"
	"html"
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

// run shows the dialog modally and applies the answers if accepted. The
// dialog is raised to the front: the app usually sits in the background, so
// without this a timer-triggered check-in can open unnoticed behind the
// active application.
func (s *dialogSpec) run() (dialogResult, error) {
	s.dialog.Show()
	s.dialog.Raise()
	s.dialog.ActivateWindow()
	switch s.dialog.Exec() {
	case int(qt.QDialog__Accepted):
		return dialogAccepted, s.apply()
	case snoozeCode:
		return dialogSnoozed, nil
	default:
		return dialogCanceled, nil
	}
}

func (a *App) runWeeklySummaryDialog(week store.WeekID) (dialogResult, error) {
	spec, err := a.buildWeeklySummaryDialog(week)
	if err != nil {
		return dialogCanceled, err
	}
	return spec.run()
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

	// Load the existing plan so we can show a summary when re-running the
	// morning dialog after the plan has already been recorded.
	daily, dailyExists, err := a.store.LoadDaily(today)
	if err != nil {
		return nil, err
	}

	dialog := qt.NewQDialog(a.window.win.QWidget)
	dialog.SetWindowTitle("Morning Check-in")
	dialog.SetMinimumWidth(460)
	layout := qt.NewQVBoxLayout(dialog.QWidget)

	layout.AddWidget(qt.NewQLabel3("<b>What are you planning to work on today?</b>").QWidget)

	// When today's plan already exists, show a read-only summary so the user
	// knows their earlier tasks are still there (they are not visible in the
	// editor because ApplyMorning deduplicates on accept).
	if dailyExists && daily != nil && len(daily.Plan) > 0 {
		n := len(daily.Plan)
		word := "items"
		if n == 1 {
			word = "item"
		}
		summary := qt.NewQLabel3(fmt.Sprintf("Already planned today: %d %s.", n, word))
		summary.SetTextFormat(qt.PlainText)
		var tipLines []string
		for _, it := range daily.Plan {
			tipLines = append(tipLines, "• "+it.Text)
		}
		summary.SetToolTip(strings.Join(tipLines, "\n"))
		layout.AddWidget(summary.QWidget)
	}

	editor := qt.NewQPlainTextEdit2()
	editor.SetPlaceholderText("One task per line…")
	editor.SetFocus()
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
			// Backlog items default unchecked: they were explicitly parked
			// away from today's plan, so re-adopting them requires an active
			// choice. Same-week carry-overs default checked: they were planned
			// recently and are likely still relevant.
			if c.FromBacklog {
				item.SetCheckState(qt.Unchecked)
			} else {
				item.SetCheckState(qt.Checked)
			}
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
	dialog.SetMinimumWidth(460)
	layout := qt.NewQVBoxLayout(dialog.QWidget)

	layout.AddWidget(qt.NewQLabel3("<b>How did today go?</b>").QWidget)

	selectors := make([]*stateSelector, len(plan))
	if len(plan) > 0 {
		area, rows := newRowsArea()
		for i, item := range plan {
			row := qt.NewQHBoxLayout2()
			label := qt.NewQLabel3(item.Text)
			label.SetTextFormat(qt.PlainText)
			label.SetWordWrap(true)
			selector := newStateSelector(item.State)
			selectors[i] = selector
			row.AddWidget2(label.QWidget, 1)
			row.AddWidget(selector.widget)
			rows.AddLayout(row.QLayout)
		}
		layout.AddWidget(area.QWidget)
	} else {
		noplan := qt.NewQLabel3("No plan was recorded for today.")
		noplan.SetTextFormat(qt.PlainText)
		layout.AddWidget(noplan.QWidget)
	}

	layout.AddWidget(qt.NewQLabel3("Anything else you accomplished?").QWidget)
	editor := qt.NewQPlainTextEdit2()
	editor.SetPlaceholderText("One accomplishment per line…")
	editor.SetFocus()
	layout.AddWidget(editor.QWidget)
	attachButtons(dialog, layout)

	apply := func() error {
		decisions := make([]store.EveningDecision, len(plan))
		for i, item := range plan {
			decisions[i] = store.EveningDecision{
				Text:  item.Text,
				State: selectors[i].state(),
			}
		}
		return a.store.ApplyEvening(today, decisions, splitLines(editor.ToPlainText()))
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
	dialog.SetMinimumWidth(460)
	layout := qt.NewQVBoxLayout(dialog.QWidget)

	title := fmt.Sprintf("<b>Starting a new week.</b> These items from %s are still open. Are they still relevant?", week)
	if len(items) == 0 {
		title = fmt.Sprintf("<b>Starting a new week.</b> Nothing left open from %s. Great job!", week)
	}
	label := qt.NewQLabel3(title)
	label.SetWordWrap(true)
	layout.AddWidget(label.QWidget)

	reviewChoices := []choice{
		{id: int(store.ReviewKeep), icon: qt.QStyle__SP_DialogApplyButton, tooltip: "Keep this week"},
		{id: int(store.ReviewPostpone), customIcon: postponeIcon(), tooltip: "Postpone to next week"},
		{id: int(store.ReviewDrop), icon: qt.QStyle__SP_TrashIcon, tooltip: "Drop"},
	}

	selectors := make([]*choiceSelector, len(items))
	if len(items) > 0 {
		area, rows := newRowsArea()
		for i, text := range items {
			row := qt.NewQHBoxLayout2()
			itemLabel := qt.NewQLabel3(text)
			itemLabel.SetTextFormat(qt.PlainText)
			itemLabel.SetWordWrap(true)
			sel := newChoiceSelector(reviewChoices, int(store.ReviewKeep))
			selectors[i] = sel
			row.AddWidget2(itemLabel.QWidget, 1)
			row.AddWidget(sel.widget)
			rows.AddLayout(row.QLayout)
		}
		layout.AddWidget(area.QWidget)
	}
	attachButtons(dialog, layout)

	apply := func() error {
		decisions := make([]store.ReviewDecision, len(items))
		for i, text := range items {
			decisions[i] = store.ReviewDecision{
				Text:   text,
				Action: store.ReviewAction(selectors[i].group.CheckedId()),
			}
		}
		return a.store.ApplyWeekReview(week, decisions)
	}
	return &dialogSpec{dialog: dialog, apply: apply}, nil
}

// buildWeeklySummaryDialog shows the current week's accomplishments for a
// quick Friday review and lets the user open the weekly file.
func (a *App) buildWeeklySummaryDialog(week store.WeekID) (*dialogSpec, error) {
	dailies, err := a.store.DailiesInWeek(week)
	if err != nil {
		return nil, err
	}

	dialog := qt.NewQDialog(a.window.win.QWidget)
	dialog.SetWindowTitle(fmt.Sprintf("Week Summary: %s", week))
	dialog.SetMinimumWidth(500)
	layout := qt.NewQVBoxLayout(dialog.QWidget)

	// Build the HTML content for the browser.
	var sb strings.Builder
	sb.WriteString("<h3>Done this week</h3>")
	totalDone := 0
	for _, dd := range store.DoneByDay(dailies) {
		fmt.Fprintf(&sb, "<b>%s, %d %s</b><ul>",
			dd.Date.Weekday(), dd.Date.Day(), dd.Date.Month())
		for _, text := range dd.Items {
			fmt.Fprintf(&sb, "<li>%s</li>", html.EscapeString(text))
		}
		sb.WriteString("</ul>")
		totalDone += len(dd.Items)
	}
	if totalDone == 0 {
		sb.WriteString("<p><i>Nothing completed yet this week.</i></p>")
	} else {
		itemWord := "items"
		if totalDone == 1 {
			itemWord = "item"
		}
		fmt.Fprintf(&sb, "<p><i>%d %s completed.</i></p>", totalDone, itemWord)
	}

	browser := qt.NewQTextBrowser2()
	browser.SetHtml(sb.String())
	browser.SetReadOnly(true)
	browser.SetMinimumHeight(240)
	layout.AddWidget(browser.QWidget)

	// "Open Weekly File" button regenerates the file so it exists, then opens it.
	openBtn := qt.NewQPushButton3("Open Weekly File")
	weeklyPath := a.store.WeeklyPath(week)
	openBtn.OnClicked(func() {
		// Regenerate so the file exists even if this is an early-week view.
		_ = a.store.RegenerateWeekly(week)
		qt.QDesktopServices_OpenUrl(qt.QUrl_FromLocalFile(weeklyPath))
	})
	layout.AddWidget(openBtn.QWidget)

	attachButtons(dialog, layout)

	apply := func() error {
		return a.store.MarkWeekSummarized(week)
	}
	return &dialogSpec{dialog: dialog, apply: apply}, nil
}

// attachButtons appends an OK / Postpone 1h / Skip Today button box wired
// to the dialog. "Postpone 1h" snoozes the check-in; "Skip Today" (the
// reject role, so Escape triggers it) silences it until tomorrow.
func attachButtons(dialog *qt.QDialog, layout *qt.QVBoxLayout) {
	buttons := qt.NewQDialogButtonBox4(qt.QDialogButtonBox__Ok | qt.QDialogButtonBox__Cancel)
	buttons.Button(qt.QDialogButtonBox__Ok).SetDefault(true)
	skip := buttons.Button(qt.QDialogButtonBox__Cancel)
	skip.SetText("Skip Today")
	skip.SetToolTip("Don't ask again today")
	snooze := buttons.AddButton2("Remind me in 1h", qt.QDialogButtonBox__ActionRole)
	snooze.SetToolTip("Ask again in an hour")
	snooze.OnClicked(func() { dialog.Done(snoozeCode) })
	buttons.OnAccepted(dialog.Accept)
	buttons.OnRejected(dialog.Reject)
	layout.AddWidget(buttons.QWidget)
}

// newRowsArea wraps per-item rows in a scroll container so long plans don't
// grow a dialog past the screen.
func newRowsArea() (*qt.QScrollArea, *qt.QVBoxLayout) {
	container := qt.NewQWidget2()
	rows := qt.NewQVBoxLayout(container)
	area := qt.NewQScrollArea2()
	area.SetWidgetResizable(true)
	area.SetWidget(container)
	area.SetMaximumHeight(320)
	return area, rows
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
