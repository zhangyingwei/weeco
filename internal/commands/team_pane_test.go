package commands

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

type fakeHerdrPane struct {
	PaneID    string `json:"pane_id"`
	TabID     string `json:"tab_id"`
	Workspace string `json:"workspace_id"`
	Cwd       string `json:"cwd"`
	Label     string `json:"label"`
}

type fakeHerdrTab struct {
	TabID     string `json:"tab_id"`
	Workspace string `json:"workspace_id"`
	Label     string `json:"label"`
}

type fakeHerdrState struct {
	Panes      map[string]fakeHerdrPane `json:"panes"`
	Tabs       map[string]fakeHerdrTab  `json:"tabs"`
	Workspaces map[string]any           `json:"workspaces"`
	Agents     map[string]any           `json:"agents"`
}

func readFakeState(t *testing.T, f runtimeFixture) fakeHerdrState {
	t.Helper()
	data, err := os.ReadFile(f.state)
	if err != nil {
		t.Fatal(err)
	}
	var st fakeHerdrState
	if err := json.Unmarshal(data, &st); err != nil {
		t.Fatal(err)
	}
	return st
}

func addTestPane(t *testing.T, f runtimeFixture, paneID, workspace, tab string) {
	t.Helper()
	f.mutate(t, func(st map[string]any) {
		panes, _ := st["panes"].(map[string]any)
		if panes == nil {
			panes = map[string]any{}
			st["panes"] = panes
		}
		panes[paneID] = map[string]any{
			"pane_id":      paneID,
			"tab_id":       tab,
			"workspace_id": workspace,
		}
		tabs, _ := st["tabs"].(map[string]any)
		if tabs == nil {
			tabs = map[string]any{}
			st["tabs"] = tabs
		}
		tabs[tab] = map[string]any{"tab_id": tab, "workspace_id": workspace, "label": "1"}
		workspaces, _ := st["workspaces"].(map[string]any)
		if workspaces == nil {
			workspaces = map[string]any{}
			st["workspaces"] = workspaces
		}
		workspaces[workspace] = map[string]any{"workspace_id": workspace, "label": workspace, "active_tab_id": tab}
	})
}

func TestRuntimeStartTeamPaneUsesWorktreeWorkspace(t *testing.T) {
	f := setupRuntime(t)
	addTestPane(t, f, "wWT:p1", "wWT", "wWT:t1")
	alt := t.TempDir()
	code, output := runtimeOutput(t, "start", "--name", "writer", "--role", "写作", "--task", f.task, "--dir", alt, "--team-pane", "wWT:p1")
	if code != 0 {
		t.Fatalf("start with team pane failed: %s", output)
	}
	run := f.run(t)
	if run.TeamPane != "wWT:p1" || run.TeamWorkspace != "wWT" || run.TeamTab != "wWT:t1" || run.TeamWorkspaceOwned {
		t.Fatalf("team location = %+v", run)
	}
	m := run.Members["writer"]
	if m.PaneID == "" || m.TabID == "" || m.TabID == "wWT:t1" || m.WorkspaceID != "wWT" {
		t.Fatalf("member must own a tab in the worktree workspace: %+v", m)
	}
	st := readFakeState(t, f)
	pane := st.Panes[m.PaneID]
	if pane.Workspace != "wWT" || pane.TabID != m.TabID || pane.Cwd != run.Dir {
		t.Fatalf("member pane = %+v, want worktree workspace, own tab and cwd %s", pane, run.Dir)
	}
	if tab := st.Tabs[m.TabID]; tab.Label != "writer" {
		t.Fatalf("member tab = %+v, want label writer", tab)
	}
	code, output = runtimeOutput(t, "status", "--json")
	if code != 0 || !strings.Contains(output, `"team_pane": "wWT:p1"`) || !strings.Contains(output, `"team_workspace": "wWT"`) || !strings.Contains(output, `"tab_id": "`+m.TabID+`"`) {
		t.Fatalf("status does not expose team location and member tab: %d %s", code, output)
	}

	f.asMember(t, "writer")
	if Run([]string{"report", "--turn", "1", "--file", f.result}) != 0 {
		t.Fatal("member in worktree workspace could not report")
	}
	f.asMember(t, "writer")
	if Run([]string{"send", "lead", "--file", f.result}) != 0 {
		t.Fatal("member in worktree workspace could not message lead")
	}
	f.asLead(t)
	if Run([]string{"send", "writer", "--file", f.task}) != 0 {
		t.Fatal("lead could not send to member in worktree workspace")
	}
	f.asMember(t, "writer")
	if Run([]string{"report", "--turn", "2", "--file", f.result}) != 0 {
		t.Fatal("member in worktree workspace could not report second turn")
	}
	f.asLead(t)
	acceptPass(t, f, "writer", "2")
	if Run([]string{"close"}) != 0 {
		t.Fatal("close failed for team in worktree workspace")
	}
	if got := f.run(t).Members["writer"].State; got != "closed" {
		t.Fatalf("member state = %q, want closed", got)
	}
	after := readFakeState(t, f)
	if _, ok := after.Panes["w1:p1"]; !ok {
		t.Fatal("close removed the lead pane")
	}
	if _, ok := after.Panes["wWT:p1"]; !ok {
		t.Fatal("close removed the worktree root pane")
	}
	if _, ok := after.Tabs[m.TabID]; ok {
		t.Fatal("close left the member tab behind")
	}
	if _, ok := after.Workspaces["wWT"]; !ok {
		t.Fatal("close removed the borrowed worktree workspace")
	}
}

func TestRuntimeStartRejectsDifferentTeamPaneForExistingRun(t *testing.T) {
	f := setupRuntime(t)
	addTestPane(t, f, "wWT:p1", "wWT", "wWT:t1")
	addTestPane(t, f, "wWT:p2", "wWT", "wWT:t1")
	if code := Run([]string{"start", "--name", "writer", "--role", "写作", "--task", f.task, "--team-pane", "wWT:p1"}); code != 0 {
		t.Fatal("initial start failed")
	}
	if code := Run([]string{"start", "--name", "other", "--role", "写作", "--task", f.task, "--team-pane", "wWT:p2"}); code == 0 {
		t.Fatal("existing run accepted a different team pane")
	}
}

func TestRuntimeStartRejectsUnavailableTeamPane(t *testing.T) {
	f := setupRuntime(t)
	if code := Run([]string{"start", "--name", "writer", "--role", "写作", "--task", f.task, "--team-pane", "wWT:missing"}); code == 0 {
		t.Fatal("start accepted an unavailable team pane")
	}
}
