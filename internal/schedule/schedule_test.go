package schedule

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func at(hour, minute int) time.Time {
	return time.Date(2026, 7, 7, hour, minute, 0, 0, time.Local)
}

// at returns a time on the test date 2026-07-07 (Tuesday) at hour:minute.
// Use atDay to get a time on a specific weekday for summary tests.
func atDay(weekday time.Weekday, hour, minute int) time.Time {
	// 2026-07-07 is Tuesday. Offset from Tuesday to get the desired weekday.
	offset := (int(weekday) - int(time.Tuesday) + 7) % 7
	base := time.Date(2026, 7, 7, hour, minute, 0, 0, time.Local)
	return base.AddDate(0, 0, offset)
}

func TestDue(t *testing.T) {
	t.Parallel()
	morning := TimeOfDay{Hour: 9, Minute: 0}
	evening := TimeOfDay{Hour: 17, Minute: 30}
	summaryTOD := TimeOfDay{Hour: 17, Minute: 0}
	summaryDay := time.Friday

	tests := []struct {
		name  string
		now   time.Time
		state State
		want  []Prompt
	}{
		{
			name: "early morning nothing due",
			now:  at(7, 59),
		},
		{
			name: "morning due at exact time",
			now:  at(9, 0),
			want: []Prompt{PromptMorning},
		},
		{
			name:  "morning already done",
			now:   at(10, 0),
			state: State{MorningDone: true},
		},
		{
			name:  "evening due",
			now:   at(17, 30),
			state: State{MorningDone: true},
			want:  []Prompt{PromptEvening},
		},
		{
			// When the evening window is already open, the missed morning
			// is dropped: planning at day's end is noise; the evening
			// dialog captures what was done and has a free-text field for
			// unplanned work.
			name: "missed morning dropped when evening is due",
			now:  at(18, 0),
			want: []Prompt{PromptEvening},
		},
		{
			name: "morning only mid-day before evening window",
			now:  at(10, 0),
			want: []Prompt{PromptMorning},
		},
		{
			name:  "both done",
			now:   at(23, 0),
			state: State{MorningDone: true, EveningDone: true},
		},
		{
			name:  "week review due any hour and comes first",
			now:   at(6, 0),
			state: State{WeekReviewPending: true},
			want:  []Prompt{PromptWeekReview},
		},
		{
			// At 19:00 both morning and evening are overdue; morning is
			// dropped. Week review still precedes evening.
			name:  "full stack ordered: review then evening (morning dropped)",
			now:   at(19, 0),
			state: State{WeekReviewPending: true},
			want:  []Prompt{PromptWeekReview, PromptEvening},
		},
		{
			name:  "weekly summary due on friday after summary time when daily done",
			now:   atDay(time.Friday, 17, 0),
			state: State{MorningDone: true, EveningDone: true, SummaryPending: true},
			want:  []Prompt{PromptWeeklySummary},
		},
		{
			name:  "weekly summary not due before summary time on friday",
			now:   atDay(time.Friday, 16, 59),
			state: State{MorningDone: true, EveningDone: true, SummaryPending: true},
		},
		{
			name:  "weekly summary not due on non-summary day even after time",
			now:   atDay(time.Thursday, 18, 0),
			state: State{MorningDone: true, EveningDone: true, SummaryPending: true},
		},
		{
			name:  "weekly summary not due when not pending",
			now:   atDay(time.Friday, 18, 0),
			state: State{MorningDone: true, EveningDone: true, SummaryPending: false},
		},
		{
			// Past-week summary (missed Friday) fires on any day (finding 41).
			name:  "missed Friday summary fires on Monday via SummaryPendingPastWeek",
			now:   atDay(time.Monday, 9, 30),
			state: State{MorningDone: false, SummaryPending: true, SummaryPendingPastWeek: true},
			want:  []Prompt{PromptMorning, PromptWeeklySummary},
		},
		{
			// Without the past-week flag, a summary pending on Monday is NOT shown.
			name:  "current-week summary not due on Monday before summary day",
			now:   atDay(time.Monday, 9, 30),
			state: State{MorningDone: false, SummaryPending: true, SummaryPendingPastWeek: false},
			want:  []Prompt{PromptMorning},
		},
		{
			// At Friday 18:00 both morning and evening are overdue; morning
			// is dropped. Weekly summary follows evening.
			name:  "full friday stack: evening then summary (morning dropped)",
			now:   atDay(time.Friday, 18, 0),
			state: State{SummaryPending: true},
			want:  []Prompt{PromptEvening, PromptWeeklySummary},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, Due(tt.now, morning, evening, tt.state, summaryDay, summaryTOD))
		})
	}
}

func TestFilter(t *testing.T) {
	t.Parallel()
	now := at(10, 0)
	due := []Prompt{PromptMorning, PromptEvening}

	tests := []struct {
		name        string
		snoozed     map[Prompt]time.Time
		skipped     map[Prompt]string
		wantShow    []Prompt
		wantPending bool
	}{
		{
			name:        "nothing filtered",
			wantShow:    due,
			wantPending: true,
		},
		{
			name:        "active snooze hides but stays pending",
			snoozed:     map[Prompt]time.Time{PromptMorning: at(10, 30)},
			wantShow:    []Prompt{PromptEvening},
			wantPending: true,
		},
		{
			name:        "expired snooze shows again",
			snoozed:     map[Prompt]time.Time{PromptMorning: at(9, 59)},
			wantShow:    due,
			wantPending: true,
		},
		{
			name:        "skipped today is gone and not pending",
			skipped:     map[Prompt]string{PromptMorning: "2026-07-07", PromptEvening: "2026-07-07"},
			wantPending: false,
		},
		{
			name:        "skip from yesterday no longer applies",
			skipped:     map[Prompt]string{PromptMorning: "2026-07-06"},
			wantShow:    due,
			wantPending: true,
		},
		{
			name:        "one skipped one snoozed stays pending",
			snoozed:     map[Prompt]time.Time{PromptEvening: at(11, 0)},
			skipped:     map[Prompt]string{PromptMorning: "2026-07-07"},
			wantPending: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			show, pending := Filter(due, now, tt.snoozed, tt.skipped)
			assert.Equal(t, tt.wantShow, show)
			assert.Equal(t, tt.wantPending, pending)
		})
	}
}

func TestPromptString(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "week review", PromptWeekReview.String())
	assert.Equal(t, "morning check-in", PromptMorning.String())
	assert.Equal(t, "evening check-in", PromptEvening.String())
	assert.Equal(t, "weekly summary", PromptWeeklySummary.String())
	assert.Equal(t, "unknown prompt", Prompt(99).String())
}
