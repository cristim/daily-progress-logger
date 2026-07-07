package ui

import (
	"fmt"
	"strings"
	"time"

	qt "github.com/mappu/miqt/qt6"

	"github.com/cristim/daily-progress-logger/internal/schedule"
	"github.com/cristim/daily-progress-logger/internal/store"
)

// mainWindow is the resident window showing today's plan.
type mainWindow struct {
	app        *App
	win        *qt.QMainWindow
	heading    *qt.QLabel
	planList   *qt.QListWidget
	newItem    *qt.QLineEdit
	refreshing bool // suppresses OnItemChanged while repopulating
}

func newMainWindow(app *App) *mainWindow {
	w := &mainWindow{app: app}
	w.win = qt.NewQMainWindow2()
	w.win.SetWindowTitle("Daily Progress Logger")
	w.win.Resize(520, 560)

	central := qt.NewQWidget2()
	layout := qt.NewQVBoxLayout(central)

	w.heading = qt.NewQLabel2()
	layout.AddWidget(w.heading.QWidget)

	w.planList = qt.NewQListWidget2()
	w.planList.OnItemChanged(w.onItemChanged)
	layout.AddWidget(w.planList.QWidget)

	addRow := qt.NewQHBoxLayout2()
	w.newItem = qt.NewQLineEdit2()
	w.newItem.SetPlaceholderText("Add a task for today…")
	w.newItem.OnReturnPressed(w.addItem)
	addButton := qt.NewQPushButton3("Add")
	addButton.OnClicked(w.addItem)
	addRow.AddWidget(w.newItem.QWidget)
	addRow.AddWidget(addButton.QWidget)
	layout.AddLayout(addRow.QLayout)

	itemActions := qt.NewQHBoxLayout2()
	postponeButton := qt.NewQPushButton3("Postpone to Next Week")
	postponeButton.OnClicked(w.postponeSelected)
	backlogButton := qt.NewQPushButton3("Move to Backlog")
	backlogButton.OnClicked(w.moveSelectedToBacklog)
	itemActions.AddWidget(postponeButton.QWidget)
	itemActions.AddWidget(backlogButton.QWidget)
	layout.AddLayout(itemActions.QLayout)

	checkIns := qt.NewQHBoxLayout2()
	morningButton := qt.NewQPushButton3("Morning Check-in…")
	morningButton.OnClicked(func() { w.app.runPrompt(schedule.PromptMorning) })
	eveningButton := qt.NewQPushButton3("Evening Check-in…")
	eveningButton.OnClicked(func() { w.app.runPrompt(schedule.PromptEvening) })
	checkIns.AddWidget(morningButton.QWidget)
	checkIns.AddWidget(eveningButton.QWidget)
	layout.AddLayout(checkIns.QLayout)

	w.win.SetCentralWidget(central)
	w.setUpMenu()

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
	addMenuAction(fileMenu, "Review Last Week…", w.app.runWeekReviewManually)
	fileMenu.AddSeparator()
	addMenuAction(fileMenu, "Quit", qt.QCoreApplication_Quit)
	w.win.SetMenuBar(menuBar)
}

func (w *mainWindow) openDataFolder() {
	url := qt.QUrl_FromLocalFile(w.app.store.DataDir)
	qt.QDesktopServices_OpenUrl(url)
}

// refresh reloads today's plan into the list.
func (w *mainWindow) refresh() {
	today := time.Now()
	daily, exists, err := w.app.store.LoadDaily(today)
	if err != nil {
		w.app.reportError(err)
		return
	}

	w.refreshing = true
	defer func() { w.refreshing = false }()

	w.heading.SetText(fmt.Sprintf("<b>%s, %d %s %d</b> &nbsp; (week %s)",
		today.Weekday(), today.Day(), today.Month(), today.Year(), store.WeekOf(today)))

	w.planList.Clear()
	if !exists {
		return
	}
	for _, item := range daily.Plan {
		listItem := qt.NewQListWidgetItem2(item.Text)
		switch item.State {
		case store.StatePostponed:
			listItem.SetText(item.Text + "  (postponed)")
			listItem.SetFlags(qt.ItemIsSelectable)
		case store.StateDone:
			listItem.SetFlags(qt.ItemIsSelectable | qt.ItemIsEnabled | qt.ItemIsUserCheckable)
			listItem.SetCheckState(qt.Checked)
		case store.StateTodo:
			listItem.SetFlags(qt.ItemIsSelectable | qt.ItemIsEnabled | qt.ItemIsUserCheckable)
			listItem.SetCheckState(qt.Unchecked)
		}
		w.planList.AddItemWithItem(listItem)
	}
}

// onItemChanged syncs a checkbox toggle back to the daily file.
func (w *mainWindow) onItemChanged(item *qt.QListWidgetItem) {
	if w.refreshing {
		return
	}
	row := w.planList.Row(item)
	state := store.StateTodo
	if item.CheckState() == qt.Checked {
		state = store.StateDone
	}
	if err := w.app.store.SetPlanItemState(time.Now(), row, state); err != nil {
		w.app.reportError(err)
	}
	w.refresh()
}

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

func (w *mainWindow) postponeSelected() {
	w.withSelectedRow(w.app.store.PostponePlanItem)
}

func (w *mainWindow) moveSelectedToBacklog() {
	w.withSelectedRow(w.app.store.MoveToBacklog)
}

func (w *mainWindow) withSelectedRow(op func(time.Time, int) error) {
	row := w.planList.CurrentRow()
	if row < 0 {
		return
	}
	if err := op(time.Now(), row); err != nil {
		w.app.reportError(err)
		return
	}
	w.refresh()
}

// trimmed collapses surrounding whitespace.
func trimmed(s string) string {
	return strings.TrimSpace(s)
}
