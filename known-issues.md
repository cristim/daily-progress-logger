# Known Issues

- **Backlog dedup is exact-text only.** Rewording an item ("fix flaky test"
  vs "fix the flaky test") creates a duplicate; no fuzzy matching.
- **Week review only covers the most recent week with data.** If the review
  prompt is ignored for an entire week, the week before it is never triaged
  (its items remain in the daily files but leave the carry-over flow).
- **Evening check-in decisions are index-based.** If the daily file is
  hand-edited while the evening dialog is open, applying fails with an
  explicit mismatch error (by design, but the answers are lost).
- **Interactive click-through not yet verified end-to-end.** The store logic
  is unit-tested and all four screens were pixel-verified via offscreen
  rendering (`-screenshot`), but the dialogs have not been driven by real
  clicks in a live session yet.
- **No in-app browser for past days/weeks.** Use "File → Open Data Folder";
  the markdown files are the interface.
