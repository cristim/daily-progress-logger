# Known Issues

Statuses: **open**, **planned** (fix scheduled), **done** (fixed, kept for
reference until the next release), **wontfix** (deliberate).

- **[planned] Backlog dedup is exact-text only.** Rewording an item ("fix
  flaky test" vs "fix the flaky test") creates a duplicate. Planned fix:
  normalize case and whitespace when comparing; no fuzzy matching.
- **[planned] Week review only covers the most recent week with data.** If
  the review prompt is ignored for an entire week, the week before it is
  never triaged. Planned fix: walk all unreviewed weeks, oldest first.
- **[planned] Evening check-in decisions are index-based.** If the daily
  file is hand-edited while the evening dialog is open, applying fails with
  a mismatch error and the answers are lost. Planned fix: match decisions
  by item text instead of position.
- **[open] Interactive click-through not yet verified end-to-end.** The
  store logic is unit-tested and all screens pixel-verified via offscreen
  rendering (`-screenshot`), but the dialogs have not been driven by real
  clicks in a live session. Needs a human at the screen.
- **[wontfix] No in-app browser for past days/weeks.** By design: the
  markdown files are the interface; use "File → Open Data Folder".
