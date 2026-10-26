package app

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func cmdDoctor(args []string) int {
	if _, err := parseFlags(args, map[string]bool{}); err != nil {
		return fail(err)
	}
	type row struct {
		name     string
		ok       bool
		required bool
		detail   string
	}
	var rows []row
	add := func(name string, ok, required bool, detail string) {
		rows = append(rows, row{name, ok, required, detail})
	}

	for _, tool := range []string{"git", "herdr", "claude"} {
		path, err := exec.LookPath(tool)
		if err != nil {
			add(tool, false, true, "not found in PATH")
		} else {
			add(tool, true, true, path)
		}
	}
	herdrVersion := ""
	if _, err := exec.LookPath("herdr"); err == nil {
		res, _ := runCmd([]string{"herdr", "--version"}, "", 10*time.Second, nil)
		herdrVersion = strings.TrimSpace(res.Stdout + res.Stderr)
	}
	add("herdr runs", herdrVersion != "", true, orDefault(herdrVersion, "herdr --version failed"))

	integration := ""
	if _, err := exec.LookPath("herdr"); err == nil {
		res, _ := runCmd([]string{"herdr", "integration", "status"}, "", 15*time.Second, nil)
		integration = res.Stdout + res.Stderr
	}
	okIntegration := strings.Contains(integration, "claude: current")
	detail := "run: herdr integration install claude"
	if okIntegration {
		detail = "claude: current"
	}
	add("claude integration", okIntegration, true, detail)

	skill := filepath.Join(skillDir(), "weefleet-coordinator", "SKILL.md")
	add("coordinator skill", fileExists(skill), false, orDefault(installedDetail(skill), "missing "+skill+"; run: weefleet install"))
	weefleetPath, _ := exec.LookPath("weefleet")
	add("weefleet on PATH", weefleetPath != "", false, orDefault(weefleetPath, "not on PATH; add the install bin dir to PATH"))
	commandPath := filepath.Join(commandDir(), "weefleet.md")
	add("slash command /weefleet", fileExists(commandPath), false, orDefault(installedDetail(commandPath), "missing "+commandPath+"; run: weefleet install"))
	launcher, _ := exec.LookPath("weefleet-claude")
	add("weefleet-claude launcher", launcher != "", false, orDefault(launcher, "not on PATH; run: weefleet install"))
	inHerdr := os.Getenv("HERDR_ENV") == "1"
	add("inside Herdr pane", inHerdr, false, map[bool]string{true: "yes", false: "dispatch/wait must run inside Herdr"}[inHerdr])

	failedRequired, warnings := false, false
	for _, r := range rows {
		mark := "ok"
		if !r.ok {
			if r.required {
				mark = "!!"
			} else {
				mark = "--"
			}
		}
		fmt.Printf("[%s] %s: %s\n", mark, r.name, r.detail)
		if !r.ok && r.required {
			failedRequired = true
		} else if !r.ok {
			warnings = true
		}
	}
	if failedRequired {
		fmt.Println("doctor: required checks failed")
		return 1
	}
	suffix := ""
	if warnings {
		suffix = " (with warnings)"
	}
	fmt.Printf("doctor: ready%s\n", suffix)
	return 0
}

func installedDetail(path string) string {
	if fileExists(path) {
		return "installed"
	}
	return ""
}

