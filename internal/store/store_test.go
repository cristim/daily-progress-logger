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
	s, err := New(t.TempDir())
	require.NoError(t, err)
	return s
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

	decisions := []EveningDecision{
		{Text: "task a", Action: EveningActionDone},
		{Text: "task b", Action: EveningActionTodo},
		{Text: "task c", Action: EveningActionNextWeek},
	}
	extra := []string{"pair-debugged deploy issue"}
	require.NoError(t, s.ApplyEvening(tuesday, decisions, extra))

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

func TestStore_EveningPlanChangedMidDialog(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	require.NoError(t, s.ApplyMorning(tuesday, []string{"task a", "task b"}, nil))

	// Simulate a plan item added while the evening dialog was already open.
	require.NoError(t, s.AddPlanItem(tuesday, "task c"))

	// The dialog only captured decisions for the original two items.
	decisions := []EveningDecision{
		{Text: "task a", Action: EveningActionDone},
		{Text: "task b", Action: EveningActionNextWeek},
	}
	// Applying must not error; "task c" must keep its current (Todo) state.
	require.NoError(t, s.ApplyEvening(tuesday, decisions, nil))

	d, _, err := s.LoadDaily(tuesday)
	require.NoError(t, err)
	require.Len(t, d.Plan, 3)
	assert.Equal(t, StateDone, d.Plan[0].State)
	assert.Equal(t, StatePostponed, d.Plan[1].State)
	assert.Equal(t, StateTodo, d.Plan[2].State, "plan item added mid-dialog must keep its current state")
}

func TestStore_EveningUnknownTextIgnored(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	require.NoError(t, s.ApplyMorning(tuesday, []string{"task a"}, nil))

	// Decision for a text that no longer exists must be silently ignored.
	decisions := []EveningDecision{
		{Text: "task a", Action: EveningActionDone},
		{Text: "deleted item", Action: EveningActionTodo},
	}
	require.NoError(t, s.ApplyEvening(tuesday, decisions, nil))

	d, _, err := s.LoadDaily(tuesday)
	require.NoError(t, err)
	require.Len(t, d.Plan, 1)
	assert.Equal(t, StateDone, d.Plan[0].State)
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

func TestStore_PostponeToNextDay(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	wednesday := date("2026-07-08")
	require.NoError(t, s.ApplyMorning(tuesday, []string{"task a", "task b"}, nil))

	// Postpone "task a" to the next day: it leaves Tuesday and appears as a
	// fresh todo on Wednesday.
	require.NoError(t, s.PostponeToNextDay(tuesday, 0))

	tue, _, err := s.LoadDaily(tuesday)
	require.NoError(t, err)
	assert.Equal(t, []Item{{Text: "task b", State: StateTodo}}, tue.Plan)

	wed, exists, err := s.LoadDaily(wednesday)
	require.NoError(t, err)
	require.True(t, exists)
	assert.Equal(t, []Item{{Text: "task a", State: StateTodo}}, wed.Plan)

	// It leaves no backlog entry (unlike next-week).
	backlog, err := s.LoadBacklog()
	require.NoError(t, err)
	assert.Empty(t, backlog.NextWeek)
	assert.Empty(t, backlog.Current)

	// Dedup: postponing "task b" to Wednesday where an identical item already
	// exists must not duplicate it.
	require.NoError(t, s.AddPlanItem(wednesday, "task b"))
	require.NoError(t, s.PostponeToNextDay(tuesday, 0)) // "task b" is now index 0
	wed, _, err = s.LoadDaily(wednesday)
	require.NoError(t, err)
	assert.Equal(t, []Item{
		{Text: "task a", State: StateTodo},
		{Text: "task b", State: StateTodo},
	}, wed.Plan)

	require.ErrorContains(t, s.PostponeToNextDay(tuesday, 9), "out of range")
}

func TestStore_PostponeToNextDayClearsStalePostpone(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	require.NoError(t, s.ApplyMorning(tuesday, []string{"task a"}, nil))

	// Mark it postponed (next week) first, then carry it to the next day: the
	// stale next-week backlog entry must be dropped.
	require.NoError(t, s.PostponePlanItem(tuesday, 0))
	require.NoError(t, s.PostponeToNextDay(tuesday, 0))

	backlog, err := s.LoadBacklog()
	require.NoError(t, err)
	assert.Empty(t, backlog.NextWeek)

	wed, _, err := s.LoadDaily(date("2026-07-08"))
	require.NoError(t, err)
	assert.Equal(t, []Item{{Text: "task a", State: StateTodo}}, wed.Plan)
}

func TestStore_EveningNextDayAndBacklog(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	wednesday := date("2026-07-08")
	require.NoError(t, s.ApplyMorning(tuesday, []string{"to tomorrow", "to backlog", "keep todo"}, nil))

	require.NoError(t, s.ApplyEvening(tuesday, []EveningDecision{
		{Text: "to tomorrow", Action: EveningActionNextDay},
		{Text: "to backlog", Action: EveningActionBacklog},
		{Text: "keep todo", Action: EveningActionTodo},
	}, nil))

	// Only the kept item remains in Tuesday's plan.
	tue, _, err := s.LoadDaily(tuesday)
	require.NoError(t, err)
	assert.Equal(t, []Item{{Text: "keep todo", State: StateTodo}}, tue.Plan)

	// Next-day item moved into Wednesday's plan.
	wed, _, err := s.LoadDaily(wednesday)
	require.NoError(t, err)
	assert.Equal(t, []Item{{Text: "to tomorrow", State: StateTodo}}, wed.Plan)

	// Backlog item landed in this week's Current section, not NextWeek.
	backlog, err := s.LoadBacklog()
	require.NoError(t, err)
	assert.Equal(t, []string{"to backlog"}, backlog.Current)
	assert.Empty(t, backlog.NextWeek)
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

	require.NoError(t, s.ApplyEvening(tuesday, []EveningDecision{
		{Text: "task a", Action: EveningActionDone},
		{Text: "task b", Action: EveningActionTodo},
	}, nil))
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
	require.NoError(t, s.ApplyEvening(tuesday, []EveningDecision{
		{Text: "leftover a", Action: EveningActionTodo},
		{Text: "finished b", Action: EveningActionDone},
	}, nil))
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
	}, true))

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

