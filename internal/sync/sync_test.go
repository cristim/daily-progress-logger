package sync

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cristim/daily-progress-logger/internal/drive"
)

// fakeDrive is an in-memory DriveClient shared by test engines.
type fakeDrive struct {
	mu    sync.Mutex
	files map[string]*fakeFile // by id
	seq   int
	clock time.Time
}

type fakeFile struct {
	id       string
	path     string
	content  []byte
	modified time.Time
}

func newFakeDrive() *fakeDrive {
	return &fakeDrive{files: map[string]*fakeFile{}, clock: time.Date(2026, 7, 10, 9, 0, 0, 0, time.UTC)}
}

func (f *fakeDrive) List(context.Context) ([]drive.File, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]drive.File, 0, len(f.files))
	for _, ff := range f.files {
		out = append(out, drive.File{Path: ff.path, ID: ff.id, MD5: md5hex(ff.content), Modified: ff.modified})
	}
	return out, nil
}

func (f *fakeDrive) Download(_ context.Context, id string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	ff, ok := f.files[id]
	if !ok {
		return nil, fmt.Errorf("no file %s", id)
	}
	return append([]byte(nil), ff.content...), nil
}

func (f *fakeDrive) Upload(_ context.Context, path, id string, content []byte) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.clock = f.clock.Add(time.Second)
	if id != "" {
		ff, ok := f.files[id]
		if !ok {
			return "", fmt.Errorf("update missing %s", id)
		}
		ff.content, ff.path, ff.modified = append([]byte(nil), content...), path, f.clock
		return id, nil
	}
	f.seq++
	nid := fmt.Sprintf("id%d", f.seq)
	f.files[nid] = &fakeFile{id: nid, path: path, content: append([]byte(nil), content...), modified: f.clock}
	return nid, nil
}

func (f *fakeDrive) Delete(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.files, id)
	return nil
}

func writeFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	full := filepath.Join(dir, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o750))
	require.NoError(t, os.WriteFile(full, []byte(content), 0o600))
}

func readFile(t *testing.T, dir, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
	require.NoError(t, err)
	return string(b)
}

func exists(dir, rel string) bool {
	_, err := os.Stat(filepath.Join(dir, filepath.FromSlash(rel)))
	return err == nil
}

// conflictCopy finds the single "(conflict ...)" file in dir's tree.
func conflictCopy(t *testing.T, dir string) string {
	t.Helper()
	var found string
	require.NoError(t, filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.Contains(d.Name(), "(conflict ") {
			rel, _ := filepath.Rel(dir, p)
			found = filepath.ToSlash(rel)
		}
		return nil
	}))
	return found
}

func TestSync_CreateEditDeleteConverge(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fd := newFakeDrive()
	dirA, dirB := t.TempDir(), t.TempDir()
	a := New(dirA, fd, "laptop")
	b := New(dirB, fd, "desktop")

	// Create on A, sync up, sync down to B.
	writeFile(t, dirA, "backlog.md", "v0")
	res, err := a.Run(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, res.Uploaded)
	res, err = b.Run(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, res.Downloaded)
	assert.Equal(t, "v0", readFile(t, dirB, "backlog.md"))

	// Edit on A propagates to B.
	writeFile(t, dirA, "backlog.md", "v1")
	_, err = a.Run(ctx)
	require.NoError(t, err)
	_, err = b.Run(ctx)
	require.NoError(t, err)
	assert.Equal(t, "v1", readFile(t, dirB, "backlog.md"))

	// Delete on A propagates to B.
	require.NoError(t, os.Remove(filepath.Join(dirA, "backlog.md")))
	_, err = a.Run(ctx)
	require.NoError(t, err)
	_, err = b.Run(ctx)
	require.NoError(t, err)
	assert.False(t, exists(dirB, "backlog.md"), "delete synced down")
}

func TestSync_NestedNewRemote(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fd := newFakeDrive()
	dirA, dirB := t.TempDir(), t.TempDir()
	a, b := New(dirA, fd, "laptop"), New(dirB, fd, "desktop")

	writeFile(t, dirA, "daily/2026/07/2026-07-10.md", "plan")
	_, err := a.Run(ctx)
	require.NoError(t, err)
	_, err = b.Run(ctx)
	require.NoError(t, err)
	assert.Equal(t, "plan", readFile(t, dirB, "daily/2026/07/2026-07-10.md"), "nested path recreated")
}

