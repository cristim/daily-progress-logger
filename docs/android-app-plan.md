# Android App Architecture Plan

Date: 2026-07-17 · Branch: `feat/unified` · Status: plan (no `android/` app code exists yet)

Companion to the iOS app plan; the two apps deliberately share v1 scope, screen structure,
and the Core JSON contract so features land in lockstep. The Qt desktop app
(`internal/ui/*`) is the feature reference; the shared Go core (`mobilecore/*`, 44 exported
`Core` methods) is consumed as a gomobile-generated AAR (`android/core/core.aar`, built by
`make android-core`).

**Contract note**: the mobilecore JSON contract is being finalized (see
`docs/mobilecore-review.md`, fixes C1/H1-H4/M5 in flight). This plan targets the FINAL
contract, not today's HEAD:

- Explicit DTOs with **snake_case** keys everywhere (including `TreeJSON`).
- Task/item state as **string enums**: `"todo"` / `"done"` / `"postponed"`.
- All dates as **`"YYYY-MM-DD"`** strings (no RFC3339 in tree payloads).
- Empty collections serialize as **`[]`, never `null`**.
- Errors carry **stable machine-readable prefixes** the host prefix-matches:
  `"CAS_MISMATCH:"`, `"NOT_FOUND:"`, `"BAD_INPUT:"`, `"SYNC_AUTH:"`.
- Task actions take **`(date, index, expectedText)`** with a compare-and-swap guard;
  `CAS_MISMATCH` means "refresh `TreeJSON` and re-present".
- **`SyncNow` returns an envelope** `{"conflicts":[...], "token":{...}}`; the host persists
  the returned token (refresh-token rotation safety) before doing anything else.

---

## 1. v1 Scope

Mirrors the iOS prioritization: the daily loop is the product; everything else follows.

### Ships in v1 (in build order)

| Feature | Qt reference | Core methods |
|---|---|---|
| Day tree (projects / unfiled / subtasks, done rollup, date nav) | `mainwindow.go` refresh → `store.BuildProjectTree`; recurring materialized on view | `TreeJSON(date)` |
| Add task (unfiled or to project) | `mainwindow.go` addItem, `tree.go` addProjectTask | `AddTask(date, text, projectID)` |
| Check / uncheck / postpone-glyph | `tree.go` checkbox + statebuttons | `SetTaskState(date, index, expectedText, state)` |
| Edit task text | `tree.go` editTask | `EditTaskText` |
| Delete → recycle bin | `tree.go` delete | `DeleteTask` |
| Postpone to next day / next week | `tree.go` hover actions | `PostponeToNextDay`, `PostponeToNextWeek` |
| Move task to backlog | `tree.go` → `store.MoveToBacklog` | `MoveTaskToBacklog` |
| Add subtask; nest existing task via menu | `tree.go` "+ Sub", drag-nest | `AddSubtask`, `MakeSubtask` |
| Move task to project / unassign (long-press menu, not drag) | `tree.go` drag onto project | `MoveTaskToProject`, `UnassignTaskProject` |
| Morning check-in | `dialogs.go` buildMorningDialog | `MorningCandidatesJSON`, `ApplyMorning` |
| Evening check-in | `dialogs.go` buildEveningDialog | `ApplyEvening` |
| Weekly plan | `dialogs.go` buildWeeklyPlanDialog | `WeeklyPlanJSON`, `SetWeeklyPlan` |
| Week review (oldest-first loop) | `app.go` review loop, `dialogs.go` buildWeekReviewDialog | `UnreviewedWeekJSON`, `WeekReviewCandidatesJSON`, `ApplyWeekReview` |
| Weekly summary | `dialogs.go` buildWeeklySummaryDialog | `WeeklySummaryJSON`, `MarkWeekSummarized`, `WeeklySummaryPendingJSON` |
| Backlog (view / adopt / shuttle Current↔Next week) | `backlogdialog.go` | `BacklogJSON`, `AdoptFromBacklog`, `MoveBacklogItem` |
| Recurring templates (list / add with `@daily` etc. / remove) | `tree.go` Recurring section, add-row recur detection | `RecurringJSON`, `AddRecurring`, `RemoveRecurring` |
| Recycle bin (list / restore / purge) | `tree.go` Recycle section | `RecycleJSON`, `RestoreTask`, `PurgeRecycled` |
| Projects (list / add / rename / close / reopen) | `mainwindow.go` New Project, `tree.go` rename/close | `ProjectsJSON`, `AddProject`, `RenameProject`, `CloseProject`, `ReopenProject` |
| **Sync: sign-in + manual sync + conflicts** (see §5) | `gsync.go`, `conflicts.go` | `SyncNow`, `ConflictsJSON`, `ResolveConflict` |
| In-app due-prompt banner ("Morning check-in is due") shown on app open/resume | `app.go` CheckPrompts | `DuePromptsJSON(nowRFC3339)` |

