package tui

import (
	"cmp"
	"fmt"
	"slices"
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
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/prview"
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
		// At least an hour old: ages are formatted to the second, and a fixture
		// reading "now" would read "1s" in a model built a moment later.
		at := time.Now().Add(-time.Duration(i+1) * time.Hour)
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
//
// Each model gets its own state directory. Models built in one test would
// otherwise share layout.json, and one model's remembered selection would be
// restored into the next -- two "identical" models starting on different rows.
func perfModel(tb testing.TB, rowsPerTab ...int) Model {
	tb.Helper()
	tb.Setenv("XDG_STATE_HOME", tb.TempDir())
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

// BenchmarkScrollDuringSync replays scrolling while a refresh is in flight --
// the situation where the event loop used to fall seconds behind. 3s of events
// arrive (spinner ticks at the 34/s measured during a refetch, trackpad wheel
// events at 40/s), then each tab's results land and the enrichments the
// scrolling requested come back. The loop handles them one at a time; behind_ms
// is how much longer than the 3s it needed, i.e. how far the screen lags.
func BenchmarkScrollDuringSync(b *testing.B) {
	for _, rows := range []int{20, 40} {
		b.Run(fmt.Sprintf("rows=%d", rows), func(b *testing.B) {
			var behind time.Duration
			for b.Loop() {
				behind += scrollDuringSync(b, rows)
			}
			b.ReportMetric(float64(behind.Milliseconds())/float64(b.N), "behind_ms")
		})
	}
}

func scrollDuringSync(b *testing.B, rows int) time.Duration {
	b.StopTimer()
	m := perfModel(b, rows, rows, 1)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 112, Height: 50})
	m = next.(Model)
	b.StartTimer()

	const window = 3 * time.Second
	t0 := time.Now()
	next, _ = m.Update(intervalRefresh(time.Now()))
	m = next.(Model)
	_ = m.View()

	type arrival struct {
		at   time.Duration
		tick bool
	}
	var sched []arrival
	for k := range 34 * 3 {
		sched = append(sched, arrival{time.Duration(k) * time.Second / 34, true})
	}
	for j := range 40 * 3 {
		sched = append(sched, arrival{time.Duration(j) * time.Second / 40, false})
	}
	slices.SortStableFunc(sched, func(a, c arrival) int { return cmp.Compare(a.at, c.at) })
	wheels := 0
	for _, a := range sched {
		var ev tea.Msg = spinner.TickMsg{}
		if !a.tick {
			ev = wheelEvent(wheels * 15 / 30) // flicks alternate every 30 events
			wheels++
		}
		m = step(m, ev)
	}

	for id := 1; id <= 3; id++ {
		ps := m.prs[id].(*prssection.Model)
		n := []int{rows, rows, 1}[id-1]
		m = step(m, constants.TaskFinishedMsg{
			SectionId: ps.Id, SectionType: prssection.SectionType, TaskId: ps.LastFetchTaskId,
			Msg: prssection.SectionPullRequestsFetchedMsg{Prs: perfPRs(id-1, n), TotalCount: n,
				TaskId: ps.LastFetchTaskId, PageInfo: data.PageInfo{HasNextPage: false}},
		})
	}
	ps := m.prs[2].(*prssection.Model)
	for i := range min(40, len(ps.Prs)) {
		m = step(m, prview.EnrichedPrMsg{Id: ps.Id, Type: prssection.SectionType, Data: ps.Prs[i].Enriched})
	}

	return max(0, time.Since(t0)-window)
}
