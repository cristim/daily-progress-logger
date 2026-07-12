package ui

import (
	"fmt"
	"strings"
	"time"

	qt "github.com/mappu/miqt/qt6"

	"github.com/cristim/daily-progress-logger/internal/recur"
	"github.com/cristim/daily-progress-logger/internal/schedule"
	"github.com/cristim/daily-progress-logger/internal/store"
)

// mainWindow is the resident window showing the Projects → Stories → tasks tree.
type mainWindow struct {
	app          *App
	win          *qt.QMainWindow
	heading      *qt.QLabel
	dateEdit     *qt.QDateEdit
	tree         *qt.QTreeWidget
	newItem      *qt.QLineEdit
	refreshTimer *qt.QTimer
	// quitAction is the File-menu Quit action; its shortcut is set from config
	// in App.applyShortcuts.
	quitAction *qt.QAction
	// viewedDate is the day whose tasks the tree currently shows (default today);
	// the date arrows / calendar change it.
	viewedDate time.Time
	// renderedDate records today's date (time.DateOnly) at the last refresh so
	// the midnight watchdog in CheckPrompts can detect a day rollover.
	renderedDate string
	// expanded remembers each node's expand state by node key across rebuilds;
	// a key absent from the map defaults to expanded.
	expanded map[string]bool
}

func newMainWindow(app *App) *mainWindow {
	w := &mainWindow{app: app, expanded: map[string]bool{}, viewedDate: midnight(time.Now())}
	w.win = qt.NewQMainWindow2()
	w.win.SetWindowTitle("Daily Progress Logger")
	w.win.Resize(620, 620)

	central := qt.NewQWidget2()
	layout := qt.NewQVBoxLayout(central)

	// Date navigation: prev/next day, a calendar picker, a Today reset, and the
	// ISO-week label.
	dateRow := qt.NewQHBoxLayout2()
	prevDay := qt.NewQPushButton3("◀")
	prevDay.SetToolTip("Previous day")
	prevDay.OnClicked(func() { w.shiftDay(-1) })
	w.dateEdit = qt.NewQDateEdit2()
	w.dateEdit.SetCalendarPopup(true)
	w.dateEdit.SetDisplayFormat("ddd d MMM yyyy")
	w.dateEdit.SetDate(*timeToQDate(w.viewedDate))
	w.dateEdit.OnDateChanged(func(date qt.QDate) { w.setViewedDate(dateToTime(&date)) })
	nextDay := qt.NewQPushButton3("▶")
	nextDay.SetToolTip("Next day")
	nextDay.OnClicked(func() { w.shiftDay(1) })
	todayBtn := qt.NewQPushButton3("Today")
	todayBtn.OnClicked(w.goToday)
	w.heading = qt.NewQLabel2()
	dateRow.AddWidget(prevDay.QWidget)
	dateRow.AddWidget(w.dateEdit.QWidget)
	dateRow.AddWidget(nextDay.QWidget)
	dateRow.AddWidget(todayBtn.QWidget)
	dateRow.AddWidget2(w.heading.QWidget, 1)
	layout.AddLayout(dateRow.QLayout)

	addRow := qt.NewQHBoxLayout2()
	w.newItem = qt.NewQLineEdit2()
	w.newItem.SetPlaceholderText(`Add a task for today…  (or "Standup @weekly @mon @9:00" to repeat)`)
	w.newItem.OnReturnPressed(w.addItem)
	addButton := qt.NewQPushButton3("Add")
	addButton.OnClicked(w.addItem)
	newProject := qt.NewQPushButton3("New Project…")
	newProject.OnClicked(w.addProject)
	addRow.AddWidget(w.newItem.QWidget)
	addRow.AddWidget(addButton.QWidget)
	addRow.AddWidget(newProject.QWidget)
	layout.AddLayout(addRow.QLayout)

	w.tree = qt.NewQTreeWidget2()
	w.tree.SetHeaderHidden(true)
	w.tree.SetColumnCount(1)
	w.tree.SetSelectionMode(qt.QAbstractItemView__SingleSelection)
	// Enable drag-drop so tasks can nest under other tasks (subtasks) or move
	// between projects; the drop is applied to the store in onDrop (see tree.go).
	// DragDrop mode (not InternalMove) plus explicit accept in drag-enter/move
	// lets a drop land *onto* a row (a project/task), which InternalMove's
	// "between items" bias would reject — so the drop event actually fires.
	w.tree.SetDragEnabled(true)
	w.tree.SetAcceptDrops(true)
	w.tree.SetDragDropMode(qt.QAbstractItemView__DragDrop)
	w.tree.SetDropIndicatorShown(true)
	w.tree.OnDragEnterEvent(func(_ func(event *qt.QDragEnterEvent), event *qt.QDragEnterEvent) {
		event.AcceptProposedAction()
	})
	w.tree.OnDragMoveEvent(func(_ func(event *qt.QDragMoveEvent), event *qt.QDragMoveEvent) {
		event.AcceptProposedAction()
	})
	w.tree.OnDropEvent(func(_ func(event *qt.QDropEvent), event *qt.QDropEvent) {
		w.onDrop(event)
	})
	w.tree.OnItemExpanded(func(item *qt.QTreeWidgetItem) { w.setExpanded(item, true) })
	w.tree.OnItemCollapsed(func(item *qt.QTreeWidgetItem) { w.setExpanded(item, false) })
	// Double-click edits a task's text or renames a project; single clicks on
	// the row's own widgets (checkbox, buttons) are unaffected since they are
	// handled by those child widgets directly.
	w.tree.OnItemDoubleClicked(func(item *qt.QTreeWidgetItem, _ int) {
		key := keyOf(item)
		switch {
		case strings.HasPrefix(key, "t:"):
			if date, index, ok := decodeTaskKey(key); ok {
				w.editTask(date, index, item.Data(0, taskTextRole).ToString())
			}
		case strings.HasPrefix(key, "p:"):
			w.renameProject(strings.TrimPrefix(key, "p:"), item.Data(0, taskTextRole).ToString())
		}
	})
	layout.AddWidget(w.tree.QWidget)

	checkIns := qt.NewQHBoxLayout2()
	morningButton := qt.NewQPushButton3("Morning Check-in…")
	morningButton.OnClicked(func() { w.app.runPrompt(schedule.PromptMorning, true) })
	eveningButton := qt.NewQPushButton3("Evening Check-in…")
	eveningButton.OnClicked(func() { w.app.runPrompt(schedule.PromptEvening, true) })
	backlogButton := qt.NewQPushButton3("Backlog…")
	backlogButton.OnClicked(func() { w.app.openBacklogDialog() })
	checkIns.AddWidget(morningButton.QWidget)
	checkIns.AddWidget(eveningButton.QWidget)
	checkIns.AddWidget(backlogButton.QWidget)
	layout.AddLayout(checkIns.QLayout)

	w.win.SetCentralWidget(central)
	w.setUpMenu()

	// Row-button handlers rebuild the tree; doing that while the clicked
	// button's signal is still being delivered would destroy the sender, so
	// refreshes triggered from rows are deferred to the event loop.
	w.refreshTimer = qt.NewQTimer2(w.win.QObject)
	w.refreshTimer.SetSingleShot(true)
	w.refreshTimer.OnTimeout(w.refresh)

	// Closing the window hides it; the app stays in the menu bar.
	w.win.OnCloseEvent(func(_ func(event *qt.QCloseEvent), event *qt.QCloseEvent) {
		event.Ignore()
		w.win.Hide()
	})

	return w
}

