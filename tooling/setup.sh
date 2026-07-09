#!/usr/bin/env bash
# Set up this gh-dash fork on a machine, end to end. Idempotent — safe to re-run.
#
#   1. build the versioned binary
#   2. install it as a local `gh` extension (symlinked to this checkout)
#   3. (macOS) Nerd Font + Terminal profile + `ghd` launcher
#   4. put the `ghd-repo` helper on PATH
#
# See FORK_NOTES.md for what each piece does.
set -euo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TOOLING="$REPO/tooling"

echo "==> building"
"$TOOLING/build.sh"

echo "==> installing gh extension (local, from this checkout)"
if gh extension list 2>/dev/null | grep -q 'gh dash'; then
  gh extension remove dash 2>/dev/null || true
fi
( cd "$REPO" && gh extension install . )

if [[ "$(uname)" == "Darwin" ]]; then
  echo "==> terminal font/profile/launcher"
  "$TOOLING/setup-terminal.sh"
fi

echo "==> linking ghd-repo helper"
mkdir -p "$HOME/.bin"
ln -sf "$TOOLING/ghd-repo.py" "$HOME/.bin/ghd-repo"
echo "linked ~/.bin/ghd-repo -> $TOOLING/ghd-repo.py (ensure ~/.bin is on PATH)"

echo ""
echo "done. open a new terminal (or 'source ~/.zshrc'), then run:  ghd"
