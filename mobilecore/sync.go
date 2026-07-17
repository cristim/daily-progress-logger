package mobilecore

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	syncengine "github.com/cristim/daily-progress-logger/internal/sync"
)

// SyncNow runs a Drive sync using the OAuth token JSON supplied by the host
// (from Google Sign-In on the device).
//
// Response (syncResultDTO, see dto.go):
//
//	{
//	  "conflicts": [...],  // new conflicts detected during the run; [] if none
//	  "token":     "…"    // updated OAuth JSON when access token was refreshed,
//	                      // or "" when unchanged.  Host MUST persist non-empty
//	                      // values back to Keychain / EncryptedSharedPrefs.
//	}
//
// The host is responsible for secure token storage (iOS Keychain /
// Android EncryptedSharedPrefs) and must pass a valid, unexpired token.
// The core does not persist the token to disk; refreshed tokens are surfaced
// in the response envelope.
func (c *Core) SyncNow(tokenJSON string) (string, error) {
	// Validate token JSON before acquiring the lock or starting network I/O.
	if err := validateTokenJSON(tokenJSON); err != nil {
		return "", err
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	engine, ts, err := c.engineWithStore(tokenJSON)
	if err != nil {
		return "", err
	}
	res, err := engine.Run(context.Background())
	if err != nil {
		return "", fmt.Errorf("%s: %w", ErrCodeSyncAuth, err)
	}

	conflicts := make([]conflictDTO, len(res.Conflicts))
	for i, cf := range res.Conflicts {
		conflicts[i] = conflictDTO{
			Path:         cf.Path,
			ConflictCopy: cf.ConflictCopy,
			Time:         cf.Time.Format(time.RFC3339),
		}
	}
	return toJSON(syncResultDTO{
		Conflicts: conflicts,
		Token:     ts.updatedJSON(),
	})
}

// ConflictsJSON returns unresolved conflicts recorded locally, as a JSON array
// of conflictDTO objects (see dto.go).  Returns "[]" when there are no conflicts.
func (c *Core) ConflictsJSON(tokenJSON string) (string, error) {
	if err := validateTokenJSON(tokenJSON); err != nil {
		return "", err
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	engine, _, err := c.engineWithStore(tokenJSON)
	if err != nil {
		return "", err
	}
	raw, err := engine.Conflicts()
	if err != nil {
		return "", err
	}
	out := make([]conflictDTO, len(raw)) // always [] not null
	for i, cf := range raw {
		out[i] = conflictDTO{
			Path:         cf.Path,
			ConflictCopy: cf.ConflictCopy,
			Time:         cf.Time.Format(time.RFC3339),
		}
	}
	return toJSON(out)
}

// ResolveConflict settles a conflict for the given file path.
// choice must be exactly one of: "keep_local", "keep_remote", "keep_both".
// An unknown choice returns a BAD_INPUT coded error — no silent data decisions.
func (c *Core) ResolveConflict(tokenJSON, path, choice string) error {
	// Validate choice before any network call so the error is cheaply testable.
	switch choice {
	case string(syncengine.KeepLocal), string(syncengine.KeepRemote), string(syncengine.KeepBoth):
		// valid
	default:
		return fmt.Errorf("%s: unknown conflict resolution choice %q"+
			" (want keep_local / keep_remote / keep_both)", ErrCodeBadInput, choice)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	engine, _, err := c.engineWithStore(tokenJSON)
	if err != nil {
		return err
	}
	return engine.Resolve(path, syncengine.ResolveChoice(choice))
}

// validateTokenJSON checks that tokenJSON is parseable as a JSON object.
// This is a cheap pre-flight check before acquiring the lock or making any
// network call; malformed JSON is always a host programming error.
func validateTokenJSON(tokenJSON string) error {
	if tokenJSON == "" {
		return fmt.Errorf("%s: tokenJSON must not be empty", ErrCodeBadInput)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(tokenJSON), &raw); err != nil {
		return fmt.Errorf("%s: tokenJSON is not valid JSON: %w", ErrCodeBadInput, err)
	}
	return nil
}
