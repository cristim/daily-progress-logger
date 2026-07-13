# UI/UX Review — 2026-07-07

Thorough pass over the app against desktop/macOS UI conventions and general
UX best practices. Each finding is implemented as its own commit; the
Status column tracks that.

## Findings

### 1. Dialogs lack a default button and initial focus
**Problem:** Pressing Return in a check-in dialog does nothing predictable;
the OK button is not marked default, and the morning dialog opens without
focus in the plan editor, so the first keystroke is lost.
**Fix:** Mark OK as the default button in every check-in dialog; focus the
text editor when the morning/evening dialog opens.
**Status:** implemented

### 2. "Cancel" hides real semantics (skip for the day)
**Problem:** Cancel actually means "don't ask me again today", but the
label doesn't say so — users may cancel expecting to be re-asked, then
wonder why the check-in never returns.
**Fix:** Label the button "Skip Today" (keeping its reject role, so Escape
still triggers it) and keep "Postpone 1h" for the snooze.
**Status:** implemented

### 3. Check-in dialogs can appear behind other windows
**Problem:** Prompts are parented to a usually-hidden main window in a
background app; when the timer fires, the dialog may open unfocused behind
the active app and go unnoticed — defeating the app's core purpose.
**Fix:** Raise and activate each check-in dialog when shown.
**Status:** implemented

### 4. Long plan lists overflow the evening/review dialogs
**Problem:** Item rows are added directly to the dialog layout; 20 plan
items produce a dialog taller than the screen with no scrolling.
**Fix:** Put the per-item rows in a scrollable area with a sane maximum
height; enable word wrap on item labels so long tasks don't force extreme
widths.
**Status:** implemented

