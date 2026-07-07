// Command daily-progress-logger is a macOS desktop app that maintains daily
// plan/done logs and weekly summaries as markdown files, prompting each
// morning for the day's plan and each evening for what was accomplished.
package main

import (
	"flag"
	"log/slog"
	"os"

	qt "github.com/mappu/miqt/qt6"

	"github.com/cristim/daily-progress-logger/internal/config"
	"github.com/cristim/daily-progress-logger/internal/store"
	"github.com/cristim/daily-progress-logger/internal/ui"
)

func main() {
	checkin := flag.String("checkin", "",
		"show the named check-in (morning, evening or review), then exit")
	promptDue := flag.Bool("prompt-due", false,
		"show any check-ins currently due, then exit (for cron/launchd)")
	hidden := flag.Bool("hidden", false,
		"start with the main window hidden (menu bar icon only)")
	screenshotDir := flag.String("screenshot", "",
		"render the UI offscreen into PNGs under this directory and exit")
	flag.Parse()

	qapp := qt.NewQApplication(os.Args)

	cfg, err := config.Load()
	if err != nil {
		fatal(err)
	}
	st := store.New(cfg.DataDir)
	app, err := ui.New(st, cfg)
	if err != nil {
		fatal(err)
	}
	if *screenshotDir != "" {
		if err := app.GrabScreenshots(*screenshotDir); err != nil {
			fatal(err)
		}
		return
	}

	// Oneshot modes for cron/launchd (or manual invocation): show the
	// relevant check-ins and exit. A "Postpone 1h" answer keeps the process
	// alive until the snooze resolves.
	if *promptDue || *checkin != "" {
		if *checkin != "" {
			if err := app.ForcePrompt(*checkin); err != nil {
				fatal(err)
			}
		}
		app.SetOneshot()
		app.CheckPrompts()
		if !app.OneshotPending() {
			return
		}
		os.Exit(qt.QApplication_Exec())
	}

	// Resident mode: menu bar icon plus periodic prompt checks.
	// Re-show the main window when the user clicks the Dock icon.
	app.HandleReopen(qapp)
	if !*hidden {
		app.Show()
		app.MaybeOfferLoginItem()
	}

	// Run the startup prompts once the event loop is up.
	startup := qt.NewQTimer()
	startup.SetSingleShot(true)
	startup.OnTimeout(app.CheckPrompts)
	startup.Start(200)

	os.Exit(qt.QApplication_Exec())
}

// fatal reports a startup error both on stderr and as a dialog (the app may
// have been launched from Finder, where stderr is invisible), then exits.
func fatal(err error) {
	slog.Error("startup failed", "error", err)
	qt.QMessageBox_Critical2(nil, "Daily Progress Logger", err.Error(),
		qt.QMessageBox__Ok, qt.QMessageBox__NoButton)
	os.Exit(1)
}