func TestStore_UnreviewedWeeksOldestFirst(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)

	// Create data in two past weeks: week 27 (older) and week 28.
	week27monday := date("2026-06-29")
	week27 := WeekID{Year: 2026, Week: 27}
	nextMonday := date("2026-07-13")

	require.NoError(t, s.ApplyMorning(week27monday, []string{"week27 task"}, nil))
	require.NoError(t, s.ApplyMorning(tuesday, []string{"week28 task"}, nil))

	// First unreviewed week should be the older one (week 27).
	week, pending, err := s.UnreviewedWeek(nextMonday)
	require.NoError(t, err)
	require.True(t, pending)
	assert.Equal(t, week27, week, "oldest unreviewed week must come first")

	// After reviewing week 27, the next call should return week 28.
	require.NoError(t, s.ApplyWeekReview(week27, nil, true))
	week, pending, err = s.UnreviewedWeek(nextMonday)
	require.NoError(t, err)
	require.True(t, pending)
	assert.Equal(t, week28, week, "second call must return the next-oldest unreviewed week")

	// After reviewing week 28 as well, nothing is pending.
	require.NoError(t, s.ApplyWeekReview(week28, nil, true))
	_, pending, err = s.UnreviewedWeek(nextMonday)
	require.NoError(t, err)
	assert.False(t, pending)
}

func TestStore_WeekSummaryPendingAndMark(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)

	// No daily data: summary is not pending.
	_, pending, err := s.WeekSummaryPending(tuesday)
	require.NoError(t, err)
	assert.False(t, pending, "no data means summary not pending")

	// Add data for the current week.
	require.NoError(t, s.ApplyMorning(tuesday, []string{"task a"}, nil))

	_, pending, err = s.WeekSummaryPending(tuesday)
	require.NoError(t, err)
	assert.True(t, pending, "week with data must be pending")

	// Mark summarized.
	require.NoError(t, s.MarkWeekSummarized(week28))

	_, pending, err = s.WeekSummaryPending(tuesday)
	require.NoError(t, err)
	assert.False(t, pending, "after marking summarized the week must not be pending")

	// Summarized flag survives regeneration.
	require.NoError(t, s.RegenerateWeekly(week28))
	meta, _, err := s.loadWeeklyMeta(week28)
	require.NoError(t, err)
	assert.True(t, meta.Summarized, "summarized flag must survive regeneration")
}

