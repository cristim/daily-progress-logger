// Package config loads the app configuration from a JSON file under the
// user's config directory, creating it with defaults on first run.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	appDirName     = "DailyProgressLogger"
	configFileName = "config.json"

	defaultMorningTime = "09:30"
	defaultEveningTime = "17:30"
	defaultSummaryDay  = "Friday"
	defaultSummaryTime = "17:00"
	defaultDataDirName = "DailyProgress" // under the home directory
)

// Config holds the user-tunable settings.
type Config struct {
	// DataDir is where the markdown files live.
	DataDir string `json:"data_dir"`
	// MorningTime is when the morning check-in becomes due, as "HH:MM".
	MorningTime string `json:"morning_time"`
	// EveningTime is when the evening check-in becomes due, as "HH:MM".
	EveningTime string `json:"evening_time"`
	// SummaryDay is the weekday on which the weekly summary dialog is shown
	// (e.g. "Friday"). Defaults to "Friday".
	SummaryDay string `json:"summary_day"`
	// SummaryTime is the time of day after which the weekly summary dialog is
	// shown on SummaryDay, as "HH:MM". Defaults to "17:00".
	SummaryTime string `json:"summary_time"`
	// LoginItemOffered records that the user was already asked about installing
	// the login item, so the prompt is shown at most once.
	LoginItemOffered bool `json:"login_item_offered"`
}

// Path returns the config file location.
func Path() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locating user config dir: %w", err)
	}
	return filepath.Join(base, appDirName, configFileName), nil
}

// Load reads the config file, creating it with defaults if it does not
// exist. A malformed or invalid config is an error, never silently replaced.
func Load() (*Config, error) {
	path, err := Path()
	if err != nil {
		return nil, err
	}
	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		cfg, err := defaults()
		if err != nil {
			return nil, err
		}
		if err := write(path, cfg); err != nil {
			return nil, err
		}
		return cfg, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading config %s: %w", path, err)
	}

	cfg := &Config{}
	if err := json.Unmarshal(content, cfg); err != nil {
		return nil, fmt.Errorf("parsing config %s: %w", path, err)
	}
	// Apply defaults for fields introduced after initial release so that
	// existing config files do not require manual migration.
	if cfg.SummaryDay == "" {
		cfg.SummaryDay = defaultSummaryDay
	}
	if cfg.SummaryTime == "" {
		cfg.SummaryTime = defaultSummaryTime
	}
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid config %s: %w", path, err)
	}
	cfg.DataDir, err = expandHome(cfg.DataDir)
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

func defaults() (*Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("locating home dir: %w", err)
	}
	return &Config{
		DataDir:     filepath.Join(home, defaultDataDirName),
		MorningTime: defaultMorningTime,
		EveningTime: defaultEveningTime,
		SummaryDay:  defaultSummaryDay,
		SummaryTime: defaultSummaryTime,
	}, nil
}

func write(path string, cfg *Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}
	content, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding config: %w", err)
	}
	if err := os.WriteFile(path, append(content, '\n'), 0o600); err != nil {
		return fmt.Errorf("writing config %s: %w", path, err)
	}
	return nil
}

func (c *Config) validate() error {
	if c.DataDir == "" {
		return errors.New("data_dir must not be empty")
	}
	for key, value := range map[string]string{
		"morning_time": c.MorningTime,
		"evening_time": c.EveningTime,
		"summary_time": c.SummaryTime,
	} {
		if _, _, err := ParseTimeOfDay(value); err != nil {
			return fmt.Errorf("%s: %w", key, err)
		}
	}
	if _, err := ParseDay(c.SummaryDay); err != nil {
		return fmt.Errorf("summary_day: %w", err)
	}
	return nil
}

// Save writes the current config back to its file, preserving 0600 permissions.
func (c *Config) Save() error {
	path, err := Path()
	if err != nil {
		return err
	}
	return write(path, c)
}

// ParseTimeOfDay parses "HH:MM" into hour and minute.
func ParseTimeOfDay(s string) (hour, minute int, err error) {
	t, err := time.Parse("15:04", s)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid time of day %q, want HH:MM: %w", s, err)
	}
	return t.Hour(), t.Minute(), nil
}

// weekdayNames maps lowercase weekday names to time.Weekday values.
var weekdayNames = map[string]time.Weekday{
	"sunday":    time.Sunday,
	"monday":    time.Monday,
	"tuesday":   time.Tuesday,
	"wednesday": time.Wednesday,
	"thursday":  time.Thursday,
	"friday":    time.Friday,
	"saturday":  time.Saturday,
}

// ParseDay parses a weekday name (case-insensitive) into a time.Weekday.
func ParseDay(s string) (time.Weekday, error) {
	if wd, ok := weekdayNames[strings.ToLower(s)]; ok {
		return wd, nil
	}
	return 0, fmt.Errorf("unknown weekday %q, want e.g. Monday, Friday", s)
}

// expandHome resolves a leading "~/" to the user's home directory.
func expandHome(path string) (string, error) {
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("locating home dir: %w", err)
		}
		return filepath.Join(home, strings.TrimPrefix(path, "~")), nil
	}
	return path, nil
}
