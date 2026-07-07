// Package ui implements the Qt desktop interface: a resident main window
// with a menu-bar (system tray) icon, and the morning / evening / week
// review check-in dialogs.
package ui

import (
	"fmt"
	"log/slog"
	"time"

	qt "github.com/mappu/miqt/qt6"

	"github.com/cristim/daily-progress-logger/internal/config"
	"github.com/cristim/daily-progress-logger/internal/schedule"
	"github.com/cristim/daily-progress-logger/internal/store"
)

// checkInterval is how often the app re-evaluates which prompts are due.
const checkInterval = 60 * time.Second

// App owns the Qt widgets and drives the check-in prompts.
type App struct {
	store   *store.Store
	cfg     *config.Config
	morning schedule.TimeOfDay
	evening schedule.TimeOfDay

	window     *mainWindow
	tray       *qt.QSystemTrayIcon
	timer      *qt.QTimer
	dialogOpen bool
}

// New builds the application UI. The Qt application object must already
// exist.
func New(st *store.Store, cfg *config.Config) (*App, error) {
	morningHour, morningMinute, err := config.ParseTimeOfDay(cfg.MorningTime)
	if err != nil {
		return nil, err
	}
	eveningHour, eveningMinute, err := config.ParseTimeOfDay(cfg.EveningTime)
	if err != nil {
		return nil, err
	}
	app := &App{
		store:   st,
		cfg:     cfg,
		morning: schedule.TimeOfDay{Hour: morningHour, Minute: morningMinute},
		evening: schedule.TimeOfDay{Hour: eveningHour, Minute: eveningMinute},
	}
	app.window = newMainWindow(app)
	app.setUpTray()

	// Closing the window keeps the app resident in the menu bar.
	qt.QGuiApplication_SetQuitOnLastWindowClosed(false)

	app.timer = qt.NewQTimer2(app.window.win.QObject)
	app.timer.OnTimeout(app.CheckPrompts)
	app.timer.Start(int(checkInterval.Milliseconds()))
	return app, nil
}

// Show displays the main window.
func (a *App) Show() {
	a.window.refresh()
	a.window.win.Show()
}

func (a *App) setUpTray() {
	if !qt.QSystemTrayIcon_IsSystemTrayAvailable() {
		slog.Warn("system tray unavailable; running with window only")
		return
	}
	a.tray = qt.NewQSystemTrayIcon2(trayIcon())

	menu := qt.NewQMenu2()
	addMenuAction(menu, "Show Window", func() { a.Show() })
	menu.AddSeparator()
	addMenuAction(menu, "Morning Check-in…", func() { a.runPrompt(schedule.PromptMorning) })
	addMenuAction(menu, "Evening Check-in…", func() { a.runPrompt(schedule.PromptEvening) })
	addMenuAction(menu, "Review Last Week…", a.runWeekReviewManually)
	menu.AddSeparator()
	addMenuAction(menu, "Quit", qt.QCoreApplication_Quit)

	a.tray.SetContextMenu(menu)
	a.tray.SetToolTip("Daily Progress Logger")
	a.tray.Show()
}

// CheckPrompts shows every check-in that is currently due. Called at
// startup and every minute by the timer.
func (a *App) CheckPrompts() {
	if a.dialogOpen {
		return
	}
	now := time.Now()
	state, err := a.scheduleState(now)
	if err != nil {
		a.reportError(err)
		return
	}
	for _, prompt := range schedule.Due(now, a.morning, a.evening, state) {
		a.runPrompt(prompt)
	}
}

func (a *App) scheduleState(now time.Time) (schedule.State, error) {
	var st schedule.State
	daily, exists, err := a.store.LoadDaily(now)
	if err != nil {
		return st, err
	}
	if exists {
		st.MorningDone = daily.MorningDone
		st.EveningDone = daily.EveningDone
	}
	_, pending, err := a.store.UnreviewedWeek(now)
	if err != nil {
		return st, err
	}
	st.WeekReviewPending = pending
	return st, nil
}

