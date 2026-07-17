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
	assert.Equal(t, "Friday", cfg.SummaryDay)
	assert.Equal(t, "17:00", cfg.SummaryTime)

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
		{
			name:    "bad summary day",
			content: `{"data_dir": "/d", "morning_time": "09:00", "evening_time": "17:30", "summary_day": "Funday", "summary_time": "17:00"}`,
			wantErr: "summary_day",
		},
		{
			name:    "bad summary time",
			content: `{"data_dir": "/d", "morning_time": "09:00", "evening_time": "17:30", "summary_day": "Friday", "summary_time": "99:00"}`,
			wantErr: "summary_time",
		},
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

func TestLoadSeedsDefaultShortcuts(t *testing.T) {
	isolateHome(t)
	cfg, err := Load()
	require.NoError(t, err)
	// Every known action is present with its default.
	for _, a := range ShortcutActions {
		assert.Equal(t, a.Default, cfg.Shortcuts[a.ID], "action %q", a.ID)
	}
	// The defaults are all distinct (no accidental collisions in the table).
	require.NoError(t, cfg.validate())
}

func TestLoadMigratesShortcuts(t *testing.T) {
	isolateHome(t)
	path, err := Path()
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
	// A pre-existing config that overrides one shortcut, omits the rest, and
	// carries an unknown ID that must be dropped.
	require.NoError(t, os.WriteFile(path, []byte(
		`{"data_dir": "/d", "morning_time": "09:00", "evening_time": "17:30",`+
			`"shortcuts": {"item.done": "Ctrl+K", "legacy.action": "Ctrl+9"}}`), 0o600))

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "Ctrl+K", cfg.Shortcuts[ShortcutItemDone], "explicit override kept")
	assert.Equal(t, "Ctrl+Shift+T", cfg.Shortcuts[ShortcutItemTodo], "missing entry seeded")
	_, ok := cfg.Shortcuts["legacy.action"]
	assert.False(t, ok, "unknown shortcut ID dropped")
}

func TestLoadFailsOnShortcutConflict(t *testing.T) {
	isolateHome(t)
	path, err := Path()
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
	require.NoError(t, os.WriteFile(path, []byte(
		`{"data_dir": "/d", "morning_time": "09:00", "evening_time": "17:30",`+
			`"shortcuts": {"item.done": "Ctrl+1", "checkin.morning": "ctrl+1"}}`), 0o600))
	_, err = Load()
	require.ErrorContains(t, err, "conflicts")
}

func TestParseDay(t *testing.T) {
	t.Parallel()
	for _, good := range []string{"Friday", "friday", "FRIDAY", "Monday", "Sunday"} {
		_, err := ParseDay(good)
		require.NoError(t, err, "input %q", good)
	}
	for _, bad := range []string{"", "Funday", "Mon", "TGIF"} {
		_, err := ParseDay(bad)
		require.Errorf(t, err, "input %q should produce an error", bad)
	}
	wd, err := ParseDay("Friday")
	require.NoError(t, err)
	assert.Equal(t, 5, int(wd)) // time.Friday == 5
}

func TestConfigSave(t *testing.T) {
	isolateHome(t)
	cfg, err := Load()
	require.NoError(t, err)
	cfg.LoginItemOffered = true
	require.NoError(t, cfg.Save())
	cfg2, err := Load()
	require.NoError(t, err)
	assert.True(t, cfg2.LoginItemOffered)
}

func TestNotifyCheckinsDefault(t *testing.T) {
	// A fresh config (no notify_checkins field) must default to enabled.
	isolateHome(t)
	cfg, err := Load()
	require.NoError(t, err)
	assert.Nil(t, cfg.NotifyCheckins, "field absent from new config means nil")
	assert.True(t, cfg.NotifyCheckinsEnabled(), "nil means notifications enabled")
}

func TestNotifyCheckinsRoundTrip(t *testing.T) {
	isolateHome(t)
	cfg, err := Load()
	require.NoError(t, err)

	// Disable and persist.
	cfg.NotifyCheckins = new(false)
	require.NoError(t, cfg.Save())

	cfg2, err := Load()
	require.NoError(t, err)
	require.NotNil(t, cfg2.NotifyCheckins)
	assert.False(t, cfg2.NotifyCheckinsEnabled(), "persisted false must survive round-trip")

	// Re-enable and persist.
	cfg2.NotifyCheckins = new(true)
	require.NoError(t, cfg2.Save())

	cfg3, err := Load()
	require.NoError(t, err)
	require.NotNil(t, cfg3.NotifyCheckins)
	assert.True(t, cfg3.NotifyCheckinsEnabled(), "persisted true must survive round-trip")
}

// TestNotifyCheckinsOldConfig verifies that a config file written before
// notify_checkins existed (field absent) enables notifications by default.
func TestNotifyCheckinsOldConfig(t *testing.T) {
	isolateHome(t)
	path, err := Path()
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
	// Old-style config without notify_checkins.
	require.NoError(t, os.WriteFile(path, []byte(
		`{"data_dir": "/d", "morning_time": "09:00", "evening_time": "17:30"}`), 0o600))

	cfg, err := Load()
	require.NoError(t, err)
	assert.Nil(t, cfg.NotifyCheckins)
	assert.True(t, cfg.NotifyCheckinsEnabled())
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
