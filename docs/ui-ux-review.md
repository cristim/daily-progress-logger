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
**Status:** partially implemented (feedback); in-app backlog view deferred —
after a successful MoveToBacklog the tray (if present) shows a balloon
"Moved to backlog / <item text>" via `App.notifyBacklogMove`. Edit-in-place,
delete, and an in-app backlog view are not yet implemented.

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
**Status:** open

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

## Other notes

- **[wontfix] Dock icon visibility:** hiding the Dock icon (LSUIElement) is
  common for menu-bar apps, but the main window is a first-class part of
  this app; keeping the Dock icon aids discoverability. Revisit if noisy.
- **[wontfix] In-app history browser:** "Open Data Folder" remains the way
  to browse past days; the markdown files are the interface by design.