func (w *mainWindow) setUpMenu() {
	menuBar := qt.NewQMenuBar2()
	fileMenu := menuBar.AddMenuWithTitle("File")
	addMenuAction(fileMenu, "Open Data Folder", w.openDataFolder)
	addMenuAction(fileMenu, "Weekly Plan…", w.app.runWeeklyPlanManually)
	addMenuAction(fileMenu, "Backlog…", func() { w.app.openBacklogDialog() })
	addMenuAction(fileMenu, "This Week's Summary…", w.app.runWeeklySummaryManually)
	addMenuAction(fileMenu, "Review Last Week…", w.app.runWeekReviewManually)
	fileMenu.AddSeparator()
	prefs := addMenuAction(fileMenu, "Preferences…", w.app.openPreferencesDialog)
	prefs.SetShortcut(qt.NewQKeySequence6(qt.QKeySequence__Preferences))
	fileMenu.AddSeparator()
	// The Quit shortcut is bound from config in App.applyShortcuts.
	w.quitAction = addMenuAction(fileMenu, "Quit", qt.QCoreApplication_Quit)
	w.win.SetMenuBar(menuBar)
}

func (w *mainWindow) openDataFolder() {
	url := qt.QUrl_FromLocalFile(w.app.store.DataDir)
	qt.QDesktopServices_OpenUrl(url)
}

