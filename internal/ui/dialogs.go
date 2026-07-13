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
// active application. It is also pinned above every other window and onto the
// user's current Space/monitor (see pinDialogOnTop / pinAcrossSpaces) so it
// cannot be buried or missed while it waits for an answer.
func (s *dialogSpec) run() (dialogResult, error) {
	pinDialogOnTop(s.dialog) // before Show: window-flag change can need a re-show
	s.dialog.Show()
	s.dialog.Raise()
	s.dialog.ActivateWindow()
	pinAcrossSpaces(s.dialog) // after Show: needs the native window to exist
	switch s.dialog.Exec() {
	case int(qt.QDialog__Accepted):
		return dialogAccepted, s.apply()
	case snoozeCode:
		return dialogSnoozed, nil
	default:
		return dialogCanceled, nil
	}
}

// runWeeklySummaryDialog shows the summary dialog for week. When markOnAccept
// is true (scheduled path) accepting the dialog marks the week summarized;
// when false (manual / on-demand view) accepting is a read-only action and
// the summarized flag is left untouched so the scheduled Friday prompt still
// fires.
func (a *App) runWeeklySummaryDialog(week store.WeekID, markOnAccept bool) (dialogResult, error) {
	spec, err := a.buildWeeklySummaryDialog(week, markOnAccept)
	if err != nil {
		return dialogCanceled, err
	}
	return spec.run()
}

func (a *App) runMorningDialog(manual bool) (dialogResult, error) {
	spec, err := a.buildMorningDialog(time.Now(), manual)
	if err != nil {
		return dialogCanceled, err
	}
	return spec.run()
}

func (a *App) runEveningDialog(manual bool) (dialogResult, error) {
	spec, err := a.buildEveningDialog(time.Now(), manual)
	if err != nil {
		return dialogCanceled, err
	}
	return spec.run()
}

// runWeekReviewDialog shows the week-review dialog. Pass rollover=true for
// the scheduled path (Monday review: NextWeek items roll into Current before
// applying decisions) and rollover=false for on-demand manual re-triages.
func (a *App) runWeekReviewDialog(week store.WeekID, rollover bool) (dialogResult, error) {
	spec, err := a.buildWeekReviewDialog(week, rollover)
	if err != nil {
		return dialogCanceled, err
	}
	return spec.run()
}

// runWeeklyPlanDialog shows the weekly-plan dialog for week.
func (a *App) runWeeklyPlanDialog(week store.WeekID, manual bool) (dialogResult, error) {
	spec, err := a.buildWeeklyPlanDialog(week, manual)
	if err != nil {
		return dialogCanceled, err
	}
	return spec.run()
}

// buildWeeklyPlanDialog captures the week's "big things". Existing goals are
// shown with a Done / Not-done selector, so re-opening the dialog is how goals
// get ticked through the week; a text field adds more. manual is forwarded to
// attachButtons.
func (a *App) buildWeeklyPlanDialog(week store.WeekID, manual bool) (*dialogSpec, error) {
	goals, _, err := a.store.WeeklyPlan(week)
	if err != nil {
		return nil, err
	}

	dialog := qt.NewQDialog(a.window.win.QWidget)
	dialog.SetWindowTitle(fmt.Sprintf("Weekly Plan: %s", week))
	dialog.SetMinimumWidth(460)
	layout := qt.NewQVBoxLayout(dialog.QWidget)

	layout.AddWidget(qt.NewQLabel3("<b>What are the big things you want to get done this week?</b>").QWidget)

	selectors := make([]*stateSelector, len(goals))
	if len(goals) > 0 {
		layout.AddWidget(qt.NewQLabel3("This week's goals (tick off what's done):").QWidget)
		area, rows := newRowsArea()
		for i, g := range goals {
			row := qt.NewQHBoxLayout2()
			label := qt.NewQLabel3(g.Text)
			label.SetTextFormat(qt.PlainText)
			label.SetWordWrap(true)
			sel := newStateSelector(g.State)
			selectors[i] = sel
			row.AddWidget2(label.QWidget, 1)
			row.AddWidget(sel.widget)
			rows.AddLayout(row.QLayout)
		}
		layout.AddWidget(area.QWidget)
	}

	editor := qt.NewQPlainTextEdit2()
	editor.SetPlaceholderText("Add more big things, one per line…")
	editor.SetFocus()
	layout.AddWidget(editor.QWidget)
	attachButtons(dialog, layout, manual)

	apply := func() error {
		next := make([]store.Item, 0, len(goals))
		for i, g := range goals {
			state := g.State
			if selectors[i] != nil {
				state = store.ItemState(selectors[i].group.CheckedId())
			}
			next = append(next, store.Item{Text: g.Text, State: state})
		}
		for _, text := range splitLines(editor.ToPlainText()) {
			next = append(next, store.Item{Text: text, State: store.StateTodo})
		}
		return a.store.SetWeeklyPlan(week, next)
	}
	return &dialogSpec{dialog: dialog, apply: apply}, nil
}