### 5. Main window has a blank empty state
**Problem:** Before the morning check-in the plan list is simply empty —
no hint about what the app is for or what to do next.
**Fix:** Show a placeholder ("No plan for today yet — run the Morning
Check-in") when the day has no items.
**Status:** implemented

### 6. Done/postponed items look identical to open ones
**Problem:** In the main window every row's label renders the same; the
only state cue is which small button is checked.
**Fix:** Render done items struck through and dimmed, postponed items
dimmed, so state is visible at a glance.
**Status:** implemented

### 7. Un-postponing leaves a stale backlog entry
**Problem:** Postponing pushes the item into the backlog's "Next week"
list; flipping it back to Done/Not done in the same day leaves that backlog
entry behind, so the item resurfaces next week even though it was handled.
**Fix:** When a plan item leaves the postponed state, remove its text from
the backlog's Next week list (store-level change with a regression test).
**Status:** implemented

### 8. No keyboard shortcut for Quit
**Problem:** The File → Quit action has no Cmd+Q binding (the menu-bar app
ignores the standard shortcut).
**Fix:** Bind Cmd+Q to Quit in the menu.
**Status:** implemented

### 9. Tray icon ignores menu bar appearance
**Problem:** The hand-drawn blue/white circle stays identical in light and
dark menu bars instead of adapting like native template icons.
**Fix:** Draw the glyph in black and mark the icon as a template
(`QIcon::setIsMask`), letting macOS tint it for the current appearance.
**Status:** implemented

### 10. Dialog proportions
**Problem:** Check-in dialogs open at whatever size the layout computes —
the morning dialog can come up cramped.
**Fix:** Give dialogs a reasonable minimum width so text fields are
comfortable.
**Status:** implemented

### 11. Checked state of state-selector buttons indistinct in dark mode
**Problem:** On macOS dark mode the three QToolButtons (Done / Not done /
Postpone) show no clear visual difference between checked and unchecked
states, making the current selection hard to spot.
**Fix:** Apply a palette-based stylesheet to the container widget so the
checked button is highlighted using the system highlight colour.
**Status:** implemented

### 12. Main window plan list showed a horizontal scrollbar when rows overflow
**Problem:** When plan item rows were wider than the list widget (e.g. due
to long task text or many buttons) a horizontal scrollbar appeared, wasting
vertical space and looking out of place.
**Fix:** Disable the horizontal scrollbar on the plan QListWidget via
SetHorizontalScrollBarPolicy(ScrollBarAlwaysOff).
**Status:** implemented

### 13. Add-task field was below the list; row-button captions consumed horizontal space
**Problem:** The "Add a task for today..." input field was positioned below
the plan list, making it easy to overlook and inconsistent with the natural
top-to-bottom flow (add then review). Additionally, the Done / Not done /
Postpone buttons and the Backlog button displayed text labels beside their
icons, consuming significant horizontal space and causing the last button to
clip on narrow windows.
**Fix:** Moved the add-task field above the plan list so it appears
immediately after the heading. Converted all row buttons to icon-only
(ToolButtonIconOnly) and folded their captions into tooltips: "Done",
"Not done (keep as an open todo)", "Postpone to next week", "Move to the
cross-week backlog". Updated the empty-state placeholder text to reference
"above" since the add-task field now sits above the list.
**Status:** implemented

### 14. Week-review dialog used QComboBox dropdowns
**Problem:** The review dialog offered each leftover item a plain dropdown
("Keep this week / Postpone to next week / Drop"), inconsistent with the
icon-button row used everywhere else in the app for state selection.
**Fix:** Extracted `newChoiceSelector(choices []choice, initialID int)`
in statebuttons.go as a generic icon-button row; `newStateSelector` is
now a thin wrapper. The review dialog uses three icon buttons per item
(Apply = Keep, ArrowForward = Postpone, TrashIcon = Drop) defaulting to
Keep. Removed the now-unused `actionForComboIndex` helper.
**Status:** implemented

### 15. Plan-row buttons did not form an aligned column
**Problem:** Each plan-item row widget was sized to its natural content width,
so rows with short task labels ended before the list's right edge. This caused
the Done / Not done / Postpone / Backlog buttons on the right of each row to
sit at inconsistent horizontal positions, breaking the visual column alignment
that makes state scanning fast.
**Fix:** In `refresh()`, capture the list viewport width before clearing,
then call `SetSizeHint` on every item to force it to that width (taking the
natural hint when the natural width is larger). Added a `rowWidth()` helper
that caps the viewport's pre-show over-large value to the window width.
An `OnResizeEvent` handler schedules a refresh so rows re-span whenever the
window is resized. `GrabScreenshots` shows the window and processes events
before refreshing so the correct viewport geometry is used for screenshots.
**Status:** implemented

## Review round 2 — Fable (2026-07-07)

Fresh-eyes adversarial pass. Evidence screenshots live under
`/tmp/claude/dpl-review-shots/` (subdirs: `seeded`, `empty`, `stress`,
`longone`, `manyshort`), rendered offscreen against seeded homes under
`/tmp/claude/dpl-home*`. The `stress` home has 25 plan items (incl. 200+
char, emoji, Cyrillic, Hebrew, Arabic, and HTML-like texts), 15 backlog
items and 23 week-review leftovers; `longone` isolates a single long item;
`manyshort` has 25 short items; `empty` is a brand-new home with no data.

### 16. One long plan item hides every row's controls in the main window
**Severity:** high
**Problem:** Main-window row labels do not word-wrap, so a single long item
(200+ chars) has a natural width of ~1800px. The list then lays out *every*
row at that width: all Done / Not done / Postpone / Backlog buttons move far
past the right edge, and with the horizontal scrollbar disabled (finding 12)
they are completely invisible and unreachable for **all** items, not just the
long one. RTL items (Hebrew/Arabic) right-align inside their over-wide labels
and render as blank rows. The long text itself is clipped with no ellipsis.
Repro: add one 200-char task; see
`stress/main-window.png` (no buttons anywhere, two blank RTL rows) vs
`manyshort/main-window.png` (25 short items, buttons fine) and
`longone/main-window.png` (one long item among three short ones breaks all
four rows).
**Fix:** Enable word wrap on plan-row labels and stop taking
`max(targetWidth, naturalHint.Width())` in `refresh()`; size every row to the
viewport width and let the label wrap within its allotted space (recompute
the row height from the wrapped label's `heightForWidth`).
**Status:** implemented — display text hard-capped at 120 runes with "…" and
full text in the label tooltip; `refresh()` always sizes rows to `targetWidth`
(no longer `max(targetWidth, naturalHint.Width())`). RTL rows now render
correctly within the 560 px viewport. Manyshort regression passes.

### 17. Item text containing `<`, `>` or `&` is silently mangled
**Severity:** medium
**Problem:** `QLabel` auto-detects rich text. Todo rows in the main window
(`labelText = item.Text`) and all item labels in the evening and week-review
dialogs pass raw text, so a task like `compare a<b && b>c in the benchmark
harness` renders as "compare ac in the benchmark harness" (bold), and
`<b>ship the release</b> & tag v2.0` renders bold without the literal tags —
see rows 9-10 of `stress/main-window.png`. Meanwhile done/postponed rows DO
escape via `html.EscapeString`, so the same item changes rendering as its
state changes.
**Fix:** Call `SetTextFormat(Qt::PlainText)` on every item label, and apply
the done/postponed strike/dim styling via `QFont`/palette (or keep HTML but
escape uniformly, as the weekly summary already does).
**Status:** implemented — todo labels in `buildPlanRow` now call
`SetTextFormat(qt.PlainText)`; evening and week-review dialog item labels
likewise. Done/postponed labels keep HTML with `html.EscapeString` as before.

### 18. Morning check-in pre-checks the entire backlog by default
**Severity:** medium
**Problem:** Every backlog "Current" item appears in the morning carry-over
list pre-checked (`item.SetCheckState(qt.Checked)`), so pressing Return (OK
is default) adopts the whole backlog into today's plan and *removes those
entries from the backlog*. With 15 backlog items (`stress/morning.png`) the
default action floods the day and drains the backlog. It also defeats the
Backlog button: an item parked out of today's plan comes back pre-checked
the very next morning.
**Fix:** Pre-check only same-week carry-over items; backlog candidates
default unchecked (optionally add "select all / none" toggles).
**Status:** implemented — same-week carry-over candidates remain pre-checked
(planned recently, likely still relevant); `FromBacklog` candidates default
unchecked so adopting them requires an active choice. Pressing Return (OK
default) no longer drains the backlog.

### 19. Skip/snooze semantics leak into manually invoked check-ins
**Severity:** medium
**Problem:** Tray "Morning Check-in…"/"Evening Check-in…" run through
`runPrompt`, so pressing Escape ("Skip Today") on a *manually opened* dialog
records `skippedOn` and silently cancels that day's scheduled prompt (peek at
08:00, hit Esc, the 09:30 prompt never fires). Conversely, on the manual
review/summary paths (`runWeekReviewManually`, `runWeeklySummaryManually`)
the result is ignored, so "Postpone 1h" promises "Ask again in an hour" and
does nothing.
**Fix:** Don't record skip bookkeeping for user-invoked runs (or show a
plain Close/Cancel instead of Skip Today there), and drop the snooze/skip
buttons from manual review/summary dialogs.
**Status:** implemented — `runPrompt` now takes a `manual bool`. For
manual runs (tray menu, main-window buttons, `-checkin` flag): cancel
closes without setting `skippedOn`; snooze sets `snoozeUntil` AND adds
the prompt to `forced` so it re-fires after the hour. `runWeekReviewManually`
and `runWeeklySummaryManually` now capture the dialog result and apply the
same manual bookkeeping via `applyManualResult`. Scheduled behavior
(`CheckPrompts`) is unchanged (manual=false).

