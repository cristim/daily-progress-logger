package drive

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRelPath(t *testing.T) {
	t.Parallel()
	// root r0; daily(f1<-r0) / 2026(f2<-f1) / 07(f3<-f2)
	folders := map[string]folderNode{
		"f1": {name: "daily", parent: "r0"},
		"f2": {name: "2026", parent: "f1"},
		"f3": {name: "07", parent: "f2"},
	}

	p, ok := relPath("2026-07-10.md", "f3", folders, "r0")
	assert.True(t, ok)
	assert.Equal(t, "daily/2026/07/2026-07-10.md", p)

	p, ok = relPath("projects.md", "r0", folders, "r0")
	assert.True(t, ok)
	assert.Equal(t, "projects.md", p)

	_, ok = relPath("orphan.md", "ghost", folders, "r0")
	assert.False(t, ok, "file whose parent chain never reaches root is skipped")
}

func TestConflictName(t *testing.T) {
	t.Parallel()
	ts := time.Date(2026, 7, 10, 9, 30, 0, 0, time.UTC)
	assert.Equal(t,
		"daily/2026/07/2026-07-10 (conflict laptop 2026-07-10 093000).md",
		ConflictName("daily/2026/07/2026-07-10.md", "laptop", ts))
	assert.Equal(t,
		"backlog (conflict phone 2026-07-10 093000).md",
		ConflictName("backlog.md", "phone", ts))
}
