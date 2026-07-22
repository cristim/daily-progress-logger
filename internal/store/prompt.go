package store

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DailyPromptPath returns the path of the daily-prompt file.
func (s *Store) DailyPromptPath() string {
	return filepath.Join(s.DataDir, "daily-prompt.md")
}

// DailyPrompt reads daily-prompt.md; a missing or empty file means the
// prompt is unset, returned as "".
func (s *Store) DailyPrompt() (string, error) {
	content, err := os.ReadFile(s.DailyPromptPath())
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("reading daily prompt: %w", err)
	}
	return strings.TrimSpace(string(content)), nil
}

// SetDailyPrompt writes daily-prompt.md. Writing an empty (or
// whitespace-only) string clears the prompt back to unset.
func (s *Store) SetDailyPrompt(text string) error {
	return writeFile(s.DailyPromptPath(), strings.TrimSpace(text))
}
