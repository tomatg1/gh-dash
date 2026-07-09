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

**On "each instance":** no per-window identity survives a restart — a tty is
recycled, a terminal session id is minted fresh. So the key is what actually
defines the dashboard: its config path and the repo it was launched in
(`global|` for you). Export **`GH_DASH_INSTANCE=left-window`** to give one
particular window its own remembered layout.

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
~/code/gh-dash-build.sh
```

| tag | contents |
| --- | --- |
| `v4.25.0-local.1` | mouseMode config, clickable tabs/rows, wheel, drag-select + click-to-open, non-git-repo startup fix |
| `v4.25.0-local.2` | + double-click/number-to-open, clickable icons + footer + preview tabs, draggable & persisted divider, help-toggle fix, sloppy-click drag threshold, prior-row-clear fix, injectable version string |

Remember: the checkout **is** the installed extension, so rebuild after any
`git checkout` or `gh dash` serves a stale binary.

### The version string in the logo

The logo shows `git describe --tags` (e.g. `v4.25.0-local.2`), injected at build
time via `-ldflags -X …/internal/tui.Version=…`. **This is why you build through
`gh-dash-build.sh`** — a bare `go build` has no version to inject and the logo
falls back to "dev". The "Update available!" nag is suppressed for local builds
(a fork is always ahead of the upstream tag, so the nag would be permanent and
misleading).

## Build / install

The fork directory **is** the installed extension (gh symlinks to it):

```
~/.local/share/gh/extensions/gh-dash -> ~/code/gh-dash
```

```bash
# build (bakes the git-describe version into the logo; fully pathed, no cd)
~/code/gh-dash-build.sh

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
~/code/gh-dash-build.sh
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

## Surrounding setup (not part of the fork)

### Fonts

gh-dash draws its state icons with **Nerd Font** glyphs. Terminal.app has no
glyph fallback — it can only draw glyphs present in the one selected font — so
the `?` tofu boxes mean "wrong font", never "broken config".

```bash
brew install --cask font-meslo-lg-nerd-font
osascript -e 'tell application "Terminal" to set font name of settings set "gh-dash" to "MesloLGSDZNFM-Regular"'
```

Terminal stores fonts by **PostScript** name, not the family name in the font
panel. `MesloLGSDZNFM-Regular` = *MesloLGSDZ Nerd Font **Mono***. The `Mono` face
matters: the non-Mono `MesloLGSDZNF-Regular` renders icons double-width and
misaligns the columns.

### `ghd` — launch into the Nerd Font profile

Shell function in `~/.zshrc`. Focuses an open gh-dash window if there is one,
else opens a new one, always pinned to the `gh-dash` Terminal profile. Fails
loudly if that profile is missing.

Terminal binds fonts to **profiles**, not windows — a per-window font means a
one-off profile applied to just that window.

### `ghd-repo` — manage per-repo tabs

`~/.bin/ghd-repo` → `~/.local/lib/gh-dash/ghd-repo.py`

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
