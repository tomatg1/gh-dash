package tui

import (
	"os"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"

	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/prssection"
)

// prsModelWithSections builds the model the way the app does: sections created
// through the same FetchAllSections the refresh tick uses, so index 0 is the
// search section and the real one sits at 1.
func prsModelWithSections(t *testing.T) Model {
	t.Helper()
	// Don't touch the real layout.json. Only claim a dir if the test hasn't
	// already set one -- a restart is simulated by building a second model, and
	// it has to read the state the first one wrote.
	if os.Getenv("XDG_STATE_HOME") == "" {
		t.Setenv("XDG_STATE_HOME", t.TempDir())
	}

	m := bottomPreviewModel(t)
	newSections, _ := m.fetchAllViewSections()
	m.setCurrentViewSections(newSections)
	m.currSectionId = 1

	return m
}

func currPRSection(t *testing.T, m Model) *prssection.Model {
	t.Helper()
	s := m.getCurrSection()
	require.NotNil(t, s, "expected a current PR section")
	ps, ok := s.(*prssection.Model)
	require.True(t, ok, "current section should be a PR section")

	return ps
}

// The refresh tick rebuilds every section from scratch. Anything not explicitly
// carried over is destroyed -- and a filter being typed right now is the most
// expensive thing to lose, because the edit is gone with no way to recover it.
func TestIntervalRefresh_DoesNotInterruptFilterEditing(t *testing.T) {
	m := prsModelWithSections(t)

	// Open the search bar and type into it, through the real key path.
	next, _ := m.Update(tea.KeyPressMsg{Text: "/"})
	m = next.(Model)
	require.True(t, currPRSection(t, m).IsSearchFocused(), "`/` should focus the search bar")

	for _, r := range "author:@me" {
		next, _ = m.Update(tea.KeyPressMsg{Text: string(r)})
		m = next.(Model)
	}
	typed := currPRSection(t, m).SearchBar.Value()
	require.Contains(t, typed, "author:@me", "the typed text should be in the search bar")

	// The refetch fires while the edit is still in progress.
	next, _ = m.Update(intervalRefresh(time.Now()))
	m = next.(Model)

	after := currPRSection(t, m)
	require.True(t, after.IsSearchFocused(),
		"a refresh must not close the search bar mid-edit")
	require.Equal(t, typed, after.SearchBar.Value(),
		"a refresh must not discard what was typed")
}

// A filter that was actually applied (Enter) is the section's filter until the
// user changes it. The refresh must not silently put the configured one back --
// the list would quietly repopulate with rows the user had filtered out.
func TestIntervalRefresh_KeepsAnAppliedFilter(t *testing.T) {
	m := prsModelWithSections(t)
	configured := currPRSection(t, m).GetSearchValue()

	next, _ := m.Update(tea.KeyPressMsg{Text: "/"})
	m = next.(Model)
	edited := "is:open author:@me label:bug"
	currPRSection(t, m).SearchBar.SetValue(edited)
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)

	applied := currPRSection(t, m)
	require.Equal(t, edited, applied.GetSearchValue(), "Enter should apply the edit")
	require.NotEqual(t, configured, edited, "test is meaningless if they match")

	next, _ = m.Update(intervalRefresh(time.Now()))
	m = next.(Model)

	require.Equal(t, edited, currPRSection(t, m).GetSearchValue(),
		"a refresh must not revert an applied filter to the configured one")
}

// A `/` edit is worth keeping across a restart, but only as an override: clear
// it and the section goes back to whatever config.yml says, which stays the
// source of truth (gh-dash never writes it).
func TestFilterEdit_PersistsPerInstanceAndFallsBackToConfig(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir()) // shared by both models in this test
	m := prsModelWithSections(t)
	m.layoutStateKey = "instance:test"
	configured := currPRSection(t, m).GetConfig().Filters

	apply := func(m Model, filter string) Model {
		next, _ := m.Update(tea.KeyPressMsg{Text: "/"})
		m = next.(Model)
		currPRSection(t, m).SearchBar.SetValue(filter)
		next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

		return next.(Model)
	}

	edited := "is:open author:@me label:bug"
	m = apply(m, edited)
	require.Equal(t, edited, loadFilter("instance:test", "Mine"),
		"applying a filter should remember it for this instance")

	// A fresh launch of the same instance adopts it, templates and all -- the
	// raw filter is stored, never the template-expanded one.
	fresh := prsModelWithSections(t)
	fresh.layoutStateKey = "instance:test"
	sections, _ := fresh.fetchAllViewSections()
	fresh.restorePersistedFilters(sections)
	fresh.setCurrentViewSections(sections)
	require.Equal(t, edited, currPRSection(t, fresh).GetSearchState().Applied,
		"a restart should come back to the remembered filter")

	// Putting the configured filter back clears the override rather than
	// remembering a duplicate of it.
	m = apply(m, configured)
	require.Empty(t, loadFilter("instance:test", "Mine"),
		"restoring the configured filter should drop the override")
}

// The restore has to be wired into startup, not merely available: without the
// call in the initMsg handler, filters persist to disk and are never read back,
// which looks identical to not persisting at all.
func TestInitMsg_RestoresPersistedFilter(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	m := prsModelWithSections(t)
	m.layoutStateKey = "instance:init"

	remembered := "is:open author:@me label:regression"
	require.NoError(t, saveFilter("instance:init", "Mine", remembered))

	next, _ := m.Update(initMsg{Config: *m.ctx.Config})
	m = next.(Model)

	require.Equal(t, remembered, currPRSection(t, m).GetSearchState().Applied,
		"startup should adopt this instance's remembered filter")
}
