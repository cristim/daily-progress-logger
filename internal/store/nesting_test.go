package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStore_AddSubtask(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	pid, err := s.AddProject("Ship v2")
	require.NoError(t, err)
	require.NoError(t, s.AddTaggedTask(tuesday, "Task A", pid))

	require.NoError(t, s.AddSubtask(tuesday, 0, "S1"))
	require.NoError(t, s.AddSubtask(tuesday, 0, "S2")) // appended as the last child, after S1

	d, _, err := s.LoadDaily(tuesday)
	require.NoError(t, err)
	require.Len(t, d.Plan, 3)
	assert.Equal(t, []Item{
		{Text: "Task A @ship-v2", State: StateTodo, Depth: 0},
		{Text: "S1", State: StateTodo, Depth: 1},
		{Text: "S2", State: StateTodo, Depth: 1},
	}, d.Plan)

	require.ErrorContains(t, s.AddSubtask(tuesday, 9, "x"), "out of range")
	require.NoError(t, s.AddSubtask(tuesday, 0, "  "), "blank text is a silent no-op")
	d, _, err = s.LoadDaily(tuesday)
	require.NoError(t, err)
	assert.Len(t, d.Plan, 3, "blank text did not add anything")
}

func TestStore_MakeSubtask(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	pidA, err := s.AddProject("Project A")
	require.NoError(t, err)
	pidB, err := s.AddProject("Project B")
	require.NoError(t, err)
	require.NoError(t, s.AddTaggedTask(tuesday, "Task A", pidA))
	require.NoError(t, s.AddTaggedTask(tuesday, "Task B", pidB))

	require.NoError(t, s.MakeSubtask(tuesday, 1, 0)) // Task B becomes a subtask of Task A

	d, _, err := s.LoadDaily(tuesday)
	require.NoError(t, err)
	assert.Equal(t, []Item{
		{Text: "Task A @" + pidA, State: StateTodo, Depth: 0},
		{Text: "Task B", State: StateTodo, Depth: 1}, // own project tag stripped
	}, d.Plan)
}

func TestStore_MakeSubtaskMovesWholeSubtreeAndShiftsDepth(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	pidA, err := s.AddProject("Project A")
	require.NoError(t, err)
	pidB, err := s.AddProject("Project B")
	require.NoError(t, err)
	require.NoError(t, s.AddTaggedTask(tuesday, "Task A", pidA)) // index 0
	require.NoError(t, s.AddTaggedTask(tuesday, "Task B", pidB)) // index 1
	require.NoError(t, s.AddSubtask(tuesday, 0, "S1"))           // index 1 (Task B pushed to 2)
	require.NoError(t, s.AddSubtask(tuesday, 1, "S2"))           // index 2 (Task B pushed to 3)

	d, _, err := s.LoadDaily(tuesday)
	require.NoError(t, err)
	require.Len(t, d.Plan, 4)
	require.Equal(t, "Task B @"+pidB, d.Plan[3].Text, "sanity: Task B sits last, untouched so far")

	// Make Task A (whose subtree is itself + S1 + S2) a subtask of Task B.
	require.NoError(t, s.MakeSubtask(tuesday, 0, 3))

	d, _, err = s.LoadDaily(tuesday)
	require.NoError(t, err)
	assert.Equal(t, []Item{
		{Text: "Task B @" + pidB, State: StateTodo, Depth: 0},
		{Text: "Task A", State: StateTodo, Depth: 1}, // tag stripped, whole subtree shifted +1
		{Text: "S1", State: StateTodo, Depth: 2},
		{Text: "S2", State: StateTodo, Depth: 3},
	}, d.Plan)
}

