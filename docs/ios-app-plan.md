# iOS App Architecture Plan

Date: 2026-07-17 · Branch: `feat/unified` · Status: planning (no app code exists yet)

The iOS app is a native SwiftUI host over the shared Go core (`mobilecore`,
bound via gomobile into `ios/Frameworks/Core.xcframework`). The Qt desktop app
(`internal/ui/*`) is the feature reference; `docs/feature-parity.md` enumerates
the 40 reference capabilities and `docs/mobilecore-review.md` audits the core.

**Contract assumption**: this plan targets the *finalized* mobilecore JSON
contract (in flight at the time of writing), not the exact bytes the current
`mobilecore/*.go` emit. The target contract is:

- Explicit DTOs with **snake_case** keys everywhere (including TreeJSON, which
  today leaks PascalCase internals - review item C1).
- Task/item state is the **string enum** `"todo"` / `"done"` / `"postponed"`.
- All dates are **`"YYYY-MM-DD"`** strings (TreeJSON's `date` included).
- Empty collections serialize as **`[]`, never `null`**.
- Errors carry **stable machine-readable prefixes** the host prefix-matches:
  `"CAS_MISMATCH:"`, `"NOT_FOUND:"`, `"BAD_INPUT:"`, `"SYNC_AUTH:"` (review
  item H2).
- Task actions take **`(date, index, expectedText)`** with a compare-and-swap
  guard; `CAS_MISMATCH` means "refresh TreeJSON and re-present".
- **`SyncNow` returns an envelope** `{"conflicts": [...], "token": {...}}`;
  the host must persist the returned token back to the Keychain after every
  sync (review item H3 - Google may rotate the refresh token).

If the shipped contract diverges from this, the Swift decoding layer
(section 2.4) is the single place to adjust.

---

## 1. Scope for v1

### v1 (ships first)

The daily loop plus every flow that is pure UI over an existing Core method.
All of these are wiring: the Go side is done and tested.

| Area | Features | Core methods |
|---|---|---|
| Tree / daily loop | Project → task → subtask tree with rollup done states; Unfiled section; date navigation (prev/next/today/date picker); add task (unfiled or to project); add subtask; check/uncheck; edit text; delete to recycle; postpone to next day; postpone to next week; move to backlog; assign/unassign project | `TreeJSON`, `AddTask`, `AddSubtask`, `SetTaskState`, `EditTaskText`, `DeleteTask`, `PostponeToNextDay`, `PostponeToNextWeek`, `MoveTaskToBacklog`, `MoveTaskToProject` |
| Morning check-in | Carry-over candidates (week + backlog, backlog default-unchecked exactly like Qt `buildMorningDialog`), weekly goals shown read-only, free-text new items | `MorningCandidatesJSON`, `ApplyMorning`, `WeeklyPlanJSON` |
| Evening check-in | Per-plan-item triage (todo/done/next-day/next-week/backlog), extra-done free text | `TreeJSON` (plan items), `ApplyEvening` |
| Weekly | Weekly plan (goals with done toggles + add more); week review triage (keep/postpone/drop, oldest-unreviewed-first loop like Qt `runWeekReviewLoop`); weekly summary (goals + done-by-day, mark summarized) | `WeeklyPlanJSON`, `SetWeeklyPlan`, `WeekReviewCandidatesJSON`, `ApplyWeekReview`, `UnreviewedWeekJSON`, `WeeklySummaryJSON`, `MarkWeekSummarized`, `WeeklySummaryPendingJSON` |
| Backlog | Current + Next-week lists; adopt into today; shuttle between lists | `BacklogJSON`, `AdoptFromBacklog`, `MoveBacklogItem` |
| Recurring | List templates; add (`@daily` / `@weekly @mon @9:00` syntax with inline validation error from core); delete | `RecurringJSON`, `AddRecurring`, `RemoveRecurring` (materialization happens inside `TreeJSON`) |
| Recycle bin | List; restore to original day; purge | `RecycleJSON`, `RestoreTask`, `PurgeRecycled` |
| Projects | List (open + closed); add; rename; close; reopen | `ProjectsJSON`, `AddProject`, `RenameProject`, `CloseProject`, `ReopenProject` |
| Settings | Check-in times, summary day/time, Google client ID; notify toggle | `ConfigJSON`, `SetConfig` |
| **Sync (foreground)** | Google Sign-In (ASWebAuthenticationSession + PKCE), Keychain token storage, manual "Sync now", auto-sync on app foreground/activation, conflicts list + resolve | `SyncNow`, `ConflictsJSON`, `ResolveConflict` |
| Check-in prompting (foreground) | On launch/foreground, ask the core what is due and route to the right check-in screen | `DuePromptsJSON` |
| **Local notifications (simple)** | Static `UNCalendarNotificationTrigger` reminders at the configured morning/evening times and summary day/time; tapping deep-links to the check-in screen | none (host-side; times from `ConfigJSON`) |

**Sync is v1** because the core makes it cheap (the host only does OAuth +
Keychain + calling `SyncNow`), and because without it the iOS app is an island:
the whole point of a phone companion is seeing the same data as the desktop.
Scoped to *foreground* sync: a sync runs on app activation and on demand.
That covers the realistic usage pattern (open app → fresh data) without any
BGTaskScheduler complexity.

**Local notifications are v1 in their dumb form** because check-in reminders
are the product's core behavior (Qt fires them via tray balloons,
`app.go:327-336`) and time-based `UNCalendarNotificationTrigger`s are a small,
self-contained amount of work. The v1 notification is *unconditional* (fires
at 09:30 even if the morning check-in was already done on desktop); tapping it
opens the app, which calls `DuePromptsJSON` and either shows the check-in or a
"nothing due" state. Suppression requires background execution - deferred.

