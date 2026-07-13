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

	sid, err := s.AddStory(pid, "Payments flow")
	require.NoError(t, err)
	assert.Equal(t, "payments-flow", sid)

	// Second story with a colliding slug base gets a unique suffix.
	sid2, err := s.AddStory(pid, "Payments flow")
	require.NoError(t, err)
	assert.Equal(t, "payments-flow-2", sid2)

	// A story under a second project shares the ID namespace with everything.
	pid2, err := s.AddProject("Ship v2")
	require.NoError(t, err)
	assert.Equal(t, "ship-v2-2", pid2, "project slug collides with the first and is suffixed")

	require.ErrorContains(t, func() error { _, e := s.AddStory("nope", "x"); return e }(), "not found")
	require.ErrorContains(t, func() error { _, e := s.AddProject("  "); return e }(), "must not be empty")

	projects, err := s.LoadProjects()
	require.NoError(t, err)
	require.Len(t, projects, 2)
	assert.Equal(t, []string{"payments-flow", "payments-flow-2"},
		[]string{projects[0].Stories[0].ID, projects[0].Stories[1].ID})
}

func TestStore_ProjectsRenameKeepsID(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	pid, err := s.AddProject("Ship v2")
	require.NoError(t, err)
	sid, err := s.AddStory(pid, "Payments")
	require.NoError(t, err)

	require.NoError(t, s.RenameProject(pid, "Ship version 2"))
	require.NoError(t, s.RenameStory(sid, "Payment flows"))

	projects, err := s.LoadProjects()
	require.NoError(t, err)
	require.Len(t, projects, 1)
	assert.Equal(t, "ship-v2", projects[0].ID, "rename keeps the stable id")
	assert.Equal(t, "Ship version 2", projects[0].Name)
	require.Len(t, projects[0].Stories, 1)
	assert.Equal(t, "payments", projects[0].Stories[0].ID)
	assert.Equal(t, "Payment flows", projects[0].Stories[0].Name)
}

func TestStore_ProjectsCloseReopen(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	pid, err := s.AddProject("Ship v2")
	require.NoError(t, err)
	sid, err := s.AddStory(pid, "Payments")
	require.NoError(t, err)

	require.NoError(t, s.SetStoryStatus(sid, StatusClosed))
	require.NoError(t, s.SetProjectStatus(pid, StatusClosed))
	projects, err := s.LoadProjects()
	require.NoError(t, err)
	assert.Equal(t, StatusClosed, projects[0].Status)
	assert.Equal(t, StatusClosed, projects[0].Stories[0].Status)

	require.NoError(t, s.SetProjectStatus(pid, StatusOpen))
	projects, err = s.LoadProjects()
	require.NoError(t, err)
	assert.Equal(t, StatusOpen, projects[0].Status)

	require.ErrorContains(t, s.RenameProject("nope", "x"), "not found")
	require.ErrorContains(t, s.SetStoryStatus("nope", StatusOpen), "not found")
}

func TestStore_ProjectsRoundTrip(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	pid, err := s.AddProject("Ship v2")
	require.NoError(t, err)
	_, err = s.AddStory(pid, "Payments")
	require.NoError(t, err)
	pid2, err := s.AddProject("Internal tooling")
	require.NoError(t, err)
	_, err = s.AddStory(pid2, "CI speedups")
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
	// A hand-written file with no id lines: slugs are derived on load.
	require.NoError(t, writeFile(s.ProjectsPath(),
		"# Projects\n\n## Ship v2\n\n### Payments\n"))
	projects, err := s.LoadProjects()
	require.NoError(t, err)
	require.Len(t, projects, 1)
	assert.Equal(t, "ship-v2", projects[0].ID)
	require.Len(t, projects[0].Stories, 1)
	assert.Equal(t, "payments", projects[0].Stories[0].ID)
	assert.Equal(t, StatusOpen, projects[0].Stories[0].Status)
}

