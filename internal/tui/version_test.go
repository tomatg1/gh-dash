package tui

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dlvhdr/gh-dash/v4/internal/config"
)

func TestVersion_UsesInjectedValue(t *testing.T) {
	orig := Version
	Version = "v4.25.0-local.test"
	t.Cleanup(func() { Version = orig })

	m := NewModel(config.Location{SkipGlobalConfig: true}, Repositories{})
	require.Equal(t, "v4.25.0-local.test", m.ctx.Version,
		"the logo version should come from the injected Version var")
}

func TestVersion_FallsBackWhenDev(t *testing.T) {
	orig := Version
	Version = "dev"
	t.Cleanup(func() { Version = orig })

	m := NewModel(config.Location{SkipGlobalConfig: true}, Repositories{})
	require.NotEmpty(t, m.ctx.Version, "there is always some version string")
}
