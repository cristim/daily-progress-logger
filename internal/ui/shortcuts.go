package ui

import (
	"time"

	qt "github.com/mappu/miqt/qt6"

	"github.com/cristim/daily-progress-logger/internal/config"
	"github.com/cristim/daily-progress-logger/internal/schedule"
	"github.com/cristim/daily-progress-logger/internal/store"
)

// applyShortcuts (re)binds every configurable action to its key sequence from
// cfg. On the first call it creates the QShortcuts (parented to the main window,
// application-wide context) and wires each handler; on later calls it just
// updates the key of the existing shortcut, so editing a binding in Preferences
// takes effect live without leaking duplicate shortcuts. The quit action is
// bound on the File-menu QAction instead (so its key shows in the menu), which
// also avoids an "ambiguous shortcut" clash with a QShortcut on the same key.
//
// Shortcuts fire while the app's window is the active window. When the window is
// hidden (resident in the menu bar) they do not fire; the tray menu remains the
// way back in.
func (a *App) applyShortcuts(cfg *config.Config) {
	if a.shortcuts == nil {
		a.shortcuts = map[string]*qt.QShortcut{}
	}

	if a.window.quitAction != nil {
		a.window.quitAction.SetShortcut(qt.NewQKeySequence2(cfg.Shortcuts[config.ShortcutAppQuit]))
	}

	for id, handler := range a.shortcutHandlers() {
		seq := cfg.Shortcuts[id]
		if seq == "" {
			continue
		}
		key := qt.NewQKeySequence2(seq)
		if sc, ok := a.shortcuts[id]; ok {
			sc.SetKey(key)
			continue
		}
		sc := qt.NewQShortcut2(key, a.window.win.QObject)
		sc.SetContext(qt.ApplicationShortcut)
		sc.OnActivated(handler)
		a.shortcuts[id] = sc
	}
}

// shortcutHandlers maps each action ID to its handler. app.quit is intentionally
// absent (bound on the menu action in applyShortcuts). Item actions operate on
// the currently selected plan row and are no-ops when a modal dialog is open or
// no row is selected.
func (a *App) shortcutHandlers() map[string]func() {
	// item wraps a store operation so it runs against the selected plan row,
	// resolving the row's text (stable across list rebuilds) to an index.
	item := func(action func(now time.Time, idx int, text string) error) func() {
		return func() {
			if a.dialogOpen {
				return
			}
			text, ok := a.window.currentItemText()
			if !ok {
				return
			}
			a.window.runItemAction(text, func(now time.Time, idx int) error {
				return action(now, idx, text)
			})
		}
	}

	return map[string]func(){
		config.ShortcutItemDone: item(func(now time.Time, idx int, _ string) error {
			return a.store.SetPlanItemState(now, idx, store.StateDone)
		}),
		config.ShortcutItemTodo: item(func(now time.Time, idx int, _ string) error {
			return a.store.SetPlanItemState(now, idx, store.StateTodo)
		}),
		config.ShortcutItemNextDay: item(func(now time.Time, idx int, _ string) error {
			return a.store.PostponeToNextDay(now, idx)
		}),
		config.ShortcutItemNextWeek: item(func(now time.Time, idx int, _ string) error {
			return a.store.PostponePlanItem(now, idx)
		}),
		config.ShortcutItemBacklog: item(func(now time.Time, idx int, text string) error {
			if err := a.store.MoveToBacklog(now, idx); err != nil {
				return err
			}
			a.notifyBacklogMove(text)
			return nil
		}),
		config.ShortcutCheckinMorning: func() { a.runPrompt(schedule.PromptMorning, true) },
		config.ShortcutCheckinEvening: func() { a.runPrompt(schedule.PromptEvening, true) },
		config.ShortcutViewBacklog:    a.openBacklogDialog,
		config.ShortcutViewSummary:    a.runWeeklySummaryManually,
		config.ShortcutReviewWeek:     a.runWeekReviewManually,
		config.ShortcutWindowToggle:   a.toggleWindow,
		config.ShortcutWindowAddTask:  a.focusAddTask,
	}
}

// toggleWindow hides the main window when visible, or shows it when hidden.
func (a *App) toggleWindow() {
	if a.window.win.IsVisible() {
		a.window.win.Hide()
	} else {
		a.Show()
	}
}

// focusAddTask shows the window and moves keyboard focus to the add-task field.
func (a *App) focusAddTask() {
	a.Show()
	a.window.newItem.SetFocus()
}