### 20. Viewing "This Week's Summary…" mid-week kills the Friday prompt
**Severity:** medium
**Problem:** The summary dialog looks like a passive report, but OK (the
default button, triggered by Return) calls `MarkWeekSummarized`. A user who
opens the tray summary on Tuesday and dismisses it with Return has silently
consumed the scheduled Friday-afternoon summary for that week.
**Fix:** A manual summary view should be read-only with a Close button;
only the scheduled prompt (or an explicit "Mark week summarized" control)
should set the flag.
**Status:** implemented — `buildWeeklySummaryDialog` now takes `markOnAccept
bool`. The scheduled path (`runWeeklySummaryForNow`) passes `true`; the
manual tray path (`runWeeklySummaryManually`) passes `false` so accepting
the dialog is a no-op write-wise. The dialog content and buttons are
identical in both cases.

### 21. Manual "Review Last Week…" prematurely rolls the backlog over
**Severity:** medium
**Problem:** Accepting the review dialog runs `ApplyWeekReview`, which always
calls `backlog.rollOver()` (Next week -> Current). Invoked manually mid-week,
one Return keypress promotes every item the user postponed "to next week"
into Current *today*, resurfacing them (pre-checked, per finding 18) in the
next morning's candidates. The rollover is correct for the scheduled
Monday review, not for an on-demand re-triage on Wednesday.
**Fix:** Only roll over NextWeek items when the review runs for a week
boundary that has actually passed since the last rollover (e.g. skip
`rollOver()` when the reviewed week's successor is still the current week
and a rollover already happened), or confirm before rolling over on manual
runs.
**Status:** implemented — `ApplyWeekReview` now takes `rollover bool`.
The scheduled path (`runWeekReviewLoop`) passes `true`, keeping existing
behaviour. The manual path (`runWeekReviewManually`) passes `false` so
NextWeek items are not promoted mid-week. Two new store tests assert both
paths independently.

### 22. Morning and evening dialogs fire back-to-back, in an odd order
**Severity:** medium
**Problem:** `CheckPrompts` shows every due prompt sequentially. Launching
the app (or coming back from a day away) after the evening time with the
morning check-in not done produces: "What are you planning to work on
today?" followed immediately by "How did today go?" — planning a day that is
already over, then triaging it seconds later. On a Monday this can chain
week review + morning + evening with no indication that more dialogs are
queued.
**Fix:** When both daily check-ins are due, show only the evening one (the
plan can be captured there); when several prompts are queued, indicate
progress (e.g. "1 of 2") or at least announce the next dialog.
**Status:** implemented — in `schedule.Due`, when both morning and evening
are due simultaneously the morning prompt is omitted; morning is included
only when the evening window has not yet opened. Schedule tests updated:
"missed morning stacks with evening" renamed and now expects only
`[PromptEvening]`; "full stack" and "full friday stack" tests likewise
updated; new "morning only mid-day" case asserts morning is shown when
evening has not opened yet.

