package cmd

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
)

const (
	instanceConfigDir  = ".gh-dash"
	instanceConfigFile = "config.yml"
)

// resolveInstance turns the --instance flag (and, as a fallback, an implicit
// local config in the cwd) into the config path to load and a synthetic
// instance name used to scope persisted state (layout + selection).
//
//	--instance <file>   use that config file
//	--instance <dir>    use <dir>/.gh-dash/config.yml (created if missing)
//	--instance .        use $PWD/.gh-dash/config.yml (created if missing)
//	(no flag) + local   use $PWD/.gh-dash/config.yml if it already exists
//	(no flag)           fall through to configFlag / the global config
//
// The returned name is empty for the "no explicit instance" case, in which the
// TUI keys persisted state by the launch directory instead.
func resolveInstance(instanceFlag, configFlag string) (cfgPath, instanceName string) {
	if instanceFlag == "" {
		// Implicit: a local config in the cwd defines an instance.
		if local := localInstanceConfig(); fileExists(local) {
			return local, syntheticName(local)
		}
		return configFlag, "" // global / --config; state keyed by cwd
	}

	var target string
	switch {
	case instanceFlag == ".":
		target = localInstanceConfig()
	case isDir(instanceFlag):
		target = filepath.Join(instanceFlag, instanceConfigDir, instanceConfigFile)
	default:
		target = instanceFlag // a config file path
	}
	if abs, err := filepath.Abs(target); err == nil {
		target = abs
	}
	ensureInstanceConfig(target)

	return target, syntheticName(target)
}

func localInstanceConfig() string {
	pwd, err := os.Getwd()
	if err != nil {
		return ""
	}

	return filepath.Join(pwd, instanceConfigDir, instanceConfigFile)
}

// syntheticName is a short, stable, human-ish id for a config path: the project
// directory name that contains the .gh-dash/ folder, plus a hash of the
// absolute path so two same-named dirs never collide.
func syntheticName(cfgPath string) string {
	abs, err := filepath.Abs(cfgPath)
	if err != nil {
		abs = cfgPath
	}
	sum := sha256.Sum256([]byte(abs))
	// .../<project>/.gh-dash/config.yml -> <project>
	base := slug(filepath.Base(filepath.Dir(filepath.Dir(abs))))
	if base == "" {
		base = "inst"
	}

	return base + "-" + hex.EncodeToString(sum[:])[:8]
}

func ensureInstanceConfig(path string) {
	if fileExists(path) {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	_ = os.WriteFile(path, []byte(instanceConfigTemplate), 0o644)
}

const instanceConfigTemplate = `# yaml-language-server: $schema=https://gh-dash.dev/schema.json
# gh-dash instance config (created by --instance). This overlays your global
# config; add prSections here to give this instance its own tabs/filters. Its
# layout and selection are persisted separately from other instances.
prSections:
  - title: All
    filters: is:open author:@me sort:updated-desc
`

func fileExists(p string) bool {
	if p == "" {
		return false
	}
	info, err := os.Stat(p)

	return err == nil && !info.IsDir()
}

func isDir(p string) bool {
	info, err := os.Stat(p)

	return err == nil && info.IsDir()
}

func slug(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-':
			b.WriteRune(r)
		case r == ' ' || r == '_' || r == '.':
			b.WriteRune('-')
		}
	}

	return strings.Trim(b.String(), "-")
}
