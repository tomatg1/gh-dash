// Package prefs stores small, durable per-repo choices that gh-dash learns at
// runtime -- things the user answered once and shouldn't be asked again.
//
// Deliberately keyed by repo and NOT by dashboard instance: "how do we merge
// owner/repo" is a property of the repo, so every gh-dash window on the machine
// should agree on it. (Contrast internal/tui/layoutstate.go, whose keys are
// per-instance because layout and selection are per-window.)
//
// Everything here is best-effort: a failure degrades to "nothing remembered",
// never to an error the user has to care about.
package prefs

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// state is the on-disk shape of the prefs file.
type state struct {
	// MergeMethod maps "owner/name" (lowercased) -> "squash"|"merge"|"rebase".
	MergeMethod map[string]string `json:"mergeMethod,omitempty"`
}

// dir follows the XDG state spec -- state that should persist but isn't
// configuration, and that the user never edits by hand.
func dir() string {
	if d := os.Getenv("XDG_STATE_HOME"); d != "" {
		return filepath.Join(d, "gh-dash")
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	return filepath.Join(home, ".local", "state", "gh-dash")
}

func path() string {
	d := dir()
	if d == "" {
		return ""
	}

	return filepath.Join(d, "prefs.json")
}

func read() state {
	st := state{MergeMethod: map[string]string{}}

	p := path()
	if p == "" {
		return st
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return st // nothing remembered yet, or unreadable
	}
	if err := json.Unmarshal(b, &st); err != nil {
		return state{MergeMethod: map[string]string{}} // corrupt: start over
	}
	if st.MergeMethod == nil {
		st.MergeMethod = map[string]string{}
	}

	return st
}

// write atomically persists st (temp file + rename), so a crash mid-write can
// never leave a half-parsed file behind.
func write(st state) error {
	d := dir()
	if d == "" {
		return nil
	}
	if err := os.MkdirAll(d, 0o755); err != nil {
		return err
	}

	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}

	tmp := filepath.Join(d, "prefs.json.tmp")
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}

	return os.Rename(tmp, path())
}

func repoKey(repoNameWithOwner string) string {
	return strings.ToLower(strings.TrimSpace(repoNameWithOwner))
}

// LoadMergeMethod returns the remembered merge strategy for a repo, or "".
func LoadMergeMethod(repoNameWithOwner string) string {
	if repoNameWithOwner == "" {
		return ""
	}

	return read().MergeMethod[repoKey(repoNameWithOwner)]
}

// SaveMergeMethod remembers the merge strategy for a repo, merging into whatever
// else is stored. Callers should only record a method that actually worked, so a
// rejected strategy (one the repo doesn't allow) is never learned.
func SaveMergeMethod(repoNameWithOwner, method string) error {
	if dir() == "" || repoNameWithOwner == "" || method == "" {
		return nil
	}

	st := read()
	st.MergeMethod[repoKey(repoNameWithOwner)] = strings.ToLower(method)

	return write(st)
}
