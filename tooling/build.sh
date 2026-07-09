#!/usr/bin/env bash
# Build this gh-dash fork with its local version string baked into the logo.
#
# The gh extension is a symlink to this repo, so this rebuilds the binary in
# place; `gh dash` picks it up on the next launch. A plain `go build` also works
# but leaves the logo showing "dev" instead of the git-describe version.
set -euo pipefail

# repo root = parent of this tooling/ dir, wherever it's checked out
REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VERSION="$(git -C "$REPO" describe --tags --always --dirty 2>/dev/null || echo dev)"

go -C "$REPO" build \
  -ldflags "-X 'github.com/dlvhdr/gh-dash/v4/internal/tui.Version=${VERSION}'" \
  -o "$REPO/gh-dash" .

echo "built $REPO/gh-dash  (version: $VERSION)"