func TestSplitStoryTag(t *testing.T) {
	t.Parallel()
	known := map[string]bool{"payments": true}
	cases := []struct {
		text, clean, slug string
	}{
		// Canonical # prefix.
		{"Fix payment bug #payments", "Fix payment bug", "payments"},
		{"#payments", "", "payments"},
		{"Fix bug #payments  ", "Fix bug", "payments"}, // trailing space tolerated
		{"#unknown", "#unknown", ""},                   // unknown id: left as plain text
		// Legacy @ prefix (backward compat for files before migration).
		{"Fix payment bug @payments", "Fix payment bug", "payments"},
		{"@payments", "", "payments"},
		{"Fix bug @payments  ", "Fix bug", "payments"},
		// Non-ref tokens: unknown body means it stays as plain text.
		{"ping @alice about it", "ping @alice about it", ""}, // not trailing
		{"ping @alice", "ping @alice", ""},                   // unknown slug
		{"no tag here", "no tag here", ""},
	}
	for _, c := range cases {
		clean, slug := splitStoryTag(c.text, known)
		assert.Equal(t, c.clean, clean, "clean of %q", c.text)
		assert.Equal(t, c.slug, slug, "slug of %q", c.text)
	}
}

func TestStore_AssignAndUnassignTaskStory(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	pid, err := s.AddProject("Ship v2")
	require.NoError(t, err)
	sid, err := s.AddStory(pid, "Payments")
	require.NoError(t, err)
	require.NoError(t, s.ApplyMorning(tuesday, []string{"task a"}, nil))

	require.NoError(t, s.AssignTaskStory(tuesday, 0, sid))
	d, _, err := s.LoadDaily(tuesday)
	require.NoError(t, err)
	assert.Equal(t, "task a #payments", d.Plan[0].Text)

	// Reassigning replaces the tag rather than stacking a second one.
	require.NoError(t, s.AssignTaskStory(tuesday, 0, sid))
	d, _, err = s.LoadDaily(tuesday)
	require.NoError(t, err)
	assert.Equal(t, "task a #payments", d.Plan[0].Text)

	require.NoError(t, s.UnassignTaskStory(tuesday, 0))
	d, _, err = s.LoadDaily(tuesday)
	require.NoError(t, err)
	assert.Equal(t, "task a", d.Plan[0].Text)

	require.ErrorContains(t, s.AssignTaskStory(tuesday, 0, "nope"), "not found")
	require.ErrorContains(t, s.AssignTaskStory(tuesday, 9, sid), "out of range")
}

func TestStore_BuildProjectTree(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	pid, err := s.AddProject("Ship v2")
	require.NoError(t, err)
	sid, err := s.AddStory(pid, "Payments")
	require.NoError(t, err)

	// Two tagged tasks under the story plus one untagged task, on Tuesday.
	require.NoError(t, s.ApplyMorning(tuesday, []string{"wire refunds", "add receipts", "standalone chore"}, nil))
	require.NoError(t, s.AssignTaskStory(tuesday, 0, sid))
	require.NoError(t, s.AssignTaskStory(tuesday, 1, sid))

	tree, err := s.BuildProjectTree(tuesday)
	require.NoError(t, err)
	require.Len(t, tree.Projects, 1)
	require.Len(t, tree.Projects[0].Stories, 1)
	story := tree.Projects[0].Stories[0]
	assert.Equal(t, []string{"wire refunds", "add receipts"},
		[]string{story.Tasks[0].Text, story.Tasks[1].Text}, "tags stripped for display")
	assert.False(t, story.Done, "open tasks remain")
	require.Len(t, tree.Unfiled, 1)
	assert.Equal(t, "standalone chore", tree.Unfiled[0].Text)

	// A different day does not show Tuesday's tasks.
	tree, err = s.BuildProjectTree(monday)
	require.NoError(t, err)
	assert.Empty(t, tree.Projects[0].Stories[0].Tasks, "other days do not show this day's tasks")

	// Complete both tagged tasks -> globally done; Tuesday still lists them
	// (struck through), and the project reads as done too.
	require.NoError(t, s.SetPlanItemState(tuesday, 0, StateDone))
	require.NoError(t, s.SetPlanItemState(tuesday, 1, StateDone))
	tree, err = s.BuildProjectTree(tuesday)
	require.NoError(t, err)
	story = tree.Projects[0].Stories[0]
	assert.True(t, story.Done, "no open tasks on any day")
	require.Len(t, story.Tasks, 2, "done tasks stay visible")
	assert.Equal(t, StateDone, story.Tasks[0].State)
	assert.True(t, tree.Projects[0].Done)

	// An open task on Monday reopens the story globally (seen when viewing Monday).
	require.NoError(t, s.AddPlanItem(monday, "hotfix refund rounding"))
	require.NoError(t, s.AssignTaskStory(monday, 0, sid))
	tree, err = s.BuildProjectTree(tuesday)
	require.NoError(t, err)
	assert.False(t, tree.Projects[0].Stories[0].Done, "an open task on another day reopens the story")
	tree, err = s.BuildProjectTree(monday)
	require.NoError(t, err)
	require.Len(t, tree.Projects[0].Stories[0].Tasks, 1)
	assert.Equal(t, "hotfix refund rounding", tree.Projects[0].Stories[0].Tasks[0].Text)
}

