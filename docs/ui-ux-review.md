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

## Other notes

- **[wontfix] Dock icon visibility:** hiding the Dock icon (LSUIElement) is
  common for menu-bar apps, but the main window is a first-class part of
  this app; keeping the Dock icon aids discoverability. Revisit if noisy.
- **[wontfix] In-app history browser:** "Open Data Folder" remains the way
  to browse past days; the markdown files are the interface by design.
