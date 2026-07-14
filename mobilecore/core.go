// Package mobilecore is the gomobile-bound surface shared by the iOS (and later
// Android) apps. It wraps the same internal/store model and internal/sync engine
// the desktop uses, exposing a small API of string/JSON in, string/JSON out so
// it binds cleanly through gomobile.
package mobilecore

import (
	"context"
	"encoding/json"
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

// Core is the app core over a data directory. Bound to Swift as an opaque
// handle; all interaction is through its methods.
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

// TreeJSON returns the Projects/tasks tree for date ("YYYY-MM-DD") as JSON
// (see store.ProjectTree).
func (c *Core) TreeJSON(date string) (string, error) {
	d, err := parseDate(date)
	if err != nil {
		return "", err
	}
	tree, err := c.store.BuildProjectTree(d)
	if err != nil {
		return "", err
	}
	return toJSON(tree)
}

// AddTask adds a task to date's plan. When projectID is non-empty the task is
// tagged to that project.
func (c *Core) AddTask(date, text, projectID string) error {
	d, err := parseDate(date)
	if err != nil {
		return err
	}
	if projectID == "" {
		return c.store.AddPlanItem(d, text)
	}
	return c.store.AddTaggedTask(d, text, projectID)
}

// SetTaskState sets a task's state ("todo", "done", or "postponed") by its
// display text on date.
func (c *Core) SetTaskState(date, text, state string) error {
	d, err := parseDate(date)
	if err != nil {
		return err
	}
	st, err := parseState(state)
	if err != nil {
		return err
	}
	idx, err := c.findByDisplayText(d, text)
	if err != nil {
		return err
	}
	return c.store.SetPlanItemState(d, idx, st)
}

// DeleteTask deletes a task (by display text on date) to the recycle bin.
func (c *Core) DeleteTask(date, text string) error {
	d, err := parseDate(date)
	if err != nil {
		return err
	}
	idx, err := c.findByDisplayText(d, text)
	if err != nil {
		return err
	}
	return c.store.DeleteTask(d, idx)
}

// AddProject creates a project and returns its ID.
func (c *Core) AddProject(name string) (string, error) {
	return c.store.AddProject(name)
}

// SyncNow runs a Drive sync using the OAuth token JSON (from Google Sign-In on
// device) and returns the run's conflicts as JSON.
func (c *Core) SyncNow(tokenJSON string) (string, error) {
	engine, err := c.engine(tokenJSON)
	if err != nil {
		return "", err
	}
	res, err := engine.Run(context.Background())
	if err != nil {
		return "", err
	}
	return toJSON(res.Conflicts)
}

// ConflictsJSON returns unresolved conflicts recorded locally, as JSON.
func (c *Core) ConflictsJSON(tokenJSON string) (string, error) {
	engine, err := c.engine(tokenJSON)
	if err != nil {
		return "", err
	}
	conflicts, err := engine.Conflicts()
	if err != nil {
		return "", err
	}
	return toJSON(conflicts)
}

// ResolveConflict settles the conflict for path ("keep_local"/"keep_remote"/
// "keep_both").
func (c *Core) ResolveConflict(tokenJSON, path, choice string) error {
	engine, err := c.engine(tokenJSON)
	if err != nil {
		return err
	}
	return engine.Resolve(path, syncengine.ResolveChoice(choice))
}

// findByDisplayText returns the plan-item index whose display text (project tag
// stripped) matches text on date, or an error when the task is not found.
func (c *Core) findByDisplayText(date time.Time, text string) (int, error) {
	known, err := c.store.KnownProjectIDs()
	if err != nil {
		return -1, err
	}
	d, exists, err := c.store.LoadDaily(date)
	if err != nil {
		return -1, err
	}
	if !exists {
		return -1, fmt.Errorf("task %q not found on %s: no daily file", text, date.Format(dateLayout))
	}
	needle := strings.TrimSpace(text)
	for i, item := range d.Plan {
		if store.DisplayText(item, known) == needle {
			return i, nil
		}
	}
	return -1, fmt.Errorf("task %q not found on %s", text, date.Format(dateLayout))
}

// engine builds a sync engine authenticated with the given token.
// Concurrency contract: each call creates a fresh engine; the iOS host must
// not call SyncNow and ResolveConflict (or ConflictsJSON) concurrently —
// they would use distinct engine instances whose mutexes don't share state.
// The typical host pattern (one call at a time from Swift) is safe (M4).
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

// memTokenStore is a non-persistent TokenStore; the mobile app owns token
// persistence (iOS Keychain) and re-supplies the token each launch.
type memTokenStore struct{ tok *oauth2.Token }

func (m memTokenStore) Load() (*oauth2.Token, error) { return m.tok, nil }
func (m memTokenStore) Save(_ *oauth2.Token) error   { return nil }

func parseDate(s string) (time.Time, error) {
	return time.ParseInLocation(dateLayout, s, time.Local)
}

func parseState(s string) (store.ItemState, error) {
	switch s {
	case "todo":
		return store.StateTodo, nil
	case "done":
		return store.StateDone, nil
	case "postponed":
		return store.StatePostponed, nil
	}
	return store.StateTodo, fmt.Errorf("unknown state %q", s)
}

func toJSON(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
