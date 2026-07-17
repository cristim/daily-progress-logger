package mobilecore

import (
	"encoding/json"
	"fmt"

	"github.com/cristim/daily-progress-logger/internal/config"
)

// ConfigJSON returns the current app configuration as JSON. The returned
// object mirrors config.Config with kebab-case JSON keys (matching the on-disk
// format). Returns an error when the config file cannot be read or parsed.
func (c *Core) ConfigJSON() (string, error) {
	cfg, err := config.Load()
	if err != nil {
		return "", fmt.Errorf("loading config: %w", err)
	}
	b, err := json.Marshal(cfg)
	if err != nil {
		return "", fmt.Errorf("encoding config: %w", err)
	}
	return string(b), nil
}

// configPatch is a subset of config.Config fields settable from the mobile host.
// Only fields relevant to mobile are included; data_dir changes are handled
// by the host (re-opening the Core with a new dataDir).
type configPatch struct {
	MorningTime    *string `json:"morning_time,omitempty"`
	EveningTime    *string `json:"evening_time,omitempty"`
	SummaryDay     *string `json:"summary_day,omitempty"`
	SummaryTime    *string `json:"summary_time,omitempty"`
	GoogleClientID *string `json:"google_client_id,omitempty"`
	NotifyCheckins *bool   `json:"notify_checkins,omitempty"`
}

// SetConfig applies a partial JSON config update. Only fields present in the
// JSON are changed; absent fields are left at their current values.
// Example (change morning time only):
//
//	{"morning_time":"08:30"}
//
// The config is validated before saving; an invalid value returns an error
// without writing.
func (c *Core) SetConfig(patchJSON string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	var patch configPatch
	if err := json.Unmarshal([]byte(patchJSON), &patch); err != nil {
		return fmt.Errorf("parsing config patch: %w", err)
	}
	if patch.MorningTime != nil {
		cfg.MorningTime = *patch.MorningTime
	}
	if patch.EveningTime != nil {
		cfg.EveningTime = *patch.EveningTime
	}
	if patch.SummaryDay != nil {
		cfg.SummaryDay = *patch.SummaryDay
	}
	if patch.SummaryTime != nil {
		cfg.SummaryTime = *patch.SummaryTime
	}
	if patch.GoogleClientID != nil {
		cfg.GoogleClientID = *patch.GoogleClientID
	}
	if patch.NotifyCheckins != nil {
		cfg.NotifyCheckins = patch.NotifyCheckins
	}
	return cfg.Save()
}
