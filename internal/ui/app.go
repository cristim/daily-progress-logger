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
	syncengine "github.com/cristim/daily-progress-logger/internal/sync"
	"github.com/cristim/daily-progress-logger/internal/update"
)

// checkInterval is how often the app re-evaluates which prompts are due.
const checkInterval = 60 * time.Second

// updateCheckInterval is how often the app re-checks for a new release.
const updateCheckInterval = 24 * time.Hour

// noPrompt is a sentinel value for pendingNotifyPrompt meaning no check-in
// notification is currently waiting to be clicked.
const noPrompt schedule.Prompt = -1

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
	syncTimer   *qt.QTimer
	syncEngine  *syncengine.Engine // long-lived; shared by runSync and Resolve (M4)
	syncing     bool
	dialogOpen  bool

	// H3: tray-notification dedup for timer-triggered sync errors so we don't
	// flood the user with notifications when offline.  lastSyncErrKey is a
	// coarse category string; lastSyncErrTime is when we last showed it.
	lastSyncErrKey  string
	lastSyncErrTime time.Time

	// shortcuts holds the app-wide QShortcut per action ID (config.Shortcut*),
	// so applyShortcuts can rebind keys in place when Preferences change rather
	// than leaking a fresh set. app.quit is bound on the menu action instead.
	shortcuts map[string]*qt.QShortcut

	// snoozeUntil holds "Postpone 1h" deadlines per prompt.
	snoozeUntil map[schedule.Prompt]time.Time
	// skippedOn records the day (time.DateOnly) a prompt was canceled, so
	// it stays quiet until the next day (or app restart).
	skippedOn map[schedule.Prompt]string
	// forced prompts were requested explicitly (-checkin flag or tray menu) and
	// are kept pending even when the schedule alone would not show them. The
	// bool value records whether the prompt originated from a manual invocation
	// (true) or a scheduled one (false), so a snoozed manual prompt re-fires
	// with "Close" semantics rather than "Skip Today".
	forced map[schedule.Prompt]bool
	// oneshot mode (cron/launchd): quit once nothing is pending.
	oneshot bool

	// notifiedOn records the date (DateOnly) on which a scheduled prompt was
	// surfaced as a macOS notification banner, so the 60s timer does not
	// re-notify the same prompt within the same calendar day.
	notifiedOn map[schedule.Prompt]string
	// pendingNotifyPrompt is the prompt for which the last check-in
	// notification was posted. OnMessageClicked opens its dialog. noPrompt
	// means no check-in notification is currently outstanding.
	pendingNotifyPrompt schedule.Prompt
}

