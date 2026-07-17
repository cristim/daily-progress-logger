# Adversarial review: mobilecore expansion (9 -> ~45 methods)

Reviewed at HEAD `31526f9` (detached worktree `.claude/worktrees/mcore-review`).
Scope: `mobilecore/*.go`, `internal/store/schedule_state.go`, `internal/store/weekly.go` (WeekFlags),
compared against the Qt reference (`internal/ui`) and `internal/store`.

## Build / test / lint status

| Check | Result |
|---|---|
| `CGO_CXXFLAGS="-std=c++20" go build ./...` | PASS |
| `go test -race ./mobilecore/... ./internal/store/...` | PASS (217 tests) |
| `golangci-lint run ./mobilecore/... ./internal/store/...` | PASS (no issues) |
| `CGO_ENABLED=0 GOOS=linux go build ./mobilecore` | PASS (no darwin leakage) |

Severity counts: **1 Critical, 5 High, 8 Medium, 7 Low**.

Ranking legend: **[HOST-BLOCKER]** = must fix before the iOS/Android hosts hardcode
against it; **[LATER]** = can fix after host work starts without breaking contracts.

---

## Critical

### C1. TreeJSON leaks the internal `store.ProjectTree` structs verbatim — an unstable, inconsistent contract [HOST-BLOCKER]

`mobilecore/tree.go:28` does `toJSON(tree)` on `*store.ProjectTree` directly.
`ProjectTree` / `TreeProject` / `TreeTask` / `RecurringTask` (`internal/store/tree.go:17-45`,
`internal/store/recurring.go:32-37`) carry **no json tags**, so the wire format is whatever
Go reflection produces from internal structs:

- **PascalCase keys** (`"Projects"`, `"Unfiled"`, `"Index"`, `"Text"`, `"State"`, `"Children"`)
  while every other endpoint uses snake_case (`"next_week"`, `"from_backlog"`, `"done_by_day"`).
  The tests confirm the split (`core_test.go:55` `task["Text"]` vs `core_test.go:174` `bl["next_week"]`).
- **`State` is a bare int** (`ItemState`: 0=todo, 1=done, 2=postponed — `internal/store/item.go:9-18`),
  while `RecycleJSON` emits `"todo"/"done"/"postponed"` strings for the same enum
  (`mobilecore/recycle.go:28` via `stateString`). Two encodings of one enum in one API.
- **`Date` is a full `time.Time`** -> `"2026-07-20T00:00:00+03:00"` (local-midnight RFC3339 with
  device offset), while every other date in the API is `"YYYY-MM-DD"`. Round-tripping this back
  into `SetTaskState(date, ...)` requires the host to reformat, and the offset changes with DST/travel.
- **Nil slices serialize as `null`, not `[]`** (`Projects`, `Unfiled`, `Recycled`, `Children`),
  while BacklogJSON/WeekReviewCandidatesJSON deliberately normalize to `[]`
  (`mobilecore/backlog.go:17-22`, `weekly.go:86-88`). Hosts must special-case null only here.
- **`ProjectTree.Recurring` leaks `recur.Recurrence` internals** (`Kind`, `Weekday`, `MonthDay`,
  `Hour`, `Minute` as raw ints — `internal/recur/recur.go:30-37`), duplicating `RecurringJSON`
  (`mobilecore/recurring.go`) with different casing and shape. Any rename of an internal store
  field is now a silent mobile API break, invisible to the compiler and to the store's tests.

**Why it matters**: this is the single most-consumed payload (the main screen of both apps).
Once two hosts hardcode `"Unfiled"`, `State==1`, and RFC3339 dates, the contract is frozen around
an accident of reflection.

**Fix**: mirror the pattern already used everywhere else in mobilecore — define explicit
`treeJSON`/`treeTaskJSON`/`treeProjectJSON` DTOs in `mobilecore/tree.go` with snake_case tags,
`state` as the `"todo"/"done"/"postponed"` wire string (reuse `stateString`), `date` as
`"YYYY-MM-DD"`, `[]` for empty, and either drop `Recurring` (RecurringJSON exists) or map it to
the `recurringTaskJSON` DTO. This decouples the wire format from `internal/store` forever.

---

## High

### H1. `ResolveConflict` with an unknown choice string silently discards the conflict record [HOST-BLOCKER]

