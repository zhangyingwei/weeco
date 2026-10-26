package app

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var (
	weefleetBin string
	herdrBin    string
)

func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "weefleet-test-bin-*")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer os.RemoveAll(tmp)
	weefleetBin = filepath.Join(tmp, "weefleet")
	herdrBin = filepath.Join(tmp, "herdr")
	build := func(out, pkg string) error {
		cmd := exec.Command("go", "build", "-o", out, pkg)
		cmd.Dir = filepath.Join("..", "..")
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		return cmd.Run()
	}
	if err := build(weefleetBin, "./cmd/weefleet"); err != nil {
		os.Exit(1)
	}
	if err := build(herdrBin, "./internal/app/testdata/fakeherdr"); err != nil {
		os.Exit(1)
	}
	os.Exit(m.Run())
}

type testEnv struct {
	t     *testing.T
	root  string
	home  string
	repo  string
	bin   string
	state string
	env   []string
}

type cliResult struct {
	stdout string
	stderr string
	code   int
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	root := t.TempDir()
	e := &testEnv{
		t:     t,
		root:  root,
		home:  filepath.Join(root, "home"),
		repo:  filepath.Join(root, "repo"),
		bin:   filepath.Join(root, "bin"),
		state: filepath.Join(root, "fake-herdr.json"),
	}
	for _, dir := range []string{e.home, e.repo, e.bin} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// The fake herdr and the weefleet binary share one PATH entry; copying the
	// fake keeps each test's PATH self-contained.
	data, err := os.ReadFile(herdrBin)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(e.bin, "herdr"), data, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(e.home, "config.json"),
		[]byte(fmt.Sprintf(`{"worktree_root":%q}`, filepath.Join(root, "worktrees"))), 0o644); err != nil {
		t.Fatal(err)
	}
	e.env = append(os.Environ(),
		"WEEFLEET_HOME="+e.home,
		"HERDR_ENV=1",
		"HERDR_WORKSPACE_ID=w1",
		"FAKE_HERDR_STATE="+e.state,
		"PATH="+e.bin+string(os.PathListSeparator)+os.Getenv("PATH"),
	)
	e.git("init", "-b", "main")
	e.git("config", "user.email", "test@example.com")
	e.git("config", "user.name", "Test User")
	if err := os.MkdirAll(filepath.Join(e.repo, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(e.repo, "src", "app.py"), []byte("x = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	e.git("add", "-A")
	e.git("commit", "-m", "init")
	return e
}

func (e *testEnv) git(args ...string) string {
	return e.gitIn(e.repo, args...)
}

func (e *testEnv) gitIn(dir string, args ...string) string {
	e.t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		e.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func (e *testEnv) runCLI(args ...string) cliResult {
	return e.runCLIExpect(args, true)
}

func (e *testEnv) runCLIExpect(args []string, expectOK bool) cliResult {
	e.t.Helper()
	cmd := exec.Command(weefleetBin, args...)
	cmd.Dir = e.repo
	cmd.Env = e.env
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	res := cliResult{stdout: stdout.String(), stderr: stderr.String()}
	if exitErr, ok := err.(*exec.ExitError); ok {
		res.code = exitErr.ExitCode()
	} else if err != nil {
		e.t.Fatalf("run weefleet %v: %v", args, err)
	}
	if expectOK && res.code != 0 {
		e.t.Fatalf("weefleet %v failed (%d)\nSTDOUT:\n%s\nSTDERR:\n%s", args, res.code, res.stdout, res.stderr)
	}
	return res
}

func (e *testEnv) prepareTask(budget map[string]any, checkCmd string) (string, string) {
	e.t.Helper()
	goal := filepath.Join(e.root, "goal.md")
	checks := filepath.Join(e.root, "checks.json")
	if err := os.WriteFile(goal, []byte("change app behavior\n"), 0o644); err != nil {
		e.t.Fatal(err)
	}
	if checkCmd == "" {
		checkCmd = "echo ok"
	}
	if budget == nil {
		budget = map[string]any{
			"max_total_seconds":  300,
			"max_runs_per_check": 2,
			"max_output_lines":   80,
		}
	}
	checksJSON, _ := json.Marshal(map[string]any{
		"checks": []map[string]any{{
			"id": "A1", "cmd": checkCmd, "expect_exit": 0, "timeout_s": 30,
		}},
		"budget": budget,
	})
	if err := os.WriteFile(checks, checksJSON, 0o644); err != nil {
		e.t.Fatal(err)
	}
	e.runCLI("init", "--repo", e.repo)
	e.runCLI("new", "--repo", e.repo,
		"--goal-file", goal, "--checks", checks, "--scope", "src/**",
		"--approve", "--id", "T-20261026-01")
	e.runCLI("dispatch", "T-20261026-01")
	taskDirs, _ := filepath.Glob(filepath.Join(e.home, "repos", "*", "tasks", "T-20261026-01"))
	worktrees, _ := filepath.Glob(filepath.Join(e.root, "worktrees", "*", "T-20261026-01"))
	if len(taskDirs) != 1 || len(worktrees) != 1 {
		e.t.Fatalf("unexpected task/worktree dirs: %v %v", taskDirs, worktrees)
	}
	return taskDirs[0], worktrees[0]
}

func (e *testEnv) commitWorker(worktree, taskDir string) (string, string) {
	e.t.Helper()
	base := strings.TrimSpace(e.git("rev-parse", "main"))
	if err := os.WriteFile(filepath.Join(worktree, "src", "app.py"), []byte("x = 2\n"), 0o644); err != nil {
		e.t.Fatal(err)
	}
	e.gitIn(worktree, "add", "-A")
	message := "fix: change app\n\n" +
		"Task: T-20261026-01\n" +
		"Attempt: 1\n" +
		"Checks: A1\n" +
		"Report: report.json\n" +
		"Base: " + base + "\n" +
		"Status: done"
	e.gitIn(worktree, "commit", "-m", message)
	head := strings.TrimSpace(e.gitIn(worktree, "rev-parse", "HEAD"))
	if err := os.WriteFile(filepath.Join(taskDir, "evidence", "A1.log"), []byte("ok\n"), 0o644); err != nil {
		e.t.Fatal(err)
	}
	report := map[string]any{
		"task_id": "T-20261026-01", "status": "done", "summary": "changed app",
		"base_sha": base, "head_sha": head,
		"changed_files": []string{"src/app.py"},
		"checks": []map[string]any{{
			"id": "A1", "cmd": "echo ok", "exit": 0, "log": "evidence/A1.log",
		}},
		"evidence": []string{"evidence/A1.log"}, "open_risks": []string{},
		"needs_decision": nil,
	}
	data, _ := json.Marshal(report)
	if err := os.WriteFile(filepath.Join(taskDir, "report.json"), data, 0o644); err != nil {
		e.t.Fatal(err)
	}
	return base, head
}

func (e *testEnv) statusState() string {
	e.t.Helper()
	var rows []StatusRow
	if err := json.Unmarshal([]byte(e.runCLI("status", "--json").stdout), &rows); err != nil {
		e.t.Fatal(err)
	}
	if len(rows) == 0 {
		return ""
	}
	return rows[0].State
}

func (e *testEnv) ledgerEvents(taskDir string) []Event {
	e.t.Helper()
	ledger := filepath.Join(filepath.Dir(filepath.Dir(taskDir)), "ledger.jsonl")
	data, err := os.ReadFile(ledger)
	if err != nil {
		e.t.Fatal(err)
	}
	var events []Event
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var ev Event
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			e.t.Fatal(err)
		}
		events = append(events, ev)
	}
	return events
}

func countEvents(events []Event, name string) int {
	n := 0
	for _, ev := range events {
		if ev.Event == name {
			n++
		}
	}
	return n
}

func TestEndToEnd(t *testing.T) {
	e := newTestEnv(t)
	taskDir, worktree := e.prepareTask(nil, "")
	if got := e.statusState(); got != "running" {
		t.Fatalf("state = %q, want running", got)
	}
	_, head := e.commitWorker(worktree, taskDir)

	if out := e.runCLI("wait", "T-20261026-01").stdout; !strings.Contains(out, "state: reported") {
		t.Fatalf("wait output: %s", out)
	}
	if got := e.statusState(); got != "reported" {
		t.Fatalf("state = %q, want reported", got)
	}
	out := e.runCLI("verify", "T-20261026-01").stdout
	if !strings.Contains(out, "verify: PASS") {
		t.Fatalf("verify output: %s", out)
	}
	var verify VerifyResult
	data, _ := os.ReadFile(filepath.Join(taskDir, "verify.json"))
	if err := json.Unmarshal(data, &verify); err != nil {
		t.Fatal(err)
	}
	if !verify.Passed || verify.HeadSHA != head {
		t.Fatalf("verify.json = %+v", verify)
	}
	if n := countEvents(e.ledgerEvents(taskDir), "check_run"); n != 1 {
		t.Fatalf("check_run events = %d, want 1", n)
	}
	if out := e.runCLI("verify", "T-20261026-01").stdout; !strings.Contains(out, "cached") {
		t.Fatalf("second verify should be cached: %s", out)
	}
	if n := countEvents(e.ledgerEvents(taskDir), "check_run"); n != 1 {
		t.Fatalf("cached verify re-ran checks: %d", n)
	}
	if got := e.statusState(); got != "verified" {
		t.Fatalf("state = %q, want verified", got)
	}
	e.runCLI("accept", "T-20261026-01", "--yes", "--by", "tester")
	if got := e.statusState(); got != "accepted" {
		t.Fatalf("state = %q, want accepted", got)
	}
	e.runCLI("record", "T-20261026-01", "--merge-commit", head)
	record := filepath.Join(e.repo, "docs", "weefleet", "tasks", "T-20261026-01.md")
	if !fileExists(record) {
		t.Fatal("record file was not written")
	}
	if log := e.git("log", "-1", "--format=%B"); !strings.Contains(log, "Task: T-20261026-01") {
		t.Fatalf("record commit message: %s", log)
	}
	e.runCLI("close", "T-20261026-01", "--delete-branch")
	if got := e.statusState(); got != "closed" {
		t.Fatalf("state = %q, want closed", got)
	}
	if fileExists(worktree) {
		t.Fatal("worktree should be removed")
	}
	if !fileExists(taskDir) || !fileExists(record) {
		t.Fatal("task dir and record must be kept")
	}
	e.runCLI("gc", "--older-than", "0", "--delete")
	if fileExists(taskDir) {
		t.Fatal("gc should remove the task dir")
	}
	if !fileExists(record) {
		t.Fatal("gc must keep the repo record")
	}
}

func TestBudgetExceeded(t *testing.T) {
	e := newTestEnv(t)
	taskDir, worktree := e.prepareTask(map[string]any{
		"max_total_seconds": 0, "max_runs_per_check": 2, "max_output_lines": 80,
	}, "")
	e.commitWorker(worktree, taskDir)
	out := e.runCLI("verify", "T-20261026-01").stdout
	if !strings.Contains(out, "BUDGET EXCEEDED") {
		t.Fatalf("verify output: %s", out)
	}
	if n := countEvents(e.ledgerEvents(taskDir), "check_run"); n != 0 {
		t.Fatalf("checks ran despite the budget: %d", n)
	}
}

func TestInitDetectsMainOnFeatureBranch(t *testing.T) {
	e := newTestEnv(t)
	e.git("checkout", "-b", "feature")
	e.runCLI("init", "--repo", e.repo)
	files, _ := filepath.Glob(filepath.Join(e.home, "repos", "*", "repo.json"))
	if len(files) != 1 {
		t.Fatalf("repo.json not found: %v", files)
	}
	var meta RepoMeta
	data, _ := os.ReadFile(files[0])
	if err := json.Unmarshal(data, &meta); err != nil {
		t.Fatal(err)
	}
	if meta.DefaultBranch != "main" {
		t.Fatalf("default branch = %q, want main", meta.DefaultBranch)
	}
}

func TestInitWritesClaudeMDOnce(t *testing.T) {
	e := newTestEnv(t)
	e.runCLI("init", "--repo", e.repo, "--write-claude-md")
	path := filepath.Join(e.repo, "CLAUDE.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "weefleet-coordinator") || !strings.Contains(text, "<!-- weefleet:begin -->") {
		t.Fatalf("claude.md content: %s", text)
	}
	e.runCLI("init", "--repo", e.repo, "--write-claude-md")
	again, _ := os.ReadFile(path)
	if string(again) != text {
		t.Fatal("init appended the block twice")
	}
}

func TestInitMigratesOldClaudeBlock(t *testing.T) {
	e := newTestEnv(t)
	path := filepath.Join(e.repo, "CLAUDE.md")
	old := "before\n\n<!-- fleet:begin -->\nold block\n<!-- fleet:end -->\nafter\n"
	if err := os.WriteFile(path, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	e.runCLI("init", "--repo", e.repo)
	data, _ := os.ReadFile(path)
	text := string(data)
	if !strings.Contains(text, "<!-- weefleet:begin -->") || strings.Contains(text, "<!-- fleet:begin -->") {
		t.Fatalf("migration failed: %s", text)
	}
	if !strings.Contains(text, "before") || !strings.Contains(text, "after") {
		t.Fatalf("surrounding content lost: %s", text)
	}
}

func TestContractGate(t *testing.T) {
	e := newTestEnv(t)
	goal := filepath.Join(e.root, "goal.md")
	checks := filepath.Join(e.root, "checks.json")
	_ = os.WriteFile(goal, []byte("change app behavior\n"), 0o644)
	_ = os.WriteFile(checks, []byte(`{"checks":[{"id":"A1","cmd":"echo ok","expect_exit":0,"timeout_s":30}]}`), 0o644)
	e.runCLI("init", "--repo", e.repo)
	e.runCLI("new", "--repo", e.repo, "--goal-file", goal, "--checks", checks, "--id", "T-20261026-03")
	res := e.runCLIExpect([]string{"dispatch", "T-20261026-03"}, false)
	if res.code != 1 || !strings.Contains(res.stderr, "not approved") {
		t.Fatalf("dispatch should be refused: code=%d stderr=%s", res.code, res.stderr)
	}
	e.runCLI("approve", "T-20261026-03", "--by", "tester", "--note", "looks good, go ahead")
	var rows []StatusRow
	if err := json.Unmarshal([]byte(e.runCLI("status", "--json").stdout), &rows); err != nil {
		t.Fatal(err)
	}
	if rows[0].Note != "ready to dispatch" {
		t.Fatalf("note = %q", rows[0].Note)
	}
}

func TestFailedThenRetry(t *testing.T) {
	e := newTestEnv(t)
	taskDir, worktree := e.prepareTask(nil, "exit 1")
	e.commitWorker(worktree, taskDir)
	out := e.runCLI("verify", "T-20261026-01").stdout
	if !strings.Contains(out, "verify: FAIL") {
		t.Fatalf("verify output: %s", out)
	}
	if got := e.statusState(); got != "failed" {
		t.Fatalf("state = %q, want failed", got)
	}
	e.runCLI("retry", "T-20261026-01", "--reason", "fix the failing check")
	if got := e.statusState(); got != "draft" {
		t.Fatalf("state = %q, want draft", got)
	}
	var rows []StatusRow
	if err := json.Unmarshal([]byte(e.runCLI("status", "--json").stdout), &rows); err != nil {
		t.Fatal(err)
	}
	if rows[0].Note != "ready to dispatch" {
		t.Fatalf("note = %q", rows[0].Note)
	}
}

func TestReject(t *testing.T) {
	e := newTestEnv(t)
	taskDir, worktree := e.prepareTask(nil, "")
	e.commitWorker(worktree, taskDir)
	e.runCLI("verify", "T-20261026-01")
	if got := e.statusState(); got != "verified" {
		t.Fatalf("state = %q, want verified", got)
	}
	e.runCLI("reject", "T-20261026-01", "--reason", "not the behavior we want")
	if got := e.statusState(); got != "rejected" {
		t.Fatalf("state = %q, want rejected", got)
	}
	e.runCLI("close", "T-20261026-01")
	if fileExists(worktree) {
		t.Fatal("worktree should be removed")
	}
	if !fileExists(taskDir) {
		t.Fatal("task dir should be kept")
	}
}

func TestChecksTamperingRejected(t *testing.T) {
	e := newTestEnv(t)
	taskDir, worktree := e.prepareTask(nil, "")
	e.commitWorker(worktree, taskDir)
	var checks map[string]any
	data, _ := os.ReadFile(filepath.Join(taskDir, "checks.json"))
	_ = json.Unmarshal(data, &checks)
	checks["checks"].([]any)[0].(map[string]any)["cmd"] = "true"
	tampered, _ := json.Marshal(checks)
	_ = os.WriteFile(filepath.Join(taskDir, "checks.json"), tampered, 0o644)
	out := e.runCLI("verify", "T-20261026-01").stdout
	if !strings.Contains(out, "checks.json changed") {
		t.Fatalf("verify output: %s", out)
	}
	if got := e.statusState(); got != "failed" {
		t.Fatalf("state = %q, want failed", got)
	}
}

func TestMissingTrailersRejected(t *testing.T) {
	e := newTestEnv(t)
	taskDir, worktree := e.prepareTask(nil, "")
	base := strings.TrimSpace(e.git("rev-parse", "main"))
	_ = os.WriteFile(filepath.Join(worktree, "src", "app.py"), []byte("x = 3\n"), 0o644)
	e.gitIn(worktree, "add", "-A")
	e.gitIn(worktree, "commit", "-m", "fix without trailers")
	head := strings.TrimSpace(e.gitIn(worktree, "rev-parse", "HEAD"))
	report := map[string]any{
		"task_id": "T-20261026-01", "status": "done", "summary": "no trailers",
		"base_sha": base, "head_sha": head,
		"changed_files": []string{"src/app.py"},
		"checks":        []map[string]any{}, "evidence": []string{},
	}
	data, _ := json.Marshal(report)
	_ = os.WriteFile(filepath.Join(taskDir, "report.json"), data, 0o644)
	out := e.runCLI("verify", "T-20261026-01").stdout
	if !strings.Contains(out, "verify: FAILED") {
		t.Fatalf("verify output: %s", out)
	}
	var verify VerifyResult
	vdata, _ := os.ReadFile(filepath.Join(taskDir, "verify.json"))
	if err := json.Unmarshal(vdata, &verify); err != nil {
		t.Fatal(err)
	}
	if verify.Passed || !strings.Contains(verify.Reason, "trailer Task") {
		t.Fatalf("verify.json = %+v", verify)
	}
}

func TestInstallWritesAssetsIdempotently(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	bin := filepath.Join(root, "bin")
	skills := filepath.Join(root, "skills")
	commands := filepath.Join(root, "commands")
	env := append(os.Environ(),
		"WEEFLEET_HOME="+home,
		"WEEFLEET_BIN_DIR="+bin,
		"WEEFLEET_SKILL_DIR="+skills,
		"WEEFLEET_COMMAND_DIR="+commands,
		"PATH="+filepath.Dir(weefleetBin)+string(os.PathListSeparator)+os.Getenv("PATH"),
	)
	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command(weefleetBin, args...)
		cmd.Dir = root
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("weefleet %v: %v\n%s", args, err, out)
		}
		return string(out)
	}
	out := run("install")
	for _, path := range []string{
		filepath.Join(bin, "weefleet"),
		filepath.Join(skills, "weefleet-coordinator", "SKILL.md"),
		filepath.Join(commands, "weefleet.md"),
		filepath.Join(home, "templates", "brief.md"),
		filepath.Join(home, "templates", "checks.json"),
		filepath.Join(home, "templates", "commit.txt"),
		filepath.Join(home, "installed.json"),
	} {
		if !fileExists(path) {
			t.Fatalf("install did not write %s\n%s", path, out)
		}
	}
	link, err := os.Readlink(filepath.Join(bin, "weefleet-claude"))
	if err != nil || link != "weefleet" {
		t.Fatalf("launcher symlink = %q, %v", link, err)
	}
	skillPath := filepath.Join(skills, "weefleet-coordinator", "SKILL.md")
	skill, _ := os.ReadFile(skillPath)
	if !strings.Contains(string(skill), "name: weefleet-coordinator") {
		t.Fatalf("skill content: %s", skill)
	}
	if out := run("install"); !strings.Contains(out, "skill: unchanged") {
		t.Fatalf("second install should be idempotent:\n%s", out)
	}
	if err := os.WriteFile(skillPath, []byte("custom\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out := run("install"); !strings.Contains(out, "skipped") {
		t.Fatalf("install should not overwrite a foreign skill:\n%s", out)
	}
	if data, _ := os.ReadFile(skillPath); string(data) != "custom\n" {
		t.Fatal("install overwrote a foreign skill without --force")
	}
	run("install", "--force")
	if data, _ := os.ReadFile(skillPath); !strings.Contains(string(data), "name: weefleet-coordinator") {
		t.Fatal("--force did not restore the embedded skill")
	}
}

func TestInstalledPathsAndParseFlags(t *testing.T) {
	fs, err := parseFlags([]string{"--scope", "src/**", "T-1", "--yes", "--scope=test/**"}, map[string]bool{"scope": true})
	if err != nil {
		t.Fatal(err)
	}
	taskID, err := fs.positional(0, "task")
	if err != nil {
		t.Fatal(err)
	}
	if taskID != "T-1" {
		t.Errorf("positionals = %v", fs.positionals)
	}
	if got := fs.values("scope"); len(got) != 2 || got[0] != "src/**" || got[1] != "test/**" {
		t.Errorf("scopes = %v", got)
	}
	if !fs.bool("yes") {
		t.Error("--yes should be true")
	}
}