func TestStore_BuildProjectTreeDedupsCarryover(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	pid, err := s.AddProject("Ship v2")
	require.NoError(t, err)
	sid, err := s.AddStory(pid, "Payments")
	require.NoError(t, err)

	// The same task, open on Monday and carried over (still open) to Tuesday.
	require.NoError(t, s.AddPlanItem(monday, "wire refunds"))
	require.NoError(t, s.AssignTaskStory(monday, 0, sid))
	require.NoError(t, s.AddPlanItem(tuesday, "wire refunds"))
	require.NoError(t, s.AssignTaskStory(tuesday, 0, sid))

	tree, err := s.BuildProjectTree(tuesday)
	require.NoError(t, err)
	story := tree.Projects[0].Stories[0]
	require.Len(t, story.Tasks, 1, "Tuesday's copy shown")
	assert.False(t, story.Done)

	// Marking the latest (Tuesday) done makes the story globally done even though
	// the Monday copy is still todo in its own file (dedup keeps the latest).
	require.NoError(t, s.SetPlanItemState(tuesday, 0, StateDone))
	tree, err = s.BuildProjectTree(tuesday)
	require.NoError(t, err)
	story = tree.Projects[0].Stories[0]
	assert.True(t, story.Done)
	require.Len(t, story.Tasks, 1, "the done task stays visible")
	assert.Equal(t, StateDone, story.Tasks[0].State)
}

func TestStore_MoveStory(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	p1, err := s.AddProject("Ship v2")
	require.NoError(t, err)
	p2, err := s.AddProject("Internal tooling")
	require.NoError(t, err)
	sid, err := s.AddStory(p1, "Payments")
	require.NoError(t, err)

	// A task tagged to the story keeps its tag (the story ID is stable).
	require.NoError(t, s.ApplyMorning(tuesday, []string{"wire refunds"}, nil))
	require.NoError(t, s.AssignTaskStory(tuesday, 0, sid))

	require.NoError(t, s.MoveStory(sid, p2))
	projects, err := s.LoadProjects()
	require.NoError(t, err)
	assert.Empty(t, projects[0].Stories, "story left the source project")
	require.Len(t, projects[1].Stories, 1)
	assert.Equal(t, sid, projects[1].Stories[0].ID)

	// The tagged task now aggregates under the new project.
	tree, err := s.BuildProjectTree(tuesday)
	require.NoError(t, err)
	require.Len(t, tree.Projects, 2)
	assert.Empty(t, tree.Projects[0].Stories)
	require.Len(t, tree.Projects[1].Stories, 1)
	require.Len(t, tree.Projects[1].Stories[0].Tasks, 1)
	assert.Equal(t, "wire refunds", tree.Projects[1].Stories[0].Tasks[0].Text)

	require.ErrorContains(t, s.MoveStory("nope", p2), "not found")
	require.ErrorContains(t, s.MoveStory(sid, "nope"), "not found")
}

