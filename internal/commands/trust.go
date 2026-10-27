package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"weeco/internal/store"
	"weeco/internal/sysutil"
)

// claudeConfigPath is ~/.claude.json by default. WEECO_CLAUDE_CONFIG exists so
// tests can point at an isolated config instead of the user's real one.
func claudeConfigPath() string {
	if p := os.Getenv("WEECO_CLAUDE_CONFIG"); p != "" {
		return sysutil.ExpandHome(p)
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ".claude.json"
	}
	return filepath.Join(home, ".claude.json")
}

func cmdTrust(args []string) int {
	fs, err := parseFlags(args, map[string]bool{"project": true})
	if err != nil {
		return fail(err)
	}
	if helpRequested(fs) {
		fmt.Println("usage: weeco trust [PATH] [--project PATH]")
		fmt.Println("records PATH (default: current project or cwd) as trusted by Claude Code")
		fmt.Println("Claude stores the grant in ~/.claude.json; worktrees under the same repository inherit it")
		return 0
	}
	target := strings.TrimSpace(fs.value("project"))
	if target == "" && len(fs.positionals) > 0 {
		target = strings.TrimSpace(fs.positionals[0])
	}
	if target == "" {
		if project, err := store.FindProject(""); err == nil {
			target = project.Root
		} else if wd, err := os.Getwd(); err == nil {
			target = wd
		}
	}
	if target == "" {
		return fail(fmt.Errorf("cannot determine a path to trust"))
	}
	root, workspace, backup, err := setClaudeTrust(target)
	if err != nil {
		return fail(err)
	}
	fmt.Printf("trusted:   %s\n", root)
	if workspace != "" && workspace != root {
		fmt.Printf("workspace: %s\n", workspace)
	}
	if backup != "" {
		fmt.Printf("backup:    %s\n", backup)
	}
	fmt.Println("worktrees under this repository inherit the trust")
	return 0
}

// claudeTrust reports whether Claude Code has accepted the workspace trust
// dialog for path or its canonical git root. It returns the root used as the
// trust key even when the path is not trusted.
func claudeTrust(path string) (root string, trusted bool, err error) {
	keys, root, err := claudeTrustKeys(path)
	if err != nil {
		return "", false, err
	}
	top, err := readClaudeConfig()
	if os.IsNotExist(err) {
		return root, false, nil
	}
	if err != nil {
		return root, false, err
	}
	projects, err := claudeProjects(top)
	if err != nil {
		return root, false, err
	}
	for _, key := range keys {
		entry, err := claudeProjectEntry(projects, key)
		if err != nil {
			return root, false, err
		}
		var accepted bool
		if raw, ok := entry["hasTrustDialogAccepted"]; ok {
			if err := json.Unmarshal(raw, &accepted); err != nil {
				return root, false, fmt.Errorf("parse projects[%q].hasTrustDialogAccepted in %s: %w", key, claudeConfigPath(), err)
			}
		}
		if accepted {
			return root, true, nil
		}
	}
	return root, false, nil
}

// setClaudeTrust records the grant for the canonical git root and, when it is
// different, the workspace path. It returns the root, workspace and the path of
// the one-time backup.
func setClaudeTrust(path string) (root, workspace, backup string, err error) {
	keys, root, err := claudeTrustKeys(path)
	if err != nil {
		return "", "", "", err
	}
	workspace = keys[len(keys)-1]
	configPath := claudeConfigPath()
	if resolved, err := filepath.EvalSymlinks(configPath); err == nil {
		configPath = resolved
	}
	if err := os.MkdirAll(filepath.Dir(configPath), 0o700); err != nil {
		return "", "", "", err
	}
	unlock, err := lockFile(configPath + ".weeco.lock")
	if err != nil {
		return "", "", "", err
	}
	defer unlock()

	top, err := readClaudeConfig()
	if os.IsNotExist(err) {
		top = map[string]json.RawMessage{}
	} else if err != nil {
		return "", "", "", err
	}
	projects, err := claudeProjects(top)
	if err != nil {
		return "", "", "", err
	}
	for _, key := range keys {
		entry, err := claudeProjectEntry(projects, key)
		if err != nil {
			return "", "", "", err
		}
		entry["hasTrustDialogAccepted"] = json.RawMessage("true")
		data, err := json.Marshal(entry)
		if err != nil {
			return "", "", "", err
		}
		projects[key] = data
	}
	projectsData, err := json.Marshal(projects)
	if err != nil {
		return "", "", "", err
	}
	top["projects"] = projectsData
	data, err := json.Marshal(top)
	if err != nil {
		return "", "", "", err
	}
	if created, err := backupClaudeConfig(configPath); err != nil {
		return "", "", "", err
	} else {
		backup = created
	}
	mode := os.FileMode(0o600)
	if info, err := os.Stat(configPath); err == nil {
		mode = info.Mode().Perm()
	}
	if err := writeFileAtomic(configPath, data, mode); err != nil {
		return "", "", "", err
	}
	return root, workspace, backup, nil
}

