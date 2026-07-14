// Package sync keeps a local data directory and a Google Drive folder in step,
// two-way, using last-writer-wins with conflict copies so no edit is ever lost.
// It reads remote state before it writes (pull before push) and only uploads a
// local change when the remote still matches the last synced version, so a newer
// remote edit is never blindly overwritten.
package sync

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/cristim/daily-progress-logger/internal/drive"
)

// DriveClient is the subset of the Drive wrapper the engine needs (so tests can
// substitute a fake). *drive.Client satisfies it.
type DriveClient interface {
	List(ctx context.Context) ([]drive.File, error)
	Download(ctx context.Context, id string) ([]byte, error)
	Upload(ctx context.Context, relPath, id string, content []byte) (string, error)
	Delete(ctx context.Context, id string) error
}

// Engine syncs dir against a Drive client. It is safe for concurrent Run calls
// (they serialize) but the caller must not mutate dir mid-run.
type Engine struct {
	dir    string
	drive  DriveClient
	device string
	mu     sync.Mutex
}

// New returns an engine syncing dir with dc, tagging conflict copies with device.
func New(dir string, dc DriveClient, device string) *Engine {
	return &Engine{dir: dir, drive: dc, device: device}
}

// NewLocal returns an engine with no Drive client, for reading and resolving
// conflicts offline (Conflicts/Resolve touch only local files). Run errors until
// a Drive client is set.
func NewLocal(dir, device string) *Engine {
	return &Engine{dir: dir, device: device}
}

// Conflict records a file that changed on both sides; both versions are kept
// (the remote saved as ConflictCopy) until the user resolves it.
type Conflict struct {
	Path         string    `json:"path"`
	ConflictCopy string    `json:"conflict_copy"`
	Time         time.Time `json:"time"`
}

// Result summarizes one sync run.
type Result struct {
	Uploaded   int
	Downloaded int
	Deleted    int
	Conflicts  []Conflict
}

const (
	stateFile     = ".sync-state.json"
	conflictsFile = ".conflicts.json"
)

// fileState is the per-path bookkeeping between runs: the Drive file ID and the
// md5 both sides agreed on at the last successful sync.
type fileState struct {
	DriveID   string `json:"drive_id"`
	SyncedMD5 string `json:"synced_md5"`
}

type persistedState struct {
	Files map[string]fileState `json:"files"`
}

const (
	actNone = iota - 1 // -1: nothing to transfer
	actUpload
	actDownload
	actDeleteLocal
	actDeleteRemote
	actConflict
)

type decision struct {
	kind int
	path string
}

// Run performs one two-way sync and returns the outcome, including any new
// conflicts (also appended to the persisted conflict list).
func (e *Engine) Run(ctx context.Context) (Result, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.drive == nil {
		return Result{}, errors.New("sync engine has no Drive client (not signed in)")
	}
	state, err := e.loadState()
	if err != nil {
		return Result{}, err
	}
	local, err := e.scanLocal()
	if err != nil {
		return Result{}, err
	}
	remoteList, err := e.drive.List(ctx)
	if err != nil {
		return Result{}, err
	}
	remote := map[string]drive.File{}
	for _, f := range remoteList {
		remote[f.Path] = f
	}

	decisions, newState := classify(local, remote, state.Files)

	rs := &runState{e: e, remote: remote, newState: newState, local: local}
	if err := rs.pulls(ctx, decisions); err != nil { // read remote down first
		return rs.res, err
	}
	if err := rs.conflicts(ctx, decisions); err != nil { // keep both versions
		return rs.res, err
	}
	if err := rs.pushes(ctx, decisions); err != nil { // send local up
		return rs.res, err
	}
	if err := e.saveState(persistedState{Files: newState}); err != nil {
		return rs.res, err
	}
	if len(rs.res.Conflicts) > 0 {
		if err := e.appendConflicts(rs.res.Conflicts); err != nil {
			return rs.res, err
		}
	}
	return rs.res, nil
}

