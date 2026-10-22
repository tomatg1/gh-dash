package tui

// Version is the string shown next to the logo. It defaults to "dev" and is
// meant to be overridden at build time via the linker:
//
//	go build -ldflags "-X 'github.com/dlvhdr/gh-dash/v4/internal/tui.Version=$(git describe --tags --always --dirty)'"
//
// A plain `go build` (no ldflags) has no module version to read, so without
// this override the logo just says "dev". The fork's build recipe sets it to
// `git describe`, so the logo shows which local build is running.
var Version = "dev"
