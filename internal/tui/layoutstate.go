package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// layoutState is the sliver of UI layout remembered between runs. Everything
// here is best-effort: any failure degrades to "use the configured default",
// never to an error the user has to care about.
type layoutState struct {
	PreviewHeight map[string]int `json:"previewHeight"`
}

// layoutStateDir follows the XDG state spec -- state that should persist but
// isn't configuration, and that the user never edits by hand.
func layoutStateDir() string {
	if dir := os.Getenv("XDG_STATE_HOME"); dir != "" {
		return filepath.Join(dir, "gh-dash")
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	return filepath.Join(home, ".local", "state", "gh-dash")
}

func layoutStatePath() string {
	dir := layoutStateDir()
	if dir == "" {
		return ""
	}

	return filepath.Join(dir, "layout.json")
}

// layoutKey identifies "this dashboard" across runs.
//
// No per-window identity survives a restart: a tty gets recycled, a terminal
// session id is minted fresh. So the key is what actually defines the dashboard
// -- the config it loaded and the repo it was launched in. Export
// GH_DASH_INSTANCE to give a particular window its own remembered layout.
func layoutKey(configFlag, repoPath string) string {
	if inst := os.Getenv("GH_DASH_INSTANCE"); inst != "" {
		return "instance:" + inst
	}

	cfg := configFlag
	if cfg == "" {
		cfg = "global"
	}

	return cfg + "|" + repoPath
}

func readLayoutState() layoutState {
	st := layoutState{PreviewHeight: map[string]int{}}

	path := layoutStatePath()
	if path == "" {
		return st
	}

	b, err := os.ReadFile(path)
	if err != nil {
		return st // no state yet, or unreadable: fall back to defaults
	}
	if err := json.Unmarshal(b, &st); err != nil {
		return layoutState{PreviewHeight: map[string]int{}} // corrupt: start over
	}
	if st.PreviewHeight == nil {
		st.PreviewHeight = map[string]int{}
	}

	return st
}

// loadPreviewHeight returns the remembered preview height for key, or 0 for
// "nothing remembered, use the config".
func loadPreviewHeight(key string) int {
	return readLayoutState().PreviewHeight[key]
}

// savePreviewHeight persists h under key, merging into whatever other instances
// have stored. Written to a temp file and renamed, so a crash mid-write can
// never leave a half-parsed layout behind.
func savePreviewHeight(key string, h int) error {
	dir := layoutStateDir()
	if dir == "" || h <= 0 {
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	st := readLayoutState()
	st.PreviewHeight[key] = h

	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}

	tmp := filepath.Join(dir, "layout.json.tmp")
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}

	return os.Rename(tmp, layoutStatePath())
}
