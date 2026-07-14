package store

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStore_MigratesLegacyStoriesToProjects seeds an old-format projects.md
// (a project with a nested story) plus a daily file whose plan item is tagged
// with that story's ID, then constructs a Store over the directory (which
// runs the one-time migration) and asserts: a backup of the pre-migration
// state exists, projects.md is rewritten story-free, and the daily item's tag
// became the parent project's ID.
func TestStore_MigratesLegacyStoriesToProjects(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	const legacyProjectsMD = "# Projects\n" +
		"\n## Ship v2\nid: ship-v2\nstatus: open\n" +
		"\n### Payments flow\nid: payments-flow\nstatus: open\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "projects.md"), []byte(legacyProjectsMD), 0o600))

	dailyPath := filepath.Join(dir, "daily", "2026", "07", "2026-07-07.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(dailyPath), 0o750))
	const legacyDaily = "---\n" +
		"date: 2026-07-07\n" +
		"morning_done: true\n" +
		"evening_done: false\n" +
		"---\n\n" +
		"## Plan\n\n" +
		"- [ ] wire refunds @payments-flow\n" +
		"- [x] untagged chore\n" +
		"\n## Done\n"
	require.NoError(t, os.WriteFile(dailyPath, []byte(legacyDaily), 0o600))

	s, err := New(dir)
	require.NoError(t, err)

	// The backup exists and holds the pre-migration files, untouched.
	backupProjects := filepath.Join(dir, backupDirName, "projects.md")
	backupDaily := filepath.Join(dir, backupDirName, "daily", "2026", "07", "2026-07-07.md")
	require.FileExists(t, backupProjects)
	require.FileExists(t, backupDaily)
	gotBackupProjects, err := os.ReadFile(backupProjects)
	require.NoError(t, err)
	assert.Equal(t, legacyProjectsMD, string(gotBackupProjects), "backup preserves the exact pre-migration content")
	gotBackupDaily, err := os.ReadFile(backupDaily)
	require.NoError(t, err)
	assert.Equal(t, legacyDaily, string(gotBackupDaily))

	// projects.md is now story-free and loads with the new (project-only) parser.
	rewritten, err := os.ReadFile(filepath.Join(dir, "projects.md"))
	require.NoError(t, err)
	assert.NotContains(t, string(rewritten), "### ", "projects.md must no longer contain story headings")
	projects, err := s.LoadProjects()
	require.NoError(t, err)
	require.Len(t, projects, 1)
	assert.Equal(t, "ship-v2", projects[0].ID)
	assert.Equal(t, "Ship v2", projects[0].Name)
	assert.Equal(t, StatusOpen, projects[0].Status)

	// The daily item's story tag became the parent project's tag; text/state
	// and the untagged item are untouched.
	d, exists, err := s.LoadDaily(date("2026-07-07"))
	require.NoError(t, err)
	require.True(t, exists)
	require.Len(t, d.Plan, 2)
	assert.Equal(t, "wire refunds @ship-v2", d.Plan[0].Text)
	assert.Equal(t, StateTodo, d.Plan[0].State)
	assert.Equal(t, "untagged chore", d.Plan[1].Text)
	assert.Equal(t, StateDone, d.Plan[1].State)

	// Idempotent: constructing a fresh Store over the same directory again
	// must not touch anything further (no story headings remain, so the
	// migration is a cheap no-op) and must not overwrite the backup.
	backupModTime, err := os.Stat(backupProjects)
	require.NoError(t, err)
	s2, err := New(dir)
	require.NoError(t, err)
	backupModTime2, err := os.Stat(backupProjects)
	require.NoError(t, err)
	assert.Equal(t, backupModTime.ModTime(), backupModTime2.ModTime(), "backup must never be overwritten")
	projects2, err := s2.LoadProjects()
	require.NoError(t, err)
	assert.Equal(t, projects, projects2)
}

// TestStore_BothMigrationsInSequence exercises the full startup migration
// seam: story->project migration (store.New) followed by @ref->##ref tag
// migration (MigrateRefTags), on a dataset that includes a legacy projects.md
// with stories, a daily file with @story-ref tags, a recurring template with
// an @-style project tag and a recurrence keyword, and a backlog entry.
//
// This is M9's regression test — the seam that hid C1: story->project
// migration writes "@project" tags (legacy form) that MigrateRefTags then
// converts to "#project". If Parse did not scan "#project" tokens, the
// recurring template would silently lose its recurrence after the second
// migration (C1 fix is exercised here end-to-end).
//
// Assertions:
// - projects.md is story-free after New()
// - daily tasks carry "#project" tags after MigrateRefTags
// - RecurringTasks() count is preserved through both migrations
// - running both again is a no-op (idempotent)
func TestStore_BothMigrationsInSequence(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Legacy projects.md: one project with one story.
	const legacyProjects = "# Projects\n" +
		"\n## Ship v2\nid: ship-v2\nstatus: open\n" +
		"\n### Payments flow\nid: payments-flow\nstatus: open\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "projects.md"), []byte(legacyProjects), 0o600))

	// Daily file with a story ref tag in @legacy form.
	dailyPath := filepath.Join(dir, "daily", "2026", "07", "2026-07-07.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(dailyPath), 0o750))
	const legacyDaily = "---\ndate: 2026-07-07\nmorning_done: true\nevening_done: false\n---\n\n" +
		"## Plan\n\n- [ ] wire refunds @payments-flow\n\n## Done\n"
	require.NoError(t, os.WriteFile(dailyPath, []byte(legacyDaily), 0o600))

	// Backlog with a direct project ref (not a story ref — the story->project
	// migration only retags daily files; existing backlog entries keep their old
	// project tag and are then converted by MigrateRefTags).
	backlogPath := filepath.Join(dir, "backlog.md")
	const legacyBacklog = "# Backlog\n\n## Current\n\n- [ ] backlog task @ship-v2\n\n## Next week\n"
	require.NoError(t, os.WriteFile(backlogPath, []byte(legacyBacklog), 0o600))

	// Recurring template: project ref + recurrence keyword in @legacy form.
	// After story->project migration, the ref will be "@ship-v2".
	// After MigrateRefTags it will be "#ship-v2".
	// If Parse did not scan #-prefixed tokens (C1 bug), this template would
	// be dropped from RecurringTasks() after the second migration.
	recurPath := filepath.Join(dir, recurringFileName)
	const legacyRecur = "# Recurring\n\n- [ ] Weekly review @ship-v2 @weekly @fri @9:00\n"
	require.NoError(t, os.WriteFile(recurPath, []byte(legacyRecur), 0o600))

	// --- Pass 1: run both migrations ---
	s, err := New(dir)
	require.NoError(t, err)
	require.NoError(t, s.MigrateRefTags())

	// projects.md must be story-free.
	projects, err := s.LoadProjects()
	require.NoError(t, err)
	require.Len(t, projects, 1, "story dropped; only the parent project remains")
	assert.Equal(t, "ship-v2", projects[0].ID)

	// Daily task must carry the "#project" canonical tag now.
	d, exists, err := s.LoadDaily(date("2026-07-07"))
	require.NoError(t, err)
	require.True(t, exists)
	require.Len(t, d.Plan, 1)
	assert.Equal(t, "wire refunds #ship-v2", d.Plan[0].Text,
		"story tag rewritten to parent project, then @-> # migrated")

	// Backlog must also be migrated.
	bl, err := s.LoadBacklog()
	require.NoError(t, err)
	require.Len(t, bl.Current, 1)
	assert.Equal(t, "backlog task #ship-v2", bl.Current[0])

	// RecurringTasks must still see the template after both migrations (C1).
	tasks, err := s.RecurringTasks()
	require.NoError(t, err)
	require.Len(t, tasks, 1, "recurring template must survive both migrations")
	assert.Equal(t, "Weekly review", tasks[0].Text)
	assert.Equal(t, "ship-v2", tasks[0].Project)

	// --- Pass 2: running both again must be a no-op (idempotent) ---
	s2, err := New(dir)
	require.NoError(t, err)
	require.NoError(t, s2.MigrateRefTags())

	tasks2, err := s2.RecurringTasks()
	require.NoError(t, err)
	require.Len(t, tasks2, 1, "idempotent: template count unchanged on re-run")
	assert.Equal(t, tasks[0].Text, tasks2[0].Text)
	assert.Equal(t, tasks[0].Project, tasks2[0].Project)

	d2, _, err := s2.LoadDaily(date("2026-07-07"))
	require.NoError(t, err)
	require.Len(t, d2.Plan, 1)
	assert.Equal(t, d.Plan[0].Text, d2.Plan[0].Text, "daily file unchanged on second run")
}

// TestStore_MigrationNoOpWithoutStories verifies that a store constructed
// over a data directory with no legacy story headings (the common case,
// including a brand-new directory) never creates a backup or touches
// anything.
func TestStore_MigrationNoOpWithoutStories(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	s, err := New(dir)
	require.NoError(t, err)
	_, err = s.AddProject("Ship v2")
	require.NoError(t, err)

	assert.NoDirExists(t, filepath.Join(dir, backupDirName))

	s2, err := New(dir)
	require.NoError(t, err)
	assert.NoDirExists(t, filepath.Join(dir, backupDirName))
	projects, err := s2.LoadProjects()
	require.NoError(t, err)
	require.Len(t, projects, 1)
	assert.Equal(t, "Ship v2", projects[0].Name)
}
