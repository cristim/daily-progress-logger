// Package ui implements the Qt desktop interface: a resident main window
// with a menu-bar (system tray) icon, and the morning / evening / week
// review check-in dialogs.
package ui

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"time"

	qt "github.com/mappu/miqt/qt6"
	"github.com/mappu/miqt/qt6/mainthread"

	"github.com/cristim/daily-progress-logger/internal/config"
	"github.com/cristim/daily-progress-logger/internal/loginitem"
	"github.com/cristim/daily-progress-logger/internal/schedule"
	"github.com/cristim/daily-progress-logger/internal/store"
	"github.com/cristim/daily-progress-logger/internal/update"
)

// checkInterval is how often the app re-evaluates which prompts are due.
const checkInterval = 60 * time.Second

// updateCheckInterval is how often the app re-checks for a new release.
const updateCheckInterval = 24 * time.Hour

// App owns the Qt widgets and drives the check-in prompts.
type App struct {
	store      *store.Store
	cfg        *config.Config
	morning    schedule.TimeOfDay
	evening    schedule.TimeOfDay
	summaryDay time.Weekday
	summary    schedule.TimeOfDay
	version    string

	window      *mainWindow
	tray        *qt.QSystemTrayIcon
	timer       *qt.QTimer
	updateTimer *qt.QTimer
	dialogOpen  bool

	// snoozeUntil holds "Postpone 1h" deadlines per prompt.
	snoozeUntil map[schedule.Prompt]time.Time
	// skippedOn records the day (time.DateOnly) a prompt was canceled, so
	// it stays quiet until the next day (or app restart).
	skippedOn map[schedule.Prompt]string
	// forced prompts were requested explicitly (-checkin flag) and are kept
	// pending even when the schedule alone would not show them.
	forced map[schedule.Prompt]bool
	// oneshot mode (cron/launchd): quit once nothing is pending.
	oneshot bool
}

// New builds the application UI. The Qt application object must already
// exist. appVersion is the running binary's version string (e.g. "0.1.0" or
// "dev") and is used by the auto-updater.
func New(st *store.Store, cfg *config.Config, appVersion string) (*App, error) {
	morningHour, morningMinute, err := config.ParseTimeOfDay(cfg.MorningTime)
	if err != nil {
		return nil, err
	}
	eveningHour, eveningMinute, err := config.ParseTimeOfDay(cfg.EveningTime)
	if err != nil {
		return nil, err
	}
	summaryHour, summaryMinute, err := config.ParseTimeOfDay(cfg.SummaryTime)
	if err != nil {
		return nil, err
	}
	summaryDay, err := config.ParseDay(cfg.SummaryDay)
	if err != nil {
		return nil, err
	}
	app := &App{
		store:       st,
		cfg:         cfg,
		morning:     schedule.TimeOfDay{Hour: morningHour, Minute: morningMinute},
		evening:     schedule.TimeOfDay{Hour: eveningHour, Minute: eveningMinute},
		summaryDay:  summaryDay,
		summary:     schedule.TimeOfDay{Hour: summaryHour, Minute: summaryMinute},
		version:     appVersion,
		snoozeUntil: map[schedule.Prompt]time.Time{},
		skippedOn:   map[schedule.Prompt]string{},
		forced:      map[schedule.Prompt]bool{},
	}
	app.window = newMainWindow(app)
	app.setUpTray()

	// Closing the window keeps the app resident in the menu bar.
	qt.QGuiApplication_SetQuitOnLastWindowClosed(false)

	app.timer = qt.NewQTimer2(app.window.win.QObject)
	app.timer.OnTimeout(app.CheckPrompts)
	app.timer.Start(int(checkInterval.Milliseconds()))

	// Schedule the first automatic update check 2 minutes after launch, then
	// every 24 hours. The check runs in a goroutine; results are marshalled
	// back to the Qt main thread via mainthread.Start.
	firstCheck := qt.NewQTimer2(app.window.win.QObject)
	firstCheck.SetSingleShot(true)
	firstCheck.OnTimeout(func() {
		app.checkForUpdatesBackground(false)
		// Arm the recurring 24-hour timer.
		app.updateTimer = qt.NewQTimer2(app.window.win.QObject)
		app.updateTimer.OnTimeout(func() { app.checkForUpdatesBackground(false) })
		app.updateTimer.Start(int(updateCheckInterval.Milliseconds()))
	})
	firstCheck.Start(int((2 * time.Minute).Milliseconds()))

	return app, nil
}

