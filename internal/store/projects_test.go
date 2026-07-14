package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStore_ProjectsMissing(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	projects, err := s.LoadProjects()
	require.NoError(t, err)
	assert.Empty(t, projects)
}

func TestStore_ProjectsCRUD(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)

	pid, err := s.AddProject("Ship v2")
	require.NoError(t, err)
	assert.Equal(t, "ship-v2", pid)

	// A second project with a colliding slug base gets a unique suffix.
	pid2, err := s.AddProject("Ship v2")
	require.NoError(t, err)
	assert.Equal(t, "ship-v2-2", pid2, "project slug collides with the first and is suffixed")

	require.ErrorContains(t, func() error { _, e := s.AddProject("  "); return e }(), "must not be empty")

	projects, err := s.LoadProjects()
	require.NoError(t, err)
	require.Len(t, projects, 2)
	assert.Equal(t, []string{"ship-v2", "ship-v2-2"}, []string{projects[0].ID, projects[1].ID})
}

func TestStore_ProjectsRenameKeepsID(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	pid, err := s.AddProject("Ship v2")
	require.NoError(t, err)

	require.NoError(t, s.RenameProject(pid, "Ship version 2"))

	projects, err := s.LoadProjects()
	require.NoError(t, err)
	require.Len(t, projects, 1)
	assert.Equal(t, "ship-v2", projects[0].ID, "rename keeps the stable id")
	assert.Equal(t, "Ship version 2", projects[0].Name)
}

func TestStore_ProjectsCloseReopen(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	pid, err := s.AddProject("Ship v2")
	require.NoError(t, err)

	require.NoError(t, s.SetProjectStatus(pid, StatusClosed))
	projects, err := s.LoadProjects()
	require.NoError(t, err)
	assert.Equal(t, StatusClosed, projects[0].Status)

	require.NoError(t, s.SetProjectStatus(pid, StatusOpen))
	projects, err = s.LoadProjects()
	require.NoError(t, err)
	assert.Equal(t, StatusOpen, projects[0].Status)

	require.ErrorContains(t, s.RenameProject("nope", "x"), "not found")
	require.ErrorContains(t, s.SetProjectStatus("nope", StatusOpen), "not found")
}

func TestStore_ProjectsRoundTrip(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	_, err := s.AddProject("Ship v2")
	require.NoError(t, err)
	_, err = s.AddProject("Internal tooling")
	require.NoError(t, err)

	want, err := s.LoadProjects()
	require.NoError(t, err)
	// Re-render and re-parse: identical structure.
	got, err := parseProjects(renderProjects(want))
	require.NoError(t, err)
	require.NoError(t, assignMissingIDs(got))
	assert.Equal(t, want, got)
}

func TestStore_ProjectsHandEditedMissingIDs(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	// A hand-written file with no id line: the slug is derived on load.
	require.NoError(t, writeFile(s.ProjectsPath(), "# Projects\n\n## Ship v2\n"))
	projects, err := s.LoadProjects()
	require.NoError(t, err)
	require.Len(t, projects, 1)
	assert.Equal(t, "ship-v2", projects[0].ID)
	assert.Equal(t, StatusOpen, projects[0].Status)
}

func TestSplitProjectTag(t *testing.T) {
	t.Parallel()
	known := map[string]bool{"payments": true}
	cases := []struct {
		text, clean, slug string
	}{
		// Canonical # prefix.
		{"Fix payment bug #payments", "Fix payment bug", "payments"},
		{"#payments", "", "payments"},
		{"Fix bug #payments  ", "Fix bug", "payments"}, // trailing space tolerated
		// Legacy @ prefix (backward compat).
		{"Fix payment bug @payments", "Fix payment bug", "payments"},
		{"@payments", "", "payments"},
		// Non-project tokens must be left untouched.
		{"ping @alice about it", "ping @alice about it", ""}, // not trailing
		{"ping @alice", "ping @alice", ""},                   // unknown slug
		{"ping #unknown", "ping #unknown", ""},               // unknown slug with #
		{"no tag here", "no tag here", ""},
	}
	for _, c := range cases {
		clean, slug := splitProjectTag(c.text, known)
		assert.Equal(t, c.clean, clean, "clean of %q", c.text)
		assert.Equal(t, c.slug, slug, "slug of %q", c.text)
	}
}

func TestStore_AssignAndUnassignTaskProject(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	pid, err := s.AddProject("Payments")
	require.NoError(t, err)
	require.NoError(t, s.ApplyMorning(tuesday, []string{"task a"}, nil))

	require.NoError(t, s.AssignTaskProject(tuesday, 0, pid))
	d, _, err := s.LoadDaily(tuesday)
	require.NoError(t, err)
	assert.Equal(t, "task a #payments", d.Plan[0].Text)

	// Reassigning replaces the tag rather than stacking a second one.
	require.NoError(t, s.AssignTaskProject(tuesday, 0, pid))
	d, _, err = s.LoadDaily(tuesday)
	require.NoError(t, err)
	assert.Equal(t, "task a #payments", d.Plan[0].Text)

	require.NoError(t, s.UnassignTaskProject(tuesday, 0))
	d, _, err = s.LoadDaily(tuesday)
	require.NoError(t, err)
	assert.Equal(t, "task a", d.Plan[0].Text)

	require.ErrorContains(t, s.AssignTaskProject(tuesday, 0, "nope"), "not found")
	require.ErrorContains(t, s.AssignTaskProject(tuesday, 9, pid), "out of range")
}

