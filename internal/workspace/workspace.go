// Package workspace creates the working directory for a requirement. Git
// worktrees are an optional adapter; a plain directory is equally valid.
package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"weeco/internal/sysutil"
)

type Result struct {
	Path    string
	Branch  string
	BaseRef string
	Managed bool
}

func IsGit(path string) bool {
	res, err := sysutil.Run([]string{"git", "-C", path, "rev-parse", "--is-inside-work-tree"}, "", 0, nil)
	return err == nil && res.ExitCode == 0 && strings.TrimSpace(res.Stdout) == "true"
}

func DefaultBranch(root string) string {
	res, err := sysutil.Run([]string{"git", "-C", root, "symbolic-ref", "--quiet", "--short", "HEAD"}, "", 0, nil)
	if err == nil && res.ExitCode == 0 {
		if branch := strings.TrimSpace(res.Stdout); branch != "" {
			return branch
		}
	}
	res, err = sysutil.Run([]string{"git", "-C", root, "symbolic-ref", "--quiet", "--short", "refs/remotes/origin/HEAD"}, "", 0, nil)
	if err == nil && res.ExitCode == 0 {
		out := strings.TrimSpace(res.Stdout)
		if idx := strings.LastIndex(out, "/"); idx >= 0 {
			return out[idx+1:]
		}
		if out != "" {
			return out
		}
	}
	for _, name := range []string{"main", "master"} {
		res, err := sysutil.Run([]string{"git", "-C", root, "rev-parse", "--verify", "--quiet", name}, "", 0, nil)
		if err == nil && res.ExitCode == 0 {
			return name
		}
	}
	return ""
}

func Create(projectRoot, target, reqID, base, explicitPath string) (Result, error) {
	if explicitPath != "" {
		path, err := sysutil.AbsPath(explicitPath)
		if err != nil {
			return Result{}, err
		}
		if err := os.MkdirAll(path, 0o755); err != nil {
			return Result{}, err
		}
		return Result{Path: path}, nil
	}
	if !IsGit(projectRoot) {
		if target == "" {
			target = projectRoot
		}
		if err := os.MkdirAll(target, 0o755); err != nil {
			return Result{}, err
		}
		return Result{Path: target}, nil
	}
	if base == "" {
		base = DefaultBranch(projectRoot)
	}
	if base == "" {
		return Result{}, fmt.Errorf("cannot determine the default branch; pass --base")
	}
	if target == "" {
		return Result{}, fmt.Errorf("git workspace target is empty")
	}
	branch := "req/" + reqID
	if info, err := os.Stat(target); err == nil {
		if !info.IsDir() {
			return Result{}, fmt.Errorf("workspace path exists and is not a directory: %s", target)
		}
		if !IsGit(target) {
			return Result{}, fmt.Errorf("workspace path exists but is not a git worktree: %s", target)
		}
		return Result{Path: target, Branch: branch, BaseRef: base, Managed: true}, nil
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return Result{}, err
	}
	exists, err := gitBranchExists(projectRoot, branch)
	if err != nil {
		return Result{}, err
	}
	var args []string
	if exists {
		args = []string{"git", "-C", projectRoot, "worktree", "add", target, branch}
	} else {
		args = []string{"git", "-C", projectRoot, "worktree", "add", "-b", branch, target, base}
	}
	if _, err := sysutil.RunChecked(args, "", 0); err != nil {
		return Result{}, err
	}
	return Result{Path: target, Branch: branch, BaseRef: base, Managed: true}, nil
}

func Remove(projectRoot, path string, managed, force bool) error {
	if !managed || path == "" {
		return nil
	}
	if !sysutil.FileExists(path) {
		_, _ = sysutil.Run([]string{"git", "-C", projectRoot, "worktree", "prune"}, "", 0, nil)
		return nil
	}
	args := []string{"git", "-C", projectRoot, "worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	args = append(args, path)
	if _, err := sysutil.RunChecked(args, "", 0); err != nil {
		return err
	}
	return nil
}

func Head(path string) string {
	res, err := sysutil.Run([]string{"git", "-C", path, "rev-parse", "HEAD"}, "", 0, nil)
	if err != nil || res.ExitCode != 0 {
		return ""
	}
	return strings.TrimSpace(res.Stdout)
}

func gitBranchExists(root, branch string) (bool, error) {
	res, err := sysutil.Run([]string{"git", "-C", root, "rev-parse", "--verify", "--quiet", "refs/heads/" + branch}, "", 0, nil)
	if err != nil {
		return false, err
	}
	return res.ExitCode == 0, nil
}
