package store

import (
	"time"

	"github.com/cristim/daily-progress-logger/internal/schedule"
)

// ScheduleState computes the schedule.State for now by querying the store.
// This is the shared computation previously inlined in App.scheduleState
// (internal/ui/app.go); extracting it here lets non-Qt surfaces (mobile core,
// future CLI "dpr due") call schedule.Due without depending on the Qt App type.
//
// Callers supply now in local time. The Qt App.scheduleState is intentionally
// left intact for now; once Qt is migrated it can delegate here instead.
func (s *Store) ScheduleState(now time.Time) (schedule.State, error) {
	var st schedule.State

	daily, exists, err := s.LoadDaily(now)
	if err != nil {
		return st, err
	}
	if exists {
		st.MorningDone = daily.MorningDone
		st.EveningDone = daily.EveningDone
	}

	_, pending, err := s.UnreviewedWeek(now)
	if err != nil {
		return st, err
	}
	st.WeekReviewPending = pending

	_, planPending, err := s.WeeklyPlanPending(now)
	if err != nil {
		return st, err
	}
	st.WeeklyPlanPending = planPending

	pendingWeek, summaryPending, err := s.WeekSummaryPending(now)
	if err != nil {
		return st, err
	}
	st.SummaryPending = summaryPending
	if summaryPending {
		currentWeek := WeekOf(now)
		st.SummaryPendingPastWeek = pendingWeek.Before(currentWeek)
	}

	return st, nil
}
