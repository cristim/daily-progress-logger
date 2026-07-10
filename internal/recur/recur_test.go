package recur

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func at(y int, mo time.Month, d, h, m int) time.Time {
	return time.Date(y, mo, d, h, m, 0, 0, time.Local)
}

func TestParse(t *testing.T) {
	t.Parallel()
	cases := []struct {
		text     string
		wantOK   bool
		clean    string
		kind     Kind
		hour     int
		minute   int
		weekday  time.Weekday
		monthday int
	}{
		{text: "Standup @weekday @9:00", wantOK: true, clean: "Standup", kind: Weekday, hour: 9},
		{
			text: "Review metrics @weekly @mon @16:00", wantOK: true,
			clean: "Review metrics", kind: Weekly, hour: 16, weekday: time.Monday,
		},
		{text: "Rent @monthly @1 @9:00", wantOK: true, clean: "Rent", kind: Monthly, hour: 9, monthday: 1},
		{text: "Vitamins @daily @8:30", wantOK: true, clean: "Vitamins", kind: Daily, hour: 8, minute: 30},
		{text: "Take out trash @weekly", wantOK: true, clean: "Take out trash", kind: Weekly, hour: 9, weekday: time.Monday},
		// A story tag before the recurrence tokens stays in the clean text.
		{
			text: "Review @payments @weekly @tue @16:30", wantOK: true,
			clean: "Review @payments", kind: Weekly, hour: 16, minute: 30, weekday: time.Tuesday,
		},
		{text: "Buy milk", wantOK: false},
		{text: "Fix bug @payments", wantOK: false}, // @payments is not a recurrence keyword
	}
	for _, c := range cases {
		clean, rec, ok := Parse(c.text, 9, 0, nil)
		assert.Equal(t, c.wantOK, ok, "ok for %q", c.text)
		if !c.wantOK {
			continue
		}
		assert.Equal(t, c.clean, clean, "clean for %q", c.text)
		assert.Equal(t, c.kind, rec.Kind, "kind for %q", c.text)
		assert.Equal(t, c.hour, rec.Hour, "hour for %q", c.text)
		assert.Equal(t, c.minute, rec.Minute, "minute for %q", c.text)
		if c.kind == Weekly {
			assert.Equal(t, c.weekday, rec.Weekday, "weekday for %q", c.text)
		}
		if c.kind == Monthly {
			assert.Equal(t, c.monthday, rec.MonthDay, "monthday for %q", c.text)
		}
	}
}

func TestParseStoryTagNotConsumed(t *testing.T) {
	t.Parallel()
	// A story slug shaped like a month-day ("15") or a weekday ("mon") must not
	// be consumed as a recurrence token: it stays in the clean text and does not
	// alter the schedule.
	isStory := func(s string) bool { return s == "15" || s == "mon" }

	clean, rec, ok := Parse("Reconcile @15 @weekly @fri @17:00", 9, 0, isStory)
	assert.True(t, ok)
	assert.Equal(t, "Reconcile @15", clean)
	assert.Equal(t, Weekly, rec.Kind)
	assert.Equal(t, time.Friday, rec.Weekday) // not rewritten by the "15" token

	clean, rec, ok = Parse("Standup @mon @daily @9:00", 9, 0, isStory)
	assert.True(t, ok)
	assert.Equal(t, "Standup @mon", clean)
	assert.Equal(t, Daily, rec.Kind)
}

func TestDailyOccurrences(t *testing.T) {
	t.Parallel()
	r := Recurrence{Kind: Daily, Hour: 9}
	assert.Equal(t, at(2026, 7, 10, 9, 0), r.Next(at(2026, 7, 10, 8, 0)))
	assert.Equal(t, at(2026, 7, 9, 9, 0), r.MostRecent(at(2026, 7, 10, 8, 0)))
	assert.Equal(t, at(2026, 7, 11, 9, 0), r.Next(at(2026, 7, 10, 10, 0)))
	assert.Equal(t, at(2026, 7, 10, 9, 0), r.MostRecent(at(2026, 7, 10, 10, 0)))
}

func TestWeekdayOccurrences(t *testing.T) {
	t.Parallel()
	r := Recurrence{Kind: Weekday, Hour: 9}
	sat := at(2026, 7, 11, 10, 0) // Saturday
	assert.Equal(t, at(2026, 7, 13, 9, 0), r.Next(sat), "skips the weekend to Monday")
	assert.Equal(t, at(2026, 7, 10, 9, 0), r.MostRecent(sat), "back to Friday")
}

func TestWeeklyOccurrences(t *testing.T) {
	t.Parallel()
	r := Recurrence{Kind: Weekly, Weekday: time.Monday, Hour: 16, Minute: 30}
	fri := at(2026, 7, 10, 12, 0) // Friday
	assert.Equal(t, at(2026, 7, 13, 16, 30), r.Next(fri))
	assert.Equal(t, at(2026, 7, 6, 16, 30), r.MostRecent(fri))
}

func TestMonthlyOccurrences(t *testing.T) {
	t.Parallel()
	r := Recurrence{Kind: Monthly, MonthDay: 1, Hour: 9}
	now := at(2026, 7, 10, 8, 0)
	assert.Equal(t, at(2026, 8, 1, 9, 0), r.Next(now))
	assert.Equal(t, at(2026, 7, 1, 9, 0), r.MostRecent(now))

	// Day 31 clamps to the month's last day: Feb 2026 (non-leap) -> 28.
	feb := Recurrence{Kind: Monthly, MonthDay: 31, Hour: 9}
	assert.Equal(t, at(2026, 2, 28, 9, 0), feb.Next(at(2026, 2, 15, 0, 0)))
	assert.Equal(t, at(2026, 1, 31, 9, 0), feb.MostRecent(at(2026, 2, 15, 0, 0)))
	// Leap February (2028) -> 29.
	assert.Equal(t, at(2028, 2, 29, 9, 0), feb.Next(at(2028, 2, 15, 0, 0)))
}

func TestDescribe(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "daily 08:00", Recurrence{Kind: Daily, Hour: 8}.Describe())
	assert.Equal(t, "weekly Mon 16:30", Recurrence{Kind: Weekly, Weekday: time.Monday, Hour: 16, Minute: 30}.Describe())
	assert.Equal(t, "monthly day 1 09:00", Recurrence{Kind: Monthly, MonthDay: 1, Hour: 9}.Describe())
	assert.Equal(t, "weekdays 09:00", Recurrence{Kind: Weekday, Hour: 9}.Describe())
}
