//go:build darwin && !ios

package drive

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os/exec"

	"golang.org/x/oauth2"
)

// KeychainStore persists the OAuth token as a generic password in the macOS
// login Keychain via the `security` CLI, so the refresh token never lands in the
// plaintext config file.
type KeychainStore struct {
	Service string
	Account string
}

// Load returns the stored token, or an error when none is saved.
func (k KeychainStore) Load() (*oauth2.Token, error) {
	out, err := exec.CommandContext(context.Background(), "security", "find-generic-password",
		"-s", k.Service, "-a", k.Account, "-w").Output()
	if err != nil {
		return nil, errors.New("no stored Google token")
	}
	var tok oauth2.Token
	if err := json.Unmarshal(bytes.TrimSpace(out), &tok); err != nil {
		return nil, err
	}
	return &tok, nil
}

// Save upserts the token in the Keychain.
func (k KeychainStore) Save(token *oauth2.Token) error {
	data, err := json.Marshal(token)
	if err != nil {
		return err
	}
	// -U updates the entry if it already exists.
	return exec.CommandContext(context.Background(), "security", "add-generic-password", "-U",
		"-s", k.Service, "-a", k.Account, "-w", string(data)).Run()
}

// Delete removes the stored token (sign-out). A missing entry is not an error.
func (k KeychainStore) Delete() error {
	_ = exec.CommandContext(context.Background(), "security", "delete-generic-password",
		"-s", k.Service, "-a", k.Account).Run()
	return nil
}

// HasToken reports whether a token is currently stored.
func (k KeychainStore) HasToken() bool {
	_, err := k.Load()
	return err == nil
}
