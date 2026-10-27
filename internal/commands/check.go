package commands

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"weeco/internal/model"
	"weeco/internal/store"
	"weeco/internal/sysutil"
)

type acceptanceFile struct {
	Commands         []acceptanceCommand `json:"commands"`
	Artifacts        []string            `json:"artifacts"`
	RequiredFiles    []string            `json:"required_files,omitempty"`
	RequireCleanTree bool                `json:"require_clean_tree,omitempty"`
	RecordGit        bool                `json:"record_git,omitempty"`
}

type acceptanceCommand struct {
	ID         string  `json:"id"`
	Cmd        string  `json:"cmd"`
	ExpectExit int     `json:"expect_exit"`
	TimeoutS   float64 `json:"timeout_s"`
}

type checkCommandResult struct {
	ID         string  `json:"id"`
	Cmd        string  `json:"cmd"`
	Exit       int     `json:"exit"`
	ExpectExit int     `json:"expect_exit"`
	Passed     bool    `json:"passed"`
	TimedOut   bool    `json:"timed_out,omitempty"`
	DurationS  float64 `json:"duration_s"`
	Tail       string  `json:"tail,omitempty"`
}

type checkArtifactResult struct {
	Path   string `json:"path"`
	Exists bool   `json:"exists"`
	SHA256 string `json:"sha256,omitempty"`
}

type checkGitResult struct {
	Commit string `json:"commit,omitempty"`
	Tree   string `json:"tree,omitempty"`
	Branch string `json:"branch,omitempty"`
	Status string `json:"status,omitempty"`
	Clean  bool   `json:"clean"`
}

type checkResult struct {
	TeamID    string                `json:"team_id"`
	Workspace string                `json:"workspace"`
	Passed    bool                  `json:"passed"`
	Git       *checkGitResult       `json:"git,omitempty"`
	Commands  []checkCommandResult  `json:"commands,omitempty"`
	Artifacts []checkArtifactResult `json:"artifacts,omitempty"`
	Files     []checkArtifactResult `json:"files,omitempty"`
	At        string                `json:"at"`
}

func cmdTeamCheck(args []string) int {
	fs, err := parseFlags(args, map[string]bool{"project": true})
	if err != nil {
		return fail(err)
	}
	if helpRequested(fs) {
		fmt.Println("usage: weeco team check T-...")
		return 0
	}
	teamID, err := fs.positional(0, "team")
	if err != nil {
		return fail(err)
	}
	project, team, err := resolveTeam(fs, teamID)
	if err != nil {
		return fail(err)
	}
	teamDir := store.TeamDir(project.Key, teamID)
	acceptancePath := filepath.Join(teamDir, "acceptance.json")
	var acceptance acceptanceFile
	ok, err := sysutil.ReadJSONOr(acceptancePath, &acceptance)
	if err != nil {
		return fail(err)
	}
	if !ok {
		fmt.Println("no acceptance.json; manual review only")
		return 0
	}
	if len(acceptance.Commands) == 0 && len(acceptance.Artifacts) == 0 &&
		len(acceptance.RequiredFiles) == 0 && !acceptance.RequireCleanTree && !acceptance.RecordGit {
		fmt.Println("acceptance.json has no checks; manual review only")
		return 0
	}
	workDir := team.Workspace
	if workDir == "" {
		workDir = project.Root
	}
	if !sysutil.FileExists(workDir) {
		return fail(fmt.Errorf("workspace does not exist: %s", workDir))
	}
	result := checkResult{TeamID: teamID, Workspace: workDir, Passed: true, At: sysutil.NowISO()}
	gitState, gitErr := readGitState(workDir)
	if gitErr == nil {
		result.Git = gitState
		if acceptance.RequireCleanTree && !gitState.Clean {
			result.Passed = false
		}
	} else if acceptance.RequireCleanTree || acceptance.RecordGit {
		return fail(fmt.Errorf("git evidence requested but workspace is not a readable git repo: %v", gitErr))
	}
	for i, check := range acceptance.Commands {
		id := strings.TrimSpace(check.ID)
		if id == "" {
			id = fmt.Sprintf("C%d", i+1)
		}
		if strings.TrimSpace(check.Cmd) == "" {
			return fail(fmt.Errorf("acceptance command %s has an empty cmd", id))
		}
		timeout := check.TimeoutS
		if timeout <= 0 {
			timeout = 300
		}
		started := time.Now()
		res, err := sysutil.Run([]string{"sh", "-c", check.Cmd}, workDir, time.Duration(timeout*float64(time.Second)), nil)
		if err != nil {
			return fail(err)
		}
		duration := time.Since(started).Seconds()
		output := res.Stdout + res.Stderr
		logPath := filepath.Join(teamDir, "logs", id+".log")
		if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
			return fail(err)
		}
		if err := os.WriteFile(logPath, []byte(output), 0o644); err != nil {
			return fail(err)
		}
		passed := !res.TimedOut && res.ExitCode == check.ExpectExit
		if !passed {
			result.Passed = false
		}
		result.Commands = append(result.Commands, checkCommandResult{
			ID: id, Cmd: check.Cmd, Exit: res.ExitCode, ExpectExit: check.ExpectExit,
			Passed: passed, TimedOut: res.TimedOut, DurationS: round2(duration), Tail: lastLines(output, 40),
		})
	}
	for _, raw := range acceptance.Artifacts {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if filepath.IsAbs(raw) {
			return fail(fmt.Errorf("artifact path must be relative: %s", raw))
		}
		path := filepath.Join(workDir, raw)
		artifact := checkArtifactResult{Path: raw}
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			digest, err := sysutil.SHA256File(path)
			if err != nil {
				return fail(err)
			}
			artifact.Exists = true
			artifact.SHA256 = digest
		} else {
			result.Passed = false
		}
		result.Artifacts = append(result.Artifacts, artifact)
	}
	for _, raw := range acceptance.RequiredFiles {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if filepath.IsAbs(raw) {
			return fail(fmt.Errorf("required file must be relative: %s", raw))
		}
		path := filepath.Join(workDir, raw)
		file := checkArtifactResult{Path: raw}
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			digest, err := sysutil.SHA256File(path)
			if err != nil {
				return fail(err)
			}
			file.Exists = true
			file.SHA256 = digest
		} else {
			result.Passed = false
		}
		result.Files = append(result.Files, file)
	}
	checkPath := filepath.Join(teamDir, "check.json")
	if err := sysutil.WriteJSON(checkPath, result); err != nil {
		return fail(err)
	}
	if err := store.WithLock(project.Key, func() error {
		return store.Append(project.Key, "team", teamID, "team_checked", "boss", map[string]any{
			"passed": result.Passed, "check_path": checkPath, "workspace": workDir,
		})
	}); err != nil {
		return fail(err)
	}
	fmt.Println(checkSummary(result))
	fmt.Printf("check: %s\n", checkPath)
	return 0
}