**Sync is v1** (sign-in, sync-on-open/resume, explicit "Sync now", conflict resolution).
Rationale: the user's data lives on the desktop; a mobile app without Drive sync starts
empty and is pointless. Foreground-triggered sync covers the phone usage pattern (open app
→ sync → act → sync on background) without any background machinery.

### Deferred to v2 (with rationale)

| Feature | Rationale |
|---|---|
| Background periodic sync (WorkManager) | On-open/on-stop foreground sync covers >90% of the value; background sync adds Doze/constraint/token-refresh-while-backgrounded complexity and needs the mutex + token-envelope contract proven in the field first. |
| Check-in reminder notifications (`DuePromptsJSON` + AlarmManager/WorkManager + NotificationManager) | Depends on config source-of-truth fix (mobilecore-review M3: `config.Load()` uses `os.UserConfigDir`, unreliable under Android's env) and on exact-alarm permission UX (API 31+). The in-app due banner delivers the core nudge in v1. |
| Reorder tasks (touch drag) | Qt uses drag-and-drop (`tree.go` → `store.ReorderTask`); LazyColumn drag-reorder with CAS guards on both indices (`ReorderTask(date, srcIndex, expectedSrcText, refIndex, expectedRefText, below)`) is fiddly; menu-driven actions cover v1. |
| Nest-by-drag | Same; v1 exposes `MakeSubtask` via a "Nest under…" picker instead. |
| Preferences editing (check-in times via `ConfigJSON`/`SetConfig`) | Blocked on M3 config relocation; only matters once notifications exist. |
| Home-screen widget, Wear tile, app shortcuts | Pure polish. |

Explicitly out of scope (desktop-only, per feature-parity n/a rows): tray/menu-bar
residency, login item, auto-update, keyboard-shortcut editor, screenshot mode.

---

## 2. Tech Choices

### Platform

- **Kotlin 2.0.x + Jetpack Compose** (Compose BOM, Material 3), **Navigation Compose**.
- **minSdk 26, targetSdk 35, compileSdk 35.** The AAR is built with `-androidapi 21`
  (its manifest minSdk 21 merges cleanly under an app minSdk 26). 26 buys notification
  channels (v2), `java.time` without desugaring, and ~97% device coverage; nothing in the
  app needs lower.
- **JDK 17** toolchain (matches CI), **AGP 8.7.x**, **Gradle 8.10.x** wrapper — the
  AGP 8.7 line is the newest verified against JDK 17 + compileSdk 35 + build-tools 35.0.1.
  NDK 26.3 is only used by `gomobile bind` (via `ANDROID_NDK_HOME`), never by Gradle; the
  app module contains no native code of its own.

### Architecture: MVVM

- **One `CoreClient`** (singleton) wraps the gomobile binding. gomobile generates Java
  package `mobilecore` inside `core.aar`: `Mobilecore.open(dataDir, clientID, deviceID)`
  returns a `Core`; every method returns `String`/`void` and `throws Exception`; **Go `int`
  binds as Java `long`** (all `index` parameters are `Long` in Kotlin).
- **`CoreRepository`** on top of `CoreClient`: decodes JSON to data classes, maps thrown
  exceptions to a typed `CoreError`, and **serializes all Core calls on a single-threaded
  dispatcher** (`Dispatchers.IO.limitedParallelism(1)`). The core is getting an internal
  mutex (mobilecore-review M2), but the store is read-modify-write over files, so the host
  serializes anyway to keep last-write-wins ordering deterministic and never blocks the
  main thread (gomobile calls are synchronous and do file I/O).
- **ViewModel + StateFlow** per screen: each ViewModel exposes
  `StateFlow<UiState>` (`Loading` / `Content(data)` / `Error(CoreError)`), mutations are
  `viewModelScope.launch` → repository → refresh. No optimistic UI in v1 (refresh after
  every mutation, see §3).
- DI: manual (a small `AppContainer` on `Application`) — Hilt is overkill for one
  repository; can be introduced later without churn.

### JSON decoding: kotlinx.serialization

`kotlinx-serialization-json` with `Json { ignoreUnknownKeys = true; explicitNulls = false }`
so additive core changes never crash old app versions. All DTOs are `@Serializable` data
classes with `@SerialName` snake_case mapping. The complete DTO set (mirrors the target
contract 1:1):

```kotlin
// TreeJSON
@Serializable data class TreeDto(
    val projects: List<TreeProjectDto> = emptyList(),
    val unfiled: List<TreeTaskDto> = emptyList(),
    val recycled: List<TreeTaskDto> = emptyList(),
    val recurring: List<RecurringDto> = emptyList(),
)
@Serializable data class TreeProjectDto(
    val id: String, val name: String, val done: Boolean,
    val tasks: List<TreeTaskDto> = emptyList(),
)
@Serializable data class TreeTaskDto(
    val index: Long,               // stable plan-file index → all task actions
    val depth: Int,
    val text: String,              // display text (project tag stripped) → expectedText
    val state: TaskState,          // "todo" | "done" | "postponed"
    val date: String,              // "YYYY-MM-DD"
    val done: Boolean,             // rolled-up display state
    val project: String = "",      // set for recycle-bin entries
    val children: List<TreeTaskDto> = emptyList(),
)
@Serializable enum class TaskState {
    @SerialName("todo") TODO,
    @SerialName("done") DONE,
    @SerialName("postponed") POSTPONED,
}

// Backlog / check-ins
@Serializable data class BacklogDto(
    val current: List<String> = emptyList(),
    @SerialName("next_week") val nextWeek: List<String> = emptyList(),
)
@Serializable data class MorningCandidateDto(
    val text: String, @SerialName("from_backlog") val fromBacklog: Boolean)
@Serializable data class MorningDecisionsDto(          // → ApplyMorning
    @SerialName("new_items") val newItems: List<String>,
    val adopted: List<MorningCandidateDto>)
@Serializable data class EveningDecisionDto(val text: String, val action: Int)
    // 0=todo 1=done 2=next_day 3=next_week 4=backlog (mobilecore/checkin.go)
@Serializable data class EveningDecisionsDto(          // → ApplyEvening
    val decisions: List<EveningDecisionDto>,
    @SerialName("extra_done") val extraDone: List<String>)

// Weekly
@Serializable data class WeeklyGoalDto(val text: String, val done: Boolean = false)
@Serializable data class WeeklyPlanDto(
    val week: String, val planned: Boolean, val goals: List<WeeklyGoalDto> = emptyList())
@Serializable data class WeekReviewCandidatesDto(
    val week: String, val candidates: List<String> = emptyList())
@Serializable data class ReviewDecisionDto(val text: String, val action: Int)
    // 0=keep 1=postpone 2=drop (mobilecore/weekly.go)
@Serializable data class ReviewDecisionsDto(           // → ApplyWeekReview
    val decisions: List<ReviewDecisionDto>, val rollover: Boolean)
@Serializable data class DayDoneDto(val date: String, val items: List<String> = emptyList())
@Serializable data class WeeklySummaryDto(
    val week: String, val start: String, val end: String,
    val summarized: Boolean, val reviewed: Boolean,
    val goals: List<WeeklyGoalDto> = emptyList(),
    @SerialName("done_by_day") val doneByDay: List<DayDoneDto> = emptyList())
@Serializable data class PendingWeekDto(val pending: Boolean, val week: String = "")
    // shared by WeeklySummaryPendingJSON and UnreviewedWeekJSON

// Projects / recurring / recycle
@Serializable data class ProjectDto(val id: String, val name: String, val status: ProjectStatus)
@Serializable enum class ProjectStatus {
    @SerialName("open") OPEN, @SerialName("closed") CLOSED }
@Serializable data class RecurringDto(
    val text: String, val project: String = "", val raw: String) // raw → RemoveRecurring
@Serializable data class RecycleEntryDto(
    val date: String, val text: String, val state: TaskState)

// Sync
@Serializable data class ConflictDto(
    val path: String, @SerialName("conflict_copy") val conflictCopy: String, val time: String)
@Serializable data class SyncResultDto(                // SyncNow envelope
    val conflicts: List<ConflictDto> = emptyList(),
    val token: JsonObject? = null)   // opaque oauth2.Token JSON; persist verbatim, never model
@Serializable enum class ConflictChoice {              // → ResolveConflict
    @SerialName("keep_local") KEEP_LOCAL,
    @SerialName("keep_remote") KEEP_REMOTE,
    @SerialName("keep_both") KEEP_BOTH }

// Schedule / config
@Serializable data class DuePromptDto(val id: Int, val name: String)
    // 0=week review 1=weekly plan 2=morning 3=evening 4=weekly summary (mobilecore/schedule.go)
@Serializable data class DuePromptsDto(val due: List<DuePromptDto> = emptyList())
```

The token stays an opaque `JsonObject`/raw String end to end (EncryptedSharedPreferences →
`SyncNow` → envelope → back to EncryptedSharedPreferences); the app never inspects
`oauth2.Token` fields.

### Error handling: coded prefixes + CAS retry loop

```kotlin
sealed class CoreError(val raw: String) : Exception(raw) {
    class CasMismatch(raw: String) : CoreError(raw)   // "CAS_MISMATCH:"
    class NotFound(raw: String)    : CoreError(raw)   // "NOT_FOUND:"
    class BadInput(raw: String)    : CoreError(raw)   // "BAD_INPUT:"
    class SyncAuth(raw: String)    : CoreError(raw)   // "SYNC_AUTH:"
    class Unknown(raw: String)     : CoreError(raw)
}

suspend fun <T> CoreRepository.call(block: (Core) -> T): T =
    withContext(coreDispatcher) {
        try { block(core) }
        catch (e: Exception) { throw parsePrefix(e.message.orEmpty()) }
    }
```

`parsePrefix` prefix-matches the four codes (everything else → `Unknown`). Handling:

- **`CasMismatch`** — the tree went stale under the user (background sync rewrote the day
  file). Repository re-fetches `TreeJSON`, ViewModel replaces state, UI shows a snackbar
  "List changed — refreshed, try again". **No automatic retry of the mutation** in v1:
  after a rewrite, index+text may now identify a different task; re-presenting is the safe
  Qt-equivalent contract (`ErrCASMismatch` doc: "call TreeJSON again and re-present").
- **`SyncAuth`** — token invalid/revoked: clear stored token, flip sync state to
  "signed out", surface a "Reconnect Google Drive" banner.
- **`NotFound` / `BadInput`** — programming or staleness errors: refresh + snackbar with
  the message body.
- **`Unknown`** — snackbar with message; never crash on a Core error.

---

## 3. Data Flow & State

**Offline-first over local markdown.** `Mobilecore.open(dataDir, clientID, deviceID)` is
called once at startup with `dataDir = context.filesDir.resolve("data").absolutePath`
(app-internal storage: sandboxed, no runtime permissions, backed up by Auto Backup —
exclude `.oauth-token.json`-style files via backup rules, though v1 never uses
`FileTokenStore`). Every screen works fully offline; sync merely reconciles files.

- `deviceID`: stable per-install ID (random UUID persisted in SharedPreferences on first
  launch) — names this device in sync conflict copies.
- `clientID`: the Google OAuth Android client ID, from `BuildConfig` (see §5).

**Read path**: `DayViewModel` holds `selectedDate: LocalDate`; any change →
`repository.tree(date)` → `TreeJSON` (which also materializes recurring occurrences due
that day, matching Qt `materializeViewedDate`) → decode `TreeDto` → `StateFlow` → Compose.

**Mutation path — index + expectedText captured at render**: each rendered row carries the
`(date, index, text)` triple from the `TreeTaskDto` it was built from. Actions pass all
three to the Core (`SetTaskState(date, index, expectedText, state)` etc.); the core's CAS
guard verifies `expectedText` still lives at `index` before acting. Compose never
re-derives indices — they flow from the last decoded tree only.

**Refresh discipline**: the store has no per-item stable IDs (markdown lines addressed by
index), so **every mutation is followed by a `TreeJSON` re-fetch** and full state
replacement. Same rule for the other screens (backlog, recycle, projects, weekly:
re-fetch their read endpoint after each `Apply*`/mutating call). Tree payloads are
KB-scale; re-decode cost is negligible against file I/O. Also refresh on `ON_RESUME`
lifecycle events (another surface or sync may have written the files) and after every
`SyncNow`.

**State keying**: expansion state (collapsed projects/tasks) is host-side, keyed by
project ID / task text path (mirroring Qt `tree.go` expandedOr), kept in the ViewModel so
it survives refreshes but not process death (fine for v1).

---

## 4. Screens & Navigation

Single-activity Compose Navigation graph. Bottom bar: **Today · Backlog · Weekly · More**.
Every v1 feature maps to a screen + Core calls below.

| # | Screen (route) | Shows | Core calls |
|---|---|---|---|
| 1 | **Day** (`day/{date}`, start dest, default today) | Date header with prev/next + date picker + "Today"; project tree: sections per open project, Unfiled, collapsible Recurring + Recycle previews (tap-through to 9/10); checkbox rows with done rollup; due-prompt banner when `DuePromptsJSON` reports pending; FAB add | `TreeJSON`; banner: `DuePromptsJSON(now)`; on-resume sync trigger: `SyncNow` |
| 2 | **Task actions sheet** (modal bottom sheet from long-press on a row) | Edit, Done/Todo, Postpone → tomorrow, Postpone → next week, Move to backlog, Add subtask, Nest under…, Move to project…, Delete | `EditTaskText`, `SetTaskState`, `PostponeToNextDay`, `PostponeToNextWeek`, `MoveTaskToBacklog`, `AddSubtask`, `MakeSubtask`, `MoveTaskToProject`/`UnassignTaskProject`, `DeleteTask` |
| 3 | **Add task sheet** (FAB / per-project "+") | Text field + project picker; detects recurrence tags (`@daily`, `@weekly @mon @09:00`) like Qt `mainwindow.go` addItem and routes to recurring | `AddTask(date, text, projectID)`; `AddRecurring(text)` when a recur tag is present; picker: `ProjectsJSON` |
| 4 | **Morning check-in** (`checkin/morning/{date}`) | Carry-over candidates as check rows (backlog-sourced marked), multiline "new items" field; mirrors `dialogs.go` buildMorningDialog | `MorningCandidatesJSON` → `ApplyMorning(date, MorningDecisionsDto)` |
| 5 | **Evening check-in** (`checkin/evening/{date}`) | Per-plan-item action picker (todo/done/next day/next week/backlog) + "extra done" field; mirrors buildEveningDialog | `TreeJSON` (item list) → `ApplyEvening(date, EveningDecisionsDto)` |
| 6 | **Weekly hub** (`weekly`) | Cards: Plan (goals + planned state), Review (pending badge from oldest unreviewed week), Summary (pending badge) | `WeeklyPlanJSON`, `UnreviewedWeekJSON`, `WeeklySummaryPendingJSON` |
| 6a | **Weekly plan editor** | Editable goal list with done ticks; mirrors buildWeeklyPlanDialog | `WeeklyPlanJSON` → `SetWeeklyPlan(date, goalsJSON)` |
| 6b | **Week review** (loops oldest-first like Qt `app.go` review loop) | Candidates with keep/postpone/drop segmented buttons, rollover flag for Monday reviews | `UnreviewedWeekJSON` → `WeekReviewCandidatesJSON` → `ApplyWeekReview` |
| 6c | **Weekly summary** | Goals with states, done-by-day list, "Mark summarized"; mirrors buildWeeklySummaryDialog | `WeeklySummaryJSON` → `MarkWeekSummarized` |
| 7 | **Backlog** (`backlog`) | Current + Next-week sections; per-row: Plan today, shuttle Current↔Next week; mirrors `backlogdialog.go` | `BacklogJSON`, `AdoptFromBacklog(date, text)`, `MoveBacklogItem(text, toNextWeek)` |
| 8 | **Recurring** (`recurring`) | Template list (text, project, schedule from raw), add field with tag validation, swipe/menu delete; mirrors Qt Recurring section `tree.go` | `RecurringJSON`, `AddRecurring`, `RemoveRecurring(raw)` |
| 9 | **Recycle bin** (`recycle`) | Deleted items grouped by date; Restore / Purge (with confirm); mirrors Qt Recycle section | `RecycleJSON`, `RestoreTask(date, text)`, `PurgeRecycled(date, text)` |
| 10 | **Projects** (`projects`) | Open + closed lists; add, rename, close, reopen | `ProjectsJSON`, `AddProject`, `RenameProject`, `CloseProject`, `ReopenProject` |
| 11 | **Sync & account** (`settings/sync`) | Google connection state, Connect/Disconnect (AppAuth flow), "Sync now" with last-result line, conflict count badge; mirrors `gsync.go` Drive section | `SyncNow(tokenJSON)` (persist returned token), `ConflictsJSON(tokenJSON)` |
| 12 | **Conflicts** (`settings/conflicts`) | Per-file rows with Keep local / Keep remote / Keep both; mirrors `conflicts.go` | `ConflictsJSON`, `ResolveConflict(tokenJSON, path, choice)` |
| 13 | **More/Settings** (`settings`) | Entry points to 8/9/10/11, app version, licenses. (Check-in time editing = v2, blocked on config M3.) | — |

The due-prompt banner on Day deep-links: prompt id 2 → screen 4, 3 → 5, 1 → 6a, 0 → 6b,
4 → 6c (IDs per `mobilecore/schedule.go`).

---

## 5. Android-Specific Concerns

### Storage

- Markdown store: `filesDir/data` (internal, sandboxed). Never external storage in v1;
  a SAF export can come later.
- OAuth token: **EncryptedSharedPreferences** (`androidx.security:security-crypto`,
  MasterKey AES256-GCM). The core's token contract (`mobilecore/core.go` package doc) makes
  the host the owner: pass `tokenJSON` into every `SyncNow`/`ConflictsJSON`/
  `ResolveConflict`, and **write back the token from `SyncNow`'s envelope after every run**
  (refresh-token rotation, review H3). Do not use `FileTokenStore` on Android (unencrypted
  at rest; it exists for the CLI).
- Backup rules (`dataExtractionRules`): include `files/data`, exclude the encrypted prefs
  (keys don't survive restore to a new device anyway).

### Google Sign-In: AppAuth + PKCE (v1)

The desktop uses an installed-app loopback+PKCE flow (`internal/drive`, Drive scope). The
Android equivalent of "host does sign-in, core gets a token" is **AppAuth-Android**
(`net.openid:appauth`), not Credential Manager — Credential Manager authenticates identity
but does not mint the Drive-scoped `oauth2.Token` JSON the core consumes:

1. Create a Google OAuth **Android client ID** (package name + SHA-1). Android clients
   have no client secret — matching `drive.Config(clientID, "")` in `mobilecore/core.go`.
2. AppAuth Custom-Tab authorization request: scope `https://www.googleapis.com/auth/drive.file`
   (mirror whatever `internal/drive` requests), redirect
   `com.googleusercontent.apps.<client-id>:/oauth2redirect`, PKCE S256 (AppAuth default).
3. Exchange code → `TokenResponse`; assemble the `oauth2.Token`-shaped JSON
   (`access_token`, `token_type`, `refresh_token`, `expiry` RFC3339) → EncryptedSharedPrefs.
4. Every sync call passes that JSON; persist the envelope's returned token after.
5. `SYNC_AUTH:` error → wipe token, show reconnect banner.

### Sync scheduling

- **v1: foreground only.** `SyncNow` on: app `ON_START` (if signed in and >5 min since
  last run — matching Qt's 5-minute timer cadence in `gsync.go` startSyncTimer), explicit
  "Sync now", and `ON_STOP` best-effort. Runs on the serialized core dispatcher, so a
  user mutation and a sync never interleave inside the process; cross-process staleness is
  what the CAS guard is for.
- **v2: WorkManager** periodic sync (15-min minimum, network-constrained, backoff), one
  unique work name so it never overlaps the foreground path (both funnel through the same
  repository dispatcher).

### Notifications

- **v1**: in-app due banner only (`DuePromptsJSON` on resume). No permissions needed.
- **v2**: check-in reminders. `POST_NOTIFICATIONS` runtime permission (API 33+), one
  channel per prompt type, scheduled via WorkManager windows (good enough for "09:30-ish")
  rather than exact alarms (avoids `SCHEDULE_EXACT_ALARM` policy friction); tapping deep-
  links to the check-in screen. Requires the config source-of-truth fix (M3) so
  morning/evening times are readable on-device.

---

## 6. Project Layout & Build

```
android/
├── settings.gradle.kts          # rootProject "daily-progress-logger-android"; :app
├── build.gradle.kts             # plugin versions via libs.versions.toml
├── gradle.properties            # AndroidX, Jetifier off, parallel
├── gradle/
│   ├── libs.versions.toml       # version catalog (versions pinned below)
│   └── wrapper/                 # Gradle 8.10.x wrapper (committed)
├── gradlew / gradlew.bat
├── core/
│   └── core.aar                 # output of `make android-core` — gitignored, never committed
└── app/
    ├── build.gradle.kts
    └── src/main/
        ├── AndroidManifest.xml
        ├── kotlin/com/cristim/dailyprogress/
        │   ├── App.kt                    # Application: AppContainer, Mobilecore.open
        │   ├── core/                     # CoreClient, CoreRepository, CoreError, dispatcher
        │   ├── model/                    # DTOs from §2
        │   ├── auth/                     # AppAuth flow, TokenStore (EncryptedSharedPrefs)
        │   ├── sync/                     # SyncManager (v1 foreground; v2 Worker slots in here)
        │   └── ui/                       # theme/, nav/, day/, checkin/, weekly/, backlog/,
        │                                 # recurring/, recycle/, projects/, settings/
        └── res/
```

**Consuming the AAR**: the app module depends on the file directly —
`implementation(files("../core/core.aar"))`. Local-file AAR deps are supported in
*application* modules under AGP 8.x (the restriction hits *library* modules); no flatDir
repo, no wrapper module needed. `make android-core` (already in the Makefile:
`gomobile bind -target=android -androidapi 21 -o android/core/core.aar ./mobilecore`) is
the single producer; a `preBuild` Gradle check fails fast with "run `make android-core`"
when the file is missing.

**Pinned versions** (all verified compatible with JDK 17 + platform android-35 +
build-tools 35.0.1):

| Component | Version |
|---|---|
| Gradle wrapper | 8.10.2 |
| AGP | 8.7.3 |
| Kotlin + `org.jetbrains.kotlin.plugin.compose` | 2.0.21 |
| kotlinx-serialization plugin/runtime | 2.0.21 / 1.7.3 |
| Compose BOM | 2024.12.01 |
| Navigation Compose | 2.8.5 |
| Lifecycle (viewmodel-compose, runtime-compose) | 2.8.7 |
| AppAuth | 0.11.1 |
| security-crypto | 1.1.0-alpha06 |
| WorkManager (v2) | 2.10.0 |

**ABIs**: gomobile builds arm64-v8a, armeabi-v7a, x86, x86_64 by default. Keep debug lean
via `ndk.abiFilters += listOf("arm64-v8a", "x86_64")` (device + emulator); release ships
as an **app bundle** so Play splits per-ABI (or `make android-core` moves to
`-target=android/arm64,android/amd64` if armeabi-v7a support is formally dropped — decide
at release time, not now).

**CI** (`.github/workflows/mobile.yml` android job — the placeholder step is already
there): after "Build Core.aar" add

```yaml
      - name: Set up Gradle
        uses: gradle/actions/setup-gradle@v4
      - name: Build Android app
        run: ./android/gradlew -p android assembleDebug
      - name: Unit tests
        run: ./android/gradlew -p android testDebugUnitTest
      - name: Upload debug APK
        uses: actions/upload-artifact@v4
        with: { name: app-debug.apk, path: android/app/build/outputs/apk/debug/app-debug.apk }
```

The runner already has JDK 17 (`setup-java` step) and the Android SDK; the aar is produced
by the preceding step, so ordering is inherent. `sdkmanager "platforms;android-35"
"build-tools;35.0.1"` only if the runner image lacks them (currently it doesn't).

---

## 7. Build Sequence (ordered, independently verifiable)

Each step ends green on `./gradlew -p android assembleDebug` (plus the listed extra check).

1. **Scaffold**: `android/` Gradle project per §6 (wrapper, version catalog, empty Compose
   `MainActivity` with Material 3 theme). *Verify*: `assembleDebug`; APK installs and shows
   a blank screen on an emulator.
2. **Wire the AAR**: run `make android-core`; add the `files(...)` dependency + missing-aar
   preBuild check; call `Mobilecore.open(filesDir/data, clientID, deviceID)` in `App`
   on the core dispatcher and log a `TreeJSON(today)` round-trip. *Verify*: `assembleDebug`;
   logcat shows tree JSON (empty tree on first run).
3. **Decoding layer**: §2 DTOs, `CoreClient`, `CoreRepository`, `CoreError` prefix parser.
   *Verify*: JVM unit tests decoding fixture JSON for every DTO (copy fixtures from
   `mobilecore/core_test.go` expected outputs) + error-prefix mapping tests;
   `testDebugUnitTest` green.
4. **Day screen (read-only)**: screen 1 — tree rendering with sections, rollup, expansion
   state, date navigation, pull-to-refresh. *Verify*: seed `filesDir/data` with sample
   markdown (adb push or a debug-only seeding button), tree renders; date nav re-fetches.
5. **Task actions**: screens 2 + 3 — all mutations with (date, index, expectedText),
   refresh-after-mutation, CAS_MISMATCH snackbar path (simulate by editing the file via
   `adb shell run-as` between render and tap). *Verify*: each action against seeded data;
   CAS path shows refresh snackbar and no wrong-task mutation.
6. **Check-ins**: screens 4 + 5 + due banner. *Verify*: morning adopt/new-items writes the
   plan; evening triage moves items (check tomorrow's date view and backlog).
7. **Weekly**: screens 6/6a/6b/6c incl. oldest-first review loop. *Verify*: plan goals
   round-trip; review keep/postpone/drop reflected in backlog; summary marks summarized
   and the pending badge clears.
8. **Backlog / Recurring / Recycle / Projects**: screens 7-10. *Verify*: adopt appears in
   today's tree; `@daily` template materializes on next Day refresh; restore returns an
   item to its day; project rename reflected in tree section headers.
9. **Sync**: AppAuth flow, token store, screens 11 + 12, on-resume sync, envelope-token
   persistence, `SYNC_AUTH` reconnect path. *Verify*: end-to-end against a real Drive
   account — desktop edit ⇄ phone edit, conflict created and resolved each of the three
   ways; token survives access-token expiry (wait >1h or shorten expiry manually).
10. **CI + polish**: extend `mobile.yml` per §6; app icon; `dataExtractionRules`.
    *Verify*: green `mobile.yml` run building aar → APK on a PR.
11. **v2 (separate plans)**: WorkManager background sync; reminder notifications;
    drag reorder/nest; preferences editing (after core M3 lands).

---

## 8. Risks & Unknowns

- **gomobile ↔ Kotlin interop**: generated Java has no nullability annotations → platform
  types (`String!`); treat every return as non-null only on the success path and keep all
  gomobile touches inside `CoreClient`. Go `int` → Java `long`: all index params are
  `Long` (DTO field is `Long` from the start so no lossy casts). Errors arrive as plain
  `java.lang.Exception` whose `message` is the Go error string — the coded-prefix contract
  is the only machine-readable channel; if a prefix is missing we fall to `Unknown`
  (degraded UX, not corruption).
- **Threading**: Core calls are synchronous file I/O — never on the main thread (ANR).
  The core is gaining an internal mutex (review M2), but the app still serializes on one
  dispatcher: two interleaved read-modify-write calls are individually safe yet
  last-write-wins, so ordering must stay deterministic in-process. `SyncNow` can take
  seconds; it shares the same serial dispatcher, so user mutations queue behind it —
  acceptable for v1 (show a sync indicator), revisit if it bites.
- **CAS is best-effort** (review M1: TOCTOU window, duplicate-text blind spot). The
  refresh-after-mutation + refresh-on-CAS_MISMATCH discipline is the mitigation; do not
  build optimistic UI or auto-retry on top of it in v1.
- **Config coupling** (review M3): `ConfigJSON`/`SetConfig`/`DuePromptsJSON` read desktop
  config via `os.UserConfigDir()`, which is unreliable under Android's process env. v1
  only consumes `DuePromptsJSON`, which falls back to defaults (09:30/17:30/Fri 17:00) —
  acceptable; preferences editing and notifications stay v2 until the core relocates
  config under `dataDir`.
- **AAR size**: 4 ABIs × Go runtime ≈ 40-80 MB uncompressed lib payload. Debug abiFilters
  (arm64-v8a + x86_64) and release app-bundle splits keep installed size ~15-20 MB;
  confirm with `apkanalyzer` at step 2 and revisit `-target` if worse.
- **Toolchain compatibility**: AGP 8.7.3 ⇔ Gradle 8.10.x ⇔ JDK 17 ⇔ compileSdk 35 is a
  known-good matrix; NDK 26.3 only feeds `gomobile bind` (set `ANDROID_NDK_HOME`; CI uses
  `ANDROID_NDK_LATEST_HOME`, fine since gomobile supports r23+). Risk: `gomobile@latest`
  is unpinned in CI — pin it (`golang.org/x/mobile@v0.0.0-…`) the first time a
  breakage appears, or preemptively during step 10.
- **Contract drift while iOS/Android build in parallel**: both hosts hardcode the target
  contract; any core wire change (key rename, new error code) must update
  `mobilecore/core_test.go` fixtures AND both decoding layers. Mitigation: the fixture-
  driven decode tests in step 3 fail CI on drift (fixtures copied from core test goldens);
  keep DTO names/fields aligned with the iOS `Codable` structs one-to-one.
- **AppAuth redirect capture**: the custom-scheme redirect requires the manifest
  `RedirectUriReceiverActivity` intent filter to exactly match the reversed client ID;
  a mismatch fails silently (Custom Tab just closes). Test on a clean emulator early in
  step 9, including the no-browser edge case.
