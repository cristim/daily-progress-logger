package store

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cristim/daily-progress-logger/internal/recur"
)

func TestStore_RecurringRoundTrip(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)

	require.NoError(t, s.AddRecurring("Team standup @weekday @9:30"))
	require.NoError(t, s.AddRecurring("Team standup @weekday @9:30")) // dedup
	require.NoError(t, s.AddRecurring("Review metrics @weekly @mon @16:00"))

	tasks, err := s.RecurringTasks()
	require.NoError(t, err)
	require.Len(t, tasks, 2)

	assert.Equal(t, "Team standup", tasks[0].Text)
	assert.Equal(t, recur.Weekday, tasks[0].Rec.Kind)
	assert.Equal(t, 9, tasks[0].Rec.Hour)
	assert.Equal(t, 30, tasks[0].Rec.Minute)

	assert.Equal(t, "Review metrics", tasks[1].Text)
	assert.Equal(t, recur.Weekly, tasks[1].Rec.Kind)
	assert.Equal(t, time.Monday, tasks[1].Rec.Weekday)

	require.NoError(t, s.RemoveRecurring("Team standup @weekday @9:30"))
	tasks, err = s.RecurringTasks()
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	assert.Equal(t, "Review metrics", tasks[0].Text)
}

func TestStore_RecurringDefaultTime(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	s.SetDefaultReminderTime(8, 15)
	require.NoError(t, s.AddRecurring("Vitamins @daily"))

	tasks, err := s.RecurringTasks()
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	assert.Equal(t, 8, tasks[0].Rec.Hour)
	assert.Equal(t, 15, tasks[0].Rec.Minute)
}

func TestStore_RecurringPreservesProjectTag(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	pid, err := s.AddProject("Payments")
	require.NoError(t, err)

	require.NoError(t, s.AddRecurring("Reconcile ledger @"+pid+" @weekly @fri @17:00"))
	tasks, err := s.RecurringTasks()
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	assert.Equal(t, "Reconcile ledger", tasks[0].Text)
	assert.Equal(t, pid, tasks[0].Project)
	assert.Equal(t, time.Friday, tasks[0].Rec.Weekday)
}

func TestStore_RecurringProjectSlugShapedLikeToken(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	// H1: A project named "Mon" now receives a non-reserved slug (suffixed,
	// e.g. "mon-2") because "mon" is a weekday recurrence token. The recurring
	// template parser must still handle the suffixed slug correctly as a project
	// tag and not consume it as a recurrence weekday.
	pid, err := s.AddProject("Mon")
	require.NoError(t, err)
	assert.NotEqual(t, "mon", pid, "reserved slug must be suffixed")

	require.NoError(t, s.AddRecurring("Standup @"+pid+" @weekly @fri @9:00"))
	tasks, err := s.RecurringTasks()
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	assert.Equal(t, "Standup", tasks[0].Text)
	assert.Equal(t, pid, tasks[0].Project)
	assert.Equal(t, time.Friday, tasks[0].Rec.Weekday) // weekday set by @fri, not by pid
}

func TestStore_AddRecurringRejectsNonRecurring(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	require.Error(t, s.AddRecurring("Just a normal task"))
	require.Error(t, s.AddRecurring("@daily @9:00"), "tags-only, no description")
	tasks, err := s.RecurringTasks()
	require.NoError(t, err)
	assert.Empty(t, tasks)
}

func TestStore_RecurringDueFiresOncePerOccurrence(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	require.NoError(t, s.AddRecurring("Standup @daily @9:00"))

	// Wednesday 08:00 — first sight baselines to Tuesday 09:00 without firing.
	wedMorning := time.Date(2026, 7, 8, 8, 0, 0, 0, time.Local)
	due, err := s.RecurringDue(wedMorning)
	require.NoError(t, err)
	assert.Empty(t, due, "first sight baselines, does not fire")

	// Still before today's 09:00 — nothing new.
	due, err = s.RecurringDue(wedMorning.Add(30 * time.Minute))
	require.NoError(t, err)
	assert.Empty(t, due)

	// Wednesday 09:05 — today's occurrence passed, fires once.
	due, err = s.RecurringDue(time.Date(2026, 7, 8, 9, 5, 0, 0, time.Local))
	require.NoError(t, err)
	require.Len(t, due, 1)
	assert.Equal(t, "Standup", due[0].Text)

	// Same day, later — does not fire again.
	due, err = s.RecurringDue(time.Date(2026, 7, 8, 14, 0, 0, 0, time.Local))
	require.NoError(t, err)
	assert.Empty(t, due)

	// Next day past 09:00 — the new occurrence fires (catch-up across the gap).
	due, err = s.RecurringDue(time.Date(2026, 7, 9, 12, 0, 0, 0, time.Local))
	require.NoError(t, err)
	require.Len(t, due, 1)
	assert.Equal(t, "Standup", due[0].Text)
}