func TestStore_MakeSubtaskRejectsCycles(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	pid, err := s.AddProject("Ship v2")
	require.NoError(t, err)
	require.NoError(t, s.AddTaggedTask(tuesday, "Task A", pid))
	require.NoError(t, s.AddSubtask(tuesday, 0, "S1")) // index 1, child of Task A

	require.ErrorContains(t, s.MakeSubtask(tuesday, 0, 0), "itself")
	require.ErrorContains(t, s.MakeSubtask(tuesday, 0, 1), "descendant", "S1 is inside Task A's own subtree")
	require.ErrorContains(t, s.MakeSubtask(tuesday, 9, 0), "out of range")
	require.ErrorContains(t, s.MakeSubtask(tuesday, 0, 9), "out of range")
}

func TestStore_MoveTaskToProject(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	pidA, err := s.AddProject("Project A")
	require.NoError(t, err)
	pidB, err := s.AddProject("Project B")
	require.NoError(t, err)
	require.NoError(t, s.AddTaggedTask(tuesday, "Task A", pidA)) // index 0
	require.NoError(t, s.AddSubtask(tuesday, 0, "S1"))           // index 1, depth 1

	// Pull the subtask out to the top level, tagged to a different project.
	require.NoError(t, s.MoveTaskToProject(tuesday, 1, pidB))

	d, _, err := s.LoadDaily(tuesday)
	require.NoError(t, err)
	assert.Equal(t, []Item{
		{Text: "Task A @" + pidA, State: StateTodo, Depth: 0}, // untouched
		{Text: "S1 @" + pidB, State: StateTodo, Depth: 0},     // moved to the end, tagged, depth reset
	}, d.Plan)

	// Empty projectID clears the tag (Unfiled) rather than removing the item.
	require.NoError(t, s.MoveTaskToProject(tuesday, 1, ""))
	d, _, err = s.LoadDaily(tuesday)
	require.NoError(t, err)
	assert.Equal(t, "S1", d.Plan[1].Text)

	require.ErrorContains(t, s.MoveTaskToProject(tuesday, 0, "nope"), "not found")
	require.ErrorContains(t, s.MoveTaskToProject(tuesday, 9, pidA), "out of range")
}

// TestStore_BuildProjectTreeNestsSubtasksAndRollsUpDone verifies that a
// subtask (added under a top-level task via AddSubtask) shows up nested under
// its parent in the tree, inherits the parent's project without carrying its
// own tag, and that the parent's Done state rolls up from its children rather
// than its own (never independently checked) checkbox marker.
func TestStore_BuildProjectTreeNestsSubtasksAndRollsUpDone(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	pid, err := s.AddProject("Ship v2")
	require.NoError(t, err)
	require.NoError(t, s.AddTaggedTask(tuesday, "Launch", pid))
	require.NoError(t, s.AddSubtask(tuesday, 0, "Write docs"))
	require.NoError(t, s.AddSubtask(tuesday, 0, "Ship code"))

	tree, err := s.BuildProjectTree(tuesday)
	require.NoError(t, err)
	require.Len(t, tree.Projects, 1)
	require.Len(t, tree.Projects[0].Tasks, 1)
	parent := tree.Projects[0].Tasks[0]
	assert.Equal(t, "Launch", parent.Text)
	require.Len(t, parent.Children, 2)
	assert.Equal(t, "Write docs", parent.Children[0].Text)
	assert.Equal(t, "Ship code", parent.Children[1].Text)
	assert.False(t, parent.Done, "children still open")

	// Complete both children: the parent rolls up to done even though its own
	// checkbox marker (never independently checked) is still todo.
	require.NoError(t, s.SetPlanItemState(tuesday, 1, StateDone))
	require.NoError(t, s.SetPlanItemState(tuesday, 2, StateDone))
	tree, err = s.BuildProjectTree(tuesday)
	require.NoError(t, err)
	parent = tree.Projects[0].Tasks[0]
	assert.True(t, parent.Done, "all children done rolls the parent up to done")

	// Reopening one child reopens the parent.
	require.NoError(t, s.SetPlanItemState(tuesday, 1, StateTodo))
	tree, err = s.BuildProjectTree(tuesday)
	require.NoError(t, err)
	assert.False(t, tree.Projects[0].Tasks[0].Done, "an open child reopens the parent")
}
