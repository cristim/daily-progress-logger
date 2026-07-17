// Package mobilecore is the gomobile-bound surface shared by the iOS (and later
// Android) apps. It wraps the same internal/store model and internal/sync engine
// the desktop uses, exposing a small API of string/JSON in, string/JSON out so
// it binds cleanly through gomobile.
//
// Design constraints (gomobile):
//   - All exported method parameters and return types must be gomobile-safe:
//     string, int, int64, float64, bool, []byte, error, or named struct pointers
//     whose fields are also gomobile-safe.
//   - No maps, []SomeStruct, time.Time, or interface values on exported methods.
//   - Complex data uses JSON strings (JSON in / JSON out pattern).
//
// Concurrency contract: each Core method is synchronous; the host must not call
// methods concurrently (one call at a time). SyncNow creates a fresh engine per
// call, so calling SyncNow while ResolveConflict is in flight is a misuse.
//
// Token contract: the host owns secure token storage (iOS Keychain /
// Android EncryptedSharedPrefs) and supplies the OAuth JSON token on every
// call that needs sync. The core never stores credentials to disk unless the
// host explicitly uses FileTokenStore.
package mobilecore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"golang.org/x/oauth2"

	"github.com/cristim/daily-progress-logger/internal/drive"
	"github.com/cristim/daily-progress-logger/internal/store"
	syncengine "github.com/cristim/daily-progress-logger/internal/sync"
)

const dateLayout = "2006-01-02"

// ErrCASMismatch is returned when the task at the given index no longer carries
// the expected display text (e.g. because a background Drive sync rewrote the
// file between the host's tree snapshot and the action). The host should call
// TreeJSON again and re-present the action to the user with the refreshed index.
var ErrCASMismatch = errors.New("task text mismatch: tree is stale, please refresh")

// Core is the app core over a data directory. Bound to Swift/Kotlin as an
// opaque handle; all interaction is through its methods.
type Core struct {
	store    *store.Store
	dir      string
	clientID string
	device   string
}

// Open returns a Core rooted at dataDir, or an error if the store cannot be
// initialised. clientID is the Google OAuth client ID (for sync); deviceID
// names this install in conflict copies.
func Open(dataDir, clientID, deviceID string) (*Core, error) {
	st, err := store.New(dataDir)
	if err != nil {
		return nil, fmt.Errorf("opening store at %s: %w", dataDir, err)
	}
	if err := st.MigrateRefTags(); err != nil {
		slog.Warn("ref tag migration failed; app continues with backward-compat parsing", "error", err)
	}
	return &Core{store: st, dir: dataDir, clientID: clientID, device: deviceID}, nil
}

// verifyIndex confirms that date's plan item at index still carries the expected
// display text (project tag stripped). Returns ErrCASMismatch when out of range
// or text changed. When expectedText is empty the check is skipped (no guard).
// On a read error the action is allowed through rather than silently blocked,
// matching Qt's taskIndexValid contract.
func (c *Core) verifyIndex(date time.Time, index int, expectedText string) error {
	if strings.TrimSpace(expectedText) == "" {
		return nil
	}
	known, err := c.store.KnownProjectIDs()
	if err != nil {
		//nolint:nilerr // documented fail-open contract (matches Qt taskIndexValid):
		// if the guard cannot read state it allows the action, and the store op re-checks.
		return nil
	}
	d, exists, err := c.store.LoadDaily(date)
	if err != nil || !exists {
		return ErrCASMismatch
	}
	if index < 0 || index >= len(d.Plan) {
		return ErrCASMismatch
	}
	if store.DisplayText(d.Plan[index], known) != strings.TrimSpace(expectedText) {
		return ErrCASMismatch
	}
	return nil
}

// engine builds a sync engine authenticated with the given token JSON.
func (c *Core) engine(tokenJSON string) (*syncengine.Engine, error) {
	var tok oauth2.Token
	if err := json.Unmarshal([]byte(tokenJSON), &tok); err != nil {
		return nil, fmt.Errorf("parsing token: %w", err)
	}
	cfg := drive.Config(c.clientID, "")
	httpClient, err := drive.HTTPClient(context.Background(), cfg, memTokenStore{tok: &tok})
	if err != nil {
		return nil, err
	}
	dc, err := drive.New(context.Background(), httpClient)
	if err != nil {
		return nil, err
	}
	return syncengine.New(c.dir, dc, c.device), nil
}

// memTokenStore is a non-persistent TokenStore; the mobile host supplies the
// token on each call so it can keep it in the platform secure store
// (iOS Keychain / Android EncryptedSharedPrefs).
type memTokenStore struct{ tok *oauth2.Token }

func (m memTokenStore) Load() (*oauth2.Token, error) { return m.tok, nil }
func (m memTokenStore) Save(_ *oauth2.Token) error   { return nil }

// parseDate parses "YYYY-MM-DD" in local time.
func parseDate(s string) (time.Time, error) {
	return time.ParseInLocation(dateLayout, s, time.Local)
}

// weekFromDate parses a date string and returns its ISO week.
func weekFromDate(s string) (store.WeekID, error) {
	d, err := parseDate(s)
	if err != nil {
		return store.WeekID{}, err
	}
	return store.WeekOf(d), nil
}

// Wire strings for ItemState, shared by parseState and stateString.
const (
	stateTodoStr      = "todo"
	stateDoneStr      = "done"
	statePostponedStr = "postponed"
)

// parseState maps the string state ("todo"/"done"/"postponed") to ItemState.
func parseState(s string) (store.ItemState, error) {
	switch s {
	case stateTodoStr:
		return store.StateTodo, nil
	case stateDoneStr:
		return store.StateDone, nil
	case statePostponedStr:
		return store.StatePostponed, nil
	}
	return store.StateTodo, fmt.Errorf("unknown state %q (want todo/done/postponed)", s)
}

// stateString maps ItemState back to its wire string.
func stateString(st store.ItemState) string {
	switch st {
	case store.StateDone:
		return stateDoneStr
	case store.StatePostponed:
		return statePostponedStr
	case store.StateTodo:
		return stateTodoStr
	default:
		return stateTodoStr
	}
}

// toJSON marshals v to a compact JSON string.
func toJSON(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
