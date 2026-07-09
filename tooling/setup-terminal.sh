#!/usr/bin/env bash
# Font + Terminal.app profile + `ghd` launcher that gh-dash's icons need.
# macOS + Terminal.app only. Idempotent: skips anything already in place.
#
# Why this exists: Terminal.app has no glyph fallback, so gh-dash's Nerd Font
# icons render as tofu boxes unless the terminal uses a font that bundles them.
# This installs such a font, creates a dedicated "gh-dash" Terminal profile that
# uses it, and wires up the `ghd` launcher that opens gh dash in that profile.
set -euo pipefail

FONT_CASK="font-meslo-lg-nerd-font"
FONT_PS_NAME="MesloLGSDZNFM-Regular"  # PostScript name Terminal stores: Meslo, dotted-zero, Mono
PROFILE="gh-dash"
FONT_SIZE=12
TOOLING_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

if [[ "$(uname)" != "Darwin" ]]; then
  echo "setup-terminal.sh is macOS/Terminal.app only — skipping." >&2
  exit 0
fi

# 1. Nerd Font. The Mono face keeps gh-dash's columns aligned.
if brew list --cask "$FONT_CASK" >/dev/null 2>&1; then
  echo "font: $FONT_CASK already installed"
else
  echo "font: installing $FONT_CASK ..."
  brew install --cask "$FONT_CASK"
fi

# 2. Terminal profile. AppleScript can create a settings set and set its font by
#    PostScript name. Only created if missing (existing profiles are left alone).
osascript <<AS >/dev/null
tell application "Terminal"
  if not (exists settings set "$PROFILE") then
    set s to make new settings set with properties {name:"$PROFILE"}
    set font name of s to "$FONT_PS_NAME"
    set font size of s to $FONT_SIZE
  end if
end tell
AS
echo "profile: '$PROFILE' ready (font $FONT_PS_NAME @ ${FONT_SIZE}pt)"

# 3. `ghd` launcher, sourced from ~/.zshrc.
GHD_SRC="$TOOLING_DIR/ghd.zsh"
MARKER="# gh-dash: source ghd launcher"
if grep -qF "$MARKER" ~/.zshrc 2>/dev/null || grep -q '^ghd()' ~/.zshrc 2>/dev/null; then
  echo "launcher: ghd already wired into ~/.zshrc"
else
  {
    echo ""
    echo "$MARKER"
    echo "[ -f \"$GHD_SRC\" ] && source \"$GHD_SRC\""
  } >>~/.zshrc
  echo "launcher: added ghd to ~/.zshrc — run 'source ~/.zshrc' or open a new shell"
fi
