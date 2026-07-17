package mobilecore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/oauth2"

	"github.com/cristim/daily-progress-logger/internal/drive"
	syncengine "github.com/cristim/daily-progress-logger/internal/sync"
)

// FileTokenStore is a portable drive.TokenStore that persists the OAuth token
// as JSON in a file under the Core's data directory (mode 0600).
//
// Intended for CLI / server callers on systems that have no platform keychain.
// Mobile hosts should keep the token in the platform secure store (iOS Keychain /
// Android EncryptedSharedPrefs) and pass it on each SyncNow call instead.
// The file is NOT encrypted at rest.
//
// NOTE: FileTokenStore.Load and FileTokenStore.Save take and return *oauth2.Token,
// which is not a gomobile-compatible type. These methods are NOT part of the
// gomobile-bound mobile surface and cannot be called from Swift / Kotlin.
// Use SyncWithFileTokenStore (a Core method) for file-backed sync from Go code.
type FileTokenStore struct {
	path string
}

// Ensure FileTokenStore satisfies the drive.TokenStore interface at compile time.
var _ drive.TokenStore = (*FileTokenStore)(nil)

// NewFileTokenStore returns a FileTokenStore that reads and writes the token
// to tokenFilePath. The file is created with mode 0600 on first save.
// This constructor does not create the file; it is written on the first Save.
func NewFileTokenStore(tokenFilePath string) *FileTokenStore {
	return &FileTokenStore{path: tokenFilePath}
}

// Load reads the stored OAuth token. Returns an error wrapping
// os.ErrNotExist when no token has been saved yet.
func (f *FileTokenStore) Load() (*oauth2.Token, error) {
	data, err := os.ReadFile(f.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("no token stored at %s: %w", f.path, os.ErrNotExist)
	}
	if err != nil {
		return nil, fmt.Errorf("reading token from %s: %w", f.path, err)
	}
	var tok oauth2.Token
	if err := json.Unmarshal(data, &tok); err != nil {
		return nil, fmt.Errorf("parsing token from %s: %w", f.path, err)
	}
	return &tok, nil
}

// Save writes the token to the file atomically (tmp+rename) with mode 0600.
func (f *FileTokenStore) Save(tok *oauth2.Token) error {
	data, err := json.MarshalIndent(tok, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding token: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(f.path), 0o750); err != nil {
		return fmt.Errorf("creating token dir: %w", err)
	}
	tmp := f.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("writing token to %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, f.path); err != nil {
		return fmt.Errorf("replacing token file %s: %w", f.path, err)
	}
	return nil
}

// TokenFilePath returns the default path for the token file under dataDir.
// Callers pass this to NewFileTokenStore.
func TokenFilePath(dataDir string) string {
	return filepath.Join(dataDir, ".oauth-token.json")
}

// SyncWithFileTokenStore runs a Drive sync using a token persisted in a file.
// tokenFilePath must point to an existing file containing a valid OAuth JSON
// token (e.g. one previously written by FileTokenStore.Save).  Refreshed tokens
// are written back to the file automatically via FileTokenStore.Save.
//
// This method is intended for CLI / server callers running as pure Go; it does
// NOT require a platform keychain.  Mobile hosts should use SyncNow instead
// (which receives the token from the host's secure store on every call).
//
// Returns a syncResultDTO JSON string with an empty "token" field (the file
// store handles persistence).
func (c *Core) SyncWithFileTokenStore(tokenFilePath string) (string, error) {
	ts := NewFileTokenStore(tokenFilePath)

	c.mu.Lock()
	defer c.mu.Unlock()

	cfg := drive.Config(c.clientID, "")
	httpClient, err := drive.HTTPClient(context.Background(), cfg, ts)
	if err != nil {
		return "", fmt.Errorf("%s: %w", ErrCodeSyncAuth, err)
	}
	dc, err := drive.New(context.Background(), httpClient)
	if err != nil {
		return "", fmt.Errorf("%s: %w", ErrCodeSyncAuth, err)
	}
	engine := syncengine.New(c.dir, dc, c.device)

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
	// Token field is omitted: file store writes refreshed tokens itself.
	return toJSON(syncResultDTO{Conflicts: conflicts})
}
