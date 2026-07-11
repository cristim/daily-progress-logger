package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEffectiveProjects(t *testing.T) {
	t.Parallel()
	known := map[string]bool{"ship-v2": true, "internal": true}
	plan := []Item{
		{Text: "Launch @ship-v2", Depth: 0},
		{Text: "Write docs", Depth: 1}, // inherits ship-v2
		{Text: "Proofread", Depth: 2},  // inherits ship-v2 (grandparent)
		{Text: "Chore", Depth: 0},      // Unfiled (no tag)
		{Text: "Sub chore", Depth: 1},  // inherits Unfiled ("")
		{Text: "Ops @internal", Depth: 0},
	}
	got := effectiveProjects(plan, known)
	assert.Equal(t, []string{"ship-v2", "ship-v2", "ship-v2", "", "", "internal"}, got)
}

func TestSubtreeSpan(t *testing.T) {
	t.Parallel()
	plan := []Item{
		{Text: "a", Depth: 0},
		{Text: "b", Depth: 1},
		{Text: "c", Depth: 2},
		{Text: "d", Depth: 1},
		{Text: "e", Depth: 0},
	}
	start, end := subtreeSpan(plan, 0)
	assert.Equal(t, 0, start)
	assert.Equal(t, 4, end, "a's subtree covers b, c, d but stops before e")

	start, end = subtreeSpan(plan, 1)
	assert.Equal(t, 1, start)
	assert.Equal(t, 3, end, "b's subtree covers c but stops before d")

	start, end = subtreeSpan(plan, 4)
	assert.Equal(t, 4, start)
	assert.Equal(t, 5, end, "e is a leaf at the end of the plan")
}

func TestRollupDone(t *testing.T) {
	t.Parallel()
	plan := []Item{
		{Text: "Launch", State: StateTodo, Depth: 0},
		{Text: "Write docs", State: StateDone, Depth: 1},
		{Text: "Ship code", State: StateDone, Depth: 1},
		{Text: "Solo leaf", State: StateTodo, Depth: 0},
	}
	done := rollupDone(plan)
	assert.True(t, done[0], "all children done rolls the parent up to done, regardless of its own marker")
	assert.True(t, done[1])
	assert.True(t, done[2])
	assert.False(t, done[3], "a leaf uses its own checkbox state")

	// Reopen one child: the parent must reopen too.
	plan[2].State = StateTodo
	done = rollupDone(plan)
	assert.False(t, done[0], "an open descendant reopens every ancestor")
}

// TestStore_DayTasksNestsSubtasksAndInheritsProject verifies that dayTasks
// (feeding BuildProjectTree) nests subtasks under their top-level ancestor,
// carries the ancestor's effective project down through every depth, and
// rolls up Done bottom-up rather than reading a parent's own (never checked)
// state.
func TestStore_DayTasksNestsSubtasksAndInheritsProject(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	pid, err := s.AddProject("Ship v2")
	require.NoError(t, err)

	d := &Daily{
		Date: tuesday,
		Plan: []Item{
			{Text: "Launch @" + pid, State: StateTodo, Depth: 0},
			{Text: "Write docs", State: StateTodo, Depth: 1},
			{Text: "Proofread", State: StateTodo, Depth: 2},
			{Text: "Ship code", State: StateDone, Depth: 1},
		},
	}
	require.NoError(t, s.SaveDaily(d))

	tree, err := s.BuildProjectTree(tuesday)
	require.NoError(t, err)
	require.Len(t, tree.Projects, 1)
	require.Len(t, tree.Projects[0].Tasks, 1)
	launch := tree.Projects[0].Tasks[0]
	assert.Equal(t, "Launch", launch.Text, "the project tag is stripped for display")
	assert.Equal(t, 0, launch.Index)
	require.Len(t, launch.Children, 2)
	assert.Equal(t, "Write docs", launch.Children[0].Text)
	assert.Equal(t, 1, launch.Children[0].Index)
	require.Len(t, launch.Children[0].Children, 1)
	assert.Equal(t, "Proofread", launch.Children[0].Children[0].Text)
	assert.Equal(t, 2, launch.Children[0].Children[0].Index)
	assert.False(t, launch.Done, "Write docs is still open, so Launch is not done")

	// Complete the remaining open descendant: Launch rolls up to done.
	require.NoError(t, s.SetPlanItemState(tuesday, 1, StateDone))
	require.NoError(t, s.SetPlanItemState(tuesday, 2, StateDone))
	tree, err = s.BuildProjectTree(tuesday)
	require.NoError(t, err)
	assert.True(t, tree.Projects[0].Tasks[0].Done)
}