### 23. Week-review Drop gives no confirmation or summary
**Severity:** low
**Problem:** The trash button sits one icon away from Postpone; OK applies
all decisions with no recap, so a mis-clicked Drop removes an item from the
backlog silently. (Mitigation: dropped texts are recorded under the weekly
file's dropped list, so the data is recoverable by hand.)
**Fix:** When any Drop is selected, show a one-line count in the dialog
("2 items will be dropped") or confirm on OK.
**Status:** open

### 24. Postpone arrow icon is nearly invisible in dark mode
**Severity:** low
**Problem:** `SP_ArrowForward` renders as a dark glyph on the dark dialog
background; unchecked it is barely visible next to the colored check/cross
icons (`seeded/evening.png`, first two rows; same in the week review and
main window). Users may not realize a third state exists.
**Fix:** Tint the arrow like the tray glyph (template/mask icon painted from
the palette), or choose a higher-contrast standard icon.
**Status:** implemented — `postponeIcon()` in statebuttons.go draws a
right-pointing chevron in mid-gray (140,140,140) on a 16x16 transparent
pixmap; used in both `newStateSelector` and the week-review choices.

### 25. "Postpone" means two different things in the same dialog
**Severity:** low
**Problem:** In the evening dialog the per-item arrow tooltip says "Postpone
to next week" while the bottom button says "Postpone 1h" (snooze the
dialog). Same verb, wholly different consequences.
**Fix:** Rename the snooze button to "Remind me in 1h" (tooltip already
says "Ask again in an hour").
**Status:** implemented — `attachButtons` label changed to "Remind me in 1h";
README updated accordingly.

### 26. Plan items cannot be edited or deleted; the backlog is invisible
**Severity:** low
**Problem:** A mistyped task can only be marked done/not-done/postponed or
moved to the backlog; there is no rename or delete anywhere in the UI, and
the backlog itself has no in-app view, so "Move to the cross-week backlog"
makes an item vanish with no feedback, no undo, and no way to check where it
went (short of opening the markdown file). The markdown-is-the-interface
design covers browsing history, but fixing a typo in *today's* plan
shouldn't require a text editor.
**Fix:** Add edit-in-place (double-click) and a Delete action to plan rows;
show a transient status ("Moved to backlog") or a small backlog count next
to the heading.
**Status:** implemented (feedback + backlog manager); after a successful
MoveToBacklog the tray (if present) shows a balloon "Moved to backlog /
<item text>" via `App.notifyBacklogMove`. The Backlog dialog (File menu,
tray menu, and "Backlog…" button in the main window) lists both "This week"
(Current) and "Next week" sections with per-row "Add to today's plan" and
"Move to next/this week" icon buttons; actions apply immediately and the
dialog refreshes in place. Edit-in-place and delete for plan rows are still
not implemented — use File → Open Data Folder to edit the markdown directly
for typo fixes and deletions.

### 27. Re-running the morning check-in hides the existing plan
**Severity:** low
**Problem:** Invoking the morning dialog when today already has a plan shows
an empty editor and no mention of the existing items (they are excluded from
the candidate list). It looks like the earlier plan was lost; re-typing an
item silently dedupes.
**Fix:** Show the current plan (read-only list or prefilled editor) when
`morning_done` is already true, or change the heading to "Add more tasks for
today".
**Status:** implemented — when today's plan is non-empty, `buildMorningDialog`
adds a PlainText QLabel "Already planned today: N items." with a tooltip
listing each item. Not shown on the first run (no plan yet).

### 28. Empty-state polish in the summary and review dialogs
**Severity:** low
**Problem:** The empty weekly summary shows both "Nothing completed yet this
week." and "0 items completed." (`empty/weekly-summary.png`); the
nothing-left-open week review still offers "Postpone 1h" and "Skip Today"
even though there is nothing to come back for (`empty/week-review.png`);
"(No plan was recorded for today.)" uses a parenthetical tone no other
string uses.
**Fix:** Drop the count line when it is zero; give the no-op review dialog a
single Close/OK button; align the evening empty-state wording with the rest
("No plan was recorded for today.").
**Status:** partially implemented — "0 items completed." count line is
suppressed when totalDone == 0; evening no-plan label changed to "No plan was
recorded for today." (parentheses removed). The no-op week-review Close/OK
simplification is deferred (review dialog still shows Remind/Skip when empty).

### 29. Icon-only controls lack accessible names and keyboard access
**Severity:** low
**Problem:** All state/backlog/review buttons are icon-only QToolButtons
with tooltips but no accessible names, so VoiceOver reads nothing useful;
on macOS tool buttons are typically excluded from the tab order, making the
main window's row actions and the dialogs' per-item selectors mouse-only
(not verified in a live session; flagged from code).
**Fix:** Call `SetAccessibleName` with the tooltip text on every icon
button, give them an explicit focus policy, and verify tabbing with Full
Keyboard Access enabled.
**Status:** implemented — `SetAccessibleName(c.tooltip)` added to every
button in `newChoiceSelector`; backlog button likewise. Focus policy and
live VoiceOver verification remain manual (flagged in the finding).

