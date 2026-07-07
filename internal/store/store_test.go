package store

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	monday  = date("2026-07-06")
	tuesday = date("2026-07-07")
	week28  = WeekID{Year: 2026, Week: 28}
	week29  = WeekID{Year: 2026, Week: 29}
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	return New(t.TempDir())
}

func TestStore_LoadDailyMissing(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	d, exists, err := s.LoadDaily(monday)
	require.NoError(t, err)
	assert.False(t, exists)
	assert.Nil(t, d)
}

func TestStore_MorningFlow(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)

	// Monday: plan two items, complete one.
	require.NoError(t, s.ApplyMorning(monday, []string{"ship feature", "review PR"}, nil))
	require.NoError(t, s.SetPlanItemState(monday, 0, StateDone))

	// A backlog item exists.
	require.NoError(t, s.SaveBacklog(&Backlog{Current: []string{"update docs"}}))

	// Tuesday morning: candidates are Monday's leftover and the backlog item.
	candidates, err := s.MorningCandidates(tuesday)
	require.NoError(t, err)
	require.Equal(t, []Candidate{
		{Text: "review PR"},
		{Text: "update docs", FromBacklog: true},
	}, candidates)

	// Adopt both plus a new item; the adopted backlog item leaves the backlog.
	require.NoError(t, s.ApplyMorning(tuesday, []string{"new task"}, candidates))
	d, exists, err := s.LoadDaily(tuesday)
	require.NoError(t, err)
	require.True(t, exists)
	assert.True(t, d.MorningDone)
	assert.Equal(t, []Item{
		{Text: "review PR", State: StateTodo},
		{Text: "update docs", State: StateTodo},
		{Text: "new task", State: StateTodo},
	}, d.Plan)

	backlog, err := s.LoadBacklog()
	require.NoError(t, err)
	assert.Empty(t, backlog.Current)

	// Candidates already planned today are not offered again: Monday's
	// "review PR" is still open there but now lives in today's plan.
	candidates, err = s.MorningCandidates(tuesday)
	require.NoError(t, err)
	assert.Empty(t, candidates)
}

func TestStore_MorningCandidatesExcludeTodaysPlan(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	require.NoError(t, s.ApplyMorning(monday, []string{"carry me"}, nil))
	require.NoError(t, s.AddPlanItem(tuesday, "carry me"))
	candidates, err := s.MorningCandidates(tuesday)
	require.NoError(t, err)
	assert.Empty(t, candidates)
}

func TestStore_EveningFlow(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	require.NoError(t, s.ApplyMorning(tuesday, []string{"task a", "task b", "task c"}, nil))

	states := []ItemState{StateDone, StateTodo, StatePostponed}
	extra := []string{"pair-debugged deploy issue"}
	require.NoError(t, s.ApplyEvening(tuesday, states, extra))

	d, _, err := s.LoadDaily(tuesday)
	require.NoError(t, err)
	assert.True(t, d.EveningDone)
	assert.Equal(t, StateDone, d.Plan[0].State)
	assert.Equal(t, StateTodo, d.Plan[1].State)
	assert.Equal(t, StatePostponed, d.Plan[2].State)
	assert.Equal(t, extra, d.Done)

	// Postponed item landed in next week's backlog.
	backlog, err := s.LoadBacklog()
	require.NoError(t, err)
	assert.Equal(t, []string{"task c"}, backlog.NextWeek)

	// Weekly summary was regenerated.
	content, err := os.ReadFile(s.WeeklyPath(week28))
	require.NoError(t, err)
	assert.Contains(t, string(content), "- task a")
	assert.Contains(t, string(content), "- pair-debugged deploy issue")
	assert.Contains(t, string(content), "## Not completed\n\n- task b")
	assert.Contains(t, string(content), "## Postponed\n\n- task c")
}

func TestStore_EveningStateMismatch(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	require.NoError(t, s.ApplyMorning(tuesday, []string{"task a"}, nil))
	err := s.ApplyEvening(tuesday, []ItemState{StateDone, StateDone}, nil)
	require.ErrorContains(t, err, "do not match plan items")
}

