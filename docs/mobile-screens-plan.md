# Mobile Screens Implementation Plan — iOS + Android Feature Parity

Date: 2026-07-17 · Branch: `feat/unified` · Author: planning pass for phases A–I (tasks #36–#44)

This plan drives Sonnet implementers phase by phase. Every phase specifies, per
platform: files, DTOs, client/repository methods, store/viewmodel shape, view
composition, edge cases, commit breakdown, and tests. The **frozen contract** is
`mobilecore/dto.go` + the `mobilecore/*.go` method surface; the **behavioral
reference** is the Qt app (`internal/ui/*.go`); the **code pattern** is the
merged Today/Day implementation
(`ios/DailyProgress/Features/Today/*`, `android/.../ui/day/*`).

---

## Information Architecture

Matches how Qt organizes the same features (main window = daily tree; File/tray
menu = weekly plan, backlog, summary, review; Preferences = settings + Drive;
check-ins = modal dialogs on top of everything).

### iOS — `RootTabView` (already scaffolded, keep as-is)

| Tab | Screen | Content |
|---|---|---|
| Today | `TodayView` (exists) | daily tree; phase I adds drag; toolbar menu gains "Morning Check-in…" / "Evening Check-in…" (phase A) |
| Week | `WeekView` (stub → phase B) | weekly plan (goals), summary (done-by-day), review entry, week navigation |
| Backlog | `BacklogView` (stub → phase C) | Current + Next-week lists, adopt/shuttle |
| More | `MoreView` (exists) | Projects, Recurring, Recycle Bin, Sync & Account, Settings (stubs → phases E, D, F, H, G) |

Check-in sheets (`MorningCheckinSheet` / `EveningCheckinSheet`) and the
week-review / weekly-summary prompt sheets are **modal sheets presented from
`RootTabView`**, routed by `AppState.duePrompts` — the mobile analog of Qt's
modal check-in dialogs that appear over whatever is open (`app.go runPrompt`).
Manual entry points mirror Qt's tray menu: Today toolbar menu (morning/evening)
and WeekView toolbar (plan / summary / review).

### Android — bottom navigation + nav graph (new in phase A)

`ui/nav/NavGraph.kt` grows a `Scaffold` host (`ui/nav/RootScaffold.kt`) with a
Material 3 `NavigationBar`:

| Item | Route | Screen |
|---|---|---|
| Today | `day/{date}` | `DayScreen` (exists) |
| Week | `week` | `WeekScreen` (phase B) |
| Backlog | `backlog` | `BacklogScreen` (phase C) |
| More | `more` | `MoreScreen` (phase A skeleton) → nested routes `projects`, `recurring`, `recycle`, `sync`, `settings` |

Check-in flows are **dialog destinations / full-screen sheets** launched from a
`CheckinCoordinator` (phase A) that mirrors iOS's `AppState.duePrompts`
routing, plus manual menu entries on the Day top bar.

**Cross-platform IA invariant**: the same feature lives in the same place on
both platforms (Today, Week, Backlog as top-level; Projects/Recurring/Recycle/
Sync/Settings under More), and check-ins are always modal, never tabs.

---

## Shared conventions (apply to every phase)

These are the rules the Today/Day implementation already follows. Every new
screen copies them exactly; deviations are review findings.

1. **One core handle, serialized.** iOS: all calls via the `CoreClient` actor
   behind the `CoreAPI` protocol; stores receive `any CoreAPI`. Android: all
   calls via `CoreRepository` on `Dispatchers.IO.limitedParallelism(1)`;
   ViewModels never touch `mobilecore.Core` directly.
2. **No optimistic UI.** Every mutation = call → re-fetch the read endpoint →
   replace state wholesale. `AppState.bumpDataVersion()` (iOS) after every
   mutation so sibling tabs refresh; Android equivalent: a shared
   `DataVersion` (`MutableStateFlow<Int>` in `AppContainer`, phase A) that all
   ViewModels observe and bump — Android currently has no cross-screen
   invalidation; this is required once more than one screen mutates data.
3. **CAS handling** (only for endpoints taking `(date, index, expectedText)`):
   catch `CoreError.casMismatch` / `CoreError.CasMismatch` → silent refresh +
   toast/snackbar "List changed — please retry." Never auto-retry the mutation.
4. **Error surfacing regardless of loaded state** (the Today-review bug class):
   loading failure with no content → full-screen error state with Retry;
   mutation/refresh failure with content on screen → alert (iOS) / snackbar
   (Android). An error must never be swallowed because content is present, and
   content must never be blanked because a refresh failed.
5. **Fail-loud decode.** All enums decode strictly (`ItemState`, `TaskState`
   pattern): unknown value → decode error → `CoreError.contractViolation` /
   `CoreError.ContractViolation`. No silent defaults for *semantic* fields.
   Kotlin `= emptyList()` defaults are allowed only for collections documented
   as "always [] never null" and for genuinely-absent `omitempty` fields.
6. **Decode in one place.**
   - iOS: phase A introduces `CoreKit/CoreDecoding.swift` with
     `CoreDecoding.decode<T>(_:from:) throws -> T` (wraps `JSONDecoder`,
     rethrows as `.contractViolation`) and `encode<T>(_:) throws -> String`.
     `TodayStore.decode` and `AppState.refreshDuePrompts` are refactored onto
     it (fixes known-issue "bare JSONDecoder + swallowed errors").
   - Android: already centralized in `CoreRepository.call` — keep it there;
     new repo methods must decode inside `call { }`.
7. **Dates and timestamps.** iOS: `Date.coreDate` / `DateFormatting`;
   **never** a new formatter. Android: `LocalDate.toString()` for wire dates;
   for `DuePromptsJSON` **always** `OffsetDateTime.now().format(DateTimeFormatter.ISO_OFFSET_DATE_TIME)`
   (local offset, never `Instant`/UTC — known-issues.md). Phase A adds
   `Time.kt` helper `nowRfc3339Local()` so no call site can get this wrong.
8. **Empty JSON payloads**: send real payloads, never `""`/`"null"`.
   `SetWeeklyPlan` in particular requires at least `[]` (core rejects
   null/empty by design).
9. **Both platforms model the same DTO with the same optionality.** The
   per-phase tables below say exactly which fields are optional. Where Go has
   `omitempty` on a *string*, iOS uses `String?` and Android uses a default
   `""` **only if** empty-vs-absent is not semantically different (e.g.
   `taskDTO.project`); where it is different, both sides use nullable.
10. **Confirmations**: destructive one-tap actions that are recoverable
    (Delete task → recycle bin) need no confirm (matches Qt). Irrecoverable
    ones (Purge recycled, Close project, Remove recurring template) get a
    confirmation dialog on both platforms.
11. **Reuse**: iOS toast overlay pattern (TodayView `.overlay(alignment: .bottom)`)
    is extracted in phase A to `Support/ToastView.swift` (`.toast(_:)` modifier)
    instead of being copy-pasted into each new screen. Android reuses
    `SnackbarEvent` + `Channel` exactly as `DayViewModel`.
12. **Commit style**: conventional commits, small and atomic, per platform:
    `feat(ios): …` / `feat(android): …` / `test(ios): …` etc. Each phase lands
    as its own PR train per platform (or one PR with the listed commits).

### Client/Repository surface audit (what exists vs what phases must add)

Missing today and added in the phase that needs it:

| Method (core) | iOS `CoreAPI`/`CoreClient` | Android `CoreRepository` | Added in |
|---|---|---|---|
| `MakeSubtask` | **missing** | exists (`makeSubtask`) | I |
| `ReorderTask` | **missing** | **missing** | I |
| `UnassignTaskProject` | **missing** | exists | E |
| `AssignTaskProject` | not needed — use `moveTaskToProject` (core alias; do NOT add a second path) | same | — |
| `ConfigJSON` / `SetConfig` | exists | **missing** (`config()`, `setConfig(patchJson)`) | G |
| `SyncWithFileTokenStore` | not exposed to hosts (CLI-only by design) | same | — |

Everything else already exists on both sides.

---

## Phase A — Check-ins (morning + evening) + prompt routing

Qt reference: `dialogs.go buildMorningDialog/buildEveningDialog`, `app.go
CheckPrompts/runPrompt` (snooze 1h capped at 23:59:59, skip-today, manual vs
scheduled semantics), `statebuttons.go eveningChoices` +
`store.EveningActionForState` (done→Done, todo→Todo, **postponed→NextWeek**
initial selection).

Core endpoints: `DuePromptsJSON(nowRFC3339)`, `MorningCandidatesJSON(date)`
(**bare array**, text may include raw `#project` tags — display as-is, matches
Qt), `ApplyMorning(date, {new_items, adopted})`, `ApplyEvening(date,
{decisions:[{text,action}], extra_done})`, actions 0=todo 1=done 2=next_day
3=next_week 4=backlog.

Behavior to match (both platforms):
- **Morning sheet**: title "What are you planning to work on today?"; weekly
  goals shown read-only when planned && non-empty (via `WeeklyPlanJSON`, done
  goals struck through); "Already planned today: N items" summary line when
  the tree already has plan items (from `TreeJSON` counts — flatten projects +
  unfiled); multiline "one task per line" input; candidate checklist with
  "(backlog)" suffix, **backlog candidates default unchecked, same-week
  carry-overs default checked**; empty-candidate case just hides the list.
  Apply → `ApplyMorning` with `new_items` = trimmed non-empty lines, `adopted`
  = checked candidates (exact `{text, from_backlog}` objects passed back).
- **Evening sheet**: title "How did today go?"; one row per plan item —
  sourced from `TreeJSON` flattened pre-order (projects' tasks then unfiled,
  children inline, indent by `depth`; `ApplyEvening` matches by normalized
  text so tree-vs-file ordering differences are safe); per-row 5-way selector
  (Done / Not done / Next day / Next week / Backlog) seeded from state via the
  `EveningActionForState` mapping above; "No plan was recorded for today."
  empty state; "Anything else you accomplished?" multiline → `extra_done`.
- **Prompt routing** (mobile analog of `CheckPrompts`): on app
  launch/foreground, call `DuePromptsJSON(now-local-offset)`; for each due
  prompt not snoozed/skipped today, present its sheet (one at a time,
  morning→evening ordering as returned). Buttons per Qt `attachButtons`:
  scheduled presentation → **OK / Remind me in 1h / Skip Today**; manual
  presentation (menu-invoked) → **OK / Remind me in 1h / Close** (Close does
  no bookkeeping). Snooze = suppress until now+1h capped at end of day (and
  reschedule a local notification for that time in phase G); Skip Today =
  persist `skippedOn[prompt] = today` so it stays quiet until tomorrow.
  Snooze/skip state is **host-side** (UserDefaults / SharedPreferences), keyed
  by prompt id — the core deliberately owns only "due", not "snoozed".
- Week-review and weekly-summary prompts (ids 0, 4) route to the phase-B
  sheets; until phase B lands they are ignored by the router (explicit
  `// phase B` switch arm, not a silent default). Weekly-plan prompt (id 1)
  also routes to phase B's plan sheet.

### iOS

1. **Files**: replace stubs
   `Features/Checkins/{CheckinStore,MorningCheckinSheet,EveningCheckinSheet}.swift`;
   new `CoreKit/CoreDecoding.swift`, `Support/ToastView.swift`,
   `Features/Checkins/CheckinCoordinator.swift` (prompt routing + snooze/skip
   persistence); modify `App/AppState.swift` (use `CoreDecoding`, expose
   `refreshDuePrompts` on foreground via `scenePhase`), `Features/RootTabView.swift`
   (present routed sheets), `Features/Today/TodayView.swift` (toolbar menu with
   "Morning Check-in…" / "Evening Check-in…" manual triggers).
2. **DTOs**: all exist (`Checkin.swift`, `Prompts.swift`). Reuse
   `MorningCandidate`, `MorningDecisions`, `EveningDecisions`,
   `EveningItemDecision`, `EveningAction`, `DuePrompts`, `PromptID`. No new
   DTOs. Encode payloads with `CoreDecoding.encode` (snake_case handled by
   CodingKeys already).
3. **Client methods**: all exist (`morningCandidatesJSON`, `applyMorning`,
   `applyEvening`, `duePromptsJSON`). None to add.
4. **Store**: `CheckinStore` (@Observable @MainActor, injected `any CoreAPI`):
   - state: `morningCandidates: [MorningCandidate]?`, per-candidate
     `adoptedFlags: [Bool]`, `weeklyGoals: [WeeklyGoal]`, `alreadyPlannedCount:
     Int`, `eveningItems: [EveningItem]` (`struct EveningItem { text, depth,
     action: EveningAction }`), `isLoading`, `errorMessage`, `toast`.
   - actions: `loadMorning(date:)` (parallel: candidates + weeklyPlan + tree
     count), `applyMorning(date:newItemsText:) async -> Bool`,
     `loadEvening(date:)` (tree → flatten), `applyEvening(date:extraText:)
     async -> Bool`. Return Bool so the sheet dismisses only on success.
   - No CAS here (check-in endpoints are text-matched, not indexed); errors →
     `errorMessage` alert inside the sheet, sheet stays open (Qt keeps the
     dialog open on apply error).
5. **Views**: `MorningCheckinSheet` — Form with goals section (read-only),
   planned-summary row, TextEditor, candidate toggle list; toolbar
   OK/snooze/skip-or-close per presentation mode (`enum CheckinPresentation
   { scheduled, manual }` passed in). `EveningCheckinSheet` — List of rows:
   indented text + segmented/menu 5-way `Picker` per row with SF Symbols
   (checkmark, xmark, arrow.right, arrow.up, tray), TextEditor for extras,
   same toolbar. `CheckinCoordinator`: owns `snoozeUntil:[Int:Date]`,
   `skippedOn:[Int:String]` in UserDefaults, computes `nextPresentable(from:
   [DuePrompt])`, invoked on `.task` + `scenePhase == .active` +
   post-notification-tap (phase G).
6. **Edge cases**: apply with all candidates unchecked and empty editor is
   legal (records MorningDone, matches Qt OK-with-nothing); evening with no
   plan → still allow OK (records EveningDone + extras); JSON encode failures
   → contractViolation alert; candidate list uses **array index as
   `Identifiable` id** (`enumerated()`), not `text` (duplicate texts are legal
   — same bug class as the RecycleEntry id collision); errors surface inside
   the sheet even while content is loaded (rule 4).
7. **Commits**:
   1. `feat(ios): add CoreDecoding + ToastView helpers, adopt in TodayStore/AppState`
   2. `feat(ios): morning + evening check-in sheets with CheckinStore`
   3. `feat(ios): prompt routing via CheckinCoordinator (due/snooze/skip) + manual menu entries`
   4. `test(ios): check-in decode/encode + coordinator snooze/skip tests`
8. **Tests** (`DailyProgressTests`): decode bare `[MorningCandidate]` array
   fixture; encode `MorningDecisions`/`EveningDecisions` and assert exact
   snake_case JSON (`new_items`, `from_backlog`, `extra_done`, int actions);
   `EveningAction`/`PromptID` unknown-value decode fails loud;
   CheckinCoordinator: skip persists per-day, snooze caps at end of day,
   manual presentation never sets skippedOn.

### Android

1. **Files**: new `ui/nav/RootScaffold.kt` (bottom bar + NavHost),
   `ui/more/MoreScreen.kt` (menu skeleton with disabled placeholders for
   later phases), `ui/checkin/{CheckinViewModel,MorningCheckinScreen,EveningCheckinScreen}.kt`,
   `ui/checkin/CheckinCoordinator.kt` (due-prompt routing + snooze/skip in
   SharedPreferences), `util/Time.kt` (`nowRfc3339Local()`); modify
   `MainActivity.kt` (host RootScaffold), `ui/nav/NavGraph.kt` (routes
   `week`/`backlog`/`more` + checkin dialogs), `App.kt` (`AppContainer` gains
   `dataVersion: MutableStateFlow<Int>`), `ui/day/DayScreen.kt` (top-bar
   overflow menu: Morning/Evening check-in; collect `dataVersion` to refresh).
2. **DTOs**: exist (`MorningCandidateDto`, `MorningDecisionsDto`,
   `EveningDecisionDto/EveningDecisionsDto`, `DuePromptDto/DuePromptsDto`).
   **Consistency fixes**: add `enum class EveningAction(val wire: Int)`
   {TODO(0), DONE(1), NEXT_DAY(2), NEXT_WEEK(3), BACKLOG(4)} and
   `enum class PromptId(val wire: Int)` {WEEK_REVIEW(0), WEEKLY_PLAN(1),
   MORNING(2), EVENING(3), WEEKLY_SUMMARY(4)} with a fail-loud
   `fromWire(int)` that throws on unknown — mirrors iOS's typed enums; the
   serialized DTOs keep `Int` on the wire, the UI layer converts through the
   enum immediately (no bare magic ints in ViewModel/UI).
3. **Repository**: all methods exist (`morningCandidates`, `applyMorning`,
   `applyEvening`, `duePrompts`). None to add. All DuePrompts call sites use
   `Time.nowRfc3339Local()`.
4. **ViewModel**: `CheckinViewModel(repository, dataVersion)`:
   `sealed interface CheckinUiState { Loading; Morning(candidates, adopted:List<Boolean>, goals, plannedCount); Evening(items:List<EveningItem>); Error(CoreError) }`
   with `data class EveningItem(text, depth, action: EveningAction)`;
   actions `loadMorning(date)`, `toggleCandidate(i)`, `setEveningAction(i, a)`,
   `applyMorning(date, newItemsText): emits Done event`, `applyEvening(...)`;
   snackbar `Channel<SnackbarEvent>`; on success bump `dataVersion`. Errors on
   apply keep the screen open + snackbar (rule 4).
5. **Screens**: `MorningCheckinScreen` / `EveningCheckinScreen` as full-screen
   dialog destinations (`dialog()` nav or full route): same sections as iOS;
   evening per-row action = `SingleChoiceSegmentedButtonRow` (5 icon
   segments) or a compact `ExposedDropdownMenu` — pick segmented icons to
   match Qt's icon-button row; buttons bottom bar OK / Remind in 1h /
   Skip today-or-Close per presentation mode. `CheckinCoordinator`: called
   from `RootScaffold` `LaunchedEffect(lifecycle resume)`; reads
   `repository.duePrompts(nowRfc3339Local())`, filters snoozed/skipped,
   navigates to the first presentable check-in route.
6. **Edge cases**: same as iOS list; additionally — candidate list keys use
   index (duplicate texts legal); `PromptId.fromWire` unknown → log + ignore
   that prompt (advisory path) but never crash; DuePrompts failure is
   non-fatal (advisory) yet logged, matching iOS.
7. **Commits**:
   1. `feat(android): bottom navigation scaffold + More skeleton + shared dataVersion`
   2. `feat(android): typed EveningAction/PromptId enums + Time.nowRfc3339Local`
   3. `feat(android): morning + evening check-in screens with CheckinViewModel`
   4. `feat(android): check-in prompt routing (due/snooze/skip) + manual menu entries`
   5. `test(android): check-in DTO encode/decode + coordinator tests`
8. **Tests** (`DtoDecodingTest` + new `CheckinTest`): encode
   `MorningDecisionsDto`/`EveningDecisionsDto` → exact JSON keys; decode bare
   morning-candidate array; `EveningAction.fromWire(9)` and
   `PromptId.fromWire(9)` throw; coordinator skip/snooze persistence; assert
   `nowRfc3339Local()` output matches `\+|-\d{2}:\d{2}$` (never `Z`) —
   regression for the DuePrompts offset bug.

**Platforms must agree on**: candidate default-checked rule; evening initial
action mapping (incl. postponed→NextWeek); snooze cap at end of day;
skip-today persistence semantics; presenting one sheet at a time; empty
decisions payloads still allowed on OK.

---

## Phase B — Weekly: plan, summary, week review

Qt reference: `dialogs.go buildWeeklyPlanDialog/buildWeeklySummaryDialog/
buildWeekReviewDialog`, `app.go runWeekReviewLoop` (oldest-first loop until
not-accepted), `runWeeklySummaryForNow` (same loop), manual-vs-scheduled
`markOnAccept`/`rollover` semantics.

Core: `WeeklyPlanJSON(date)` → `{week, planned, goals:[{text,done}]}`;
`SetWeeklyPlan(date, goalsJSON)` (full-array replace; **never "" / "null"**,
`[]` clears explicitly); `WeeklySummaryJSON(date)` → `{week,start,end,
summarized,reviewed,goals,done_by_day:[{date,items}]}`;
`WeeklySummaryPendingJSON(date)` / `UnreviewedWeekJSON(date)` →
`{pending, week?}` (`week` present only when pending — iOS `String?`; Android
default `""` acceptable since consumers only read it when `pending`);
`WeekReviewCandidatesJSON(date)` → `{week, candidates:[string]}`;
`ApplyWeekReview(date, {decisions:[{text,action}], rollover})`, 0=keep
1=postpone 2=drop.

Key date mechanics: all weekly endpoints take **any date inside the target
week**. Hosts derive: current week = today; previous week = today−7d; the
pending loops re-query `UnreviewedWeekJSON`/`WeeklySummaryPendingJSON` after
each apply and feed the returned `week`'s Monday back in. Add a shared helper
to convert `"2026-W29"` → its Monday date (ISO-8601 week parsing:
iOS `DateFormatting.date(fromISOWeek:)` using `Calendar(identifier:
.iso8601)`; Android `WeekFields.ISO`/`LocalDate.parse` helper in `Time.kt`).
Unit-test this on both platforms with year-boundary weeks (e.g. `2026-W01`,
`2025-W53`).

Behavior to match:
- **Weekly Plan** (tab section + sheet): show goals with done/not-done toggle;
  add "big things" one per line; save = `SetWeeklyPlan` with the **complete**
  rebuilt array (existing goals with possibly-changed states + appended new
  todos). Ticking a goal mid-week re-opens/re-saves the same way.
- **Weekly Summary**: goals (struck when done) + "Done this week" grouped by
  day (weekday + date header, items list), total count, "Nothing completed yet
  this week." empty state. **Scheduled path marks summarized on OK; manual
  view never does** (`markOnAccept` flag). Qt's "Open Weekly File" button is
  n/a on mobile (no Files-app flow in v1; the summary content is fully
  rendered in-app).
- **Week Review**: list of open items for the reviewed week, per-row 3-way
  Keep-on-backlog / Postpone-to-next-week / Drop, default Keep; empty state
  "Nothing left open from <week>. Great job!" with plain OK. Scheduled path:
  `rollover=true` and **loop oldest-first** over `UnreviewedWeekJSON` until
  `pending=false` or the user snoozes/skips. Manual path ("Review Last
  Week…"): previous week, `rollover=false`, single pass.

### iOS

1. **Files**: replace `Features/Week/{WeekStore,WeekView}.swift`; new
   `Features/Week/{WeeklyPlanSheet,WeekReviewSheet,WeeklySummarySheet}.swift`;
   modify `Features/Checkins/CheckinCoordinator.swift` (route ids 0/1/4 to
   these sheets, implementing the oldest-first loops), `Support/DateFormatting.swift`
   (ISO-week → Monday helper).
2. **DTOs**: all exist in `Models/Weekly.swift` (`WeeklyPlan`, `WeeklyGoal`,
   `WeeklySummary`, `DayDone`, `PendingWeek`, `WeekReviewCandidates`,
   `WeekReviewDecisions`, `ReviewItemDecision`, `ReviewAction`). One fix:
   `WeeklyGoal.id`/`DayDone.id` are text/date-based — goals with duplicate
   text are legal; render goal rows by `enumerated()` index instead of relying
   on `Identifiable` (same duplicate-text rule as phase A).
3. **Client methods**: all exist. None to add.
4. **Store**: `WeekStore`:
   - state: `referenceDate: Date` (any day of viewed week; prev/next week
     nav = ±7d), `plan: WeeklyPlan?`, `summary: WeeklySummary?`,
     `reviewCandidates: WeekReviewCandidates?`, `reviewActions: [ReviewAction]`,
     `pendingReviewWeek: String?`, `isLoading`, `errorMessage`, `toast`.
   - actions: `refresh()` (plan + summary in parallel for referenceDate),
     `setGoalDone(index:Bool)` + `addGoals(text:)` → rebuild full array →
     `setWeeklyPlan` → refresh; `loadReview(date:)`,
     `applyReview(date:rollover:) -> Bool`;
     `markSummarized(date:) -> Bool`; loop drivers `nextUnreviewedWeek()` /
     `nextPendingSummaryWeek()` returning the Monday date or nil.
   - No CAS (text-matched endpoints); plan save conflicts are last-write-wins
     by design — note in code comment.
5. **Views**: `WeekView` — NavigationStack; week navigation header (‹ week
   label ›, "This Week" reset); Section "Big things" (goal rows with
   checkmark toggle + "Add…" field), badge/button "Review last week" (shows
   orange badge when `UnreviewedWeekJSON.pending`), Section "Done this week"
   (done-by-day groups, per-day header `Fri 17 Jul`, total footer, empty
   state). Toolbar menu: "This Week's Summary…" (manual, no mark), "Review
   Last Week…" (manual). Sheets reuse the check-in button row (OK / Remind 1h /
   Skip-or-Close) via a shared `CheckinButtonsBar` extracted in this phase.
6. **Edge cases**: `SetWeeklyPlan` always sends `[]` at minimum (rule 8);
   summary for a week with zero dailies renders the empty state, not an
   error; `PendingWeek.week` read only when `pending`; review apply with an
   action for a since-removed item is fine (core matches by text, ignores
   missing); errors during the scheduled loops abort the loop and surface
   (parity with Qt returning the error); duplicate goal texts render correctly.
7. **Commits**:
   1. `feat(ios): week screen with weekly plan section (WeeklyPlanJSON/SetWeeklyPlan)`
   2. `feat(ios): weekly summary view + mark-summarized flow`
   3. `feat(ios): week review sheet + oldest-first review/summary prompt loops`
   4. `test(ios): weekly DTO decode + ISO-week date helper tests`
8. **Tests**: decode fixtures for `WeeklyPlan` (planned true/false),
   `WeeklySummary` (with/without done days), `PendingWeek` both shapes
   (`{"pending":false}` without `week`); `ReviewAction` unknown int fails
   loud; encode `WeekReviewDecisions` exact JSON; ISO-week→Monday helper
   across year boundary; goals-array rebuild preserves order + states.

### Android

1. **Files**: new `ui/week/{WeekViewModel,WeekScreen,WeeklyPlanSheet,WeekReviewScreen,WeeklySummarySheet}.kt`;
   modify `ui/nav/NavGraph.kt` (route `week`, review/summary dialog routes),
   `ui/checkin/CheckinCoordinator.kt` (ids 0/1/4 + loops), `util/Time.kt`
   (ISO-week → Monday).
2. **DTOs**: all exist (`WeeklyPlanDto`, `WeeklyGoalDto`, `WeeklySummaryDto`,
   `DayDoneDto`, `PendingWeekDto`, `WeekReviewCandidatesDto`,
   `ReviewDecisionDto/ReviewDecisionsDto`). Add `enum class ReviewAction(val
   wire: Int)` {KEEP(0), POSTPONE(1), DROP(2)} with fail-loud `fromWire` —
   UI never handles bare ints (mirrors iOS `ReviewAction`).
3. **Repository**: all methods exist (`weeklyPlan`, `setWeeklyPlan`,
   `weeklySummary`, `weeklySummaryPending`, `unreviewedWeek`,
   `weekReviewCandidates`, `applyWeekReview`, `markWeekSummarized`). None to
   add. Note `setWeeklyPlan(date, goals: List<WeeklyGoalDto>)` already
   encodes a JSON array — even an empty list serializes to `[]`, satisfying
   the core's non-null rule.
4. **ViewModel**: `WeekViewModel(repository, dataVersion)`: state
   `WeekUiState { Loading; Content(referenceDate, plan, summary, reviewPending:Boolean); Error }`;
   actions mirror the iOS store 1:1 (`prevWeek/nextWeek/thisWeek`,
   `setGoalDone`, `addGoals`, `loadReview`, `setReviewAction`,
   `applyReview(rollover)`, `markSummarized`, loop drivers). Bumps
   `dataVersion` after mutations; observes it for refresh.
5. **Screens**: `WeekScreen` — top bar week nav; plan section (checkbox rows +
   add field); done-by-day list; review badge/button; overflow menu manual
   entries. Review/summary as dialog destinations sharing the check-in bottom
   button bar composable from phase A.
6. **Edge cases**: identical list to iOS §6, plus: goal row keys by index.
7. **Commits**:
   1. `feat(android): week screen with weekly plan (WeeklyPlanJSON/SetWeeklyPlan)`
   2. `feat(android): weekly summary + mark-summarized flow`
   3. `feat(android): week review screen + oldest-first prompt loops + ReviewAction enum`
   4. `test(android): weekly DTO fixtures + ISO-week helper tests`
8. **Tests**: mirror iOS §8 exactly in `DtoDecodingTest`/new `WeekTest`
   (same fixtures, same edge weeks), plus `ReviewAction.fromWire` fail-loud.

**Platforms must agree on**: full-array `SetWeeklyPlan` semantics; scheduled
vs manual mark/rollover flags; oldest-first loop stop conditions; ISO-week →
Monday conversion; `PendingWeek.week` only-when-pending handling.

---

## Phase C — Backlog

Qt reference: `backlogdialog.go` — two sections "This week" / "Next week";
per-row actions Plan-Today (adopt) and shuttle move; not-found on move shows
the friendly "This item is no longer in the backlog." info (not an error);
refresh after every action; adopt also refreshes the daily plan + shows a
confirmation ("Planned for today: <text>").

Core: `BacklogJSON()` → `{current:[string], next_week:[string]}` (always
arrays); `AdoptFromBacklog(date, text)`; `MoveBacklogItem(text, toNextWeek)`
→ `NOT_FOUND` coded error when the item vanished.

### iOS

1. **Files**: replace `Features/Backlog/{BacklogStore,BacklogView}.swift`.
2. **DTOs**: `Backlog` exists (`Models/Backlog.swift`). Items are bare
   strings; rows identified by `(section, index)` — do **not** use the string
   as `Identifiable` id (duplicates across sections are possible).
3. **Client**: `backlogJSON`, `adoptFromBacklog`, `moveBacklogItem` all exist.
4. **Store**: `BacklogStore`: state `backlog: Backlog?`, `isLoading`,
   `errorMessage`, `toast`; actions `refresh()`, `adopt(text:) `
   (`AdoptFromBacklog(todayDate, text)` → refresh + toast "Planned for
   today" + `appState.bumpDataVersion()`), `move(text:toNextWeek:)`. Error
   mapping: `CoreError.notFound` → toast "This item is no longer in the
   backlog." + refresh (Qt's friendly path); other errors → rule-4 surfacing.
5. **View**: `BacklogView` — List with "This week" / "Next week" sections;
   row = text + swipe actions: leading "Plan Today" (green,
   `arrow.down.circle`), trailing "Next week"/"This week" shuttle
   (`arrow.right`/`arrow.left`); context menu duplicates both. Empty state
   `ContentUnavailableView("Nothing in the backlog", systemImage: "tray")`.
   Pull-to-refresh. Adopt always targets **today** regardless of the Today
   tab's viewed date (Qt uses `time.Now()`).
6. **Edge cases**: NOT_FOUND double-tap race handled as above; adopting the
   last item leaves a live empty state; long texts wrap (no elision needed on
   mobile rows); refresh on `appState.dataVersion` change (evening check-in
   action 4 and Today's "Move to Backlog" both add items).
7. **Commits**:
   1. `feat(ios): backlog screen with adopt + section shuttle`
   2. `test(ios): backlog decode + not-found handling tests`
8. **Tests**: decode `{current, next_week}` incl. empty; store-level test with
   a mock `CoreAPI` asserting NOT_FOUND → toast-and-refresh (not
   errorMessage).

### Android

1. **Files**: new `ui/backlog/{BacklogViewModel,BacklogScreen}.kt`; NavGraph
   route `backlog`.
2. **DTOs**: `BacklogDto` exists. Row keys `(section, index)`.
3. **Repository**: `backlog()`, `adoptFromBacklog`, `moveBacklogItem` exist.
4. **ViewModel**: `BacklogViewModel(repository, dataVersion)`:
   `BacklogUiState { Loading; Content(BacklogDto); Error }`; actions
   `refresh`, `adopt(text)` (today's `LocalDate.now()`), `move(text,
   toNextWeek)`; `CoreError.NotFound` → snackbar "This item is no longer in
   the backlog." + refresh; success adopt → snackbar "Planned for today" +
   bump dataVersion.
5. **Screen**: two labeled sections in one LazyColumn; row with two trailing
   icon buttons (adopt = down-arrow, shuttle = chevron matching direction) —
   Compose swipe is optional sugar; icon buttons are the accessible baseline
   (Qt uses buttons). Empty state text. Pull-refresh via
   `PullToRefreshBox`.
6. **Edge cases**: same as iOS.
7. **Commits**:
   1. `feat(android): backlog screen with adopt + section shuttle`
   2. `test(android): backlog DTO + not-found handling tests`
8. **Tests**: DTO fixtures (populated/empty); ViewModel test with fake
   repository for the NOT_FOUND snackbar path.

**Platforms must agree on**: adopt targets today (not viewed date); NOT_FOUND
is a friendly toast + refresh, never an error state; section names "This
week" / "Next week".

---

## Phase D — Recurring templates

Qt reference: `tree.go addRecurringNode/recurringRow` (text + gray schedule
description + Delete), `mainwindow.go addItem` (recur-tag detection on add).

Core: `RecurringJSON()` → **management shape** `[{text, project, raw}]` (no
schedule fields!); `AddRecurring(text)` → `BAD_INPUT` when no valid
recurrence tag; `RemoveRecurring(raw)`. The **display shape** with
`describe/kind/weekday/month_day/hour/minute` comes only from
`TreeJSON.recurring`.

**Design decision (both platforms)**: the Recurring management screen loads
`TreeJSON(today).recurring` for display (it has `describe`) and uses `raw`
for deletion; `RecurringJSON` is used only as the fallback/simple list.
Simpler alternative — using only TreeJSON — is chosen: one endpoint, full
data, and `raw` is present in both. `RecurringJSON` remains covered by DTO
tests but the screen reads TreeJSON. (Qt's management UI is also built from
the tree.)

### Android — fix the known silent-default issue first

`RecurringTemplateDto` currently defaults `describe=""`, `kind=0`, etc., so
the 3-field management shape is indistinguishable from a real `kind=0`
(known-issues.md). Fix in this phase, mirroring iOS's optionals:

```kotlin
@Serializable
data class RecurringTemplateDto(
    val text: String,
    val project: String = "",
    val describe: String? = null,
    val kind: Int? = null,
    val weekday: Int? = null,
    @SerialName("month_day") val monthDay: Int? = null,
    val hour: Int? = null,
    val minute: Int? = null,
    val raw: String,
)
```

Update `DtoDecodingTest` accordingly (management-shape test asserts `null`,
not `""`/`0`). This exactly matches iOS `RecurringTemplate` optionality.

### iOS

1. **Files**: replace `Features/Recurring/{RecurringStore,RecurringView}.swift`;
   new `Features/Recurring/AddRecurringSheet.swift`.
2. **DTOs**: `RecurringTemplate` exists with correct optionals; `id: raw` is
   fine (raw lines are unique in the store file; duplicates would still
   render — acceptable, matches deletion semantics which remove first match).
3. **Client**: `recurringJSON`, `addRecurring`, `removeRecurring`, `treeJSON`
   all exist.
4. **Store**: `RecurringStore`: state `templates: [RecurringTemplate]?`,
   `isLoading`, `errorMessage`, `toast`; `refresh()` (TreeJSON(today) →
   `.recurring`), `add(text:) -> Bool` (BAD_INPUT → inline field error
   message in the sheet, sheet stays open), `remove(raw:)` (after confirm),
   bump dataVersion after mutations (templates materialize into Today).
5. **Views**: `RecurringView` — List rows: repeat icon + text +
   `describe` caption (secondary) + project name caption when non-empty;
   swipe-to-delete + context menu Delete with confirmation dialog
   ("Delete this recurring task? Already-created occurrences stay.").
   Toolbar +: `AddRecurringSheet` — TextField with helper text explaining
   syntax (`Standup @daily`, `Report @weekly @fri @16:00`, `Rent @monthly
   @1`, optional `@<project-slug>` token), Add disabled when empty; core-side
   BAD_INPUT shown under the field ("needs a recurrence tag like @daily").
6. **Edge cases**: BAD_INPUT keeps sheet open with message (fail-loud, no
   silent drop); template list from TreeJSON is date-independent (templates
   are global) — use today, don't wire the viewed date; `describe == nil`
   (shouldn't happen from TreeJSON) renders no caption rather than empty
   text; removing a template does not remove already-materialized tasks
   (copy explains).
7. **Commits**:
   1. `feat(ios): recurring templates screen (list via TreeJSON, delete with confirm)`
   2. `feat(ios): add-recurring sheet with @-syntax validation surfacing`
   3. `test(ios): recurring decode both shapes + BAD_INPUT path`
8. **Tests**: decode management-shape fixture → optionals nil; decode
   TreeJSON-shape fixture → all fields; mock-core store test: add with
   BAD_INPUT keeps state and surfaces message.

### Android

1. **Files**: new `ui/recurring/{RecurringViewModel,RecurringScreen}.kt`
   (+ add dialog inside the screen file, matching DayScreen's dialog style);
   NavGraph route `recurring` under More; modify `model/Dtos.kt`
   (nullable fix above) + `DtoDecodingTest`.
2. **DTOs**: nullable-fixed `RecurringTemplateDto` (above).
3. **Repository**: `recurring()`, `addRecurring`, `removeRecurring`, `tree`
   exist. None to add.
4. **ViewModel**: `RecurringViewModel(repository, dataVersion)`: state
   `{Loading; Content(templates); Error}` sourced from
   `repository.tree(LocalDate.now().toString()).recurring`; actions `add(text)`
   (BadInput → field-error state in dialog), `remove(raw)` after confirm
   dialog; snackbar channel; bump dataVersion.
5. **Screen**: LazyColumn rows (repeat icon, text, describe caption, project
   caption); trailing delete icon → `AlertDialog` confirm; FAB + →
   AddRecurringDialog with syntax helper text and inline error.
6. **Edge cases**: same as iOS §6.
7. **Commits**:
   1. `fix(android): RecurringTemplateDto nullable schedule fields (no silent defaults)`
   2. `feat(android): recurring templates screen with add/remove`
   3. `test(android): recurring DTO both shapes + BadInput path`
8. **Tests**: updated management-shape fixture asserting nulls; TreeJSON-shape
   fixture; ViewModel BadInput test.

**Platforms must agree on**: list sourced from TreeJSON(today); delete
confirmation copy; BAD_INPUT surfaces inline in the add UI; describe caption
formatting; occurrences-not-deleted explanation.

---

## Phase E — Projects

Qt reference: `tree.go projectRow/renameProject/closeProject`,
`mainwindow.go addProject`. Qt has no dedicated projects screen (tree headers
only) and offers no reopen UI; mobile's Projects screen is a superset that
also exposes `ReopenProject` (core supports it; parity plus).

Core: `ProjectsJSON()` → `[{id, name, status: "open"|"closed"}]`;
`AddProject(name) -> id`; `RenameProject(id,newName)`; `CloseProject(id)`;
`ReopenProject(id)`; task-side `MoveTaskToProject(date,index,expected,projectID)`
("" clears → Unfiled), `UnassignTaskProject(date,index,expected)`. `NOT_FOUND`
coded errors via `codeStoreErr`. Project **slug/id** is what `#slug`
references in raw task text resolve to; hosts never parse `#slug` themselves —
assignment is always explicit via the picker (AddTaskSheet pattern) or
Move-to-project.

### iOS

1. **Files**: replace `Features/Projects/{ProjectsStore,ProjectsView}.swift`;
   modify `Features/Today/TaskRow.swift` (context menu gains "Move to
   Project…" submenu incl. "None (Unfiled)") and `Features/Today/TodayStore.swift`
   already has `moveToProject` — reuse it; `CoreClient`/`CoreAPI` gain
   `unassignTaskProject` **or** we keep using `moveTaskToProject(projectID:
   "")` which the core treats identically — **decision: use
   `moveTaskToProject` with empty id everywhere; do not add the alias**
   (one path, zero new surface; document in code comment).
2. **DTOs**: `Project`/`ProjectStatus` exist (fail-loud enum already —
   unknown status throws).
3. **Client**: `projectsJSON`, `addProject`, `renameProject`, `closeProject`,
   `reopenProject` exist. None to add.
4. **Store**: `ProjectsStore`: state `projects: [Project]?`, `showClosed:
   Bool`, `isLoading`, `errorMessage`, `toast`; actions `refresh()`,
   `add(name:) -> Bool`, `rename(id:newName:)`, `close(id:)` (confirm),
   `reopen(id:)`; NOT_FOUND → toast "Project no longer exists." + refresh;
   bump dataVersion (Today tree shows project sections).
5. **Views**: `ProjectsView` — List section "Open" (rows: folder icon + name
   + slug caption in secondary); section "Closed" behind a
   toggle/DisclosureGroup (collapsed by default; Qt hides closed entirely —
   mobile shows them to host Reopen). Row swipe/context: Rename… (alert with
   TextField), Close (confirm: "Close this project? Its tasks stay; the
   project moves to Closed.") / Reopen for closed rows. Toolbar + → alert
   prompt "Project name:". Empty state "No projects yet."
6. **Edge cases**: rename to empty is blocked client-side (Qt's promptText
   requires non-empty); duplicate names allowed (ids differ — display slug
   caption to disambiguate); closing a project whose tasks are visible in
   Today just removes the section on next refresh (tasks keep their tag;
   parity with Qt); NOT_FOUND races per rule above; status decode fail-loud.
7. **Commits**:
   1. `feat(ios): projects screen (list/add/rename/close/reopen)`
   2. `feat(ios): move-to-project action on Today task rows`
   3. `test(ios): projects decode + status fail-loud + store not-found tests`
8. **Tests**: decode open/closed fixture; unknown status throws; mock-core
   rename/close/NOT_FOUND paths; encode nothing (all scalar params).

### Android

1. **Files**: new `ui/projects/{ProjectsViewModel,ProjectsScreen}.kt`;
   NavGraph route `projects`; modify `ui/day/DayScreen.kt` TaskActionsSheet
   (add "Move to project…" → project-picker dialog using
   `repository.projects()`, incl. "None (Unfiled)" → `moveTaskToProject(...,
   "")`) and `DayViewModel` (add `moveToProject(index, expectedText, date,
   projectId)` via existing repo method, CAS-handled through `mutate {}`).
2. **DTOs**: `ProjectDto`/`ProjectStatus` exist (fail-loud `@SerialName`
   enum).
3. **Repository**: all exist (`projects`, `addProject`, `renameProject`,
   `closeProject`, `reopenProject`, `moveTaskToProject`,
   `unassignTaskProject`). Same decision as iOS: UI uses
   `moveTaskToProject(projectId = "")` for unassign (keep the existing
   `unassignTaskProject` repo method but the picker uses one path).
4. **ViewModel**: `ProjectsViewModel(repository, dataVersion)`: state
   `{Loading; Content(open:List, closed:List, showClosed:Boolean); Error}`;
   actions add/rename/close/reopen with confirm handled in UI; NotFound →
   snackbar + refresh; bump dataVersion.
5. **Screen**: sections Open/Closed (closed collapsed); row overflow menu
   (Rename, Close/Reopen); FAB + name dialog; confirm dialog for Close.
6. **Edge cases**: same list as iOS §6.
7. **Commits**:
   1. `feat(android): projects screen (list/add/rename/close/reopen)`
   2. `feat(android): move-to-project picker on day task sheet`
   3. `test(android): projects DTO + ViewModel not-found tests`
8. **Tests**: mirror iOS.

**Platforms must agree on**: unassign = `MoveTaskToProject` with `""`; closed
projects visible but collapsed with Reopen; close confirmation copy; slug
caption display; non-empty name validation.

---

## Phase F — Recycle bin

Qt reference: `tree.go addRecycleNode/recycleRow` — collapsed section; rows
show text (state-styled), origin project (gray), origin date ("2 Jan");
actions Restore ("Restore to its day") and Delete ("Delete permanently").
Qt has no purge confirmation; **mobile adds one** (irrecoverable + fat-finger
risk on touch; deviation noted deliberately).

Core: `RecycleJSON()` → `[{date, text, state}]` (**no project field** — minor
display-parity gap vs Qt's tree-sourced rows; accepted: mobile rows show date
+ state only). `RestoreTask(date, displayText)`, `PurgeRecycled(date,
displayText)` — both **no-ops when missing** (not errors).

### iOS — fix the known id-collision issue here

`RecycleEntry.id = "date#text"` collides when the same text was deleted twice
on one day (known-issues.md). Fix: stop using `RecycleEntry` as
`Identifiable` directly; the store wraps entries as
`struct RecycleRow: Identifiable { let id: Int  // array index; stable per snapshot
let entry: RecycleEntry }` built via `enumerated()`. Remove the `Identifiable`
conformance + computed `id` from `RecycleEntry` (list identity is a UI
concern; per-snapshot index identity is correct because every mutation
replaces the whole array).

1. **Files**: replace `Features/Recycle/{RecycleStore,RecycleView}.swift`;
   modify `CoreKit/Models/Recycle.swift` (drop Identifiable, per above);
   modify `Features/Today/TodayView.swift` (Recycle Bin placeholder
   NavigationLink now pushes the real `RecycleView`).
2. **DTOs**: `RecycleEntry` (date, text, state:ItemState) exists; change per
   above only.
3. **Client**: `recycleJSON`, `restoreTask`, `purgeRecycled` exist.
4. **Store**: `RecycleStore`: state `rows: [RecycleRow]?`, `isLoading`,
   `errorMessage`, `toast`; actions `refresh()`, `restore(entry:)` (→ toast
   "Restored to <date>" + refresh + bump dataVersion), `purge(entry:)`
   (after confirm), `purgeAll()` optional — **not in scope** (core has no
   bulk purge; skip). Duplicate-entry nuance: restore/purge act on the
   *first* matching (date,text) in the core — with duplicates, two taps
   restore both; acceptable and documented in code comment.
5. **View**: `RecycleView` — List rows: state-styled text (strikethrough when
   done, dimmed when postponed — reuse the TaskRow styling helpers), caption
   "from <d MMM yyyy>"; swipe leading Restore (green), trailing Delete
   Permanently (destructive) with `confirmationDialog` ("Delete permanently?
   This cannot be undone."); context menu duplicates. Empty state
   `ContentUnavailableView("Recycle bin is empty", systemImage: "trash")`.
6. **Edge cases**: restore/purge of a since-synced-away entry is a silent
   no-op per core — always refresh after, so the row disappears either way;
   state decode fail-loud (ItemState reused); id stability across
   refreshes not required (whole-array replace).
7. **Commits**:
   1. `fix(ios): RecycleEntry list identity via array index (id-collision fix)`
   2. `feat(ios): recycle bin screen with restore + purge-with-confirm`
   3. `test(ios): recycle decode + duplicate-entry identity test`
8. **Tests**: decode fixture incl. two identical (date,text) entries →
   distinct row ids; ItemState fail-loud already covered; mock-core
   restore/purge refresh behavior.

### Android

1. **Files**: new `ui/recycle/{RecycleViewModel,RecycleScreen}.kt`; NavGraph
   route `recycle`.
2. **DTOs**: `RecycleEntryDto` exists (no change needed — Compose keys are
   explicit; use the **item index** as the LazyColumn key, mirroring the iOS
   fix; never `"$date$text"`).
3. **Repository**: `recycle()`, `restoreTask`, `purgeRecycled` exist.
4. **ViewModel**: `RecycleViewModel(repository, dataVersion)`: state
   `{Loading; Content(entries); Error}`; `restore(entry)`, `purge(entry)`
   (confirm in UI); snackbar "Restored to <date>"; bump dataVersion.
5. **Screen**: rows with state-styled text + date caption; trailing icons
   Restore / Delete-forever (confirm `AlertDialog`); empty state.
6. **Edge cases**: same as iOS §6; keys by index.
7. **Commits**:
   1. `feat(android): recycle bin screen with restore + purge-with-confirm`
   2. `test(android): recycle DTO + index-keyed duplicates test`
8. **Tests**: fixture with duplicate (date,text) entries decodes to 2 rows;
   ViewModel refresh-after-noop behavior.

**Platforms must agree on**: purge requires confirm; restore toast copy;
index-based row identity; silent-no-op semantics after refresh.

---

## Phase G — Settings + local notifications

Qt reference: `preferences.go` (morning/evening/summary times, summary day
combo, notify-vs-modal checkbox), `gsync.go driveSection` (client ID field),
`app.go CheckPrompts/canNotify/promptNotificationText` (banner text per
prompt, once per day per prompt, click opens the dialog).

Core: `ConfigJSON()` → `mobileConfigDTO` (all fields optional/omitted);
`SetConfig(patchJSON)` — **partial patch**: only non-empty strings / non-null
`notify_checkins` change; corrupt config file → `BAD_INPUT` from both.
**Contract gap (flag)**: a string field can never be cleared back to
empty/default via SetConfig (empty string means "leave unchanged") — notably
`google_client_id` cannot be unset. See Contract gaps §3 at the end.

**Android repository gap**: `config()` / `setConfig()` are missing — added
here. **Android client-ID gap**: `CoreClient.openCore` reads
`BuildConfig.GOOGLE_CLIENT_ID` (compile-time); phase G switches both
platforms to the same runtime scheme (below).

**Client-ID / Core-reopen scheme (both platforms, decide once)**: the core
captures `clientID` at `Open` and uses it for token refresh inside
`SyncNow`. Therefore:
- The host keeps a mirror of the client ID in host storage (iOS
  UserDefaults — already does; Android SharedPreferences replaces
  BuildConfig) and passes it to `Open` at launch.
- Settings "Google Client ID" save = `SetConfig({"google_client_id":…})`
  (so it syncs across devices) **and** host mirror update **and core handle
  re-open** so sync uses it immediately: iOS `AppState.reopenCore()` replaces
  `core` with a fresh `CoreClient` (stores hold `any CoreAPI` by reference —
  they are recreated per-view, safe); Android `CoreClient` gains
  `fun reset(clientId: String)` that nulls `_core` (executed **on the
  dispatcher** so no call races the handle swap).
- On launch, if the mirror is empty but `ConfigJSON.google_client_id` is not
  (config synced from another device), adopt it into the mirror and reopen.

**Notification design (both platforms, v1)**: static local notifications at
the configured times; when tapped, the app foregrounds, runs
`DuePromptsJSON`, and opens the corresponding check-in **only if still due**
(otherwise no-op) — this reproduces Qt's click-to-open while tolerating
fire-when-already-done. Banner titles/bodies copy `promptNotificationText`
exactly ("Morning Check-in" / "What are you planning to work on today?",
etc.). Notifications scheduled for: morning (daily), evening (daily), weekly
summary (weekly on summary_day). Week-review/weekly-plan prompts have no
fixed time (they're Monday-ish states) — v1 surfaces them only on app open
via the phase-A coordinator, matching the "prompt on open" behavior; no
notification. Toggling `notify_checkins` off cancels all scheduled
notifications; on (re)enable or any time-field change, reschedule. Snoozing
a check-in (phase A) schedules a one-shot notification at the snooze
deadline when notifications are authorized.

### iOS

1. **Files**: replace `Features/Settings/{SettingsStore,SettingsView}.swift`;
   modify `Support/Notifications.swift` (finish: cancel-by-id sets, one-shot
   snooze trigger, delegate routing), new `App/NotificationDelegate.swift`
   (`UNUserNotificationCenterDelegate` — foreground presentation + tap →
   `AppState.pendingPromptID` → coordinator), modify `App/AppState.swift`
   (+`reopenCore()`, client-ID mirror), `App/DailyProgressApp.swift`
   (install delegate), `Features/Checkins/CheckinCoordinator.swift`
   (handle `pendingPromptID`, schedule snooze one-shots).
2. **DTOs**: `CoreConfig` exists — all-optional, matches `mobileConfigDTO`
   exactly (incl. `notify_checkins: Bool?`). Patch payloads are encoded from
   a `CoreConfig` containing only the changed field (nil fields drop out via
   `encoder` + omit-nil — use `CoreDecoding.encode` with
   `.withoutEscapingSlashes` and manual nil-skipping via the optional fields;
   verify in tests that nil fields are absent, not `null` — set
   `JSONEncoder` default behavior which omits nils for optionals with
   `encodeIfPresent`; CodingKeys synthesized conformance already does this).
3. **Client**: `configJSON` / `setConfig` exist.
4. **Store**: `SettingsStore`: state `config: CoreConfig?`, editable drafts
   (`morningTime`, `eveningTime`, `summaryTime` as `Date` via
   `DatePicker(hourAndMinute)`, `summaryDay: String` from the fixed
   Monday…Sunday list, `notifyCheckins: Bool`, `googleClientID: String`),
   `isLoading`, `errorMessage`, `toast`; actions `load()` (**errors surface
   — replace the stub's silent catch**; BAD_INPUT = corrupt config file must
   alert, per core docs), `save()` → build patch of changed fields only →
   `setConfig` → reload → reschedule notifications → if clientID changed:
   mirror + `appState.reopenCore()`; `toggleNotifications(on:)` → request
   authorization → schedule or cancel; denied permission → `notifyCheckins`
   stays off + alert pointing to system Settings.
5. **View**: `SettingsView` Form — Section "Check-ins": morning/evening time
   pickers; Section "Weekly summary": day picker (Monday…Sunday) + time;
   Section "Notifications": toggle (+ footnote "Banners open the matching
   check-in"); Section "Google Drive": client ID TextField (monospaced,
   placeholder `xxxxx.apps.googleusercontent.com`) + footnote that sign-in
   lives in Sync & Account; Save button (disabled when unchanged). Times
   convert `Date ⇄ "HH:MM"` via a new `DateFormatting.hhmm` helper (24h,
   POSIX locale — never locale-dependent).
6. **Edge cases**: HH:MM formatting is zero-padded 24h (core parses
   `config.ParseTimeOfDay`); empty draft fields are simply not included in
   the patch (cannot clear — see contract gap; UI never offers a "clear"
   affordance for strings); `notify_checkins` uses true/false explicitly
   (nullable on wire, tri-state honored: nil = default-on per Qt
   `NotifyCheckinsEnabled`); corrupt-config BAD_INPUT renders full error
   state with Retry; notification identifiers namespaced
   (`checkin.morning`, `checkin.evening`, `checkin.summary`,
   `checkin.snooze.<id>`) and **only these** are removed on reschedule
   (never `removeAllPendingNotificationRequests` — don't clobber future
   features).
7. **Commits**:
   1. `feat(ios): settings screen wired to ConfigJSON/SetConfig (partial patch)`
   2. `feat(ios): client-ID mirror + core reopen on change`
   3. `feat(ios): local notifications for check-ins + tap routing + snooze one-shots`
   4. `test(ios): config patch encoding + hhmm helper + notification id tests`
8. **Tests**: `CoreConfig` decode of `{}` (all nil) and full fixture; patch
   encoding omits nil fields (assert absent keys); `hhmm` round-trip incl.
   "07:05"; unknown `summary_day` string passes through (free-form,
   validated by core); notification scheduling unit-testable parts
   (component building from "HH:MM", weekly weekday mapping Mon=2…Sun=1).

### Android

1. **Files**: new `ui/settings/{SettingsViewModel,SettingsScreen}.kt`,
   `notifications/{CheckinNotifications.kt,CheckinAlarmReceiver.kt,BootReceiver.kt}`;
   modify `core/CoreRepository.kt` (**add** `config(): MobileConfigDto`,
   `setConfig(patch: MobileConfigDto)`), `model/Dtos.kt` (**add**
   `MobileConfigDto`), `core/CoreClient.kt` (client-ID from SharedPreferences
   mirror + `reset(clientId)`), `AndroidManifest.xml` (+
   `POST_NOTIFICATIONS`, `RECEIVE_BOOT_COMPLETED`, receivers),
   `ui/checkin/CheckinCoordinator.kt` (notification-tap intent extra
   `prompt_id` routing; snooze one-shot alarms), `MainActivity.kt`
   (intent handling), remove `GOOGLE_CLIENT_ID` from `build.gradle.kts`.
2. **DTOs**: add — must mirror iOS optionality exactly:

   ```kotlin
   @Serializable
   data class MobileConfigDto(
       @SerialName("morning_time") val morningTime: String? = null,
       @SerialName("evening_time") val eveningTime: String? = null,
       @SerialName("summary_day") val summaryDay: String? = null,
       @SerialName("summary_time") val summaryTime: String? = null,
       @SerialName("google_client_id") val googleClientId: String? = null,
       @SerialName("notify_checkins") val notifyCheckins: Boolean? = null,
   )
   ```

   Repository `json` already has `explicitNulls = false`, so nulls are
   omitted on encode — exactly the partial-patch semantics SetConfig expects.
3. **Repository additions**:

   ```kotlin
   suspend fun config(): MobileConfigDto = call { core ->
       json.decodeFromString(core.configJSON())
   }
   suspend fun setConfig(patch: MobileConfigDto) =
       call { core -> core.setConfig(json.encodeToString(patch)) }
   ```
4. **ViewModel**: `SettingsViewModel(repository, dataVersion, notifications)`:
   state `{Loading; Content(config, drafts…); Error}` (corrupt-config
   BadInput → Error state with Retry, not silent defaults); `save()` builds
   a patch DTO of changed fields, calls `setConfig`, reloads, reschedules
   notifications, and on clientID change updates the prefs mirror +
   `coreClient.reset(...)`; `toggleNotifications(on)` requests
   `POST_NOTIFICATIONS` (API 33+) via the screen's permission launcher.
5. **Screen**: same sections/ordering as iOS. Time fields use
   `TimePickerDialog`-backed rows rendering "HH:MM"; summary day =
   `ExposedDropdownMenu` Monday…Sunday.
6. **Notifications impl**: `CheckinNotifications` — a `NotificationChannel`
   "check-ins"; scheduling = `AlarmManager.setWindow` (inexact, ±10 min fine)
   for the **next** occurrence of each enabled time; `CheckinAlarmReceiver`
   posts the banner (content intent = MainActivity with `prompt_id` extra)
   and re-arms the next occurrence; `BootReceiver` re-arms after reboot.
   No exact-alarm permission needed. Cancel by stable request codes on
   toggle-off/reschedule.
7. **Edge cases**: iOS §6 list applies (zero-padded 24h, no clear affordance,
   tri-state notify, namespaced ids ⇒ request codes); additionally:
   permission denied → toggle reverts + snackbar with link to app settings;
   receiver work is trivial (no core call in the receiver — decision noted
   in Notification design; the app re-checks due-ness on open).
8. **Commits**:
   1. `feat(android): MobileConfigDto + config/setConfig repository methods`
   2. `feat(android): settings screen wired to ConfigJSON/SetConfig`
   3. `feat(android): runtime client-ID mirror + CoreClient.reset (drop BuildConfig id)`
   4. `feat(android): check-in notifications (AlarmManager + receivers + tap routing)`
   5. `test(android): config DTO patch encoding + settings ViewModel tests`

   **Tests**: `MobileConfigDto` decode `{}`; encode patch omits nulls
   (assert exact JSON `{"morning_time":"08:30"}`); corrupt-config BadInput →
   Error state; alarm time computation ("next occurrence" across midnight
   and week wrap for summary day).

**Platforms must agree on**: partial-patch build (changed fields only, nulls
omitted); notify tri-state default-on; notification copy from
`promptNotificationText`; tap → re-check DuePrompts → open only if due;
client-ID mirror + reopen scheme; HH:MM zero-padded 24h.

---

## Phase H — Google Drive sync (OAuth + PKCE, host-owned tokens)

Qt reference: `gsync.go` (sign in/out, status line "Connected as <email>",
sync now, auto-sync toggle + 5-min timer, error dedup for background syncs),
`conflicts.go` (list + per-file Keep-this-device / Keep-the-other / Keep-both,
resolve → immediate re-sync).

Core: `SyncNow(tokenJSON)` → `{conflicts:[], token?}` — **token present ⇒
persist to secure storage immediately**; `ConflictsJSON(tokenJSON)` →
`[conflictDTO]`; `ResolveConflict(tokenJSON, path, choice)` with exactly
`keep_local|keep_remote|keep_both` (BAD_INPUT otherwise); auth failures →
`SYNC_AUTH` coded. Token wire shape = `oauth2.Token` JSON:
`{access_token, token_type, refresh_token, expiry}` with **expiry as RFC 3339**
— both hosts must convert their OAuth library's expiry representation to this
exact shape (iOS: `expires_in` seconds → absolute RFC 3339; Android: AppAuth
`accessTokenExpirationTime` epoch-ms → RFC 3339). A malformed token JSON is
BAD_INPUT (host bug, fail loud).

Auth design (per platform, both PKCE S256, no client secret):
- **iOS**: `ASWebAuthenticationSession` with the **iOS-type** client ID from
  Settings; redirect `com.googleusercontent.apps.<id>:/oauthredirect`
  (reversed-client-id scheme registered in Info.plist at build time is not
  possible for a runtime-entered ID — use the session's
  `callbackURLScheme` parameter, which does not require an Info.plist entry);
  scope `https://www.googleapis.com/auth/drive.file` + `email` (email only to
  render "Connected as …"; parse from the ID token or the token response's
  `id_token` claims — no extra network call); token endpoint exchange via
  `URLSession` (form-encoded POST, no SDK dependency).
- **Android**: AppAuth (already a dependency; manifest placeholder activity
  exists) with the **Android-type** client ID; redirect scheme = reversed
  client id — the manifest `<data android:scheme>` placeholder must be set
  via a **manifest placeholder from the runtime mirror is impossible**; use
  AppAuth's `RedirectUriReceiverActivity` with a fixed custom scheme is also
  clientID-dependent ⇒ **decision needed** (see Contract gaps §5): simplest
  robust option is requiring the client ID at build time for Android's
  redirect (gradle `manifestPlaceholders["appAuthRedirectScheme"]` fed from
  `local.properties`), while the *core* still receives the runtime/config
  value. Plan assumes this build-time placeholder.
- Tokens: iOS Keychain via existing `KeychainTokenStore` (add
  `kSecAttrAccount`, keep `AfterFirstUnlock`); Android
  `EncryptedSharedPreferences` (security-crypto dependency already present) in
  new `sync/TokenStore.kt` with the same `load/save/delete` shape.
- **Auto-sync**: on app foreground + after every mutation-burst is overkill;
  match Qt's cadence pragmatically: sync on app foreground/activation and
  after explicit user actions on the Sync screen; a periodic 5-min timer only
  while the app is foregrounded (iOS `Timer` in SyncStore; Android
  `LaunchedEffect` loop in RootScaffold). Background sync (BGTaskScheduler /
  WorkManager) is explicitly **v2**.
- **Error surfacing**: user-initiated sync errors → alert/snackbar with full
  message; foreground-timer sync errors → single deduped toast per error
  category (copy Qt's `syncErrKey` idea: auth errors always surface,
  network errors once per interval); `SYNC_AUTH` → "Sign in again" call to
  action, clears `isSignedIn` display state but **keeps** the token until the
  user re-auths (refresh may recover).

### iOS

1. **Files**: replace `Features/Sync/{SyncStore,SyncView,GoogleAuth,KeychainTokenStore}.swift`;
   new `Features/Sync/ConflictsView.swift`; modify `App/AppState.swift`
   (foreground sync hook), `CoreKit/Models/Sync.swift` (add `expiresIn` →
   expiry conversion helper on `OAuthToken` or a `TokenResponse` type).
2. **DTOs**: `SyncResult`, `SyncConflict`, `ResolveChoice`, `OAuthToken`
   exist. Add `struct TokenResponse: Codable` (`access_token`, `expires_in`,
   `refresh_token?`, `id_token?`, `token_type`) + `func toOAuthToken(now:)`
   producing RFC 3339 `expiry`. `SyncResult.token` is `String?` — persist
   when non-nil **and non-empty**.
3. **Client**: `syncNow`, `conflictsJSON`, `resolveConflict` exist.
4. **Store**: `SyncStore`: state `isSignedIn` (token exists), `accountEmail:
   String?` (from Keychain-adjacent UserDefaults, like Qt's cfg.GoogleAccount),
   `isSyncing`, `lastSyncAt: Date?`, `lastError: String?`, `conflicts:
   [SyncConflict]`, `toast`; actions `signIn()` (GoogleAuth PKCE flow →
   token → Keychain → immediate `syncNow`), `signOut()` (delete token +
   email; no revoke call in v1), `syncNow()` (load token → call → **persist
   returned token if non-empty** → refresh conflicts + bump dataVersion),
   `loadConflicts()`, `resolve(path:choice:)` (→ re-sync, matching Qt).
   Guard `isSyncing` re-entrancy (Qt's `syncing` flag).
5. **Views**: `SyncView` — status header ("Connected as **email**" / "Not
   signed in"); Sign in with Google / Sign out button (busy state while the
   web session runs); "Sync Now" with spinner + "Last synced …" caption +
   last error line; Conflicts row with count badge → `ConflictsView`: intro
   copy ("These files changed on two devices…"), per-row path + three
   buttons Keep this device / Keep the other / Keep both; empty state "No
   conflicts — everything is in sync." Client-ID missing → inline prompt
   linking to Settings (Qt: "enter your Google client ID in Preferences
   first").
6. **Edge cases**: token refresh mid-sync → returned `token` persisted
   before anything else; `SYNC_AUTH` → status flips to "Session expired —
   sign in again" (token kept); resolve with all conflicts gone (synced from
   elsewhere) → NOT_FOUND-ish core error surfaces as toast + refresh; user
   cancels ASWebAuthenticationSession → silent return (not an error);
   concurrent syncNow calls collapse via `isSyncing`; conflicts list uses
   `path` as id (unique per core contract).
7. **Commits**:
   1. `feat(ios): GoogleAuth PKCE sign-in + Keychain token store (runtime scheme)`
   2. `feat(ios): sync screen (sign in/out, sync now, token write-back, status)`
   3. `feat(ios): conflicts list + resolve flow with re-sync`
   4. `feat(ios): foreground auto-sync with deduped error toasts`
   5. `test(ios): token response conversion + sync result decode + store tests`
8. **Tests**: `TokenResponse → OAuthToken` expiry math (RFC 3339, local
   offset ok); `SyncResult` decode with/without `token`; mock-core store
   test: non-empty token persisted, empty/nil not; `ResolveChoice` raw
   values exactly `keep_local/keep_remote/keep_both`; SYNC_AUTH path keeps
   token.

### Android

1. **Files**: new `sync/{GoogleAuth.kt,TokenStore.kt}`,
   `ui/sync/{SyncViewModel,SyncScreen,ConflictsScreen}.kt`; modify
   `AndroidManifest.xml` (real `appAuthRedirectScheme` placeholder),
   `app/build.gradle.kts` (manifestPlaceholders from `local.properties`),
   NavGraph route `sync`, `ui/nav/RootScaffold.kt` (foreground auto-sync
   LaunchedEffect).
2. **DTOs**: `SyncResultDto`, `ConflictDto`, `ConflictChoice` exist. Note
   `SyncResultDto.token` default `""` = not refreshed (persist only when
   non-empty) — matches iOS's nil-or-empty rule. Add token conversion helper
   in `GoogleAuth.kt`: AppAuth `TokenResponse` → oauth2-wire JSON string
   (`access_token`, `token_type`, `refresh_token`, `expiry` =
   `Instant.ofEpochMilli(accessTokenExpirationTime).atOffset(...)` RFC 3339).
3. **Repository**: `syncNow`, `conflicts`, `resolveConflict` exist. Note:
   `resolveConflict` currently sends `choice.name.lowercase()` ⇒
   `"keep_local"` — correct, but brittle vs the enum's `@SerialName`; change
   to an explicit `wire` property on `ConflictChoice` (`KEEP_LOCAL("keep_local")`…)
   and send that — removes the name-derivation magic (flagged in review
   rules).
4. **ViewModel**: `SyncViewModel(repository, tokenStore, dataVersion)` —
   state/actions mirror iOS `SyncStore` 1:1 incl. `isSyncing` guard, token
   write-back rule, SYNC_AUTH handling, deduped foreground-error snackbars.
   Sign-in launches AppAuth intent from the screen (ActivityResult API);
   ViewModel receives the `TokenResponse`.
5. **Screens**: `SyncScreen` + `ConflictsScreen` mirroring iOS §5 content and
   copy exactly.
6. **Edge cases**: same as iOS §6, plus: AppAuth cancellation → silent;
   process-death during auth → AppAuth state handled by ActivityResult
   restoration (out of scope beyond default behavior).
7. **Commits**:
   1. `feat(android): AppAuth PKCE sign-in + EncryptedSharedPreferences token store`
   2. `feat(android): sync screen (sign in/out, sync now, token write-back, status)`
   3. `feat(android): conflicts screen + resolve flow with re-sync`
   4. `feat(android): foreground auto-sync + ConflictChoice wire property`
   5. `test(android): token conversion + sync DTO + ViewModel tests`
8. **Tests**: token conversion produces RFC 3339 expiry; `SyncResultDto`
   token empty-vs-present; resolve sends exact wire strings; write-back only
   when non-empty; SYNC_AUTH keeps token.

**Platforms must agree on**: oauth2-wire token shape incl. RFC 3339 expiry;
persist-refreshed-token-immediately rule; `isSyncing` re-entrancy guard;
resolve → re-sync; SYNC_AUTH copy + keep-token behavior; conflicts screen
copy; foreground-only auto-sync cadence.

---

## Phase I — Drag-to-reorder + drag-to-nest on the daily screen

Qt reference: `tree.go resolveDropZone/onDrop/applyDrop` — target row split in
**thirds**: top third = reorder **before** target (`below=false`), bottom
third = reorder **after** (`below=true`), middle third = **nest onto**;
onto a task ⇒ `MakeSubtask(child, parent)`; onto a project header ⇒
`MoveTaskToProject(id)`; onto Unfiled header ⇒ `MoveTaskToProject("")`;
between-zones on a non-task target fall through to onto-handling; projects
are not draggable; same-day only; indicator = 2px line for between, row
highlight for onto; source `(index, text)` captured at drag start for the CAS
guard.

Core: `ReorderTask(date, srcIndex, expectedSrcText, refIndex,
expectedRefText, below)` — CAS on **both** endpoints; BAD_INPUT for
self/descendant targets (cycle guard); `MakeSubtask(date, childIndex,
expectedChildText, parentIndex, expectedParentText)`;
`MoveTaskToProject(date, index, expectedText, projectID)`.

CAS discipline: capture `(index, text)` of the dragged task **at drag
start**, and of the target row **at drop time** (row data is from the last
refresh — fine, that is exactly what the CAS guard protects); on
`casMismatch` → refresh + toast, no retry. On BAD_INPUT (cycle) → toast
"Can't drop a task into its own subtask." + refresh.

Semantic mapping (identical on both platforms):

| Drop target | Zone | Call |
|---|---|---|
| task row | above | `ReorderTask(src, ref, below=false)` |
| task row | below | `ReorderTask(src, ref, below=true)` |
| task row | onto | `MakeSubtask(src, ref)` |
| project header | any | `MoveTaskToProject(src, projectID)` |
| Unfiled header | any | `MoveTaskToProject(src, "")` |
| recurring/recycled sections, outside list | — | no-op |

Cross-project between-drop nuance: `ReorderTask` operates on the flat plan
file; reordering relative to a task in another project is legal in the store
(it moves file position, tags unchanged). Qt allows it (same-day check only).
Mobile allows it identically — no extra host-side restriction.

### iOS

Approach: SwiftUI native drag & drop (`.draggable` / `.dropDestination`,
iOS 16+) — **not** `List.onMove` (onMove cannot express onto-nesting or
cross-section drops). List rows stay in `List`; each `TaskRow` becomes a drag
source and a drop target with zone math from the drop `location` (the
`dropDestination` closure receives the location in the row's local space;
row height via a `GeometryReader`-captured height or fixed row-height
convention — capture real height, don't hardcode).

1. **Files**: new `Features/Today/TaskDrag.swift` (`struct DraggedTask:
   Codable, Transferable { date, index, text }` with a custom UTType
   `com.cristim.dailyprogress.task`; `enum DropZone { above, below, onto }` +
   `func zone(for location: CGPoint, in height: CGFloat) -> DropZone`
   (thirds, exactly Qt's `resolveDropZone`)); modify
   `Features/Today/TaskRow.swift` (`.draggable(DraggedTask(task))`,
   `.dropDestination(for: DraggedTask.self)` with `isTargeted` visual —
   top/bottom 2pt overlay line or `.background` highlight for onto),
   `Features/Today/TodayView.swift` (project section headers + Unfiled
   header become onto-only drop targets), `Features/Today/TodayStore.swift`
   (+`reorder(src:DraggedTask, ref:TreeTask, below:Bool)`,
   `nest(src:DraggedTask, under:TreeTask)`, `moveDropped(src:DraggedTask,
   toProject:String)` — all through the existing `mutate`-style CAS
   handling, generalized to take explicit `(date,index,text)` instead of a
   `TreeTask`), `CoreKit/CoreClient.swift` + `CoreAPI` (+`makeSubtask`,
   `reorderTask` — check the gomobile-generated Swift selector names in
   `Core.xcframework` headers; expect `makeSubtask(_:childIndex:expectedChildText:parentIndex:expectedParentText:)`
   and `reorderTask(_:srcIndex:expectedSrcText:refIndex:expectedRefText:below:)`,
   both BOOL-bridged to `throws`).
2. **DTOs**: none new on the wire; `DraggedTask` is host-local.
3. **Client additions**: `makeSubtask`, `reorderTask` per above.
4. **Store**: additions in §1; same-day guard (`src.date == target.date`)
   client-side before calling (drag across a date change mid-flight);
   casMismatch/BAD_INPUT handling per the shared mapping.
5. **View behavior**: drag any task row (grip affordance not needed —
   long-press-drag is the iOS convention); while targeted: above/below → 2pt
   accent line at the corresponding edge (overlay), onto → row background
   tint (`Color.accentColor.opacity(0.2)`) — mirrors Qt's indicator design;
   headers highlight for onto only. Recurring/recycled rows: no drop
   modifiers.
6. **Edge cases**: drop onto self → zone math yields onto ⇒ `MakeSubtask(src,
   src)` would be a cycle — guard client-side (ignore drops where
   `src.index == target.index && src.date == target.date`); descendant drops
   → rely on core BAD_INPUT + toast; drop on a `done`-rolled parent is legal;
   stale rows after background sync → CAS refresh path; children move with
   the parent (store subtree semantics — no host handling needed); List
   section boundaries: dropping in empty space between sections hits no
   target ⇒ no-op (match Qt's nil-target behavior).
7. **Commits**:
   1. `feat(ios): add makeSubtask + reorderTask to CoreClient/CoreAPI`
   2. `feat(ios): three-zone drag & drop on Today (reorder/nest/re-home) with indicators`
   3. `test(ios): drop-zone math + drag CAS handling tests`
8. **Tests**: `zone(for:in:)` thirds math incl. boundaries (exactly Qt: `relY <
   third` above, `relY >= height-third` below, else onto); self-drop guard;
   mock-core tests asserting the exact call per (target, zone) mapping table;
   casMismatch → refresh + toast assertion.

### Android

Approach evaluated: reorderable libraries (`sh.calvin.reorderable` etc.) only
support between-item reordering — they cannot express the onto-zone or
header targets. **Decision: hand-rolled drag in a small, self-contained
helper** (`ui/day/DragDropState.kt`), using
`Modifier.pointerInput(detectDragGesturesAfterLongPress)` on the LazyColumn
container + `LazyListState.layoutInfo` to hit-test rows and compute thirds —
the Compose analog of Qt's cursor-mapped `dragPoint()`.

1. **Files**: new `ui/day/DragDropState.kt` (`class DragDropState(listState,
   onDrop: (src: DragItem, target: DropTarget) -> Unit)`; `data class
   DragItem(date, index, text)`; `sealed interface DropTarget { data class
   Task(item, zone: DropZone); data class Project(id); object Unfiled }`;
   `enum class DropZone { ABOVE, BELOW, ONTO }`; zone math from item offset +
   size thirds; exposes `draggingKey`, `targetIndicator` state for
   rendering); modify `ui/day/DayScreen.kt` (row/header composables gain
   stable **drop metadata** — the flattened LazyColumn items list becomes a
   typed `List<DayListItem>` (`TaskItem`/`ProjectHeader`/`UnfiledHeader`/…)
   built once per Content state so hit-testing maps list positions to
   targets; draw indicator: 2dp `HorizontalDivider`-style line above/below
   target row, or row background tint for onto; dragged row elevated with
   translationY), `ui/day/DayViewModel.kt` (+`reorder(src, refIndex,
   refText, date, below)`, `nest(src, parentIndex, parentText, date)`,
   `moveDroppedToProject(src, projectId)` via `mutate {}`),
   `core/CoreRepository.kt` (**add** `reorderTask`):

   ```kotlin
   suspend fun reorderTask(
       date: String,
       srcIndex: Long, expectedSrcText: String,
       refIndex: Long, expectedRefText: String,
       below: Boolean,
   ) = call { core ->
       core.reorderTask(date, srcIndex, expectedSrcText, refIndex, expectedRefText, below)
   }
   ```
2. **DTOs**: none new on the wire.
3. **Repository additions**: `reorderTask` (above); `makeSubtask` and
   `moveTaskToProject` already exist.
4. **ViewModel**: additions in §1, all through `mutate {}` (CAS snackbar
   path already correct); BAD_INPUT cycle → snackbar "Can't drop a task into
   its own subtask."; self-drop guard before calling.
5. **Screen behavior**: long-press starts drag (matches existing long-press
   affordance conflict — long-press currently opens TaskActionsSheet;
   resolve: drag starts on long-press **move** (press-and-drag), plain
   long-press without movement still opens the sheet — implement via
   `detectDragGesturesAfterLongPress` canceling the sheet trigger once drag
   distance > touch slop); auto-scroll when dragging near list edges
   (LazyListState.scrollBy in a loop — keep simple: fixed-speed edge
   scroll); indicators per iOS §5.
6. **Edge cases**: same list as iOS §6; additionally: flattened-list ↔ tree
   consistency (the typed items list is derived from the same flatten pass
   already used for rendering, so indices/texts always match what is shown);
   drag state reset on refresh (dataVersion/uiState change cancels an
   in-flight drag).
7. **Commits**:
   1. `feat(android): add reorderTask repository method`
   2. `feat(android): typed day-list items + DragDropState (three-zone hit testing)`
   3. `feat(android): drag-to-reorder/nest/re-home on DayScreen with indicators`
   4. `test(android): drop-zone math + drop-mapping + CAS handling tests`
8. **Tests**: zone math (thirds, boundaries — same fixtures as iOS);
   DropTarget mapping table test (pure function: (src, target) → repository
   call, via fake repository); self-drop guard; CAS mismatch snackbar path;
   long-press-vs-drag disambiguation is manual-verification (note in PR).

**Platforms must agree on**: the zone-thirds math and boundary conditions;
the (target, zone) → core-call mapping table; capture-at-drag-start for src
and capture-at-drop for target; self-drop client guard + core cycle-error
toast copy; no drop targets on recurring/recycled sections.

---

## Dependency & ordering notes

Phases land in the listed order A → I; hard dependencies:

- **A first**: it creates the Android navigation shell, `dataVersion`,
  `CheckinCoordinator`, `Time.kt`, iOS `CoreDecoding`/`ToastView`/coordinator —
  every later phase uses these.
- **B depends on A** (prompt routing hosts the review/summary/plan loops;
  shared check-in button bar).
- **C–F are independent of each other** (all depend on A's shell); they can
  be parallelized across implementers if desired, C→D→E→F otherwise.
- **G depends on A** (notification tap → coordinator) and should precede H
  (client-ID mirror + core-reopen scheme is built in G; H consumes it).
- **H depends on G** (client ID entry + reopen).
- **I is last**: it touches the most-reviewed screen and adds the final two
  client methods; it benefits from every prior stabilization.
- Phase-A Android commit 1 (nav scaffold) is the single biggest merge-conflict
  surface — land it before parallelizing anything.

Per-phase Definition of Done (all phases): both platforms build
(`xcodebuild` simulator target; `gradlew assembleDebug test`); new unit
tests green; screen exercised in simulator/emulator against a data dir with
fixture data (§4 verification rules from the repo standards — "tests pass"
alone is not done); known-issues.md entries fixed in D/F/H marked done.

## Contract gaps / decisions needed before coding

1. **`SetConfig` cannot clear string fields** (empty = leave unchanged) —
   `google_client_id` can never be unset from a device. Proposed: accept for
   v1 (UI offers no clear affordance); if clearing is ever needed, add a
   core-side sentinel or explicit-null handling in a minor contract rev.
   **Needs sign-off.**
2. **`ApplyEvening` JSON-parse error lacks the `BAD_INPUT` prefix**
   (`checkin.go:108` — plain `parsing evening decisions: %w`, unlike
   ApplyMorning). Hosts always send machine-built JSON so it classifies as
   `other/Unknown` harmlessly, but it is a core inconsistency worth a
   one-line fix in the next core touch. **Flagging, not blocking.**
3. **`RecycleJSON` carries no `project` field**, so mobile recycle rows can't
   show the origin project Qt shows (hover metadata). Accepted cosmetic gap
   for v1 (alternative: source the screen from `TreeJSON.recycled`, which
   has project — rejected because RecycleJSON is the dedicated endpoint and
   date+text suffice for actions). **Needs sign-off.**
4. **Client ID at `Open` vs `SetConfig`**: the core builds its OAuth config
   from the `Open`-time clientID, so a Settings change requires the host to
   re-open the core handle (scheme specified in phase G). Works, but a
   `Core.SetClientID` or reading the config file at engine-build time would
   remove host complexity in a future core rev. **Flagging.**
5. **Android OAuth redirect scheme is build-time** (AppAuth manifest
   placeholder can't be set from a runtime-entered client ID), while iOS can
   pass the scheme at runtime. Plan assumes Android's redirect scheme comes
   from `local.properties` at build time while the core still gets the
   runtime/config value. If a fully runtime-configurable Android client ID is
   required, the alternative is an `https` App Links redirect (needs a
   hosted domain) — out of scope. **Needs sign-off.**
6. **gomobile Swift selector names for `MakeSubtask`/`ReorderTask`** must be
   confirmed against the generated `Core.xcframework` headers before phase I
   (prepositional renames bit us before — see CoreClient comments). One-line
   check at implementation start.
7. **Week-review / weekly-plan prompts get no local notification** in v1
   (no fixed trigger time in config; Qt fires them opportunistically from its
   60s timer, which mobile cannot do in background). They surface on app
   open via the coordinator. **Needs sign-off** (alternative: notify at the
   morning time on Mondays).

## Risk list

- **R1 (High) — Phase I gesture conflicts (Android)**: long-press already
  opens the actions sheet; press-drag disambiguation needs real-device
  tuning. Mitigation: isolated `DragDropState` with pure-function zone
  mapping (unit-tested); manual device pass required before merge.
- **R2 (High) — OAuth client types + redirect config**: wrong client type
  (iOS vs Android vs Desktop) or scheme mismatch fails only at runtime with
  opaque Google errors. Mitigation: document exact console setup in
  `docs/`, test on device early (phase H commit 1 is auth alone).
- **R3 (Medium) — Token wire-shape drift**: hosts hand-build oauth2.Token
  JSON; a wrong `expiry` format silently degrades to refresh-every-call or
  auth loops. Mitigation: conversion helpers are pure + unit-tested on both
  platforms; SyncNow token write-back tested with mocks.
- **R4 (Medium) — Prompt-timing regressions**: any DuePrompts caller using
  UTC reintroduces the offset bug. Mitigation: single helper per platform
  (`Date.rfc3339` / `Time.nowRfc3339Local`) + regression tests asserting a
  non-`Z` suffix; grep-check in review for `Instant.now`/`ISO_INSTANT`.
- **R5 (Medium) — TreeJSON-derived evening list**: evening rows come from the
  tree (grouped) while ApplyEvening matches by text against the flat plan;
  duplicate texts across projects collapse to one decision (store dedup
  semantics). Same behavior as Qt only when texts are unique; Qt iterates
  the raw plan. Accepted: decisions are text-keyed in the frozen contract
  regardless of source; note in code.
- **R6 (Low) — Core re-open races (phase G)**: swapping the handle while a
  call is in flight. Mitigation: iOS actor recreation is atomic behind
  `AppState`; Android `reset()` runs on the single-threaded dispatcher.
- **R7 (Low) — Notification permission flows**: iOS denied-authorization and
  Android 13+ runtime permission must both leave the toggle off + explain.
  Covered in phase G edge cases; verify manually on device.
- **R8 (Low) — DTO drift between platforms**: mitigated by the per-phase
  "platforms must agree on" checklists and mirrored test fixtures (same JSON
  strings on both platforms; reviewers diff the fixtures).