// runState carries the per-run maps and result across the pull/conflict/push
// phases without long parameter lists.
type runState struct {
	e        *Engine
	remote   map[string]drive.File
	newState map[string]fileState
	local    map[string]string // md5s captured by scanLocal; used for TOCTOU check in pull
	res      Result
}

func (r *runState) pulls(ctx context.Context, decisions []decision) error {
	for _, d := range decisions {
		switch d.kind {
		case actDownload:
			// TOCTOU guard: re-hash the local file immediately before overwriting.
			// If it changed since scanLocal ran (e.g. the user or an editor saved
			// it), the classifier's actDownload decision is stale — keep both
			// versions as a conflict instead of silently clobbering the new edit.
			if cur, err := r.e.readLocal(d.path); err == nil {
				if md5hex(cur) != r.local[d.path] {
					c, cerr := r.e.resolveConflictCopy(ctx, d.path, r.remote[d.path], r.newState)
					if cerr != nil {
						return cerr
					}
					r.res.Conflicts = append(r.res.Conflicts, c)
					continue
				}
			}
			if err := r.e.pull(ctx, d.path, r.remote[d.path], r.newState); err != nil {
				return err
			}
			r.res.Downloaded++
		case actDeleteLocal:
			if err := r.e.removeLocal(d.path); err != nil {
				return err
			}
			delete(r.newState, d.path)
			r.res.Deleted++
		}
	}
	return nil
}

func (r *runState) conflicts(ctx context.Context, decisions []decision) error {
	for _, d := range decisions {
		if d.kind != actConflict {
			continue
		}
		c, err := r.e.resolveConflictCopy(ctx, d.path, r.remote[d.path], r.newState)
		if err != nil {
			return err
		}
		r.res.Conflicts = append(r.res.Conflicts, c)
	}
	return nil
}

func (r *runState) pushes(ctx context.Context, decisions []decision) error {
	for _, d := range decisions {
		switch d.kind {
		case actUpload:
			if err := r.e.push(ctx, d.path, r.remote, r.newState); err != nil {
				return err
			}
			r.res.Uploaded++
		case actDeleteRemote:
			if err := r.e.drive.Delete(ctx, r.remote[d.path].ID); err != nil {
				return err
			}
			delete(r.newState, d.path)
			r.res.Deleted++
		}
	}
	return nil
}

// classify decides an action per path and carries forward unchanged state.
func classify(local map[string]string, remote map[string]drive.File, synced map[string]fileState) ([]decision, map[string]fileState) {
	newState := map[string]fileState{}
	paths := map[string]struct{}{}
	for p := range local {
		paths[p] = struct{}{}
	}
	for p := range remote {
		paths[p] = struct{}{}
	}
	for p := range synced {
		paths[p] = struct{}{}
	}

	var decisions []decision
	for p := range paths {
		fs, sok := synced[p]
		kind, keep := decidePath(local[p], remote[p], fs.SyncedMD5, has(local, p), has(remote, p), sok)
		if keep != nil {
			newState[p] = *keep
		}
		if kind != actNone {
			decisions = append(decisions, decision{kind, p})
		}
	}
	return decisions, newState
}

func has[V any](m map[string]V, k string) bool {
	_, ok := m[k]
	return ok
}

// decidePath returns the action for one path given the local md5, remote file,
// the last-synced md5 (base), and whether each side has it. keep is non-nil when
// the path's state carries forward unchanged (already-equal sides).
func decidePath(lm string, rf drive.File, base string, lok, rok, sok bool) (int, *fileState) {
	switch {
	case lok && rok:
		return decideBoth(lm, rf, base)
	case lok:
		return decideLocalOnly(lm, base, sok) // new locally, or remote-deleted
	case rok:
		return decideRemoteOnly(rf, base, sok) // new remotely, or local-deleted
	default:
		return actNone, nil // gone on both sides
	}
}

