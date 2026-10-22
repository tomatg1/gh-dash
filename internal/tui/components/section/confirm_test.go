package section

import "testing"

// y/N confirmations must resolve on a single keystroke — no Enter — and a repeat
// of the key that opened the prompt counts as "yes" (press `m` to merge, `m`
// again to go through with it).
func TestDecideConfirmKey(t *testing.T) {
	cases := []struct {
		name    string
		action  string
		openKey string
		pressed string
		want    ConfirmDecision
	}{
		{"y accepts", "merge_squash", "m", "y", ConfirmAccept},
		{"Y accepts", "merge_squash", "m", "Y", ConfirmAccept},
		{"repeat of opening key accepts", "merge_squash", "m", "m", ConfirmAccept},
		{"n cancels", "merge_squash", "m", "n", ConfirmCancel},
		{"N cancels", "merge_squash", "m", "N", ConfirmCancel},
		{"esc cancels", "merge_squash", "m", "esc", ConfirmCancel},
		{"ctrl+c cancels", "merge_squash", "m", "ctrl+c", ConfirmCancel},
		{"unrelated key does nothing", "merge_squash", "m", "z", ConfirmPassthrough},
		{"enter is left to the existing path", "merge_squash", "m", "enter", ConfirmPassthrough},

		// A different action's opening key: `x` opened a close prompt, so `x`
		// confirms it and `m` does not.
		{"close confirmed by its own key", "close", "x", "x", ConfirmAccept},
		{"close not confirmed by merge key", "close", "x", "m", ConfirmPassthrough},

		// No opening key recorded (e.g. a mouse-driven prompt): y/n still work.
		{"no open key still accepts y", "close", "", "y", ConfirmAccept},
		{"no open key ignores empty match", "close", "", "", ConfirmPassthrough},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := &BaseModel{PromptConfirmationAction: tc.action, PromptConfirmationKey: tc.openKey}
			if got := m.DecideConfirmKey(tc.pressed); got != tc.want {
				t.Errorf("DecideConfirmKey(%q) with action=%q openKey=%q = %v, want %v",
					tc.pressed, tc.action, tc.openKey, got, tc.want)
			}
		})
	}
}

// Prompts that collect typed text must keep requiring Enter: there, a keystroke
// is a character, not an answer. Typing "y" in a branch name must not submit.
func TestDecideConfirmKey_TextEntryPromptsPassThrough(t *testing.T) {
	for _, action := range []string{"new", "create_pr", "merge_method"} {
		m := &BaseModel{PromptConfirmationAction: action, PromptConfirmationKey: "n"}
		for _, pressed := range []string{"y", "Y", "n", "N", "esc", "m"} {
			if got := m.DecideConfirmKey(pressed); got != ConfirmPassthrough {
				t.Errorf("action %q: key %q returned %v, want ConfirmPassthrough",
					action, pressed, got)
			}
		}
	}
}
