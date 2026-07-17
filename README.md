# Daily Progress Logger

A macOS desktop app (Go + Qt) implementing the ["promotion doc that writes
itself"](https://dev.to/ohkpond/the-promotion-doc-that-writes-itself-2g1i)
system: it prompts every morning for what you plan to work on, every evening
for what you actually did, and keeps everything as plain, human-editable
markdown files you can grep, sync, or feed to an LLM at review time.

## Install

```sh
brew install --cask cristim/tap/daily-progress-logger
```

The app is ad-hoc signed (not notarized); the cask strips the quarantine
attribute so Gatekeeper allows first launch. To build from source instead,
see [Building](#building).

## How it works

- **Weekly plan** (Monday morning): asks *"What are the big things you want to
  get done this week?"* and captures them as trackable goals. If you don't open
  the app Monday it catches up on the first open any later weekday. Each goal is
  done / not-done: re-open *Weekly Plan…* (File menu, tray menu, or ⌘6) any time
  to tick goals off or add more, and the Friday summary shows which were
  achieved. This week's goals also appear read-only atop the morning check-in.
- **Morning check-in** (default 09:30): asks *"What are you planning to work
  on today?"* — one task per line — and offers to carry over still-open items
  from earlier in the week and from the backlog. Saves the plan as a checkbox
  list in the day's file.
- **Evening check-in** (default 17:30): shows the day's plan with per-item
  buttons — *Done*, *Not done*, *Postpone to next day*, *Postpone to next week*,
  and *Move to backlog* — plus anything else you accomplished. Regenerates the
  weekly summary afterwards.
- When a scheduled check-in becomes due, the app posts a **macOS notification
  banner** ("Morning Check-in — What are you planning to work on today?")
  instead of immediately popping a dialog. Click the notification to open the
  dialog; the tray menu items and keyboard shortcuts always open the dialog
  directly. To switch back to the old focus-stealing behavior, uncheck
  **Check-in delivery** in Preferences.
- Every check-in offers **Remind me in 1h** (snooze: it re-appears an hour
  later) and **Skip Today** (it stays quiet until tomorrow).
- **Week review** (first launch in a new ISO week): lists the previous week's
  leftover items and asks whether each is still relevant — *Keep this week*,
  *Postpone to next week*, or *Drop*. Postponed items automatically surface
  the following week.
- **Backlog** (File menu, tray menu, or the *Backlog…* button in the main
  window): shows the cross-week todo list divided into *This week* (active
  candidates) and *Next week* (postponed items). Each row offers *Add to
  today's plan* (adopts the item into today's plan and removes it from the
  backlog) and *Move to next/this week* (shuttles it between sections). The
  backlog file (`backlog.md`) remains plain markdown and can be edited
  directly via File → Open Data Folder for renaming or deleting items.
  Note: the dialog shows "This week" for the section stored as `## Current`
  in the file, and "Next week" for `## Next week`.
- The app stays resident in the menu bar; the main window shows today's plan.
  Each item has *Done* / *Not done* buttons plus three defer actions —
  *Postpone to next day* (`>`), *Postpone to next week* (`^`, queues it in the
  backlog), and *Add to backlog* — and there is an add-task field at the top.
  Postponing to the next day removes the item from today and re-adds it to
  tomorrow's plan; postponing to next week leaves it as `- [>]` and queues it in
  the backlog's *Next week* section.
- **Recurring tasks & reminders**: type a task with a recurrence tag in the
  add-task field to make it repeat, e.g. `Team standup @weekday @9:30`,
  `Review metrics @weekly @mon @16:00`, `Rent @monthly @1 @9:00`, or
  `Vitamins @daily`. The keyword (`@daily`, `@weekday`, `@weekly`, `@monthly`)
  can be followed by an optional day (`@mon`..`@sun` for weekly, `@1`..`@31` for
  monthly) and time (`@HH:MM`, defaulting to the morning check-in time); a project
  or story ref tag placed *before* the recurrence keywords
  (`Reconcile #payments @weekly`) is preserved. To tag a task with a project or
  story use `#slug` (e.g. `DMs #marketing`); `@` is reserved for recurrence. Recurring templates live in a collapsible **Recurring** section of
  the tree (each with its schedule and a *Delete* button). When an occurrence
  comes due the app shows a notification and adds the task to that day's plan so
  you can check it off; each occurrence fires once, and a reminder missed while
  the app was closed fires as a catch-up on the next launch.
- **Keyboard shortcuts & Preferences**: every action has a configurable keyboard
  shortcut — the per-item *Done / Not done / next day / next week / backlog* on
  the selected plan row, each check-in, and window show/hide, focus-add-task and
  quit. Open *Preferences…* (File menu, tray menu, or ⌘,) to edit the shortcuts
  along with the check-in times, weekly-summary day/time and data folder; changes
  take effect immediately. Item shortcuts act on the selected row and fire while
  the app window is focused.

## Data layout

Everything lives under `~/DailyProgress` (configurable):

```
daily/2026/07/2026-07-07.md   one file per day: ## Plan checklist + ## Done
weekly/2026/2026-W28.md       weekly plan (## Week plan) + derived summary
backlog.md                    cross-week todo list (Current + Next week)
recurring.md                  recurring task templates (one checkbox per line)
```

Recurring templates are stored verbatim in `recurring.md`, e.g.
`- [ ] Team standup @weekday @9:30`; you can add or edit them by hand. Which
occurrences have already fired is tracked in `.recurring-fired.json` (per
device, not synced), so editing `recurring.md` never re-triggers past reminders.

Plan items use checkbox markers: `- [ ]` open, `- [x]` done, `- [>]`
postponed. Weekly goals use `- [ ]` / `- [x]` in the `## Week plan` section.
The files are yours to edit — the app re-reads them before every operation and
refuses to overwrite anything it cannot parse.

Weekly summary sections are regenerated from the daily files, so never
hand-edit those; the `reviewed` / `summarized` / `planned` flags, the
`## Week plan` goals and the `## Dropped at review` list are preserved across
regenerations.

## Configuration

`~/Library/Application Support/DailyProgressLogger/config.json` (created on
first run) holds the check-in times, weekly-summary schedule, data folder and
keyboard shortcuts. Edit it directly, or use *Preferences…* (⌘,):

```json
{
  "data_dir": "~/DailyProgress",
  "morning_time": "09:30",
  "evening_time": "17:30",
  "summary_day": "Friday",
  "summary_time": "17:00",
  "notify_checkins": true,
  "shortcuts": {
    "item.next_day": "Ctrl+Shift+D",
    "item.next_week": "Ctrl+Shift+W",
    "item.backlog": "Ctrl+Shift+B"
  }
}
```

Set `notify_checkins` to `false` (or uncheck **Check-in delivery** in Preferences) to revert to the old focus-stealing dialog behavior. When the field is absent the app defaults to `true`.

Shortcuts use Qt key-sequence strings (`Ctrl` renders as ⌘ on macOS); any
omitted action falls back to its default, and each must be unique.

## Running it

Two supported setups:

- **Resident app** (`make install-agent`): starts at login, lives in the
  menu bar, checks every minute and pops each check-in once its time has
  passed — including the first app open after 09:30 / 17:30.
- **Scheduled check-ins** (`make install-checkin-agent`): no resident app;
  launchd runs `daily-progress-logger -prompt-due` at 09:30 and 17:30, which
  shows whatever is due and exits (it stays alive only while a "Remind me in
  1h" snooze is pending). Adjust `packaging/checkin-agent.plist.template` if you
  change the times in config.

## Building

Requires Go ≥ 1.26, Qt 6 (`brew install qt`), and Xcode command line tools.
The first build compiles the [miqt](https://github.com/mappu/miqt) Qt
bindings and takes several minutes; later builds hit the Go build cache.

```sh
make build          # build/daily-progress-logger
make test           # unit tests (race detector on)
make lint           # golangci-lint
make app            # self-contained build/DailyProgressLogger.app (macdeployqt)
make install-agent  # start at login via a LaunchAgent (menu bar only)
```

Qt headers require C++17+; the Makefile exports `CGO_CXXFLAGS=-std=c++20`
for every Go invocation — set it yourself when calling `go build`/`go test`
directly.

## CLI (`dpl`)

A pure-Go command-line companion that reads and writes the same data files as
the GUI.  No Qt, no CGO, no dependencies beyond the standard library.

### Install / build

```sh
make cli                          # builds build/dpl
CGO_ENABLED=0 go build ./cmd/dpl # or directly
```

The binary is fully static — place it anywhere on your `PATH`.

### Shared data directory

`dpl` uses the same `data_dir` as the GUI (read from
`~/Library/Application Support/DailyProgressLogger/config.json`).  Both
processes can run concurrently: all writes are atomic, so there are no races.
Override the path with `--data-dir /custom/path`.

### Commands

```sh
# List today's plan (1-based numbered)
dpl list
dpl list --json   # machine-readable output

# Add tasks
dpl add Buy milk
dpl add --project myproj Implement the feature
dpl add --parent 2 Write unit tests   # subtask of item 2

# Change state
dpl done 1        # mark item 1 done   [x]
dpl undone 1      # revert to to-do    [ ]

# Edit / remove
dpl edit 3 Revised task text
dpl rm 3          # moves to recycle bin (recoverable in GUI)

# Postpone
dpl postpone 2             # move to tomorrow's plan
dpl postpone 2 --week      # mark [>] and queue in next week's backlog

# Backlog
dpl backlog 1              # move item 1 to current backlog
dpl backlog list           # show Current + Next week sections

# Projects
dpl projects               # list all projects (id, name, status)
dpl project add My Project # create a project, prints its id

# Recurring templates
dpl recur list

# Use a different date
dpl --date 2025-06-09 list

# Help
dpl help
```

## GUI flags

```
-checkin morning|evening|review   show the named check-in, then exit
-prompt-due                       show any check-ins currently due, then exit
-hidden                           start without showing the main window
-screenshot <dir>                 render the UI offscreen to PNGs and exit
```

## Distribution

### Building a DMG

```sh
make dmg   # produces build/DailyProgressLogger-<version>.dmg
```

This assembles the self-contained `.app` (via `macdeployqt`) and packages it
into a compressed UDZO disk image with an `/Applications` symlink.

### Creating a GitHub release

```sh
make release   # runs make dmg, then gh release create v<version> --generate-notes
```

Requires the `gh` CLI authenticated to `cristim/daily-progress-logger`.

### Homebrew cask (cristim/tap)

The cask template lives at `packaging/daily-progress-logger.rb` in this repo.
After a `make release`, publish it to the tap with:

```sh
TAP_DIR=$(brew --repository cristim/tap)
cp packaging/daily-progress-logger.rb "$TAP_DIR/Casks/"
# Update sha256 in the copied file:
SHA=$(shasum -a 256 build/DailyProgressLogger-0.1.0.dmg | awk '{print $1}')
sed -i '' "s/sha256 :no_check/sha256 \"$SHA\"/" "$TAP_DIR/Casks/daily-progress-logger.rb"
cd "$TAP_DIR" && git add Casks/daily-progress-logger.rb && git commit -m "feat: add daily-progress-logger cask" && git push
```

After that, users can install with:

```sh
brew install --cask cristim/tap/daily-progress-logger
```

### CI / automated releases

Pushing a `v*` tag (e.g. `v1.2.0`) triggers `.github/workflows/release.yml`,
which:

1. Builds the DMG on a macOS-15 runner (`make dmg VERSION=<tag>`).
2. Creates a GitHub release with `--generate-notes` and attaches the DMG.
3. Clones `cristim/homebrew-tap`, updates the cask version and sha256, and
   pushes a `chore: daily-progress-logger <version>` commit.

Step 3 requires a `TAP_GITHUB_TOKEN` repository secret: a fine-grained
personal access token with **Contents: Read and write** on
`cristim/homebrew-tap`. If the secret is absent the step prints a notice and
exits cleanly -- the release and DMG upload still complete.

A separate `.github/workflows/ci.yml` runs on every push to `main` and on
pull requests: it builds, tests (race detector on), lints with golangci-lint,
and checks for known vulnerabilities with govulncheck.

First-run caveat: the miqt Qt bindings require a full cgo compile on a cold
runner (~15 min). Subsequent runs restore `~/Library/Caches/go-build` from
the Actions cache and finish in a few minutes.

## Google Drive sync (optional)

Sync is off by default and needs no account. To sync your `~/DailyProgress`
folder across machines via Google Drive, open **Preferences → Google Drive**
and paste your own OAuth **client ID** (`xxxxx.apps.googleusercontent.com`).
Create one in the [Google Cloud console](https://console.cloud.google.com/)
as an *OAuth client ID → Desktop app*; the app uses the loopback + PKCE flow
(no client secret) and requests only the least-privilege `drive.file` scope,
so it can touch only the files it creates. The token is stored in your macOS
Keychain. Nothing is uploaded until you sign in.

## License

MIT — see [LICENSE](LICENSE).
