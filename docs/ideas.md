# Ideas — potential improvements

Forward-looking product ideas, distinct from `known-issues.md` (defects) and
`docs/ui-ux-review.md` (review findings). Nothing here is committed work;
effort is a rough t-shirt size.

## Career-evidence value (the app's core purpose)

- **Monthly / quarterly rollups** (M): aggregate weekly summaries into
  `monthly/2026-07.md` and quarterly files — the natural inputs to a review
  packet.
- **Promotion-doc export** (M): compile a selected date range into one
  polished markdown/PDF (via pandoc) with done items grouped by theme.
- **Tags on items** (M): parse `#project` and `@person` tokens; weekly and
  rollup files gain per-tag sections ("everything on #migration",
  "collaborations with @ana" — the article's peer-feedback query).
- **Article parity fields** (S): optional per-day Impact / Recognition
  sections and an energy (1-5) frontmatter field; energy trends over time.
- **Aging-item insights** (S): flag items postponed N+ weeks ("this has
  slipped 5 times — still relevant?"); completion/postpone-rate stats in the
  weekly summary.
- **LLM assist** (M): "draft a narrative summary of this quarter" — feed the
  markdown to an LLM (CLI or API) and save the draft alongside the rollup.

## Capture & flow

- **Global hotkey quick-add** (M): add a task from any app without focusing
  the window.
- **Git/GitHub activity import** (M): suggest evening Done items from the
  day's commits/merged PRs (`gh` is already a dependency of the release
  flow).
- **Calendar awareness** (M/L): show today's meetings alongside the morning
  plan; count meeting-heavy days in summaries.
- **Recurring plan templates** (S): items auto-added each day/week (standup,
  inbox zero).
- **Workweek awareness** (S): configurable workdays; no prompts on weekends
  or holidays.
- **Vacation mode** (S): pause prompts for a date range from the tray menu.
- **Notification-first prompts** (M): a macOS notification ("Evening
  check-in ready") that opens the dialog on click, instead of a dialog
  stealing focus.

## UI

- **In-app backlog viewer/editor** (M): shipped — the Backlog dialog (File
  menu, tray menu, "Backlog…" button) shows both sections with adopt and
  move actions; edit-in-place and delete remain out of scope (use File →
  Open Data Folder).
- **History browser** (M): calendar picker rendering past days/weeks
  read-only (currently by-design wontfix; revisit if Finder round-trips
  annoy).
- **Drag-to-reorder plan items; priorities** (M).
- **Undo toast for destructive-ish actions** (S): drop at review,
  move-to-backlog.
- **First-run onboarding tour** (S): one dialog explaining the daily rhythm
  and where files live.
- **Menu-bar-only mode** (S): config flag mapping to LSUIElement for people
  who never want a Dock icon.

## Data & sync

- **External-edit conflict detection** (M): watch data files (FSEvents);
  if a file changed since a dialog opened, re-read before applying instead
  of relying on by-text matching alone.
- **Derived search index** (M): optional SQLite index built from the
  markdown for fast cross-year search/stats — markdown stays the source of
  truth (see the earlier markdown-vs-SQLite decision).
- **Obsidian-friendly frontmatter** (S): optional aliases/links so vaults
  can adopt the folder as-is.

## Engineering & distribution

- **Code signing + notarization** (M): required before sharing DMGs beyond
  this machine without Gatekeeper friction.
- **Universal binary** (M): arm64+amd64 via CI matrix and `lipo`.
- **True auto-update** (L): download-and-swap update flow (Sparkle-style)
  instead of open-release-page; needs signing first.
- **UI test harness** (L): drive the dialogs with synthesized Qt events in
  CI to close the "interactive click-through" gap in known-issues.md.
- **Public repo + published cask** (S): flips the distribution story on;
  everything is already wired for it.
