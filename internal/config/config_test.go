package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// isolateHome points HOME (and thus os.UserConfigDir on darwin) at a temp
// dir so tests never touch the real config.
func isolateHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	return home
}

func TestLoadCreatesDefaults(t *testing.T) {
	home := isolateHome(t)

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(home, "DailyProgress"), cfg.DataDir)
	assert.Equal(t, "09:30", cfg.MorningTime)
	assert.Equal(t, "17:30", cfg.EveningTime)

	path, err := Path()
	require.NoError(t, err)
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(content), `"morning_time": "09:30"`)
}

func TestLoadExistingAndTildeExpansion(t *testing.T) {
	home := isolateHome(t)
	path, err := Path()
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
	require.NoError(t, os.WriteFile(path, []byte(
		`{"data_dir": "~/logs", "morning_time": "08:15", "evening_time": "18:00"}`), 0o600))

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(home, "logs"), cfg.DataDir)
	assert.Equal(t, "08:15", cfg.MorningTime)
}

func TestLoadFailsLoud(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr string
	}{
		{name: "malformed json", content: `{`, wantErr: "parsing config"},
		{
			name:    "empty data dir",
			content: `{"data_dir": "", "morning_time": "09:00", "evening_time": "17:30"}`,
			wantErr: "data_dir must not be empty",
		},
		{name: "bad morning time", content: `{"data_dir": "/d", "morning_time": "9am", "evening_time": "17:30"}`, wantErr: "morning_time"},
		{name: "bad evening time", content: `{"data_dir": "/d", "morning_time": "09:00", "evening_time": "25:00"}`, wantErr: "evening_time"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isolateHome(t)
			path, err := Path()
			require.NoError(t, err)
			require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
			require.NoError(t, os.WriteFile(path, []byte(tt.content), 0o600))
			_, err = Load()
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestParseTimeOfDay(t *testing.T) {
	t.Parallel()
	for _, good := range []string{"07:45", "7:45"} {
		hour, minute, err := ParseTimeOfDay(good)
		require.NoError(t, err, "input %q", good)
		assert.Equal(t, 7, hour)
		assert.Equal(t, 45, minute)
	}

	for _, bad := range []string{"", "07:60", "24:00", "07:4x", "noon"} {
		_, _, err := ParseTimeOfDay(bad)
		assert.Error(t, err, "input %q", bad)
	}
}
