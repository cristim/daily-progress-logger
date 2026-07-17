package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScheduleState_Empty(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	// An empty store: no check-ins done, no review pending (no past week data),
	// weekly plan pending (not set yet), no summary pending (no daily data).
	st, err := s.ScheduleState(monday)
	require.NoError(t, err)
	assert.False(t, st.MorningDone)
	assert.False(t, st.EveningDone)
	assert.False(t, st.WeekReviewPending)
	assert.True(t, st.WeeklyPlanPending) // no plan set yet
	assert.False(t, st.SummaryPending)   // no daily data yet
	assert.False(t, st.SummaryPendingPastWeek)
}

func TestScheduleState_MorningAndEveningDone(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	require.NoError(t, s.ApplyMorning(monday, []string{"task"}, nil))
	require.NoError(t, s.ApplyEvening(monday, nil, nil))

	st, err := s.ScheduleState(monday)
	require.NoError(t, err)
	assert.True(t, st.MorningDone)
	assert.True(t, st.EveningDone)
}

func TestScheduleState_WeeklyPlanPending(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	// monday is in week28. WeeklyPlanPending should be true before any plan is set.
	st, err := s.ScheduleState(monday)
	require.NoError(t, err)
	// No daily data yet, so WeeklyPlanPending may still be true (week has no plan).
	// WeekOf(monday) => week28; loadWeeklyMeta will return planned=false.
	assert.True(t, st.WeeklyPlanPending)

	// Set a plan; the pending flag should clear.
	require.NoError(t, s.SetWeeklyPlan(week28, []Item{{Text: "big thing", State: StateTodo}}))
	st, err = s.ScheduleState(monday)
	require.NoError(t, err)
	assert.False(t, st.WeeklyPlanPending)
}

func TestScheduleState_SummaryPendingCurrentWeek(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	// Add a daily file for this week so the summary is pending.
	require.NoError(t, s.ApplyMorning(monday, []string{"done thing"}, nil))
	require.NoError(t, s.SetPlanItemState(monday, 0, StateDone))

	now := monday
	st, err := s.ScheduleState(now)
	require.NoError(t, err)
	assert.True(t, st.SummaryPending)
	assert.False(t, st.SummaryPendingPastWeek) // monday is in the current week

	// Mark summarized; flag should clear.
	require.NoError(t, s.MarkWeekSummarized(week28))
	st, err = s.ScheduleState(now)
	require.NoError(t, err)
	assert.False(t, st.SummaryPending)
}

func TestScheduleState_WeekReviewPending(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	// Write a daily file in week28 (prev), then check state on the first day
	// of week29 so the review is pending.
	require.NoError(t, s.ApplyMorning(monday, []string{"prev week task"}, nil))
	// week29 starts on 2026-07-13 (Monday).
	week29Start := date("2026-07-13")
	st, err := s.ScheduleState(week29Start)
	require.NoError(t, err)
	assert.True(t, st.WeekReviewPending)

	// Apply review; flag should clear.
	require.NoError(t, s.ApplyWeekReview(week28, nil, false))
	st, err = s.ScheduleState(week29Start)
	require.NoError(t, err)
	assert.False(t, st.WeekReviewPending)
}

