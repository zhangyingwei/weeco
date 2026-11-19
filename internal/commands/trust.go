package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"weeco/internal/sysutil"
)

// Claude Code records workspace trust per directory in ~/.claude.json. An
// interactive session in a directory without that record blocks on the trust
// dialog before it is ready, so a member started by Herdr never reports ready.
// The setting is the documented manual workaround: accept the dialog once, or
// set projects[<path>].hasTrustDialogAccepted to true.

const claudeTrustKey = "hasTrustDialogAccepted"

const claudeConfigName = ".claude.json"

func claudeConfigPath() (string, error) {
	if dir := strings.TrimSpace(os.Getenv("CLAUDE_CONFIG_DIR")); dir != "" {
		return filepath.Join(dir, claudeConfigName), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, claudeConfigName), nil
}

// claudeTrustDirs returns every path Claude Code may use for the trust record.
// Inside a repository trust is recorded at the repository root; a worktree
// resolves to the main checkout root. Outside a repository the launch
// directory itself is used.
func claudeTrustDirs(dir string) ([]string, error) {
	abs, err := sysutil.AbsPath(dir)
	if err != nil {
		return nil, err
	}
	if home, err := os.UserHomeDir(); err == nil {
		if resolved, err := sysutil.AbsPath(home); err == nil && resolved == abs {
			return nil, fmt.Errorf("Claude Code does not persist workspace trust for the home directory; run the team from a project directory instead of %s", abs)
		}
	}
	dirs := []string{abs}
	res, err := sysutil.Run([]string{"git", "-C", abs, "rev-parse", "--path-format=absolute", "--git-common-dir"}, "", 5*time.Second, nil)
	if err == nil && res.ExitCode == 0 && !res.TimedOut {
		common := strings.TrimSpace(res.Stdout)
		if common != "" {
			if commonAbs, err := sysutil.AbsPath(common); err == nil && filepath.Base(commonAbs) == ".git" {
				if root := filepath.Dir(commonAbs); root != abs {
					dirs = append(dirs, root)
				}
			}
		}
	}
	return uniquePaths(dirs), nil
}

func uniquePaths(paths []string) []string {
	seen := make(map[string]bool, len(paths))
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		out = append(out, path)
	}
	return out
}

// acceptClaudeWorkspaceTrust marks the member directory, and the repository
// root Claude records trust under, as trusted. Existing content is preserved.
func acceptClaudeWorkspaceTrust(dir string) error {
	dirs, err := claudeTrustDirs(dir)
	if err != nil {
		return err
	}
	path, err := claudeConfigPath()
	if err != nil {
		return err
	}

	data, readErr := os.ReadFile(path)
	existed := true
	switch {
	case os.IsNotExist(readErr):
		existed = false
		data = []byte("{}")
	case readErr != nil:
		return fmt.Errorf("read Claude config %s: %w", path, readErr)
	case len(strings.TrimSpace(string(data))) == 0:
		data = []byte("{}")
	}

	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil {
		return fmt.Errorf("Claude config %s is not valid JSON; refusing to overwrite it: %v", path, err)
	}
	if root == nil {
		root = map[string]json.RawMessage{}
	}
	projects := map[string]json.RawMessage{}
	if raw, ok := root["projects"]; ok && len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &projects); err != nil {
			return fmt.Errorf("Claude config %s has an invalid projects object; refusing to overwrite it: %v", path, err)
		}
		if projects == nil {
			projects = map[string]json.RawMessage{}
		}
	}

	changed := false
	for _, trustedDir := range dirs {
		entry := map[string]json.RawMessage{}
		if raw, ok := projects[trustedDir]; ok && len(raw) > 0 && string(raw) != "null" {
			if err := json.Unmarshal(raw, &entry); err != nil {
				return fmt.Errorf("Claude config %s has an invalid entry for %q; refusing to overwrite it: %v", path, trustedDir, err)
			}
			if entry == nil {
				entry = map[string]json.RawMessage{}
			}
		}
		if string(entry[claudeTrustKey]) == "true" {
			continue
		}
		entry[claudeTrustKey] = json.RawMessage("true")
		encoded, err := json.Marshal(entry)
		if err != nil {
			return err
		}
		projects[trustedDir] = encoded
		changed = true
	}
	if !changed && existed {
		return nil
	}

	encodedProjects, err := json.Marshal(projects)
	if err != nil {
		return err
	}
	root["projects"] = encodedProjects
	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return err
	}
	out = append(out, '\n')

	mode := os.FileMode(0o600)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	if err := sysutil.AtomicWrite(path, out); err != nil {
		return fmt.Errorf("write Claude config %s: %w", path, err)
	}
	_ = os.Chmod(path, mode)
	return nil
}
