//go:build !ios

package drive

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// emailScope lets the sign-in fetch the account email for display; it is
// non-sensitive and does not trigger Google verification.
const emailScope = "https://www.googleapis.com/auth/userinfo.email"

// SignIn runs the desktop OAuth flow: it starts a loopback server, opens the
// browser to Google's consent screen with PKCE, exchanges the returned code,
// saves the token via store, and returns the signed-in account email. It blocks
// until the user completes consent or ctx is cancelled.
func SignIn(ctx context.Context, clientID string, store TokenStore) (string, error) {
	ln, err := (&net.ListenConfig{}).Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("opening loopback listener: %w", err)
	}
	defer func() { _ = ln.Close() }()

	addr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		return "", errors.New("unexpected loopback address type")
	}
	cfg := &oauth2.Config{
		ClientID:    clientID,
		Endpoint:    google.Endpoint,
		Scopes:      []string{Scope, emailScope},
		RedirectURL: fmt.Sprintf("http://127.0.0.1:%d/callback", addr.Port),
	}

	state := randomToken()
	verifier := oauth2.GenerateVerifier()
	authURL := cfg.AuthCodeURL(state,
		oauth2.AccessTypeOffline, oauth2.ApprovalForce,
		oauth2.S256ChallengeOption(verifier))

	codeCh, errCh := make(chan string, 1), make(chan error, 1)
	srv := &http.Server{ReadHeaderTimeout: 5 * time.Second, Handler: callbackHandler(state, codeCh, errCh)}
	go func() { _ = srv.Serve(ln) }()
	//nolint:contextcheck // shutdown runs after the flow ends, when ctx may already be cancelled
	defer func() { _ = srv.Shutdown(context.Background()) }()

	if err := exec.CommandContext(ctx, "open", authURL).Start(); err != nil {
		return "", fmt.Errorf("opening browser: %w", err)
	}

	var code string
	select {
	case code = <-codeCh:
	case err := <-errCh:
		return "", err
	case <-ctx.Done():
		return "", ctx.Err()
	}

	token, err := cfg.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		return "", fmt.Errorf("exchanging code: %w", err)
	}
	if err := store.Save(token); err != nil {
		return "", err
	}
	return fetchEmail(ctx, cfg.Client(ctx, token)), nil
}

func callbackHandler(state string, codeCh chan<- string, errCh chan<- error) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if e := q.Get("error"); e != "" {
			errCh <- fmt.Errorf("authorization denied: %s", e)
			http.Error(w, "Authorization failed. You can close this tab.", http.StatusBadRequest)
			return
		}
		if q.Get("state") != state {
			errCh <- errors.New("state mismatch")
			http.Error(w, "State mismatch. You can close this tab.", http.StatusBadRequest)
			return
		}
		codeCh <- q.Get("code")
		_, _ = w.Write([]byte("Signed in to Daily Progress Logger. You can close this tab."))
	})
}

// fetchEmail best-effort reads the account email; an empty result is fine.
func fetchEmail(ctx context.Context, client *http.Client) string {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://www.googleapis.com/oauth2/v3/userinfo", nil)
	if err != nil {
		return ""
	}
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer func() { _ = resp.Body.Close() }()
	var info struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return ""
	}
	return info.Email
}

func randomToken() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