func TestStore_WeekSummaryPendingWeekID(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	require.NoError(t, s.ApplyMorning(tuesday, []string{"task"}, nil))
	week, _, err := s.WeekSummaryPending(tuesday)
	require.NoError(t, err)
	assert.Equal(t, week28, week, "WeekSummaryPending must return the current week's WeekID")
}

func TestStore_ReviewPostpone(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	require.NoError(t, s.ApplyMorning(tuesday, []string{"maybe later"}, nil))
	require.NoError(t, s.ApplyWeekReview(week28, []ReviewDecision{
		{Text: "maybe later", Action: ReviewPostpone},
	}, true))
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

// TestStore_ApplyWeekReviewWithRollover verifies that rollover=true promotes
// NextWeek items into Current (scheduled Monday review behaviour).
func TestStore_ApplyWeekReviewWithRollover(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)

	require.NoError(t, s.SaveBacklog(&Backlog{
		NextWeek: []string{"deferred task"},
	}))

	require.NoError(t, s.ApplyMorning(tuesday, []string{"open item"}, nil))
	require.NoError(t, s.ApplyWeekReview(week28, []ReviewDecision{
		{Text: "open item", Action: ReviewKeep},
	}, true)) // rollover=true: NextWeek -> Current before decisions

	backlog, err := s.LoadBacklog()
	require.NoError(t, err)
	assert.Equal(t, []string{"deferred task", "open item"}, backlog.Current,
		"rollover must promote NextWeek items and keep decision is applied")
	assert.Empty(t, backlog.NextWeek)
}

func TestStore_AdoptFromBacklog(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	require.NoError(t, s.SaveBacklog(&Backlog{
		Current:  []string{"task a", "task b"},
		NextWeek: []string{"task c"},
	}))

	// Adopting from Current: item lands in today's plan and is removed from Current.
	require.NoError(t, s.AdoptFromBacklog(tuesday, "task a"))
	d, _, err := s.LoadDaily(tuesday)
	require.NoError(t, err)
	require.Len(t, d.Plan, 1)
	assert.Equal(t, "task a", d.Plan[0].Text)
	assert.Equal(t, StateTodo, d.Plan[0].State)

	backlog, err := s.LoadBacklog()
	require.NoError(t, err)
	assert.Equal(t, []string{"task b"}, backlog.Current)
	assert.Equal(t, []string{"task c"}, backlog.NextWeek)

	// Adopting from NextWeek: item lands in plan and is removed from NextWeek.
	require.NoError(t, s.AdoptFromBacklog(tuesday, "task c"))
	d, _, err = s.LoadDaily(tuesday)
	require.NoError(t, err)
	require.Len(t, d.Plan, 2)

	backlog, err = s.LoadBacklog()
	require.NoError(t, err)
	assert.Equal(t, []string{"task b"}, backlog.Current)
	assert.Empty(t, backlog.NextWeek)
}

func TestStore_AdoptFromBacklogAlreadyPlanned(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	require.NoError(t, s.AddPlanItem(tuesday, "task a"))
	require.NoError(t, s.SaveBacklog(&Backlog{
		Current:  []string{"task a"},
		NextWeek: []string{"task a"}, // same item in both sections
	}))

	// Already planned: plan stays at 1 item (no dup) but backlog is still cleaned,
	// and the item is reset to StateTodo.
	require.NoError(t, s.AdoptFromBacklog(tuesday, "task a"))

	d, _, err := s.LoadDaily(tuesday)
	require.NoError(t, err)
	assert.Len(t, d.Plan, 1, "already-planned item must not be duplicated")
	assert.Equal(t, StateTodo, d.Plan[0].State, "adopted item must be reset to todo")

	backlog, err := s.LoadBacklog()
	require.NoError(t, err)
	assert.Empty(t, backlog.Current, "item must be removed from Current")
	assert.Empty(t, backlog.NextWeek, "item must be removed from NextWeek")
}