func TestSync_PullBeforePush_NoBlindOverwrite(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fd := newFakeDrive()
	dirA, dirB := t.TempDir(), t.TempDir()
	a, b := New(dirA, fd, "laptop"), New(dirB, fd, "desktop")

	// Base synced on both.
	writeFile(t, dirA, "backlog.md", "v0")
	_, err := a.Run(ctx)
	require.NoError(t, err)
	_, err = b.Run(ctx)
	require.NoError(t, err)

	// Only B edits; A syncs (no local change) and must pull B's version, never
	// pushing a stale "v0".
	writeFile(t, dirB, "backlog.md", "vB")
	_, err = b.Run(ctx)
	require.NoError(t, err)
	res, err := a.Run(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, res.Downloaded)
	assert.Equal(t, 0, res.Uploaded)
	assert.Equal(t, "vB", readFile(t, dirA, "backlog.md"))
}

func TestSync_ConflictKeepsBothVersions(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fd := newFakeDrive()
	dirA, dirB := t.TempDir(), t.TempDir()
	a, b := New(dirA, fd, "laptop"), New(dirB, fd, "desktop")

	// Base synced on both.
	writeFile(t, dirA, "backlog.md", "v0")
	_, err := a.Run(ctx)
	require.NoError(t, err)
	_, err = b.Run(ctx)
	require.NoError(t, err)

	// Both edit from the same base, A syncs first.
	writeFile(t, dirA, "backlog.md", "vA")
	_, err = a.Run(ctx)
	require.NoError(t, err)
	writeFile(t, dirB, "backlog.md", "vB")
	res, err := b.Run(ctx)
	require.NoError(t, err)
	require.Len(t, res.Conflicts, 1, "both-changed is a conflict")

	// B keeps its own version canonical and saves A's as a conflict copy; both
	// survive and the conflict is recorded.
	assert.Equal(t, "vB", readFile(t, dirB, "backlog.md"))
	copyB := conflictCopy(t, dirB)
	require.NotEmpty(t, copyB)
	assert.Equal(t, "vA", readFile(t, dirB, copyB))
	recorded, err := b.Conflicts()
	require.NoError(t, err)
	require.Len(t, recorded, 1)
	assert.Equal(t, "backlog.md", recorded[0].Path)

	// A re-syncs and converges: gets B's canonical version + the conflict copy.
	_, err = a.Run(ctx)
	require.NoError(t, err)
	assert.Equal(t, "vB", readFile(t, dirA, "backlog.md"))
	assert.Equal(t, "vA", readFile(t, dirA, copyB), "conflict copy synced to A")
}

func TestSync_ResolveKeepRemote(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fd := newFakeDrive()
	dirA, dirB := t.TempDir(), t.TempDir()
	a, b := New(dirA, fd, "laptop"), New(dirB, fd, "desktop")

	writeFile(t, dirA, "backlog.md", "v0")
	_, err := a.Run(ctx)
	require.NoError(t, err)
	_, err = b.Run(ctx)
	require.NoError(t, err)
	writeFile(t, dirA, "backlog.md", "vA")
	_, err = a.Run(ctx)
	require.NoError(t, err)
	writeFile(t, dirB, "backlog.md", "vB")
	_, err = b.Run(ctx)
	require.NoError(t, err)

	// Resolve on B: keep the other device's version (A's "vA").
	require.NoError(t, b.Resolve("backlog.md", KeepRemote))
	assert.Equal(t, "vA", readFile(t, dirB, "backlog.md"), "canonical becomes the remote version")
	assert.Empty(t, conflictCopy(t, dirB), "conflict copy removed")
	remaining, err := b.Conflicts()
	require.NoError(t, err)
	assert.Empty(t, remaining, "record cleared")

	// Next sync converges: A adopts "vA" back (its own edit) and the copy is gone.
	_, err = b.Run(ctx)
	require.NoError(t, err)
	_, err = a.Run(ctx)
	require.NoError(t, err)
	assert.Equal(t, "vA", readFile(t, dirA, "backlog.md"))
	assert.Empty(t, conflictCopy(t, dirA))
}
