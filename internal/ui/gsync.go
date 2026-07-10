package ui

import (
	"context"
	"errors"
	"log/slog"
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
func (a *App) signInGoogle() {
	if a.cfg.GoogleClientID == "" {
		a.reportError(errors.New("enter your Google client ID in Preferences first"))
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), signInTimeout)
		defer cancel()
		email, err := drive.SignIn(ctx, a.cfg.GoogleClientID, a.tokenStore())
		mainthread.Start(func() {
			if err != nil {
				a.reportError(err)
				return
			}
			a.cfg.GoogleAccount = email
			if err := a.cfg.Save(); err != nil {
				slog.Warn("saving config after sign-in", "error", err)
			}
			a.startSyncTimer()
			a.runSync()
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
}

// runSync performs one background sync and surfaces the outcome.
func (a *App) runSync() {
	if a.syncing || !a.googleSignedIn() {
		return
	}
	a.syncing = true
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), syncTimeout)
		defer cancel()
		engine, err := a.newSyncEngine(ctx)
		var res syncengine.Result
		if err == nil {
			res, err = engine.Run(ctx)
		}
		mainthread.Start(func() {
			a.syncing = false
			if err != nil {
				slog.Warn("sync failed", "error", err)
				a.reportError(err)
				return
			}
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
		a.syncTimer.OnTimeout(a.runSync)
	}
	a.syncTimer.Start(int(syncInterval.Milliseconds()))
	a.runSync() // sync once on start
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
		} else {
			saveClientID()
			a.signInGoogle()
		}
		status.SetText(a.driveStatusText())
		signBtn.SetText(a.signButtonText())
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