// TestStore_AdoptFromBacklogPostponedResetsState verifies that adopting a
// postponed plan item resets its state to todo so it is re-planned for today
// (finding 32: the old AddPlanItem no-op left the item in StatePostponed).
func TestStore_AdoptFromBacklogPostponedResetsState(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)

	// Postpone an item so it appears in both the plan (as StatePostponed) and
	// the backlog NextWeek section.
	require.NoError(t, s.ApplyMorning(tuesday, []string{"update the runbook"}, nil))
	require.NoError(t, s.PostponePlanItem(tuesday, 0))

	// Confirm the setup: plan has the item as postponed, backlog has it in NextWeek.
	d, _, err := s.LoadDaily(tuesday)
	require.NoError(t, err)
	require.Equal(t, StatePostponed, d.Plan[0].State)

	backlog, err := s.LoadBacklog()
	require.NoError(t, err)
	require.Equal(t, []string{"update the runbook"}, backlog.NextWeek)

	// Adopt from the backlog dialog: plan item becomes todo, backlog is cleared.
	require.NoError(t, s.AdoptFromBacklog(tuesday, "update the runbook"))

	d, _, err = s.LoadDaily(tuesday)
	require.NoError(t, err)
	require.Len(t, d.Plan, 1)
	assert.Equal(t, StateTodo, d.Plan[0].State, "adopted postponed item must be reset to todo")

	backlog, err = s.LoadBacklog()
	require.NoError(t, err)
	assert.Empty(t, backlog.Current, "Current backlog must be empty after adopt")
	assert.Empty(t, backlog.NextWeek, "NextWeek backlog must be empty after adopt")
}

func TestStore_AdoptFromBacklogNotPresent(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	// The item is not in the backlog at all (user may have edited the file).
	require.NoError(t, s.SaveBacklog(&Backlog{Current: []string{"other"}}))

	// Must not return an error; the item still lands in the plan.
	require.NoError(t, s.AdoptFromBacklog(tuesday, "vanished item"))
	d, _, err := s.LoadDaily(tuesday)
	require.NoError(t, err)
	require.Len(t, d.Plan, 1)
	assert.Equal(t, "vanished item", d.Plan[0].Text)
}

func TestStore_MoveBacklogItem(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	require.NoError(t, s.SaveBacklog(&Backlog{
		Current:  []string{"task a"},
		NextWeek: []string{"task b"},
	}))

	// Current -> NextWeek
	require.NoError(t, s.MoveBacklogItem("task a", true))
	backlog, err := s.LoadBacklog()
	require.NoError(t, err)
	assert.Empty(t, backlog.Current)
	assert.Equal(t, []string{"task b", "task a"}, backlog.NextWeek)

	// NextWeek -> Current
	require.NoError(t, s.MoveBacklogItem("task b", false))
	backlog, err = s.LoadBacklog()
	require.NoError(t, err)
	assert.Equal(t, []string{"task b"}, backlog.Current)
	assert.Equal(t, []string{"task a"}, backlog.NextWeek)
}

func TestStore_MoveBacklogItemNotFound(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	require.NoError(t, s.SaveBacklog(&Backlog{
		Current: []string{"task a"},
	}))

	require.ErrorContains(t, s.MoveBacklogItem("unknown", true), "not found")
	require.ErrorContains(t, s.MoveBacklogItem("unknown", false), "not found")
}

func TestStore_MoveBacklogItemDedup(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	// "task a" is already in NextWeek; moving it there from Current must not duplicate.
	require.NoError(t, s.SaveBacklog(&Backlog{
		Current:  []string{"task a", "task b"},
		NextWeek: []string{"task a"},
	}))

	require.NoError(t, s.MoveBacklogItem("task a", true))
	backlog, err := s.LoadBacklog()
	require.NoError(t, err)
	assert.Equal(t, []string{"task b"}, backlog.Current)
	assert.Equal(t, []string{"task a"}, backlog.NextWeek, "duplicate must not be added on arrival")
}

// TestStore_WeekSummaryPendingLookBack verifies that WeekSummaryPending returns
// a past week when the app was not running during the scheduled Friday window
// (finding 41: the old implementation only inspected the current week).
func TestStore_WeekSummaryPendingLookBack(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	nextMonday := date("2026-07-13") // first day of week 29

	// Week 28 has data but no summary yet.
	require.NoError(t, s.ApplyMorning(tuesday, []string{"task"}, nil))

	// Querying on the following Monday (week 29): the current week (29) has no
	// data, so the look-back should find week 28.
	pendingWeek, pending, err := s.WeekSummaryPending(nextMonday)
	require.NoError(t, err)
	require.True(t, pending, "missed Friday summary must be pending on a later day")
	assert.Equal(t, week28, pendingWeek, "look-back must return week 28")

	// After marking week 28 summarized, nothing is pending.
	require.NoError(t, s.MarkWeekSummarized(week28))
	_, pending, err = s.WeekSummaryPending(nextMonday)
	require.NoError(t, err)
	assert.False(t, pending, "after marking summarized nothing must be pending")
}

