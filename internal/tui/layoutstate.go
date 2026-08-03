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
	PreviewHeight map[string]int    `json:"previewHeight"`
	Selection     map[string]string `json:"selection,omitempty"`
	// MergedHidden records the `M` toggle per instance. It stores the HIDDEN
	// state rather than the visible one so the zero value (absent key) means
	// "show merged", which is the default whenever showMergedFor is configured.
	MergedHidden map[string]bool `json:"mergedHidden,omitempty"`
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
	// Explicit name wins: --instance flag (which sets GH_DASH_INSTANCE) or the
	// env var directly.
	if inst := os.Getenv("GH_DASH_INSTANCE"); inst != "" {
		return "instance:" + inst
	}

	// Implicit: the directory gh-dash was launched from, so two dashboards run
	// from different directories are automatically distinct instances without
	// naming them.
	if cwd, err := os.Getwd(); err == nil && cwd != "" {
		return "dir:" + cwd
	}

	// Last resort if the cwd can't be read.
	cfg := configFlag
	if cfg == "" {
		cfg = "global"
	}

	return cfg + "|" + repoPath
}

func emptyLayoutState() layoutState {
	return layoutState{
		PreviewHeight: map[string]int{},
		Selection:     map[string]string{},
		MergedHidden:  map[string]bool{},
	}
}

// loadMergedHidden reports whether this instance last had merged PRs hidden.
func loadMergedHidden(key string) bool {
	return readLayoutState().MergedHidden[key]
}

// saveMergedHidden persists the `M` toggle for this instance. Only the hidden
// state is stored; showing again deletes the key.
func saveMergedHidden(key string, hidden bool) error {
	if layoutStateDir() == "" || key == "" {
		return nil
	}

	st := readLayoutState()
	if hidden {
		st.MergedHidden[key] = true
	} else {
		delete(st.MergedHidden, key)
	}

	return writeLayoutState(st)
}

func readLayoutState() layoutState {
	st := emptyLayoutState()

	path := layoutStatePath()
	if path == "" {
		return st
	}

	b, err := os.ReadFile(path)
	if err != nil {
		return st // no state yet, or unreadable: fall back to defaults
	}
	if err := json.Unmarshal(b, &st); err != nil {
		return emptyLayoutState() // corrupt: start over
	}
	if st.PreviewHeight == nil {
		st.PreviewHeight = map[string]int{}
	}
	if st.Selection == nil {
		st.Selection = map[string]string{}
	}
	if st.MergedHidden == nil {
		st.MergedHidden = map[string]bool{}
	}

	return st
}

// writeLayoutState atomically persists st (temp file + rename).
func writeLayoutState(st layoutState) error {
	dir := layoutStateDir()
	if dir == "" {
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

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

// selectionKey scopes a remembered selection to an instance + section.
func selectionKey(instanceKey, section string) string {
	return instanceKey + "\x00" + section
}

// loadSelection returns the remembered selected-item URL for a section, or "".
func loadSelection(instanceKey, section string) string {
	return readLayoutState().Selection[selectionKey(instanceKey, section)]
}

// saveSelection persists the selected-item URL for a section, merging into the
// existing state. An empty url clears it.
func saveSelection(instanceKey, section, url string) error {
	if layoutStateDir() == "" {
		return nil
	}
	st := readLayoutState()
	k := selectionKey(instanceKey, section)
	if url == "" {
		delete(st.Selection, k)
	} else {
		st.Selection[k] = url
	}

	return writeLayoutState(st)
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
	if layoutStateDir() == "" || h <= 0 {
		return nil
	}

	st := readLayoutState()
	st.PreviewHeight[key] = h

	return writeLayoutState(st)
}
