package mobilecore

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/oauth2"

	"github.com/cristim/daily-progress-logger/internal/drive"
)

// FileTokenStore is a portable drive.TokenStore that persists the OAuth token
// as JSON in a file under the Core's data directory (mode 0600). It is an
// optional alternative to the host-managed token flow: when the host cannot
// conveniently persist the token between calls (e.g. a Go CLI on Linux where
// there is no system keychain), this store handles it automatically.
//
// Mobile hosts should prefer keeping the token in the platform secure store
// (iOS Keychain / Android EncryptedSharedPrefs) and pass it on each SyncNow
// call rather than using FileTokenStore. The file is NOT encrypted at rest.
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
