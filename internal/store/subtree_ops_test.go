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
