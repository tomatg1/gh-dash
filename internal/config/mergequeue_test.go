package config

import "testing"

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
