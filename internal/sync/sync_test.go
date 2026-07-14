package sync

import (
	"context"
	"fmt"
	"io/fs"
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

// toctouDrive wraps a fakeDrive and injects a local file mutation during
// List(), simulating a concurrent edit that happens between scanLocal and pull.
type toctouDrive struct {
	*fakeDrive
	localDir   string
	path       string // relative slash path to mutate
	newContent []byte
}

func (f *toctouDrive) List(ctx context.Context) ([]drive.File, error) {
	// Simulate the concurrent local edit after scanLocal but before pull.
	if f.path != "" && f.newContent != nil {
		full := filepath.Join(f.localDir, filepath.FromSlash(f.path))
		_ = os.MkdirAll(filepath.Dir(full), 0o750)
		_ = os.WriteFile(full, f.newContent, 0o600)
	}
	return f.fakeDrive.List(ctx)
}

// TestSync_LocalChangedAfterScanIsConflict verifies H2a: if a local file
// changes between scanLocal and the actDownload write, it is reclassified
// as a conflict (both versions kept) instead of being silently overwritten.
func TestSync_LocalChangedAfterScanIsConflict(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fd := newFakeDrive()
	dirA, dirB := t.TempDir(), t.TempDir()
	a := New(dirA, fd, "laptop")

	// Establish base "v0" on both devices.
	writeFile(t, dirA, "backlog.md", "v0")
	_, err := a.Run(ctx)
	require.NoError(t, err)
	b := New(dirB, fd, "desktop")
	_, err = b.Run(ctx)
	require.NoError(t, err)
	assert.Equal(t, "v0", readFile(t, dirB, "backlog.md"))

	// A pushes "vA"; remote is now vA.
	writeFile(t, dirA, "backlog.md", "vA")
	_, err = a.Run(ctx)
	require.NoError(t, err)

	// B's classifier sees: local="v0" (matches base), remote="vA" → actDownload.
	// Inject a TOCTOU edit to "vB_local" via List() (after scan, before pull).
	tfd := &toctouDrive{
		fakeDrive:  fd,
		localDir:   dirB,
		path:       "backlog.md",
		newContent: []byte("vB_local"),
	}
	bRacy := New(dirB, tfd, "desktop")
	res, err := bRacy.Run(ctx)
	require.NoError(t, err)
	require.Len(t, res.Conflicts, 1, "TOCTOU edit must be reclassified as conflict, not overwrite")
	assert.Equal(t, "vB_local", readFile(t, dirB, "backlog.md"),
		"local TOCTOU edit must not be overwritten")
	cc := conflictCopy(t, dirB)
	require.NotEmpty(t, cc, "remote version must be saved as conflict copy")
	assert.Equal(t, "vA", readFile(t, dirB, cc))
}

// TestSync_ScanLocalIgnoresTmpFiles verifies H4: scanLocal skips *.tmp files
// (store atomics) and non-.md files so they are never uploaded.
func TestSync_ScanLocalIgnoresTmpFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeFile(t, dir, "backlog.md", "content")
	writeFile(t, dir, "backlog.md.tmp", "in-flight store write") // store's atomic temp
	writeFile(t, dir, "notes.txt", "non-md file")               // non-.md file

	e := New(dir, nil, "test")
	local, err := e.scanLocal()
	require.NoError(t, err)
	assert.Contains(t, local, "backlog.md", "real .md file is included")
	assert.NotContains(t, local, "backlog.md.tmp", "store temp is excluded")
	assert.NotContains(t, local, "notes.txt", "non-.md file is excluded")
}

// TestSync_WriteLocalIsAtomic verifies H2b: writeLocal uses tmp+rename so no
// partial file is left on disk, and the temp file (dotfile) is invisible to
// subsequent scanLocal calls.
func TestSync_WriteLocalIsAtomic(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	e := New(dir, nil, "test")

	require.NoError(t, e.writeLocal("sub/file.md", []byte("hello")))
	b, err := os.ReadFile(filepath.Join(dir, "sub/file.md"))
	require.NoError(t, err)
	assert.Equal(t, "hello", string(b))

	// No .synctmp leftovers.
	var temps []string
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err == nil && strings.HasSuffix(d.Name(), ".synctmp") {
			temps = append(temps, p)
		}
		return nil
	})
	assert.Empty(t, temps, "no .synctmp leftovers after writeLocal")
}
