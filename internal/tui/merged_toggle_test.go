package tui

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// The `M` toggle is remembered per dashboard instance, so one window hiding
// merged PRs doesn't change another's view.
func TestMergedHidden_PersistsPerInstance(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	require.False(t, loadMergedHidden("instance:a"), "merged PRs show by default")

	require.NoError(t, saveMergedHidden("instance:a", true))
	require.True(t, loadMergedHidden("instance:a"))
	require.False(t, loadMergedHidden("instance:b"), "another instance is unaffected")

	// Toggling back deletes the key rather than storing false.
	require.NoError(t, saveMergedHidden("instance:a", false))
	require.False(t, loadMergedHidden("instance:a"))
}

// Persisting the toggle must not disturb the other layout state sharing the file.
func TestMergedHidden_CoexistsWithLayoutState(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	require.NoError(t, savePreviewHeight("instance:a", 14))
	require.NoError(t, saveSelection("instance:a", "Mine", "https://github.com/o/r/pull/1"))
	require.NoError(t, saveMergedHidden("instance:a", true))

	gotH, okH := loadPreviewHeight("instance:a")
	require.True(t, okH)
	require.Equal(t, 14, gotH)
	require.Equal(t, "https://github.com/o/r/pull/1", loadSelection("instance:a", "Mine"))
	require.True(t, loadMergedHidden("instance:a"))
}
