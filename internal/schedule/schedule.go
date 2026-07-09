// Package schedule decides which check-in prompts are due at a given
// moment. Pure logic, no I/O.
package schedule

import "time"

// Prompt identifies one of the app's check-in dialogs.
type Prompt int

const (
	// PromptWeekReview triages the previous week's leftover items.
	PromptWeekReview Prompt = iota
	// PromptWeeklyPlan captures the week's "big things" on Monday morning.
	PromptWeeklyPlan
	// PromptMorning asks what the user plans to work on today.
	PromptMorning
	// PromptEvening asks what the user accomplished today.
	PromptEvening
	// PromptWeeklySummary shows the current week's summary for review on
	// the configured summary day (default: Friday afternoon).
	PromptWeeklySummary
)

// String returns a human-readable prompt name.
func (p Prompt) String() string {
	switch p {
	case PromptWeekReview:
		return "week review"
	case PromptWeeklyPlan:
		return "weekly plan"
	case PromptMorning:
		return "morning check-in"
	case PromptEvening:
		return "evening check-in"
	case PromptWeeklySummary:
		return "weekly summary"
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

// isWeekday reports whether now falls Monday through Friday.
func isWeekday(now time.Time) bool {
	wd := now.Weekday()
	return wd != time.Saturday && wd != time.Sunday
}

// State is the persisted facts Due needs to decide what to prompt for.
type State struct {
	// MorningDone: today's morning check-in already happened.
	MorningDone bool
	// EveningDone: today's evening check-in already happened.
	EveningDone bool
	// WeekReviewPending: an earlier week has data but was never reviewed.
	WeekReviewPending bool
	// WeeklyPlanPending: the current week has no weekly plan ("big things") yet.
	WeeklyPlanPending bool
	// SummaryPending: the current week has data but has not yet been summarized.
	SummaryPending bool
	// SummaryPendingPastWeek is true when the pending summary is for a past
	// week (missed Friday, holiday). In that case the prompt fires on any day
	// regardless of the configured summary day and time.
	SummaryPendingPastWeek bool
}

// Due returns the prompts due at now, in the order they should be shown:
// the week review gates the week, then the morning plan, then the evening
// report, then the weekly summary (on the configured summary day once the
// summary time has passed). The week review is due at any hour; the daily
// check-ins and the weekly summary only once their configured time has passed.
//
// When both the morning and evening check-ins are due simultaneously (the
// morning was missed and the evening window has now opened), the morning
// prompt is dropped: asking "what are you planning?" after the evening
// window is pointless planning theater. The evening dialog already captures
// what was accomplished, and any unplanned work goes in the free-text field.
func Due(now time.Time, morning, evening TimeOfDay, st State,
	summaryDay time.Weekday, summary TimeOfDay,
) []Prompt {
	var due []Prompt
	if st.WeekReviewPending {
		due = append(due, PromptWeekReview)
	}
	// The weekly plan is set Monday morning; if the app wasn't opened Monday it
	// catches up on the first open any later weekday, until the plan is set.
	// It gates the day ahead of the daily morning plan, so it comes first.
	if st.WeeklyPlanPending && isWeekday(now) && morning.reached(now) {
		due = append(due, PromptWeeklyPlan)
	}
	morningDue := !st.MorningDone && morning.reached(now)
	eveningDue := !st.EveningDone && evening.reached(now)
	// Only show morning when the evening window has not yet opened; once
	// both are overdue, skip the morning and go straight to evening.
	if morningDue && !eveningDue {
		due = append(due, PromptMorning)
	}
	if eveningDue {
		due = append(due, PromptEvening)
	}
	summaryDue := st.SummaryPending && now.Weekday() == summaryDay && summary.reached(now)
	// A past week's missed summary fires on any day (finding 41).
	if st.SummaryPendingPastWeek {
		summaryDue = st.SummaryPending
	}
	if summaryDue {
		due = append(due, PromptWeeklySummary)
	}
	return due
}

// Filter applies the user's snooze ("Postpone 1h") and skip (Cancel)
// choices to the due prompts. show is what should be displayed right now;
// pending reports whether anything remains unresolved: a snoozed prompt is
// still pending (it will be shown once the snooze expires), a prompt
// skipped today is not (it returns tomorrow).
func Filter(due []Prompt, now time.Time, snoozedUntil map[Prompt]time.Time,
	skippedOn map[Prompt]string,
) (show []Prompt, pending bool) {
	today := now.Format(time.DateOnly)
	for _, p := range due {
		if skippedOn[p] == today {
			continue
		}
		pending = true
		if until, ok := snoozedUntil[p]; ok && now.Before(until) {
			continue
		}
		show = append(show, p)
	}
	return show, pending
}
