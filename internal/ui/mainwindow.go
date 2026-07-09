package ui

import (
	"fmt"
	"strings"
	"time"

	qt "github.com/mappu/miqt/qt6"

	"github.com/cristim/daily-progress-logger/internal/schedule"
	"github.com/cristim/daily-progress-logger/internal/store"
)

// mainWindow is the resident window showing the Projects → Stories → tasks tree.
type mainWindow struct {
	app          *App
	win          *qt.QMainWindow
	heading      *qt.QLabel
	tree         *qt.QTreeWidget
	newItem      *qt.QLineEdit
	refreshTimer *qt.QTimer
	// quitAction is the File-menu Quit action; its shortcut is set from config
	// in App.applyShortcuts.
	quitAction *qt.QAction
	// renderedDate records the date (time.DateOnly) of the last refresh so the
	// midnight watchdog in CheckPrompts can detect a day rollover and refresh.
	renderedDate string
	// expanded remembers each node's expand state by node key across rebuilds;
	// a key absent from the map defaults to expanded.
	expanded map[string]bool
}

func newMainWindow(app *App) *mainWindow {
	w := &mainWindow{app: app, expanded: map[string]bool{}}
	w.win = qt.NewQMainWindow2()
	w.win.SetWindowTitle("Daily Progress Logger")
	w.win.Resize(620, 620)

	central := qt.NewQWidget2()
	layout := qt.NewQVBoxLayout(central)

	w.heading = qt.NewQLabel2()
	layout.AddWidget(w.heading.QWidget)

	addRow := qt.NewQHBoxLayout2()
	w.newItem = qt.NewQLineEdit2()
	w.newItem.SetPlaceholderText("Add a task for today…")
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
	// Enable drag-drop so tasks can move between stories and stories between
	// projects; the drop is applied to the store in onDrop (see tree.go).
	w.tree.SetDragEnabled(true)
	w.tree.SetAcceptDrops(true)
	w.tree.SetDragDropMode(qt.QAbstractItemView__InternalMove)
	w.tree.OnDropEvent(func(_ func(event *qt.QDropEvent), event *qt.QDropEvent) {
		w.onDrop(event)
	})
	w.tree.OnItemExpanded(func(item *qt.QTreeWidgetItem) { w.setExpanded(item, true) })
	w.tree.OnItemCollapsed(func(item *qt.QTreeWidgetItem) { w.setExpanded(item, false) })
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

// refresh reloads the Projects/Stories/tasks tree.
func (w *mainWindow) refresh() {
	today := time.Now()
	w.renderedDate = today.Format(time.DateOnly)
	w.heading.SetText(fmt.Sprintf("<b>%s, %d %s %d</b> &nbsp; (week %s)",
		today.Weekday(), today.Day(), today.Month(), today.Year(), store.WeekOf(today)))

	tree, err := w.app.store.BuildProjectTree()
	if err != nil {
		w.app.reportError(err)
		return
	}
	w.buildTree(tree)
}

// scheduleRefresh rebuilds the tree on the next event-loop pass; safe to call
// from handlers of widgets the rebuild will destroy.
func (w *mainWindow) scheduleRefresh() {
	w.refreshTimer.Start(0)
}

// runTaskAction resolves a tree task (identified by its day and display text) to
// its plan index on that day and applies action, then refreshes. A task removed
// while the window was open just triggers a refresh.
func (w *mainWindow) runTaskAction(date time.Time, text string, action taskFunc) {
	idx, err := w.app.store.FindTaskIndex(date, text)
	if err != nil {
		w.app.reportError(err)
		w.scheduleRefresh()
		return
	}
	if idx < 0 {
		w.scheduleRefresh()
		return
	}
	if err := action(date, idx); err != nil {
		w.app.reportError(err)
	}
	w.scheduleRefresh()
}

// addItem adds a new untagged task to today's plan (it lands under Unfiled until
// assigned to a story).
func (w *mainWindow) addItem() {
	text := trimmed(w.newItem.Text())
	if text == "" {
		return
	}
	if err := w.app.store.AddPlanItem(time.Now(), text); err != nil {
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