func decideBoth(lm string, rf drive.File, base string) (int, *fileState) {
	if lm == rf.MD5 {
		return actNone, &fileState{DriveID: rf.ID, SyncedMD5: lm} // already equal
	}
	switch localChanged, remoteChanged := lm != base, rf.MD5 != base; {
	case localChanged && !remoteChanged:
		return actUpload, nil
	case !localChanged && remoteChanged:
		return actDownload, nil
	default:
		return actConflict, nil // both changed
	}
}

func decideLocalOnly(lm, base string, sok bool) (int, *fileState) {
	if !sok || lm != base { // new, or remote-deleted-but-locally-edited: (re)upload
		return actUpload, nil
	}
	return actDeleteLocal, nil
}

func decideRemoteOnly(rf drive.File, base string, sok bool) (int, *fileState) {
	if !sok || rf.MD5 != base { // new, or local-deleted-but-remotely-edited: download
		return actDownload, nil
	}
	return actDeleteRemote, nil
}

func (e *Engine) pull(ctx context.Context, path string, rf drive.File, newState map[string]fileState) error {
	content, err := e.drive.Download(ctx, rf.ID)
	if err != nil {
		return err
	}
	if err := e.writeLocal(path, content); err != nil {
		return err
	}
	newState[path] = fileState{DriveID: rf.ID, SyncedMD5: md5hex(content)}
	return nil
}

func (e *Engine) push(ctx context.Context, path string, remote map[string]drive.File, newState map[string]fileState) error {
	content, err := e.readLocal(path)
	if err != nil {
		return err
	}
	id, err := e.drive.Upload(ctx, path, remote[path].ID, content) // remote[path].ID is "" when creating
	if err != nil {
		return err
	}
	newState[path] = fileState{DriveID: id, SyncedMD5: md5hex(content)}
	return nil
}

// resolveConflictCopy keeps local as the canonical file and saves the remote
// version as a conflict copy (both locally and on Drive), recording the conflict.
func (e *Engine) resolveConflictCopy(ctx context.Context, path string, rf drive.File, newState map[string]fileState) (Conflict, error) {
	remoteContent, err := e.drive.Download(ctx, rf.ID)
	if err != nil {
		return Conflict{}, err
	}
	now := time.Now()
	copyPath := drive.ConflictName(path, e.device, now)
	if err := e.writeLocal(copyPath, remoteContent); err != nil {
		return Conflict{}, err
	}
	copyID, err := e.drive.Upload(ctx, copyPath, "", remoteContent)
	if err != nil {
		return Conflict{}, err
	}
	newState[copyPath] = fileState{DriveID: copyID, SyncedMD5: md5hex(remoteContent)}

	localContent, err := e.readLocal(path)
	if err != nil {
		return Conflict{}, err
	}
	pathID, err := e.drive.Upload(ctx, path, rf.ID, localContent)
	if err != nil {
		return Conflict{}, err
	}
	newState[path] = fileState{DriveID: pathID, SyncedMD5: md5hex(localContent)}
	return Conflict{Path: path, ConflictCopy: copyPath, Time: now}, nil
}

// --- local filesystem helpers ---

func (e *Engine) scanLocal() (map[string]string, error) {
	out := map[string]string{}
	err := filepath.WalkDir(e.dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if p != e.dir && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(d.Name(), ".") {
			return nil // skip .sync-state.json etc.
		}
		content, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(e.dir, p)
		if err != nil {
			return err
		}
		out[filepath.ToSlash(rel)] = md5hex(content)
		return nil
	})
	if os.IsNotExist(err) {
		return out, nil
	}
	return out, err
}

func (e *Engine) abs(path string) string {
	return filepath.Join(e.dir, filepath.FromSlash(path))
}

func (e *Engine) readLocal(path string) ([]byte, error) {
	return os.ReadFile(e.abs(path))
}

