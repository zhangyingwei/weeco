package commands

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

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
	if run.TeamPane != "wWT:p1" || run.TeamWorkspace != "wWT" || run.TeamTab != "wWT:t1" {
		t.Fatalf("team location = %+v", run)
	}
	m := run.Members["writer"]
	if m.PaneID == "" {
		t.Fatalf("member missing: %+v", run)
	}
	data, err := os.ReadFile(f.state)
	if err != nil {
		t.Fatal(err)
	}
	var st struct {
		Panes map[string]struct {
			Workspace string `json:"workspace_id"`
			Tab       string `json:"tab_id"`
			Cwd       string `json:"cwd"`
		} `json:"panes"`
	}
	if err := json.Unmarshal(data, &st); err != nil {
		t.Fatal(err)
	}
	pane := st.Panes[m.PaneID]
	if pane.Workspace != "wWT" || pane.Tab != "wWT:t1" || pane.Cwd != run.Dir {
		t.Fatalf("member pane = %+v, want worktree workspace and cwd %s", pane, run.Dir)
	}
	code, output = runtimeOutput(t, "status", "--json")
	if code != 0 || !strings.Contains(output, `"team_pane": "wWT:p1"`) || !strings.Contains(output, `"team_workspace": "wWT"`) {
		t.Fatalf("status does not expose team location: %d %s", code, output)
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
	if Run([]string{"close"}) != 0 {
		t.Fatal("close failed for team in worktree workspace")
	}
	if got := f.run(t).Members["writer"].State; got != "closed" {
		t.Fatalf("member state = %q, want closed", got)
	}
	// The lead pane must survive closing the team.
	data, err = os.ReadFile(f.state)
	if err != nil {
		t.Fatal(err)
	}
	var after struct {
		Panes map[string]any `json:"panes"`
	}
	if err := json.Unmarshal(data, &after); err != nil {
		t.Fatal(err)
	}
	if _, ok := after.Panes["w1:p1"]; !ok {
		t.Fatal("close removed the lead pane")
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