func orDefault(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

func cmdInit(args []string) int {
	fs, err := parseFlags(args, map[string]bool{
		"repo": true, "default-branch": true,
	})
	if err != nil {
		return fail(err)
	}
	if fs.has("help") {
		fmt.Println("usage: weefleet init [--repo PATH] [--default-branch NAME] [--no-claude-md]")
		return 0
	}
	writeClaudeMDFlag := !fs.bool("no-claude-md")
	raw := fs.value("repo")
	if raw == "" {
		raw, _ = os.Getwd()
	}
	start, err := absPath(raw)
	if err != nil {
		return fail(err)
	}
	root, err := gitRepoRoot(start)
	if err != nil {
		return fail(err)
	}
	key := repoKeyFor(root)
	branch := fs.value("default-branch")
	if branch == "" {
		branch = gitDefaultBranch(root)
	}
	dir := repoDir(key)
	if err := os.MkdirAll(filepath.Join(dir, "tasks"), 0o755); err != nil {
		return fail(err)
	}
	if err := ensureConfig(); err != nil {
		return fail(err)
	}
	var existing RepoMeta
	_, _ = readJSONOr(filepath.Join(dir, "repo.json"), &existing)
	createdAt := existing.CreatedAt
	if createdAt == "" {
		createdAt = nowISO()
	}
	if err := writeJSON(filepath.Join(dir, "repo.json"), RepoMeta{
		RepoRoot: root, RepoKey: key, DefaultBranch: branch,
		CreatedAt: createdAt, UpdatedAt: nowISO(),
	}); err != nil {
		return fail(err)
	}
	for _, name := range []string{"ledger.jsonl", "lock"} {
		path := filepath.Join(dir, name)
		if !fileExists(path) {
			if err := os.WriteFile(path, nil, 0o644); err != nil {
				return fail(err)
			}
		}
	}
	fmt.Printf("repo: %s\nkey:  %s\nhome: %s\nbranch: %s\n", root, key, dir, branch)
	if writeClaudeMDFlag {
		if err := writeClaudeMD(root); err != nil {
			fmt.Printf("claude.md: warning: %v\n", err)
		} else {
			fmt.Printf("claude.md: weefleet block present in %s\n", filepath.Join(root, "CLAUDE.md"))
		}
	} else {
		fmt.Println("hint: CLAUDE.md was not updated (--no-claude-md)")
	}
	fmt.Println("next:")
	fmt.Println("  1. write a checks file (see ~/.weefleet/templates/checks.json)")
	fmt.Printf("  2. weefleet new --repo %s --goal '...' --checks /path/to/checks.json --scope 'src/**'\n", root)
	fmt.Println("  3. weefleet approve T-YYYYMMDD-NN   (after the human approves)")
	fmt.Println("  4. weefleet dispatch T-YYYYMMDD-NN")
	return 0
}

func cmdNew(args []string) int {
	fs, err := parseFlags(args, map[string]bool{
		"repo": true, "id": true, "goal": true, "goal-file": true,
		"brief": true, "checks": true, "scope": true, "by": true, "note": true,
	})
	if err != nil {
		return fail(err)
	}
	repoRoot, key, _, err := resolveRepo(fs.value("repo"))
	if err != nil {
		return fail(err)
	}
	if fs.value("checks") == "" {
		return fail(errorf("provide --checks (checks.json)"))
	}
	var taskID, tdir string
	err = withRepoLock(key, func() error {
		taskID = fs.value("id")
		if taskID == "" {
			id, err := nextTaskID(key)
			if err != nil {
				return err
			}
			taskID = id
		}
		if err := validateTaskID(taskID); err != nil {
			return err
		}
		tdir = taskDir(key, taskID)
		if fileExists(tdir) {
			return errorf("task already exists: %s", taskID)
		}
		scope := fs.values("scope")
		if len(scope) == 0 {
			scope = []string{"**"}
		}
		var briefText, goal string
		if brief := fs.value("brief"); brief != "" {
			data, err := os.ReadFile(expandHome(brief))
			if err != nil {
				return err
			}
			briefText = string(data)
		} else {
			goal = fs.value("goal")
			if goalFile := fs.value("goal-file"); goalFile != "" {
				data, err := os.ReadFile(expandHome(goalFile))
				if err != nil {
					return err
				}
				goal = string(data)
			}
			if strings.TrimSpace(goal) == "" {
				return errorf("provide --goal, --goal-file, or --brief")
			}
			rawChecks, _, err := readChecks(expandHome(fs.value("checks")))
			if err != nil {
				return err
			}
			briefText = renderBrief(taskID, goal, scope, checkIDs(rawChecks), filepath.Join(tdir, "report.json"))
		}
		checks, _, err := readChecks(expandHome(fs.value("checks")))
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Join(tdir, "evidence"), 0o755); err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Join(tdir, "logs"), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(tdir, "brief.md"), []byte(briefText), 0o644); err != nil {
			return err
		}
		if err := writeJSON(filepath.Join(tdir, "checks.json"), checks); err != nil {
			return err
		}
		checksSHA, err := sha256File(filepath.Join(tdir, "checks.json"))
		if err != nil {
			return err
		}
		if err := appendEvent(key, taskID, "created", map[string]any{
			"goal":          strings.TrimSpace(goal),
			"scope":         scope,
			"brief_path":    filepath.Join(tdir, "brief.md"),
			"checks_path":   filepath.Join(tdir, "checks.json"),
			"checks_sha256": checksSHA,
			"repo_root":     repoRoot,
		}); err != nil {
			return err
		}
		if fs.bool("approve") {
			if err := appendEvent(key, taskID, "contract_approved", map[string]any{
				"by": orDefault(fs.value("by"), "human"), "note": fs.value("note"),
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return fail(err)
	}
	fmt.Printf("task: %s\ndir:  %s\n", taskID, tdir)
	if fs.bool("approve") {
		fmt.Println("contract: approved")
	} else {
		fmt.Println("contract: draft (dispatch will refuse until approved)")
	}
	return 0
}

func cmdApprove(args []string) int {
	fs, err := parseFlags(args, map[string]bool{"repo": true, "by": true, "note": true})
	if err != nil {
		return fail(err)
	}
	taskID, err := fs.positional(0, "task")
	if err != nil {
		return fail(err)
	}
	repoRoot, key, _, err := resolveRepo(fs.value("repo"))
	if err != nil {
		return fail(err)
	}
	by := orDefault(fs.value("by"), orDefault(gitConfig(repoRoot, "user.name"), "human"))
	err = withRepoLock(key, func() error {
		task, err := foldTask(key, taskID)
		if err != nil {
			return err
		}
		if task.State != StateDraft {
			return errorf("task %s is %s, expected draft", taskID, task.State)
		}
		if task.ContractApproved {
			fmt.Printf("contract already approved: %s\n", taskID)
			return nil
		}
		return appendEvent(key, taskID, "contract_approved", map[string]any{"by": by, "note": fs.value("note")})
	})
	if err != nil {
		return fail(err)
	}
	fmt.Printf("contract approved: %s by %s\n", taskID, by)
	fmt.Printf("next: weefleet dispatch %s\n", taskID)
	return 0
}

func cmdDispatch(args []string) int {
	fs, err := parseFlags(args, map[string]bool{"repo": true, "timeout": true})
	if err != nil {
		return fail(err)
	}
	taskID, err := fs.positional(0, "task")
	if err != nil {
		return fail(err)
	}
	repoRoot, key, meta, err := resolveRepo(fs.value("repo"))
	if err != nil {
		return fail(err)
	}
	workspace, err := requireHerdr()
	if err != nil {
		return fail(err)
	}
	cfg := loadConfig()
	var wt, agent, paneID string
	err = withRepoLock(key, func() error {
		task, err := foldTask(key, taskID)
		if err != nil {
			return err
		}
		if task.State != StateDraft {
			return errorf("task %s is %s, expected draft; use retry first", taskID, task.State)
		}
		if !task.ContractApproved {
			return errorf("contract for %s is not approved; run: weefleet approve %s --by <human> --note \"<human words>\"", taskID, taskID)
		}
		tdir, err := taskDirFor(task)
		if err != nil {
			return err
		}
		checks, _, err := loadChecks(task)
		if err != nil {
			return err
		}
		baseSHA := task.BaseSHA
		if baseSHA == "" {
			baseSHA, err = gitRevParse(repoRoot, meta.DefaultBranch)
			if err != nil {
				return err
			}
		}
		branch := task.Branch
		if branch == "" {
			branch = "task/" + taskID
		}
		wt = task.Worktree
		if wt == "" {
			wt = worktreePath(key, taskID)
		}
		attempt := task.Attempt
		if attempt < 1 {
			attempt = 1
		}
		if !fileExists(wt) {
			if err := gitWorktreeAdd(repoRoot, wt, branch, baseSHA); err != nil {
				_ = appendEvent(key, taskID, "dispatch_failed", map[string]any{"reason": "worktree: " + err.Error()})
				return err
			}
		} else if !gitWorktreeExists(wt) {
			return errorf("path exists but is not a git worktree: %s", wt)
		}
		agent, err = agentName(taskID, attempt)
		if err != nil {
			return err
		}
		if live := herdrAgent(agent); live != nil {
			return errorf("agent name %s is already live in pane %s; close it or run weefleet retry to create a new attempt",
				agent, asString(live["pane_id"]))
		}
		var tabID string
		kind := cfg.AgentKind
		tabID, paneID, err = herdrTabCreate(workspace, wt, taskID)
		if err != nil {
			_ = appendEvent(key, taskID, "dispatch_failed", map[string]any{
				"reason": "herdr: " + err.Error(), "worktree": wt,
			})
			return err
		}
		extraDirs := []string{}
		if kind == "claude" {
			extraDirs = []string{tdir}
		}
		if err := herdrAgentStart(agent, paneID, kind, cfg.ReadyTimeoutMS, extraDirs); err != nil {
			_ = appendEvent(key, taskID, "dispatch_failed", map[string]any{
				"reason": "herdr: " + err.Error(), "worktree": wt, "tab_id": tabID, "pane_id": paneID,
			})
			return err
		}
		readyTimeout := float64(cfg.ReadyTimeoutMS) / 1000.0
		if readyTimeout < 30 {
			readyTimeout = 30
		}
		if err := herdrAgentWait(agent, readyTimeout, "idle"); err != nil {
			_ = appendEvent(key, taskID, "dispatch_failed", map[string]any{
				"reason": "herdr: " + err.Error(), "worktree": wt, "tab_id": tabID, "pane_id": paneID,
			})
			return err
		}
		live := herdrAgent(agent)
		var completionSeq any
		if live != nil {
			completionSeq = live["completion_seq"]
		}
		checkList := strings.Join(checkIDs(checks), ",")
		reportPath := filepath.Join(tdir, "report.json")
		prompt := fmt.Sprintf(
			"Read %s and do the task. Read %s too. Work only in %s. "+
				"Do not push. Do not touch main. "+
				"Commit non-interactively with git -m; the final commit message must end with these trailers.\n"+
				"Use Status: done when finished; if you cannot finish, use Status: blocked or Status: failed instead.\n"+
				"Task: %s\nAttempt: %d\nChecks: %s\nReport: report.json\nBase: %s\nStatus: done\n"+
				"Write %s; put evidence in %s. Your final line must be: REPORT: %s",
			filepath.Join(tdir, "brief.md"), filepath.Join(tdir, "checks.json"), wt,
			taskID, attempt, checkList, baseSHA,
			reportPath, filepath.Join(tdir, "evidence"), reportPath,
		)
		if err := appendEvent(key, taskID, "dispatched", map[string]any{
			"worktree": wt, "pane_id": paneID, "tab_id": tabID, "agent": agent,
			"attempt": attempt, "base_sha": baseSHA, "branch": branch, "workspace_id": workspace,
		}); err != nil {
			return err
		}
		if err := herdrAgentPrompt(agent, prompt, false, 0); err != nil {
			_ = appendEvent(key, taskID, "dispatch_failed", map[string]any{"reason": "prompt: " + err.Error()})
			return err
		}
		return appendEvent(key, taskID, "prompt_sent", map[string]any{"completion_seq": completionSeq})
	})
	if err != nil {
		return fail(err)
	}
	fmt.Printf("task:  %s\nagent: %s\npane:  %s\nwt:    %s\n", taskID, agent, paneID, wt)
	if fs.bool("wait") {
		waitArgs := []string{taskID}
		if repo := fs.value("repo"); repo != "" {
			waitArgs = append([]string{"--repo", repo}, waitArgs...)
		}
		if to := fs.value("timeout"); to != "" {
			waitArgs = append(waitArgs, "--timeout", to)
		}
		return cmdWait(waitArgs)
	}
	return 0
}

func classifySettled(key, taskID, agent string) (string, error) {
	live := herdrAgent(agent)
	if live == nil {
		_ = appendEvent(key, taskID, "lost", map[string]any{"agent": agent})
		return StateLost, nil
	}
	status := asString(live["agent_status"])
	switch status {
	case "blocked":
		_ = appendEvent(key, taskID, "blocked", map[string]any{"agent": agent})
		return StateBlocked, nil
	case "idle", "done":
		tdir := taskDir(key, taskID)
		reportPath := filepath.Join(tdir, "report.json")
		if fileExists(reportPath) {
			var report Report
			_, _ = readJSONOr(reportPath, &report)
			_ = appendEvent(key, taskID, "reported", map[string]any{"report_path": reportPath, "head_sha": report.HeadSHA})
			return StateReported, nil
		}
		_ = appendEvent(key, taskID, "uncertain", map[string]any{"reason": "agent settled without report"})
		return StateUncertain, nil
	case "working":
		return StateRunning, nil
	}
	_ = appendEvent(key, taskID, "uncertain", map[string]any{"reason": "agent status: " + status})
	return StateUncertain, nil
}

func cmdWait(args []string) int {
	fs, err := parseFlags(args, map[string]bool{"repo": true, "timeout": true, "until": true})
	if err != nil {
		return fail(err)
	}
	taskID, err := fs.positional(0, "task")
	if err != nil {
		return fail(err)
	}
	_, key, _, err := resolveRepo(fs.value("repo"))
	if err != nil {
		return fail(err)
	}
	if _, err := requireHerdr(); err != nil {
		return fail(err)
	}
	cfg := loadConfig()
	timeoutS := cfg.WaitTimeoutS
	if v := fs.value("timeout"); v != "" {
		var f float64
		if _, err := fmt.Sscanf(v, "%f", &f); err == nil && f > 0 {
			timeoutS = f
		}
	}
	until := ""
	if us := fs.values("until"); len(us) > 0 {
		until = us[0]
	}

	// Do not hold the repo lock while herdr waits: that can be tens of
	// minutes, and it would block accept/verify/resolve on an unrelated task.
	var agent string
	err = withRepoLock(key, func() error {
		task, err := foldTask(key, taskID)
		if err != nil {
			return err
		}
		if task.Agent == "" {
			return errorf("task %s has no dispatched agent; run weefleet dispatch first", taskID)
		}
		agent = task.Agent
		return nil
	})
	if err != nil {
		return fail(err)
	}
	if err := herdrAgentWait(agent, timeoutS, until); err != nil {
		_ = withRepoLock(key, func() error {
			return appendEvent(key, taskID, "uncertain", map[string]any{"reason": "wait: " + err.Error()})
		})
		fmt.Printf("state: uncertain (%v)\n", err)
		fmt.Println("do not resend the prompt; inspect the pane first")
		return 0
	}
	var state string
	err = withRepoLock(key, func() error {
		task, err := foldTask(key, taskID)
		if err != nil {
			return err
		}
		if task.Agent != agent {
			return errorf("task %s changed agent while waiting (%s -> %s); run weefleet status", taskID, agent, task.Agent)
		}
		state, err = classifySettled(key, taskID, agent)
		return err
	})
	if err != nil {
		return fail(err)
	}
	fmt.Printf("state: %s\n", state)
	if state == StateBlocked {
		task, _ := foldTask(key, taskID)
		fmt.Printf("needs you: pane %s (agent %s)\n", task.PaneID, task.Agent)
		fmt.Printf("record the decision with: weefleet resolve %s --decision '...' [--send]\n", taskID)
	}
	return 0
}

func cmdStatus(args []string) int {
	fs, err := parseFlags(args, map[string]bool{"repo": true})
	if err != nil {
		return fail(err)
	}
	repoRoot, key, _, err := resolveRepo(fs.value("repo"))
	if err != nil {
		return fail(err)
	}
	_ = repoRoot
	ids, err := listTaskIDs(key)
	if err != nil {
		return fail(err)
	}
	live := herdrAgents()
	rows := make([]StatusRow, 0)
	for _, id := range ids {
		task, err := foldTask(key, id)
		if err != nil {
			return fail(err)
		}
		liveStatus := ""
		if task.Agent != "" {
			if a := live[task.Agent]; a != nil {
				liveStatus = asString(a["agent_status"])
			}
		}
		note := statusNote(task, liveStatus)
		created := parseISOTime(orDefault(task.CreatedAt, orDefault(task.UpdatedAt, nowISO())))
		minutes := int(time.Since(created).Minutes())
		row := StatusRow{
			Task: task.ID, State: task.State,
			Agent: orDefault(task.Agent, "-"), Pane: orDefault(task.PaneID, "-"),
			Minutes: minutes, Note: note, Worktree: task.Worktree,
		}
		if task.ChecksPath != "" {
			row.TaskDir = filepath.Dir(task.ChecksPath)
		}
		rows = append(rows, row)
	}
	if fs.bool("json") {
		data, _ := json.MarshalIndent(rows, "", "  ")
		fmt.Println(string(data))
		return 0
	}
	if len(rows) == 0 {
		fmt.Println("no tasks")
		return 0
	}
	fmt.Printf("%-22s %-14s %-24s %-9s %4s  NOTE\n", "TASK", "STATE", "AGENT", "PANE", "MIN")
	for _, row := range rows {
		fmt.Printf("%-22s %-14s %-24s %-9s %4d  %s\n", row.Task, row.State, row.Agent, row.Pane, row.Minutes, row.Note)
	}
	return 0
}

func statusNote(task Task, liveStatus string) string {
	switch {
	case task.State == StateRunning && liveStatus == "blocked":
		return "needs you: " + task.PaneID
	case task.State == StateRunning && liveStatus != "":
		return liveStatus
	case task.State == StateBlocked:
		return "needs you: " + task.PaneID
	case task.State == StateDraft && task.ContractApproved:
		return "ready to dispatch"
	case task.State == StateDraft:
		return "contract pending"
	case task.State == StateReported:
		return "ready to verify"
	case task.State == StateVerified:
		return "ready to accept"
	case task.State == StateAccepted:
		return "ready to record/merge"
	case task.State == StateRejected || task.State == StateClosed:
		return ""
	case task.State == StateBudgetExceeded:
		return "budget exceeded; needs human"
	case task.State == StateUncertain || task.State == StateLost || task.State == StateFailed:
		return orDefault(task.FailureReason, task.State)
	}
	return ""
}

// installPaths keeps the install locations in one place so doctor and install
// always agree.
func binDir() string {
	if v := os.Getenv("WEEFLEET_BIN_DIR"); v != "" {
		return expandHome(v)
	}
	return filepath.Join(homeDir(), ".local", "bin")
}

func skillDir() string {
	if v := os.Getenv("WEEFLEET_SKILL_DIR"); v != "" {
		return expandHome(v)
	}
	return filepath.Join(homeDir(), ".claude", "skills")
}

func commandDir() string {
	if v := os.Getenv("WEEFLEET_COMMAND_DIR"); v != "" {
		return expandHome(v)
	}
	return filepath.Join(homeDir(), ".claude", "commands")
}

func templateDir() string {
	return filepath.Join(weefleetHome(), "templates")
}
