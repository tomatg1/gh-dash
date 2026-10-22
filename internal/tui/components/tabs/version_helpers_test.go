package tabs

import "testing"

func TestWrapVersionAtHyphen(t *testing.T) {
	cases := map[string]string{
		"v4.25.0-local.2":       "v4.25.0\n-local.2",
		"v4.25.0-local.2-3-gab": "v4.25.0\n-local.2-3-gab", // only the first hyphen wraps
		"v4.25.0":               "v4.25.0",                 // no hyphen, no wrap
		"dev":                   "dev",
		"":                      "",
	}
	for in, want := range cases {
		if got := wrapVersionAtHyphen(in); got != want {
			t.Errorf("wrapVersionAtHyphen(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsLocalVersion(t *testing.T) {
	local := []string{"", "dev", "v4.25.0-local.2", "v4.25.0-local.2-3-gabc123", "v4.25.0-dirty"}
	release := []string{"v4.25.0", "v4.25.1"}
	for _, v := range local {
		if !isLocalVersion(v) {
			t.Errorf("%q should be treated as a local build", v)
		}
	}
	for _, v := range release {
		if isLocalVersion(v) {
			t.Errorf("%q should be treated as a release", v)
		}
	}
}
