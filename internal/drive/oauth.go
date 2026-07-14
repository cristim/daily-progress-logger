package drive

import (
	"context"
	"net/http"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// Config builds the OAuth2 config for the drive.file scope from an installed-app
// client ID. The desktop flow uses a loopback redirect + PKCE, so no client
// secret is embedded.
func Config(clientID, redirectURL string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:    clientID,
		Endpoint:    google.Endpoint,
		Scopes:      []string{Scope},
		RedirectURL: redirectURL,
	}
}

// TokenStore persists the OAuth token across runs (macOS Keychain on desktop,
// the app's secure store on mobile).
type TokenStore interface {
	Load() (*oauth2.Token, error)
	Save(token *oauth2.Token) error
}

// HTTPClient returns an authenticated HTTP client that auto-refreshes using the
// stored token, writing refreshed tokens back to the store so the refresh token
// survives restarts.
func HTTPClient(ctx context.Context, cfg *oauth2.Config, store TokenStore) (*http.Client, error) {
	tok, err := store.Load()
	if err != nil {
		return nil, err
	}
	return oauth2.NewClient(ctx, &savingSource{store: store, src: cfg.TokenSource(ctx, tok)}), nil
}

// savingSource wraps a TokenSource, persisting a token whenever it is refreshed.
type savingSource struct {
	store TokenStore
	src   oauth2.TokenSource
	last  string
}

func (s *savingSource) Token() (*oauth2.Token, error) {
	tok, err := s.src.Token()
	if err != nil {
		return nil, err
	}
	if tok.AccessToken != s.last {
		_ = s.store.Save(tok)
		s.last = tok.AccessToken
	}
	return tok, nil
}