// Show displays the main window.
func (a *App) Show() {
	a.window.refresh()
	a.window.win.Show()
}

// MaybeOfferLoginItem shows a one-time "start at login?" dialog when the
// conditions are met (plist absent, not yet offered, not oneshot mode).
// Call this after Show() so the window is visible behind the dialog.
func (a *App) MaybeOfferLoginItem() {
	plistPath, err := loginitem.PlistPath()
	if err != nil {
		slog.Debug("loginitem: could not determine plist path", "error", err)
		return
	}
	if !loginitem.ShouldOffer(loginitem.Exists(plistPath), a.cfg.LoginItemOffered, a.oneshot) {
		return
	}

	// Mark offered before showing the dialog so a crash cannot re-show it.
	a.cfg.LoginItemOffered = true
	if saveErr := a.cfg.Save(); saveErr != nil {
		slog.Warn("loginitem: could not save config", "error", saveErr)
	}

	const question = "Start Daily Progress Logger at login?\n" +
		"It will run quietly in the menu bar."
	answer := qt.QMessageBox_Question2(
		a.window.win.QWidget,
		"Daily Progress Logger",
		question,
		qt.QMessageBox__Yes,
		qt.QMessageBox__No,
	)
	if answer != int(qt.QMessageBox__Yes) {
		return
	}

	exe, err := os.Executable()
	if err != nil {
		a.reportError(fmt.Errorf("locating executable for login item: %w", err))
		return
	}
	content := loginitem.RenderPlist(loginitem.BundleID, exe)
	if err := loginitem.Install(plistPath, content); err != nil {
		a.reportError(fmt.Errorf("installing login item: %w", err))
	}
}

// HandleReopen installs an event handler on qapp so that clicking the Dock
// icon while the main window is hidden brings it back to the front.
// Qt delivers QEvent::ApplicationActivate (type 121) when the application
// becomes active, including on a Dock-icon click on macOS.
func (a *App) HandleReopen(qapp *qt.QApplication) {
	qapp.OnEvent(func(super func(*qt.QEvent) bool, e *qt.QEvent) bool {
		if e.Type() == qt.QEvent__ApplicationActivate && !a.window.win.IsVisible() {
			a.Show()
		}
		return super(e)
	})
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
	addMenuAction(menu, "Morning Check-in…", func() { a.runPrompt(schedule.PromptMorning, true) })
	addMenuAction(menu, "Evening Check-in…", func() { a.runPrompt(schedule.PromptEvening, true) })
	addMenuAction(menu, "This Week's Summary…", a.runWeeklySummaryManually)
	addMenuAction(menu, "Review Last Week…", a.runWeekReviewManually)
	addMenuAction(menu, "Backlog…", a.openBacklogDialog)
	menu.AddSeparator()
	addMenuAction(menu, "Check for Updates…", a.checkForUpdatesManual)
	menu.AddSeparator()
	addMenuAction(menu, "Quit", qt.QCoreApplication_Quit)

	a.tray.SetContextMenu(menu)
	a.tray.SetToolTip("Daily Progress Logger")
	a.tray.Show()
}

// SetOneshot puts the app in cron mode: it quits as soon as no check-in is
// pending (snoozed prompts keep it alive until resolved or skipped).
func (a *App) SetOneshot() {
	a.oneshot = true
}

// OneshotPending reports whether a oneshot run still has unresolved
// check-ins (due or snoozed, and not skipped for today).
func (a *App) OneshotPending() bool {
	pending, err := a.anythingPending(time.Now())
	if err != nil {
		a.reportError(err)
		return false
	}
	return pending
}

// CheckPrompts shows every check-in that is currently due and neither
// snoozed nor skipped for today. Called at startup and every minute by the
// timer.
func (a *App) CheckPrompts() {
	if a.dialogOpen {
		return
	}
	now := time.Now()
	due, err := a.duePrompts(now)
	if err != nil {
		a.reportError(err)
		return
	}
	show, _ := schedule.Filter(due, now, a.snoozeUntil, a.skippedOn)
	for _, prompt := range show {
		a.runPrompt(prompt, false)
	}
	a.maybeQuitOneshot()
}

// duePrompts combines the schedule's due prompts with explicitly forced
// ones.
func (a *App) duePrompts(now time.Time) ([]schedule.Prompt, error) {
	state, err := a.scheduleState(now)
	if err != nil {
		return nil, err
	}
	due := schedule.Due(now, a.morning, a.evening, state, a.summaryDay, a.summary)
	for prompt := range a.forced {
		if !slices.Contains(due, prompt) {
			due = append(due, prompt)
		}
	}
	return due, nil
}

