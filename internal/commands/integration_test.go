package commands

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"weeco/internal/model"
	"weeco/internal/store"
	"weeco/internal/sysutil"
)

type fakeHerdrState struct {
	Agents   map[string]map[string]any `json:"agents"`
	Panes    map[string]map[string]any `json:"panes"`
	Tabs     map[string]map[string]any `json:"tabs"`
	NextPane int                       `json:"next_pane,omitempty"`
}

func setupProject(t *testing.T, root string) (model.Project, string) {
	t.Helper()
	home := t.TempDir()
	bin := t.TempDir()
	statePath := filepath.Join(t.TempDir(), "herdr-state.json")
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "build", "-o", filepath.Join(bin, "herdr"), "./testdata/fakeherdr")
	cmd.Dir = wd
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build fakeherdr: %v\n%s", err, out)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("WEECO_HOME", home)
	t.Setenv("FAKE_HERDR_STATE", statePath)
	t.Setenv("HERDR_ENV", "1")
	t.Setenv("HERDR_WORKSPACE_ID", "w1")
	configPath := filepath.Join(t.TempDir(), "claude.json")
	t.Setenv("WEECO_CLAUDE_CONFIG", configPath)
	t.Cleanup(func() { _ = os.Chdir(wd) })
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	if code := Run([]string{"init"}); code != 0 {
		t.Fatalf("init failed with code %d", code)
	}
	project, err := store.FindProject(root)
	if err != nil {
		t.Fatalf("find project: %v", err)
	}
	return project, statePath
}

