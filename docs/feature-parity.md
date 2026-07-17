# Cross-Surface Feature Parity Audit

Date: 2026-07-17 · Branch: `feat/unified`

Surfaces audited:

1. **Qt** — the macOS desktop app (`cmd/daily-progress-logger`, `internal/ui/*`). The reference surface; assumed full-featured.
2. **CLI** — the non-interactive `dpr` subcommands (`cmd/dpr/main.go`).
3. **TUI** — the interactive `dpr tui` tree (`cmd/dpr/tui.go`, tview/tcell).
4. **Mobile core** — the gomobile-bindable Go API (`mobilecore/core.go`, 221 lines). **There is no iOS app and no Android app**: the repository contains no Swift, Kotlin, or Java sources, no Xcode project, and no `ios/` directory; the only mobile artifact is the `ios-core` Makefile target (`Makefile:122-123`, `gomobile bind -target=ios -o ios/Frameworks/Core.xcframework ./mobilecore`), whose output path does not exist yet. "Mobile parity" below therefore audits what the Go core *exposes*, i.e. what a host app could do today without core changes.

## Baseline verification (run 2026-07-17)

| Check | Command | Result |
|---|---|---|
| Build | `CGO_CXXFLAGS="-std=c++20" go build ./...` | ✅ exit 0 |
| Tests | `CGO_CXXFLAGS="-std=c++20" go test -race ./...` | ✅ 324 passed in 12 packages, exit 0 |
| Lint | `golangci-lint run` | ✅ "No issues found", exit 0 |

The store (`internal/store`) is the capability floor: every Qt feature that touches data goes through exported `Store` methods. `internal/schedule`, `internal/config`, `internal/recur`, `internal/drive`, `internal/sync`, `internal/update`, and `internal/loginitem` are all pure-Go (except `drive/keychain_darwin.go`) and reusable by any surface.

## Legend

✅ full · ◐ partial (note in cell) · ❌ absent · n/a (not sensible for that surface by design)

## Parity matrix

### Task ops (daily plan)

| Capability | Qt | CLI | TUI | Mobile core |
|---|---|---|---|---|
| View day's tasks | ✅ project tree (`mainwindow.go:204` refresh → `store.BuildProjectTree`) | ◐ `dpr list` flat numbered, depth-indented; project only in `--json` (`main.go:456`) | ✅ project tree (`tui.go:386` reload) | ✅ `TreeJSON` (`core.go:49`), full `ProjectTree` incl. rollup/recurring/recycled |
| Add task (unfiled) | ✅ add row (`mainwindow.go:329` addItem) | ✅ `dpr add` (`main.go:486`) | ✅ `a` (`tui.go:633` promptAdd) | ✅ `AddTask` (`core.go:63`) |
| Add task to project | ✅ project row "+ Task" (`tree.go:448` addProjectTask) | ✅ `add --project SLUG` | ✅ context-aware `a` on project node (`tui.go:584` resolveAddContext) | ✅ `AddTask(date, text, projectID)` |
| Mark done / not done | ✅ checkbox (`tree.go:239`) + shortcuts `item.done`/`item.todo` (`shortcuts.go:72-77`) | ✅ `done <n>` / `undone <n>` | ✅ Space/`x`/Enter (`tui.go:524` toggleDone) | ✅ `SetTaskState` (`core.go:76`) |
| Edit task text | ✅ double-click / context "Edit…" (`tree.go:471` editTask) | ✅ `edit <n> <text>` (`main.go:591`) | ✅ `e` (`tui.go:681` promptEdit) | ❌ `EditTaskText` not exposed |
| Delete → recycle bin | ✅ hover "Delete" / context menu (`tree.go:275`) | ✅ `rm <n>` (`main.go:616`) | ✅ `d` with confirm modal (`tui.go:546`) | ✅ `DeleteTask` (`core.go:93`) |
| Postpone to next day | ✅ hover/context/shortcut (`tree.go:260`, `shortcuts.go:78`) | ✅ `postpone <n>` (`main.go:643`) | ✅ `p` (`tui.go:734`) | ❌ `PostponeToNextDay` not exposed (`SetTaskState "postponed"` only flips the `[>]` glyph, it does not move the item) |
| Postpone to next week (mark `[>]` + queue next-week backlog) | ✅ (`tree.go:263`, `store.PostponePlanItem`) | ✅ `postpone <n> --week` | ✅ `P`/`w` (`tui.go:749`) | ❌ |
| Move task to backlog | ✅ hover/context/shortcut (`tree.go:266`, `store.MoveToBacklog`) | ✅ `backlog <n>` (`main.go:685`) | ❌ no key bound (`tui.go:328` handleRune has no backlog action) | ❌ |
| Reorder tasks (between siblings) | ✅ drag between rows (`tree.go:729` → `store.ReorderTask`) | ❌ | ❌ | ❌ |
| Move task to another project / Unfiled | ✅ drag onto project/Unfiled (`tree.go:749-757` → `store.MoveTaskToProject`) | ❌ | ❌ | ❌ |