func (a *App) anythingPending(now time.Time) (bool, error) {
	due, err := a.duePrompts(now)
	if err != nil {
		return false, err
	}
	_, pending := schedule.Filter(due, now, a.snoozeUntil, a.skippedOn)
	return pending, nil
}

// maybeQuitOneshot ends a cron-mode run once every check-in is resolved.
func (a *App) maybeQuitOneshot() {
	if !a.oneshot || a.dialogOpen {
		return
	}
	pending, err := a.anythingPending(time.Now())
	if err != nil || pending {
		return
	}
	qt.QCoreApplication_Quit()
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
	_, summaryPending, err := a.store.WeekSummaryPending(now)
	if err != nil {
		return st, err
	}
	st.SummaryPending = summaryPending
	return st, nil
}

// runPrompt opens the dialog for prompt, guarding against overlapping
// dialogs (the timer keeps firing while a modal dialog runs its own event
// loop), and records the user's snooze/skip choice.
// For scheduled invocations (timer/CheckPrompts) use manual=false.
// For user-initiated invocations (tray menu, -checkin flag) use manual=true:
// cancel just closes without setting skippedOn, and snooze keeps the prompt
// forced so it re-fires after the snooze window even when not otherwise due.
func (a *App) runPrompt(prompt schedule.Prompt, manual bool) {
	if a.dialogOpen {
		return
	}
	a.dialogOpen = true
	defer func() {
		a.dialogOpen = false
		a.window.refresh()
		a.maybeQuitOneshot()
	}()

	result := dialogAccepted
	var err error
	switch prompt {
	case schedule.PromptMorning:
		result, err = a.runMorningDialog()
	case schedule.PromptEvening:
		result, err = a.runEveningDialog()
	case schedule.PromptWeekReview:
		result, err = a.runWeekReviewLoop()
	case schedule.PromptWeeklySummary:
		result, err = a.runWeeklySummaryForNow()
	}
	if err != nil {
		a.reportError(err)
		return
	}

	now := time.Now()
	switch result {
	case dialogSnoozed:
		a.snoozeUntil[prompt] = now.Add(time.Hour)
		if manual {
			// Keep the prompt forced so it re-fires after the snooze window.
			a.forced[prompt] = true
		}
	case dialogCanceled:
		if !manual {
			a.skippedOn[prompt] = now.Format(time.DateOnly)
			delete(a.forced, prompt)
		}
		// manual cancel: just close, no bookkeeping
	case dialogAccepted:
		delete(a.snoozeUntil, prompt)
		delete(a.skippedOn, prompt)
		delete(a.forced, prompt)
	}
}

// runWeekReviewLoop iterates oldest-first through all unreviewed past weeks,
// stopping when the user snoozes or skips (result != dialogAccepted).
func (a *App) runWeekReviewLoop() (dialogResult, error) {
	result := dialogAccepted
	for {
		week, pending, err := a.store.UnreviewedWeek(time.Now())
		if err != nil {
			return dialogCanceled, err
		}
		if !pending {
			break
		}
		result, err = a.runWeekReviewDialog(week, true) // scheduled: roll over NextWeek first
		if err != nil {
			return dialogCanceled, err
		}
		if result != dialogAccepted {
			break
		}
	}
	return result, nil
}

// runWeeklySummaryForNow resolves the current week's ID and shows the summary
// dialog for the scheduled path (marks the week summarized on accept).
func (a *App) runWeeklySummaryForNow() (dialogResult, error) {
	week, _, err := a.store.WeekSummaryPending(time.Now())
	if err != nil {
		return dialogCanceled, err
	}
	return a.runWeeklySummaryDialog(week, true) // scheduled: mark summarized on accept
}

// applyManualResult applies the bookkeeping for a user-initiated (manual)
// dialog result: snooze arms a forced re-fire in an hour; cancel just closes
// without setting skippedOn; accept clears all bookkeeping as usual.
func (a *App) applyManualResult(prompt schedule.Prompt, result dialogResult) {
	now := time.Now()
	switch result {
	case dialogSnoozed:
		a.snoozeUntil[prompt] = now.Add(time.Hour)
		a.forced[prompt] = true // keep pending so it re-fires after snooze
	case dialogCanceled:
		// manual cancel: just close, no skippedOn bookkeeping
	case dialogAccepted:
		delete(a.snoozeUntil, prompt)
		delete(a.skippedOn, prompt)
		delete(a.forced, prompt)
	}
}

