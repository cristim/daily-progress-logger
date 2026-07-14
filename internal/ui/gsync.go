package ui

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	qt "github.com/mappu/miqt/qt6"
	"github.com/mappu/miqt/qt6/mainthread"

	"github.com/cristim/daily-progress-logger/internal/drive"
	syncengine "github.com/cristim/daily-progress-logger/internal/sync"
)

const (
	keychainService = "com.cristim.daily-progress-logger"
	keychainAccount = "google-oauth-token"

	syncInterval  = 5 * time.Minute
	syncTimeout   = 2 * time.Minute
	signInTimeout = 3 * time.Minute
)

// tokenStore is the macOS Keychain store for the Google OAuth token.
func (a *App) tokenStore() drive.KeychainStore {
	return drive.KeychainStore{Service: keychainService, Account: keychainAccount}
}

// googleSignedIn reports whether a Google token is stored.
func (a *App) googleSignedIn() bool { return a.tokenStore().HasToken() }

// deviceID returns this install's stable sync device name.
func (a *App) deviceID() string {
	if a.cfg.DeviceID != "" {
		return a.cfg.DeviceID
	}
	return "device"
}

// newSyncEngine builds a Drive-backed sync engine from the stored token + config.
func (a *App) newSyncEngine(ctx context.Context) (*syncengine.Engine, error) {
	if a.cfg.GoogleClientID == "" {
		return nil, errors.New("set your Google client ID in Preferences first")
	}
	cfg := drive.Config(a.cfg.GoogleClientID, "")
	httpClient, err := drive.HTTPClient(ctx, cfg, a.tokenStore())
	if err != nil {
		return nil, err
	}
	dc, err := drive.New(ctx, httpClient)
	if err != nil {
		return nil, err
	}
	return syncengine.New(a.store.DataDir, dc, a.deviceID()), nil
}

// signInGoogle runs the interactive Drive sign-in, then a first sync.
// done (may be nil) is called on the main thread once sign-in completes,
// with the error if one occurred, so callers can update UI widgets (L2).
func (a *App) signInGoogle(done func(err error)) {
	if a.cfg.GoogleClientID == "" {
		err := errors.New("enter your Google client ID in Preferences first")
		a.reportError(err)
		if done != nil {
			done(err)
		}
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), signInTimeout)
		defer cancel()
		email, err := drive.SignIn(ctx, a.cfg.GoogleClientID, a.tokenStore())
		mainthread.Start(func() {
			if err != nil {
				a.reportError(err)
				if done != nil {
					done(err)
				}
				return
			}
			a.cfg.GoogleAccount = email
			if err := a.cfg.Save(); err != nil {
				slog.Warn("saving config after sign-in", "error", err)
			}
			a.startSyncTimer()
			a.runSync()
			if done != nil {
				done(nil)
			}
		})
	}()
}

// signOutGoogle forgets the stored token and account.
func (a *App) signOutGoogle() {
	_ = a.tokenStore().Delete()
	a.cfg.GoogleAccount = ""
	if err := a.cfg.Save(); err != nil {
		slog.Warn("saving config after sign-out", "error", err)
	}
	a.stopSyncTimer()
	a.syncEngine = nil // engine is bound to the outgoing token; discard it (M4)
}

// runSync performs one user-initiated sync and surfaces errors as a modal dialog.
// Use this for the "Sync now" button and the first sync after sign-in.
func (a *App) runSync() { a.doSync(true) }

// runSyncOnTimer performs a timer-triggered sync; errors are logged and shown
// as a passive tray notification with dedup/backoff instead of a modal dialog
// so the app remains usable while offline.
func (a *App) runSyncOnTimer() { a.doSync(false) }

// syncErrKey returns a coarse category string for err used to deduplicate
// passive tray notifications: identical consecutive network errors show only
// once per syncInterval; auth errors always show (once per distinct message).
func syncErrKey(err error) string {
	if err == nil {
		return ""
	}
	s := err.Error()
	if strings.Contains(s, "oauth2") ||
		strings.Contains(s, "401") ||
		strings.Contains(s, "403") ||
		(strings.Contains(s, "token") && (strings.Contains(s, "expired") ||
			strings.Contains(s, "invalid") || strings.Contains(s, "revoked"))) {
		return "auth:" + s // auth errors are each surfaced once
	}
	return "net" // all transient network errors share one bucket
}

