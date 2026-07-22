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

- **[done] Android `RecurringTemplateDto` silently defaults absent schedule
  fields.** `describe=""`, `kind=0`, etc. meant the 3-field management shape
  and a real `kind=0` were indistinguishable (violates the no-magic-default
  rule). Fixed in Phase D: schedule fields are now nullable (Kotlin `?`),
  matching iOS's optionals.
- **[done] iOS `AppState.refreshDuePrompts` decoded with a bare JSONDecoder
  and swallowed errors.** Fixed in Phase A: `CoreDecoding` is the single
  decode/encode surface and classifies failures; `refreshDuePrompts` logs on
  failure.
- **[done] Android must send local-offset RFC3339 when it wires DuePrompts.**
  Fixed in Phase A: `util/Time.nowRfc3339Local()` emits `+HH:MM` (pattern
  `yyyy-MM-dd'T'HH:mm:ssxxx`, so UTC is `+00:00` not `Z`).
- **[open] iOS `RecycleEntry.id = "date#text"` can collide** if the same text
  is deleted twice on one day (SwiftUI `Identifiable` nit). Include a stable
  discriminator (e.g. array index) when the Recycle screen is built.

### Phase B (weekly) follow-up nits - non-blocking

- **[open] iOS double `dataVersion` bump per completed prompt flow.** Each
  weekly mutation bumps via `onMutation` and `onComplete` bumps again; harmless
  (idempotent refresh), drop the `onComplete` bumps later.
- **[open] iOS WeeklyPlanSheet OK after a failed load saves `[]` sight-unseen.**
  Only reachable in a stale-prompt race (the plan prompt fires only when no plan
  exists). Optional: disable OK while `store.plan == nil`.
- **[open] Stale KDoc** on Android `WeekReviewScreen` composable ("dismisses
  immediately" - it now applies the empty review first).
- **[open] Android `submitting` flag not `rememberSaveable`** - rotation
  mid-submit re-enables buttons; ops are idempotent, cosmetic.