func TestStore_MaterializeRecurring_OccursTodayOnce(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	require.NoError(t, s.AddRecurring("Standup @daily"))

	today := midnight(time.Now())
	added, err := s.MaterializeRecurring(today)
	require.NoError(t, err)
	require.Len(t, added, 1)
	assert.Equal(t, "Standup", added[0].Text)

	d, exists, err := s.LoadDaily(today)
	require.NoError(t, err)
	require.True(t, exists)
	require.Len(t, d.Plan, 1)
	assert.Equal(t, "Standup", d.Plan[0].Text)

	// Second call for the same day materializes nothing further.
	added, err = s.MaterializeRecurring(today)
	require.NoError(t, err)
	assert.Empty(t, added)
	d, _, err = s.LoadDaily(today)
	require.NoError(t, err)
	assert.Len(t, d.Plan, 1, "not duplicated")
}

func TestStore_MaterializeRecurring_DeletedTaskNotReadded(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	require.NoError(t, s.AddRecurring("Standup @daily"))

	today := midnight(time.Now())
	added, err := s.MaterializeRecurring(today)
	require.NoError(t, err)
	require.Len(t, added, 1)

	// Delete the materialized task from the day's plan.
	require.NoError(t, s.DeleteTask(today, 0))
	d, _, err := s.LoadDaily(today)
	require.NoError(t, err)
	require.Empty(t, d.Plan, "task removed from the plan")

	// Materializing the same day again must not resurrect it: the state file
	// already marks this (template, day) as done.
	added, err = s.MaterializeRecurring(today)
	require.NoError(t, err)
	assert.Empty(t, added)
	d, _, err = s.LoadDaily(today)
	require.NoError(t, err)
	assert.Empty(t, d.Plan)
}

func TestStore_MaterializeRecurring_PastDateIsNoop(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	require.NoError(t, s.AddRecurring("Standup @daily"))

	yesterday := midnight(time.Now()).AddDate(0, 0, -1)
	added, err := s.MaterializeRecurring(yesterday)
	require.NoError(t, err)
	assert.Empty(t, added)

	_, exists, err := s.LoadDaily(yesterday)
	require.NoError(t, err)
	assert.False(t, exists, "past day's plan file must not be created")
}

func TestStore_MaterializeRecurring_NonMatchingDayAddsNothing(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	require.NoError(t, s.AddRecurring("Standup @weekly @mon"))

	// Find the next day at or after today that is not a Monday.
	notMonday := midnight(time.Now())
	for notMonday.Weekday() == time.Monday {
		notMonday = notMonday.AddDate(0, 0, 1)
	}

	added, err := s.MaterializeRecurring(notMonday)
	require.NoError(t, err)
	assert.Empty(t, added)

	_, exists, err := s.LoadDaily(notMonday)
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestStore_MaterializeRecurring_ProjectTaggedAndUnfiled(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	pid, err := s.AddProject("Payments")
	require.NoError(t, err)
	require.NoError(t, s.AddRecurring("Reconcile ledger @"+pid+" @daily"))
	require.NoError(t, s.AddRecurring("Vitamins @daily"))

	today := midnight(time.Now())
	added, err := s.MaterializeRecurring(today)
	require.NoError(t, err)
	require.Len(t, added, 2)

	tree, err := s.BuildProjectTree(today)
	require.NoError(t, err)
	require.Len(t, tree.Projects, 1)
	require.Len(t, tree.Projects[0].Tasks, 1)
	assert.Equal(t, "Reconcile ledger", tree.Projects[0].Tasks[0].Text)

	require.Len(t, tree.Unfiled, 1)
	assert.Equal(t, "Vitamins", tree.Unfiled[0].Text)
}

func TestStore_RecurringDuePersistsAcrossReload(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	s, err := New(dir)
	require.NoError(t, err)
	require.NoError(t, s.AddRecurring("Standup @daily @9:00"))

	base := time.Date(2026, 7, 8, 8, 0, 0, 0, time.Local)
	_, err = s.RecurringDue(base) // baseline
	require.NoError(t, err)
	due, err := s.RecurringDue(time.Date(2026, 7, 8, 9, 5, 0, 0, time.Local))
	require.NoError(t, err)
	require.Len(t, due, 1)

	// A fresh store over the same dir must not re-fire the already-fired
	// occurrence (firing state persisted to disk).
	s2, err := New(dir)
	require.NoError(t, err)
	due, err = s2.RecurringDue(time.Date(2026, 7, 8, 15, 0, 0, 0, time.Local))
	require.NoError(t, err)
	assert.Empty(t, due)
}
