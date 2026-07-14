# Code Review — post-integration (feat/unified @ f83a8c6)

## Resolution (2026-07-14) — ALL findings addressed

Implemented in two Sonnet clusters, each finding its own commit + regression
test where testable, all gated on `go test -race` + `golangci-lint`.

- **Cluster A (data-model/migration)**: C1, C2, H1, M6, M7, M9, L1, L4, L7, L11
  fixed. Both Criticals carry reproducing regression tests.
- **Cluster B (sync/drive)**: H2, H3, H4, M1, M2, M3, M4, M5, M8, L2, L3, L5, L8
  fixed. Testable ones covered by fake-drive engine tests; UI-only ones
  verified by build + reasoning.
- **Lint**: the two pre-existing gosec items were G703 (data-dir path taint) —
  excluded with justification alongside the existing G304; the tree is now
  fully lint-clean.
- **Not fixed (deliberate)**: L6 (md5 nolint rationale is correct as-is), L9
  (documented behavior, not a bug), L10 (folded into M4).
- **Still unverifiable locally** (unchanged from the "Not verified" section
  below): live Drive sync end-to-end, real Keychain paths, `gomobile bind`,
  and a live-click UI pass — all require credentials/devices this environment
  lacks. The sync fixes are covered by the fake-drive engine tests, `-race`,
  and code reasoning.

Result: 268 tests pass (11 packages, no races), build/vet/lint all clean.

---

