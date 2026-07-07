package ui

import (
	"fmt"
	"html"
	"strings"
	"time"

	qt "github.com/mappu/miqt/qt6"

	"github.com/cristim/daily-progress-logger/internal/schedule"
	"github.com/cristim/daily-progress-logger/internal/store"
)

// mainWindow is the resident window showing today's plan.
type mainWindow struct {
	app          *App
	win          *qt.QMainWindow
	heading      *qt.QLabel
	planList     *qt.QListWidget
	newItem      *qt.QLineEdit
	refreshTimer *qt.QTimer
}

func newMainWindow(app *App) *mainWindow {
	w := &mainWindow{app: app}
	w.win = qt.NewQMainWindow2()
	w.win.SetWindowTitle("Daily Progress Logger")
	w.win.Resize(560, 560)

	central := qt.NewQWidget2()
	layout := qt.NewQVBoxLayout(central)

	w.heading = qt.NewQLabel2()
	layout.AddWidget(w.heading.QWidget)

	w.planList = qt.NewQListWidget2()
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

	// Row-button handlers rebuild the list; doing that while the clicked
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
	addMenuAction(fileMenu, "Review Last Week…", w.app.runWeekReviewManually)
	fileMenu.AddSeparator()
	quit := addMenuAction(fileMenu, "Quit", qt.QCoreApplication_Quit)
	quit.SetShortcut(qt.NewQKeySequence6(qt.QKeySequence__Quit))
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

	w.heading.SetText(fmt.Sprintf("<b>%s, %d %s %d</b> &nbsp; (week %s)",
		today.Weekday(), today.Day(), today.Month(), today.Year(), store.WeekOf(today)))

	w.planList.Clear()
	if !exists || len(daily.Plan) == 0 {
		placeholder := qt.NewQListWidgetItem2("No plan for today yet. Run the Morning Check-in below, or add a task.")
		placeholder.SetFlags(qt.ItemFlag(0)) // informational, not selectable
		w.planList.AddItemWithItem(placeholder)
		return
	}
	for i, item := range daily.Plan {
		row := w.buildPlanRow(i, item)
		listItem := qt.NewQListWidgetItem()
		listItem.SetSizeHint(row.SizeHint())
		w.planList.AddItemWithItem(listItem)
		w.planList.SetItemWidget(listItem, row)
	}
}

// scheduleRefresh rebuilds the list on the next event-loop pass; safe to
// call from handlers of widgets the rebuild will destroy.
func (w *mainWindow) scheduleRefresh() {
	w.refreshTimer.Start(0)
}

// buildPlanRow renders one plan item: its text, the Done / Not done /
// Postpone selector, and a move-to-backlog button.
func (w *mainWindow) buildPlanRow(index int, item store.Item) *qt.QWidget {
	row := qt.NewQWidget2()
	layout := qt.NewQHBoxLayout(row)
	layout.SetContentsMargins(6, 2, 6, 2)

	// Make the item's state readable at a glance: done items are struck
	// through and dimmed, postponed ones dimmed.
	labelText := item.Text
	switch item.State {
	case store.StateDone:
		labelText = fmt.Sprintf(`<s style="color:#888888">%s</s>`, html.EscapeString(item.Text))
	case store.StatePostponed:
		labelText = fmt.Sprintf(`<span style="color:#888888">%s</span>`, html.EscapeString(item.Text))
	case store.StateTodo:
		// Plain text.
	}
	label := qt.NewQLabel3(labelText)

	selector := newStateSelector(item.State)
	selector.onChanged(func(state store.ItemState) {
		var err error
		if state == store.StatePostponed {
			err = w.app.store.PostponePlanItem(time.Now(), index)
		} else {
			err = w.app.store.SetPlanItemState(time.Now(), index, state)
		}
		if err != nil {
			w.app.reportError(err)
		}
		w.scheduleRefresh()
	})

	backlog := qt.NewQToolButton2()
	backlog.SetText("Backlog")
	backlog.SetIcon(standardIcon(qt.QStyle__SP_ArrowUp))
	backlog.SetToolButtonStyle(qt.ToolButtonTextBesideIcon)
	backlog.SetToolTip("Move to the cross-week backlog")
	backlog.OnClicked(func() {
		if err := w.app.store.MoveToBacklog(time.Now(), index); err != nil {
			w.app.reportError(err)
		}
		w.scheduleRefresh()
	})

	layout.AddWidget2(label.QWidget, 1)
	layout.AddWidget(selector.widget)
	layout.AddWidget(backlog.QWidget)
	return row
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

// trimmed collapses surrounding whitespace.
func trimmed(s string) string {
	return strings.TrimSpace(s)
}
