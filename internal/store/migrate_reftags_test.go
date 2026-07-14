package store

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const migrateTestDate = "2026-07-06"

// writeDailyWithTag is a test helper that writes a minimal daily file whose
// plan contains one task with the given trailing tag (e.g. " @cudly").
// The date is always migrateTestDate (2026-07-06) so helpers don't carry
// a date parameter that would always receive the same value.
func writeDailyWithTag(t *testing.T, s *Store, taskLine string) string {
	t.Helper()
	dir := filepath.Join(s.DataDir, "daily", "2026", "07")
	require.NoError(t, os.MkdirAll(dir, 0o750))
	path := filepath.Join(dir, migrateTestDate+".md")
	content := "---\ndate: " + migrateTestDate +
		"\nday: Monday\nweek: 2026-W28\nmorning_done: false\nevening_done: false\n---\n\n" +
		"# Monday, 6 July 2026\n\n## Plan\n\n" + taskLine + "\n\n## Done\n"
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

// TestMigrateRefTags_Basic verifies that @knownid -> #knownid in daily files
// while @nonid tokens and #already-migrated tokens are left untouched.
func TestMigrateRefTags_Basic(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	pid, err := s.AddProject("CUDly")
	require.NoError(t, err)
	assert.Equal(t, "cudly", pid)

	// File with a legacy @cudly tag.
	p := writeDailyWithTag(t, s, "- [ ] Review/merge 10 PRs @daily @cudly")

	require.NoError(t, s.MigrateRefTags())

	data, err := os.ReadFile(p)
	require.NoError(t, err)
	content := string(data)
	assert.Contains(t, content, "Review/merge 10 PRs @daily #cudly",
		"known id @cudly must become #cudly")
	assert.NotContains(t, content, "@cudly", "@cudly must not survive migration")
	// @daily is not a known project id: left untouched.
	assert.Contains(t, content, "@daily", "@daily (not a project id) must be preserved")
}

// TestMigrateRefTags_Idempotent verifies that running MigrateRefTags a second
// time is a true no-op: no files are touched and no second backup is created.
func TestMigrateRefTags_Idempotent(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	pid, err := s.AddProject("CUDly")
	require.NoError(t, err)
	assert.Equal(t, "cudly", pid)

	writeDailyWithTag(t, s, "- [ ] Task @cudly")

	// First run migrates.
	require.NoError(t, s.MigrateRefTags())

	// Note mtime of the backup dir as a change sentinel.
	backupDir := filepath.Join(s.DataDir, ".pre-hashtag-backup")
	info1, err := os.Stat(backupDir)
	require.NoError(t, err)

	// Second run: no files have @cudly any more, so nothing should change.
	require.NoError(t, s.MigrateRefTags())

	info2, err := os.Stat(backupDir)
	require.NoError(t, err)
	assert.Equal(t, info1.ModTime(), info2.ModTime(), "backup dir must not be touched on second run")
}

// TestMigrateRefTags_BackupCreated verifies that the backup directory is
// populated with a copy of the original files before any rewriting.
func TestMigrateRefTags_BackupCreated(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	pid, err := s.AddProject("CUDly")
	require.NoError(t, err)
	_ = pid

	writeDailyWithTag(t, s, "- [ ] Task @cudly")

	require.NoError(t, s.MigrateRefTags())

	backupPath := filepath.Join(s.DataDir, ".pre-hashtag-backup", "daily", "2026", "07", "2026-07-06.md")
	data, err := os.ReadFile(backupPath)
	require.NoError(t, err)
	// Backup must contain the original @cudly, not the migrated #cudly.
	assert.Contains(t, string(data), "@cudly", "backup must preserve original @-tagged content")
	assert.NotContains(t, string(data), "#cudly", "backup must not contain the rewritten # form")
}

// TestMigrateRefTags_UnknownIdUntouched verifies that @tokens whose body is
// not a known project id are never rewritten.
func TestMigrateRefTags_UnknownIdUntouched(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	// No projects registered, so "unknown" is not a known id.
	p := writeDailyWithTag(t, s, "- [ ] Ping @alice about launch")

	require.NoError(t, s.MigrateRefTags())

	data, err := os.ReadFile(p)
	require.NoError(t, err)
	assert.Contains(t, string(data), "@alice", "@alice (unknown id) must not be rewritten")
}

// TestMigrateRefTags_AlreadyHashUntouched verifies that a task already using
// the canonical #slug form is left untouched on every run.
func TestMigrateRefTags_AlreadyHashUntouched(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	pid, err := s.AddProject("CUDly")
	require.NoError(t, err)
	_ = pid

	p := writeDailyWithTag(t, s, "- [ ] Task #cudly")
	originalData, err := os.ReadFile(p)
	require.NoError(t, err)

	require.NoError(t, s.MigrateRefTags())

	newData, err := os.ReadFile(p)
	require.NoError(t, err)
	assert.Equal(t, string(originalData), string(newData), "already-migrated file must not be changed")
	// No backup should be created when there is nothing to migrate.
	backupDir := filepath.Join(s.DataDir, ".pre-hashtag-backup")
	_, statErr := os.Stat(backupDir)
	assert.True(t, os.IsNotExist(statErr), "no backup should be created when there is nothing to migrate")
}

// TestMigrateRefTags_MixedLine verifies the canonical example from the spec:
// "Review/merge 10 PRs @daily @cudly" -> "Review/merge 10 PRs @daily #cudly".
func TestMigrateRefTags_MixedLine(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	pid, err := s.AddProject("CUDly")
	require.NoError(t, err)
	_ = pid

	p := writeDailyWithTag(t, s, "- [ ] Review/merge 10 PRs @daily @cudly")
	require.NoError(t, s.MigrateRefTags())

	data, err := os.ReadFile(p)
	require.NoError(t, err)
	assert.Contains(t, string(data), "Review/merge 10 PRs @daily #cudly",
		"@daily (non-project) preserved, @cudly (project id) migrated to #cudly")
}

// TestMigrateRefTags_RecycleAndBacklog verifies that recycle.md and backlog.md
// are also migrated.
func TestMigrateRefTags_RecycleAndBacklog(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	pid, err := s.AddProject("CUDly")
	require.NoError(t, err)
	_ = pid

	// Write a recycle file with an @cudly ref.
	recycleContent := "# Recycle bin\n\n## 2026-07-06\n\n- [ ] old task @cudly\n"
	require.NoError(t, os.WriteFile(s.RecyclePath(), []byte(recycleContent), 0o600))

	// Write a backlog with an @cudly ref.
	backlogContent := "# Backlog\n\n## Current\n\n- backlog task @cudly\n\n## Next week\n"
	require.NoError(t, os.WriteFile(s.BacklogPath(), []byte(backlogContent), 0o600))

	require.NoError(t, s.MigrateRefTags())

	rd, err := os.ReadFile(s.RecyclePath())
	require.NoError(t, err)
	assert.Contains(t, string(rd), "#cudly", "recycle.md must be migrated")
	assert.NotContains(t, string(rd), "@cudly")

	bd, err := os.ReadFile(s.BacklogPath())
	require.NoError(t, err)
	assert.Contains(t, string(bd), "#cudly", "backlog.md must be migrated")
	assert.NotContains(t, string(bd), "@cudly")
}

// TestMigrateLineRefTag tests the low-level per-line replacement function
// directly to cover edge cases clearly.
func TestMigrateLineRefTag(t *testing.T) {
	t.Parallel()
	known := map[string]bool{"cudly": true, "marketing": true}
	cases := []struct{ in, want string }{
		{"- [ ] Task @cudly", "- [ ] Task #cudly"},
		{"- [ ] Task @daily @cudly", "- [ ] Task @daily #cudly"}, // only trailing
		{"- [ ] Task @unknown", "- [ ] Task @unknown"},           // unknown id: untouched
		{"- [ ] Task #cudly", "- [ ] Task #cudly"},               // already migrated: no-op
		{"@cudly", "#cudly"},                                     // tag-only line (edge case)
		{"- [ ] Plain task", "- [ ] Plain task"},                 // no tag
		{"", ""},                                                 // empty line
	}
	for _, c := range cases {
		got := migrateLineRefTag(c.in, known)
		assert.Equal(t, c.want, got, "migrateLineRefTag(%q)", c.in)
	}
}
