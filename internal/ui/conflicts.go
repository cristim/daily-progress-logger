package ui

import (
	"html"

	qt "github.com/mappu/miqt/qt6"

	syncengine "github.com/cristim/daily-progress-logger/internal/sync"
)

// openConflictsDialog shows the sync-conflict resolver. It is a short-lived,
// user-initiated sub-dialog (safe to nest under Preferences), so it does not use
// the dialogOpen guard.
func (a *App) openConflictsDialog() {
	a.buildConflictsDialog().Exec()
	a.window.scheduleRefresh()
}

// buildConflictsDialog lists unresolved conflicts (read locally, so it works
// offline) with per-file resolution actions.
func (a *App) buildConflictsDialog() *qt.QDialog {
	engine := syncengine.NewLocal(a.store.DataDir, a.deviceID())
	conflicts, err := engine.Conflicts()
	if err != nil {
		a.reportError(err)
	}

	dialog := qt.NewQDialog(a.window.win.QWidget)
	dialog.SetWindowTitle("Sync Conflicts")
	dialog.SetMinimumWidth(540)
	layout := qt.NewQVBoxLayout(dialog.QWidget)

	if len(conflicts) == 0 {
		layout.AddWidget(qt.NewQLabel3("No conflicts — everything is in sync.").QWidget)
	} else {
		intro := qt.NewQLabel3("<b>These files changed on two devices.</b> Both versions are " +
			"kept; choose which to keep for each.")
		intro.SetWordWrap(true)
		layout.AddWidget(intro.QWidget)
		area, rows := newRowsArea()
		for _, c := range conflicts {
			rows.AddWidget(a.conflictRow(dialog, engine, c))
		}
		layout.AddWidget(area.QWidget)
	}

	buttons := qt.NewQDialogButtonBox4(qt.QDialogButtonBox__Close)
	buttons.OnRejected(dialog.Reject)
	buttons.OnAccepted(dialog.Accept)
	layout.AddWidget(buttons.QWidget)
	return dialog
}

func (a *App) conflictRow(dialog *qt.QDialog, engine *syncengine.Engine, c syncengine.Conflict) *qt.QWidget {
	row, l := newRowWidget()
	label := qt.NewQLabel3(html.EscapeString(c.Path))
	label.SetTextFormat(qt.PlainText)
	label.SetWordWrap(true)
	l.AddWidget2(label.QWidget, 1)

	resolve := func(choice syncengine.ResolveChoice) {
		if err := engine.Resolve(c.Path, choice); err != nil {
			a.reportError(err)
			return
		}
		a.runSync() // propagate the resolution to Drive
		dialog.Accept()
	}
	l.AddWidget(conflictButton("Keep this device", func() { resolve(syncengine.KeepLocal) }))
	l.AddWidget(conflictButton("Keep the other", func() { resolve(syncengine.KeepRemote) }))
	l.AddWidget(conflictButton("Keep both", func() { resolve(syncengine.KeepBoth) }))
	return row
}

func conflictButton(text string, handler func()) *qt.QWidget {
	btn := qt.NewQPushButton3(text)
	btn.OnClicked(handler)
	return btn.QWidget
}
