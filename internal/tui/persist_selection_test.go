package tui

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSaveLoadSelection_RoundTripAndScoping(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	require.NoError(t, saveSelection("dir:/work", "All", "https://gh/o/r/pull/1"))
	require.NoError(t, saveSelection("dir:/work", "Mine", "https://gh/o/r/pull/2"))
	require.NoError(t, saveSelection("dir:/personal", "All", "https://gh/o/r/pull/9"))

	require.Equal(t, "https://gh/o/r/pull/1", loadSelection("dir:/work", "All"))
	require.Equal(t, "https://gh/o/r/pull/2", loadSelection("dir:/work", "Mine"),
		"different sections in the same instance are independent")
	require.Equal(t, "https://gh/o/r/pull/9", loadSelection("dir:/personal", "All"),
		"different instances are independent")
	require.Equal(t, "", loadSelection("dir:/work", "Never"), "unset section => empty")
}

func TestSaveSelection_EmptyClears(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	require.NoError(t, saveSelection("i", "s", "u"))
	require.Equal(t, "u", loadSelection("i", "s"))
	require.NoError(t, saveSelection("i", "s", ""))
	require.Equal(t, "", loadSelection("i", "s"), "empty url clears the remembered selection")
}

// layout + selection must coexist in the one state file.
func TestState_LayoutAndSelectionCoexist(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	require.NoError(t, savePreviewHeight("dir:/work", 15))
	require.NoError(t, saveSelection("dir:/work", "All", "u1"))

	gotH, okH := loadPreviewHeight("dir:/work")
	require.True(t, okH, "saving selection must not drop the layout")
	require.Equal(t, 15, gotH)
	require.Equal(t, "u1", loadSelection("dir:/work", "All"))
}
