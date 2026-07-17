package mobilecore

import (
	"context"

	syncengine "github.com/cristim/daily-progress-logger/internal/sync"
)

// SyncNow runs a Drive sync using the OAuth token JSON supplied by the host
// (from Google Sign-In on the device). Returns the conflicts found during the
// run as a JSON array (same schema as ConflictsJSON).
//
// The host is responsible for secure token storage (iOS Keychain /
// Android EncryptedSharedPrefs) and must pass a valid, unexpired token. The
// core does not persist the token to disk; call SyncWithFileToken if
// file-based persistence is wanted.
//
// Concurrency: one call at a time. Do not call SyncNow concurrently with
// ResolveConflict or ConflictsJSON — each builds a separate engine instance
// whose internal state does not synchronise with the others.
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

// ResolveConflict settles a conflict for the given file path. choice must be
// "keep_local", "keep_remote", or "keep_both".
func (c *Core) ResolveConflict(tokenJSON, path, choice string) error {
	engine, err := c.engine(tokenJSON)
	if err != nil {
		return err
	}
	return engine.Resolve(path, syncengine.ResolveChoice(choice))
}