func TestStore_BuildProjectTreeExcludesClosed(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	pid, err := s.AddProject("Ship v2")
	require.NoError(t, err)
	sid, err := s.AddStory(pid, "Payments")
	require.NoError(t, err)
	require.NoError(t, s.ApplyMorning(tuesday, []string{"task"}, nil))
	require.NoError(t, s.AssignTaskStory(tuesday, 0, sid))

	require.NoError(t, s.SetStoryStatus(sid, StatusClosed))
	tree, err := s.BuildProjectTree(tuesday)
	require.NoError(t, err)
	require.Len(t, tree.Projects, 1)
	assert.Empty(t, tree.Projects[0].Stories, "closed story hidden")

	require.NoError(t, s.SetProjectStatus(pid, StatusClosed))
	tree, err = s.BuildProjectTree(tuesday)
	require.NoError(t, err)
	assert.Empty(t, tree.Projects, "closed project hidden")
}

// TestStore_BuildProjectTreeProjectLevelTasks covers the core bug: tasks tagged
// with a project ID (not a story ID) must appear in TreeProject.Tasks, not
// disappear silently.
func TestStore_BuildProjectTreeProjectLevelTasks(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)

	// A story-less project (the common case that triggered the bug).
	pid, err := s.AddProject("Marketing")
	require.NoError(t, err)
	assert.Equal(t, "marketing", pid)

	// Three tasks on Tuesday: two tagged with the project, one untagged.
	require.NoError(t, s.ApplyMorning(tuesday, []string{"DMs", "LinkedIn post", "standalone"}, nil))
	require.NoError(t, s.AssignTaskProject(tuesday, 0, pid))
	require.NoError(t, s.AssignTaskProject(tuesday, 1, pid))

	tree, err := s.BuildProjectTree(tuesday)
	require.NoError(t, err)
	require.Len(t, tree.Projects, 1)
	p := tree.Projects[0]
	assert.Empty(t, p.Stories, "story-less project has no stories")
	require.Len(t, p.Tasks, 2, "project-tagged tasks must appear in TreeProject.Tasks")
	assert.Equal(t, "DMs", p.Tasks[0].Text)
	assert.Equal(t, "LinkedIn post", p.Tasks[1].Text)
	assert.False(t, p.Done, "open project tasks keep project open")
	require.Len(t, tree.Unfiled, 1)
	assert.Equal(t, "standalone", tree.Unfiled[0].Text, "untagged task in Unfiled")

	// A different day shows no tasks under the project.
	tree, err = s.BuildProjectTree(monday)
	require.NoError(t, err)
	assert.Empty(t, tree.Projects[0].Tasks, "other day shows no tasks")

	// Completing all project tasks marks the project globally done.
	require.NoError(t, s.SetPlanItemState(tuesday, 0, StateDone))
	require.NoError(t, s.SetPlanItemState(tuesday, 1, StateDone))
	tree, err = s.BuildProjectTree(tuesday)
	require.NoError(t, err)
	p = tree.Projects[0]
	assert.True(t, p.Done, "no open tasks on any day -> project done")
	require.Len(t, p.Tasks, 2, "done tasks remain visible")
	assert.Equal(t, StateDone, p.Tasks[0].State)
}

// TestStore_BuildProjectTreeClosedProjectTagFallback verifies the fail-visible
// rule: a task tagged with a CLOSED project ID must fall back to Unfiled
// rather than disappearing from the tree.
func TestStore_BuildProjectTreeClosedProjectTagFallback(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)

	pid, err := s.AddProject("Archive")
	require.NoError(t, err)
	require.NoError(t, s.ApplyMorning(tuesday, []string{"old task"}, nil))
	require.NoError(t, s.AssignTaskProject(tuesday, 0, pid))

	// Close the project. The task was tagged before closing; it must not vanish.
	require.NoError(t, s.SetProjectStatus(pid, StatusClosed))

	tree, err := s.BuildProjectTree(tuesday)
	require.NoError(t, err)
	assert.Empty(t, tree.Projects, "closed project hidden from tree")
	require.Len(t, tree.Unfiled, 1, "task falls back to Unfiled when its project is closed")
	assert.Equal(t, "old task", tree.Unfiled[0].Text)
}

