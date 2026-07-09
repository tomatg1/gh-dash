package urlopen

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// The custom command must receive the substituted URL. Proven by having the
// command write {{.URL}} to a file and reading it back — no real browser opened.
func TestOpen_CustomCommandReceivesURL(t *testing.T) {
	out := filepath.Join(t.TempDir(), "url.txt")
	url := "https://github.com/owner/repo/pull/12/checks"

	err := Open(`printf '%s' "{{.URL}}" > `+out, url)
	require.NoError(t, err)

	got, err := os.ReadFile(out)
	require.NoError(t, err)
	require.Equal(t, url, string(got), "the command should receive the exact URL")
}

func TestOpen_BadTemplateErrors(t *testing.T) {
	require.Error(t, Open("echo {{.URL", "https://example.com"),
		"an unparseable template should surface an error, not silently no-op")
}

// A URL is substituted verbatim; the user quotes {{.URL}} in their template.
func TestOpen_URLWithQueryString(t *testing.T) {
	out := filepath.Join(t.TempDir(), "url.txt")
	url := "https://github.com/o/r/pull/1?tab=files&x=y"

	require.NoError(t, Open(`printf '%s' "{{.URL}}" > `+out, url))
	got, _ := os.ReadFile(out)
	require.Equal(t, url, string(got))
}
