// Package urlopen opens URLs in a browser, optionally via a user-configured
// command so links can be routed to a specific browser or profile.
package urlopen

import (
	"bytes"
	"io"
	"os/exec"
	"strings"
	"text/template"

	"github.com/cli/go-gh/v2/pkg/browser"
)

// Open opens url in a browser.
//
// When cmdTemplate is non-empty it is rendered as a text/template with {{.URL}}
// and run via `sh -c`, so URLs can be routed to a specific browser or profile
// (e.g. a Chrome profile already signed in to the right account):
//
//	"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" --profile-directory="Profile 3" "{{.URL}}"
//
// When empty, the OS default browser is used (go-gh's browser).
//
// stdout/stderr are discarded so launcher noise (xdg-open / GTK warnings, a
// browser's own logging) can't leak into the TUI and corrupt the display.
func Open(cmdTemplate, url string) error {
	if strings.TrimSpace(cmdTemplate) == "" {
		return browser.New("", io.Discard, io.Discard).Browse(url)
	}

	tmpl, err := template.New("urlOpenCommand").Parse(cmdTemplate)
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, map[string]string{"URL": url}); err != nil {
		return err
	}

	cmd := exec.Command("sh", "-c", buf.String())
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard

	return cmd.Run()
}
