package prefs

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMergeMethod_RoundTripsPerRepo(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	require.Equal(t, "", LoadMergeMethod("owner/repo"), "nothing remembered yet")

	require.NoError(t, SaveMergeMethod("owner/repo", "squash"))
	require.NoError(t, SaveMergeMethod("owner/other", "rebase"))

	require.Equal(t, "squash", LoadMergeMethod("owner/repo"))
	require.Equal(t, "rebase", LoadMergeMethod("owner/other"), "one repo must not clobber another")
	require.Equal(t, "", LoadMergeMethod("owner/never-seen"))

	// Repo names are case-insensitive; the answer is the repo's, not the casing's.
	require.Equal(t, "squash", LoadMergeMethod("Owner/Repo"))

	// A later answer replaces the earlier one.
	require.NoError(t, SaveMergeMethod("owner/repo", "merge"))
	require.Equal(t, "merge", LoadMergeMethod("owner/repo"))
}

func TestMergeMethod_IgnoresEmptyInput(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	require.NoError(t, SaveMergeMethod("", "squash"))
	require.NoError(t, SaveMergeMethod("owner/repo", ""))
	require.Equal(t, "", LoadMergeMethod("owner/repo"))
}

// A corrupt prefs file must not take the dashboard down, and must be writable
// again afterwards.
func TestMergeMethod_CorruptFileFallsBackAndRecovers(t *testing.T) {
	d := t.TempDir()
	t.Setenv("XDG_STATE_HOME", d)
	require.NoError(t, os.MkdirAll(filepath.Join(d, "gh-dash"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(d, "gh-dash", "prefs.json"), []byte("{not json"), 0o644))

	require.Equal(t, "", LoadMergeMethod("owner/repo"), "corrupt reads as nothing remembered")
	require.NoError(t, SaveMergeMethod("owner/repo", "squash"))
	require.Equal(t, "squash", LoadMergeMethod("owner/repo"))
}

// The store is keyed by repo only — no instance/window scoping — so every
// gh-dash window on the machine agrees on how a repo merges.
func TestMergeMethod_IsGlobalNotPerInstance(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	t.Setenv("GH_DASH_INSTANCE", "window-a")
	require.NoError(t, SaveMergeMethod("owner/repo", "squash"))

	t.Setenv("GH_DASH_INSTANCE", "window-b")
	require.Equal(t, "squash", LoadMergeMethod("owner/repo"),
		"another dashboard instance must see the same remembered method")
}