func ensureChecksPassed(project model.Project, team model.Team, force bool) error {
	if force {
		return nil
	}
	teamDir := store.TeamDir(project.Key, team.ID)
	var acceptance acceptanceFile
	ok, err := sysutil.ReadJSONOr(filepath.Join(teamDir, "acceptance.json"), &acceptance)
	if err != nil {
		return err
	}
	if !ok || (len(acceptance.Commands) == 0 && len(acceptance.Artifacts) == 0 &&
		len(acceptance.RequiredFiles) == 0 && !acceptance.RequireCleanTree && !acceptance.RecordGit) {
		return nil
	}
	checkPath := filepath.Join(teamDir, "check.json")
	var result checkResult
	ok, err = sysutil.ReadJSONOr(checkPath, &result)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("acceptance.json exists but check.json is missing; run: weeco team check %s", team.ID)
	}
	if !result.Passed {
		return fmt.Errorf("team check failed; rework the team or use --force to override")
	}
	if acceptance.RequireCleanTree && (result.Git == nil || !result.Git.Clean) {
		return fmt.Errorf("acceptance requires a clean git tree; rerun: weeco team check %s", team.ID)
	}
	if acceptance.RecordGit && result.Git == nil {
		return fmt.Errorf("acceptance requires git evidence; rerun: weeco team check %s", team.ID)
	}
	if countNonEmpty(acceptance.RequiredFiles) != len(result.Files) {
		return fmt.Errorf("acceptance requires file evidence; rerun: weeco team check %s", team.ID)
	}
	return nil
}

func countNonEmpty(values []string) int {
	n := 0
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			n++
		}
	}
	return n
}

func readGitState(dir string) (*checkGitResult, error) {
	run := func(args ...string) (string, error) {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		out, err := cmd.CombinedOutput()
		return strings.TrimSpace(string(out)), err
	}
	inside, err := run("rev-parse", "--is-inside-work-tree")
	if err != nil || inside != "true" {
		return nil, fmt.Errorf("not a git worktree")
	}
	commit, err := run("rev-parse", "HEAD")
	if err != nil {
		return nil, fmt.Errorf("git HEAD: %w", err)
	}
	tree, err := run("rev-parse", "HEAD^{tree}")
	if err != nil {
		return nil, fmt.Errorf("git tree: %w", err)
	}
	branch, err := run("rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return nil, fmt.Errorf("git branch: %w", err)
	}
	status, err := run("status", "--porcelain")
	if err != nil {
		return nil, fmt.Errorf("git status: %w", err)
	}
	return &checkGitResult{
		Commit: commit,
		Tree:   tree,
		Branch: branch,
		Status: status,
		Clean:  status == "",
	}, nil
}

func checkSummary(result checkResult) string {
	label := "FAIL"
	if result.Passed {
		label = "PASS"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "check: %s (workspace %s)", label, result.Workspace)
	if result.Git != nil {
		clean := "dirty"
		if result.Git.Clean {
			clean = "clean"
		}
		fmt.Fprintf(&b, "\n  git: %s (%s) %s", shorten(result.Git.Commit), result.Git.Branch, clean)
	}
	for _, cmd := range result.Commands {
		mark := "FAIL"
		if cmd.Passed {
			mark = "PASS"
		}
		fmt.Fprintf(&b, "\n  %s: %s exit=%d %gs", cmd.ID, mark, cmd.Exit, cmd.DurationS)
		if !cmd.Passed && cmd.Tail != "" {
			for _, line := range strings.Split(cmd.Tail, "\n") {
				fmt.Fprintf(&b, "\n    %s", line)
			}
		}
	}
	for _, artifact := range result.Artifacts {
		mark := "missing"
		if artifact.Exists {
			mark = "ok"
		}
		fmt.Fprintf(&b, "\n  artifact %s: %s", artifact.Path, mark)
	}
	for _, file := range result.Files {
		mark := "missing"
		if file.Exists {
			mark = "ok"
		}
		fmt.Fprintf(&b, "\n  file %s: %s", file.Path, mark)
	}
	return b.String()
}

func shorten(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	return s
}

func lastLines(text string, n int) string {
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	if n > 0 && len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

func round2(v float64) float64 {
	return float64(int(v*100+0.5)) / 100
}
