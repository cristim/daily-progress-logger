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

## CLI flags

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

Note: the repo must be public (or the release asset publicly accessible) before
`brew install` can fetch the DMG.

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

Note: the cask remains uninstallable via `brew install` while the repository
(or its release assets) is private.