// New builds the application UI. The Qt application object must already
// exist. appVersion is the running binary's version string (e.g. "0.1.0" or
// "dev") and is used by the auto-updater.
func New(st *store.Store, cfg *config.Config, appVersion string) (*App, error) {
	app := &App{
		store:               st,
		cfg:                 cfg,
		version:             appVersion,
		snoozeUntil:         map[schedule.Prompt]time.Time{},
		skippedOn:           map[schedule.Prompt]string{},
		forced:              map[schedule.Prompt]bool{},
		notifiedOn:          map[schedule.Prompt]string{},
		pendingNotifyPrompt: noPrompt,
	}
	app.window = newMainWindow(app)
	app.setUpTray()

	// Derive the schedule times from cfg and install the keyboard shortcuts.
	if err := app.applyConfig(cfg); err != nil {
		return nil, err
	}

	// Closing the window keeps the app resident in the menu bar.
	qt.QGuiApplication_SetQuitOnLastWindowClosed(false)

	// Begin background Drive sync when enabled and signed in.
	app.startSyncTimer()

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

// applyConfig re-derives the schedule times from cfg and (re)installs the
// keyboard shortcuts, so a Preferences save takes effect without a restart.
// cfg must already be valid (Preferences validates before calling this).
func (a *App) applyConfig(cfg *config.Config) error {
	morningHour, morningMinute, err := config.ParseTimeOfDay(cfg.MorningTime)
	if err != nil {
		return err
	}
	eveningHour, eveningMinute, err := config.ParseTimeOfDay(cfg.EveningTime)
	if err != nil {
		return err
	}
	summaryHour, summaryMinute, err := config.ParseTimeOfDay(cfg.SummaryTime)
	if err != nil {
		return err
	}
	summaryDay, err := config.ParseDay(cfg.SummaryDay)
	if err != nil {
		return err
	}
	a.cfg = cfg
	a.morning = schedule.TimeOfDay{Hour: morningHour, Minute: morningMinute}
	a.evening = schedule.TimeOfDay{Hour: eveningHour, Minute: eveningMinute}
	a.summary = schedule.TimeOfDay{Hour: summaryHour, Minute: summaryMinute}
	a.summaryDay = summaryDay
	// Recurring tasks without an explicit @HH:MM default to the morning check-in.
	a.store.SetDefaultReminderTime(morningHour, morningMinute)
	a.applyShortcuts(cfg)
	return nil
}

// Show displays the main window.
func (a *App) Show() {
	a.window.materializeViewedDate()
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

	// If a check-in dialog is already open, skip the login offer this launch:
	// LoginItemOffered is already set, so the offer will not repeat.
	if a.dialogOpen {
		return
	}

	const question = "Start Daily Progress Logger at login?\n" +
		"It will run quietly in the menu bar."
	var answer int
	a.withModalGuard(func() {
		answer = qt.QMessageBox_Question2(
			a.window.win.QWidget,
			"Daily Progress Logger",
			question,
			qt.QMessageBox__Yes,
			qt.QMessageBox__No,
		)
	})
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

// HandleReopen re-shows the hidden main window when the application is
// activated (Dock-icon click, `open` on the bundle). Qt 6 no longer
// delivers the deprecated QEvent::ApplicationActivate, so this hooks the
// applicationStateChanged signal instead. Activations caused by a check-in
// dialog raising itself are excluded via the dialogOpen guard so a prompt
// on a hidden app does not drag the main window out with it.
func (a *App) HandleReopen(qapp *qt.QApplication) {
	qapp.OnApplicationStateChanged(func(state qt.ApplicationState) {
		if state == qt.ApplicationActive && !a.window.win.IsVisible() && !a.dialogOpen {
			a.Show()
		}
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
	addMenuAction(menu, "Weekly Plan…", a.runWeeklyPlanManually)
	addMenuAction(menu, "Morning Check-in…", func() { a.runPrompt(schedule.PromptMorning, true) })
	addMenuAction(menu, "Evening Check-in…", func() { a.runPrompt(schedule.PromptEvening, true) })
	addMenuAction(menu, "This Week's Summary…", a.runWeeklySummaryManually)
	addMenuAction(menu, "Review Last Week…", a.runWeekReviewManually)
	addMenuAction(menu, "Backlog…", a.openBacklogDialog)
	menu.AddSeparator()
	addMenuAction(menu, "Preferences…", a.openPreferencesDialog)
	addMenuAction(menu, "Check for Updates…", a.checkForUpdatesManual)
	menu.AddSeparator()
	addMenuAction(menu, "Quit", qt.QCoreApplication_Quit)

	a.tray.SetContextMenu(menu)
	a.tray.SetToolTip("Daily Progress Logger")
	a.tray.Show()

	// When the user clicks a check-in notification banner, open the dialog for
	// the prompt that triggered it. pendingNotifyPrompt is reset to noPrompt
	// after opening so a subsequent non-check-in notification (recurring
	// reminder, backlog move) does not accidentally reopen a stale dialog.
	a.tray.OnMessageClicked(func() {
		if a.pendingNotifyPrompt == noPrompt {
			return
		}
		p := a.pendingNotifyPrompt
		a.pendingNotifyPrompt = noPrompt
		a.runPrompt(p, false)
	})
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

	// Midnight watchdog: if the main window still shows yesterday's plan,
	// materialize the new today's recurring occurrences and refresh so the
	// heading and item list reflect the new day.
	if today := now.Format(time.DateOnly); a.window.renderedDate != "" && a.window.renderedDate != today {
		if _, err := a.store.MaterializeRecurring(now); err != nil {
			a.reportError(err)
		}
		a.window.scheduleRefresh()
	}

	a.fireRecurring(now)

	due, err := a.duePrompts(now)
	if err != nil {
		a.reportError(err)
		return
	}
	show, _ := schedule.Filter(due, now, a.snoozeUntil, a.skippedOn)
	if a.canNotify() {
		today := now.Format(time.DateOnly)
		for _, p := range show {
			if a.notifiedOn[p] == today {
				continue // already notified this prompt today; let the user click or use the menu
			}
			a.notifiedOn[p] = today
			a.pendingNotifyPrompt = p
			title, body := promptNotificationText(p)
			a.tray.ShowMessage5(title, body, qt.QSystemTrayIcon__NoIcon, 5000)
		}
	} else {
		for _, prompt := range show {
			a.runPrompt(prompt, false)
		}
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
	_, planPending, err := a.store.WeeklyPlanPending(now)
	if err != nil {
		return st, err
	}
	st.WeeklyPlanPending = planPending
	pendingWeek, summaryPending, err := a.store.WeekSummaryPending(now)
	if err != nil {
		return st, err
	}
	st.SummaryPending = summaryPending
	// If the pending week is before the current week, the summary was missed
	// (e.g. Friday away) and should fire on any day, not just the summary day.
	if summaryPending {
		currentWeek := store.WeekOf(now)
		st.SummaryPendingPastWeek = pendingWeek.Before(currentWeek)
	}
	return st, nil
}

// runPrompt opens the dialog for prompt, guarding against overlapping
// dialogs (the timer keeps firing while a modal dialog runs its own event
// loop), and records the user's snooze/skip choice.
// For scheduled invocations (timer/CheckPrompts) use manual=false.
// For user-initiated invocations (tray menu, -checkin flag) use manual=true:
// the reject button shows "Close" instead of "Skip Today", cancel just closes
// without setting skippedOn, and snooze keeps the prompt forced (with the
// manual bit) so the re-fired dialog also shows "Close" semantics.
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

	// A snoozed manual prompt re-fires via forced with the manual bit set:
	// honour that origin so the re-fire also shows "Close" semantics.
	if isForced, ok := a.forced[prompt]; ok && isForced {
		manual = true
	}

	result, err := a.dispatchPrompt(prompt, manual)
	if err != nil {
		a.reportError(err)
		return
	}

	now := time.Now()
	switch result {
	case dialogSnoozed:
		// Cap the snooze deadline at 23:59:59 of the current day. A snooze set
		// at 23:30 would otherwise expire at 00:30 the next day, at which point
		// the evening prompt is no longer due and the promised reminder never
		// arrives (finding 35).
		snoozeTarget := now.Add(time.Hour)
		eod := time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, 0, now.Location())
		if snoozeTarget.After(eod) {
			snoozeTarget = eod
		}
		a.snoozeUntil[prompt] = snoozeTarget
		if manual {
			// Keep the prompt forced and record the manual origin so the
			// re-fired dialog after the snooze also shows "Close" semantics.
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

// dispatchPrompt opens the dialog(s) for prompt and returns the user's result.
func (a *App) dispatchPrompt(prompt schedule.Prompt, manual bool) (dialogResult, error) {
	switch prompt {
	case schedule.PromptMorning:
		return a.runMorningDialog(manual)
	case schedule.PromptEvening:
		return a.runEveningDialog(manual)
	case schedule.PromptWeekReview:
		return a.runWeekReviewLoop()
	case schedule.PromptWeeklyPlan:
		return a.runWeeklyPlanDialog(store.WeekOf(time.Now()), manual)
	case schedule.PromptWeeklySummary:
		return a.runWeeklySummaryForNow()
	}
	return dialogAccepted, nil
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

// runWeeklySummaryForNow shows the summary dialog for the oldest unsummarized
// week. When multiple past weeks are pending it loops oldest-first (mirroring
// runWeekReviewLoop), stopping when the user snoozes or cancels.
func (a *App) runWeeklySummaryForNow() (dialogResult, error) {
	result := dialogAccepted
	for {
		week, pending, err := a.store.WeekSummaryPending(time.Now())
		if err != nil {
			return dialogCanceled, err
		}
		if !pending {
			break
		}
		result, err = a.runWeeklySummaryDialog(week, true) // mark summarized on accept
		if err != nil {
			return dialogCanceled, err
		}
		if result != dialogAccepted {
			break
		}
	}
	return result, nil
}

// applyManualResult applies the bookkeeping for a user-initiated (manual)
// dialog result: snooze arms a forced re-fire in an hour; cancel just closes
// without setting skippedOn; accept clears all bookkeeping as usual.
func (a *App) applyManualResult(prompt schedule.Prompt, result dialogResult) {
	now := time.Now()
	switch result {
	case dialogSnoozed:
		snoozeTarget := now.Add(time.Hour)
		eod := time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, 0, now.Location())
		if snoozeTarget.After(eod) {
			snoozeTarget = eod
		}
		a.snoozeUntil[prompt] = snoozeTarget
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

// runWeeklyPlanManually opens the weekly-plan dialog for the current week on
// demand, so the user can set or tick off the week's big things any time.
func (a *App) runWeeklyPlanManually() {
	if a.dialogOpen {
		return
	}
	a.dialogOpen = true
	defer func() {
		a.dialogOpen = false
		a.window.refresh()
		a.maybeQuitOneshot()
	}()
	result, err := a.runWeeklyPlanDialog(store.WeekOf(time.Now()), true)
	if err != nil {
		a.reportError(err)
		return
	}
	a.applyManualResult(schedule.PromptWeeklyPlan, result)
}

// ForcePrompt runs the named check-in immediately, regardless of schedule:
// "morning", "evening", "review", "summary" or "plan". A forced prompt stays
// pending (so a snooze re-shows it in an hour) until answered or canceled.
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
	case "plan":
		a.runWeeklyPlanManually()
		return nil
	default:
		return fmt.Errorf("unknown check-in %q, want morning, evening, review, summary or plan", name)
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
	a.window.materializeViewedDate()
	a.window.refresh()

	morning, err := a.buildMorningDialog(now, false) // screenshots: use scheduled appearance
	if err != nil {
		return err
	}
	evening, err := a.buildEveningDialog(now, false)
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
	weeklyPlan, err := a.buildWeeklyPlanDialog(store.WeekOf(now), false)
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
		"weekly-plan":    weeklyPlan.dialog.QWidget,
		"backlog":        backlogDlg.dialog.QWidget,
		"preferences":    a.buildPreferencesDialog().QWidget,
		"conflicts":      a.buildConflictsDialog().QWidget,
	} {
		path := dir + "/" + name + ".png"
		if !widget.Grab().Save(path) {
			return fmt.Errorf("saving screenshot %s failed", path)
		}
		slog.Info("screenshot saved", "path", path)
	}
	return nil
}

// withModalGuard sets dialogOpen while f runs (a modal exec loop) and clears
// it afterward via defer, so the 60 s timer and HandleReopen cannot stack a
// check-in or re-show the main window on top of update/error/offer dialogs.
// When dialogOpen is already set the call is a no-op and f is not invoked.
func (a *App) withModalGuard(f func()) {
	if a.dialogOpen {
		return
	}
	prev := a.dialogOpen
	a.dialogOpen = true
	defer func() { a.dialogOpen = prev }()
	f()
}

func (a *App) reportError(err error) {
	slog.Error("ui error", "error", err)
	prev := a.dialogOpen
	a.dialogOpen = true
	defer func() { a.dialogOpen = prev }()
	qt.QMessageBox_Critical2(a.window.win.QWidget, "Daily Progress Logger", err.Error(),
		qt.QMessageBox__Ok, qt.QMessageBox__NoButton)
}

// notifyBacklogMove shows a tray balloon confirming that an item was moved
// to the cross-week backlog. It is a no-op when the tray is unavailable.
func (a *App) notifyBacklogMove(itemText string) {
	a.showTrayMessage("Moved to backlog", itemText)
}

// fireRecurring shows a tray reminder for each recurring task whose occurrence
// has come due and adds it to today's plan (preserving any story tag), so the
// reminder is also actionable in the tree.
// fireRecurring shows a tray reminder for every recurring occurrence newly
// due at now (notification-only; RecurringDue tracks its own once-per-
// occurrence state independent of materialization), then materializes today's
// occurrences so a fired reminder makes the task appear even if today was
// never opened in the window. The tree is refreshed only when either step
// actually produced something.
func (a *App) fireRecurring(now time.Time) {
	due, err := a.store.RecurringDue(now)
	if err != nil {
		slog.Warn("checking recurring tasks", "error", err)
		due = nil
	}
	for _, t := range due {
		a.showTrayMessage("Reminder", t.Text)
	}

	added, err := a.store.MaterializeRecurring(now)
	if err != nil {
		slog.Warn("materializing recurring tasks", "error", err)
	}

	if len(due) > 0 || len(added) > 0 {
		a.window.scheduleRefresh()
	}
}

// notifyAdopt shows a tray balloon confirming that a backlog item was added
// (or re-planned) into today's plan. It is a no-op when the tray is unavailable.
func (a *App) notifyAdopt(itemText string) {
	a.showTrayMessage("Planned for today", itemText)
}

// showTrayMessage posts a non-check-in tray balloon (backlog, recurring, etc.)
// and resets pendingNotifyPrompt so a subsequent click on this banner does not
// accidentally open a stale check-in dialog.
func (a *App) showTrayMessage(title, msg string) {
	if a.tray == nil {
		return
	}
	a.pendingNotifyPrompt = noPrompt
	a.tray.ShowMessage2(title, msg)
}

// canNotify reports whether due check-ins should be surfaced as macOS
// notification banners rather than immediate modal dialogs. It is false in
// oneshot mode (the short-lived -prompt-due process must show a dialog so the
// check-in is not lost when the process exits), when the system tray is
// unavailable, or when the user has turned off notifications in Preferences.
func (a *App) canNotify() bool {
	return !a.oneshot &&
		a.cfg.NotifyCheckinsEnabled() &&
		a.tray != nil &&
		qt.QSystemTrayIcon_SupportsMessages()
}

// promptNotificationText returns the banner title and body for a scheduled
// check-in prompt notification.
func promptNotificationText(p schedule.Prompt) (title, body string) {
	switch p {
	case schedule.PromptMorning:
		return "Morning Check-in", "What are you planning to work on today?"
	case schedule.PromptEvening:
		return "Evening Check-in", "How did today go?"
	case schedule.PromptWeekReview:
		return "Week Review", "Review last week's open items."
	case schedule.PromptWeeklyPlan:
		return "Weekly Plan", "What are the big things for this week?"
	case schedule.PromptWeeklySummary:
		return "Weekly Summary", "Review this week's accomplishments."
	default:
		return "Check-in", "A check-in is due."
	}
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
					a.withModalGuard(func() {
						qt.QMessageBox_Information2(a.window.win.QWidget,
							"Daily Progress Logger",
							"You are running the latest version.",
							qt.QMessageBox__Ok)
					})
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
				a.withModalGuard(func() {
					qt.QMessageBox_Information2(a.window.win.QWidget,
						"Daily Progress Logger",
						"Could not check for updates: "+err.Error(),
						qt.QMessageBox__Ok)
				})
				return
			}
			if !newer {
				a.withModalGuard(func() {
					qt.QMessageBox_Information2(a.window.win.QWidget,
						"Daily Progress Logger",
						"You are running the latest version.",
						qt.QMessageBox__Ok)
				})
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

	a.withModalGuard(func() {
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
	})
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