### 30. "Check for Updates…" freezes the UI with no feedback
**Severity:** low
**Problem:** The tray action runs the HTTP check synchronously on the main
thread with a 10 s timeout: on a slow network the menu click appears to do
nothing while the whole UI (including the tray) is frozen for up to 10
seconds.
**Fix:** Reuse the existing background-check path with a completion dialog
(plus a busy cursor), instead of the synchronous variant.
**Status:** implemented — replaced `checkForUpdatesSynchronous` with
`checkForUpdatesManual`: the HTTP check runs in a goroutine; all results
(up-to-date, new version, error) are marshalled back to the Qt main thread
via `mainthread.Start` and shown in a dialog. The tray menu is wired to
the async handler. Disabling the menu item during the in-flight check was
skipped (not trivially easy with the current menu API).

### Checked and found fine (round 2)
- 25 short plan items: rows and button columns align, vertical scrolling
  works, no horizontal scrollbar (`manyshort/main-window.png`).
- Evening and week-review dialogs word-wrap 200+ char items correctly and
  cap their height with a scroll area; checked-state highlight is clearly
  visible in dark mode (findings 4/11 hold up).
- Emoji and Cyrillic text render fine on every screen; the weekly summary
  HTML-escapes item text properly; "1 item / n items" pluralization correct;
  date and week formats are consistent across heading, dialogs and files.
- Empty home: config auto-creates; all five screens render sensible
  first-run states; the main-window placeholder (finding 5) reads well.
- OK is the default button and Escape consistently maps to Skip Today in all
  four check-in dialogs; dialogs raise/activate; the `dialogOpen` guard
  prevents overlapping prompts; the multi-week review loop walks oldest
  first.
- Morning candidate dedup (normalized text) works across days and backlog;
  hand-edited files while the evening dialog is open are handled by
  text-matched decisions; file writes are atomic.
- Tray menu covers all core actions plus Show Window; Cmd+Q works; closing
  the window hides to the menu bar as intended.
- Borderline, not filed: the 320px scroll-area cap makes 20+ item triage
  cramped on large screens but stays usable; morning candidate list allows
  horizontal scrolling for long texts (inconsistent with finding 12 but
  functional).

## Review round 3 — Fable (2026-07-09)

Fresh-eyes adversarial pass over everything shipped through round 2, with
extra weight on the new Backlog dialog and the manual/scheduled prompt
split. Evidence screenshots live under `/tmp/claude/dpl-r3-shots/`
(subdirs: `backlog30`, `nextonly`), rendered offscreen against homes under
`/tmp/claude/dpl-r3-home-*`. The `backlog30` home has 30 backlog items
(incl. a 200+ char item, `<`/`&` texts, emoji, Cyrillic, Hebrew, Arabic)
plus a 3-item plan where one item is postponed and also present in the
backlog; `nextonly` has an empty Current section and 3 Next-week items.
Store-level claims were verified with throwaway tests against
`internal/store` (run and then deleted, not committed).

### 31. One long backlog item hides every row's buttons in the Backlog dialog
**Severity:** high
**Problem:** `buildRow` labels do not word-wrap, and the 120-rune elision cap
still allows ~800 px of text in a 460 px dialog. The widget-resizable scroll
area then scrolls horizontally: the "Add to today's plan" / "Move to next
week" buttons of **every** row sit past the right edge and are invisible
until the user h-scrolls, and RTL items (Hebrew, Arabic) right-align inside
their over-wide labels and render as blank rows. This is the same failure
class as finding 16, reintroduced in the newest dialog. Repro: one backlog
item longer than ~70 chars; see `backlog30/backlog.png` (h-scrollbar, no
buttons anywhere, two blank RTL rows) vs `nextonly/backlog.png` (short
items, buttons fine).
**Fix:** Reuse the finding-16 recipe: elide to the available pixel width
(`QFontMetrics::elidedText`) or size rows to the scroll-area viewport width
instead of a fixed rune count; keep the full text in the tooltip; disable
the horizontal scrollbar as the main window does.
**Status:** implemented

### 32. "Add to today's plan" on an already-planned item silently voids its state
**Severity:** medium
**Problem:** The normal flow puts a postponed plan item into the backlog's
Next week list, so the same item is visible in the main window (dimmed,
postponed) and in the Backlog dialog at once (the `backlog30` home models
this with "update the deployment runbook"). Clicking "Add to today's plan"
on it calls `AdoptFromBacklog`: `AddPlanItem` dedups against the existing
plan entry (no-op) and both backlog entries are removed. Verified at the
store level: the plan item stays in the postponed state, nothing visibly
joins today's plan, and the item lost its next-week resurfacing, so the
user's postpone is silently undone halfway. Adopting an item that matches a
*done* plan item likewise just drains the backlog entry with no visible
effect. There is also no feedback at all when the main window is hidden
(tray-opened dialog), unlike the move-to-backlog balloon.
**Fix:** When the adopted text already exists in today's plan, reset that
item's state to todo (that is what "add to today's plan" means) and refresh;
optionally show an "Added to today's plan" tray balloon mirroring
`notifyBacklogMove`.
**Status:** implemented

