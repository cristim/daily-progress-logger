//go:build darwin && !ios

package drive

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"golang.org/x/oauth2"
)

// errKeychainNotFound is the sentinel returned by Load when no entry exists.
// Other errors (locked keychain, permission denied) are propagated unwrapped so
// callers can distinguish "not signed in" from "Keychain inaccessible" (M2).
var errKeychainNotFound = errors.New("no stored Google token")

// KeychainStore persists the OAuth token as a generic password in the macOS
// login Keychain via the `security` CLI, so the refresh token never lands in the
// plaintext config file.
type KeychainStore struct {
	Service string
	Account string
}

// Load returns the stored token. It returns errKeychainNotFound when no entry
// exists (normal "not signed in" state), and a wrapped error for any other
// failure (e.g. locked Keychain) so callers can distinguish the two cases.
func (k KeychainStore) Load() (*oauth2.Token, error) {
	cmd := exec.CommandContext(context.Background(), "security", "find-generic-password",
		"-s", k.Service, "-a", k.Account, "-w")
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			// exit 44 = errSecItemNotFound: the item simply isn't there yet.
			if exitErr.ExitCode() == 44 {
				return nil, errKeychainNotFound
			}
			// Keychain locked, permission denied, or other genuine failure;
			// propagate so the caller can surface it to the user.
			return nil, fmt.Errorf("keychain load: %w (stderr: %s)",
				err, bytes.TrimSpace(exitErr.Stderr))
		}
		return nil, fmt.Errorf("keychain load: %w", err)
	}
	var tok oauth2.Token
	if err := json.Unmarshal(bytes.TrimSpace(out), &tok); err != nil {
		return nil, fmt.Errorf("keychain: malformed token JSON: %w", err)
	}
	return &tok, nil
}

// Save upserts the token in the Keychain. The token JSON is fed via stdin to
// security's interactive mode (-i) so it never appears in the process argument
// list (which would be visible to other same-user processes via ps). JSON never
// contains single-quote characters, so single-quoting the value is safe.
func (k KeychainStore) Save(token *oauth2.Token) error {
	data, err := json.Marshal(token)
	if err != nil {
		return err
	}
	// -i: read commands from stdin; exit code is the exit code of the last command.
	cmd := exec.CommandContext(context.Background(), "security", "-i")
	cmd.Stdin = strings.NewReader(fmt.Sprintf(
		"add-generic-password -U -s '%s' -a '%s' -w '%s'\n",
		k.Service, k.Account, string(data),
	))
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("keychain save: %s: %w", bytes.TrimSpace(out), err)
	}
	return nil
}

// Delete removes the stored token (sign-out). A missing entry is not an error.
func (k KeychainStore) Delete() error {
	_ = exec.CommandContext(context.Background(), "security", "delete-generic-password",
		"-s", k.Service, "-a", k.Account).Run()
	return nil
}

// HasToken reports whether a token is currently stored. A non-accessible
// Keychain (locked, permission denied) returns false; the error is not surfaced
// here but will appear on the next Load call (e.g. from HTTPClient).
func (k KeychainStore) HasToken() bool {
	_, err := k.Load()
	return err == nil
}