func TestCompanyLifecycle(t *testing.T) {
	root := t.TempDir()
	project, statePath := setupProject(t, root)
	claudeMD, err := os.ReadFile(filepath.Join(root, "CLAUDE.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(claudeMD), "你是老板") {
		t.Fatalf("CLAUDE.md does not contain the boss block:\n%s", claudeMD)
	}
	if code := Run([]string{"requirement", "new", "--goal", "build a widget"}); code != 0 {
		t.Fatalf("requirement new failed: %d", code)
	}
	reqIDs, err := store.ListRequirementIDs(project.Key)
	if err != nil || len(reqIDs) != 1 {
		t.Fatalf("requirement ids = %v, %v", reqIDs, err)
	}
	reqID := reqIDs[0]
	writeTestFile(t, filepath.Join(root, "contract.md"), "# Contract\n\nBuild a widget.\n")
	writeTestFile(t, filepath.Join(root, "acceptance.json"), `{"commands":[{"id":"A1","cmd":"true","expect_exit":0,"timeout_s":5}]}`)
	if code := Run([]string{"team", "recruit", "--requirement", reqID, "--purpose", "design", "--contract", "contract.md", "--acceptance", "acceptance.json", "--size", "2"}); code != 0 {
		t.Fatalf("team recruit failed: %d", code)
	}
	teamIDs, err := store.ListTeamIDs(project.Key)
	if err != nil || len(teamIDs) != 1 {
		t.Fatalf("team ids = %v, %v", teamIDs, err)
	}
	teamID := teamIDs[0]
	team, err := store.LoadTeam(project.Key, teamID)
	if err != nil {
		t.Fatal(err)
	}
	if team.State != model.TeamActive {
		t.Fatalf("team state = %s, want active", team.State)
	}
	if team.LeadAgent == "" || team.PaneID == "" || team.TabID == "" {
		t.Fatalf("team did not start: %+v", team)
	}
	if team.Workspace != project.Root {
		t.Fatalf("workspace = %s, want %s", team.Workspace, project.Root)
	}
	contract, err := os.ReadFile(team.ContractPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(contract), "Build a widget") {
		t.Fatalf("copied contract is wrong: %s", contract)
	}
	var st fakeHerdrState
	if err := sysutil.ReadJSON(statePath, &st); err != nil {
		t.Fatal(err)
	}
	agent := st.Agents[team.LeadAgent]
	if agent == nil {
		t.Fatalf("lead agent %s not started", team.LeadAgent)
	}
	native := fmt.Sprint(agent["native_args"])
	for _, want := range []string{"bypassPermissions", "--append-system-prompt-file"} {
		if !strings.Contains(native, want) {
			t.Fatalf("lead native args missing %q: %s", want, native)
		}
	}
	initialPrompt := fmt.Sprint(agent["last_prompt"])
	if !strings.Contains(initialPrompt, "formal name") {
		t.Fatalf("initial lead prompt does not require formal names: %s", initialPrompt)
	}
	teamDir := store.TeamDir(project.Key, teamID)
	if !sysutil.FileExists(filepath.Join(teamDir, "acceptance.json")) {
		t.Fatal("acceptance.json was not copied into the team directory")
	}
	promptFile := filepath.Join(teamDir, "lead-system-prompt.md")
	promptData, err := os.ReadFile(promptFile)
	if err != nil {
		t.Fatalf("read lead system prompt: %v", err)
	}
	if !strings.Contains(string(promptData), "你是 weeco 团队负责人") {
		t.Fatalf("lead system prompt is wrong:\n%s", promptData)
	}
	if !strings.Contains(string(promptData), "正式名字") {
		t.Fatalf("lead system prompt does not require formal names:\n%s", promptData)
	}
	t.Setenv("HERDR_PANE_ID", team.PaneID)
	if code := Run([]string{"team", "nudge", teamID}); code != 0 {
		t.Fatalf("team nudge failed: %d", code)
	}
	if code := Run([]string{"team", "check", teamID}); code != 0 {
		t.Fatalf("team check failed: %d", code)
	}
	var check checkResult
	if err := sysutil.ReadJSON(filepath.Join(teamDir, "check.json"), &check); err != nil {
		t.Fatal(err)
	}
	if !check.Passed {
		t.Fatalf("check did not pass: %+v", check)
	}
	writeTestFile(t, filepath.Join(teamDir, "result.md"), "# Result\n\n## 经验候选\n- 无新经验（原因）：test\n\nv1\n")
	if code := Run([]string{"lead", "submit"}); code != 0 {
		t.Fatalf("lead submit failed: %d", code)
	}
	team, _ = store.LoadTeam(project.Key, teamID)
	if team.State != model.TeamSubmitted {
		t.Fatalf("state after submit = %s, want submitted", team.State)
	}
	if code := Run([]string{"lead", "submit"}); code != 0 {
		t.Fatalf("repeat lead submit should be idempotent: %d", code)
	}
	if code := Run([]string{"team", "rework", teamID, "--issues", "add missing X"}); code != 0 {
		t.Fatalf("team rework failed: %d", code)
	}
	team, _ = store.LoadTeam(project.Key, teamID)
	if team.State != model.TeamRework || team.ReworkCount != 1 {
		t.Fatalf("state after rework = %s, count=%d", team.State, team.ReworkCount)
	}
	if sysutil.FileExists(filepath.Join(teamDir, "check.json")) {
		t.Fatal("rework should invalidate the previous check")
	}
	writeTestFile(t, filepath.Join(teamDir, "result.md"), "# Result\n\n## 经验候选\n- 无新经验（原因）：test\n\nv2 with X\n")
	if code := Run([]string{"lead", "submit"}); code != 0 {
		t.Fatalf("second lead submit failed: %d", code)
	}
	if code := Run([]string{"team", "check", teamID}); code != 0 {
		t.Fatalf("second team check failed: %d", code)
	}
	if code := Run([]string{"team", "accept", teamID, "--note", "reviewed the contract and result", "--lessons-updated"}); code != 0 {
		t.Fatalf("team accept failed: %d", code)
	}
	team, _ = store.LoadTeam(project.Key, teamID)
	if team.State != model.TeamDisbanded {
		t.Fatalf("state after accept = %s, want disbanded", team.State)
	}
	acceptanceData, err := os.ReadFile(filepath.Join(teamDir, "acceptance.md"))
	if err != nil {
		t.Fatalf("acceptance.md missing: %v", err)
	}
	if !strings.Contains(string(acceptanceData), "reviewed the contract and result") || !strings.Contains(string(acceptanceData), "lessons: updated") {
		t.Fatalf("acceptance.md missing note:\n%s", acceptanceData)
	}
	st = fakeHerdrState{}
	if err := sysutil.ReadJSON(statePath, &st); err != nil {
		t.Fatal(err)
	}
	if st.Agents[team.LeadAgent] != nil {
		t.Fatal("lead agent still exists after disband")
	}
	if _, err := store.LoadPaneContext(team.PaneID); err == nil {
		t.Fatal("pane context should be deleted after disband")
	}
	if code := Run([]string{"requirement", "close", reqID}); code != 0 {
		t.Fatalf("requirement close failed: %d", code)
	}
	req, _ := store.LoadRequirement(project.Key, reqID)
	if req.State != model.RequirementClosed {
		t.Fatalf("requirement state = %s, want closed", req.State)
	}
}

func TestGitWorkspaceSurvivesTeamDisband(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init")
	runGit(t, root, "config", "user.email", "test@example.com")
	runGit(t, root, "config", "user.name", "Test")
	writeTestFile(t, filepath.Join(root, "README.md"), "hello\n")
	runGit(t, root, "add", "README.md")
	runGit(t, root, "commit", "-m", "init")
	project, _ := setupProject(t, root)
	if code := Run([]string{"requirement", "new", "--goal", "change the readme"}); code != 0 {
		t.Fatalf("requirement new failed: %d", code)
	}
	reqIDs, _ := store.ListRequirementIDs(project.Key)
	reqID := reqIDs[0]
	writeTestFile(t, filepath.Join(root, "contract.md"), "# Contract\n\nChange README.\n")
	if code := Run([]string{"team", "recruit", "--requirement", reqID, "--purpose", "implement", "--contract", "contract.md", "--size", "1"}); code == 0 {
		t.Fatal("implement team without a workspace should be refused")
	}
	if code := Run([]string{"workspace", "create", "--requirement", reqID}); code != 0 {
		t.Fatalf("workspace create failed: %d", code)
	}
	req, err := store.LoadRequirement(project.Key, reqID)
	if err != nil {
		t.Fatal(err)
	}
	if req.Workspace == "" || !req.WorktreeManaged {
		t.Fatalf("workspace was not created as a managed worktree: %+v", req)
	}
	if _, err := os.Stat(req.Workspace); err != nil {
		t.Fatalf("worktree missing: %v", err)
	}
	if code := Run([]string{"team", "recruit", "--requirement", reqID, "--purpose", "implement", "--contract", "contract.md", "--size", "1"}); code != 0 {
		t.Fatalf("team recruit failed: %d", code)
	}
	teamIDs, _ := store.ListTeamIDs(project.Key)
	teamID := teamIDs[0]
	team, _ := store.LoadTeam(project.Key, teamID)
	if team.Workspace != req.Workspace {
		t.Fatalf("team workspace = %s, want %s", team.Workspace, req.Workspace)
	}
	writeTestFile(t, filepath.Join(store.TeamDir(project.Key, teamID), "result.md"), "# Result\n\nchanged\n")
	if code := Run([]string{"lead", "submit", "--project", root, "--team", teamID}); code != 0 {
		t.Fatalf("lead submit failed: %d", code)
	}
	if code := Run([]string{"team", "accept", teamID, "--note", "checked"}); code != 0 {
		t.Fatalf("team accept failed: %d", code)
	}
	if _, err := os.Stat(req.Workspace); err != nil {
		t.Fatalf("worktree should survive team disband: %v", err)
	}
	if code := Run([]string{"requirement", "close", reqID, "--remove-worktree"}); code != 0 {
		t.Fatalf("requirement close failed: %d", code)
	}
	if _, err := os.Stat(req.Workspace); !os.IsNotExist(err) {
		t.Fatalf("worktree should be removed on close, stat err=%v", err)
	}
}

func TestTeamRepairMarksLost(t *testing.T) {
	root := t.TempDir()
	project, statePath := setupProject(t, root)
	if code := Run([]string{"requirement", "new", "--goal", "repair"}); code != 0 {
		t.Fatalf("requirement new failed: %d", code)
	}
	reqIDs, _ := store.ListRequirementIDs(project.Key)
	writeTestFile(t, filepath.Join(root, "contract.md"), "# Contract\n")
	if code := Run([]string{"team", "recruit", "--requirement", reqIDs[0], "--contract", "contract.md", "--size", "1"}); code != 0 {
		t.Fatalf("team recruit failed: %d", code)
	}
	teamIDs, _ := store.ListTeamIDs(project.Key)
	team, _ := store.LoadTeam(project.Key, teamIDs[0])
	var st fakeHerdrState
	if err := sysutil.ReadJSON(statePath, &st); err != nil {
		t.Fatal(err)
	}
	delete(st.Agents, team.LeadAgent)
	if err := sysutil.WriteJSON(statePath, st); err != nil {
		t.Fatal(err)
	}
	if code := Run([]string{"team", "repair", team.ID}); code != 0 {
		t.Fatalf("team repair failed: %d", code)
	}
	team, _ = store.LoadTeam(project.Key, team.ID)
	if team.State != model.TeamLost {
		t.Fatalf("state after repair = %s, want lost", team.State)
	}
	if code := Run([]string{"team", "disband", team.ID, "--force"}); code != 0 {
		t.Fatalf("force disband failed: %d", code)
	}
}

func TestMemberPaneLifecycle(t *testing.T) {
	root := t.TempDir()
	project, statePath := setupProject(t, root)
	if code := Run([]string{"requirement", "new", "--goal", "members in panes"}); code != 0 {
		t.Fatalf("requirement new failed: %d", code)
	}
	reqIDs, _ := store.ListRequirementIDs(project.Key)
	reqID := reqIDs[0]
	writeTestFile(t, filepath.Join(root, "contract.md"), "# Contract\n\nDesign the app.\n")
	if code := Run([]string{"team", "recruit", "--requirement", reqID, "--purpose", "design", "--contract", "contract.md", "--size", "2"}); code != 0 {
		t.Fatalf("team recruit failed: %d", code)
	}
	teamIDs, _ := store.ListTeamIDs(project.Key)
	teamID := teamIDs[0]
	team, err := store.LoadTeam(project.Key, teamID)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HERDR_PANE_ID", team.PaneID)
	teamDir := store.TeamDir(project.Key, teamID)
	for _, args := range [][]string{
		{"requirement", "list"},
		{"requirement", "show", reqID},
		{"team", "list"},
		{"team", "status", teamID},
	} {
		if code := Run(args); code != 0 {
			t.Fatalf("%v failed: %d", args, code)
		}
	}
	writeTestFile(t, filepath.Join(root, "task-backend.md"), "# 林知远任务\n\n后端设计。\n")
	writeTestFile(t, filepath.Join(root, "task-frontend.md"), "# 沈砚秋任务\n\n前端设计。\n")

	// Names are enforced when the pane is created.
	if code := Run([]string{"member", "recruit", "--name", "员工1", "--agent", "worker1", "--role", "后端", "--task", "task-backend.md"}); code == 0 {
		t.Fatal("placeholder formal name should be refused")
	}
	if code := Run([]string{"member", "recruit", "--name", "林知远", "--agent", "Lin-Zhiyuan", "--role", "后端", "--task", "task-backend.md"}); code == 0 {
		t.Fatal("uppercase agent name should be refused")
	}
	if code := Run([]string{"member", "recruit", "--name", "林知远", "--agent", "lin-zhiyuan", "--role", "后端", "--task", "task-backend.md"}); code != 0 {
		t.Fatalf("member recruit failed: %d", code)
	}
	if code := Run([]string{"member", "recruit", "--name", "苏二", "--agent", "lin-zhiyuan", "--role", "前端", "--task", "task-frontend.md"}); code == 0 {
		t.Fatal("duplicate agent name should be refused")
	}
	if code := Run([]string{"member", "recruit", "--name", "沈砚秋", "--agent", "shen-yanqiu", "--role", "前端", "--task", "task-frontend.md"}); code != 0 {
		t.Fatalf("second member recruit failed: %d", code)
	}

	members, err := store.MembersForTeam(project.Key, teamID)
	if err != nil || len(members) != 2 {
		t.Fatalf("members = %+v, err=%v", members, err)
	}
	lin, err := store.LoadMember(project.Key, teamID, "lin-zhiyuan")
	if err != nil {
		t.Fatal(err)
	}
	if lin.Name != "林知远" || lin.Role != "后端" || lin.State != model.MemberActive || lin.PaneID == "" {
		t.Fatalf("unexpected first member: %+v", lin)
	}
	if !sysutil.FileExists(filepath.Join(teamDir, "members", "lin-zhiyuan", "system-prompt.md")) {
		t.Fatal("member system prompt was not written")
	}
	if !sysutil.FileExists(filepath.Join(teamDir, "members", "lin-zhiyuan", "task.md")) {
		t.Fatal("member task was not copied into the team directory")
	}
	var st fakeHerdrState
	if err := sysutil.ReadJSON(statePath, &st); err != nil {
		t.Fatal(err)
	}
	linAgent := st.Agents["lin-zhiyuan"]
	if linAgent == nil {
		t.Fatal("member agent was not started")
	}
	if fmt.Sprint(linAgent["pane_id"]) != lin.PaneID {
		t.Fatalf("agent pane = %v, member pane = %s", linAgent["pane_id"], lin.PaneID)
	}
	if fmt.Sprint(st.Panes[lin.PaneID]["tab_id"]) != team.TabID {
		t.Fatalf("member pane is in tab %v, want team tab %s", st.Panes[lin.PaneID]["tab_id"], team.TabID)
	}
	native := fmt.Sprint(linAgent["native_args"])
	if !strings.Contains(native, "--append-system-prompt-file") {
		t.Fatalf("member agent is missing the system prompt file arg: %s", native)
	}
	initial := fmt.Sprint(linAgent["last_prompt"])
	for _, want := range []string{"林知远", "lin-zhiyuan", "task.md"} {
		if !strings.Contains(initial, want) {
			t.Fatalf("member prompt missing %q: %s", want, initial)
		}
	}
	ctx, err := store.LoadPaneContext(lin.PaneID)
	if err != nil {
		t.Fatal(err)
	}
	if ctx.MemberAgent != "lin-zhiyuan" || ctx.MemberName != "林知远" || ctx.TeamID != teamID {
		t.Fatalf("unexpected member pane context: %+v", ctx)
	}
	shen, _ := store.LoadMember(project.Key, teamID, "shen-yanqiu")
	if shen.PaneID == "" || shen.PaneID == lin.PaneID {
		t.Fatalf("second member pane is wrong: %+v", shen)
	}
	if fmt.Sprint(st.Panes[shen.PaneID]["tab_id"]) != team.TabID {
		t.Fatalf("second member pane is not in the team tab")
	}

	for _, args := range [][]string{
		{"lead", "status"},
		{"member", "status", "lin-zhiyuan"},
	} {
		if code := Run(args); code != 0 {
			t.Fatalf("%v failed: %d", args, code)
		}
	}
	if code := Run([]string{"member", "wait", "--all", "--timeout", "5"}); code != 0 {
		t.Fatalf("member wait --all failed: %d", code)
	}
	writeTestFile(t, filepath.Join(teamDir, "result.md"), "# Result\n\n| 正式名字 |\n|---|\n| 林知远 |\n| 沈砚秋 |\n")
	if code := Run([]string{"lead", "submit"}); code == 0 {
		t.Fatal("lead submit should fail while members are still active")
	}
	if code := Run([]string{"member", "accept", "lin-zhiyuan", "--note", "checked backend"}); code != 0 {
		t.Fatalf("member accept failed: %d", code)
	}
	if code := Run([]string{"member", "accept", "shen-yanqiu", "--note", "checked frontend"}); code != 0 {
		t.Fatalf("second member accept failed: %d", code)
	}
	if code := Run([]string{"lead", "submit"}); code != 0 {
		t.Fatalf("lead submit failed after member acceptance: %d", code)
	}
	if code := Run([]string{"team", "accept", teamID, "--note", "reviewed the team result"}); code != 0 {
		t.Fatalf("team accept failed: %d", code)
	}
	team, _ = store.LoadTeam(project.Key, teamID)
	if team.State != model.TeamDisbanded {
		t.Fatalf("team state after accept = %s", team.State)
	}
	var after fakeHerdrState
	if err := sysutil.ReadJSON(statePath, &after); err != nil {
		t.Fatal(err)
	}
	if after.Agents["lin-zhiyuan"] != nil || after.Agents["shen-yanqiu"] != nil || after.Agents[team.LeadAgent] != nil {
		t.Fatalf("agents still exist after team accept: %+v", after.Agents)
	}
	for _, paneID := range []string{team.PaneID, lin.PaneID, shen.PaneID} {
		if _, err := store.LoadPaneContext(paneID); err == nil {
			t.Fatalf("pane context still exists after accept: %s", paneID)
		}
	}
}

func TestMemberCloseAndRepair(t *testing.T) {
	root := t.TempDir()
	project, statePath := setupProject(t, root)
	if code := Run([]string{"requirement", "new", "--goal", "replace a member"}); code != 0 {
		t.Fatalf("requirement new failed: %d", code)
	}
	reqIDs, _ := store.ListRequirementIDs(project.Key)
	writeTestFile(t, filepath.Join(root, "contract.md"), "# Contract\n\nDo the work.\n")
	if code := Run([]string{"team", "recruit", "--requirement", reqIDs[0], "--purpose", "implement", "--contract", "contract.md", "--size", "1"}); code != 0 {
		t.Fatalf("team recruit failed: %d", code)
	}
	teamIDs, _ := store.ListTeamIDs(project.Key)
	teamID := teamIDs[0]
	team, _ := store.LoadTeam(project.Key, teamID)
	t.Setenv("HERDR_PANE_ID", team.PaneID)
	writeTestFile(t, filepath.Join(root, "task.md"), "# Task\n\nBuild it.\n")
	if code := Run([]string{"member", "recruit", "--name", "林知远", "--agent", "lin-zhiyuan", "--role", "后端", "--task", "task.md"}); code != 0 {
		t.Fatalf("member recruit failed: %d", code)
	}
	lin, _ := store.LoadMember(project.Key, teamID, "lin-zhiyuan")
	if code := Run([]string{"member", "close", "lin-zhiyuan"}); code != 0 {
		t.Fatalf("member close failed: %d", code)
	}
	lin, _ = store.LoadMember(project.Key, teamID, "lin-zhiyuan")
	if lin.State != model.MemberClosed {
		t.Fatalf("member state after close = %s", lin.State)
	}
	if _, err := store.LoadPaneContext(lin.PaneID); err == nil {
		t.Fatal("closed member pane context still exists")
	}

	if code := Run([]string{"member", "recruit", "--name", "苏二", "--agent", "su-er", "--role", "前端", "--task", "task.md"}); code != 0 {
		t.Fatalf("replacement recruit failed: %d", code)
	}
	su, _ := store.LoadMember(project.Key, teamID, "su-er")
	if su.State != model.MemberActive || su.PaneID == "" || su.PaneID == lin.PaneID {
		t.Fatalf("unexpected replacement member: %+v", su)
	}

	var st fakeHerdrState
	if err := sysutil.ReadJSON(statePath, &st); err != nil {
		t.Fatal(err)
	}
	delete(st.Agents, "su-er")
	if err := sysutil.WriteJSON(statePath, st); err != nil {
		t.Fatal(err)
	}
	if code := Run([]string{"team", "repair", teamID}); code != 0 {
		t.Fatalf("team repair failed: %d", code)
	}
	su, _ = store.LoadMember(project.Key, teamID, "su-er")
	if su.State != model.MemberLost {
		t.Fatalf("member state after repair = %s, want lost", su.State)
	}
	if code := Run([]string{"team", "disband", teamID, "--force"}); code != 0 {
		t.Fatalf("disband failed: %d", code)
	}
	var after fakeHerdrState
	if err := sysutil.ReadJSON(statePath, &after); err != nil {
		t.Fatal(err)
	}
	if len(after.Agents) != 0 {
		t.Fatalf("agents still exist after disband: %+v", after.Agents)
	}
	for _, paneID := range []string{team.PaneID, su.PaneID} {
		if _, err := store.LoadPaneContext(paneID); err == nil {
			t.Fatalf("pane context still exists after disband: %s", paneID)
		}
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func TestTeamStateTransitions(t *testing.T) {
	team := model.Team{State: model.TeamSubmitted}
	if !team.CanAccept() || !team.CanRework() || team.CanSubmit() {
		t.Fatalf("submitted transitions wrong: %+v", team)
	}
	team.State = model.TeamActive
	if !team.CanSubmit() || team.CanAccept() || team.CanRework() {
		t.Fatalf("active transitions wrong: %+v", team)
	}
	team.State = model.TeamAccepted
	if !team.CanDisband(false) || team.IsOpen() {
		t.Fatalf("accepted transitions wrong: %+v", team)
	}
}

func TestMemberAgentNameMustBeUniqueAmongLiveAgents(t *testing.T) {
	root := t.TempDir()
	project, statePath := setupProject(t, root)
	if code := Run([]string{"requirement", "new", "--goal", "agent name collision"}); code != 0 {
		t.Fatalf("requirement new failed: %d", code)
	}
	reqIDs, err := store.ListRequirementIDs(project.Key)
	if err != nil || len(reqIDs) != 1 {
		t.Fatalf("requirement ids = %v, %v", reqIDs, err)
	}
	writeTestFile(t, filepath.Join(root, "contract.md"), "# Contract\n\nCollision.\n")
	if code := Run([]string{"team", "recruit", "--requirement", reqIDs[0], "--purpose", "design", "--contract", "contract.md", "--size", "1"}); code != 0 {
		t.Fatalf("team recruit failed: %d", code)
	}
	teamIDs, err := store.ListTeamIDs(project.Key)
	if err != nil || len(teamIDs) != 1 {
		t.Fatalf("team ids = %v, %v", teamIDs, err)
	}
	team, err := store.LoadTeam(project.Key, teamIDs[0])
	if err != nil {
		t.Fatal(err)
	}
	var st fakeHerdrState
	if err := sysutil.ReadJSON(statePath, &st); err != nil {
		t.Fatal(err)
	}
	st.Agents["taken-agent"] = map[string]any{
		"name": "taken-agent", "pane_id": "w9:p9", "agent_status": "idle",
	}
	if err := sysutil.WriteJSON(statePath, st); err != nil {
		t.Fatal(err)
	}
	err = validateMemberIdentity(project.Key, team, "顾清和", "taken-agent", "员工")
	if err == nil || !strings.Contains(err.Error(), "already live") {
		t.Fatalf("live agent name should be rejected, got: %v", err)
	}
}
