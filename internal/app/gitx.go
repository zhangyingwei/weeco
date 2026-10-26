package app

import (
	"os"
	"path/filepath"
	"strings"
)

func git(repo string, args ...string) (runResult, error) {
	cmd := append([]string{"git", "-C", repo}, args...)
	return runCmd(cmd, "", 0, nil)
}

func gitChecked(repo string, args ...string) (runResult, error) {
	cmd := append([]string{"git", "-C", repo}, args...)
	return runChecked(cmd, "", 0)
}

func gitRepoRoot(start string) (string, error) {
	res, err := runCmd([]string{"git", "-C", start, "rev-parse", "--show-toplevel"}, "", 0, nil)
	if err != nil {
		return "", err
	}
	if res.ExitCode != 0 {
		return "", errorf("not a git repository: %s", start)
	}
	out := strings.TrimSpace(res.Stdout)
	abs, err := filepath.Abs(out)
	if err != nil {
		return "", err
	}
	// Use the canonical path so a repo reached through a symlink always maps
	// to the same repo key.
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	}
	return abs, nil
}

func gitCurrentBranch(repo string) string {
	res, err := git(repo, "symbolic-ref", "--short", "HEAD")
	if err != nil || res.ExitCode != 0 {
		return ""
	}
	return strings.TrimSpace(res.Stdout)
}

func gitDefaultBranch(repo string) string {
	res, err := git(repo, "symbolic-ref", "--quiet", "refs/remotes/origin/HEAD")
	if err == nil && res.ExitCode == 0 && strings.TrimSpace(res.Stdout) != "" {
		out := strings.TrimSpace(res.Stdout)
		if idx := strings.LastIndex(out, "/"); idx >= 0 {
			return out[idx+1:]
		}
		return out
	}
	for _, name := range []string{"main", "master"} {
		if gitBranchExists(repo, name) {
			return name
		}
	}
	if cur := gitCurrentBranch(repo); cur != "" {
		return cur
	}
	return "main"
}

func gitRevParse(repo, ref string) (string, error) {
	res, err := gitChecked(repo, "rev-parse", ref)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(res.Stdout), nil
}

func gitHead(repo string) (string, error) {
	return gitRevParse(repo, "HEAD")
}

func gitIsClean(repo string) (bool, error) {
	res, err := gitChecked(repo, "status", "--porcelain")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(res.Stdout) == "", nil
}

func gitChangedFiles(repo, base, head string) ([]string, error) {
	res, err := gitChecked(repo, "diff", "--name-only", base+".."+head)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, line := range strings.Split(res.Stdout, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			files = append(files, line)
		}
	}
	return files, nil
}

func gitDiffStat(repo, base, head string) (string, error) {
	res, err := gitChecked(repo, "diff", "--stat", base+".."+head)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(res.Stdout), nil
}

// gitTrailers reads trailers from the commit message. `git interpret-trailers`
// reads the message from stdin, not from a commit ref.
func gitTrailers(repo, ref string) map[string]string {
	msg, err := git(repo, "log", "-1", "--format=%B", ref)
	if err != nil || msg.ExitCode != 0 {
		return map[string]string{}
	}
	res, err := runCmdWithInput([]string{"git", "-C", repo, "interpret-trailers", "--parse"}, msg.Stdout)
	if err != nil || res.ExitCode != 0 {
		return map[string]string{}
	}
	trailers := map[string]string{}
	for _, line := range strings.Split(res.Stdout, "\n") {
		idx := strings.Index(line, ":")
		if idx < 0 {
			continue
		}
		trailers[strings.TrimSpace(line[:idx])] = strings.TrimSpace(line[idx+1:])
	}
	return trailers
}

func gitWorktreeExists(path string) bool {
	res, err := runCmd([]string{"git", "-C", path, "rev-parse", "--is-inside-work-tree"}, "", 0, nil)
	return err == nil && res.ExitCode == 0 && strings.TrimSpace(res.Stdout) == "true"
}

func gitBranchExists(repo, branch string) bool {
	res, err := git(repo, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch)
	return err == nil && res.ExitCode == 0
}

func gitWorktreeAdd(repo, path, branch, baseSHA string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if gitBranchExists(repo, branch) {
		_, err := gitChecked(repo, "worktree", "add", path, branch)
		return err
	}
	_, err := gitChecked(repo, "worktree", "add", "-b", branch, path, baseSHA)
	return err
}

func gitWorktreeRemove(repo, path string, force bool) error {
	args := []string{"worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	args = append(args, path)
	_, err := gitChecked(repo, args...)
	return err
}

func gitBranchDelete(repo, branch string) error {
	_, err := gitChecked(repo, "branch", "-D", branch)
	return err
}

// gitNote writes an audit note. Notes are a convenience, never a requirement.
func gitNote(repo, ref, message string) {
	_, _ = git(repo, "notes", "--ref="+ref, "add", "-f", "-m", message, "HEAD")
}

func gitConfig(repo, key string) string {
	res, err := git(repo, "config", key)
	if err != nil || res.ExitCode != 0 {
		return ""
	}
	return strings.TrimSpace(res.Stdout)
}
