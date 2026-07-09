package tui

import (
	"regexp"

	tea "charm.land/bubbletea/v2"

	"github.com/dlvhdr/gh-dash/v4/internal/config"
	"github.com/dlvhdr/gh-dash/v4/internal/urlopen"
)

var (
	repoFilterRe = regexp.MustCompile(`repo:([\w.-]+/[\w.-]+)`)
	orgFilterRe  = regexp.MustCompile(`org:([\w.-]+)`)
)

// firstRepoPullsURL derives a "PR list" URL from the instance's PR sections: the
// first repo: found across them (in section order), else the first org:, else
// the user's own PR inbox.
func firstRepoPullsURL(sections []config.PrsSectionConfig) string {
	for _, s := range sections {
		if m := repoFilterRe.FindStringSubmatch(s.Filters); m != nil {
			return "https://github.com/" + m[1] + "/pulls"
		}
	}
	for _, s := range sections {
		if m := orgFilterRe.FindStringSubmatch(s.Filters); m != nil {
			return "https://github.com/orgs/" + m[1] + "/pulls"
		}
	}

	return "https://github.com/pulls"
}

// prewarmBrowserCmd opens the first configured repo's PR list on launch so the
// urlOpenCommand's browser/profile window is ready — and its window-id cache
// warm — before the first real open, making that open instant.
//
// No-op unless urlOpenCommand is set and prewarm isn't disabled. Returned as a
// tea.Cmd, which bubbletea runs in its own goroutine, so it never blocks startup.
func (m *Model) prewarmBrowserCmd() tea.Cmd {
	cmdTemplate := m.ctx.Config.Defaults.URLOpenCommand
	if cmdTemplate == "" || m.ctx.Config.Defaults.DisableBrowserPrewarm {
		return nil
	}

	url := firstRepoPullsURL(m.ctx.Config.PRSections)
	if url == "" {
		return nil
	}

	return func() tea.Msg {
		_ = urlopen.Open(cmdTemplate, url)
		return nil
	}
}