func TestStore_PostponeAndMoveToBacklog(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	require.NoError(t, s.ApplyMorning(tuesday, []string{"task a", "task b"}, nil))

	require.NoError(t, s.PostponePlanItem(tuesday, 0))
	require.NoError(t, s.MoveToBacklog(tuesday, 1))

	d, _, err := s.LoadDaily(tuesday)
	require.NoError(t, err)
	require.Len(t, d.Plan, 1)
	assert.Equal(t, Item{Text: "task a", State: StatePostponed}, d.Plan[0])

	backlog, err := s.LoadBacklog()
	require.NoError(t, err)
	assert.Equal(t, []string{"task a"}, backlog.NextWeek)
	assert.Equal(t, []string{"task b"}, backlog.Current)

	require.ErrorContains(t, s.PostponePlanItem(tuesday, 5), "out of range")
	require.ErrorContains(t, s.MoveToBacklog(tuesday, -1), "out of range")
}

func TestStore_UnpostponeRemovesBacklogEntry(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	require.NoError(t, s.ApplyMorning(tuesday, []string{"task a", "task b"}, nil))

	// Postpone both, then revert one directly and one via the evening
	// check-in; neither revert may leave a stale next-week backlog entry.
	require.NoError(t, s.PostponePlanItem(tuesday, 0))
	require.NoError(t, s.PostponePlanItem(tuesday, 1))

	require.NoError(t, s.SetPlanItemState(tuesday, 0, StateDone))
	backlog, err := s.LoadBacklog()
	require.NoError(t, err)
	assert.Equal(t, []string{"task b"}, backlog.NextWeek)

	require.NoError(t, s.ApplyEvening(tuesday, []ItemState{StateDone, StateTodo}, nil))
	backlog, err = s.LoadBacklog()
	require.NoError(t, err)
	assert.Empty(t, backlog.NextWeek)
}

func TestStore_UnreviewedWeekAndReview(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	nextMonday := date("2026-07-13") // first day of week 29

	// No data at all: nothing to review.
	_, pending, err := s.UnreviewedWeek(nextMonday)
	require.NoError(t, err)
	assert.False(t, pending)

	// Week 28 has data with leftovers; backlog has a current and a postponed item.
	require.NoError(t, s.ApplyMorning(tuesday, []string{"leftover a", "finished b"}, nil))
	require.NoError(t, s.ApplyEvening(tuesday, []ItemState{StateTodo, StateDone}, nil))
	require.NoError(t, s.SaveBacklog(&Backlog{
		Current:  []string{"stale current"},
		NextWeek: []string{"postponed to w29"},
	}))

	week, pending, err := s.UnreviewedWeek(nextMonday)
	require.NoError(t, err)
	require.True(t, pending)
	assert.Equal(t, week28, week)

	// Same week is not pending while it is still the current week.
	_, pending, err = s.UnreviewedWeek(tuesday)
	require.NoError(t, err)
	assert.False(t, pending)

	candidates, err := s.WeekReviewCandidates(week28)
	require.NoError(t, err)
	assert.Equal(t, []string{"leftover a", "stale current"}, candidates)

	require.NoError(t, s.ApplyWeekReview(week28, []ReviewDecision{
		{Text: "leftover a", Action: ReviewKeep},
		{Text: "stale current", Action: ReviewDrop},
	}))

	backlog, err := s.LoadBacklog()
	require.NoError(t, err)
	assert.Equal(t, []string{"postponed to w29", "leftover a"}, backlog.Current,
		"NextWeek rolls over, kept item stays, dropped item is gone")
	assert.Empty(t, backlog.NextWeek)

	// The reviewed week records the drop and the reviewed flag survives a regen.
	meta, exists, err := s.loadWeeklyMeta(week28)
	require.NoError(t, err)
	require.True(t, exists)
	assert.True(t, meta.Reviewed)
	assert.Equal(t, []string{"stale current"}, meta.Dropped)

	require.NoError(t, s.RegenerateWeekly(week28))
	meta, _, err = s.loadWeeklyMeta(week28)
	require.NoError(t, err)
	assert.True(t, meta.Reviewed, "reviewed flag must survive regeneration")
	assert.Equal(t, []string{"stale current"}, meta.Dropped, "dropped items must survive regeneration")

	// Once reviewed, no longer pending.
	_, pending, err = s.UnreviewedWeek(nextMonday)
	require.NoError(t, err)
	assert.False(t, pending)
}

