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
