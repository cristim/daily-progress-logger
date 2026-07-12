package store

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedParentChildDay builds a day whose plan is:
//
//	Alpha @proja   (depth 0)
//	  Beta         (depth 1, subtask of Alpha)
//	Gamma @projc   (depth 0)
func seedParentChildDay(t *testing.T) (s *Store, day time.Time, projA, projC string) {
	t.Helper()
	s, err := New(t.TempDir())
	require.NoError(t, err)
	projA, err = s.AddProject("ProjA")
	require.NoError(t, err)
	projC, err = s.AddProject("ProjC")
	require.NoError(t, err)
	day = time.Date(2026, 7, 12, 12, 0, 0, 0, time.Local)
	require.NoError(t, s.AddTaggedTask(day, "Alpha", projA))
	require.NoError(t, s.AddSubtask(day, 0, "Beta"))
	require.NoError(t, s.AddTaggedTask(day, "Gamma", projC))

	d, ok, err := s.LoadDaily(day)
	require.NoError(t, err)
	require.True(t, ok)
	require.Len(t, d.Plan, 3)
	require.Equal(t, 1, d.Plan[1].Depth, "Beta is a subtask")
	return s, day, projA, projC
}

func planOf(t *testing.T, s *Store, day time.Time) []Item {
	t.Helper()
	d, ok, err := s.LoadDaily(day)
	require.NoError(t, err)
	require.True(t, ok)
	return d.Plan
}

func TestDeleteTaskRemovesWholeSubtree(t *testing.T) {
	t.Parallel()
	s, day, _, projC := seedParentChildDay(t)
	require.NoError(t, s.DeleteTask(day, 0)) // delete Alpha, the parent of Beta

	plan := planOf(t, s, day)
	require.Len(t, plan, 1, "only Gamma remains; Beta must not be orphaned or reparented")
	assert.Equal(t, "Gamma @"+projC, plan[0].Text)
	assert.Equal(t, 0, plan[0].Depth)

	bin, err := s.LoadRecycleBin()
	require.NoError(t, err)
	require.Len(t, bin, 2, "both Alpha and Beta recycled")
	for _, e := range bin {
		assert.Equal(t, 0, e.Item.Depth, "recycled items flattened to top level")
	}
}

func TestPostponeToNextDayCarriesSubtree(t *testing.T) {
	t.Parallel()
	s, day, projA, projC := seedParentChildDay(t)
	require.NoError(t, s.PostponeToNextDay(day, 0)) // postpone Alpha + Beta

	today := planOf(t, s, day)
	require.Len(t, today, 1, "Beta must not be left behind on today")
	assert.Equal(t, "Gamma @"+projC, today[0].Text)

	tomorrow := planOf(t, s, day.AddDate(0, 0, 1))
	require.Len(t, tomorrow, 2, "Alpha and its nested Beta both carried over")
	assert.Equal(t, "Alpha @"+projA, tomorrow[0].Text)
	assert.Equal(t, StateTodo, tomorrow[0].State, "parent re-planned as todo")
	assert.Equal(t, 0, tomorrow[0].Depth)
	assert.Equal(t, "Beta", tomorrow[1].Text)
	assert.Equal(t, 1, tomorrow[1].Depth, "nesting preserved across the postpone")
}

func TestReorderTaskSiblingsWithinProject(t *testing.T) {
	t.Parallel()
	s, err := New(t.TempDir())
	require.NoError(t, err)
	proj, err := s.AddProject("Proj")
	require.NoError(t, err)
	day := time.Date(2026, 7, 12, 12, 0, 0, 0, time.Local)
	require.NoError(t, s.AddTaggedTask(day, "Alpha", proj))
	require.NoError(t, s.AddTaggedTask(day, "Beta", proj))

	// Beta (index 1) moves before Alpha (index 0): order changes, both tags
	// stay intact since both are already in proj.
	require.NoError(t, s.ReorderTask(day, 1, 0, false))

	plan := planOf(t, s, day)
	require.Len(t, plan, 2)
	assert.Equal(t, "Beta @"+proj, plan[0].Text)
	assert.Equal(t, "Alpha @"+proj, plan[1].Text)
	assert.Equal(t, 0, plan[0].Depth)
	assert.Equal(t, 0, plan[1].Depth)
}

