package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	zone "github.com/lrstanley/bubblezone/v2"

	"github.com/dlvhdr/gh-dash/v4/internal/config"
	"github.com/dlvhdr/gh-dash/v4/internal/data"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/prrow"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/prssection"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/constants"
)

// Responsiveness harness. Boots the real Model through initMsg, lands realistic
// data through the real fetch-result path, and measures what one input event
// costs the event loop -- Update AND View, since bubbletea renders after every
// message. The layout mirrors a typical setup: three PR tabs, preview open on
// the right at half width, 180x50.

// A PR body of realistic length: glamour's cost scales with the markdown.
var perfBody = strings.Repeat(`Stacked on #13042, which touches the same file. Review that one first.

## The gaps

`+"`findForeignVaultReference`"+` lets a reference through only when it names the
bound client's vault. For relative tokens it kept a single flag and never
resolved the token, so a relative path could name a sibling's vault.

- resolves each relative token against the working directory
- rejects a token that escapes the bound vault after resolution
- keeps absolute paths on the existing check

`+"```go\nfunc resolve(p string) string { return filepath.Clean(p) }\n```"+`

`, 2)

func perfPRs(section, n int) []prrow.Data {
	out := make([]prrow.Data, 0, n)
	for i := range n {
		num := 13000 - section*100 - i
		url := fmt.Sprintf("https://github.com/acme/monorepo/pull/%d", num)
		title := fmt.Sprintf("fix(scope-%d): a reasonably long pull request title for row %d", section, i)
		at := time.Now().Add(-time.Duration(i) * time.Hour)
		out = append(out, prrow.Data{
			Primary: &data.PullRequestData{
				Id: fmt.Sprintf("PR_%d", num), Number: num, Title: title, Url: url,
				State: "OPEN", Author: struct{ Login string }{Login: "someone"},
				Repository: data.Repository{NameWithOwner: "acme/monorepo"},
				UpdatedAt:  at, CreatedAt: at, Additions: 100 + i, Deletions: 7 * i,
				HeadRefName: fmt.Sprintf("fix/branch-%d", num), BaseRefName: "main",
			},
			Enriched: data.EnrichedPullRequestData{
				Number: num, Title: title, Url: url, State: "OPEN", Body: perfBody,
				Author:     struct{ Login string }{Login: "someone"},
				Repository: data.Repository{NameWithOwner: "acme/monorepo"},
				UpdatedAt:  at, CreatedAt: at,
			},
			IsEnriched: true,
		})
	}

	return out
}

// perfModel boots the app the way main does and lands rowsPerTab PRs in each tab.
func perfModel(tb testing.TB, rowsPerTab ...int) Model {
	tb.Helper()
	zone.NewGlobal()

	cfg, err := config.ParseConfig(config.Location{
		ConfigFlag:       "../config/testdata/test-config.yml",
		SkipGlobalConfig: true,
	})
	if err != nil {
		tb.Fatal(err)
	}
	cfg.Keybindings = config.Keybindings{} // test-config binds `m` etc.; irrelevant here
	cfg.Defaults.Preview.Open = true
	cfg.Defaults.Preview.Width = 0.5
	cfg.Defaults.Preview.Position = "auto"
	cfg.Defaults.RefetchIntervalMinutes = 0
	cfg.Defaults.URLOpenCommand = "" // never reach a real browser
	cfg.Defaults.DisableBrowserPrewarm = true
	cfg.PRSections = nil
	for i := range rowsPerTab {
		cfg.PRSections = append(cfg.PRSections, config.PrsSectionConfig{
			Title: fmt.Sprintf("tab %d", i+1), Filters: "is:open",
		})
	}

	m := NewModel(config.Location{ConfigFlag: "../config/testdata/test-config.yml"}, Repositories{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 180, Height: 50})
	m = next.(Model)
	next, _ = m.Update(initMsg{Config: cfg})
	m = next.(Model)
	next, _ = m.Update(tea.WindowSizeMsg{Width: 180, Height: 50})
	m = next.(Model)

	for i, n := range rowsPerTab {
		ps, ok := m.prs[i+1].(*prssection.Model)
		if !ok {
			tb.Fatalf("tab %d is not a PR section", i+1)
		}
		prs := perfPRs(i, n)
		next, _ = m.Update(constants.TaskFinishedMsg{
			SectionId: ps.Id, SectionType: prssection.SectionType, TaskId: ps.LastFetchTaskId,
			Msg: prssection.SectionPullRequestsFetchedMsg{
				Prs: prs, TotalCount: n, TaskId: ps.LastFetchTaskId,
				PageInfo: data.PageInfo{HasNextPage: false},
			},
		})
		m = next.(Model)
	}
	m.currSectionId = 2 // the busy middle tab, as in the live runs
	_ = m.View()

	return m
}

// step runs one event through the loop the way bubbletea does: Update, then View.
func step(m Model, msg tea.Msg) Model {
	next, _ := m.Update(msg)
	m = next.(Model)
	_ = m.View()

	return m
}

// wheelEvents alternates direction in runs so the cursor sweeps the list rather
// than pinning at an end, which would make every event a no-op.
func wheelEvent(i int) tea.Msg {
	btn := tea.MouseWheelUp
	if (i/15)%2 == 1 {
		btn = tea.MouseWheelDown
	}

	return tea.MouseWheelMsg{X: 20, Y: 20, Button: btn}
}

func BenchmarkResponsiveness(b *testing.B) {
	b.Setenv("XDG_STATE_HOME", b.TempDir())
	for _, rows := range []int{20, 60} {
		b.Run(fmt.Sprintf("wheel/rows=%d", rows), func(b *testing.B) {
			m := perfModel(b, rows, rows, 1)
			i := 0
			b.ReportAllocs()
			for b.Loop() {
				m = step(m, wheelEvent(i))
				i++
			}
		})
		b.Run(fmt.Sprintf("spinnerTick/rows=%d", rows), func(b *testing.B) {
			m := perfModel(b, rows, rows, 1)
			b.ReportAllocs()
			for b.Loop() {
				m = step(m, spinner.TickMsg{})
			}
		})
	}
}