### 33. Main window goes stale at midnight; row actions hit today's file by index
**Severity:** medium
**Problem:** `refresh()` only runs on user actions and dialog closes, so a
window left open overnight keeps yesterday's heading and plan until the
next interaction. Worse, the row handlers call
`SetPlanItemState(time.Now(), index)` / `PostponePlanItem` /
`MoveToBacklog` with the index captured at render time against *today's*
file: after midnight the new day's plan is empty, so clicking any state
button on the stale list produces "plan item index 0 out of range
(0 items)" in an error dialog. The same index addressing means a hand-edit
of today's file (item deleted in an editor) makes a click on a stale row
silently toggle the *wrong* item; the store already fixed this class for
the evening dialog (text-matched `EveningDecision`, see known-issues.md)
but not for the main window rows.
**Fix:** Remember the date the list was rendered for and refresh when it
changes (e.g. from the existing 60 s `CheckPrompts` tick); address row
actions by item text like `ApplyEvening` does, or at least verify the text
at `index` still matches before applying.
**Status:** implemented

### 34. "Skip Today" is inaccurate on manual dialogs, and its meaning flips after a manual snooze
**Severity:** low
**Problem:** Residue of finding 19. On a tray-invoked check-in the reject
button still says "Skip Today" (tooltip "Don't ask again today") but now
deliberately does nothing beyond closing: the scheduled prompt fires later
anyway, contradicting the label. Conversely, a manual "Remind me in 1h"
arms a `forced` re-fire that arrives via `CheckPrompts` with
`manual=false`, so on the visually identical re-fired dialog "Skip Today"
*does* record `skippedOn` and silences that day's real scheduled prompt.
Two same-looking dialogs, opposite Skip semantics, with no way to tell
which one is on screen.
**Fix:** Pass `manual` into `attachButtons` and label the reject button
"Close" (no tooltip) on manual runs; carry the manual origin through the
`forced` map (e.g. `forced[prompt] = manual`) so a snoozed manual prompt
re-fires with manual semantics.
**Status:** implemented

### 35. "Remind me in 1h" late in the evening silently drops the check-in at midnight
**Severity:** low
**Problem:** Snoozing at 23:30 sets `snoozeUntil` 00:30. At 00:30 the
evening prompt is no longer due (the new day's evening time has not been
reached), so the promised reminder never comes; the previous day's evening
check-in is silently lost, and a re-fire would target the new date anyway
because the dialog is built for `time.Now()`. `skippedOn` handles the day
boundary correctly; only snooze leaks across it.
**Fix:** Cap snooze deadlines at the end of the current day, or when a
snooze crosses midnight re-target the re-fired evening dialog at the day
the snooze was created for.
**Status:** implemented

### 36. Update dialog, login-item offer and error boxes bypass the dialogOpen guard
**Severity:** low
**Problem:** `showUpdateDialog`, the message boxes in
`checkForUpdatesManual`/`checkForUpdatesBackground`, `reportError` and
`MaybeOfferLoginItem` all run modal exec loops without setting
`dialogOpen`. The 60 s timer keeps firing during them, so a scheduled
check-in can stack on top of the update dialog; the 200 ms startup
`CheckPrompts` can stack on the login-item question; the first background
update check (t+2 min) can put its dialog on top of an open check-in; and
`HandleReopen`'s `!dialogOpen` exclusion does not apply, so an activation
while one of these dialogs is up pops the hidden main window beneath it.
**Fix:** Route every modal surface through a shared guard, e.g. a small
helper that sets/clears `dialogOpen` around any exec'd dialog or message
box.
**Status:** implemented

