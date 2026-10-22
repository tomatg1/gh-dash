package prssection

import "testing"

// The picker must only merge on a deliberate strategy answer: a bare Enter or a
// stray key cancels rather than merging with a guessed method.
func TestMergeMethodFromKey(t *testing.T) {
	cases := map[string]string{
		"s":      "squash",
		"S":      "squash",
		"squash": "squash",
		"m":      "merge",
		"merge":  "merge",
		"r":      "rebase",
		"rebase": "rebase",
		" s ":    "squash",
		"":       "",
		"y":      "",
		"x":      "",
		"sq":     "",
	}
	for in, want := range cases {
		if got := mergeMethodFromKey(in); got != want {
			t.Errorf("mergeMethodFromKey(%q) = %q, want %q", in, got, want)
		}
	}
}