// TestStore_ApplyWeekReviewWithoutRollover verifies that rollover=false leaves
// NextWeek items untouched (manual mid-week re-triage behaviour).
func TestStore_ApplyWeekReviewWithoutRollover(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)

	require.NoError(t, s.SaveBacklog(&Backlog{
		NextWeek: []string{"deferred task"},
	}))

	require.NoError(t, s.ApplyMorning(tuesday, []string{"open item"}, nil))
	require.NoError(t, s.ApplyWeekReview(week28, []ReviewDecision{
		{Text: "open item", Action: ReviewKeep},
	}, false)) // rollover=false: NextWeek must not be touched

	backlog, err := s.LoadBacklog()
	require.NoError(t, err)
	assert.Equal(t, []string{"open item"}, backlog.Current,
		"without rollover only the kept item enters Current")
	assert.Equal(t, []string{"deferred task"}, backlog.NextWeek,
		"NextWeek must remain untouched when rollover=false")
}

// TestStore_ReviewDropClearsNextWeek verifies that a Drop decision removes the
// item from the NextWeek section as well as Current, so it cannot resurface via
// rollover (finding 40). Tests both the scheduled (rollover=true) and manual
// (rollover=false) paths.
func TestStore_ReviewDropClearsNextWeek(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)

	// An item is both a review candidate (open in the week) and in NextWeek
	// (e.g. postponed on a different day of the same week).
	require.NoError(t, s.ApplyMorning(tuesday, []string{"item to drop"}, nil))
	require.NoError(t, s.SaveBacklog(&Backlog{
		NextWeek: []string{"item to drop"},
	}))

	require.NoError(t, s.ApplyWeekReview(week28, []ReviewDecision{
		{Text: "item to drop", Action: ReviewDrop},
	}, false)) // rollover=false: manual review, NextWeek must still be cleared on drop

	backlog, err := s.LoadBacklog()
	require.NoError(t, err)
	assert.Empty(t, backlog.Current, "dropped item must not appear in Current")
	assert.Empty(t, backlog.NextWeek, "dropped item must be removed from NextWeek")
}

func TestStore_WeeklyPlan(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)

	// Before planning, the current week is pending.
	wk, pending, err := s.WeeklyPlanPending(tuesday)
	require.NoError(t, err)
	assert.Equal(t, week28, wk)
	assert.True(t, pending)

	// Set the big things: one already done, plus a blank and a normalized
	// duplicate that must be dropped.
	require.NoError(t, s.SetWeeklyPlan(week28, []Item{
		{Text: "Ship v2", State: StateDone},
		{Text: "Hire designer", State: StateTodo},
		{Text: "  ", State: StateTodo},
		{Text: "ship v2", State: StateTodo},
	}))

	goals, planned, err := s.WeeklyPlan(week28)
	require.NoError(t, err)
	assert.True(t, planned)
	assert.Equal(t, []Item{
		{Text: "Ship v2", State: StateDone},
		{Text: "Hire designer", State: StateTodo},
	}, goals)

	// No longer pending once planned.
	_, pending, err = s.WeeklyPlanPending(tuesday)
	require.NoError(t, err)
	assert.False(t, pending)
}

func TestStore_WeeklyPlanSurvivesRegeneration(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	require.NoError(t, s.SetWeeklyPlan(week28, []Item{{Text: "Big goal", State: StateDone}}))

	// A daily edit regenerates the weekly file; the plan and its ticks must
	// survive because they live in the carried-over meta, not a derived section.
	require.NoError(t, s.ApplyMorning(tuesday, []string{"task"}, nil))
	require.NoError(t, s.ApplyEvening(tuesday, []EveningDecision{
		{Text: "task", Action: EveningActionDone},
	}, nil))

	goals, planned, err := s.WeeklyPlan(week28)
	require.NoError(t, err)
	assert.True(t, planned)
	assert.Equal(t, []Item{{Text: "Big goal", State: StateDone}}, goals)

	// An empty plan still marks the week planned (so it is not re-prompted).
	require.NoError(t, s.SetWeeklyPlan(week29, nil))
	goals, planned, err = s.WeeklyPlan(week29)
	require.NoError(t, err)
	assert.True(t, planned)
	assert.Empty(t, goals)
}
