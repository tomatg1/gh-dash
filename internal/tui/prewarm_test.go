package tui

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dlvhdr/gh-dash/v4/internal/config"
)

func sec(f string) config.PrsSectionConfig { return config.PrsSectionConfig{Filters: f} }

func TestFirstRepoPullsURL(t *testing.T) {
	require.Equal(t, "https://github.com/AutonomousTechnologies/autonomous/pulls",
		firstRepoPullsURL([]config.PrsSectionConfig{
			sec("org:AutonomousTechnologies is:open author:@me"),
			sec("repo:AutonomousTechnologies/autonomous is:open"),
		}), "first repo across sections wins, even if an org section is first")

	require.Equal(t, "https://github.com/orgs/acme/pulls",
		firstRepoPullsURL([]config.PrsSectionConfig{sec("org:acme is:open author:@me")}),
		"no repo, an org -> org pulls")

	require.Equal(t, "https://github.com/pulls",
		firstRepoPullsURL([]config.PrsSectionConfig{sec("is:open author:@me")}),
		"neither -> the user's PR inbox")

	require.Equal(t, "https://github.com/my-org/repo.js/pulls",
		firstRepoPullsURL([]config.PrsSectionConfig{sec("repo:my-org/repo.js is:open")}))
}
