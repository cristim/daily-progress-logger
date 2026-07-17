package mobilecore

import (
	"encoding/json"
	"fmt"
)

// ConfigJSON returns the current mobile configuration as JSON.
// The schema is mobileConfigDTO (see dto.go): mobile-relevant settings only.
// Desktop-specific fields (data_dir, shortcuts, login_item_offered, etc.) are
// intentionally absent; Open's dataDir is the authoritative data location.
//
// Configuration is stored in <dataDir>/mobile-config.json, which syncs to
// Drive like all other data files.  Returns a BAD_INPUT coded error when the
// config file exists but cannot be parsed.
func (c *Core) ConfigJSON() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	cfg, err := c.loadMobileConfig()
	if err != nil {
		return "", err
	}
	return toJSON(cfg)
}

// SetConfig applies a partial JSON config update. Only fields present in the
// JSON are changed; absent fields are left at their current values.
// Example (change morning time only):
//
//	{"morning_time":"08:30"}
//
// The config is written to <dataDir>/mobile-config.json atomically.
// An invalid value (e.g. malformed time string) returns a BAD_INPUT error
// without writing.
func (c *Core) SetConfig(patchJSON string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	cfg, err := c.loadMobileConfig()
	if err != nil {
		return err
	}
	var patch mobileConfigDTO
	if err := json.Unmarshal([]byte(patchJSON), &patch); err != nil {
		return fmt.Errorf("%s: parsing config patch: %w", ErrCodeBadInput, err)
	}
	if patch.MorningTime != "" {
		cfg.MorningTime = patch.MorningTime
	}
	if patch.EveningTime != "" {
		cfg.EveningTime = patch.EveningTime
	}
	if patch.SummaryDay != "" {
		cfg.SummaryDay = patch.SummaryDay
	}
	if patch.SummaryTime != "" {
		cfg.SummaryTime = patch.SummaryTime
	}
	if patch.GoogleClientID != "" {
		cfg.GoogleClientID = patch.GoogleClientID
	}
	if patch.NotifyCheckins != nil {
		cfg.NotifyCheckins = patch.NotifyCheckins
	}
	return c.saveMobileConfig(cfg)
}
