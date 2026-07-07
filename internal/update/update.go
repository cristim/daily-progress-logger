// Package update checks GitHub releases for a newer version of the app.
// Network failures are non-fatal: callers should log them at debug level
// so an offline machine or a private repo never produces error dialogs.
package update

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// DefaultReleaseURL is the GitHub releases API endpoint for this project.
const DefaultReleaseURL = "https://api.github.com/repos/cristim/daily-progress-logger/releases/latest"

// releaseResponse is the subset of the GitHub releases API that we need.
type releaseResponse struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
}

// Check fetches the latest release from releaseEndpoint and compares it with
// current. It returns the latest tag, whether it is newer than current, and
// the release HTML URL. Errors are propagated so callers can decide how to
// handle them (typically: slog.Debug only).
//
// Pass DefaultReleaseURL for normal use; supply a different URL in tests.
func Check(ctx context.Context, current, releaseEndpoint string) (latest string, newer bool, releasePageURL string, err error) {
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, releaseEndpoint, nil)
	if err != nil {
		return "", false, "", fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := client.Do(req)
	if err != nil {
		return "", false, "", fmt.Errorf("fetching release: %w", err)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			slog.Debug("update: closing response body", "error", cerr)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return "", false, "", fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}

	var rel releaseResponse
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return "", false, "", fmt.Errorf("decoding response: %w", err)
	}

	latest = rel.TagName
	newer = isNewer(current, latest)
	return latest, newer, rel.HTMLURL, nil
}

// isNewer reports whether latestTag is strictly newer than currentTag.
// Leading "v" is stripped before comparison. The string "dev" is never
// considered outdated. Parts are compared numerically; missing parts are
// treated as zero.
func isNewer(currentTag, latestTag string) bool {
	current := stripV(currentTag)
	if current == "dev" || current == "" {
		return false
	}
	latest := stripV(latestTag)
	cParts := versionParts(current)
	lParts := versionParts(latest)
	maxLen := max(len(cParts), len(lParts))
	for i := range maxLen {
		c := partAt(cParts, i)
		l := partAt(lParts, i)
		if l > c {
			return true
		}
		if l < c {
			return false
		}
	}
	return false
}

func stripV(s string) string {
	return strings.TrimPrefix(s, "v")
}

func versionParts(s string) []int {
	fields := strings.Split(s, ".")
	parts := make([]int, 0, len(fields))
	for _, f := range fields {
		n, _ := strconv.Atoi(f)
		parts = append(parts, n)
	}
	return parts
}

func partAt(parts []int, i int) int {
	if i < len(parts) {
		return parts[i]
	}
	return 0
}
