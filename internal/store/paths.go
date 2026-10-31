package store

import (
	"os"
	"path/filepath"

	"weeco/internal/sysutil"
)

func Home() string {
	raw := os.Getenv("WEECO_HOME")
	if raw == "" {
		raw = "~/.weeco"
	}
	path := sysutil.ExpandHome(raw)
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	return filepath.Clean(path)
}

func BinDir() string {
	if path := os.Getenv("WEECO_BIN_DIR"); path != "" {
		return sysutil.ExpandHome(path)
	}
	return filepath.Join(sysutil.HomeDir(), ".local", "bin")
}

func SkillDir() string {
	if path := os.Getenv("WEECO_SKILL_DIR"); path != "" {
		return sysutil.ExpandHome(path)
	}
	return filepath.Join(sysutil.HomeDir(), ".claude", "skills")
}

// CommandDir locates the old slash command for manifest-owned upgrade cleanup.
func CommandDir() string {
	if path := os.Getenv("WEECO_COMMAND_DIR"); path != "" {
		return sysutil.ExpandHome(path)
	}
	return filepath.Join(sysutil.HomeDir(), ".claude", "commands")
}

func ManifestPath() string { return filepath.Join(Home(), "installed.json") }