`mobilecore/sync.go:48-54` casts the host string straight into the enum:
`engine.Resolve(path, syncengine.ResolveChoice(choice))` — no validation.
In `internal/sync/sync.go:498-518`, `Engine.Resolve`'s switch has cases for
`KeepLocal`/`KeepRemote`/`KeepBoth` and **no default**; execution falls through to
`saveConflicts(remaining)` at line 517, which **clears the conflict record having taken no
file action** — i.e. any typo (`"keep-local"`, `"local"`, `"KEEP_LOCAL"`) behaves like
`keep_both` and permanently dismisses the conflict. This is a silent data-decision on a
sync-conflict path, exactly the fail-open class the codebase rules forbid.

**Fix**: validate at the boundary in mobilecore (switch on the three wire strings, return
`fmt.Errorf("unknown choice %q (want keep_local/keep_remote/keep_both)")`) **and** add a
`default: return error` arm in `Engine.Resolve` so no future caller can repeat this.
Also note `Resolve` on an unknown `path` returns `nil` (sync.go:494-496) — a host acting on a
stale conflict list gets success for a no-op; consider returning a not-found error.

### H2. No machine-readable error contract across the gomobile boundary [HOST-BLOCKER]

gomobile surfaces Go errors as `NSError`/`Exception` carrying only the message string.
The API defines meaningful sentinels — `ErrCASMismatch` (`core.go:45`),
`store.ErrProjectNotFound` (`projects.go:347`), `store.ErrBacklogItemNotFound`
(`store.go:645`) — but a Swift/Kotlin host **cannot `errors.Is`**; it can only substring-match
prose like `"task text mismatch: tree is stale, please refresh"`. The CAS-refresh loop is the
core UX flow of the addressing model: the host *must* reliably detect ErrCASMismatch to know
"refresh tree and retry" vs "show error toast". A wording tweak breaks both apps silently.

**Fix**: prefix each sentinel with a stable, documented code the host can prefix-match, e.g.
`"CAS_MISMATCH: ..."`, `"PROJECT_NOT_FOUND: ..."`, `"BACKLOG_ITEM_NOT_FOUND: ..."` — or add
`LastErrorCode()`-style plumbing / JSON-envelope results. Whatever the choice, document it in
the package doc as a frozen contract before host code is written.

### H3. Refreshed OAuth tokens are silently lost — and the loss is invisible to the host [HOST-BLOCKER]

`memTokenStore.Save` is a documented no-op (`core.go:119-122`), so when
`drive.savingSource` refreshes an expired access token mid-call (`internal/drive/oauth.go:51-72`)
the new token is dropped. Consequences for a mobile host:

1. Every `SyncNow` after access-token expiry burns an extra refresh round-trip (the host keeps
   supplying the stale access token) — battery/latency cost, tolerable.
2. **If Google rotates the refresh token** (testing-mode OAuth consent, security events,
   granular-consent clients), the *new* refresh token is discarded and the host's stored one is
   dead. Next sync fails permanently until re-auth. The host has no way to know a refresh even
   happened.

Neither `core.go` nor `sync.go` documents this for the host, and there is no API to retrieve
the post-call token.

**Fix**: make `SyncNow` return an envelope `{"conflicts": [...], "token": {...}}` (or add
`LastTokenJSON() string`) populated from the engine's token source after the run, and document
"persist this back to Keychain after every sync". Cheap now; a wire-format break later.

### H4. `SyncWithFileToken` is documented but does not exist; `FileTokenStore` is unusable from a mobile host [HOST-BLOCKER]

`mobilecore/sync.go:15` tells the host to "call SyncWithFileToken if file-based persistence is
wanted" — grep confirms **no such method exists** anywhere in the repo. Meanwhile
`FileTokenStore`'s exported methods take/return `*oauth2.Token` (`tokenstore.go:40,56`), which
is not a gomobile-supported type: `gomobile bind` skips those methods, so from Swift/Kotlin the
type is an opaque handle with a constructor and nothing callable, and **no Core method accepts
a token store**. The type is currently reachable only from Go callers (the stated CLI use case),
yet it lives in the gomobile-bound package with mobile-facing docs.

**Fix**: either implement `SyncWithFileToken()` (build the engine over a `FileTokenStore` at
`TokenFilePath(c.dir)`) or delete the dangling doc reference and move `FileTokenStore` out of
the bound package (e.g. into `internal/drive`) so the bound surface only contains usable API.

### H5. `verifyIndex` fails open on `KnownProjectIDs` error — on destructive paths [LATER, but decide now]