func readClaudeConfig() (map[string]json.RawMessage, error) {
	data, err := os.ReadFile(claudeConfigPath())
	if err != nil {
		return nil, err
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil {
		return nil, fmt.Errorf("parse %s: %w", claudeConfigPath(), err)
	}
	if top == nil {
		top = map[string]json.RawMessage{}
	}
	return top, nil
}

func claudeProjects(top map[string]json.RawMessage) (map[string]json.RawMessage, error) {
	raw, ok := top["projects"]
	if !ok || len(raw) == 0 || string(raw) == "null" {
		return map[string]json.RawMessage{}, nil
	}
	var projects map[string]json.RawMessage
	if err := json.Unmarshal(raw, &projects); err != nil {
		return nil, fmt.Errorf("parse projects in %s: %w", claudeConfigPath(), err)
	}
	if projects == nil {
		projects = map[string]json.RawMessage{}
	}
	return projects, nil
}

func claudeProjectEntry(projects map[string]json.RawMessage, key string) (map[string]json.RawMessage, error) {
	raw, ok := projects[key]
	if !ok || len(raw) == 0 || string(raw) == "null" {
		return map[string]json.RawMessage{}, nil
	}
	var entry map[string]json.RawMessage
	if err := json.Unmarshal(raw, &entry); err != nil {
		return nil, fmt.Errorf("parse projects[%q] in %s: %w", key, claudeConfigPath(), err)
	}
	if entry == nil {
		entry = map[string]json.RawMessage{}
	}
	return entry, nil
}

// claudeTrustKeys returns the canonical git root first and the workspace path
// second when they differ. Claude Code keys the grant by the repository root,
// so linked worktrees agree with their main repository.
func claudeTrustKeys(path string) ([]string, string, error) {
	abs, err := sysutil.AbsPath(path)
	if err != nil {
		return nil, "", err
	}
	if info, err := os.Stat(abs); err == nil && !info.IsDir() {
		abs = filepath.Dir(abs)
	}
	root := gitCommonRoot(abs)
	keys := []string{root}
	if root != abs {
		keys = append(keys, abs)
	}
	return keys, root, nil
}

func gitCommonRoot(abs string) string {
	res, err := sysutil.Run([]string{"git", "-C", abs, "rev-parse", "--path-format=absolute", "--git-common-dir"}, "", 5*time.Second, nil)
	if err == nil && res.ExitCode == 0 {
		common := strings.TrimSpace(res.Stdout)
		if common != "" {
			if !filepath.IsAbs(common) {
				common = filepath.Join(abs, common)
			}
			root := filepath.Dir(filepath.Clean(common))
			if canonical, err := sysutil.AbsPath(root); err == nil {
				return canonical
			}
		}
	}
	res, err = sysutil.Run([]string{"git", "-C", abs, "rev-parse", "--show-toplevel"}, "", 5*time.Second, nil)
	if err == nil && res.ExitCode == 0 {
		if top := strings.TrimSpace(res.Stdout); top != "" {
			if canonical, err := sysutil.AbsPath(top); err == nil {
				return canonical
			}
		}
	}
	return abs
}

func backupClaudeConfig(path string) (string, error) {
	backup := path + ".weeco-backup"
	if _, err := os.Stat(backup); err == nil {
		return "", nil
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	mode := os.FileMode(0o600)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	if err := writeFileAtomic(backup, data, mode); err != nil {
		return "", err
	}
	return backup, nil
}

func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".weeco-write-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func lockFile(path string) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, err
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}, nil
}
