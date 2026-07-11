#!/usr/bin/env bash
# Fast-open a URL in a specific Google Chrome profile.
#
# Launching the Chrome binary with --profile-directory to hand off a URL takes
# ~4s. Talking to the already-running Chrome via AppleScript takes ~0.2s, but
# AppleScript can't target a *profile*. So: cache the target profile's window id
# and add a tab to it (fast); use the slow binary launch only to bootstrap or
# repair that cache (e.g. after the window is closed).
#
# Usage:  open-url.sh "<profile-directory>" "<url>"
#   e.g.  open-url.sh "Profile 3" "https://github.com/o/r/pull/1"
set -euo pipefail

PROFILE="${1:?profile-directory required}"
URL="${2:?url required}"
CHROME="/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
CACHE_DIR="${XDG_STATE_HOME:-$HOME/.local/state}/gh-dash"
CACHE="$CACHE_DIR/chrome-window-$(printf '%s' "$PROFILE" | tr -c 'A-Za-z0-9' '_')"

# Add a tab to window $1 and bring that window forward. Prints "ok" if the
# window exists.
#
# Two things that are easy to get wrong here:
#   * Reference the window by `window id $1`, NOT a `repeat with w in windows`
#     loop variable. The loop variable is positional (item i); `activate`
#     reorders the windows, so a later `set index of w` would raise whatever
#     window now sits at that old position -- the previously-focused one. An
#     id reference stays pinned to the right window.
#   * Order: select the new tab, `activate`, then `set index ... to 1` LAST.
#     Setting the index before `activate` doesn't stick after `make new tab`.
add_tab() {
  osascript 2>/dev/null <<AS
tell application "Google Chrome"
  if not (exists window id $1) then return "gone"
  make new tab at end of tabs of window id $1 with properties {URL:"$URL"}
  set active tab index of window id $1 to (count of tabs of window id $1)
  activate
  set index of window id $1 to 1
  return "ok"
end tell
AS
}

# Prints "yes" if a Chrome window with id $1 is open (no tab added).
window_exists() {
  osascript 2>/dev/null <<AS
tell application "Google Chrome"
  repeat with w in windows
    if (id of w as string) is "$1" then return "yes"
  end repeat
  return "no"
end tell
AS
}

# Pre-warm mode (GH_DASH_PREWARM set): if the cached window is still open, the
# cache is already warm — do nothing, so we don't add a tab on every launch. If
# it's cold, fall through and bootstrap (which opens the URL + caches the window).
if [[ -n "${GH_DASH_PREWARM:-}" && -f "$CACHE" ]]; then
  wid="$(cat "$CACHE" 2>/dev/null || true)"
  if [[ -n "$wid" && "$(window_exists "$wid")" == "yes" ]]; then
    exit 0
  fi
fi

# Fast path: reuse the cached profile window if it's still open.
if [[ -f "$CACHE" ]]; then
  wid="$(cat "$CACHE" 2>/dev/null || true)"
  if [[ -n "$wid" && "$(add_tab "$wid")" == "ok" ]]; then
    exit 0
  fi
fi

# Bootstrap / repair: the binary launch lands the URL in the right profile and
# brings that window to the front; record its id for next time.
"$CHROME" --profile-directory="$PROFILE" "$URL" >/dev/null 2>&1 || true
mkdir -p "$CACHE_DIR"
for _ in 1 2 3 4 5 6 7 8 9 10; do
  wid="$(osascript -e 'tell application "Google Chrome" to id of front window as string' 2>/dev/null || true)"
  if [[ -n "$wid" ]]; then
    printf '%s' "$wid" >"$CACHE"
    break
  fi
  sleep 0.2
done