func (e *Engine) writeLocal(path string, content []byte) error {
	full := e.abs(path)
	dir := filepath.Dir(full)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	// Dot-prefixed temp name: scanLocal already skips dotfiles, so this temp
	// never appears as a sync candidate, and it never collides with the store's
	// own *.md.tmp temporaries (H4).
	tmp := filepath.Join(dir, "."+filepath.Base(full)+".synctmp")
	if err := os.WriteFile(tmp, content, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, full); err != nil {
		_ = os.Remove(tmp) // best-effort cleanup on rename failure
		return err
	}
	return nil
}

func (e *Engine) removeLocal(path string) error {
	err := os.Remove(e.abs(path))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// --- state / conflicts persistence ---

func (e *Engine) loadState() (persistedState, error) {
	content, err := os.ReadFile(filepath.Join(e.dir, stateFile))
	if os.IsNotExist(err) {
		return persistedState{Files: map[string]fileState{}}, nil
	}
	if err != nil {
		return persistedState{}, err
	}
	var st persistedState
	if err := json.Unmarshal(content, &st); err != nil {
		return persistedState{}, fmt.Errorf("parsing sync state: %w", err)
	}
	if st.Files == nil {
		st.Files = map[string]fileState{}
	}
	return st, nil
}

func (e *Engine) saveState(st persistedState) error {
	content, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(e.dir, stateFile), content, 0o600)
}

// ResolveChoice is how the user settles a conflict.
type ResolveChoice string

const (
	// KeepLocal keeps this device's version and discards the conflict copy.
	KeepLocal ResolveChoice = "keep_local"
	// KeepRemote adopts the other device's version (the conflict copy).
	KeepRemote ResolveChoice = "keep_remote"
	// KeepBoth leaves both files in place.
	KeepBoth ResolveChoice = "keep_both"
)

// Resolve settles the conflict for path per choice and clears its record. File
// changes propagate to Drive on the next Run (the conflict copy is removed
// locally, and for KeepRemote the canonical file is rewritten from it).
func (e *Engine) Resolve(path string, choice ResolveChoice) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	conflicts, err := e.Conflicts()
	if err != nil {
		return err
	}
	var target Conflict
	remaining := conflicts[:0:0]
	found := false
	for _, c := range conflicts {
		if !found && c.Path == path {
			target, found = c, true
			continue
		}
		remaining = append(remaining, c)
	}
	if !found {
		return nil
	}

	switch choice {
	case KeepLocal:
		if err := e.removeLocal(target.ConflictCopy); err != nil {
			return err
		}
	case KeepRemote:
		content, err := e.readLocal(target.ConflictCopy)
		if err != nil {
			return err
		}
		if err := e.writeLocal(target.Path, content); err != nil {
			return err
		}
		if err := e.removeLocal(target.ConflictCopy); err != nil {
			return err
		}
	case KeepBoth:
		// Leave both files; just clear the record.
	}
	return e.saveConflicts(remaining)
}

// Conflicts returns the unresolved conflicts recorded across runs.
func (e *Engine) Conflicts() ([]Conflict, error) {
	content, err := os.ReadFile(filepath.Join(e.dir, conflictsFile))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var cs []Conflict
	if err := json.Unmarshal(content, &cs); err != nil {
		return nil, fmt.Errorf("parsing conflicts: %w", err)
	}
	return cs, nil
}

func (e *Engine) appendConflicts(cs []Conflict) error {
	existing, err := e.Conflicts()
	if err != nil {
		return err
	}
	return e.saveConflicts(append(existing, cs...))
}

func (e *Engine) saveConflicts(cs []Conflict) error {
	if len(cs) == 0 {
		return e.removeLocalRaw(conflictsFile)
	}
	content, err := json.MarshalIndent(cs, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(e.dir, conflictsFile), content, 0o600)
}

func (e *Engine) removeLocalRaw(name string) error {
	err := os.Remove(filepath.Join(e.dir, name))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func md5hex(b []byte) string {
	sum := md5.Sum(b)
	return hex.EncodeToString(sum[:])
}