// weeklyGoalsLabel renders a read-only rich-text label of the week's goals,
// done ones struck through with a check. Shared by the morning check-in and the
// weekly summary.
func weeklyGoalsLabel(title string, goals []store.Item) *qt.QLabel {
	var b strings.Builder
	fmt.Fprintf(&b, "<b>%s</b>", html.EscapeString(title))
	for _, g := range goals {
		if g.State == store.StateDone {
			fmt.Fprintf(&b, `<div style="color:#888888">&#10003; <s>%s</s></div>`, html.EscapeString(g.Text))
		} else {
			fmt.Fprintf(&b, "<div>&bull; %s</div>", html.EscapeString(g.Text))
		}
	}
	label := qt.NewQLabel3(b.String())
	label.SetTextFormat(qt.RichText)
	label.SetWordWrap(true)
	return label
}

// plannedTodayLabel returns a read-only summary of today's already-recorded plan
// (with the items in a tooltip), or nil when there is nothing planned yet.
func plannedTodayLabel(daily *store.Daily, exists bool) *qt.QLabel {
	if !exists || daily == nil || len(daily.Plan) == 0 {
		return nil
	}
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
	return summary
}

// buildMorningDialog asks what the user plans to work on today, offering
// carry-over candidates from earlier in the week and the backlog.
// manual mirrors runPrompt's manual flag and is forwarded to attachButtons.
func (a *App) buildMorningDialog(today time.Time, manual bool) (*dialogSpec, error) {
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

	// Show this week's big things (the weekly plan) read-only for reference, so
	// daily planning stays aligned with the week's intentions.
	if goals, planned, err := a.store.WeeklyPlan(store.WeekOf(today)); err != nil {
		return nil, err
	} else if planned && len(goals) > 0 {
		layout.AddWidget(weeklyGoalsLabel("This week's big things:", goals).QWidget)
	}

	// When today's plan already exists, show a read-only summary so the user
	// knows their earlier tasks are still there (they are not visible in the
	// editor because ApplyMorning deduplicates on accept).
	if summary := plannedTodayLabel(daily, dailyExists); summary != nil {
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
	attachButtons(dialog, layout, manual)

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
// manual mirrors runPrompt's manual flag and is forwarded to attachButtons.
func (a *App) buildEveningDialog(today time.Time, manual bool) (*dialogSpec, error) {
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

	selectors := make([]*choiceSelector, len(plan))
	if len(plan) > 0 {
		area, rows := newRowsArea()
		for i, item := range plan {
			row := qt.NewQHBoxLayout2()
			label := qt.NewQLabel3(item.Text)
			label.SetTextFormat(qt.PlainText)
			label.SetWordWrap(true)
			selector := newChoiceSelector(eveningChoices(), int(store.EveningActionForState(item.State)))
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

	layout.AddWidget(qt.NewQLabel3("Anything else you accomplished? (Items checked above are recorded automatically.)").QWidget)
	editor := qt.NewQPlainTextEdit2()
	editor.SetPlaceholderText("One accomplishment per line…")
	editor.SetFocus()
	layout.AddWidget(editor.QWidget)
	attachButtons(dialog, layout, manual)

	apply := func() error {
		decisions := make([]store.EveningDecision, len(plan))
		for i, item := range plan {
			decisions[i] = store.EveningDecision{
				Text:   item.Text,
				Action: store.EveningAction(selectors[i].group.CheckedId()),
			}
		}
		return a.store.ApplyEvening(today, decisions, splitLines(editor.ToPlainText()))
	}
	return &dialogSpec{dialog: dialog, apply: apply}, nil
}

// buildWeekReviewDialog triages the given week's leftover items.
// rollover controls whether NextWeek backlog items are promoted to Current
// before applying decisions: true for the scheduled Monday review, false for
// on-demand manual re-triages mid-week.
// manual is forwarded to attachButtons (Close vs Skip Today label).
func (a *App) buildWeekReviewDialog(week store.WeekID, rollover bool) (*dialogSpec, error) {
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
		// "Keep on backlog" is the honest label: the item stays in backlog
		// Current (not in today's plan), and will surface again at the next
		// morning check-in with the "(backlog)" suffix.
		{id: int(store.ReviewKeep), icon: qt.QStyle__SP_DialogApplyButton, tooltip: "Keep on backlog"},
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
	// rollover=true means scheduled Monday review; rollover=false means manual.
	attachButtons(dialog, layout, !rollover)

	apply := func() error {
		decisions := make([]store.ReviewDecision, len(items))
		for i, text := range items {
			decisions[i] = store.ReviewDecision{
				Text:   text,
				Action: store.ReviewAction(selectors[i].group.CheckedId()),
			}
		}
		return a.store.ApplyWeekReview(week, decisions, rollover)
	}
	return &dialogSpec{dialog: dialog, apply: apply}, nil
}

// buildWeeklySummaryDialog shows the current week's accomplishments for a
// quick Friday review and lets the user open the weekly file.
// markOnAccept controls whether accepting the dialog marks the week
// summarized: true for the scheduled Friday prompt, false for on-demand
// (manual) views so a mid-week peek does not consume the Friday summary.
func (a *App) buildWeeklySummaryDialog(week store.WeekID, markOnAccept bool) (*dialogSpec, error) {
	dailies, err := a.store.DailiesInWeek(week)
	if err != nil {
		return nil, err
	}

	dialog := qt.NewQDialog(a.window.win.QWidget)
	dialog.SetWindowTitle(fmt.Sprintf("Week Summary: %s", week))
	dialog.SetMinimumWidth(500)
	layout := qt.NewQVBoxLayout(dialog.QWidget)

	// Show the week's big things (the weekly plan) with their done/not state.
	if goals, planned, err := a.store.WeeklyPlan(week); err != nil {
		return nil, err
	} else if planned && len(goals) > 0 {
		layout.AddWidget(weeklyGoalsLabel("Big things this week:", goals).QWidget)
	}

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

	// markOnAccept=true is the scheduled path; markOnAccept=false is the manual view.
	attachButtons(dialog, layout, !markOnAccept)

	apply := func() error {
		if !markOnAccept {
			return nil // manual view: do not consume the scheduled summary
		}
		return a.store.MarkWeekSummarized(week)
	}
	return &dialogSpec{dialog: dialog, apply: apply}, nil
}

// attachButtons appends an OK / Postpone 1h / reject button box to the
// dialog. When manual is true (tray-invoked check-in) the reject button
// says "Close" with no tooltip and no skippedOn side effect; when false
// (scheduled prompt) it says "Skip Today" and silences the check-in until
// tomorrow. "Remind me in 1h" always snoozes.
func attachButtons(dialog *qt.QDialog, layout *qt.QVBoxLayout, manual bool) {
	buttons := qt.NewQDialogButtonBox4(qt.QDialogButtonBox__Ok | qt.QDialogButtonBox__Cancel)
	buttons.Button(qt.QDialogButtonBox__Ok).SetDefault(true)
	reject := buttons.Button(qt.QDialogButtonBox__Cancel)
	if manual {
		reject.SetText("Close")
	} else {
		reject.SetText("Skip Today")
		reject.SetToolTip("Don't ask again today")
	}
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