func TestStore_ReviewPostpone(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	require.NoError(t, s.ApplyMorning(tuesday, []string{"maybe later"}, nil))
	require.NoError(t, s.ApplyWeekReview(week28, []ReviewDecision{
		{Text: "maybe later", Action: ReviewPostpone},
	}))
	backlog, err := s.LoadBacklog()
	require.NoError(t, err)
	assert.Empty(t, backlog.Current)
	assert.Equal(t, []string{"maybe later"}, backlog.NextWeek)
}

func TestStore_CorruptFilesFailLoud(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	require.NoError(t, os.MkdirAll(s.DataDir+"/daily/2026/07", 0o750))
	require.NoError(t, os.WriteFile(s.DailyPath(tuesday), []byte("not a daily file"), 0o600))

	_, _, err := s.LoadDaily(tuesday)
	require.ErrorContains(t, err, "missing frontmatter")

	require.NoError(t, os.WriteFile(s.BacklogPath(), []byte("## Wrong section\n"), 0o600))
	_, err = s.LoadBacklog()
	require.ErrorContains(t, err, "unknown section")
}

func TestStore_RegenerateWeeklySkipsEmptyWeek(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	require.NoError(t, s.RegenerateWeekly(week29))
	_, err := os.Stat(s.WeeklyPath(week29))
	assert.True(t, os.IsNotExist(err), "no weekly file should be created for a week without data")
}

func TestStore_DailiesInWeekOrder(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	require.NoError(t, s.AddPlanItem(tuesday, "b"))
	require.NoError(t, s.AddPlanItem(monday, "a"))
	dailies, err := s.DailiesInWeek(week28)
	require.NoError(t, err)
	require.Len(t, dailies, 2)
	assert.Equal(t, monday, dailies[0].Date)
	assert.Equal(t, tuesday, dailies[1].Date)
}

func TestMidnight(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 7, 15, 30, 45, 123, time.Local)
	assert.Equal(t, tuesday, midnight(now))
}

func TestNormalizeText(t *testing.T) {
	t.Parallel()
	tests := []struct {
		a, b  string
		equal bool
	}{
		{"Fix flaky test", "fix  flaky   test", true},
		{"Fix flaky test", "FIX FLAKY TEST", true},
		{"  leading spaces  ", "leading spaces", true},
		{"ship feature", "ship feature", true},
		{"ship feature", "ship features", false},
		{"review PR", "Update docs", false},
	}
	for _, tt := range tests {
		t.Run(tt.a+"_vs_"+tt.b, func(t *testing.T) {
			t.Parallel()
			got := normalizeText(tt.a) == normalizeText(tt.b)
			assert.Equal(t, tt.equal, got)
		})
	}
}

func TestStore_NormalizedDedup(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)

	// Different casing / whitespace should not create a duplicate plan item.
	require.NoError(t, s.ApplyMorning(monday, []string{"Fix flaky test"}, nil))
	require.NoError(t, s.AddPlanItem(monday, "fix  flaky   test"))
	d, _, err := s.LoadDaily(monday)
	require.NoError(t, err)
	assert.Len(t, d.Plan, 1, "normalized duplicate must not be added to the plan")

	// Distinct texts must both be kept.
	require.NoError(t, s.AddPlanItem(monday, "review PR"))
	d, _, err = s.LoadDaily(monday)
	require.NoError(t, err)
	assert.Len(t, d.Plan, 2, "distinct items must both appear in the plan")

	// Backlog dedup: adding an already-present entry (different case) is a no-op.
	b := &Backlog{Current: []string{"update docs"}}
	b.addCurrent("Update docs")
	assert.Len(t, b.Current, 1, "normalized duplicate must not be added to the backlog")

	b2 := &Backlog{NextWeek: []string{"big refactor"}}
	b2.addNextWeek("BIG REFACTOR")
	assert.Len(t, b2.NextWeek, 1, "normalized duplicate must not be added to NextWeek")
}
