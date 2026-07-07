# Daily Progress Logger

A macOS desktop app (Go + Qt) implementing the ["promotion doc that writes
itself"](https://dev.to/ohkpond/the-promotion-doc-that-writes-itself-2g1i)
system: it prompts every morning for what you plan to work on, every evening
for what you actually did, and keeps everything as plain, human-editable
markdown files you can grep, sync, or feed to an LLM at review time.

## How it works

- **Morning check-in** (default 09:30): asks *"What are you planning to work
  on today?"* — one task per line — and offers to carry over still-open items
  from earlier in the week and from the backlog. Saves the plan as a checkbox
  list in the day's file.
- **Evening check-in** (default 17:30): shows the day's plan with
  *Done / Not done / Postpone to next week* buttons per item, plus anything
  else you accomplished. Regenerates the weekly summary afterwards.
- Every check-in offers **Postpone 1h** (snooze: it re-appears an hour
  later) and **Skip Today** (it stays quiet until tomorrow).
- **Week review** (first launch in a new ISO week): lists the previous week's
  leftover items and asks whether each is still relevant — *Keep this week*,
  *Postpone to next week*, or *Drop*. Postponed items automatically surface
  the following week.
- The app stays resident in the menu bar; the main window shows today's plan
  with add / check off / postpone / move-to-backlog actions.

## Data layout

Everything lives under `~/DailyProgress` (configurable):

```
daily/2026/07/2026-07-07.md   one file per day: ## Plan checklist + ## Done
weekly/2026/2026-W28.md       derived weekly summary of everything done
backlog.md                    cross-week todo list (Current + Next week)
```

Plan items use checkbox markers: `- [ ]` open, `- [x]` done, `- [>]`
postponed. The files are yours to edit — the app re-reads them before every
operation and refuses to overwrite anything it cannot parse.

Weekly summaries are regenerated from the daily files, so never hand-edit
those sections; the `reviewed` flag and `## Dropped at review` list are
preserved across regenerations.

## Configuration

`~/Library/Application Support/DailyProgressLogger/config.json` (created on
first run):

```json
{
  "data_dir": "~/DailyProgress",
  "morning_time": "09:30",
  "evening_time": "17:30"
}
```

## Running it

Two supported setups:

- **Resident app** (`make install-agent`): starts at login, lives in the
  menu bar, checks every minute and pops each check-in once its time has
  passed — including the first app open after 09:30 / 17:30.
- **Scheduled check-ins** (`make install-checkin-agent`): no resident app;
  launchd runs `daily-progress-logger -prompt-due` at 09:30 and 17:30, which
  shows whatever is due and exits (it stays alive only while a "Postpone 1h"
  snooze is pending). Adjust `packaging/checkin-agent.plist.template` if you
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

## CLI flags

```
-checkin morning|evening|review   show the named check-in, then exit
-prompt-due                       show any check-ins currently due, then exit
-hidden                           start without showing the main window
-screenshot <dir>                 render the UI offscreen to PNGs and exit
```
