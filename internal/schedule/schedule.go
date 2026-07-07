// Package schedule decides which check-in prompts are due at a given
// moment. Pure logic, no I/O.
package schedule

import "time"

// Prompt identifies one of the app's check-in dialogs.
type Prompt int

const (
	// PromptWeekReview triages the previous week's leftover items.
	PromptWeekReview Prompt = iota
	// PromptMorning asks what the user plans to work on today.
	PromptMorning
	// PromptEvening asks what the user accomplished today.
	PromptEvening
)

// String returns a human-readable prompt name.
func (p Prompt) String() string {
	switch p {
	case PromptWeekReview:
		return "week review"
	case PromptMorning:
		return "morning check-in"
	case PromptEvening:
		return "evening check-in"
	}
	return "unknown prompt"
}

// TimeOfDay is a wall-clock time within any day.
type TimeOfDay struct {
	Hour   int
	Minute int
}

// reached reports whether now is at or past the time of day.
func (t TimeOfDay) reached(now time.Time) bool {
	return now.Hour()*60+now.Minute() >= t.Hour*60+t.Minute
}

// State is the persisted facts Due needs to decide what to prompt for.
type State struct {
	// MorningDone: today's morning check-in already happened.
	MorningDone bool
	// EveningDone: today's evening check-in already happened.
	EveningDone bool
	// WeekReviewPending: an earlier week has data but was never reviewed.
	WeekReviewPending bool
}

// Due returns the prompts due at now, in the order they should be shown:
// the week review gates the week, then the morning plan, then the evening
// report. The week review is due at any hour; the daily check-ins only once
// their configured time has passed.
func Due(now time.Time, morning, evening TimeOfDay, st State) []Prompt {
	var due []Prompt
	if st.WeekReviewPending {
		due = append(due, PromptWeekReview)
	}
	if !st.MorningDone && morning.reached(now) {
		due = append(due, PromptMorning)
	}
	if !st.EveningDone && evening.reached(now) {
		due = append(due, PromptEvening)
	}
	return due
}