### Subtasks

| Capability | Qt | CLI | TUI | Mobile core |
|---|---|---|---|---|
| Add subtask under a task | ✅ "+ Sub" / context / add-row-on-selection (`tree.go:259`, `mainwindow.go:358`) | ✅ `add --parent N` (`main.go:530`) | ◐ `a` on a task node adds a subtask only when the node already has children (`tui.go:616` addContextForTask); no way to create the *first* subtask of a leaf | ❌ `AddSubtask` not exposed |
| Nest an existing task (MakeSubtask) | ✅ drag onto a task (`tree.go:746` → `store.MakeSubtask`) | ❌ | ❌ | ❌ |
| Rolled-up done display for parents | ✅ disabled checkbox, rollup (`tree.go:236`) | ◐ raw states only; no rollup in `list` output | ✅ `t.Done` rollup (`tui.go:51` stateCheckGlyph) | ✅ rollup included in `TreeJSON` |

### Projects

| Capability | Qt | CLI | TUI | Mobile core |
|---|---|---|---|---|
| List projects | ✅ tree headers | ✅ `dpr projects` (id, name, status) (`main.go:734`) | ◐ tree headers only (open projects with the day's tasks; no status listing) | ✅ in `TreeJSON`; `LoadProjects` itself not exposed |
| Add project | ✅ "New Project…" (`mainwindow.go:376`) | ✅ `dpr project add` (`main.go:763`) | ❌ | ✅ `AddProject` (`core.go:106`) |
| Rename project | ✅ double-click / "Rename" (`tree.go:480` → `store.RenameProject`) | ❌ | ❌ | ❌ |
| Close (archive) project | ✅ "Close" (`tree.go:489` → `store.SetProjectStatus`) | ❌ | ❌ | ❌ |

### Backlog

| Capability | Qt | CLI | TUI | Mobile core |
|---|---|---|---|---|
| View backlog (Current + Next week) | ✅ Backlog dialog (`backlogdialog.go:22`) | ✅ `dpr backlog list` (`main.go:717`) | ❌ | ❌ |
| Adopt backlog item into today's plan | ✅ "Plan Today" button (`backlogdialog.go:130` → `store.AdoptFromBacklog`) | ❌ | ❌ | ❌ |
| Shuttle item Current ↔ Next week | ✅ move buttons (`backlogdialog.go:147,157` → `store.MoveBacklogItem`) | ❌ | ❌ | ❌ |

### Check-ins / weekly flows

| Capability | Qt | CLI | TUI | Mobile core |
|---|---|---|---|---|
| Morning check-in (carry-over candidates + new plan) | ✅ (`dialogs.go:212` buildMorningDialog → `store.MorningCandidates`/`ApplyMorning`) | ❌ | ❌ | ❌ |
| Evening check-in (per-item triage + extra done) | ✅ (`dialogs.go:295` → `store.ApplyEvening`) | ❌ | ❌ | ❌ |
| Weekly plan (set/tick "big things") | ✅ (`dialogs.go:113` → `store.WeeklyPlan`/`SetWeeklyPlan`) | ❌ | ❌ | ❌ |
| Week review (triage leftovers: keep/postpone/drop) | ✅ incl. oldest-first loop (`app.go:497`, `dialogs.go:367` → `store.ApplyWeekReview`) | ❌ | ❌ | ❌ |
| Weekly summary (done-by-day view, mark summarized, open weekly file) | ✅ (`dialogs.go:432` → `store.DailiesInWeek`/`DoneByDay`/`MarkWeekSummarized`/`RegenerateWeekly`) | ❌ | ❌ | ❌ |
| Prompt scheduling (due/snooze/skip/oneshot cron mode) | ✅ (`app.go:302` CheckPrompts, `schedule` pkg; `-checkin`/`-prompt-due` flags in `cmd/daily-progress-logger/main.go:26-29`) | n/a (interactive by nature; a "what's due" query would be S) | ❌ | n/a (host schedules local notifications) |

### Recurring tasks

| Capability | Qt | CLI | TUI | Mobile core |
|---|---|---|---|---|
| List templates | ✅ Recurring section (`tree.go:56`) | ✅ `dpr recur list` (`main.go:790`) | ✅ read-only section (`tui.go:461`) | ✅ in `TreeJSON` (`ProjectTree.Recurring`) |
| Add template (`@daily`/`@weekly`… parsing) | ✅ add-row detects recur tags (`mainwindow.go:342` → `recur.Parse` + `store.AddRecurring`) | ❌ `dpr add "x @daily"` stores literal text as a plan item | ❌ same | ❌ |
| Delete template | ✅ per-row "Delete" (`tree.go:88` → `store.RemoveRecurring`) | ❌ | ❌ | ❌ |
| Materialize today's occurrences | ✅ on view/date-change/midnight (`mainwindow.go:240`, `app.go:311`) | ❌ **never calls `store.MaterializeRecurring`** — a CLI-only user's recurring tasks never appear | ❌ same | ❌ same |
| Recurring reminders (notifications) | ✅ tray balloons (`app.go:776` fireRecurring → `store.RecurringDue`) | n/a | n/a | n/a (host notification job; needs `RecurringDue` exposed) |

### Recycle bin

| Capability | Qt | CLI | TUI | Mobile core |
|---|---|---|---|---|
| View deleted items | ✅ Recycle Bin section (`tree.go:138`) | ❌ | ✅ read-only section (`tui.go:478`) | ✅ in `TreeJSON` (`ProjectTree.Recycled`) |
| Restore to its day | ✅ (`tree.go:180` → `store.RestoreTask`) | ❌ | ❌ | ❌ |
| Purge permanently | ✅ (`tree.go:186` → `store.PurgeRecycled`) | ❌ | ❌ | ❌ |

### Google Drive sync

| Capability | Qt | CLI | TUI | Mobile core |
|---|---|---|---|---|
| Sign in (OAuth loopback) / sign out | ✅ Preferences Drive section (`gsync.go:63` signInGoogle → `drive.SignIn`) | ❌ (loopback flow opens a browser — feasible from a terminal, see gap list) | ❌ | n/a by design — host does Google Sign-In and passes token JSON (`core.go:110-112`, `memTokenStore` `core.go:194`) |
| Sync now | ✅ button + first-run (`gsync.go:110`) | ❌ | ❌ | ✅ `SyncNow` (`core.go:112`) |
| Automatic background sync | ✅ 5-min timer (`gsync.go:223` startSyncTimer) | n/a (cron a future `dpr sync`) | ❌ | n/a (host background task calls `SyncNow`) |
| List / resolve conflicts | ✅ Conflicts dialog (`conflicts.go:24`) | ❌ | ❌ | ✅ `ConflictsJSON` / `ResolveConflict` (`core.go:125,139`) |
| Sync-error surfacing with dedup | ✅ (`gsync.go:200` handleSyncError) | n/a | n/a | ◐ errors returned as Go errors; host must render |

### Notifications

| Capability | Qt | CLI | TUI | Mobile core |
|---|---|---|---|---|
| Check-in notification banners (click-to-open) | ✅ (`app.go:327-336`, tray `OnMessageClicked` `app.go:272`) | n/a | n/a | ❌ core exposes no "what's due" query; host would need `schedule` logic exposed |
| Backlog-move / adopt confirmations | ✅ tray balloons (`app.go:763,798`) | ◐ prints confirmation lines | ◐ footer status line | n/a |
| Notify vs modal toggle | ✅ Preferences (`preferences.go:78`, `cfg.NotifyCheckins`) | n/a | n/a | n/a |

### Preferences / config

| Capability | Qt | CLI | TUI | Mobile core |
|---|---|---|---|---|
| Edit check-in times, summary day/time | ✅ Preferences (`preferences.go:47`) | ◐ hand-edit config JSON only (`config.Load` shared) | ❌ | ❌ (`Open` takes only dataDir/clientID/deviceID `core.go:36`) |
| Data dir selection (with migration on change) | ✅ (`preferences.go:179` rebuilds store + sync engine) | ◐ `--data-dir` per-invocation override (`main.go:257`) | ◐ same flag | ◐ fixed at `Open` |
| Keyboard shortcut editor (14 actions, `config.ShortcutActions`) | ✅ (`preferences.go:84-101`, `shortcuts.go:24`) | n/a | ◐ hardcoded keys (`tui.go:328`) | n/a |
| Google client ID entry | ✅ (`gsync.go:269`) | ◐ config file only | ❌ | ✅ `Open` parameter |
| Login item (start at login) | ✅ one-time offer (`app.go:179` MaybeOfferLoginItem) | n/a | n/a | n/a |
| Auto-update check | ✅ 24 h + manual (`app.go:847,877`) | n/a | n/a | n/a |

### Navigation / window

| Capability | Qt | CLI | TUI | Mobile core |
|---|---|---|---|---|
| Date navigation | ✅ prev/next day, calendar popup, Today (`mainwindow.go:59-77`) | ✅ `--date YYYY-MM-DD` | ◐ `[`/`]`/`t` day-wise only; `nextWeekDay`/`prevWeekDay` helpers exist (`tui.go:240,243`) but are **not bound to any key** | ✅ every call takes a date string |
| Expand/collapse with state memory | ✅ per-key across rebuilds (`tree.go:431` expandedOr) | n/a | ◐ expand/collapse works; state resets on reload except re-selected task | n/a (host concern) |
| Open data folder | ✅ File menu (`mainwindow.go:198`) | n/a | n/a | n/a |
| Menu-bar residency, tray menu, window toggle, reopen-on-dock-click | ✅ (`app.go:242` setUpTray, `shortcuts.go:104`) | n/a | n/a | n/a |
| Offscreen screenshot mode | ✅ `-screenshot` (`app.go:684`) | n/a | n/a | n/a |

## Summary counts

Counting the 40 Qt-reference capability rows above (excluding cells marked n/a for a surface):

| Surface | ✅ full | ◐ partial | ❌ absent | n/a |
|---|---|---|---|---|
| Qt (reference) | 40 | 0 | 0 | — |
| CLI | 14 | 6 | 17 | 3 |
| TUI | 10 | 5 | 22 | 3 |
| Mobile core | 10 | 2 | 19 | 9 |

## Gap lists (prioritized per surface)

Unless noted, every gap is a **wiring job**: the store method already exists and is unit-tested; the surface just needs a subcommand/keybinding/exported method.

### CLI (`cmd/dpr/main.go`)

1. **Recurring tasks never materialize** — a CLI-only workflow silently drops recurring tasks. Fix: call `store.MaterializeRecurring(date)` in `run()` (or at least in `cmdList`/`cmdTUI`) the way Qt does in `materializeViewedDate` (`mainwindow.go:240`). Store: `MaterializeRecurring` (`recurring.go:220`). Effort: **S**. High priority — correctness, not convenience.
2. **Backlog adopt + shuttle** — `dpr backlog plan <n>` / `dpr backlog move <n>`. Store: `AdoptFromBacklog` (`store.go:608`), `MoveBacklogItem` (`store.go:649`). Effort: **S**.
3. **Recur add/rm** — `dpr recur add "Standup @weekly @mon @9:00"` / `dpr recur rm <n>`. Store: `AddRecurring` (`recurring.go:111`), `RemoveRecurring` (`recurring.go:137`); parse validation via `recur.Parse`. Effort: **S**.
4. **Recycle bin** — `dpr recycle list|restore <n>|purge <n>`. Store: `LoadRecycleBin` (`recycle.go:24`), `RestoreTask` (`recycle.go:124`), `PurgeRecycled` (`recycle.go:143`). Effort: **S**.
5. **Project rename/close** — `dpr project rename <id> <name>` / `dpr project close <id>`. Store: `RenameProject` (`projects.go:259`), `SetProjectStatus` (`projects.go:277`). Effort: **S**.
6. **Weekly plan / summary read-outs** — `dpr plan [--set]`, `dpr summary`. Store: `WeeklyPlan`/`SetWeeklyPlan` (`store.go:936,948`), `DailiesInWeek` + `DoneByDay` (`store.go:127`, `weekly.go:24`), `RegenerateWeekly` (`store.go:717`). Effort: **M** (output formatting).
7. **Task reorder / reparent / retag** — `dpr mv` variants. Store: `ReorderTask` (`nesting.go:89`), `MakeSubtask` (`nesting.go:39`), `MoveTaskToProject` (`nesting.go:147`). Effort: **S** each.
8. **Sync** — `dpr sync [--login]`. `drive.SignIn`'s loopback flow opens a browser, which works fine launched from a terminal on the same machine, so it is *not* inherently surface-inappropriate; a headless/SSH session is. Caveat: token persistence is `drive.KeychainStore`, implemented only for darwin (`internal/drive/keychain_darwin.go`); a portable CLI needs a file-based `TokenStore` implementation (the interface already exists — `mobilecore/core.go:194` shows a trivial one). Effort: **M**.
9. **Morning/evening check-in, week review** — n/a by design as non-interactive commands (they are triage conversations). Sensible only as TUI screens (below). A read-only `dpr due` (print which prompts are due, for scripting) would be **S** using `schedule.Due` + the `scheduleState` logic from `app.go:382`.
10. Notifications, preferences editing, login item, auto-update: **n/a by design**.

### TUI (`cmd/dpr/tui.go`)

1. **Move-to-backlog key** — the CLI has it, the TUI doesn't; `b` → `store.MoveToBacklog`. Effort: **S**.
2. **Materialize recurring on load/date-change** — same correctness gap as CLI #1; call `MaterializeRecurring` in `reload()`/`changeDay()`. Effort: **S**.
3. **Recycle-bin actions** — restore/purge on `kindRecycled` nodes (currently display-only, `tui.go:478-491`). Store: `RestoreTask`, `PurgeRecycled`. Effort: **S**.
4. **Recurring-template actions** — delete on `kindRecurring` nodes; recur-tag detection in the add prompt (reuse `recur.Parse` exactly as `mainwindow.go:342`). Effort: **S**.
5. **Subtask on a leaf** — `resolveAddContext` (`tui.go:584`) only creates subtasks under branch nodes; add an explicit "add subtask" key (e.g. `A`) calling `store.AddSubtask`. Effort: **S**.
6. **Week-jump keys** — `{`/`}` bound to the already-written-but-unbound `prevWeekDay`/`nextWeekDay` (`tui.go:240,243`). Effort: **S** (dead helpers today).
7. **Project add/rename/close** — keys on `kindProject`/root. Store: `AddProject`, `RenameProject`, `SetProjectStatus`. Effort: **S-M**.
8. **Backlog viewer/adopt screen** — a modal list mirroring `backlogdialog.go`. Store: `LoadBacklog`, `AdoptFromBacklog`, `MoveBacklogItem`. Effort: **M**.
9. **Reorder / reparent / retag** — keyboard equivalents of Qt's drag-drop (`ReorderTask`, `MakeSubtask`, `MoveTaskToProject`). Effort: **M**.
10. **Check-in / weekly-plan / review / summary screens** — all store logic exists (`MorningCandidates`/`ApplyMorning`, `ApplyEvening`, `WeeklyPlan`/`SetWeeklyPlan`, `WeekReviewCandidates`/`ApplyWeekReview`, `DoneByDay`/`MarkWeekSummarized`); each is a tview form. Effort: **M-L** (the largest sensible TUI investment).
11. Sync/conflicts: plausible (**M**, same keychain caveat as CLI) but arguably better left to the CLI/Qt. Notifications/scheduling: **n/a by design** (TUI runs only while open).

### Mobile core (`mobilecore/core.go`)

**(a) Exposed today vs the full set.** Exposed (9 methods): `Open`, `TreeJSON`, `AddTask` (incl. project tag), `SetTaskState` (todo/done/postponed glyph only), `DeleteTask`, `AddProject`, `SyncNow`, `ConflictsJSON`, `ResolveConflict`. Verified missing: subtasks (`AddSubtask`/`MakeSubtask`), edit (`EditTaskText`), postpone-to-next-day / postpone-to-next-week / move-to-backlog (`PostponeToNextDay`/`PostponePlanItem`/`MoveToBacklog`), all backlog reads/ops, weekly plan/review/summary, morning/evening check-in flows, recurring (list is only implicit via `TreeJSON`; no add/remove/materialize/due), recycle restore/purge, project rename/close, reorder, preferences, notifications, any date navigation beyond passing a `"YYYY-MM-DD"` string. All of these except notifications/preferences are S-effort wiring onto existing store methods.

Prioritized core additions (all wiring):

1. **`MaterializeRecurring(date)`** — same correctness gap as CLI/TUI: a mobile host today would never surface recurring tasks. Effort: **S**.
2. **`EditTask`, `PostponeNextDay`, `PostponeNextWeek`, `MoveToBacklog`** — the daily-driver quartet; without them a mobile app is read-and-check-only. Effort: **S** each.
3. **`AddSubtask`** and backlog read/adopt/move (`BacklogJSON`, `AdoptFromBacklog`, `MoveBacklogItem`). Effort: **S**.
4. **Check-in flows as JSON** — `MorningCandidatesJSON`/`ApplyMorningJSON`, `ApplyEveningJSON`, weekly plan/review/summary equivalents; the store's decision types (`Candidate`, `EveningDecision`, `ReviewDecision`) marshal cleanly. Effort: **M** (API design, not logic).
5. **`RecurringDueJSON` + schedule-due query** — so the host can schedule local notifications; `internal/schedule` is pure Go and could be bound too. Effort: **S-M**.
6. Recycle restore/purge, project rename/close. Effort: **S**.

**(b) No app host exists.** The gap to a usable mobile app is not core wiring but an entire SwiftUI/Compose application: UI, Google Sign-In (the core deliberately delegates it — `memTokenStore`, `core.go:192-197`), Keychain token persistence, background sync scheduling, local notifications, and an Xcode/Gradle project. That is a build-from-scratch **L/XL** project; the core additions above are the cheap part.

**(c) Is the core API shape sufficient?** Adequate for a v1 read-mostly app; **not sufficient for a full app without one redesign**:

- **Text-based task identification is fragile.** `SetTaskState`/`DeleteTask` resolve tasks via `findByDisplayText` (`core.go:149-168`), which returns the *first* plan item whose display text matches — two tasks with the same text (entirely legal; the store dedups only in check-in flows) make the second unaddressable and the first silently absorb both actions. Every other surface uses the plan index (Qt with a stale-text guard, `mainwindow.go:291` `runTaskAction`; CLI/TUI directly). Fix: expose index-based ops mirroring `TreeJSON`'s `Index` field, plus a compare-and-swap guard (expected display text, exactly Qt's M3 pattern) for sync races. Effort: **M**, and worth doing *before* any host app hardcodes the text-based API.
- The store has no per-item stable IDs at all (markdown lines addressed by index), which is fine for synchronous local surfaces but means a mobile app must re-fetch `TreeJSON` after every mutation and between syncs. Acceptable contract; document it.
- JSON-string-in/string-out is the right gomobile shape (avoids binding complex types); keep it.
- The per-call fresh sync engine's concurrency contract ("one call at a time from the host", `core.go:170-174`) is documented but unenforced; a serializing wrapper inside the core would remove a host footgun. Effort: **S**.

## True store gaps (new logic, not wiring)

The store covers everything the Qt surface does; nothing a CLI/TUI needs is missing from it. The only genuinely new work identified:

1. **Portable OAuth token store** — `drive.KeychainStore` is darwin-only (`internal/drive/keychain_darwin.go`); Linux/Windows CLI sync needs a new `TokenStore` implementation (interface exists). **S-M**.
2. **Stable/guarded task addressing for async hosts** — the index+expected-text CAS pattern lives in the Qt layer (`mainwindow.go:291-323`), not the store; promoting it into `internal/store` (or `mobilecore`) so all surfaces share it is new (small) logic. **M**.
3. **Schedule state for non-Qt surfaces** — `App.scheduleState` (`app.go:382`) composes store queries into `schedule.State`; a shared helper would need extracting before CLI `dpr due` or a mobile notification scheduler can reuse it. **S**.
