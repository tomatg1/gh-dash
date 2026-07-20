# gh-dash fork — local notes

Private notes for a personal `gh-dash` fork, checked out at `~/code/gh-dash`.
These notes describe local build tooling and conventions; paths assume a
`~/code/gh-dash` checkout — adjust to yours.

Upstream: `dlvhdr/gh-dash` (forked at v4.25.0, `49f37e4`).

---

## Why the fork exists

Two gaps in upstream, both of which turned out to be small once the internals were read:

1. **Mouse capture was unconditional.** `internal/tui/ui.go` hardcoded
   `MouseModeCellMotion`. Capture is what makes UI clickable — but it also
   intercepts click-drag, which is what your terminal uses for native text
   selection. There was no way to opt out, so "highlight to copy" was impossible.
2. **Nothing was actually clickable.** `bubblezone` was already a dependency and
   `zone.Scan()` already wrapped the composed view, but the only marked zone in
   the entire codebase was the **donate button**. Tabs and rows did nothing.
   (Upstream issue [#722](https://github.com/dlvhdr/gh-dash/issues/722), still open.)
3. **v4.25.0 refuses to start outside a git repo.** Found the hard way while
   restarting: `gh dash` from `~` exits 1. See below — it's a real upstream bug.

## What the fork adds

### `defaults.mouseMode` (commit `d4a6ff9`)

| value | effect |
| --- | --- |
| `cellMotion` | clicks, wheel, drag — **default**, matches upstream |
| `allMotion` | also motion with no button held |
| `none` | no capture → **native highlight-to-copy works again** |

The default is `cellMotion`, so behavior is unchanged unless you opt out.
Note `MouseModeNone` is `iota 0` in bubbletea, so a careless `bool` field would
have silently defaulted the feature *off*; the config default is set explicitly
in `getDefaultConfig()` and pinned by a test.

### Clickable tabs + rows + wheel (commit `6815868`)

Marked bubblezone zones on the tab titles and table rows, and handled the wheel.
Click semantics were superseded by `5d6569b` below.

### Drag to select (commit `5d6569b`, threshold `a3c069a`)

Drag highlights text and copies it on release. The highlight is dropped on the
next press, on any keystroke, and on wheel scroll — all three move what sits
under it. A drag that wanders back onto the cell it started on is still a drag,
so it can never accidentally open a browser.

**Drag threshold (`a3c069a`).** A press only becomes a drag once the pointer
moves past a threshold (2 columns / 1 row); within that it stays a click.
Without this, a trackpad tap's inevitable one-cell jitter registered as a drag —
copying a stray character and leaving a lingering partial highlight stuck on the
row instead of selecting it. Vertical counts double because rows are two cells tall.

**Prior-row clearing (`f6080e4`).** `BuildRows()` bakes the selected styling into
each row, and the section rebuilds its rows only at the tail of its `Update` (on
any message). The keyboard path reaches that; the early-returning mouse-click
path skipped it — so a click moved the cursor but left the old row highlighted
(and the new row "split") until an async fetch rebuilt "a few seconds later." The
click path now forwards through the same rebuild, so the old row clears at once.

### Opening takes intent (commit `2ac3d2f`)

Single-click-to-open made every click a browser launch, which is the wrong
default for a list you navigate with the mouse.

| gesture | result |
| --- | --- |
| click the **`#1234`** | open the PR |
| click the **CI icon** | open `<pr>/checks` |
| click the **review icon** | open `<pr>/files` |
| click the **comments count** | open the conversation |
| **double-click** a row | open the PR |
| click **anywhere else** in a row | select it, nothing more |
| click a **tab** | switch section |
| **drag** | highlight text; copies on release |
| **wheel** | move the row selection |

The `#1234` is not a table cell — it lives inside `renderExtendedTitle`'s top
line — so only that substring is marked, not the whole line. Compact rows have
no extended title and therefore no number zone; they still open on double-click.

Bubbletea reports presses and releases but **no click count**, so the
double-click window (400ms) is measured in `registerRowClick()`. A completed
pair is consumed, so a third click starts a fresh one rather than re-opening.
Clicking an icon does not arm a pairing.

### Clickable chrome (commit `6762327`)

Everything else that's logical to click now activates:

| click | result |
| --- | --- |
| footer **PRs / Issues / Notifications** | switch straight to that view (not a cycle) |
| footer **`? help`** | toggle the full help pane |
| preview **Overview / Activity / Commits / Checks / Files** | switch the detail tab |

Two implementation notes:

- Footer buttons switch *directly*. `switchSelectedView`'s tail was factored
  into `applyViewChange()`, shared by the keyboard cycle and a new
  `setSelectedView(target)` the click uses. Clicking the active view is a no-op.
- The preview tabs share the same carousel component as the section tabs, so
  they mark the same way — but the body dispatch was `switch SelectedItem()`
  against the raw labels, and embedding zone markers in the items would have
  broken that string compare. It's now keyed on `carousel.Cursor()`, and
  `SelectedTab()` returns the clean label by index.

**`? help` toggle (fix `54b35c6`).** Two bugs made the help click glitchy:
mouse branches `return` early and skipped the `syncProgramContext()` at the end
of `Update` that the keyboard path falls through to, so `ShowAll` flipped but the
section viewports never resized — the frame grew, panes weren't restored on
close, and the growth pushed the `? help` bar out from under the pointer. Fixed
by running both syncs in the click branch. Separately, help now renders **above**
the status bar (it was below), pinning the bar — and the `? help` toggle and view
switcher on it — to the bottom line, so you can click the same spot to open and
close. The view-switcher click had the same early-return gap and got the same fix.

### Draggable preview divider, remembered (commit `9265f8f`)

Press the divider under the list to resize instead of starting a selection; drag
moves it; release persists it. Both panes are floored at one row — a zero-height
pane would be unrecoverable, since its own divider would be unreachable.

Persisted to `$XDG_STATE_HOME/gh-dash/layout.json` (else `~/.local/state/…`),
written temp-file-then-rename so a crash mid-write can't leave a half-parsed
layout. A corrupt file reads as "nothing remembered".

**On "each instance":** see the [Instances](#instances) section — persisted
state (layout + selection) is scoped per instance, defaulting implicitly to the
launch directory's config.

A height saved on a taller terminal is clamped on load, so a big preview can
never swallow the list. Only a **bottom-docked** preview has an up/down divider;
a right-docked one would need a left/right drag, which isn't implemented.

### Don't die outside a git repo (commit `a63ef7f`)

An upstream v4.25.0 regression. `cmd/root.go` did this:

```go
gitRepo, ghRepo, err := getCurrentGitAndGitHubRepos()
if err != nil {
    log.Error("error while determining git and github repos", "err", err)  // handled
}
...                                                                        // nil checks warn + carry on
if err != nil {
    log.Fatal("Cannot parse debug flag", err)   // ← fatals on that same handled error
}
```

Launching from `~` (or anywhere not a repo) died at startup:

```
FATA Cannot parse debug flag failed to run git: fatal: not a git repository ...  ="missing value"
```

The stray `="missing value"` is `charmbracelet/log` receiving `err` as a *key*
rather than a key/value pair — a second symptom of the same stray line. Dropping
the re-check restores the pre-#931 behavior (empty repo path, global config).

This one is worth upstreaming on its own; branch `fix/no-git-repo-fatal`.

### Sub-minute auto-refresh (commit `667f72b`)

Upstream's `defaults.refetchIntervalMinutes` is whole-minute granularity, so
anything under a minute was impossible. Added `defaults.refetchIntervalSeconds`,
which takes precedence when set:

```yaml
defaults:
  refetchIntervalSeconds: 30   # fork-only; sub-minute cadence
  # refetchIntervalMinutes: 5  # still works; seconds wins when both are set
```

`0` on both disables auto-refresh. The refresh tick is driven off the effective
seconds value. **This config's current setting is `refetchIntervalSeconds: 30`.**

### Open links in a chosen browser profile (commit `9adb343`)

By default gh-dash opens PR/issue URLs in the OS default browser — which may be
the wrong browser profile, signed in to the wrong GitHub account.
`defaults.urlOpenCommand` overrides that: it's rendered as a text/template with
`{{.URL}}` and run via `sh -c`, so links go to a specific browser/profile.

```yaml
defaults:
  # macOS: open in the Chrome profile already signed in to the right account.
  # Find the profile directory in ~/Library/Application Support/Google/Chrome
  # (Default, Profile 1, Profile 3, …) — NOT the display name.
  urlOpenCommand: '"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" --profile-directory="Profile 3" "{{.URL}}"'
```

Empty (the default) = unchanged OS-default-browser behavior. All PR-open paths
(`o`, click, double-click, the `#number`, the CI/review/comments icons) route
through it via `internal/urlopen`.

**Speed matters here.** Launching the Chrome *binary* with `--profile-directory`
to hand off a URL takes **~4s**; `open`/AppleScript talk to the running Chrome in
**~0.2s** but can't target a *profile*. So `tooling/open-url.sh` caches the target
profile's **window id** and adds a tab to it via AppleScript (fast), using the
slow binary launch only to bootstrap/repair the cache. Point `urlOpenCommand` at
it:

```yaml
urlOpenCommand: '~/code/gh-dash/tooling/open-url.sh "Profile 3" "{{.URL}}"'
```

The window-id cache lives in `$XDG_STATE_HOME/gh-dash/chrome-window-<profile>`.
First open after a Chrome-window closes is slow (re-bootstrap); the rest are
instant. It brings the target window to the front and selects the tab.

**No duplicate tabs.** If the window already has the URL open, that tab is
selected instead of a second one being appended. A tab matches when its URL,
minus any `#fragment`, is the target *or a sub-page of it* — so a PR you're
reading on `/files`, or scrolled to an `#issuecomment` anchor, is reused rather
than duplicated (you land on that tab as-is, not a fresh Conversation view). The
`/` and `?` bounds keep `.../pull/627` from matching `.../pull/6270`.

**Focusing the right window** (two AppleScript traps): reference the window by
`window id <n>`, never a `repeat with w in windows` loop variable — the loop
variable is positional, and `activate` reorders the windows, so a later
`set index` would raise whatever window slid into that old slot (the one you were
just on). And set the index **after** `activate`, not before — before
`make new tab` it won't stick.

**Only raise when needed.** If the target is already Chrome's front (most-
recently-active) window, skip `activate`/`set index` entirely — `activate` is
app-level (it pulls *every* Chrome window, all profiles, above other apps), so
raising a window that's already active needlessly surfaces the other-profile
windows. Verified: with the target already front, opening a link leaves the whole
window z-order unchanged. Caveat: `make new tab` (and navigating a tab) brings
Chrome-the-app forward on their own — that's Chrome's behavior, not ours — but no
*other* window ever comes above the target, and the reuse path (URL already open,
just `set active tab index`) doesn't activate Chrome at all.

**Startup pre-warm.** When `urlOpenCommand` is set, gh-dash opens the first
configured repo's PR list on launch — in a background goroutine, so it never
blocks startup — which warms the window-id cache and leaves a useful tab open, so
the first real open is already instant. The URL is derived from the PR sections
(first `repo:`, else first `org:`, else your PR inbox). Disable with
`defaults.disableBrowserPrewarm: true`.

The pre-warm sets `GH_DASH_PREWARM=1` on the command (`urlopen.Prewarm`), and
`open-url.sh` **skips entirely when its cached window is still open** — so a fresh
tab is only opened when the cache is actually cold (first launch, or after you've
closed that Chrome window). Frequent restarts don't pile up tabs.

> Chrome maps a display name (e.g. "Work") to a directory (`Profile N`) in
> `Local State` / each profile's `Preferences`. gh-dash needs the **directory**.

### Mouse wheel direction (commit `5af211b`)

The wheel-to-selection mapping felt reversed under macOS **natural scrolling**.
Default is now tuned for it: a two-finger-**down** gesture moves the selection
**down** the list. Set `defaults.mouseWheelReverse: true` for a classic mouse.

> macOS *natural scrolling* (System Settings → Trackpad → "Natural scrolling",
> on by default) makes content follow your fingers, like a touchscreen — which
> flips what direction the terminal reports for a wheel event. What you want for
> a list (push down → go down) is the mouse-wheel feel; the default now delivers
> that, and the toggle covers the other setup.

### Persisted selection (commit `825d501`)

The selected item's URL is remembered per section per instance in the state
file, and restored on the next launch (the section re-selects that URL once its
first fetch lands). Saved on every selection change.

A refresh rebuilds each section from scratch (fresh table, cursor 0). Each
`FetchAllSections` (PRs, issues, notifications) carries the old rows over **and
places the cursor immediately** — not only when the fetch lands — otherwise the
list flashes to the top row (or blanks, for issues, which didn't carry rows at
all) for the duration of the fetch, and a keypress in that window acts on the
wrong item. The pending-selection URL stays armed to re-pin the cursor after the
fresh (possibly reordered) data arrives.

### `defaults.mergeQueueRepos` — `m` drives the native merge queue

On a repo with a merge queue but **"Allow auto-merge" disabled**, upstream's `m`
(which runs `gh pr merge`) fails: GitHub routes queue-add through the
`enablePullRequestAutoMerge` mutation, which that setting rejects
(`Auto merge is not allowed for this repository`).

List such a repo under `defaults.mergeQueueRepos` (`"owner/name"`, or `"*"` for
all) and `m` instead toggles the **native** merge queue — the same path as the
web "Merge when ready" button, needing no extra permission:

```yaml
defaults:
  mergeQueueRepos:
    - AutonomousTechnologies/autonomous
```

- PR not queued → `m` runs `enqueuePullRequest(input:{pullRequestId})`.
- PR already queued → `m` runs `dequeuePullRequest(input:{id})` (note the
  asymmetric input field — enqueue takes `pullRequestId`, dequeue takes `id`).
- The queued state shows as the merge-queue icon in the row (`IsInMergeQueue`,
  flipped optimistically and confirmed on the next fetch).

List **only** repos that actually have a merge queue — enqueue errors on a repo
without one. Scope is the PRs-view `m` (list + its sidebar); the notifications
PR-preview still does a plain merge. Failures now surface gh's real error in the
footer (fork also made `fireTask` capture stderr).

**Showing queued state in the list.** The list is fetched via GitHub's `search`
API, which does **not** populate `isInMergeQueue` (an expensive computed field —
it comes back `false`, same as `mergeStateStatus` comes back `UNKNOWN`). So the
merge-queue icon never triggered from the search alone. The list is now
**enriched** after each fetch from the authoritative `repository.mergeQueue.
entries`, scoped to `mergeQueueRepos` — one extra query per distinct (repo, base
branch) present, so an `org:` tab doesn't fan out. `data/mergequeue.go`.

### `defaults.showMergedFor` — keep recently-merged PRs visible

A PR filtered by `is:open` vanishes the instant it merges (e.g. through the
queue). Set `defaults.showMergedFor` (a Go duration like `"1h"`; empty/`"0"` =
off) and each PR section runs a **second** search — the section's filter with
`is:open`→`is:merged` plus `merged:>=<now-window>` — and appends the results
(they render with the merged icon). Global default, overridable per section:

```yaml
defaults:
  showMergedFor: "1h"
prSections:
  - title: All (org)
    showMergedFor: "30m"
```

Fetched only on the first page (not while paginating). `MergedSinceQuery` in
`data/mergequeue.go`; wired in the PR section's fetch.

## Instances

An **instance** scopes persisted state (layout height + selection). Identity:

```
--instance <file>   use that config file
--instance <dir>    use <dir>/.gh-dash/config.yml (created if missing)
--instance .        use $PWD/.gh-dash/config.yml (created if missing)
(none) + local      use $PWD/.gh-dash/config.yml if it already exists
(none)              global config; state keyed implicitly by the launch directory
```

So **two dashboards launched from different directories are automatically
distinct instances** — different layouts, different remembered selections —
without naming them. `--instance .` bootstraps a hidden `./.gh-dash/config.yml`
(seeded template) so a directory can have its own filters/tabs too. The instance
name is synthetic: the project directory name plus a hash of the config path
(e.g. `work-592fa798`). `GH_DASH_INSTANCE=<name>` still works as an explicit
override.

State lives in `$XDG_STATE_HOME/gh-dash/layout.json` (else `~/.local/state/…`),
keyed by instance; `ghd` launches from `~`, so all ghd windows share the `~`
instance unless launched elsewhere or given `--instance`.

## How selection and clicking coexist

The moment a program enables mouse reporting, the terminal hands drag events to
the program and **stops drawing its own selection**. That's why highlight-to-copy
appeared impossible alongside click-to-open.

It isn't. The program receives *press*, *motion* and *release* — enough to tell a
click from a drag, and enough to draw the selection itself. gh-dash now does:

- **press** arms a maybe-drag; nothing happens yet
- **motion while held** → it's a drag; extend and reverse-video the span
- **release with no motion** → it was a click; open the PR
- **release after motion** → copy the selected text

The selection is drawn by the app, so it only covers the **visible frame** — it
can't reach into the terminal's scrollback. For that (or to select across the
whole window at once) you still have the terminal's own selection:

1. Hold **⌥ Option** while dragging → bypasses mouse capture entirely.
2. Press **`y`** / **`Y`** → copy the PR number / URL.
3. Set `mouseMode: none` and restart → the terminal owns the mouse again, and
   clicking stops.

## Implementation notes (the parts that are easy to get wrong)

- **Row zones are marked OUTSIDE the row style.** That style applies
  `MaxWidth()`, which truncates. A zone marker sliced in half is never scanned
  back out.
- **Tab titles are marked BEFORE the carousel**, which runs `lipgloss.Width()`
  and `ansi.Truncate()` over them. This is only safe because bubblezone's
  markers are private CSI sequences (`\x1B[<n>z`) that both treat as zero-width.
  That's a property of a third-party library, so
  `TestMouseZones_TabSurvivesCarouselTruncation` pins it.
- **`common.MarkZone()` tolerates a nil global manager.** `zone.Mark` panics if
  `zone.NewGlobal()` was never called. Marking happens during render, and render
  happens in tests and headless snapshots where the program never ran. It
  degrades to the unmarked string — an unclickable zone is correct when there is
  no mouse.
- **`zone.Scan()` is asynchronous.** It hands zones to a worker goroutine over a
  channel, so `zone.Get()` is not populated the instant `Scan()` returns. Tests
  must poll. In the running TUI this is invisible (frames redraw constantly).
- **`listviewport.SetCurrItem()` walks via `Next/PrevItem`** rather than
  recomputing scroll bounds, keeping `topBoundId`/`bottomBoundId`/viewport offset
  consistent. The walk is bounded by the visible rows, so it's cheap.
- **`highlightFrame()` strips the selected span before reversing it.** An SGR
  reset nested inside the span would otherwise cancel the reverse attribute
  partway through. Stripping removes only zero-width escapes, so cell widths —
  and the layout — are unchanged. `TestHighlightFrame_PreservesTextAndWidth`
  pins that, because a layout that shifts mid-drag is unusable.
- **`frameBuf` is a `*string`.** `View()` has a value receiver and can't write to
  the model, but a drag-release needs the exact frame that was highlighted.
- **`copyToClipboard` is a package var**, so tests don't scribble on the real
  clipboard.

### Known limitations

- A row only **half-scrolled** into view can have one of its two bubblezone
  markers clipped, so its zone may not register until the row is fully visible.
  Clicking it is a no-op; scroll one line and it works.
- The drag-selection is **drawn by the app**, so it covers only the visible
  frame — it cannot reach into the terminal's scrollback. Use ⌥-drag for that.
- A drag **copies on release**, with no separate ⌘C step. That's deliberate
  (there's no way to intercept ⌘C), but it does mean a drag overwrites the
  clipboard.

---

## Known-good version labels

Local annotated tags, never pushed. The logo shows the current one (via
`git describe`, baked in at build time). To roll back to one:

```bash
git -C ~/code/gh-dash checkout v4.25.0-local.1
~/code/gh-dash/tooling/build.sh
```

| tag | contents |
| --- | --- |
| `v4.25.0-local.1` | mouseMode config, clickable tabs/rows, wheel, drag-select + click-to-open, non-git-repo startup fix |
| `v4.25.0-local.2` | + double-click/number-to-open, clickable icons + footer + preview tabs, draggable & persisted divider, help-toggle fix, sloppy-click drag threshold, prior-row-clear fix, injectable version string |
| `v4.25.0-local.3`–`.8` | urlOpenCommand + fast Chrome-profile open (window-id cache), sub-minute refetch, per-instance state, persisted selection across restarts, mouse-wheel direction, startup browser pre-warm (cold-cache-only) |
| `v4.25.0-local.10` | footer stays visible when the divider is dragged high + help expanded (reserve the list's minimum height) |
| `v4.25.0-local.11` | `defaults.mergeQueueRepos`: `m` toggles the native merge queue (enqueue/dequeue) on listed repos; `fireTask` surfaces gh's real stderr |
| `v4.25.0-local.12` | `open-url.sh` focuses the window that opened the link (id reference + set-index-after-activate), not the previously-focused one |
| `v4.25.0-local.13` | `open-url.sh` reuses a tab already on the URL (incl. sub-pages/anchors) instead of opening a duplicate |
| `v4.25.0-local.14` | `open-url.sh` skips the raise when the target is already Chrome's front window — no more surfacing other-profile windows |
| `v4.25.0-local.15` | refresh no longer flashes the PR-list cursor to the top before restoring the selection (place the cursor in `FetchAllSections`) |
| `v4.25.0-local.16` | same refresh smoothing for the issues + notifications lists (issues also no longer blanks mid-refresh) |
| `v4.25.0-local.17` | show merge-queue state in the list (enrich from `mergeQueue.entries`, since search omits it) + `defaults.showMergedFor` to keep recently-merged PRs |

Remember: the checkout **is** the installed extension, so rebuild after any
`git checkout` or `gh dash` serves a stale binary.

### The version string in the logo

The logo shows `git describe --tags` (e.g. `v4.25.0-local.2`), injected at build
time via `-ldflags -X …/internal/tui.Version=…`. **This is why you build through
`tooling/build.sh`** — a bare `go build` has no version to inject and the logo
falls back to "dev". The "Update available!" nag is suppressed for local builds
(a fork is always ahead of the upstream tag, so the nag would be permanent and
misleading).

## `tooling/` — where the setup lives

All the reproducible setup for this fork lives in `tooling/` (next to this file):

| file | what it does |
| --- | --- |
| `setup.sh` | run everything, idempotent: build → install extension → font/profile/launcher → link `ghd-repo` |
| `build.sh` | build the binary with the git-describe version baked into the logo |
| `setup-terminal.sh` | macOS: Nerd Font + `gh-dash` Terminal profile + `ghd` launcher |
| `ghd.zsh` | the `ghd` launcher function (sourced from `~/.zshrc`) |
| `ghd-repo.py` | manage the per-repo tabs in `config.yml` (symlinked to `~/.bin/ghd-repo`) |
| `open-url.sh` | fast-open a URL in a specific Chrome profile (window-id cache); used by `urlOpenCommand` |
| `FORK_NOTES.md` | this file |

> Why `tooling/` and not `docs/`? Upstream's `docs/` is the Starlight source for
> the gh-dash.dev **website** (astro, pnpm, its own Dockerfile) — not a general
> docs folder. Dropping fork notes there would pollute a directory the maintainer
> ships from, so everything custom is namespaced under `tooling/` instead.

### Reproduce from scratch

```bash
git clone <your-fork> ~/code/gh-dash && cd ~/code/gh-dash
git remote add upstream https://github.com/dlvhdr/gh-dash.git
./tooling/setup.sh          # build, install, font/profile/launcher, ghd-repo
```

## Build / install

The fork directory **is** the installed extension (gh symlinks to it):

```
~/.local/share/gh/extensions/gh-dash -> ~/code/gh-dash
```

```bash
# build (bakes the git-describe version into the logo; derives repo from its path)
~/code/gh-dash/tooling/build.sh

# install — `.` is MANDATORY; gh rejects absolute paths for local extensions,
# and the binary must already exist and be named after the repo.
cd ~/code/gh-dash && gh extension install .
```

**The footgun:** because the checkout *is* the extension, `git checkout` to
another branch without rebuilding leaves `gh dash` running a stale binary.
Rebuild after every branch switch or upstream pull.

### Sync with upstream

```bash
git -C ~/code/gh-dash fetch upstream
git -C ~/code/gh-dash rebase upstream/main
~/code/gh-dash/tooling/build.sh
```

No reinstall needed — it's a symlink.

### Downgrade protection

Local extensions are immune to `gh extension upgrade`:

```
$ gh extension upgrade --all   →  [dash]: local extensions can not be upgraded
```

So a routine `gh extension upgrade --all` cannot silently revert you to upstream.
gh-dash was never brew-installed, so brew has nothing to reconcile.

### Revert to upstream

```bash
gh extension remove dash && gh extension install dlvhdr/gh-dash
```

---

## Surrounding setup (macOS / Terminal.app)

### Fonts + profile + launcher — `tooling/setup-terminal.sh`

gh-dash draws its state icons with **Nerd Font** glyphs. Terminal.app has no
glyph fallback — it can only draw glyphs present in the one selected font — so
the `?` tofu boxes mean "wrong font", never "broken config".

`tooling/setup-terminal.sh` does all three, idempotently:

1. `brew install --cask font-meslo-lg-nerd-font`
2. creates a `gh-dash` Terminal profile (via AppleScript — `make new settings
   set` + set font) using **`MesloLGSDZNFM-Regular`**
3. wires the `ghd` launcher into `~/.zshrc`

Terminal stores fonts by **PostScript** name, not the family name in the font
panel. `MesloLGSDZNFM-Regular` = *MesloLGSDZ Nerd Font **Mono***. The `Mono` face
matters: the non-Mono `MesloLGSDZNF-Regular` renders icons double-width and
misaligns the columns.

### `ghd` — launch into the Nerd Font profile

`tooling/ghd.zsh`, sourced from `~/.zshrc`. Focuses an open gh-dash window if
there is one, else opens a new one, always pinned to the `gh-dash` Terminal
profile. Fails loudly if that profile is missing.

Terminal binds fonts to **profiles**, not windows — a per-window font means a
one-off profile applied to just that window.

### `ghd-repo` — manage per-repo tabs

`~/.bin/ghd-repo` → `tooling/ghd-repo.py`

```bash
ghd-repo ls
ghd-repo add deploys      # bare or Org/deploys
ghd-repo rm  deploys
ghd-repo sync             # regenerate tabs from the All (org) filter
```

The **`All (org)` filter line is the single source of truth**. Every per-repo tab
is generated from it, so changing the window/sort/author means editing one line
then running `sync`. Writes a `.bak`, validates the YAML, restores on failure.

### Filters

- **Ad-hoc:** press **`/`** inside gh dash — the search bar opens pre-filled with
  the current tab's filter. Edit, Enter, refetch. No restart.
- **Persistent:** `~/.config/gh-dash/config.yml`. There is **no hot reload** —
  press `q`, then `ghd`.

### Approve / merge queue

If your repo has GitHub's native merge queue enabled, the built-in `m` uses it:

- `v` → approve
- `m` → `gh pr merge` → **adds to the merge queue** when checks have passed
- direct merge needs `--admin`, which bypasses the queue and required checks

A one-keystroke `--admin` merge is deliberately **not** bound. On any repo where
merges should always be a deliberate human action, that's a footgun. Add it
yourself if you want it:

```yaml
keybindings:
  prs:
    - key: M
      command: gh pr merge --admin --repo {{.RepoName}} {{.PrNumber}}
```

### Editor warning

`mouseMode` is a fork-only key. The `yaml-language-server` schema is fetched from
upstream `gh-dash.dev`, which doesn't know it, so your editor may squiggle it.
Harmless.

---

## Keys worth remembering

| gesture | action |
| --- | --- |
| click a row | select it (nothing opens) |
| double-click a row | open the PR |
| click `#1234` | open the PR |
| click CI / review / comments icon | open `/checks` / `/files` / conversation |
| click a section tab | switch section |
| click footer PRs/Issues/Notifications | switch view |
| click footer `? help` | toggle full help |
| click a preview tab | switch detail tab (Overview/Commits/Checks/…) |
| drag | highlight text; copies on release |
| drag the divider | resize the preview (remembered across runs) |
| ⌥-drag | terminal's own selection (reaches scrollback) |
| wheel | move the row selection |

| key | action |
| --- | --- |
| `/` | edit the current tab's filter, live |
| `←/h` `→/l` | previous / next tab |
| `↑/k` `↓/j` | move the selection |
| `o` | open on GitHub |
| `y` / `Y` | copy PR number / URL |
| `v` | approve |
| `m` | merge (→ merge queue) |
| `r` / `R` | refresh this tab / all tabs |
| `?` | full key map |
