#!/usr/bin/env bash
# Restart every running gh-dash so it picks up a freshly built binary.
#
# A running process keeps its old memory image: replacing the file on disk
# (tooling/build.sh writes the very inode the gh extension serves) does NOT
# update anything already running. `strings` on the path proves nothing about a
# running process -- the only proof is that each pid started AFTER the binary's
# mtime, which is what this script verifies before exiting.
#
# Terminal tabs are driven by AppleScript matched on the exact tty. Any tab
# running `claude` is refused outright, so this can never type into the session
# that invoked it.
#
# Known limitation: a gh-dash living in an abduco/dtach-style detached pane is
# not a Terminal tab, so it can't be reached this way -- attaching a second
# abduco client returns zero bytes (no repaint for a late joiner), so keystrokes
# are blind. Those are reported at the end for a manual `q` + `ghd`.
set -uo pipefail

BIN="$HOME/.local/share/gh/extensions/gh-dash/gh-dash"
[[ -x "$BIN" ]] || { echo "no gh-dash binary at $BIN" >&2; exit 1; }
BIN_EPOCH="$(stat -f %m "$BIN")"
echo "binary: $BIN ($(stat -f '%Sm' -t '%b %e %H:%M:%S' "$BIN"))"

# tty per running instance, e.g. "ttys001". Instances with no controlling
# terminal come back as "??" and are reported as unreachable.
ttys="$(pgrep -x gh-dash | while read -r p; do ps -o tty= -p "$p" | tr -d ' '; done | sort -u)"
[[ -n "$ttys" ]] || { echo "no gh-dash running; nothing to restart"; exit 0; }

unreachable=()
for tty in $ttys; do
  if [[ "$tty" == "??" || -z "$tty" ]]; then
    unreachable+=("(no tty)")
    continue
  fi
  dev="/dev/$tty"
  echo "--- $dev"

  # Quit. Note: `try` is deliberately absent and tabs are never addressed by
  # `index of t` -- Terminal's tab class has no index property, so that errors
  # and, inside a try, silently skips the very tab being looked for.
  quit=$(osascript <<AS 2>&1
tell application "Terminal"
  repeat with w in windows
    repeat with t in tabs of w
      if (tty of t) is "$dev" then
        if ((processes of t) as string) contains "claude" then return "REFUSED: claude in this tab"
        if ((processes of t) as string) contains "gh-dash" then do script "q" in t
        return "quit sent"
      end if
    end repeat
  end repeat
  return "not a Terminal tab"
end tell
AS
)
  echo "    $quit"
  case "$quit" in
    "quit sent") ;;
    *) unreachable+=("$dev: $quit"); continue ;;
  esac

  sleep 2

  echo "    $(osascript <<AS 2>&1
tell application "Terminal"
  repeat with w in windows
    repeat with t in tabs of w
      if (tty of t) is "$dev" then
        if ((processes of t) as string) contains "gh-dash" then return "still running -- not relaunching"
        do script "cd ~ && gh dash" in t
        return "launched"
      end if
    end repeat
  end repeat
  return "tab vanished"
end tell
AS
)"

  # Re-pin the Nerd Font profile; `do script` in an existing tab keeps the tab's
  # settings, but a tab that was reused from elsewhere may not have them.
  osascript <<AS >/dev/null 2>&1
delay 3
tell application "Terminal"
  set gd to settings set "gh-dash"
  repeat with w in windows
    repeat with t in tabs of w
      if (tty of t) is "$dev" then set current settings of t to gd
    end repeat
  end repeat
end tell
AS
done

sleep 1

# Proof, not assumption: every surviving instance must have started after the
# binary was written.
echo "--- verification"
stale=0
running=0
for p in $(pgrep -x gh-dash); do
  running=$((running + 1))
  started="$(ps -o lstart= -p "$p")"
  started_epoch="$(date -j -f "%a %b %e %T %Y" "$(echo "$started" | xargs)" +%s 2>/dev/null || echo 0)"
  if [[ "$started_epoch" -ge "$BIN_EPOCH" ]]; then
    echo "    pid $p on $(ps -o tty= -p "$p" | tr -d ' '): started $(echo "$started" | xargs) -- fresh"
  else
    echo "    pid $p on $(ps -o tty= -p "$p" | tr -d ' '): started $(echo "$started" | xargs) -- STALE (predates the build)"
    stale=$((stale + 1))
  fi
done
[[ "$running" -gt 0 ]] || echo "    no gh-dash running after restart"

if ((${#unreachable[@]})); then
  echo "--- not restartable programmatically (needs a manual q + ghd):"
  printf '    %s\n' "${unreachable[@]}"
fi

if [[ "$stale" -gt 0 ]] || ((${#unreachable[@]})); then
  exit 1
fi
echo "all instances are running the current binary"
