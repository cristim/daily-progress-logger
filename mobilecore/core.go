// Package mobilecore is the gomobile-bound mobile API shared by the iOS and Android apps.
// It wraps the same internal/store model and internal/sync engine the desktop uses,
// exposing a small API of string/JSON in, string/JSON out so it binds cleanly through gomobile.
//
// Design constraints (gomobile):
//   - All exported method parameters and return types must be gomobile-safe:
//     string, int, int64, float64, bool, []byte, error, or named struct pointers
//     whose fields are also gomobile-safe.
//   - No maps, []SomeStruct, time.Time, or interface values on exported methods.
//   - Complex data uses JSON strings (JSON in / JSON out pattern).
//
// Concurrency contract: Core is safe for concurrent use from multiple goroutines.
// All public methods are serialised by an internal mutex so the host may call
// them from any thread.  SyncNow holds the lock for the duration of the network
// sync; overlapping user-action calls will queue behind it.
//
// Token contract: the host owns secure token storage (iOS Keychain /
// Android EncryptedSharedPrefs) and supplies the OAuth JSON token on every
// call that needs sync.  Refreshed tokens are returned in the SyncNow result
// envelope (see syncResultDTO); the host MUST persist the updated token back to
// secure storage after every SyncNow call that returns a non-empty token field.
//
// Error codes: every error returned by a Core method carries a stable code
// prefix (ErrCodeCASMismatch, ErrCodeNotFound, ErrCodeBadInput, ErrCodeSyncAuth,
// ErrCodeInternal).  Use ClassifyError to extract the code for branching.
// See dto.go for the full contract.
package mobilecore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"

	"github.com/cristim/daily-progress-logger/internal/drive"
	"github.com/cristim/daily-progress-logger/internal/store"
	syncengine "github.com/cristim/daily-progress-logger/internal/sync"
)

const dateLayout = "2006-01-02"

// mobileConfigFileName is the config file stored under the Core's data directory.
// Mobile-relevant settings (check-in times, notify flag, OAuth client ID) live here.
// Desktop-specific settings (data_dir, shortcuts, …) are absent by design.
const mobileConfigFileName = "mobile-config.json"

// ErrCASMismatch is returned when the task at the given index no longer carries
// the expected display text (e.g. because a background Drive sync rewrote the
// file between the host's tree snapshot and the action). The host should call
// TreeJSON again and re-present the action to the user with the refreshed index.
//
// The error message carries the stable prefix "CAS_MISMATCH: " so hosts can
// detect it by prefix-matching across the gomobile boundary (errors.Is is not
// available from Swift/Kotlin).  Use ClassifyError to extract the code.
var ErrCASMismatch = errors.New("CAS_MISMATCH: tree is stale, please refresh")

// Core is the app core over a data directory. Bound to Swift/Kotlin as an
// opaque handle; all interaction is through its methods.
type Core struct {
	mu       sync.Mutex
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
		_ = err // non-fatal migration failure; already logged inside MigrateRefTags
	}
	return &Core{store: st, dir: dataDir, clientID: clientID, device: deviceID}, nil
}

// mobileConfigPath returns the path of the mobile-specific config file.
func (c *Core) mobileConfigPath() string {
	return filepath.Join(c.dir, mobileConfigFileName)
}

// loadMobileConfig reads the mobile config from the data directory.
// A missing file returns an empty (defaults) config; a parse error returns a
// BAD_INPUT coded error so the host knows the config is corrupt (not just absent).
func (c *Core) loadMobileConfig() (*mobileConfigDTO, error) {
	data, err := os.ReadFile(c.mobileConfigPath())
	if errors.Is(err, os.ErrNotExist) {
		return &mobileConfigDTO{}, nil // first-run: use defaults
	}
	if err != nil {
		return nil, fmt.Errorf("%s: reading mobile config: %w", ErrCodeBadInput, err)
	}
	var cfg mobileConfigDTO
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("%s: parsing mobile config: %w", ErrCodeBadInput, err)
	}
	return &cfg, nil
}

// saveMobileConfig writes the mobile config atomically (tmp+rename, mode 0600).
func (c *Core) saveMobileConfig(cfg *mobileConfigDTO) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("%s: encoding mobile config: %w", ErrCodeInternal, err)
	}
	tmp := c.mobileConfigPath() + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("%s: writing mobile config: %w", ErrCodeInternal, err)
	}
	if err := os.Rename(tmp, c.mobileConfigPath()); err != nil {
		return fmt.Errorf("%s: replacing mobile config: %w", ErrCodeInternal, err)
	}
	return nil
}