// TestStore_BuildProjectTreeClosedStoryTagFallback mirrors the above for a
// closed story within an open project.
func TestStore_BuildProjectTreeClosedStoryTagFallback(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)

	pid, err := s.AddProject("Ship v2")
	require.NoError(t, err)
	sid, err := s.AddStory(pid, "Payments")
	require.NoError(t, err)
	require.NoError(t, s.ApplyMorning(tuesday, []string{"fix charge"}, nil))
	require.NoError(t, s.AssignTaskStory(tuesday, 0, sid))

	require.NoError(t, s.SetStoryStatus(sid, StatusClosed))

	tree, err := s.BuildProjectTree(tuesday)
	require.NoError(t, err)
	require.Len(t, tree.Projects, 1)
	assert.Empty(t, tree.Projects[0].Stories, "closed story hidden")
	require.Len(t, tree.Unfiled, 1, "task falls back to Unfiled when its story is closed")
	assert.Equal(t, "fix charge", tree.Unfiled[0].Text)
}

// TestStore_BuildProjectTreeMixedProjectAndStoryTasks verifies that project-
// level tasks and story-level tasks coexist correctly within the same project.
func TestStore_BuildProjectTreeMixedProjectAndStoryTasks(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)

	pid, err := s.AddProject("Ship v2")
	require.NoError(t, err)
	sid, err := s.AddStory(pid, "Payments")
	require.NoError(t, err)

	// One task tagged with the project, one with the story.
	require.NoError(t, s.ApplyMorning(tuesday, []string{"planning call", "wire refunds"}, nil))
	require.NoError(t, s.AssignTaskProject(tuesday, 0, pid))
	require.NoError(t, s.AssignTaskStory(tuesday, 1, sid))

	tree, err := s.BuildProjectTree(tuesday)
	require.NoError(t, err)
	require.Len(t, tree.Projects, 1)
	p := tree.Projects[0]
	require.Len(t, p.Tasks, 1, "project-level task appears in TreeProject.Tasks")
	assert.Equal(t, "planning call", p.Tasks[0].Text)
	require.Len(t, p.Stories, 1, "story still rendered")
	require.Len(t, p.Stories[0].Tasks, 1, "story-level task appears under story")
	assert.Equal(t, "wire refunds", p.Stories[0].Tasks[0].Text)
	assert.Empty(t, tree.Unfiled, "no task leaked to Unfiled")
}

// TestStore_AssignTaskProject verifies the store method that re-tags a plan
// item with a project ID.
func TestStore_AssignTaskProject(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	pid, err := s.AddProject("Ship v2")
	require.NoError(t, err)
	require.NoError(t, s.ApplyMorning(tuesday, []string{"task a"}, nil))

	require.NoError(t, s.AssignTaskProject(tuesday, 0, pid))
	d, _, err := s.LoadDaily(tuesday)
	require.NoError(t, err)
	assert.Equal(t, "task a #ship-v2", d.Plan[0].Text)

	// Reassigning replaces the tag.
	require.NoError(t, s.AssignTaskProject(tuesday, 0, pid))
	d, _, err = s.LoadDaily(tuesday)
	require.NoError(t, err)
	assert.Equal(t, "task a #ship-v2", d.Plan[0].Text)

	require.ErrorContains(t, s.AssignTaskProject(tuesday, 0, "nope"), "not found")
	require.ErrorContains(t, s.AssignTaskProject(tuesday, 9, pid), "out of range")
}

// TestStore_AddTaggedTaskEmitsHash verifies that AddTaggedTask writes the
// canonical "#<storyID>" suffix rather than the legacy "@<storyID>" form.
func TestStore_AddTaggedTaskEmitsHash(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	pid, err := s.AddProject("Ship v2")
	require.NoError(t, err)
	sid, err := s.AddStory(pid, "Payments")
	require.NoError(t, err)

	require.NoError(t, s.AddTaggedTask(tuesday, "wire refunds", sid))
	d, _, err := s.LoadDaily(tuesday)
	require.NoError(t, err)
	require.Len(t, d.Plan, 1)
	assert.Equal(t, "wire refunds #payments", d.Plan[0].Text, "AddTaggedTask must use # prefix")

	// Dedup: adding the same text again is a no-op.
	require.NoError(t, s.AddTaggedTask(tuesday, "wire refunds", sid))
	d, _, err = s.LoadDaily(tuesday)
	require.NoError(t, err)
	assert.Len(t, d.Plan, 1, "duplicate must not be added")
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