`core.go:79-84`: when the projects file cannot be read, the CAS guard is skipped entirely
(`return nil`) and the action proceeds on a possibly stale index. This deliberately mirrors Qt
`taskIndexValid` (`internal/ui/mainwindow.go:311-315`) — faithful porting, verified. But the
threat model differs on mobile: background Drive sync rewrites files far more often than on
desktop, and the fail-open path guards `DeleteTask`, `PostponeToNextWeek`, `MoveTaskToBacklog`
— data-mutation ops where acting on the wrong index destroys/moves the wrong task, with only
the store's bounds check (`recycle.go:99`) left standing. On a data-mutation path the
CLAUDE.md fail-loud rule says fail closed.

**Fix**: split the guard: reads of `KnownProjectIDs` failing -> fall back to comparing **raw**
item text (tag included) instead of skipping verification, or fail closed (`ErrCASMismatch`)
for destructive ops (`DeleteTask`, `PurgeRecycled`, postpones, moves) while keeping fail-open
for benign ones (`SetTaskState`). Either preserves the Qt contract's spirit without the
delete-the-wrong-task window. Decide before hosts encode retry semantics.

---

## Medium

### M1. The CAS check is advisory, not atomic (TOCTOU), and defeated by duplicate texts

`verifyIndex` reads the daily file (`core.go:85`), returns, and then the store op re-reads it
(`loadOrNewDaily`) — two independent reads with no lock. A concurrent writer (desktop GUI, CLI,
Drive sync — all doing atomic tmp+rename writes, `store.go:1031-1043`) landing between the two
reads yields verify-against-old-file, act-on-new-file. Additionally the text comparison cannot
distinguish same-text items: `AddPlanItem`/`AddTaggedTask` dedupe top-level texts, but
`EditTaskText` and same-text subtasks under different parents legitimately produce duplicates,
so a shifted index that lands on an identical-text sibling passes the guard and mutates the
wrong node. Survivable (the window is milliseconds and same-text mutations are usually the
user's intent), but it should be an explicit documented limitation.

**Fix (cheap)**: document "CAS is best-effort, single-writer-per-moment assumed". **Fix (right)**:
push `expectedText` down into the store ops so load-verify-mutate-save is one read-modify-write
(the store already re-reads anyway — verification there costs nothing and closes the window
within a single process).

### M2. Thread-safety contract exists only as a doc comment — add the mutex

`core.go:13-15` says "the host must not call methods concurrently". gomobile explicitly allows
calls from any thread, and iOS/Android hosts will inevitably call `TreeJSON` from a refresh
hook while a user action runs. `*Core`/`*store.Store` are stateless read-modify-write over
files; interleaved calls mean lost updates (last save wins), not crashes — which makes
violations silent. A single `sync.Mutex` around each Core method serializes everything at
negligible cost and turns the doc contract into a guarantee. Note the current package doc also
overpromises: "SyncNow while ResolveConflict is in flight is a misuse" — with a mutex it
becomes safe. Do this before host teams build their own (differing) serialization.

### M3. `config.Load()` couples the mobile core to desktop config, with write side effects on read paths

`ConfigJSON`, `SetConfig` (`config.go:13-23,45-73`) and `DuePromptsJSON` (`schedule.go:45`) all
call `config.Load()`, which (a) resolves `os.UserConfigDir()` — depends on `HOME`/XDG being set,
which gomobile on Android does not guarantee; (b) **creates the config file with desktop
defaults on first read** (`internal/config/config.go:149-157`) — a data dir under `$HOME`, Qt
`Ctrl+…` shortcuts — i.e. a nominally read-only call (`ConfigJSON`, even `DuePromptsJSON`)
writes desktop-shaped state inside the app sandbox; and (c) yields a `data_dir` field the Core
ignores entirely (the host passed `dataDir` to `Open`) — two sources of truth a host could
plausibly hardcode against. **Fix**: store mobile-relevant settings in a small file under the
Core's own `dataDir` (which also makes check-in times sync via Drive like everything else), or
at minimum strip `data_dir`/`shortcuts` from `ConfigJSON` output and document that `Open`'s
`dataDir` is authoritative.

### M4. `DuePromptsJSON` silently swallows config *parse* errors, not just absence

`schedule.go:45-58`: any `config.Load()` or `ParseTimeOfDay` failure falls back to
09:30/17:30/Fri-17:00 defaults with no signal. First-run-no-file -> defaults is right; but a
*corrupt or invalid* config (user set evening to 18:30 on desktop; file syncs over broken) means
prompts silently fire at the wrong times, with no way for the host to notice. Distinguish
`os.ErrNotExist` (defaults, fine) from parse/validation errors (return the error, or include a
`"config_fallback": true` field in the response so the host can surface it).

### M5. `SyncNow`/`ConflictsJSON` return `"null"` for zero conflicts

`sync.go:30,43`: `res.Conflicts`/`engine.Conflicts()` are nil when empty ->
`toJSON` yields the string `null`, while sibling endpoints normalize nils to `[]`
(backlog.go:17-22, weekly.go:86-88, checkin.go:29). A Kotlin host doing
`Json.decodeFromString<List<Conflict>>` throws on `null`. Normalize to `[]` like the others —
two lines, but a wire-contract fix, so do it before hosts ship.

### M6. `ReorderTask` cycle requests silently succeed

`internal/store/nesting.go:100-102`: reordering a task relative to its own descendant returns
`nil` without changing anything ("silently ignored... rather than corrupting the plan"). Via
mobile, the host gets success, the UI optimistically shows the new order, and the next
`TreeJSON` snaps back — an unexplainable ghost revert on a touch-drag UI. `MakeSubtask` in the
same file returns an explicit error for the same condition (`nesting.go:51-53`); make
`ReorderTask` consistent with it.

### M7. FileTokenStore tests are test theater; failure-path coverage has real gaps

- `TestFileTokenStore_SaveLoad`/`_AtomicWrite` (`core_test.go:572-617`) **never call
  `store.Save()`** — they write the file manually with `os.WriteFile` (line 588, 611) and even
  `_ = store // silence unused` (line 616). The atomic tmp+rename and the 0600 mode of `Save`
  — the two properties the test names claim — are unexercised. (Manually verified correct by
  reading `tokenstore.go:56-72`, but the tests prove nothing.)
- No malformed-JSON tests for any inbound decision payload (`ApplyMorning`, `ApplyEvening`,
  `ApplyWeekReview`, `SetWeeklyPlan`, `SetConfig`), no invalid-action-code (`action: 7`) or
  invalid-state-string tests, no unknown-project test for `AddTask`/`MoveTaskToProject`, no
  `ResolveConflict` choice-validation test (which would have caught H1), no test that a
  subtask CAS mismatch after a parent move is caught.
- CAS coverage is happy-path-plus-one: wrong-text yes; stale-index-after-sync-rewrite,
  duplicate-text, and fail-open (unreadable projects file) paths untested.

These are exactly the paths a host will hit in week one. Add them before host development so
contract regressions fail CI, not the apps.

### M8. `TestDuePromptsJSON` is timezone-dependent

`core_test.go:547` builds `now` as `today() + "T09:35:00Z"` (UTC), and `DuePromptsJSON`
converts to `time.Local` (`schedule.go:42`). In any zone west of UTC (e.g. `TZ=America/New_York`,
09:35Z = 04:35 local) the morning threshold (09:30 local) is not reached and the
`assert.Contains(names, "morning check-in")` fails. It passes today only because CI runners and
the dev machine sit at UTC or east of it. Use an explicit local-offset timestamp or set a fixed
`TZ` for the test.

---

## Low

- **L1. Empty/whitespace `expectedText` disables the CAS guard silently** (`core.go:76-78`).
  Documented, and Qt does the same (`mainwindow.go:308-310`), but for a host it is a one-string
  footgun: forget to thread the text through and every action runs unguarded with no warning.
  Consider logging a debug line, and state loudly in the method docs that hosts must always
  pass the text from the last TreeJSON.
- **L2. `MorningCandidatesJSON` returns raw text including `#project` tags**
  (`store.Candidate.Text` is raw, `store.go:180,193`) while TreeJSON/RecycleJSON strip tags.
  Qt's dialog shows the raw text too (`dialogs.go:255-259`), so it is faithful — but the
  asymmetry ("some endpoints stripped, some raw") must be documented or hosts will
  double-strip or display tags inconsistently.
- **L3. `SetWeeklyPlan(date, "null")`** unmarshals to a nil slice and calls
  `store.SetWeeklyPlan(week, [])`, which **marks the week planned with zero goals** — a host
  serialization bug silently erases the weekly plan and suppresses the weekly-plan prompt.
  Reject `null`/empty input explicitly.
- **L4. `RestoreTask`/`PurgeRecycled` on a missing entry are silent no-ops**
  (`recycle.go:35,45` doc'd) — a stale recycle view purges nothing and reports success.
  Acceptable, but worth documenting as intentional in the JSON contract doc.
- **L5. `stateString` default arm maps unknown states to `"todo"`** (`core.go:167-168`) —
  unreachable today, but a silent-fallback pattern; a `panic`/error is more in line with
  fail-loud since it can only mean a new enum value was added without updating the wire mapping.
- **L6. `AddTask` of a duplicate text is a silent no-op** (`store.AddPlanItem`/`AddTaggedTask`
  dedupe via `hasPlanItem`) — success returned, nothing added; host UIs should be told so they
  refresh rather than assume an append.
- **L7. `verifyIndex` trims `expectedText` before comparing; Qt compares exact**
  (`core.go:92` vs `mainwindow.go:322`). Harmless divergence (store texts are stored trimmed),
  noted for fidelity.

---

## Verified fine

- **scheduleState extraction is faithful**: `internal/store/schedule_state.go:16-51` is a
  field-for-field port of Qt `App.scheduleState` (`internal/ui/app.go:382-412`), including the
  `SummaryPendingPastWeek` past-week comparison. Qt intentionally keeps its own copy for now
  (documented in the extraction).
- **Config-time fallbacks match config defaults exactly**: 09:30 / 17:30 / Friday / 17:00
  (`mobilecore/schedule.go:75-80` vs `internal/config/config.go:21-24`).
- **`WeekFlags` is correct**: `internal/store/weekly.go:128-134` reads frontmatter via
  `loadWeeklyMeta`, returns `false,false,nil` for a missing file (`store.go:741-743`) and a
  real error on parse failure — no silent fallback.
- **Prompt ID mapping is stable and documented**: `duePromptJSON` IDs match the
  `schedule.Prompt` iota order 0..4 (`internal/schedule/schedule.go:10-22`), names via
  `Prompt.String()`.
- **gomobile type safety of the Core surface**: every exported `Core` method uses only
  string/int/bool/error returns and params; `Open` returns `(*Core, error)`; `int` indices are
  64-bit on both bound platforms (Java `long`, Swift `Int`) so no 32-bit index overflow. The
  only bind-hostile exports are the `FileTokenStore` methods (H4).
- **Portability**: `CGO_ENABLED=0 GOOS=linux go build ./mobilecore` passes; the darwin Keychain
  code is isolated in `internal/drive/keychain_darwin.go` and not referenced by mobilecore;
  CI has a dedicated guard (`.github/workflows/mobile.yml:57`).
- **Store-level index safety**: every wrapped store op re-checks bounds itself
  (`SetPlanItemState` store.go:494, `DeleteTask` recycle.go:99, `ReorderTask`/`MakeSubtask`/
  `AddSubtask` nesting.go, `MoveTaskToProject` nesting.go:152), and subtree ops move whole
  subtrees via `extractSubtree`/`subtreeSpan` so children travel with parents.
- **Unknown-project validation**: `AddTaggedTask` returns `ErrProjectNotFound`
  (projects.go:352-364); `MoveTaskToProject` errors on unknown IDs (nesting.go:159-161).
- **Atomic writes everywhere**: store `writeFile` (store.go:1031-1043), sync state/conflicts
  (`writeFileAtomic`), and `FileTokenStore.Save` all use tmp+rename with restrictive modes.
- **Inbound decision payloads fail loud on malformed JSON**: `ApplyMorning`/`ApplyEvening`/
  `ApplyWeekReview`/`SetWeeklyPlan`/`SetConfig` all return wrapped unmarshal errors and
  validate action codes via exhaustive switches with explicit unknown-value errors
  (checkin.go:112-126, weekly.go:130-140).
- **TreeJSON materialization matches Qt**: `MaterializeRecurring` before building the tree,
  failure logged and non-fatal, same as `materializeViewedDate` — a deliberate, documented
  availability-over-strictness choice consistent with the desktop.

## Suggested fix order before host development starts

1. C1 (TreeJSON DTO) — the largest frozen surface.
2. H2 (error codes) — the CAS retry loop depends on it.
3. H1 (ResolveConflict validation) + M5 (`null` vs `[]`) — sync wire contract.
4. H3/H4 (token return + FileTokenStore cleanup) — auth lifecycle.
5. H5 + M1/M2 (CAS fail-closed decision, store-level verify, Core mutex).
6. M3/M4 (config source of truth) and the M7/M8 test debt.
