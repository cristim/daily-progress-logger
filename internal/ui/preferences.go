package ui

import (
	"log/slog"

	qt "github.com/mappu/miqt/qt6"

	"github.com/cristim/daily-progress-logger/internal/config"
	"github.com/cristim/daily-progress-logger/internal/store"
)

// weekdayChoices lists the weekday names offered for the weekly-summary day,
// in the order shown in the Preferences combo box. Each is accepted by
// config.ParseDay.
var weekdayChoices = []string{
	"Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday",
}

// openPreferencesDialog shows the Preferences window and applies the result.
// Guarded by dialogOpen like the other modal dialogs so a scheduled prompt
// cannot stack on top of it.
func (a *App) openPreferencesDialog() {
	if a.dialogOpen {
		return
	}
	a.dialogOpen = true
	defer func() {
		a.dialogOpen = false
		a.window.refresh()
		a.maybeQuitOneshot()
	}()

	dialog := a.buildPreferencesDialog()
	dialog.Show()
	dialog.Raise()
	dialog.ActivateWindow()
	dialog.Exec()
}

// buildPreferencesDialog constructs the Preferences dialog: editable check-in
// times, weekly-summary day/time, data folder, and a keyboard-shortcut editor
// per action grouped by category. Accepting validates and saves the new config,
// then reloads it (to expand "~" and normalize) and applies it live via
// applyConfig, so time and shortcut changes take effect without a restart.
// Building and running are separate so the dialog can also be rendered
// offscreen for screenshots (see GrabScreenshots).
func (a *App) buildPreferencesDialog() *qt.QDialog {
	dialog := qt.NewQDialog(a.window.win.QWidget)
	dialog.SetWindowTitle("Preferences")
	dialog.SetMinimumWidth(480)
	layout := qt.NewQVBoxLayout(dialog.QWidget)

	// General settings.
	form := qt.NewQFormLayout2()
	dataDir := qt.NewQLineEdit2()
	dataDir.SetText(a.cfg.DataDir)
	form.AddRow3("Data folder:", dataDir.QWidget)

	morning := qt.NewQLineEdit2()
	morning.SetText(a.cfg.MorningTime)
	form.AddRow3("Morning check-in (HH:MM):", morning.QWidget)

	evening := qt.NewQLineEdit2()
	evening.SetText(a.cfg.EveningTime)
	form.AddRow3("Evening check-in (HH:MM):", evening.QWidget)

	summaryTime := qt.NewQLineEdit2()
	summaryTime.SetText(a.cfg.SummaryTime)
	form.AddRow3("Weekly summary time (HH:MM):", summaryTime.QWidget)

	summaryDay := qt.NewQComboBox2()
	for _, name := range weekdayChoices {
		summaryDay.AddItem(name)
	}
	summaryDay.SetCurrentText(a.cfg.SummaryDay)
	form.AddRow3("Weekly summary day:", summaryDay.QWidget)
	layout.AddLayout(form.QLayout)

	// Keyboard shortcuts, grouped by category in a scroll area.
	layout.AddWidget(qt.NewQLabel3("<b>Keyboard shortcuts</b>").QWidget)
	edits := map[string]*qt.QKeySequenceEdit{}
	area, rows := newRowsArea()
	var lastCategory string
	var catForm *qt.QFormLayout
	for _, act := range config.ShortcutActions {
		if act.Category != lastCategory {
			group := qt.NewQGroupBox3(act.Category)
			catForm = qt.NewQFormLayout(group.QWidget)
			rows.AddWidget(group.QWidget)
			lastCategory = act.Category
		}
		edit := qt.NewQKeySequenceEdit2()
		edit.SetKeySequence(qt.NewQKeySequence2(a.cfg.Shortcuts[act.ID]))
		edits[act.ID] = edit
		catForm.AddRow3(act.Label+":", edit.QWidget)
	}
	layout.AddWidget(area.QWidget)

	// Google Drive sync (self-persisting account actions).
	layout.AddWidget(a.driveSection())

	buttons := qt.NewQDialogButtonBox4(qt.QDialogButtonBox__Ok | qt.QDialogButtonBox__Cancel)
	buttons.Button(qt.QDialogButtonBox__Ok).SetDefault(true)
	buttons.OnRejected(dialog.Reject)
	buttons.OnAccepted(func() {
		if a.savePreferences(dialog, prefInputs{
			dataDir:     dataDir,
			morning:     morning,
			evening:     evening,
			summaryTime: summaryTime,
			summaryDay:  summaryDay,
			edits:       edits,
		}) {
			dialog.Accept()
		}
	})
	layout.AddWidget(buttons.QWidget)
	return dialog
}

// prefInputs bundles the Preferences dialog's editable widgets so savePreferences
// can read them without a long parameter list.
type prefInputs struct {
	dataDir     *qt.QLineEdit
	morning     *qt.QLineEdit
	evening     *qt.QLineEdit
	summaryTime *qt.QLineEdit
	summaryDay  *qt.QComboBox
	edits       map[string]*qt.QKeySequenceEdit
}

// savePreferences reads the widgets into a copy of the config, saves it (which
// validates), reloads and applies it live. It returns true on success; on a
// validation/IO error it shows a message, leaves the config untouched, and
// returns false so the dialog stays open for correction.
func (a *App) savePreferences(dialog *qt.QDialog, in prefInputs) bool {
	next := *a.cfg
	next.DataDir = trimmed(in.dataDir.Text())
	next.MorningTime = trimmed(in.morning.Text())
	next.EveningTime = trimmed(in.evening.Text())
	next.SummaryTime = trimmed(in.summaryTime.Text())
	next.SummaryDay = in.summaryDay.CurrentText()
	next.Shortcuts = make(map[string]string, len(in.edits))
	for id, edit := range in.edits {
		next.Shortcuts[id] = edit.KeySequence().ToString()
	}

	if err := next.Save(); err != nil {
		a.showPreferencesError(dialog, err)
		return false
	}

	// Reload so "~" is expanded and the config is re-normalized before it drives
	// the running app.
	reloaded, err := config.Load()
	if err != nil {
		a.showPreferencesError(dialog, err)
		return false
	}
	if err := a.applyConfig(reloaded); err != nil {
		a.showPreferencesError(dialog, err)
		return false
	}

	// If the data folder changed, rebuild the store through store.New (which
	// runs the story→project migration) and then MigrateRefTags. Simply
	// overwriting DataDir in place skips both migrations and leaves the sync
	// engine pointing at a directory whose .sync-state.json doesn't match,
	// causing mass conflict copies on the next run (M5).
	if reloaded.DataDir != a.store.DataDir {
		newStore, err := store.New(reloaded.DataDir)
		if err != nil {
			a.showPreferencesError(dialog, err)
			return false
		}
		if err := newStore.MigrateRefTags(); err != nil {
			// Non-fatal: backward-compat @ parsing is still active (see main.go).
			slog.Warn("MigrateRefTags on new data dir", "error", err)
		}
		a.store = newStore
		// Discard the live sync engine; it is bound to the old DataDir. The next
		// sync will create a fresh engine rooted at the new directory.
		a.syncEngine = nil
		// Re-arm the sync timer with the new data directory.
		a.stopSyncTimer()
		a.startSyncTimer()
	}
	return true
}

// showPreferencesError reports a save failure without dismissing the dialog.
func (a *App) showPreferencesError(dialog *qt.QDialog, err error) {
	qt.QMessageBox_Critical2(dialog.QWidget, "Preferences", err.Error(),
		qt.QMessageBox__Ok, qt.QMessageBox__NoButton)
}