// doSync runs one sync cycle. When userInitiated is true, errors surface as a
// modal; when false (timer-driven) errors are logged and optionally shown as a
// passive tray message with dedup so repeated offline errors don't flood.
// doSync reuses a.syncEngine when one is already live (M4) so that Engine.mu
// serializes all Run and Resolve calls against the same instance.
func (a *App) doSync(userInitiated bool) {
	if a.syncing || !a.googleSignedIn() {
		return
	}
	a.syncing = true
	// Capture the current engine on the main thread; the goroutine may create a
	// new one if nil and store it back via mainthread.Start.
	capturedEngine := a.syncEngine
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), syncTimeout)
		defer cancel()
		engine := capturedEngine
		var buildErr error
		if engine == nil {
			engine, buildErr = a.newSyncEngine(ctx)
		}
		var res syncengine.Result
		var runErr error
		if buildErr == nil {
			res, runErr = engine.Run(ctx)
		}
		err := buildErr
		if err == nil {
			err = runErr
		}
		mainthread.Start(func() {
			a.syncing = false
			if buildErr == nil && runErr == nil && engine != nil {
				// Persist the working engine for future syncs and conflict resolution.
				a.syncEngine = engine
			}
			if err != nil {
				slog.Warn("sync failed", "error", err, "user_initiated", userInitiated)
				if userInitiated {
					// User explicitly asked to sync: show a modal with full detail.
					a.reportError(err)
				} else {
					// Timer-triggered: show a tray notification but only when the
					// error category changes or syncInterval has passed (backoff).
					key := syncErrKey(err)
					if key != a.lastSyncErrKey || time.Since(a.lastSyncErrTime) >= syncInterval {
						if a.tray != nil {
							msg := err.Error()
							if len(msg) > 120 {
								msg = msg[:117] + "..."
							}
							a.tray.ShowMessage2("Sync error", msg)
						}
						a.lastSyncErrKey = key
						a.lastSyncErrTime = time.Now()
					}
				}
				return
			}
			// Reset dedup on success so a future error is surfaced even if it
			// matches the last shown one.
			a.lastSyncErrKey = ""
			a.window.scheduleRefresh()
			if len(res.Conflicts) > 0 && a.tray != nil {
				a.tray.ShowMessage2("Sync conflicts",
					"Some files changed on two devices. Open Preferences → Resolve.")
			}
		})
	}()
}

// startSyncTimer arms periodic auto-sync when enabled and signed in.
func (a *App) startSyncTimer() {
	if !a.cfg.SyncEnabled || !a.googleSignedIn() {
		return
	}
	if a.syncTimer == nil {
		a.syncTimer = qt.NewQTimer2(a.window.win.QObject)
		a.syncTimer.OnTimeout(a.runSyncOnTimer)
	}
	a.syncTimer.Start(int(syncInterval.Milliseconds()))
	a.runSync() // first run after start is user-visible; show errors as modal
}

// stopSyncTimer disables periodic auto-sync.
func (a *App) stopSyncTimer() {
	if a.syncTimer != nil {
		a.syncTimer.Stop()
	}
}

func (a *App) driveStatusText() string {
	if a.googleSignedIn() {
		if a.cfg.GoogleAccount != "" {
			return "Connected as <b>" + a.cfg.GoogleAccount + "</b>"
		}
		return "Connected."
	}
	return "Not signed in."
}

func (a *App) signButtonText() string {
	if a.googleSignedIn() {
		return "Sign out"
	}
	return "Sign in with Google"
}

// driveSection builds the Preferences "Google Drive sync" group. Its account
// actions (sign in/out, sync, auto-sync) apply immediately and self-persist,
// independent of the dialog's OK/Cancel.
func (a *App) driveSection() *qt.QWidget {
	group := qt.NewQGroupBox3("Google Drive sync")
	v := qt.NewQVBoxLayout(group.QWidget)

	idRow := qt.NewQHBoxLayout2()
	idRow.AddWidget(qt.NewQLabel3("Client ID:").QWidget)
	clientID := qt.NewQLineEdit2()
	clientID.SetText(a.cfg.GoogleClientID)
	clientID.SetPlaceholderText("xxxxx.apps.googleusercontent.com")
	idRow.AddWidget2(clientID.QWidget, 1)
	v.AddLayout(idRow.QLayout)

	status := qt.NewQLabel3(a.driveStatusText())
	status.SetTextFormat(qt.RichText)
	v.AddWidget(status.QWidget)

	saveClientID := func() {
		a.cfg.GoogleClientID = trimmed(clientID.Text())
		if err := a.cfg.Save(); err != nil {
			slog.Warn("saving client id", "error", err)
		}
	}

	btnRow := qt.NewQHBoxLayout2()
	signBtn := qt.NewQPushButton3(a.signButtonText())
	signBtn.OnClicked(func() {
		if a.googleSignedIn() {
			a.signOutGoogle()
			status.SetText(a.driveStatusText())
			signBtn.SetText(a.signButtonText())
		} else {
			saveClientID()
			// Show in-progress state immediately so the dialog doesn't look
			// frozen during the browser OAuth flow (L2).
			status.SetText("Signing in…")
			signBtn.SetEnabled(false)
			signBtn.SetText("Signing in…")
			a.signInGoogle(func(_ error) {
				// Runs on the main thread (via mainthread.Start in signInGoogle).
				signBtn.SetEnabled(true)
				status.SetText(a.driveStatusText())
				signBtn.SetText(a.signButtonText())
			})
		}
	})
	syncBtn := qt.NewQPushButton3("Sync now")
	syncBtn.OnClicked(func() { saveClientID(); a.runSync() })
	conflictsBtn := qt.NewQPushButton3("Resolve conflicts…")
	conflictsBtn.OnClicked(a.openConflictsDialog)
	btnRow.AddWidget(signBtn.QWidget)
	btnRow.AddWidget(syncBtn.QWidget)
	btnRow.AddWidget(conflictsBtn.QWidget)
	v.AddLayout(btnRow.QLayout)

	auto := qt.NewQCheckBox3("Sync automatically in the background")
	auto.SetChecked(a.cfg.SyncEnabled)
	auto.OnToggled(func(on bool) {
		a.cfg.SyncEnabled = on
		if err := a.cfg.Save(); err != nil {
			slog.Warn("saving sync toggle", "error", err)
		}
		if on {
			a.startSyncTimer()
		} else {
			a.stopSyncTimer()
		}
	})
	v.AddWidget(auto.QWidget)

	return group.QWidget
}