// runWeekReviewManually reviews the most recent past week even if it was
// already marked reviewed, so the user can re-triage the backlog on demand.
// Snooze keeps a forced prompt alive so it re-fires; cancel just closes.
func (a *App) runWeekReviewManually() {
	if a.dialogOpen {
		return
	}
	a.dialogOpen = true
	defer func() {
		a.dialogOpen = false
		a.window.refresh()
		a.maybeQuitOneshot()
	}()
	week := store.WeekOf(time.Now().AddDate(0, 0, -7))
	result, err := a.runWeekReviewDialog(week, false) // manual: do not roll over NextWeek
	if err != nil {
		a.reportError(err)
		return
	}
	a.applyManualResult(schedule.PromptWeekReview, result)
}

// openBacklogDialog shows the Backlog manager dialog, guarded by the
// dialogOpen flag so scheduled prompts cannot stack on top of it.
func (a *App) openBacklogDialog() {
	if a.dialogOpen {
		return
	}
	a.dialogOpen = true
	defer func() {
		a.dialogOpen = false
		a.window.refresh()
		a.maybeQuitOneshot()
	}()
	bd, err := a.buildBacklogDialog()
	if err != nil {
		a.reportError(err)
		return
	}
	bd.dialog.Show()
	bd.dialog.Raise()
	bd.dialog.ActivateWindow()
	bd.dialog.Exec()
}

// runWeeklySummaryManually shows the current week's summary on demand.
// It does not mark the week as summarized (that is reserved for the
// scheduled Friday prompt). Snooze keeps a forced prompt alive; cancel
// just closes.
func (a *App) runWeeklySummaryManually() {
	if a.dialogOpen {
		return
	}
	a.dialogOpen = true
	defer func() {
		a.dialogOpen = false
		a.window.refresh()
		a.maybeQuitOneshot()
	}()
	week := store.WeekOf(time.Now())
	result, err := a.runWeeklySummaryDialog(week, false) // manual: do not mark summarized
	if err != nil {
		a.reportError(err)
		return
	}
	a.applyManualResult(schedule.PromptWeeklySummary, result)
}

// ForcePrompt runs the named check-in immediately, regardless of schedule:
// "morning", "evening" or "review". A forced prompt stays pending (so a
// snooze re-shows it in an hour) until answered or canceled.
func (a *App) ForcePrompt(name string) error {
	var prompt schedule.Prompt
	switch name {
	case "morning":
		prompt = schedule.PromptMorning
	case "evening":
		prompt = schedule.PromptEvening
	case "review":
		a.runWeekReviewManually()
		return nil
	case "summary":
		a.runWeeklySummaryManually()
		return nil
	default:
		return fmt.Errorf("unknown check-in %q, want morning, evening, review or summary", name)
	}
	delete(a.snoozeUntil, prompt)
	delete(a.skippedOn, prompt)
	a.forced[prompt] = true
	a.runPrompt(prompt, true)
	return nil
}

