package mobilecore

import (
	"fmt"
	"time"

	"github.com/cristim/daily-progress-logger/internal/config"
	"github.com/cristim/daily-progress-logger/internal/schedule"
)

// duePromptJSON is one due prompt in the DuePromptsJSON response.
type duePromptJSON struct {
	ID   int    `json:"id"`   // schedule.Prompt constant value
	Name string `json:"name"` // human-readable name
}

// duePromptsResponseJSON is the DuePromptsJSON response.
type duePromptsResponseJSON struct {
	Due []duePromptJSON `json:"due"`
}

// DuePromptsJSON returns the check-in prompts due at the given moment as JSON.
// nowRFC3339 must be a valid RFC3339 timestamp (e.g. "2026-07-17T09:35:00+02:00").
//
// The response is:
//
//	{"due": [{"id":2,"name":"morning check-in"}, ...]}
//
// Prompt IDs match schedule.Prompt constants:
//
//	0 = week review, 1 = weekly plan, 2 = morning, 3 = evening, 4 = weekly summary
//
// The host uses these IDs to decide which check-in screens to show. Timing
// configuration (morning/evening times, summary day/time) is read from the
// shared config file; defaults (09:30, 17:30, Friday 17:00) are used when the
// config cannot be loaded (e.g. first run on mobile).
func (c *Core) DuePromptsJSON(nowRFC3339 string) (string, error) {
	now, err := time.Parse(time.RFC3339, nowRFC3339)
	if err != nil {
		return "", fmt.Errorf("parsing now %q as RFC3339: %w", nowRFC3339, err)
	}
	now = now.Local()

	morning, evening, summaryDay, summaryTod := defaultPromptTimes()
	if cfg, cerr := config.Load(); cerr == nil {
		if h, m, perr := config.ParseTimeOfDay(cfg.MorningTime); perr == nil {
			morning = schedule.TimeOfDay{Hour: h, Minute: m}
		}
		if h, m, perr := config.ParseTimeOfDay(cfg.EveningTime); perr == nil {
			evening = schedule.TimeOfDay{Hour: h, Minute: m}
		}
		if h, m, perr := config.ParseTimeOfDay(cfg.SummaryTime); perr == nil {
			summaryTod = schedule.TimeOfDay{Hour: h, Minute: m}
		}
		if wd, perr := config.ParseDay(cfg.SummaryDay); perr == nil {
			summaryDay = wd
		}
	}

	st, err := c.store.ScheduleState(now)
	if err != nil {
		return "", err
	}

	due := schedule.Due(now, morning, evening, st, summaryDay, summaryTod)
	out := duePromptsResponseJSON{Due: make([]duePromptJSON, len(due))}
	for i, p := range due {
		out.Due[i] = duePromptJSON{ID: int(p), Name: p.String()}
	}
	return toJSON(out)
}

// defaultPromptTimes returns the fallback check-in times used when the config
// cannot be loaded (first run, mobile with no config file yet).
func defaultPromptTimes() (morning, evening schedule.TimeOfDay, summaryDay time.Weekday, summaryTod schedule.TimeOfDay) {
	return schedule.TimeOfDay{Hour: 9, Minute: 30},
		schedule.TimeOfDay{Hour: 17, Minute: 30},
		time.Friday,
		schedule.TimeOfDay{Hour: 17, Minute: 0}
}