### v2 (explicitly deferred)

| Feature | Why deferred |
|---|---|
| **Background sync** (`BGTaskScheduler` / `BGAppRefreshTask`) | Real work: opportunistic scheduling, iOS budget heuristics, token refresh in background, testing is painful (can't force-run without a debugger). Foreground sync covers the UX until then. |
| **Smart notification suppression** (only notify when `DuePromptsJSON` says due) | Depends on background refresh to re-evaluate due state before firing; ship after background sync exists, by replacing static triggers with a BGTask that schedules/cancels one-shot notifications. |
| Drag-based reorder / re-nest (`ReorderTask`, `MakeSubtask`) | Qt does this with drag & drop; a touch UI needs `List` edit-mode plus double-CAS handling and `ReorderTask`'s silent-cycle quirk (review M6) fixed first. The daily loop does not depend on it. |
| Snooze/skip semantics for prompts (Qt's `dialogResult` snooze-1h / skip-today) | Qt keeps snooze state in-process; on iOS this becomes notification re-scheduling. Fold into the v2 notification work. |
| iPad multi-column layout, widgets, Watch, Shortcuts/App Intents | Nice-to-haves after the phone app is proven. |
| Auto-update, login item, tray/menu-bar behaviors, keyboard shortcut editor, "open data folder" | Desktop-only concepts; n/a on iOS (Files-app export can substitute for "open data folder" later via `UIFileSharingEnabled`). |

---

## 2. Tech choices

### 2.1 SwiftUI, iOS 17 floor

- **SwiftUI app lifecycle**, no UIKit scenes. Xcode 26.3 ships the iOS 26 SDK;
  a sane floor is **iOS 17.0**: it unlocks the `@Observable` macro,
  `NavigationStack`/`navigationDestination`, `ContentUnavailableView`, and
  covers every device that matters in 2026. Nothing in this app needs a newer
  API; raise the floor later if an iOS 18+ API becomes attractive.
- Swift 6 language mode with strict concurrency (the CoreClient actor design
  below is what makes this clean).

### 2.2 Architecture: MVVM with `@Observable` stores over an actor-isolated core

Three layers:

```
SwiftUI Views  →  @Observable feature stores (view models)  →  CoreClient (actor)  →  Core.xcframework (Go)
```

- **`CoreClient`** is a Swift `actor` owning the single `MobilecoreCore`
  handle. Every Core call goes through it, which (a) serializes all calls -
  matching the core's "one call at a time" contract even before/despite the
  core-side mutex (review M2), and (b) keeps Go calls off the main thread
  (gomobile calls are synchronous file I/O; an actor hop makes them
  `await`-able for free).
- **Feature stores** (`@Observable` classes, `@MainActor`): `TodayStore`,
  `CheckinStore`, `WeekStore`, `BacklogStore`, `RecurringStore`,
  `RecycleStore`, `ProjectsStore`, `SyncStore`, `SettingsStore`. Each owns the
  decoded model for its screen, exposes async intents (`func toggle(task:)`),
  and refreshes itself after mutations (section 3).
- **`AppState`** (root `@Observable`): the opened `CoreClient`, the viewed
  date, sign-in status, pending-prompt routing, and a `dataVersion` counter
  bumped after any mutation or sync so sibling screens know to refresh.

No third-party dependencies for v1. Google Sign-In is done with
`ASWebAuthenticationSession` directly (section 5.2) rather than the
GoogleSignIn SDK - the core's Drive layer expects a plain `oauth2.Token` JSON,
which the raw token endpoint response maps to trivially, and it avoids a heavy
SDK for one flow.

### 2.3 Consuming Core.xcframework

- `make ios-core` produces `ios/Frameworks/Core.xcframework` (device arm64 +
  simulator arm64/x86_64 slices). The app target links it as **Embed & Sign**.
- `import Core` in Swift exposes the gobind ObjC API. Shapes (from the
  generated `Mobilecore.objc.h`):
  - Constructor: `MobilecoreOpen(dataDir, clientID, deviceID, &error) -> MobilecoreCore?`,
    imported to Swift as `try MobilecoreOpen(dataDir, clientID, deviceID)`.
  - `(string, error)` methods: `- (NSString* _Nonnull)treeJSON:(NSString*)date error:(NSError**)err`
    → Swift `func treeJSON(_ date: String?) throws -> String`.
  - `error`-only methods: `- (BOOL)addTask:... error:` → Swift
    `func addTask(_ date: String?, text: String?, projectID: String?) throws`.
  - All parameters import as `String?` (gobind marks them `_Nullable`); the
    CoreClient wrapper takes non-optional Swift types and never passes nil.
- **Interop quirk to verify at scaffold time** (also in section 8): gomobile
  marks string returns `_Nonnull` while also taking `NSError**`. Current
  Xcode imports these as `throws -> String`, but older gomobile output has
  been imported as non-throwing with a separate error pointer. The scaffold
  task includes a smoke test asserting `try core.treeJSON("bogus")` actually
  throws; if it does not, CoreClient falls back to explicit
  `NSErrorPointer` plumbing in one place.

### 2.4 Swift decoding layer (`CoreKit` group)

One `JSONDecoder` configured with `.convertFromSnakeCase` **not** used -
models declare explicit `CodingKeys` matching the wire names so the contract
is visible and greppable in one file per DTO. Models mirroring the target
contract:

```swift
// Tree (TreeJSON) - the main-screen payload
struct ProjectTree: Codable {
    var projects: [TreeProject]        // "projects"
    var unfiled: [TreeTask]            // "unfiled"
    var recycled: [TreeTask]           // "recycled"
    var recurring: [RecurringTemplate] // "recurring" (same DTO as RecurringJSON)
}
struct TreeProject: Codable, Identifiable {
    var id: String                     // "id"
    var name: String                   // "name"
    var done: Bool                     // "done" (global strike-through state)
    var tasks: [TreeTask]              // "tasks"
}
struct TreeTask: Codable, Identifiable {
    var index: Int                     // "index" - stable plan-file index, the action address
    var depth: Int                     // "depth"
    var text: String                   // "text" - display text, project tag stripped; the CAS expectedText
    var state: ItemState               // "state" - "todo"|"done"|"postponed"
    var date: String                   // "date" - "YYYY-MM-DD"
    var done: Bool                     // "done" - rolled-up display state
    var project: String                // "project" - display name (recycle entries)
    var children: [TreeTask]           // "children"
    var id: String { "\(date)#\(index)" }
}
enum ItemState: String, Codable { case todo, done, postponed }

// Backlog (BacklogJSON)
struct Backlog: Codable { var current: [String]; var nextWeek: [String] }        // "current", "next_week"

// Check-ins
struct MorningCandidate: Codable { var text: String; var fromBacklog: Bool }     // "text", "from_backlog"
struct MorningDecisions: Codable { var newItems: [String]; var adopted: [MorningCandidate] } // "new_items", "adopted"
struct EveningDecisions: Codable { var decisions: [EveningItemDecision]; var extraDone: [String] }
struct EveningItemDecision: Codable { var text: String; var action: EveningAction }
enum EveningAction: Int, Codable { case todo = 0, done = 1, nextDay = 2, nextWeek = 3, backlog = 4 }

// Weekly
struct WeeklyGoal: Codable { var text: String; var done: Bool }
struct WeeklyPlan: Codable { var week: String; var planned: Bool; var goals: [WeeklyGoal] }
struct WeekReviewCandidates: Codable { var week: String; var candidates: [String] }
struct WeekReviewDecisions: Codable { var decisions: [ReviewItemDecision]; var rollover: Bool }
struct ReviewItemDecision: Codable { var text: String; var action: ReviewAction }
enum ReviewAction: Int, Codable { case keep = 0, postpone = 1, drop = 2 }
struct WeeklySummary: Codable {                                                  // "done_by_day" etc.
    var week: String; var start: String; var end: String
    var summarized: Bool; var reviewed: Bool
    var goals: [WeeklyGoal]; var doneByDay: [DayDone]
}
struct DayDone: Codable { var date: String; var items: [String] }
struct PendingWeek: Codable { var pending: Bool; var week: String? }             // WeeklySummaryPendingJSON / UnreviewedWeekJSON

// Projects / recurring / recycle
struct Project: Codable, Identifiable { var id: String; var name: String; var status: ProjectStatus }
enum ProjectStatus: String, Codable { case open, closed }
struct RecurringTemplate: Codable { var text: String; var project: String; var raw: String } // "raw" is the RemoveRecurring key
struct RecycleEntry: Codable { var date: String; var text: String; var state: ItemState }

// Schedule / prompts (DuePromptsJSON)
struct DuePrompts: Codable { var due: [DuePrompt] }
struct DuePrompt: Codable { var id: PromptID; var name: String }
enum PromptID: Int, Codable { case weekReview = 0, weeklyPlan = 1, morning = 2, evening = 3, weeklySummary = 4 }

// Sync
struct SyncEnvelope: Codable { var conflicts: [SyncConflict]; var token: OAuthToken } // SyncNow return
struct SyncConflict: Codable { var path: String; var conflictCopy: String; var time: String } // "conflict_copy"
struct OAuthToken: Codable {   // oauth2.Token wire form; round-trips Keychain <-> SyncNow
    var accessToken: String    // "access_token"
    var tokenType: String?     // "token_type"
    var refreshToken: String?  // "refresh_token"
    var expiry: String?        // "expiry" (RFC3339)
}
enum ResolveChoice: String { case keepLocal = "keep_local", keepRemote = "keep_remote", keepBoth = "keep_both" }
```

Config (`ConfigJSON`/`SetConfig`) is decoded into a `CoreConfig` struct with
explicit keys matching the on-disk format; the `SetConfig` patch uses the
snake_case patch keys (`morning_time`, `evening_time`, `summary_day`,
`summary_time`, `google_client_id`, `notify_checkins`).

Decoding rule: **fail loud**. Unknown enum strings or missing required keys
throw `CoreError.contractViolation(describing:)` and surface as a visible
error state, never a silent default (repo fail-loud rule; also catches
contract drift immediately in development).

### 2.5 Error mapping and the CAS-refresh loop

```swift
enum CoreError: Error {
    case casMismatch                 // "CAS_MISMATCH:" - refresh tree, re-present
    case notFound(String)            // "NOT_FOUND:"    - stale reference (project, backlog item, conflict path)
    case badInput(String)            // "BAD_INPUT:"    - validation (bad recur tag, unknown state string...)
    case syncAuth(String)            // "SYNC_AUTH:"    - token invalid/expired/revoked - trigger re-sign-in
    case contractViolation(String)   // Swift-side decode failure
    case other(String)               // anything unprefixed
}
```

`CoreClient` maps every thrown `NSError` by prefix-matching
`localizedDescription` against the coded prefixes (one function, one place).

**CAS loop** (used by every task action):

1. Render captures `(date, index, text)` from the last decoded `TreeTask`.
2. Action call passes all three, e.g.
   `try await core.setTaskState(date:index:expectedText:state:)`.
3. On success → refresh (section 3).
4. On `.casMismatch` → `TodayStore` silently re-fetches `TreeJSON`, re-renders,
   and shows a non-blocking toast "List changed - please retry". **No
   automatic retry**: the mismatch means the file changed underneath (Drive
   sync, desktop edit); re-running a destructive action against a re-matched
   index without the user re-confirming is exactly what the guard exists to
   prevent.
5. On `.notFound` (stale project/backlog/recycle reference) → same
   refresh-and-toast treatment, per-screen.
6. On `.syncAuth` → `SyncStore` flips to signed-out state and prompts
   re-authentication.

---

## 3. Data flow & state

### 3.1 Offline-first over local markdown

`dataDir` = `<app sandbox>/Documents/DailyProgress` (created on first launch;
`Documents` so a future `UIFileSharingEnabled`/Files-app export is free, and
it is backed up by iCloud device backup by default). `AppState` calls
`MobilecoreOpen(dataDir, clientID, deviceID)` once at launch:

- `clientID` comes from Settings (`ConfigJSON`'s `google_client_id`, editable
  in-app; empty is fine until the user sets up sync).
- `deviceID` = a stable per-install identifier, e.g. `"ios-" +
  UIDevice.current.identifierForVendor` persisted to UserDefaults on first
  run (used to label sync conflict copies).

Everything works with no network and no account. Sync is an optional overlay.

### 3.2 TreeJSON drives the main screen

`TodayStore.refresh()`:

```
let json = try await coreClient.treeJSON(date: viewedDate)   // actor hop, off-main
let tree = try decoder.decode(ProjectTree.self, ...)          // in CoreClient
self.tree = tree                                              // @MainActor publish
```

- `TreeJSON` **materializes recurring occurrences** server-side before
  building the tree (mirrors Qt `materializeViewedDate`), so the host never
  calls a materialize API - just fetch on view/date-change/foreground.
- The tree is small (one day's plan); full re-fetch + re-decode is
  milliseconds. **No diffing, no cache**: SwiftUI diffs the rendered list via
  `TreeTask.id`. This matches the core's design (no stable per-item IDs;
  re-fetch after every mutation is the documented contract,
  `docs/feature-parity.md` §(c)).

### 3.3 Index + expectedText addressing from the UI

- Each rendered row holds its `TreeTask` value - `index` and `text` were
  captured *at render time* from the same TreeJSON snapshot.
- Every action forwards both. `text` is the display text exactly as TreeJSON
  returned it (tag-stripped); never reconstruct or re-strip it host-side.
- Rule for the implementer: **never pass an empty `expectedText`** - empty
  disables the guard entirely (core contract, review L1).
- Double-guarded ops (`ReorderTask`, `MakeSubtask` - v2) pass both src and ref
  captures from the same snapshot.

### 3.4 Refresh strategy after mutations

- **Every successful mutation → immediate re-fetch of the owning screen's
  payload** (TreeJSON for Today, BacklogJSON for Backlog, etc.). No
  optimistic mutation of the decoded model; the store re-reads the truth.
  (This also naturally absorbs core-side quirks like duplicate-add being a
  silent no-op, review L6.)
- **Cross-screen invalidation**: mutations bump `AppState.dataVersion`; each
  screen re-fetches in `.task(id: appState.dataVersion)` when visible. Cheap
  and correct - e.g. "move to backlog" on Today invalidates Backlog.
- **On scene activation** (`scenePhase == .active`): refresh viewed screen,
  run `DuePromptsJSON(now)` for prompt routing, and kick a background-priority
  `SyncNow` if signed in; on sync completion (with any changes) bump
  `dataVersion` again.
- **Date semantics**: the core interprets `"YYYY-MM-DD"` in device-local time
  and Qt's "today" logic is local-midnight based; the app formats
  `viewedDate` with `Calendar.current` local components, and refreshes
  "today" on `NSCalendarDayChanged` notifications (midnight rollover, like
  Qt's midnight timer).

---

## 4. Screens & navigation

Root: `TabView` with four tabs - **Today**, **Week**, **Backlog**, **More** -
plus sheet/push presentation for sub-flows. Check-ins present as sheets over
whatever is active (they are prompts, not destinations).

| # | Screen | Shows | Core calls |
|---|---|---|---|
| 1 | **TodayView** (tab 1) | Date header with prev/next/today + calendar picker; sections: Projects (each project header → its tasks → nested subtasks, rollup strikethrough), Unfiled, Recurring (collapsed), Recycle Bin (collapsed link → RecycleView). Row: checkbox, text, state glyph. Swipe leading: done/todo. Swipe trailing: delete, postpone. Context menu (long-press): Edit, Add Subtask, Postpone to Tomorrow, Postpone to Next Week, Move to Backlog, Move to Project ▸, Delete. Toolbar: + (add task), project picker in the add sheet | `TreeJSON` (read); `SetTaskState`, `EditTaskText`, `DeleteTask`, `PostponeToNextDay`, `PostponeToNextWeek`, `MoveTaskToBacklog`, `MoveTaskToProject`, `AddTask`, `AddSubtask` |
| 2 | **AddTaskSheet** | Text field, optional project picker (open projects), detects `@daily`-style recur tags and offers "Add as recurring" (mirrors Qt `addItem`, `mainwindow.go:329-374`) | `AddTask` or `AddRecurring`; `ProjectsJSON` for the picker |
| 3 | **MorningCheckinSheet** | Weekly goals read-only header; "already planned today: N items" note; multiline new-items editor; candidate list with toggles (week carry-overs pre-checked, backlog items pre-unchecked, "(backlog)" suffix) - port of `buildMorningDialog` | `WeeklyPlanJSON`, `TreeJSON` (planned-today count), `MorningCandidatesJSON`; apply → `ApplyMorning` |
| 4 | **EveningCheckinSheet** | Each plan item (subtasks indented) with a 5-way segmented choice (todo/done/next day/next week/backlog), default derived from current state; extra-done multiline editor - port of `buildEveningDialog` | `TreeJSON` (plan); apply → `ApplyEvening` |
| 5 | **WeekView** (tab 2) | Current week card: goals with done toggles (+add), summary preview (done-by-day), buttons: Review Week…, Summary…; banner when `UnreviewedWeekJSON`/`WeeklySummaryPendingJSON` report pending work | `WeeklyPlanJSON`, `SetWeeklyPlan`, `WeeklySummaryJSON`, `UnreviewedWeekJSON`, `WeeklySummaryPendingJSON` |
| 6 | **WeekReviewSheet** | Per-candidate keep/postpone/drop selector; loops oldest unreviewed week first (port of `runWeekReviewLoop`, `app.go:497`); `rollover=true` only on the scheduled/prompted path | `UnreviewedWeekJSON`, `WeekReviewCandidatesJSON`, `ApplyWeekReview` |
| 7 | **WeeklySummarySheet** | Goals with states; done-by-day list; total count; "Mark summarized" (only when opened from the scheduled prompt, mirroring Qt `markOnAccept`) | `WeeklySummaryJSON`, `MarkWeekSummarized` |
| 8 | **BacklogView** (tab 3) | Two sections (Current / Next week); per-row actions: Plan Today (adopt), move to other section - port of `backlogdialog.go` | `BacklogJSON`, `AdoptFromBacklog`, `MoveBacklogItem` |
| 9 | **MoreView** (tab 4) | Links: Projects, Recurring, Recycle Bin, Sync & Account, Settings | - |
| 10 | **ProjectsView** | Open + closed sections; add (toolbar +), rename (tap), close/reopen (swipe) | `ProjectsJSON`, `AddProject`, `RenameProject`, `CloseProject`, `ReopenProject` |
| 11 | **RecurringView** | Template list (text + project + schedule from `raw`); add with syntax help + core-side validation error display; swipe delete | `RecurringJSON`, `AddRecurring`, `RemoveRecurring` |
| 12 | **RecycleView** | Deleted items grouped by date; per-row Restore / Purge (purge confirms) | `RecycleJSON`, `RestoreTask`, `PurgeRecycled` |
| 13 | **SyncView** | Sign-in state, account, Sync Now button + last-sync status/error, Conflicts badge → ConflictsView, Sign out | host OAuth (section 5.2); `SyncNow`, `ConflictsJSON` |
| 14 | **ConflictsView** | Conflict rows (path, conflict copy, time) with Keep Local / Keep Remote / Keep Both - port of `conflicts.go` | `ConflictsJSON`, `ResolveConflict` |
| 15 | **SettingsView** | Morning/evening time pickers, summary day+time, notifications toggle (drives both `notify_checkins` and local-notification scheduling), Google client ID field, data-dir path display (read-only) | `ConfigJSON`, `SetConfig` |

**Prompt routing**: on foreground, `AppState` calls
`DuePromptsJSON(now: RFC3339)`; for each due prompt ID it presents the
matching sheet in Qt's order (week review → weekly plan → morning → evening →
summary, one at a time, `PromptID` 0-4). Notification taps carry the prompt ID
in `userInfo` and route to the same presentation path.

Feature → screen coverage check: all v1 rows in section 1's table appear in a
screen above; the only Qt features with no screen are the v2/n-a items
(reorder/re-nest, snooze, tray/window management, shortcut editor,
login item, auto-update, screenshot mode).

---

## 5. iOS-specific concerns

### 5.1 Sandbox data dir

`FileManager.default.urls(for: .documentDirectory, ...)` +
`"DailyProgress"`, created before `MobilecoreOpen`. Note `config.Load()`
inside the core resolves `os.UserConfigDir()`, which on iOS lands inside the
sandbox (`HOME` is set) - workable, but the config file then lives *outside*
`dataDir` and does not Drive-sync (review M3; if the core moves config into
`dataDir`, nothing on the host changes).

### 5.2 Google Sign-In (host-owned OAuth)

The core deliberately delegates auth: it takes `tokenJSON` per sync call and
never persists it (`memTokenStore`). Host flow:

1. **`ASWebAuthenticationSession`** with the Google OAuth *iOS-type* client ID
   (from Settings), authorization-code + **PKCE** (S256), redirect via the
   reversed-client-ID custom scheme
   (`com.googleusercontent.apps.<id>:/oauthredirect`) - the iOS equivalent of
   the desktop loopback flow in `drive.SignIn`. Scope: `drive.file` (match
   whatever `internal/drive.Config` requests - verify at implementation).
2. Exchange the code at `oauth2.googleapis.com/token` (plain `URLSession`;
   iOS-type clients have no client secret, PKCE suffices).
3. Serialize as `OAuthToken` (the `oauth2.Token` wire shape - `access_token`,
   `refresh_token`, `token_type`, `expiry` as RFC3339 computed from
   `expires_in`) and store in **Keychain**
   (`kSecClassGenericPassword`, `kSecAttrAccessibleAfterFirstUnlock` so future
   background sync can read it).
4. Every `SyncNow(tokenJSON:)` passes the Keychain token; the returned
   envelope's `token` is **written back to the Keychain unconditionally**
   (the core refreshes expired access tokens internally; Google may also
   rotate the refresh token - losing it strands the user, review H3).
5. `SYNC_AUTH:`-coded errors → clear stored token state → show signed-out UI.
6. Sign out = delete the Keychain item (optionally revoke via
   `oauth2.googleapis.com/revoke`).

### 5.3 Local notifications - v1 (simple)

- Request `UNUserNotificationCenter` authorization from Settings/first-run
  (only when the notify toggle goes on; never at cold launch).
- Schedule repeating `UNCalendarNotificationTrigger`s from the config values:
  morning (daily at `morning_time`), evening (daily at `evening_time`),
  weekly summary (`summary_day` + `summary_time`), weekly plan/review
  (Monday at `morning_time`). Reschedule whenever `SetConfig` changes times
  or the toggle flips.
- `userInfo["prompt_id"]` routes the tap to the right check-in sheet;
  the sheet itself re-checks `DuePromptsJSON` so an already-done check-in
  shows "all caught up" instead of a stale form.
- Known v1 limitation (accepted, documented in-app): reminders fire even if
  the check-in was already completed (on this device or via desktop + sync),
  because suppression needs background execution.

### 5.4 Background sync - v2 (with the smart-notification upgrade)

When built: `BGAppRefreshTask` (`BGTaskScheduler`), identifier
`com.<org>.dailyprogress.refresh`; handler = Keychain token → `SyncNow` →
persist returned token → re-evaluate `DuePromptsJSON` → replace pending
notifications with only-what's-due one-shots. Budget ~30s, well within one
sync. Requires `UIBackgroundModes: fetch` + task identifier in Info.plist.
Deferred because scheduling is opportunistic (untestable-by-waiting), and
foreground-sync v1 already keeps data fresh for the actual usage pattern.

---

## 6. Project layout & build

```
ios/
  DailyProgress.xcodeproj          # committed; CI stub already references this path
  Frameworks/
    Core.xcframework               # gitignored (already in .gitignore); built by `make ios-core`
  DailyProgress/
    App/
      DailyProgressApp.swift       # @main, AppState, tab root, prompt routing, scenePhase hooks
      AppState.swift
    CoreKit/                       # the only layer that imports Core
      CoreClient.swift             # actor; owns MobilecoreCore; JSON decode; error mapping
      CoreError.swift
      Models/                      # Codable DTOs from section 2.4 (one file per domain)
        Tree.swift  Backlog.swift  Checkin.swift  Weekly.swift
        Project.swift  Recurring.swift  Recycle.swift  Sync.swift
        Prompts.swift  Config.swift
    Features/
      Today/                       # TodayView, TodayStore, TaskRow, AddTaskSheet, EditTaskSheet
      Checkins/                    # MorningCheckinSheet, EveningCheckinSheet, CheckinStore
      Week/                        # WeekView, WeekReviewSheet, WeeklySummarySheet, WeekStore
      Backlog/                     # BacklogView, BacklogStore
      Projects/                    # ProjectsView, ProjectsStore
      Recurring/                   # RecurringView, RecurringStore
      Recycle/                     # RecycleView, RecycleStore
      Sync/                        # SyncView, ConflictsView, SyncStore, GoogleAuth.swift, KeychainTokenStore.swift
      Settings/                    # SettingsView, SettingsStore
    Support/
      Notifications.swift          # UNUserNotificationCenter scheduling + tap routing
      DateFormatting.swift         # "YYYY-MM-DD" <-> Date, local calendar
    Resources/  Assets.xcassets  Info.plist
  DailyProgressTests/              # unit tests: DTO decoding fixtures, CoreError prefix mapping,
                                   # OAuthToken round-trip, CAS-flow store logic (Core mocked behind a protocol)
  DailyProgressUITests/            # smoke: launch, add task, check task (simulator, real Core, temp dataDir)
```

Notes:

- `CoreClient` fronts a small `CoreAPI` protocol so feature-store unit tests
  can inject a mock without linking the 40 MB framework into test iteration.
- **Build wiring**: developers run `make ios-core` (already defined,
  `Makefile:122-124`) whenever `mobilecore/` or `internal/` change; the Xcode
  project links `ios/Frameworks/Core.xcframework` by relative path. Add a
  guard build-phase script that fails fast with "run `make ios-core`" when
  the xcframework is missing (better than a cryptic linker error). Do NOT
  rebuild Go from an Xcode run-script phase by default - gomobile takes
  minutes cold and would wreck the edit-compile loop.
- **CI** (`.github/workflows/mobile.yml`): the iOS job already builds the
  xcframework; un-comment/extend the placeholder step exactly as stubbed:

  ```yaml
  - name: Build iOS app
    run: |
      xcodebuild -project ios/DailyProgress.xcodeproj \
        -scheme DailyProgress \
        -destination 'generic/platform=iOS Simulator' \
        CODE_SIGNING_ALLOWED=NO build
  ```

  It runs after the bind step, so the framework is present. Add a follow-up
  step for unit tests (`xcodebuild test -destination 'platform=iOS
  Simulator,name=iPhone 17'`) once tests exist. No signing on CI
  (`CODE_SIGNING_ALLOWED=NO`); device signing stays local.

---

## 7. Build sequence for the implementer

Each step is independently verifiable; "verify" means `xcodebuild build`
(and where noted `xcodebuild test` / simulator interaction) passes. Follow
repo rules: worktree per change, small PRs (~1 step ≈ 1 PR).

1. **Scaffold + link** - Create `ios/DailyProgress.xcodeproj` (iOS 17 floor,
   SwiftUI lifecycle), link `Core.xcframework` (Embed & Sign), missing-
   framework guard phase. `AppState` opens the Core at launch over the
   sandbox dataDir and renders raw `treeJSON(today)` string in a debug view.
   *Verify*: `make ios-core && xcodebuild ... build`; simulator shows JSON;
   the interop smoke test proves Go errors arrive as thrown `NSError`s
   (call `treeJSON("not-a-date")`). Extend the CI workflow's app-build step
   here (it must fail if the scaffold breaks).
2. **CoreKit** - `CoreClient` actor wrapping every v1 Core method; `CoreError`
   prefix mapping; all Codable models; decoding unit tests against fixture
   JSON matching the target contract (fixtures double as contract docs;
   coordinate drift with mobilecore's own tests). *Verify*: `xcodebuild test`.
3. **Today screen (read)** - `TodayStore` + tree rendering (projects/
   unfiled/recurring/recycled, rollup states, indentation), date navigation,
   midnight/day-change + foreground refresh. *Verify*: simulator against a
   seeded dataDir (copy a fixture markdown tree into the sandbox in a debug
   launch argument path).
4. **Task actions** - add/check/edit/delete/postpone(day|week)/backlog/
   move-to-project/add-subtask with swipe + context menus, CAS capture, and
   the mismatch → refresh + toast loop. *Verify*: simulator; force a
   CAS_MISMATCH by editing the markdown file externally (Files app or
   `simctl` file push) between render and action.
5. **Check-ins** - morning + evening sheets, `DuePromptsJSON` foreground
   routing. *Verify*: simulator with device clock/config times arranged so a
   prompt is due; confirm ApplyMorning/ApplyEvening results in the tree.
6. **Weekly** - WeekView, plan editor, review loop (oldest-first), summary
   (+ mark summarized), pending banners. *Verify*: seeded multi-week fixture.
7. **Backlog / Recurring / Recycle / Projects / Settings** - the four list
   screens + settings form (`ConfigJSON`/`SetConfig`). *Verify*: each flow in
   simulator; recurring add with an invalid tag shows the core's BAD_INPUT
   message.
8. **Sync** - `GoogleAuth` (ASWebAuthenticationSession + PKCE),
   `KeychainTokenStore`, SyncView, foreground auto-sync, token write-back
   from the `SyncNow` envelope, ConflictsView + resolve. *Verify*: real
   Google account on simulator against a Drive folder also synced by the
   desktop app; check conflict creation/resolution both ways; verify the
   Keychain token updates after an expired-access-token sync.
9. **Notifications (v1 simple)** - authorization flow, calendar triggers from
   config, reschedule on settings change, tap → prompt routing. *Verify*:
   simulator with times set 1-2 min ahead; tap routes to the correct sheet;
   toggling notify off clears pending requests
   (`UNUserNotificationCenter.getPendingNotificationRequests`).
10. **Polish + release prep** - app icon, empty states
    (`ContentUnavailableView`), error toasts, accessibility labels on
    custom rows, CI test step, TestFlight signing (local).

v2 backlog (file as triaged issues at step 10): background sync
(BGTaskScheduler) + smart notification suppression, reorder/re-nest,
prompt snooze, iPad layout.

---

## 8. Risks / unknowns

| # | Risk | Mitigation |
|---|---|---|
| 1 | **gomobile Swift interop shape** - throwing-import of `_Nonnull`-returning methods with `NSError**` may differ across Xcode/gomobile versions; param types import as `String?`. | Step-1 smoke test pins the behavior; all interop confined to `CoreClient` so a fallback to manual `NSErrorPointer` handling touches one file. |
| 2 | **Threading** - the core is getting an internal mutex (review M2), but until it lands, concurrent calls silently lose updates (last save wins). gomobile permits calls from any thread. | `CoreClient` actor serializes every call host-side regardless; keep it even after the core mutex lands (defense in depth + main-thread hygiene). |
| 3 | **Contract drift** - the JSON contract is being finalized in parallel; TreeJSON DTOs, error prefixes, and the SyncNow envelope are the moving parts. | Fixture-based decode tests (step 2) fail on drift; fail-loud decoding means drift is a red test / visible error, not silent corruption. Do not start step 3 until the mobilecore contract PR is merged. |
| 4 | **xcframework size** - current bind is ~40 MB device slice / ~79 MB simulator (fat). Go runtime + Drive/OAuth deps dominate. | Non-issue for users: only the arm64 slice ships, App Store thinning + compression bring on-the-wire size well down; monitor with `App Thinning Size Report`. Keep the artifact out of git (already gitignored) and out of PR diffs. |
| 5 | **Simulator vs device** - simulator uses the separate slice; Keychain and `ASWebAuthenticationSession` behave slightly differently (no passcode, ephemeral sessions); BGTaskScheduler basically untestable on simulator (v2). | CI builds simulator-destination only; sync (step 8) gets a device pass before TestFlight. |
| 6 | **CAS is advisory** (TOCTOU + duplicate texts, review M1) and `verifyIndex` fails open when the projects file is unreadable (H5). | UI never auto-retries after mismatch; destructive actions (delete, purge) use confirmation affordances (swipe-full or confirm dialog); revisit if the core adopts fail-closed for destructive ops. |
| 7 | **Token loss on refresh rotation** (H3) if the envelope write-back is forgotten in any code path. | Single choke point: `SyncStore.sync()` is the only caller of `SyncNow`, and persisting the returned token is part of the same function, before conflict handling. |
| 8 | **`config.Load()` sandbox behavior** (M3) - config lives outside dataDir, doesn't sync, and a nominally-read call may create the file with desktop defaults. | Acceptable for v1 (times are per-device anyway); if the core moves config under dataDir, only `SettingsStore` cares. |
| 9 | **Time zones / travel** - core parses dates in `time.Local`; a timezone change mid-day shifts "today". Qt has the same semantics. | Use `Calendar.current` consistently; refresh on `NSSystemTimeZoneDidChange` + `NSCalendarDayChanged`. Accept parity with desktop behavior. |
| 10 | **Go runtime + iOS memory/suspension** - the Go scheduler runs threads iOS may freeze on suspension; long-running calls at suspension time could be killed mid-write. | Store writes are atomic (tmp+rename, verified in review); syncs run under a short `beginBackgroundTask` grace so an in-flight foreground sync can finish. |
| 11 | **`gomobile bind` toolchain fragility** (Xcode/Go version pairs occasionally break bind). | CI already builds the bind on every mobile-touching PR; pin gomobile via `go.mod` tool dependency if a breakage bites. |