// GrabScreenshots renders the main window and every check-in dialog
// into PNG files under dir, for visual UI verification.
func (a *App) GrabScreenshots(dir string) error {
	now := time.Now()
	// Show the window briefly so Qt computes the final layout (viewport
	// widths, item-widget geometry) before we grab the frames.
	a.window.win.Show()
	qt.QCoreApplication_ProcessEvents()
	a.window.refresh()

	morning, err := a.buildMorningDialog(now)
	if err != nil {
		return err
	}
	evening, err := a.buildEveningDialog(now)
	if err != nil {
		return err
	}
	review, err := a.buildWeekReviewDialog(store.WeekOf(now.AddDate(0, 0, -7)), false)
	if err != nil {
		return err
	}
	weeklySummary, err := a.buildWeeklySummaryDialog(store.WeekOf(now), false)
	if err != nil {
		return err
	}
	backlogDlg, err := a.buildBacklogDialog()
	if err != nil {
		return err
	}

	for name, widget := range map[string]*qt.QWidget{
		"main-window":    a.window.win.QWidget,
		"morning":        morning.dialog.QWidget,
		"evening":        evening.dialog.QWidget,
		"week-review":    review.dialog.QWidget,
		"weekly-summary": weeklySummary.dialog.QWidget,
		"backlog":        backlogDlg.dialog.QWidget,
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

// notifyBacklogMove shows a tray balloon confirming that an item was moved
// to the cross-week backlog. It is a no-op when the tray is unavailable.
func (a *App) notifyBacklogMove(itemText string) {
	if a.tray == nil {
		return
	}
	a.tray.ShowMessage2("Moved to backlog", itemText)
}

// checkForUpdatesBackground runs an update check in a goroutine and, when a
// newer version is found, shows a notification on the Qt main thread.
// Errors are silently logged at debug level so offline machines are unaffected.
func (a *App) checkForUpdatesBackground(silent bool) {
	ver := a.version
	go func() {
		latest, newer, pageURL, err := update.Check(
			context.Background(), ver, update.DefaultReleaseURL)
		if err != nil {
			slog.Debug("update check failed", "error", err)
			return
		}
		if !newer {
			if silent {
				mainthread.Start(func() {
					qt.QMessageBox_Information2(a.window.win.QWidget,
						"Daily Progress Logger",
						"You are running the latest version.",
						qt.QMessageBox__Ok)
				})
			}
			return
		}
		mainthread.Start(func() { a.showUpdateDialog(latest, pageURL) })
	}()
}

// checkForUpdatesManual is the tray-menu handler: runs the HTTP check in a
// goroutine so the UI stays responsive, then marshals the outcome back to
// the Qt main thread for display. Errors are shown in a dialog (unlike the
// silent background check).
func (a *App) checkForUpdatesManual() {
	ver := a.version
	go func() {
		latest, newer, pageURL, err := update.Check(
			context.Background(), ver, update.DefaultReleaseURL)
		mainthread.Start(func() {
			if err != nil {
				slog.Debug("manual update check failed", "error", err)
				qt.QMessageBox_Information2(a.window.win.QWidget,
					"Daily Progress Logger",
					"Could not check for updates: "+err.Error(),
					qt.QMessageBox__Ok)
				return
			}
			if !newer {
				qt.QMessageBox_Information2(a.window.win.QWidget,
					"Daily Progress Logger",
					"You are running the latest version.",
					qt.QMessageBox__Ok)
				return
			}
			a.showUpdateDialog(latest, pageURL)
		})
	}()
}

// showUpdateDialog presents the "new version available" notification.
// Must be called on the Qt main thread.
func (a *App) showUpdateDialog(latest, pageURL string) {
	msg := fmt.Sprintf(
		"Daily Progress Logger %s is available (you have %s).",
		latest, a.version)

	dialog := qt.NewQDialog(a.window.win.QWidget)
	dialog.SetWindowTitle("Update Available")
	layout := qt.NewQVBoxLayout(dialog.QWidget)

	label := qt.NewQLabel3(msg)
	label.SetWordWrap(true)
	layout.AddWidget(label.QWidget)

	buttons := qt.NewQDialogButtonBox2()
	openBtn := buttons.AddButton2("Open Release Page", qt.QDialogButtonBox__AcceptRole)
	laterBtn := buttons.AddButton2("Later", qt.QDialogButtonBox__RejectRole)
	openBtn.OnClicked(func() {
		qt.QDesktopServices_OpenUrl(qt.QUrl_FromEncoded([]byte(pageURL)))
		dialog.Accept()
	})
	laterBtn.OnClicked(dialog.Reject)
	layout.AddWidget(buttons.QWidget)

	dialog.Exec()
}

// addMenuAction creates a triggered action on menu.
func addMenuAction(menu *qt.QMenu, text string, handler func()) *qt.QAction {
	action := qt.NewQAction2(text)
	action.OnTriggered(handler)
	menu.AddAction(action)
	return action
}

// trayIcon draws a ring glyph as a template icon, so macOS tints it to
// match the light or dark menu bar.
func trayIcon() *qt.QIcon {
	const size = 22
	pixmap := qt.NewQPixmap2(size, size)
	pixmap.FillWithFillColor(qt.NewQColor11(0, 0, 0, 0))
	painter := qt.NewQPainter2(pixmap.QPaintDevice)
	painter.SetRenderHint(qt.QPainter__Antialiasing)
	painter.SetPenWithStyle(qt.NoPen)
	painter.SetBrush(qt.NewQBrush3(qt.NewQColor3(0, 0, 0)))
	painter.DrawEllipse2(3, 3, size-6, size-6)
	painter.SetCompositionMode(qt.QPainter__CompositionMode_Clear)
	painter.DrawEllipse2(8, 8, size-16, size-16)
	painter.End()
	icon := qt.NewQIcon2(pixmap)
	icon.SetIsMask(true)
	return icon
}
