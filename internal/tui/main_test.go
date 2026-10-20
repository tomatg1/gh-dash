package tui

import (
	"os"
	"testing"
)

// No test in this package may open a real browser. URL opens with no
// urlOpenCommand configured go through go-gh's browser, which honours these;
// a configured urlOpenCommand is the test's own responsibility to neutralise.
func TestMain(m *testing.M) {
	os.Setenv("BROWSER", "true")
	os.Setenv("GH_BROWSER", "true")
	os.Exit(m.Run())
}
