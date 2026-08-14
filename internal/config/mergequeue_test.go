package config

import (
	"testing"
	"time"
)

func TestUsesMergeQueue(t *testing.T) {
	cases := []struct {
		name  string
		repos []string
		repo  string
		want  bool
	}{
		{"empty means off", nil, "owner/repo", false},
		{"exact match", []string{"owner/repo"}, "owner/repo", true},
		{"case-insensitive", []string{"Owner/Repo"}, "owner/repo", true},
		{"no match", []string{"owner/other"}, "owner/repo", false},
		{"wildcard matches all", []string{"*"}, "any/thing", true},
		{"one of several", []string{"a/b", "owner/repo", "c/d"}, "owner/repo", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := Defaults{MergeQueueRepos: tc.repos}
			if got := d.UsesMergeQueue(tc.repo); got != tc.want {
				t.Errorf("UsesMergeQueue(%q) with %v = %v, want %v", tc.repo, tc.repos, got, tc.want)
			}
		})
	}
}

func TestResolveMergeMethod(t *testing.T) {
	cases := []struct {
		name    string
		global  string
		perRepo map[string]string
		repo    string
		want    string
	}{
		{"nothing configured -> ask", "", nil, "owner/repo", ""},
		{"global default", "squash", nil, "owner/repo", "squash"},
		{"per-repo overrides global", "squash", map[string]string{"owner/repo": "rebase"}, "owner/repo", "rebase"},
		{"per-repo only applies to that repo", "squash", map[string]string{"owner/other": "rebase"}, "owner/repo", "squash"},
		{"per-repo is case-insensitive", "", map[string]string{"Owner/Repo": "merge"}, "owner/repo", "merge"},
		{"value is normalized", "SQUASH", nil, "owner/repo", "squash"},
		{"invalid global -> ask", "fast-forward", nil, "owner/repo", ""},
		{"invalid per-repo falls back to global", "squash", map[string]string{"owner/repo": "nope"}, "owner/repo", "squash"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := Defaults{MergeMethod: tc.global, MergeMethodRepos: tc.perRepo}
			if got := d.ResolveMergeMethod(tc.repo); got != tc.want {
				t.Errorf("ResolveMergeMethod(%q) = %q, want %q", tc.repo, got, tc.want)
			}
		})
	}
}

func TestValidMergeMethod(t *testing.T) {
	for _, ok := range []string{"squash", "merge", "rebase", "Squash", "REBASE"} {
		if !ValidMergeMethod(ok) {
			t.Errorf("ValidMergeMethod(%q) = false, want true", ok)
		}
	}
	for _, bad := range []string{"", "fast-forward", "ff", "sqush"} {
		if ValidMergeMethod(bad) {
			t.Errorf("ValidMergeMethod(%q) = true, want false", bad)
		}
	}
}

func TestResolveMergedWindow(t *testing.T) {
	cases := []struct {
		name    string
		def     string
		section string
		want    time.Duration
	}{
		{"off by default", "", "", 0},
		{"global default", "1h", "", time.Hour},
		{"section overrides default", "1h", "30m", 30 * time.Minute},
		{"section set, no default", "", "45m", 45 * time.Minute},
		{`"0" is off`, "0", "", 0},
		{"unparseable is off", "nope", "", 0},
		{"unparseable section is off", "1h", "nope", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := Defaults{ShowMergedFor: tc.def}
			if got := d.ResolveMergedWindow(tc.section); got != tc.want {
				t.Errorf("ResolveMergedWindow(def=%q, section=%q) = %v, want %v",
					tc.def, tc.section, got, tc.want)
			}
		})
	}
}
