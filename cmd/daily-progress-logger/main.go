// Command daily-progress-logger is a macOS desktop app that maintains daily
// plan/done logs and weekly summaries as markdown files, prompting each
// morning for the day's plan and each evening for what was accomplished.
package main

import (
	"flag"
	"log/slog"
	"os"
	"strings"
	"syscall"

	qt "github.com/mappu/miqt/qt6"

	"github.com/cristim/daily-progress-logger/internal/config"
	"github.com/cristim/daily-progress-logger/internal/store"
	"github.com/cristim/daily-progress-logger/internal/ui"
)

// version is overridden at build time by -ldflags "-X main.version=<tag>".
var version = "dev"

func main() {
	reexecWithAsyncPreemptOff()

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
	st, err := store.New(cfg.DataDir)
	if err != nil {
		fatal(err)
	}
	app, err := ui.New(st, cfg, version)
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

// reexecWithAsyncPreemptOff re-execs the current process with
// GODEBUG=asyncpreemptoff=1 set, unless it is already set.
//
// Go 1.26's async preemption signals goroutines with SIGURG to interrupt
// long-running loops. Qt installs its own Unix signal handling without the
// SA_ONSTACK flag, so when SIGURG arrives while Qt's C++ code is running the
// Go runtime detects the foreign handler and fatal-errors with "non-Go code
// set up signal handler without SA_ONSTACK flag", crashing the app. There is
// no supported way to disable async preemption via `//go:debug` (it is a
// runtime-internal dbgvar, not a registered godebug), so the only fix is to
// set it via the GODEBUG environment variable before the runtime starts,
// which requires re-executing the process. Do NOT remove this: without it
// the app crashes unpredictably whenever Qt is running a tight C++ loop
// (e.g. window show/paint) when a preemption signal lands.
//
// This must run before anything else in main() — before any goroutine
// starts or Qt is touched — since async preemption is armed by the runtime
// at process start.
func reexecWithAsyncPreemptOff() {
	godebug := os.Getenv("GODEBUG")
	if strings.Contains(godebug, "asyncpreemptoff=") {
		return
	}

	env := make([]string, 0, len(os.Environ())+1)
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "GODEBUG=") {
			env = append(env, kv)
		}
	}
	next := "asyncpreemptoff=1"
	if godebug != "" {
		next = godebug + "," + next
	}
	env = append(env, "GODEBUG="+next)

	exe, err := os.Executable()
	if err != nil {
		slog.Warn("reexec: could not resolve executable path, continuing without asyncpreemptoff", "error", err)
		return
	}
	// exe is this process's own resolved binary path (not attacker-controlled
	// input), and args/env are this process's own argv/environment plus the
	// single GODEBUG override above, so this is a self-re-exec, not an
	// injectable subprocess launch.
	if err := syscall.Exec(exe, os.Args, env); err != nil { //nolint:gosec // self re-exec, see comment above
		slog.Warn("reexec: exec failed, continuing without asyncpreemptoff", "error", err)
	}
}

// fatal reports a startup error both on stderr and as a dialog (the app may
// have been launched from Finder, where stderr is invisible), then exits.
func fatal(err error) {
	slog.Error("startup failed", "error", err)
	qt.QMessageBox_Critical2(nil, "Daily Progress Logger", err.Error(),
		qt.QMessageBox__Ok, qt.QMessageBox__NoButton)
	os.Exit(1)
}