// refresh reloads the tree for the viewed day.
func (w *mainWindow) refresh() {
	now := time.Now()
	w.renderedDate = now.Format(time.DateOnly)
	w.heading.SetText(fmt.Sprintf("(week %s)", store.WeekOf(w.viewedDate)))
	if sameDay(w.viewedDate, now) {
		w.newItem.SetPlaceholderText(`Add a task for today…  (or "Standup @weekly @mon @9:00" to repeat)`)
	} else {
		w.newItem.SetPlaceholderText(fmt.Sprintf("Add a task for %s…", w.viewedDate.Format("Mon 2 Jan")))
	}

	tree, err := w.app.store.BuildProjectTree(w.viewedDate)
	if err != nil {
		w.app.reportError(err)
		return
	}
	w.buildTree(tree)
}

// setViewedDate switches the tree to date (deferred rebuild). No-op when already
// viewing that day.
func (w *mainWindow) setViewedDate(date time.Time) {
	date = midnight(date)
	if sameDay(date, w.viewedDate) {
		return
	}
	w.viewedDate = date
	w.scheduleRefresh()
}

// shiftDay moves the viewed day by delta days (via the date editor, whose
// dateChanged signal drives the refresh).
func (w *mainWindow) shiftDay(delta int) {
	w.dateEdit.SetDate(*timeToQDate(w.viewedDate.AddDate(0, 0, delta)))
}

// goToday jumps back to the current day.
func (w *mainWindow) goToday() {
	w.dateEdit.SetDate(*timeToQDate(time.Now()))
}

// midnight returns t at 00:00 local time.
func midnight(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local)
}

// sameDay reports whether a and b fall on the same calendar day.
func sameDay(a, b time.Time) bool {
	return a.Year() == b.Year() && a.YearDay() == b.YearDay()
}

func timeToQDate(t time.Time) *qt.QDate {
	return qt.NewQDate2(t.Year(), int(t.Month()), t.Day())
}

func dateToTime(d *qt.QDate) time.Time {
	return time.Date(d.Year(), time.Month(d.Month()), d.Day(), 0, 0, 0, 0, time.Local)
}

// scheduleRefresh rebuilds the tree on the next event-loop pass; safe to call
// from handlers of widgets the rebuild will destroy.
func (w *mainWindow) scheduleRefresh() {
	w.refreshTimer.Start(0)
}

// runTaskAction applies action to the plan item at index on date, then
// refreshes. The tree is rebuilt from scratch on every refresh, so index
// (captured from the tree row at build time) is always current relative to
// that day's plan.
func (w *mainWindow) runTaskAction(date time.Time, index int, action taskFunc) {
	if err := action(date, index); err != nil {
		w.app.reportError(err)
	}
	w.scheduleRefresh()
}

// addItem adds a new task. Text carrying a recurrence tag (@daily/@weekly/…)
// becomes a recurring template; otherwise it is a one-off untagged top-level
// task on the viewed day's plan (landing under Unfiled until assigned to a
// project).
func (w *mainWindow) addItem() {
	text := trimmed(w.newItem.Text())
	if text == "" {
		return
	}
	add := func() error { return w.app.store.AddPlanItem(w.viewedDate, text) }
	// Detection only needs the recurrence keyword; AddRecurring resolves
	// project tags with the real known-ID predicate.
	if _, _, ok := recur.Parse(text, w.app.morning.Hour, w.app.morning.Minute, nil); ok {
		add = func() error { return w.app.store.AddRecurring(text) }
	}
	if err := add(); err != nil {
		w.app.reportError(err)
		return
	}
	w.newItem.Clear()
	w.refresh()
}

// addProject prompts for a name and creates a new project.
func (w *mainWindow) addProject() {
	if name, ok := w.promptText("New Project", "Project name:", ""); ok {
		if _, err := w.app.store.AddProject(name); err != nil {
			w.app.reportError(err)
		}
		w.refresh()
	}
}

// promptText shows a single-line input dialog, returning the entered text and
// whether the user accepted with a non-empty value.
func (w *mainWindow) promptText(title, label, initial string) (string, bool) {
	ok := false
	text := qt.QInputDialog_GetText4(w.win.QWidget, title, label, qt.QLineEdit__Normal, initial, &ok)
	text = trimmed(text)
	return text, ok && text != ""
}

// trimmed collapses surrounding whitespace.
func trimmed(s string) string {
	return strings.TrimSpace(s)
}
