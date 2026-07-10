package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStore_DeleteRestoreAndPurge(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	pid, err := s.AddProject("Ship v2")
	require.NoError(t, err)
	sid, err := s.AddStory(pid, "Payments")
	require.NoError(t, err)
	require.NoError(t, s.ApplyMorning(tuesday, []string{"wire refunds", "add receipts"}, nil))
	require.NoError(t, s.AssignTaskStory(tuesday, 0, sid))
	require.NoError(t, s.SetPlanItemState(tuesday, 0, StateDone))

	// Delete the (done, tagged) task -> it leaves the day and enters the bin.
	require.NoError(t, s.DeleteTask(tuesday, 0))
	d, _, err := s.LoadDaily(tuesday)
	require.NoError(t, err)
	assert.Equal(t, []Item{{Text: "add receipts", State: StateTodo}}, d.Plan)

	bin, err := s.LoadRecycleBin()
	require.NoError(t, err)
	require.Len(t, bin, 1)
	assert.Equal(t, "wire refunds @payments", bin[0].Item.Text, "tag and state preserved")
	assert.Equal(t, StateDone, bin[0].Item.State)

	// It appears in the tree's Recycled list (tag stripped for display).
	tree, err := s.BuildProjectTree(tuesday)
	require.NoError(t, err)
	require.Len(t, tree.Recycled, 1)
	assert.Equal(t, "wire refunds", tree.Recycled[0].Text)

	// Restore by display text -> back on its day with its state + tag, bin empty.
	require.NoError(t, s.RestoreTask(tuesday, "wire refunds"))
	d, _, err = s.LoadDaily(tuesday)
	require.NoError(t, err)
	require.Len(t, d.Plan, 2)
	assert.Equal(t, "wire refunds @payments", d.Plan[1].Text)
	assert.Equal(t, StateDone, d.Plan[1].State)
	bin, err = s.LoadRecycleBin()
	require.NoError(t, err)
	assert.Empty(t, bin)

	// Delete again, then purge permanently -> gone from both day and bin.
	require.NoError(t, s.DeleteTask(tuesday, 1))
	require.NoError(t, s.PurgeRecycled(tuesday, "wire refunds"))
	bin, err = s.LoadRecycleBin()
	require.NoError(t, err)
	assert.Empty(t, bin)
	d, _, err = s.LoadDaily(tuesday)
	require.NoError(t, err)
	assert.Equal(t, []Item{{Text: "add receipts", State: StateTodo}}, d.Plan)

	require.ErrorContains(t, s.DeleteTask(tuesday, 9), "out of range")
}

func TestStore_RecycleRoundTrip(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	require.NoError(t, s.ApplyMorning(monday, []string{"a"}, nil))
	require.NoError(t, s.ApplyMorning(tuesday, []string{"b"}, nil))
	require.NoError(t, s.DeleteTask(monday, 0))
	require.NoError(t, s.DeleteTask(tuesday, 0))

	want, err := s.LoadRecycleBin()
	require.NoError(t, err)
	got, err := parseRecycle(renderRecycle(want))
	require.NoError(t, err)
	assert.Equal(t, want, got)
}
