package config

import "testing"

func TestEffectiveRefetchSeconds(t *testing.T) {
	cases := []struct {
		name       string
		mins, secs int
		want       int
	}{
		{"minutes only", 5, 0, 300},
		{"seconds override minutes", 5, 30, 30},
		{"seconds only", 0, 30, 30},
		{"both zero disables", 0, 0, 0},
		{"minute default", 30, 0, 1800},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := Defaults{RefetchIntervalMinutes: tc.mins, RefetchIntervalSeconds: tc.secs}
			if got := d.EffectiveRefetchSeconds(); got != tc.want {
				t.Errorf("EffectiveRefetchSeconds() = %d, want %d", got, tc.want)
			}
		})
	}
}
