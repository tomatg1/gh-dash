# ghd — launch `gh dash` in its own Nerd Font Terminal profile.
#
# Focuses an already-open gh-dash window if there is one, else opens a new one,
# and always pins it to the "gh-dash" Terminal profile (created by
# setup-terminal.sh). Targets the tab that `do script` returns (never "front
# window", which races), and fails LOUDLY if the profile is missing instead of
# silently using the wrong font.
ghd() {
  osascript >/dev/null <<'AS'
tell application "Terminal"
  set gd to settings set "gh-dash"   -- errors loudly if the profile is missing
  repeat with w in windows
    repeat with t in tabs of w
      try
        if (processes of t) contains "gh-dash" then
          set current settings of t to gd
          set selected of t to true
          set index of w to 1
          activate
          return
        end if
      end try
    end repeat
  end repeat
  set newTab to do script "gh dash"  -- returns the NEW tab: no front-window race
  set current settings of newTab to gd
  activate
end tell
AS
}
