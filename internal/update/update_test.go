package update

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsNewer(t *testing.T) {
	t.Parallel()
	tests := []struct {
		current string
		latest  string
		want    bool
	}{
		// dev is never outdated.
		{"dev", "v1.0.0", false},
		{"dev", "0.1.0", false},
		// Same version.
		{"1.0.0", "v1.0.0", false},
		{"v1.0.0", "1.0.0", false},
		// Strictly newer.
		{"0.1.0", "0.2.0", true},
		{"0.1.0", "1.0.0", true},
		{"1.0.0", "1.0.1", true},
		{"1.2.3", "1.3.0", true},
		// Strictly older.
		{"1.0.0", "0.9.9", false},
		{"2.0.0", "1.9.9", false},
		// Missing patch part treated as 0.
		{"1.0", "1.0.1", true},
		{"1.0.1", "1.0", false},
		// Empty current: not outdated.
		{"", "1.0.0", false},
	}
	for _, tt := range tests {
		t.Run(tt.current+"_vs_"+tt.latest, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, isNewer(tt.current, tt.latest))
		})
	}
}

func makeReleaseServer(t *testing.T, payload releaseResponse) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	}))
}

func TestCheck_Server(t *testing.T) {
	t.Parallel()
	payload := releaseResponse{TagName: "v1.2.3", HTMLURL: "https://example.com/v1.2.3"}
	srv := makeReleaseServer(t, payload)
	defer srv.Close()

	latest, newer, url, err := Check(context.Background(), "1.0.0", srv.URL)
	require.NoError(t, err)
	assert.Equal(t, "v1.2.3", latest)
	assert.True(t, newer)
	assert.Equal(t, payload.HTMLURL, url)
}

func TestCheck_NotNewer(t *testing.T) {
	t.Parallel()
	payload := releaseResponse{TagName: "v1.0.0", HTMLURL: "https://example.com/v1.0.0"}
	srv := makeReleaseServer(t, payload)
	defer srv.Close()

	_, newer, _, err := Check(context.Background(), "1.0.0", srv.URL)
	require.NoError(t, err)
	assert.False(t, newer)
}

func TestCheck_HTTPError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	latest, newer, url, checkErr := Check(context.Background(), "1.0.0", srv.URL)
	// The return values are meaningless on error; assert error first.
	require.ErrorContains(t, checkErr, "404")
	assert.Empty(t, latest)
	assert.False(t, newer)
	assert.Empty(t, url)
}

func TestCheck_DevVersion(t *testing.T) {
	t.Parallel()
	payload := releaseResponse{TagName: "v9.9.9", HTMLURL: "https://example.com/v9.9.9"}
	srv := makeReleaseServer(t, payload)
	defer srv.Close()

	_, newer, _, err := Check(context.Background(), "dev", srv.URL)
	require.NoError(t, err)
	assert.False(t, newer, "dev version must never be considered outdated")
}