### 37. Backlog nomenclature differs between the dialog and the file; store errors surface raw
**Severity:** low
**Problem:** The dialog's sections are "This week" / "Next week" while
backlog.md, the advertised hand-editable interface, uses "## Current" /
"## Next week"; nothing connects the two names. When the file is
hand-edited while the dialog is open and the user clicks Move on a removed
row, the raw store error appears in a critical dialog: `item "…" not found
in the Current backlog section`, leaking the file-side name into UI copy.
The main-window tooltip "Move to the cross-week backlog" also never says
the item lands in "This week".
**Fix:** Pick one name pair (rename the file section to "This week" or the
dialog headers to "Current" / "Next week"); on a missing-item action,
refresh the rows and show a friendly one-liner ("This item is no longer in
the backlog.") instead of the store error.
**Status:** implemented

### 38. The adopt button reuses the checkmark icon that means Done/Keep everywhere else
**Severity:** low
**Problem:** `SP_DialogApplyButton` (green check) means "Done" in the main
window and evening dialog and "Keep this week" in the week review; in the
Backlog dialog the same glyph means "Add to today's plan". A first-time
user scanning icons may click the check on a backlog row to mark it
done/acknowledged and instead schedule it for today
(`nextonly/backlog.png`). The main window uses an up-arrow for
plan-to-backlog, but the reverse operation does not use the mirrored
metaphor.
**Fix:** Draw a distinct adopt glyph (a "+" or a down-arrow-into-list, in
the style of `postponeIcon`) so the icon language stays consistent:
check = done/keep, arrows = move.
**Status:** implemented

### 39. "Keep this week" at review demotes items to unchecked morning candidates
**Severity:** low
**Problem:** Interaction between the finding-14 and finding-18 fixes.
Review "Keep this week" does `addCurrent`, so a leftover plan item the user
just explicitly affirmed becomes a backlog item; the very next morning it
shows up with the "(backlog)" suffix and defaults *unchecked* under
finding 18's rule ("parked away from today's plan"), which is exactly what
the user did not do. The affirmative triage decision must be re-confirmed
by hand every morning after.
**Fix:** Either pre-check backlog candidates on mornings of the week in
which they were kept at review (needs a small kept-at-review marker in the
weekly meta or backlog), or rename the review choice to "Keep on backlog"
so expectations match behavior.
**Status:** implemented

### 40. Manual-review Drop can resurrect via the Next week section
**Severity:** low
**Problem:** `ApplyWeekReview` with `rollover=false` (manual "Review Last
Week…") handles Drop with `removeCurrent` only. An item that is both a
review candidate (open on one day of the week) and present in NextWeek
(postponed on another day, or hand-placed) survives the drop: verified at
the store level, after the next scheduled rollover the dropped item
reappears in Current and hence in morning candidates.
**Fix:** Drop (and Postpone-to-Drop symmetry aside) should also call
`removeNextWeek(dec.Text)`; harmless on the scheduled path where the
rollover has already emptied NextWeek.
**Status:** implemented

### 41. A Friday away silently kills that week's summary prompt
**Severity:** low
**Problem:** `PromptWeeklySummary` is only due while `now.Weekday() ==
summaryDay`, and `WeekSummaryPending` only inspects the *current* week. If
the app is not running during Friday 17:00-24:00 (holiday, laptop shut),
the prompt is gone for good: on Saturday the weekday no longer matches, and
from Monday the pending check looks at the new week. Contrast with week
reviews, which catch up via the `UnreviewedWeek` oldest-first loop
(known-issues.md fixed exactly this class for reviews).
**Fix:** Make the pending check look back like `UnreviewedWeek` does (most
recent week with data and `summarized: false`) and let the prompt fire on
any later day, or fold a missed summary into the Monday review flow.
**Status:** implemented

### Checked and found fine (round 3)
- Backlog dialog with 30 short items: sections render in order, vertical
  scrolling works, per-row buttons align (`nextonly/backlog.png`); an empty
  Current section correctly shows only "Next week"; Esc and Close both
  dismiss; "Nothing in the backlog." empty state reads fine.
- `<`/`&` texts, emoji and Cyrillic render literally (PlainText holds up) in
  the Backlog dialog rows, morning candidates and week review; no candidate
  duplication with 20 backlog items (`backlog30/morning.png`).
- Hand-editing backlog.md while the dialog is open cannot corrupt data:
  every button action is a fresh read-modify-write; adopting a
  since-deleted item no-ops gracefully; a move on a deleted item errors
  loudly then the deferred refresh resyncs the rows (wording covered by 37).
- Duplicate-path sweep: `ApplyMorning`, `AddPlanItem`, `addCurrent`,
  `addNextWeek` all dedup by normalized text, so adopting from the dialog
  and then accepting a previously built morning dialog cannot double-add;
  an adopted item flows correctly into the evening triage and, once done,
  into the weekly summary; an item present in both backlog sections
  collapses to one on adopt or move.
- Scheduled review path: `rollOver()` runs before decisions, so Drop
  correctly removes items living in NextWeek there (only the manual path
  has finding 40); morning-drop-when-evening-due behaves sensibly on a
  fresh late-day launch ("No plan was recorded for today.").
- Schedule edges: `TimeOfDay.reached` is wall-clock so DST transitions do
  not shift the 09:30/17:30 prompts; the 60 s timer catches a just-passed
  check-in time within a minute; `skippedOn` day-scoping resets naturally
  at midnight; week IDs and Monday `Start()` are DST-safe (midnight local).
- The evening dialog built before midnight applies to the day it was opened
  for (`today` captured at build time), so answers filed at 00:05 land in
  the correct file.
- Manual async update check: responsive UI, result marshalled to the main
  thread, errors shown; only the guard gap (finding 36) remains. Not
  re-filed: no in-flight indicator / double-click double-dialog (already
  noted as skipped in finding 30).
- Not verifiable offscreen, flagged for the live-session pass already
  tracked in known-issues.md: whether opening the tray menu itself
  triggers `ApplicationActive` and pops the hidden main window via
  `HandleReopen` before a tray action's `dialogOpen` guard engages.

## Post-launch fixes (2026-07-13)

### 42. Duplication in weekly Done summary (two causes)
**Severity:** medium
**Problem:** The week 2026-W28 review showed heavy duplication with two
code-addressable root causes:
(a) Users hand-copy plan lines into `## Done` with the checkbox still
attached (`- [x] Shift Invoice`). `parseDoneBullet` kept the marker,
so the stored text `"[x] Shift Invoice"` did not match the checked
plan item `"Shift Invoice"` on normalized comparison.
(b) An item carried over and checked in two daily files (e.g. Thursday
and Friday) appeared under both days in the weekly aggregation.
**Fix:**
(a) `parseDoneBullet` strips leading checkbox markers (`[ ] `, `[x] `,
`[X] `, `[>] `) before storing the text. Render is unaffected (Done
section always emits plain `- text` bullets), so a parse+save
normalizes the file.
(b) `DoneByDay` (internal/store/weekly.go) now maintains a `globalSeen`
map across all days. First occurrence wins; later days' copies of the
same normalized text are silently dropped. The weekly summary dialog
(internal/ui/dialogs.go) already calls `DoneByDay`, so no separate
loop was needed.
**Status:** implemented (commits ed7266a, 1dd5fa5)

### 43. Evening dialog free-text section misleads users into duplication
**Severity:** low
**Problem:** The label "Anything else you accomplished?" does not
signal that checked plan items are already recorded, so users re-enter
them in different words ("Visited Mom" vs checked "Visit Mom"). No
dedup can catch phrasing differences.
**Fix:** Label changed to "Anything else you accomplished? (Items
checked above are recorded automatically.)" to steer users toward the
free-text box's intended purpose (truly unplanned accomplishments).
**Status:** implemented (commit 0e14be3)

### 44. Week-review dialog buttons icon-only and always visible
**Severity:** low
**Problem:** Keep/Postpone/Drop buttons in the week-review dialog used
icons only, requiring a tooltip hover to learn the action. All buttons
were always visible, adding visual noise across many items.
**Fix:** Text-caption buttons ("Keep", "Next week", "Drop") with
tooltips retained. Buttons are hidden at rest and revealed on hover
(OnEnterEvent/OnLeaveEvent on each row widget, with a
QCursor::pos() bounds check to avoid flicker when the cursor moves onto
a child button). Keyboard users see buttons via OnFocusInEvent. An
always-visible dimmed state label at the row's right edge shows the
current choice at rest. Hover fallback was NOT needed: the bounds check
eliminates the enter/leave flicker that child widgets typically cause.
**Status:** implemented (commit d6d6e0b)

### 45. Project-tagged tasks invisible in main-window tree
**Severity:** high
**Problem:** Tasks tagged with a project ID (e.g. `@marketing`) in the daily
plan were silently dropped from the tree when the project had no stories.
`dayTasks` correctly bucketed them by their project-ID slug, but
`openProjectTree` only looked up story IDs in the `dayByStory` map; project-ID
buckets were never consumed and rendered nowhere -- not under any project, not
in Unfiled. On real data (8 story-less projects, 12 project-tagged tasks),
the tree showed zero tasks for the day.
**Fix:**
- `TreeProject` gains a `Tasks []TreeTask` field for tasks tagged with the
  project ID directly (no story level).
- `openProjectTree` attaches `dayByStory[p.ID]` to `tp.Tasks` and folds
  project-level tasks into the project's global done-state calculation
  (mirroring how story tasks feed `projectSeen`/`projectOpen`).
- `BuildProjectTree` computes a `consumed` set of open project/story IDs;
  any `dayByStory` bucket whose slug is not consumed (closed project/story)
  falls back to Unfiled rather than disappearing.
- `internal/ui/tree.go`: `addProjectNode` renders project-level tasks above
  stories, reusing existing `addTaskNode`/`taskRow` machinery; `dropTask`
  extended to handle project-node drop targets via `AssignTaskProject`.
**Status:** implemented

## Other notes

- **[wontfix] Dock icon visibility:** hiding the Dock icon (LSUIElement) is
  common for menu-bar apps, but the main window is a first-class part of
  this app; keeping the Dock icon aids discoverability. Revisit if noisy.
- **[wontfix] In-app history browser:** "Open Data Folder" remains the way
  to browse past days; the markdown files are the interface by design.
