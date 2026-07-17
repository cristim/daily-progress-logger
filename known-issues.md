# Known Issues

Statuses: **open**, **planned** (fix scheduled), **done** (fixed, kept for
reference until the next release), **wontfix** (deliberate).

- **[done] Backlog dedup is exact-text only.** Rewording an item ("fix
  flaky test" vs "fix the flaky test") creates a duplicate. Fix: added
  `normalizeText` (lower-case + collapsed whitespace) used for all
  comparisons; original text is preserved in storage.
- **[done] Week review only covers the most recent week with data.** If
  the review prompt is ignored for an entire week, the week before it is
  never triaged. Fix: UnreviewedWeek now returns the oldest unreviewed
  week; the UI loops until all are reviewed or the user snoozes/skips.
- **[done] Evening check-in decisions are index-based.** If the daily
  file is hand-edited while the evening dialog is open, applying fails with
  a mismatch error and the answers are lost. Fix: ApplyEvening now accepts
  []EveningDecision{Text, State}; items are matched by normalized text;
  decisions for non-existent items are silently ignored.
- **[open] Interactive click-through not yet verified end-to-end.** The
  store logic is unit-tested and all screens pixel-verified via offscreen
  rendering (`-screenshot`), but the dialogs have not been driven by real
  clicks in a live session. Needs a human at the screen.
- **[wontfix] No in-app browser for past days/weeks.** By design: the
  markdown files are the interface; use "File → Open Data Folder".

## Mobile apps (iOS / Android) - review follow-ups

Non-blocking residual divergences flagged during the mobile-app-foundation
review (both apps merged and building). Address before the corresponding
screens ship.

- **[open] Android `RecurringTemplateDto` silently defaults absent schedule
  fields.** `describe=""`, `kind=0`, etc. mean the 3-field management shape
  and a real `kind=0` are indistinguishable (violates the no-magic-default
  rule). iOS now models these as optionals. Fix Android before the Recurring
  screen is built.
- **[open] iOS `AppState.refreshDuePrompts` decodes with a bare JSONDecoder
  and swallows errors.** Decode-failure classification lives only in
  `TodayStore.decode`; Android classifies at the repository level for every
  endpoint. Hoist iOS decode+classify into `CoreClient`/a shared helper as
  more stores land.
- **[open] Android must send local-offset RFC3339 when it wires DuePrompts.**
  iOS was fixed to emit `+HH:MM` (the core compares the embedded offset for
  local hour/minute). Android has no DuePrompts caller yet; when added it must
  use `OffsetDateTime.now()` + `ISO_OFFSET_DATE_TIME`, or the prompt-timing
  bug reappears on Android.
- **[open] iOS `RecycleEntry.id = "date#text"` can collide** if the same text
  is deleted twice on one day (SwiftUI `Identifiable` nit). Include a stable
  discriminator (e.g. array index) when the Recycle screen is built.