// verifyIndex confirms that date's plan item at index still carries the expected
// display text (project tag stripped). Returns ErrCASMismatch when out of range
// or text changed. When expectedText is empty the check is skipped (no guard).
//
// On a KnownProjectIDs read error the action is allowed through (fail-open),
// matching Qt's taskIndexValid contract for non-destructive operations.
// Use verifyIndexStrict for destructive ops (DeleteTask, PostponeToNextDay,
// MoveTaskToBacklog) where fail-open could destroy the wrong item.
func (c *Core) verifyIndex(date time.Time, index int, expectedText string) error {
	if strings.TrimSpace(expectedText) == "" {
		return nil
	}
	known, err := c.store.KnownProjectIDs()
	if err != nil {
		//nolint:nilerr // documented fail-open contract (matches Qt taskIndexValid):
		// if the guard cannot read project state it allows the action through;
		// the store op re-checks bounds. For destructive ops use verifyIndexStrict.
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

// verifyIndexStrict is like verifyIndex but fails CLOSED on a KnownProjectIDs
// read error instead of fail-open. Use for destructive operations
// (DeleteTask, PostponeToNextDay, MoveTaskToBacklog) where a corrupt/missing
// projects file could otherwise cause the wrong item to be destroyed or moved.
func (c *Core) verifyIndexStrict(date time.Time, index int, expectedText string) error {
	if strings.TrimSpace(expectedText) == "" {
		return nil
	}
	known, err := c.store.KnownProjectIDs()
	if err != nil {
		// Fail CLOSED: cannot verify the index safely; refuse the action.
		// The host should call TreeJSON to re-read current state and retry.
		return ErrCASMismatch
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

// memTokenStore captures the OAuth token for the current request and records
// any refresh that happens mid-call (e.g. when the access token expires during
// SyncNow).  The host supplies a fresh token on every call; we capture updates
// so they can be returned in the SyncNow response envelope.
type memTokenStore struct {
	mu      sync.Mutex
	tok     *oauth2.Token
	updated bool
}

func newMemTokenStore(tok *oauth2.Token) *memTokenStore {
	return &memTokenStore{tok: tok}
}

func (m *memTokenStore) Load() (*oauth2.Token, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.tok, nil
}

// Save captures a refreshed token (called by drive.savingSource when Google
// rotates the access token mid-call).  The updated token is returned to the
// host in the SyncNow response; the host must persist it to secure storage.
func (m *memTokenStore) Save(tok *oauth2.Token) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tok = tok
	m.updated = true
	return nil
}

// updatedJSON returns the updated token as a JSON string if Save was called
// during the request, or "" if the token was not refreshed.
func (m *memTokenStore) updatedJSON() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.updated || m.tok == nil {
		return ""
	}
	b, err := json.Marshal(m.tok)
	if err != nil {
		return "" // oauth2.Token is always JSON-safe; this branch is unreachable in practice
	}
	return string(b)
}

// engineWithStore builds a sync engine authenticated with the given token JSON
// and returns it together with the memTokenStore so the caller can retrieve
// any refreshed token after the call.
func (c *Core) engineWithStore(tokenJSON string) (*syncengine.Engine, *memTokenStore, error) {
	var tok oauth2.Token
	if err := json.Unmarshal([]byte(tokenJSON), &tok); err != nil {
		return nil, nil, fmt.Errorf("%s: parsing token: %w", ErrCodeBadInput, err)
	}
	ts := newMemTokenStore(&tok)
	cfg := drive.Config(c.clientID, "")
	httpClient, err := drive.HTTPClient(context.Background(), cfg, ts)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", ErrCodeSyncAuth, err)
	}
	dc, err := drive.New(context.Background(), httpClient)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", ErrCodeSyncAuth, err)
	}
	return syncengine.New(c.dir, dc, c.device), ts, nil
}

// parseDate parses "YYYY-MM-DD" in local time.
// A parse failure is wrapped as a BAD_INPUT coded error so every date-taking
// Core method surfaces a ClassifyError-detectable code to the host.
func parseDate(s string) (time.Time, error) {
	t, err := time.ParseInLocation(dateLayout, s, time.Local)
	if err != nil {
		return time.Time{}, fmt.Errorf("%s: invalid date %q (want YYYY-MM-DD): %w", ErrCodeBadInput, s, err)
	}
	return t, nil
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
	return store.StateTodo, fmt.Errorf("%s: unknown state %q (want todo/done/postponed)", ErrCodeBadInput, s)
}

// stateString maps ItemState back to its wire string.
// Panics on unknown values — an unknown value means a new enum member was
// added without updating this mapping; that is a programming error, not a
// runtime condition (fail-loud per coding standards).
func stateString(st store.ItemState) string {
	switch st {
	case store.StateDone:
		return stateDoneStr
	case store.StatePostponed:
		return statePostponedStr
	case store.StateTodo:
		return stateTodoStr
	default:
		panic(fmt.Sprintf("mobilecore: unknown ItemState %d — update stateString", int(st)))
	}
}

// codeStoreErr maps store sentinel not-found errors to a NOT_FOUND coded
// error so hosts can branch on ClassifyError across the gomobile boundary.
// Errors wrapping store.ErrProjectNotFound or store.ErrBacklogItemNotFound
// are wrapped with the ErrCodeNotFound prefix; all other errors pass through
// unchanged.
func codeStoreErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, store.ErrProjectNotFound) || errors.Is(err, store.ErrBacklogItemNotFound) {
		return fmt.Errorf("%s: %w", ErrCodeNotFound, err)
	}
	return err
}

// toJSON marshals v to a compact JSON string.
func toJSON(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