// runPrompt opens the dialog for prompt, guarding against overlapping
// dialogs (the timer keeps firing while a modal dialog runs its own event
// loop).
func (a *App) runPrompt(prompt schedule.Prompt) {
	if a.dialogOpen {
		return
	}
	a.dialogOpen = true
	defer func() {
		a.dialogOpen = false
		a.window.refresh()
	}()

	var err error
	switch prompt {
	case schedule.PromptMorning:
		err = a.runMorningDialog()
	case schedule.PromptEvening:
		err = a.runEveningDialog()
	case schedule.PromptWeekReview:
		var pending bool
		var week store.WeekID
		week, pending, err = a.store.UnreviewedWeek(time.Now())
		if err == nil && pending {
			err = a.runWeekReviewDialog(week)
		}
	}
	if err != nil {
		a.reportError(err)
	}
}

// runWeekReviewManually reviews the most recent past week even if it was
// already marked reviewed, so the user can re-triage the backlog on demand.
func (a *App) runWeekReviewManually() {
	if a.dialogOpen {
		return
	}
	a.dialogOpen = true
	defer func() {
		a.dialogOpen = false
		a.window.refresh()
	}()
	week := store.WeekOf(time.Now().AddDate(0, 0, -7))
	if err := a.runWeekReviewDialog(week); err != nil {
		a.reportError(err)
	}
}

// ForcePrompt runs the named check-in immediately, regardless of schedule:
// "morning", "evening" or "review".
func (a *App) ForcePrompt(name string) error {
	switch name {
	case "morning":
		a.runPrompt(schedule.PromptMorning)
	case "evening":
		a.runPrompt(schedule.PromptEvening)
	case "review":
		a.runWeekReviewManually()
	default:
		return fmt.Errorf("unknown check-in %q, want morning, evening or review", name)
	}
	return nil
}

// GrabScreenshots renders the main window and every check-in dialog
// offscreen into PNG files under dir, for headless UI verification.
func (a *App) GrabScreenshots(dir string) error {
	now := time.Now()
	a.window.refresh()

	morning, err := a.buildMorningDialog(now)
	if err != nil {
		return err
	}
	evening, err := a.buildEveningDialog(now)
	if err != nil {
		return err
	}
	review, err := a.buildWeekReviewDialog(store.WeekOf(now.AddDate(0, 0, -7)))
	if err != nil {
		return err
	}

	for name, widget := range map[string]*qt.QWidget{
		"main-window": a.window.win.QWidget,
		"morning":     morning.dialog.QWidget,
		"evening":     evening.dialog.QWidget,
		"week-review": review.dialog.QWidget,
	} {
		path := dir + "/" + name + ".png"
		if !widget.Grab().Save(path) {
			return fmt.Errorf("saving screenshot %s failed", path)
		}
		slog.Info("screenshot saved", "path", path)
	}
	return nil
}

func (a *App) reportError(err error) {
	slog.Error("ui error", "error", err)
	qt.QMessageBox_Critical2(a.window.win.QWidget, "Daily Progress Logger", err.Error(),
		qt.QMessageBox__Ok, qt.QMessageBox__NoButton)
}

// addMenuAction creates a triggered action on menu.
func addMenuAction(menu *qt.QMenu, text string, handler func()) {
	action := qt.NewQAction2(text)
	action.OnTriggered(handler)
	menu.AddAction(action)
}

// trayIcon draws a simple filled circle usable as a menu-bar icon.
func trayIcon() *qt.QIcon {
	const size = 22
	pixmap := qt.NewQPixmap2(size, size)
	pixmap.FillWithFillColor(qt.NewQColor11(0, 0, 0, 0))
	painter := qt.NewQPainter2(pixmap.QPaintDevice)
	painter.SetRenderHint(qt.QPainter__Antialiasing)
	painter.SetPenWithStyle(qt.NoPen)
	painter.SetBrush(qt.NewQBrush3(qt.NewQColor3(52, 120, 246)))
	painter.DrawEllipse2(3, 3, size-6, size-6)
	painter.SetBrush(qt.NewQBrush3(qt.NewQColor3(255, 255, 255)))
	painter.DrawEllipse2(8, 8, size-16, size-16)
	painter.End()
	return qt.NewQIcon2(pixmap)
}
