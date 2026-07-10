// Package mobilecore is the gomobile-bound surface shared by the iOS (and later
// Android) apps. It wraps the same internal/store model and internal/sync engine
// the desktop uses, exposing a small API of string/JSON in, string/JSON out so
// it binds cleanly through gomobile.
package mobilecore

import (
	"context"
	"encoding/json"
	"fmt"
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

// Open returns a Core rooted at dataDir. clientID is the Google OAuth client ID
// (for sync); deviceID names this install in conflict copies.
func Open(dataDir, clientID, deviceID string) *Core {
	return &Core{store: store.New(dataDir), dir: dataDir, clientID: clientID, device: deviceID}
}

// TreeJSON returns the Projects/Stories/tasks tree for date ("YYYY-MM-DD") as
// JSON (see store.ProjectTree).
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

// AddTask adds a task to date's plan. When storyID is non-empty the task is
// tagged to that story.
func (c *Core) AddTask(date, text, storyID string) error {
	d, err := parseDate(date)
	if err != nil {
		return err
	}
	if storyID == "" {
		return c.store.AddPlanItem(d, text)
	}
	return c.store.AddTaggedTask(d, text, storyID)
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
	idx, err := c.store.FindTaskIndex(d, text)
	if err != nil {
		return err
	}
	if idx < 0 {
		return fmt.Errorf("task %q not found on %s", text, date)
	}
	return c.store.SetPlanItemState(d, idx, st)
}

// DeleteTask deletes a task (by display text on date) to the recycle bin.
func (c *Core) DeleteTask(date, text string) error {
	d, err := parseDate(date)
	if err != nil {
		return err
	}
	idx, err := c.store.FindTaskIndex(d, text)
	if err != nil {
		return err
	}
	if idx < 0 {
		return fmt.Errorf("task %q not found on %s", text, date)
	}
	return c.store.DeleteTask(d, idx)
}

// AddProject creates a project and returns its ID.
func (c *Core) AddProject(name string) (string, error) {
	return c.store.AddProject(name)
}

// AddStory creates a story under projectID and returns its ID.
func (c *Core) AddStory(projectID, name string) (string, error) {
	return c.store.AddStory(projectID, name)
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

// engine builds a sync engine authenticated with the given token.
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