Date: 2026-07-14. Scope: full codebase after the four-way integration
(recurring-tasks base + #-tag/migration ports + Drive sync/gomobile merge).
Reviewed read-only in `.claude/worktrees/review`. Findings are ordered by
severity; work top-down. Reproductions for the top three findings were run
via `go test -overlay` against real store code (no repo files were modified).

## Build / test / lint / vuln results (all with `CGO_CXXFLAGS="-std=c++20"`)

| Command | Result |
|---|---|
| `go build ./...` | PASS (exit 0) |
| `go vet ./...` | PASS (no findings) |
| `go test -race ./...` | PASS — 250 tests in 11 packages, no races |
| `golangci-lint run` | 3 issues (see below) — pre-existing classes |
| `govulncheck ./...` | 0 vulnerabilities in called code; 1 in a required module not reachable from this code |

golangci-lint details (all pre-existing patterns, nothing new-in-kind from the merge):
- `gosec` on `internal/store/migrate.go` (`os.WriteFile(dst, data, 0o600)` in `copyFileIfExists`)
- `gosec` on `internal/store/store.go` (`os.WriteFile(tmp, ...)` in `writeFile`)
- `gofmt` on `internal/store/projects_test.go` (stray blank line at `TestSlugify`, visible above it at projects_test.go:274)

---

## Critical

### C1. The @→# ref-tag migration silently kills recurring templates whose project tag is the last token
**Files**: `internal/store/migrate_reftags.go:139-155` (rewrite), `internal/recur/recur.go:71-86` (parser), `internal/store/migrate_reftags.go:89` (recurring.md is in scope).
**Confirmed by reproduction.** A pre-migration `recurring.md` line
`- [ ] Water plants @daily @home` (an order explicitly supported by
`recur.Parse`, see `TestParseIDTagOrderIndependent`) is rewritten by
`MigrateRefTags` to `Water plants @daily #home`. `recur.Parse` scans the
trailing token run right-to-left and **breaks on any token that doesn't start
with `@`** (recur.go:73: `if !strings.HasPrefix(tok, "@") { break }`), so the
`#home` token stops the scan before `@daily` is seen → `ok=false` →
`RecurringTasks` drops the template (`recurring.go:100-102`). Reproduced:
1 recurring task before migration, **0 after**. The template line still sits
in recurring.md but never fires, never materializes, and doesn't show in the
Recurring tree section — silent behavior-data loss on first startup after
upgrade. The same parser gap also means the *documented canonical* `#slug`
syntax (commit 2324434) cannot be combined with recurrence tags in
tag-last order when typed by hand into the add box.
**Fix (root cause)**: teach `recur.Parse` to treat `#<body>` the same as an
ID token: on `strings.HasPrefix(tok, "#")` with `isID(body)` true, keep the
token and continue scanning (mirror of the existing `@`+isID branch). Add a
regression test with `"X @daily #proj"` and `"X #proj @daily"`. As
belt-and-braces, `migrateLineRefTag` could skip lines that carry a
recurrence keyword anywhere in the trailing run, but fixing the parser is
the correct fix since `#` is now the canonical tag everywhere else.

### C2. Evening check-in "next day" / "backlog" on a parent task orphans its subtasks and reparents them under an unrelated task
**Files**: `internal/store/store.go:344-396` (`ApplyEvening`), contrast with `internal/store/store.go:516-540` (`PostponeToNextDay`, which does this correctly) and `store.go:626-650` (`MoveToBacklog`).
**Confirmed by reproduction.** `ApplyEvening` removes a NextDay/Backlog item
from `kept` as a *single item*, not as a subtree. Its children (separate
plan items with their own decisions, shown indented in the dialog —
`dialogs.go:319-334`) stay behind at depth 1+. On the next parse,
`Daily.parseBody`'s depth clamp (`daily.go:106-114`) silently reattaches
them **under the preceding unrelated task**. Reproduced: plan
`[other task, parent, ↳child]`, evening decision `parent → NextDay` yields
today = `[other task, ↳child-of-parent]` (child now a subtask of
"other task") and tomorrow = `[parent]` (childless). This is the same class
of bug the tree actions already fixed via `extractSubtree`
(tree.go:182-197's doc comment even names it); the evening dialog was ported
to the subtask model for display but not for the re-homing actions —
a textbook integration seam.
**Fix**: in `ApplyEvening`, apply NextDay/Backlog to the whole subtree:
collect the subtree spans first (via `subtreeSpan`) and use
`extractSubtree`/`appendSubtree` for NextDay (as `PostponeToNextDay` does)
and enqueue each subtree item for Backlog (as `MoveToBacklog` does). Decide
and document what a child's own decision means when its parent moves.
Regression test: parent-with-child + NextDay and + Backlog.

---

## High

### H1. A project whose slug collides with a recurrence keyword hijacks `@daily`/`@weekly`/`@mon`/… and the migration then destroys the recurrence permanently
**Files**: `internal/store/projects.go:133-165` (`slugify`/`uniqueSlug`, no reserved words), `internal/recur/recur.go:77-80` (isID wins over recurrence), `internal/store/migrate_reftags.go:139-155`.
**Confirmed by reproduction.** Create a project named "Daily" (slug `daily`,
per `slugify`); every template `X @daily` is now parsed as a *project ref*
(`recur.Parse` checks `isID` before `consume`), so nothing recurs; and
`MigrateRefTags` rewrites `- [ ] Vitamins @daily` → `- [ ] Vitamins #daily`,
which stays broken even after the project is deleted/renamed (reproduced:
0 recurring tasks after migration, file permanently retagged). Same applies
to `weekly`, `monthly`, `weekday(s)`, `mon`..`sunday`, and numeric slugs
1-31 collide with the month-day token (a project named "15" is possible via
an explicit `id:` line). The existing test
`TestStore_RecurringProjectSlugShapedLikeToken` covers `mon` for *parsing
priority* but nobody guards creation or migration.
**Fix**: reserve the recurrence vocabulary in `ensureID`/`AddProject`
(reject or suffix `daily`, `weekly`, `monthly`, `weekday`, `weekdays`, all
weekday names/abbrevs, pure integers 1-31, and `HH:MM` shapes), and make
`migrateLineRefTag` refuse to rewrite a candidate that is a recurrence
keyword. Validate hand-edited `id:` fields in `LoadProjects` the same way.

### H2. Sync engine can clobber a concurrent local edit, and its local writes are not atomic
**Files**: `internal/sync/sync.go:385-391` (`writeLocal` = plain `os.WriteFile`), `sync.go:103-148` (`Run`), `sync.go:288-298` (`pull`), `internal/ui/gsync.go:94-121` (background goroutine).
Two related problems on the app's highest-value data:
1. **TOCTOU**: `Run` computes local md5s in `scanLocal`, then downloads
   later. If the user (UI main thread, or an editor — README explicitly
   invites editing files while the app runs) modifies a file between the
   scan and the `actDownload` write, `pull` overwrites the *new* local
   content with the remote version. The edit is silently lost; the
   conflict-copy machinery never sees it because the decision was made
   against the stale hash.
2. **Non-atomic write**: `writeLocal` writes in place. A crash/power-loss
   mid-pull leaves a truncated daily file. The store's own `writeFile`
   (store.go:970-982) does tmp+rename precisely to avoid this; the sync
   engine skipped the pattern.
**Fix**: in `pull` (and `resolveConflictCopy`'s canonical rewrite), re-hash
the file immediately before overwriting; if it no longer matches the scanned
md5, reclassify as conflict instead of downloading. Use tmp+rename in
`writeLocal` (mind the `.tmp` suffix interaction with H4 — prefer a
dot-prefixed temp name, which `scanLocal` already skips).

### H3. Background sync failures pop a modal error dialog every 5 minutes (e.g. whenever offline)
**Files**: `internal/ui/gsync.go:107-113` (`runSync` → `a.reportError`), `gsync.go:20` (`syncInterval = 5m`), `internal/ui/app.go:703-710` (`reportError` = modal `QMessageBox_Critical`).
With auto-sync enabled, any `engine.Run` error — including plain network
unavailability — raises a modal critical dialog. On a laptop that's offline
for an evening that is a dialog every 5 minutes; it also sets `dialogOpen`
via `reportError`, suppressing scheduled check-ins while it sits there. This
makes the feature effectively unusable offline.
**Fix**: for timer-triggered runs, log + show a passive tray message (or a
status item in Preferences) with backoff; reserve the modal for the
user-initiated "Sync now" button. Distinguish auth errors (token revoked →
one actionable notification) from transient network errors (silent/backoff).

### H4. `scanLocal` picks up the store's transient `*.tmp` files → garbage uploads and spurious sync errors
**Files**: `internal/sync/sync.go:345-374` (`scanLocal` skips only dotfiles), `internal/store/store.go:974` (`tmp := path + ".tmp"`).
The store's atomic saves briefly create `daily/.../2026-07-14.md.tmp` next
to the real file. A concurrent `Run` (background goroutine, 5-min timer,
while the UI writes on the main thread) can (a) list the `.tmp` file and
upload it to Drive as a real file, where it then syncs to other devices and
lingers until a later delete-reconciliation, or (b) `os.ReadFile` it after
the rename already happened → `Run` returns an error → modal dialog (H3).
The race window is small per-save but the app saves on every checkbox
click, and the sync scan walks the whole tree.
**Fix**: skip `*.tmp` (or allowlist `*.md` + known JSON) in `scanLocal`;
with H2's fix, also switch the engine's own temp naming to a dot-prefix so
the two never overlap.

---

## Medium

### M1. OAuth token refresh persistence failures are swallowed
**File**: `internal/drive/oauth.go:48-58` (`savingSource.Token`: `_ = s.store.Save(tok)`).
If the Keychain write fails (locked keychain, denied prompt), the refreshed
token is used for the session but never persisted; after the old refresh
token ages out the user is silently signed out with no breadcrumb. Also
`s.last` is read/written without synchronization — currently safe because
`Engine.Run` serializes requests, but it's a latent race if the client is
ever used concurrently.
**Fix**: at minimum `slog.Warn` on save failure; guard `last` with a mutex
or drop the dedup and always save.

### M2. Keychain writes pass the full token JSON as a process argument
**File**: `internal/drive/keychain_darwin.go:44-45` (`security add-generic-password ... -w <json>`).
The access+refresh token appear in the process argument list while
`security` runs; argv is visible to other processes of the same user (and
historically via `ps` more broadly). Short-lived, local-only, but this is
the app's crown-jewel secret and macOS provides better paths.
**Fix**: use the Security framework via cgo or a keychain library, or run
`security -i` and feed the command via stdin so the secret never hits argv.
Related nit: `Load` (line 24-35) collapses every failure into
"no stored Google token", so a locked keychain reads as signed-out and
auto-sync silently stays off — distinguish "not found" from other errors.

### M3. Tree row actions use indices that go stale when sync (or an editor) rewrites the plan
**Files**: `internal/ui/tree.go:223-283` (closures capture `date, index`), `internal/ui/mainwindow.go:286-295` (`runTaskAction`; its comment claims the index "is always current"), `internal/ui/gsync.go:114` (refresh happens only *after* a sync run completes).
The comment's invariant held when only the UI mutated files. Now a
background Drive pull can rewrite the viewed day's file at any moment; the
tree refresh is scheduled only after the whole run finishes, so there's a
real window where a click on "Delete"/"Next week"/checkbox applies to
whatever item now sits at that index — the wrong task can be deleted. Same
exposure existed for hand-editing, but sync makes it routine.
**Fix**: `runTaskAction` should pass the row's cached display text (already
stored in `taskTextRole`) and verify it against
`DisplayText(d.Plan[index])` inside the store op (or a thin wrapper),
aborting with a refresh on mismatch. Cheap, and turns silent misfires into
no-ops.

### M4. Every sync entry point builds its own `Engine`; the engine mutex therefore serializes nothing
**Files**: `internal/ui/gsync.go:100-105` (new engine per `runSync`), `internal/ui/conflicts.go:22` (separate `NewLocal` engine for `Resolve`), `mobilecore/core.go:112-145` (new engine per call).
`Engine.mu` only guards a single instance. The UI's `a.syncing` flag
serializes timer/manual runs (main-thread only — OK), but `Resolve` from the
conflicts dialog runs on a *different* engine instance and can rewrite the
canonical file / delete the conflict copy while a background `Run` is
mid-scan/push, yielding surprising uploads or a re-recorded conflict. Same
for mobilecore if the host app calls concurrently.
**Fix**: hold one long-lived `Engine` per `App` (create at sign-in/startup,
swap the Drive client on re-auth) and route `Resolve` through it; document
the concurrency contract in mobilecore (or reuse one engine there too).

### M5. Changing the data folder in Preferences bypasses migrations and sync state
**File**: `internal/ui/preferences.go:161` (`a.store.DataDir = reloaded.DataDir`).
The store's invariants are established in `store.New` (story→project
migration) plus `MigrateRefTags` in `main.go:42-48`. Mutating `DataDir` in
place points the running store at a directory that never went through
either migration, and the next background sync run scans the new directory
against `.sync-state.json` state that may not exist there (everything
re-classified as new; with files on Drive this mass-creates conflict
copies).
**Fix**: rebuild the store via `store.New(newDir)` + `MigrateRefTags`,
stop/re-arm the sync timer, and surface errors in the dialog instead of
silently repointing.

### M6. `materializeOne` falls back on *any* error, not just "project not found"
**File**: `internal/store/recurring.go:272-279`.
`if err := s.AddTaggedTask(...); err == nil { return nil }` discards the
error and retries untagged. A genuine I/O error gets a second blind write
attempt; a validation error silently drops the project tag. Classic silent
fallback.
**Fix**: return a typed `ErrProjectNotFound` from `AddTaggedTask` and only
fall back on `errors.Is`; propagate everything else.

### M7. `EditTaskText` mangles a user-typed project tag
**File**: `internal/store/store.go:465-487`.
The edit dialog pre-fills the *display* text (tag stripped), and
`EditTaskText` re-appends the old tag. If the user types a new tag
themselves ("fix login **#payments**") the result is
`fix login #payments #oldproj`: `splitProjectTag` only reads the last token,
so the task silently stays in the old project and the literal "#payments"
pollutes the display text.
**Fix**: run `splitProjectTag` on `newText` first; if the user supplied a
known tag, it wins; otherwise re-append the old one. (Also update the stale
"@project tag" wording in the doc comment — the code writes `#`.)

### M8. `.sync-state.json` / `.conflicts.json` are written non-atomically
**File**: `internal/sync/sync.go:421-427, 512-521` (plain `os.WriteFile`).
A crash mid-write corrupts the state file; the next `Run` fails at
`loadState` json parse (`sync.go:412-414`) and sync is dead until the user
manually deletes the file (with H3 turning that into repeated modal
dialogs). Losing the file entirely would be self-healing (files re-classify
as already-equal); a *truncated* file is not.
**Fix**: reuse the store's tmp+rename pattern; on parse failure, consider
quarantining the bad state file (rename to `.sync-state.json.bad`) and
starting fresh instead of hard-failing, since the classifier converges.

### M9. No test exercises the two startup migrations in sequence — the seam that holds C1
**Files**: `internal/store/migrate_test.go`, `internal/store/migrate_reftags_test.go`, `cmd/daily-progress-logger/main.go:42-48`, `mobilecore/core.go:36-45`.
Story→project migration *writes legacy `@` tags* (`migrate.go:200`:
`" @" + storyToProject[storyID]`) that only become canonical `#` because
`MigrateRefTags` runs afterwards. Nothing tests: (a) `store.New` +
`MigrateRefTags` back-to-back on a legacy story dataset, (b) either
migration over `recurring.md` templates (which would have caught C1),
(c) re-running both on already-migrated data *with* mixed content. Also
untested: sync conflict flow through the UI-facing `Resolve`+`Run` loop on
markdown that the store then re-parses.
**Fix**: add an integration test fixture with stories + @refs + recurring
templates + recycle/backlog, run `New` then `MigrateRefTags` twice, assert
full round-trip and that `RecurringTasks` count is preserved.

---

## Low / Nits

- **L1** `internal/ui/mainwindow.go:15` — stale doc comment: "Projects → Stories → tasks tree"; stories no longer exist (integration debt from the story removal).
- **L2** `internal/ui/gsync.go:188-197` — after clicking "Sign in with Google" the button/status update immediately, while sign-in completes asynchronously; the dialog shows "Not signed in" until reopened, and there is no in-progress indication.
- **L3** `internal/ui/gsync.go:144-147` — account email interpolated into a RichText label without `html.EscapeString` (value comes from Google's userinfo; harmless today, one line to fix).
- **L4** `internal/ui/shortcuts.go:71-90` — closure parameter named `now` actually receives the *viewed date* from `currentTask()`; misleading during maintenance.
- **L5** `internal/drive/drive.go:156-161` — Drive query built with Go `%q` escaping; a folder segment containing `"` or `\` would produce an invalid/incorrect query. All current segments are app-generated (`daily/YYYY/MM`), so latent only.
- **L6** `internal/sync/sync.go:14` (md5) — fine for Drive change detection (Drive supplies `md5Checksum`; not a security context) and covered by the `.golangci.yml` G401/G501 exclusions from the merge; keep the nolint rationale.
- **L7** `internal/store/migrate_reftags.go:141` — `migrateLineRefTag` only recognizes a space (`' '`) separator; a tab-separated trailing tag is skipped (parser side `strings.Fields` would accept it). Cosmetic inconsistency; a skipped line still parses via the legacy `@` path.
- **L8** `internal/ui/dialogs.go:484` — `_ = a.store.RegenerateWeekly(week)` before opening the weekly file: on error the button opens a stale/missing file with no message.
- **L9** `internal/store/store.go:437-459` (`SetPlanItemState`) — the un-postpone backlog cleanup removes by full item text *including* the `#tag`; consistent with how `PostponePlanItem` adds it, but any hand-added tagless backlog duplicate survives. Behavior, not bug; noting for awareness.
- **L10** Per-run engine construction (`gsync.go:102`) re-does `drive.New` → `ensureFolder` list query every 5 minutes; harmless but wasteful — folds into M4's long-lived engine.
- **L11** `cmd/daily-progress-logger/main.go:46-47` — a failed `MigrateRefTags` is only `slog.Warn`. Acceptable *because* `splitProjectTag` still accepts legacy `@` (projects.go:259-287) and per-file failures are skipped inside the migration; worth a comment there tying the two together so the fallback isn't removed later.

---

## Verified fine (checked deliberately; no need to re-litigate)

- **Migration ordering & idempotency**: `store.New` runs story→project first (store.go:37-43), `MigrateRefTags` second in both entry points (main.go:42-48, mobilecore/core.go:36-45). Both are idempotent by construction (story migration keys off `### ` headings; ref-tag migration keys off remaining legacy tags) and re-run safely — verified by tests and by reading the guards. Backups: `.pre-subtasks-backup` is built under a `.tmp` sibling and atomically renamed (migrate.go:218-237, never overwrites an existing backup); `.pre-hashtag-backup` is taken before any rewrite and skipped once present (migrate_reftags.go:48-54). Both migrations write files via the atomic `writeFile`.
- **NUL-marker guard** (item.go:67-78): unknown state renders as `' '` (todo), never a zero byte; covered by `TestItemRenderUnknownStateNeverEmitsNUL`.
- **Subtask Depth model round-trip**: 2-space indent parse/render round-trips (`TestDailyRoundTripNestedPlan`), depth clamp on corrupted indents (`TestDailyParseBodyNormalizesDepth`), and all tree ops (`MakeSubtask`, `ReorderTask`, `MoveTaskToProject`, delete/postpone/backlog) move whole subtrees with cycle guards — logic in nesting.go/tree.go is careful and well-tested (subtree_ops_test.go). C2 is the *one* remaining path that bypasses `extractSubtree`.
- **Tag parsing edge cases**: `splitProjectTag` accepts `#`/legacy `@`, requires a known ID (so `#hashtag` prose and `@mentions` survive), last-token-only (projects_test.go:103). `recur.Parse` correctly keeps `@<knownid>` tokens in either order and refuses ID-shaped day/weekday tokens (recur_test.go:61-110) — the gap is only the `#` prefix (C1).
- **Closed-project fallback**: tasks tagged to closed/unknown projects land in Unfiled instead of vanishing (tree.go:75-92; `TestStore_BuildProjectTreeExcludesClosed` asserts the fallback explicitly).
- **Weekly cross-day dedup & done-bullet stripping**: `DoneByDay` first-day-wins dedup and `parseDoneBullet` checkbox stripping match their tests (`TestDoneByDayCrossDay`, `TestParseDoneBullet`, `TestWeeklyRenderDedupDoneAndPlan`).
- **Recurring materialization**: per-(template, day) state file prevents re-adding deleted occurrences; past dates are no-ops; crash-between-write-and-state is handled via `AddTaggedTask`/`AddPlanItem` dedup (recurring.go:220-267, well-tested).
- **Sync classifier**: three-way md5 classification (`decidePath`/`decideBoth`/…) is sound for the serial case — pull-before-push, upload only when remote still matches base, conflict copies keep both versions on both sides; state carried forward correctly. Verified by reading and by the 5 engine tests (create/edit/delete convergence, nested paths, no blind overwrite, conflict both-versions, resolve-keep-remote). The issues are all *environmental* (H2/H4/M4/M8), not the algorithm.
- **Re-exec crash workaround** (main.go:112-142): runs first in `main`, preserves argv (so `-hidden` from the login item survives), filters only `GODEBUG`, respects a user-set `asyncpreemptoff=`, degrades gracefully if `os.Executable`/`Exec` fails, and cannot loop (the env var check short-circuits the second pass). Login-item plist interaction checked (loginitem.go:46-62; plist has no GODEBUG but re-exec supplies it).
- **Qt threading**: all store access from UI happens on the main thread; goroutines (sync, update check, sign-in) marshal back via `mainthread.Start`; `a.syncing`/`dialogOpen` are main-thread-only. `-race` suite passes. Hover-reveal + drag-reorder: drop-indicator row widget is forgotten before `tree.Clear()` (tree.go:33-39) so no dangling QWidget; drag uses cursor-mapped coordinates with a documented rationale; deferred `scheduleRefresh` avoids destroying the sender mid-signal (mainwindow.go:166-171).
- **Checkbox vs postponed state**: parent checkboxes are disabled (roll-up only), leaf toggling maps only to Todo/Done so a postponed item's `'>'` state can be left by unchecking (checked=false → StateTodo) — no path writes an unknown marker; postponed styling is distinct (tree.go:350-361).
- **OAuth flow**: loopback + PKCE + state check, no client secret, `drive.file` least-privilege scope plus non-sensitive email scope; token exchange errors propagate (loopback.go).
- **Config**: fail-loud on malformed config, validates before save, shortcut-conflict detection, `~` expansion (config.go).

## Not verified (be honest about the limits)

- **Live Drive sync end-to-end** — needs real OAuth credentials and two devices; only the fake-drive engine tests and code reading back the behavior.
- **Keychain/`security` behavior** — not exercised against a real keychain (locked-keychain paths in particular).
- **Real-click UI verification** — screenshots/offscreen rendering exist, but no live interaction pass was run from this worktree (matches the existing open entry in `known-issues.md`).
- **gomobile binding** — `mobilecore` compiles and its store calls were reviewed, but no `gomobile bind` was run and no iOS host exercised it.
- **The SIGURG crash itself** — the workaround's logic was verified by reading; reproducing the original Qt/SIGURG crash was not attempted.
