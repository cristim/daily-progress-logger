package store

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStore_DailyPromptMissingIsUnset(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	p, err := s.DailyPrompt()
	require.NoError(t, err)
	assert.Empty(t, p)
}

func TestStore_DailyPromptRoundTrip(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	require.NoError(t, s.SetDailyPrompt("What will move the needle today?"))

	p, err := s.DailyPrompt()
	require.NoError(t, err)
	assert.Equal(t, "What will move the needle today?", p)

	content, err := os.ReadFile(s.DailyPromptPath())
	require.NoError(t, err)
	assert.Equal(t, "What will move the needle today?", string(content))
}

func TestStore_DailyPromptOverwrite(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	require.NoError(t, s.SetDailyPrompt("first prompt"))
	require.NoError(t, s.SetDailyPrompt("second prompt"))

	p, err := s.DailyPrompt()
	require.NoError(t, err)
	assert.Equal(t, "second prompt", p)
}

func TestStore_DailyPromptWhitespaceTrimmed(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	require.NoError(t, s.SetDailyPrompt("  padded prompt  \n\n"))

	p, err := s.DailyPrompt()
	require.NoError(t, err)
	assert.Equal(t, "padded prompt", p)
}

func TestStore_DailyPromptClearedByEmptyString(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	require.NoError(t, s.SetDailyPrompt("something"))
	require.NoError(t, s.SetDailyPrompt(""))

	p, err := s.DailyPrompt()
	require.NoError(t, err)
	assert.Empty(t, p)
}

func TestStore_DailyPromptClearedByWhitespaceOnly(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	require.NoError(t, s.SetDailyPrompt("something"))
	require.NoError(t, s.SetDailyPrompt("   \n  "))

	p, err := s.DailyPrompt()
	require.NoError(t, err)
	assert.Empty(t, p)
}
