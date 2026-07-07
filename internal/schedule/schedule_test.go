package schedule

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func at(hour, minute int) time.Time {
	return time.Date(2026, 7, 7, hour, minute, 0, 0, time.Local)
}

func TestDue(t *testing.T) {
	t.Parallel()
	morning := TimeOfDay{Hour: 9, Minute: 0}
	evening := TimeOfDay{Hour: 17, Minute: 30}

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
			name: "missed morning stacks with evening",
			now:  at(18, 0),
			want: []Prompt{PromptMorning, PromptEvening},
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
			name:  "full stack ordered review then morning then evening",
			now:   at(19, 0),
			state: State{WeekReviewPending: true},
			want:  []Prompt{PromptWeekReview, PromptMorning, PromptEvening},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, Due(tt.now, morning, evening, tt.state))
		})
	}
}

func TestPromptString(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "week review", PromptWeekReview.String())
	assert.Equal(t, "morning check-in", PromptMorning.String())
	assert.Equal(t, "evening check-in", PromptEvening.String())
	assert.Equal(t, "unknown prompt", Prompt(99).String())
}