func TestStore_AddTaggedTask(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	pid, err := s.AddProject("Payments")
	require.NoError(t, err)

	require.NoError(t, s.AddTaggedTask(tuesday, "wire refunds", pid))
	d, _, err := s.LoadDaily(tuesday)
	require.NoError(t, err)
	require.Len(t, d.Plan, 1)
	assert.Equal(t, "wire refunds #payments", d.Plan[0].Text)
	assert.Equal(t, 0, d.Plan[0].Depth)

	// Duplicate (already-tagged) text is a no-op, not a second entry.
	require.NoError(t, s.AddTaggedTask(tuesday, "wire refunds", pid))
	d, _, err = s.LoadDaily(tuesday)
	require.NoError(t, err)
	assert.Len(t, d.Plan, 1)

	require.ErrorContains(t, s.AddTaggedTask(tuesday, "x", "nope"), "not found")
}

func TestStore_BuildProjectTree(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	pid, err := s.AddProject("Ship v2")
	require.NoError(t, err)

	// Two tagged tasks under the project plus one untagged task, on Tuesday.
	require.NoError(t, s.ApplyMorning(tuesday, []string{"wire refunds", "add receipts", "standalone chore"}, nil))
	require.NoError(t, s.AssignTaskProject(tuesday, 0, pid))
	require.NoError(t, s.AssignTaskProject(tuesday, 1, pid))

	tree, err := s.BuildProjectTree(tuesday)
	require.NoError(t, err)
	require.Len(t, tree.Projects, 1)
	proj := tree.Projects[0]
	assert.Equal(t, []string{"wire refunds", "add receipts"},
		[]string{proj.Tasks[0].Text, proj.Tasks[1].Text}, "tags stripped for display")
	assert.False(t, proj.Done, "open tasks remain")
	require.Len(t, tree.Unfiled, 1)
	assert.Equal(t, "standalone chore", tree.Unfiled[0].Text)

	// A different day does not show Tuesday's tasks.
	tree, err = s.BuildProjectTree(monday)
	require.NoError(t, err)
	assert.Empty(t, tree.Projects[0].Tasks, "other days do not show this day's tasks")

	// Complete both tagged tasks -> globally done; Tuesday still lists them
	// (struck through), and the project reads as done too.
	require.NoError(t, s.SetPlanItemState(tuesday, 0, StateDone))
	require.NoError(t, s.SetPlanItemState(tuesday, 1, StateDone))
	tree, err = s.BuildProjectTree(tuesday)
	require.NoError(t, err)
	proj = tree.Projects[0]
	assert.True(t, proj.Done, "no open tasks on any day")
	require.Len(t, proj.Tasks, 2, "done tasks stay visible")
	assert.True(t, proj.Tasks[0].Done)
	assert.True(t, tree.Projects[0].Done)

	// An open task on Monday reopens the project globally (seen when viewing Monday).
	require.NoError(t, s.AddPlanItem(monday, "hotfix refund rounding"))
	require.NoError(t, s.AssignTaskProject(monday, 0, pid))
	tree, err = s.BuildProjectTree(tuesday)
	require.NoError(t, err)
	assert.False(t, tree.Projects[0].Done, "an open task on another day reopens the project")
	tree, err = s.BuildProjectTree(monday)
	require.NoError(t, err)
	require.Len(t, tree.Projects[0].Tasks, 1)
	assert.Equal(t, "hotfix refund rounding", tree.Projects[0].Tasks[0].Text)
}

func TestStore_BuildProjectTreeDedupsCarryover(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	pid, err := s.AddProject("Ship v2")
	require.NoError(t, err)

	// The same task, open on Monday and carried over (still open) to Tuesday.
	require.NoError(t, s.AddPlanItem(monday, "wire refunds"))
	require.NoError(t, s.AssignTaskProject(monday, 0, pid))
	require.NoError(t, s.AddPlanItem(tuesday, "wire refunds"))
	require.NoError(t, s.AssignTaskProject(tuesday, 0, pid))

	tree, err := s.BuildProjectTree(tuesday)
	require.NoError(t, err)
	proj := tree.Projects[0]
	require.Len(t, proj.Tasks, 1, "Tuesday's copy shown")
	assert.False(t, proj.Done)

	// Marking the latest (Tuesday) done makes the project globally done even
	// though the Monday copy is still todo in its own file (dedup keeps latest).
	require.NoError(t, s.SetPlanItemState(tuesday, 0, StateDone))
	tree, err = s.BuildProjectTree(tuesday)
	require.NoError(t, err)
	proj = tree.Projects[0]
	assert.True(t, proj.Done)
	require.Len(t, proj.Tasks, 1, "the done task stays visible")
	assert.True(t, proj.Tasks[0].Done)
}

func TestStore_BuildProjectTreeExcludesClosed(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	pid, err := s.AddProject("Ship v2")
	require.NoError(t, err)
	require.NoError(t, s.ApplyMorning(tuesday, []string{"task"}, nil))
	require.NoError(t, s.AssignTaskProject(tuesday, 0, pid))

	require.NoError(t, s.SetProjectStatus(pid, StatusClosed))
	tree, err := s.BuildProjectTree(tuesday)
	require.NoError(t, err)
	assert.Empty(t, tree.Projects, "closed project hidden")
}

func TestSlugify(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"Ship v2":           "ship-v2",
		"  Payments flow  ": "payments-flow",
		"CI/CD & Deploys!":  "ci-cd-deploys",
		"---":               "item",
		"Über Café":         "ber-caf",
	}
	for in, want := range cases {
		assert.Equal(t, want, slugify(in), "slugify(%q)", in)
	}
}
