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

## Noted, deliberately not changed

- **Dock icon visibility:** hiding the Dock icon (LSUIElement) is common
  for menu-bar apps, but the main window is a first-class part of this app;
  keeping the Dock icon aids discoverability. Revisit if it feels noisy.
- **Week review combos:** the review dialog keeps dropdowns; its three
  choices (keep/postpone/drop) are less frequent decisions and vertical
  space matters more there. Can switch to buttons later for consistency.
- **In-app history browser:** "Open Data Folder" remains the way to browse
  past days; the markdown files are the interface by design.
