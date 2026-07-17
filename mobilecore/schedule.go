package mobilecore

import (
	"fmt"
	"time"

	"github.com/cristim/daily-progress-logger/internal/config"
	"github.com/cristim/daily-progress-logger/internal/schedule"
)

// duePromptDTO is one due prompt in the DuePromptsJSON response.
type duePromptDTO struct {
	ID   int    `json:"id"`   // schedule.Prompt constant value
	Name string `json:"name"` // human-readable name
}

// duePromptsResponseDTO is the DuePromptsJSON response.
type duePromptsResponseDTO struct {
	Due []duePromptDTO `json:"due"`
}

// DuePromptsJSON returns the check-in prompts due at the given moment as JSON.
// nowRFC3339 must be a valid RFC3339 timestamp (e.g. "2026-07-17T09:35:00+02:00").
// The timezone offset embedded in the timestamp is used directly for
// hour/minute comparisons, so the result is deterministic regardless of the
// server process's timezone.
//
// The response is:
//
//	{"due": [{"id":2,"name":"morning check-in"}, ...]}
//
// Prompt IDs match schedule.Prompt constants:
//
//	0 = week review, 1 = weekly plan, 2 = morning, 3 = evening, 4 = weekly summary
//
// Timing configuration is read from <dataDir>/mobile-config.json.
// Defaults (09:30 morning, 17:30 evening, Friday 17:00 summary) are used when
// no config file exists (first run).  A corrupt or unparseable config file
// returns a BAD_INPUT coded error so the host can surface the problem.
func (c *Core) DuePromptsJSON(nowRFC3339 string) (string, error) {
	now, err := time.Parse(time.RFC3339, nowRFC3339)
	if err != nil {
		return "", fmt.Errorf("%s: parsing now %q as RFC3339: %w", ErrCodeBadInput, nowRFC3339, err)
	}
	// Use the timestamp as-is: its embedded timezone determines the hour/minute
	// values used by schedule.Due, so the comparison is in the caller's timezone.

	c.mu.Lock()
	defer c.mu.Unlock()

	morning, evening, summaryDay, summaryTod := defaultPromptTimes()

	cfg, cerr := c.loadMobileConfig()
	if cerr != nil {
		// Config exists but is corrupt/unparseable: return a coded error so the
		// host can alert the user instead of silently using wrong prompt times.
		return "", cerr
	}
	// Apply any non-empty overrides from the config; missing fields keep defaults.
	if cfg.MorningTime != "" {
		if h, m, perr := config.ParseTimeOfDay(cfg.MorningTime); perr == nil {
			morning = schedule.TimeOfDay{Hour: h, Minute: m}
		}
	}
	if cfg.EveningTime != "" {
		if h, m, perr := config.ParseTimeOfDay(cfg.EveningTime); perr == nil {
			evening = schedule.TimeOfDay{Hour: h, Minute: m}
		}
	}
	if cfg.SummaryTime != "" {
		if h, m, perr := config.ParseTimeOfDay(cfg.SummaryTime); perr == nil {
			summaryTod = schedule.TimeOfDay{Hour: h, Minute: m}
		}
	}
	if cfg.SummaryDay != "" {
		if wd, perr := config.ParseDay(cfg.SummaryDay); perr == nil {
			summaryDay = wd
		}
	}

	st, err := c.store.ScheduleState(now)
	if err != nil {
		return "", err
	}

	due := schedule.Due(now, morning, evening, st, summaryDay, summaryTod)
	out := duePromptsResponseDTO{Due: make([]duePromptDTO, len(due))}
	for i, p := range due {
		out.Due[i] = duePromptDTO{ID: int(p), Name: p.String()}
	}
	return toJSON(out)
}

// defaultPromptTimes returns the fallback check-in times used when the config
// cannot be loaded (first run, mobile with no config file yet).
// These match config.go defaults (09:30 / 17:30 / Friday / 17:00).
func defaultPromptTimes() (morning, evening schedule.TimeOfDay, summaryDay time.Weekday, summaryTod schedule.TimeOfDay) {
	return schedule.TimeOfDay{Hour: 9, Minute: 30},
		schedule.TimeOfDay{Hour: 17, Minute: 30},
		time.Friday,
		schedule.TimeOfDay{Hour: 17, Minute: 0}
}