func TestReorderTaskBetweenSubtasksBecomesSiblingAtDepth(t *testing.T) {
	t.Parallel()
	s, err := New(t.TempDir())
	require.NoError(t, err)
	projA, err := s.AddProject("ProjA")
	require.NoError(t, err)
	projB, err := s.AddProject("ProjB")
	require.NoError(t, err)
	day := time.Date(2026, 7, 12, 12, 0, 0, 0, time.Local)
	require.NoError(t, s.AddTaggedTask(day, "X", projA)) // index 0
	require.NoError(t, s.AddSubtask(day, 0, "C1"))       // index 1, depth 1
	require.NoError(t, s.AddSubtask(day, 0, "C2"))       // index 2, depth 1
	require.NoError(t, s.AddTaggedTask(day, "Y", projB)) // index 3

	// Y (a tagged top-level task) moves to sit right after C1, becoming X's
	// child sandwiched between C1 and C2: it adopts C1's depth (1) and drops
	// its own project tag (it now inherits X's).
	require.NoError(t, s.ReorderTask(day, 3, 1, true))

	plan := planOf(t, s, day)
	require.Len(t, plan, 4)
	assert.Equal(t, "X @"+projA, plan[0].Text)
	assert.Equal(t, "C1", plan[1].Text)
	assert.Equal(t, "Y", plan[2].Text, "Y's own @projB tag is stripped; it now inherits X")
	assert.Equal(t, 1, plan[2].Depth, "Y adopts C1's depth, becoming X's child")
	assert.Equal(t, "C2", plan[3].Text)
}

func TestReorderTaskCrossProjectRetags(t *testing.T) {
	t.Parallel()
	s, err := New(t.TempDir())
	require.NoError(t, err)
	projA, err := s.AddProject("ProjA")
	require.NoError(t, err)
	projB, err := s.AddProject("ProjB")
	require.NoError(t, err)
	day := time.Date(2026, 7, 12, 12, 0, 0, 0, time.Local)
	require.NoError(t, s.AddTaggedTask(day, "Zulu", projA))  // index 0
	require.NoError(t, s.AddTaggedTask(day, "Mango", projB)) // index 1

	// Zulu moves to sit right after Mango: the order changes and Zulu is
	// retagged to Mango's project.
	require.NoError(t, s.ReorderTask(day, 0, 1, true))

	plan := planOf(t, s, day)
	require.Len(t, plan, 2)
	assert.Equal(t, "Mango @"+projB, plan[0].Text)
	assert.Equal(t, "Zulu @"+projB, plan[1].Text, "Zulu is retagged to its new sibling's project")
}

func TestReorderTaskCycleGuardIsNoOp(t *testing.T) {
	t.Parallel()
	s, day, projA, _ := seedParentChildDay(t) // Alpha @projA (depth 0) -> Beta (depth 1)
	before := planOf(t, s, day)

	require.NoError(t, s.ReorderTask(day, 0, 0, false), "refIndex == srcIndex is a no-op")
	require.NoError(t, s.ReorderTask(day, 0, 1, false), "refIndex inside src's own subtree is a no-op")

	after := planOf(t, s, day)
	assert.Equal(t, before, after, "the plan is unchanged by either no-op request")
	assert.Equal(t, "Alpha @"+projA, after[0].Text)
}

func TestReorderTaskOutOfRange(t *testing.T) {
	t.Parallel()
	s, day, _, _ := seedParentChildDay(t)
	require.ErrorContains(t, s.ReorderTask(day, 99, 0, false), "out of range")
	require.ErrorContains(t, s.ReorderTask(day, 0, 99, false), "out of range")
}

func TestMoveToBacklogRemovesWholeSubtree(t *testing.T) {
	t.Parallel()
	s, day, _, projC := seedParentChildDay(t)
	require.NoError(t, s.MoveToBacklog(day, 0)) // backlog Alpha + Beta

	plan := planOf(t, s, day)
	require.Len(t, plan, 1, "Beta must not be orphaned on the plan")
	assert.Equal(t, "Gamma @"+projC, plan[0].Text)

	backlog, err := s.LoadBacklog()
	require.NoError(t, err)
	require.Len(t, backlog.Current, 2, "both Alpha and Beta enqueued, no orphan left behind")
}
