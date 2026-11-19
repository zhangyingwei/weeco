package commands

import (
	"testing"

	"weeco/internal/store"
)

func TestRuntimeOwnedWorkspaceGivesEachMemberATab(t *testing.T) {
	f := setupRuntime(t)
	startWriter(t, f)
	if code := Run([]string{"start", "--name", "checker", "--role", "核对", "--task", f.task}); code != 0 {
		t.Fatal("second start failed")
	}
	run := f.run(t)
	if !run.TeamWorkspaceOwned || run.TeamWorkspace == "" || run.TeamWorkspace == "w1" {
		t.Fatalf("run does not use an owned team workspace: %+v", run)
	}
	if run.TeamPane != "" || run.TeamTab != "" {
		t.Fatalf("owned workspace anchor must be consumed by the first member: pane=%q tab=%q", run.TeamPane, run.TeamTab)
	}
	writer, checker := run.Members["writer"], run.Members["checker"]
	if writer.TabID == "" || checker.TabID == "" || writer.TabID == checker.TabID || writer.PaneID == checker.PaneID {
		t.Fatalf("members do not own distinct tabs: writer=%+v checker=%+v", writer, checker)
	}
	if writer.WorkspaceID != run.TeamWorkspace || checker.WorkspaceID != run.TeamWorkspace {
		t.Fatalf("member workspace = %q/%q, want %q", writer.WorkspaceID, checker.WorkspaceID, run.TeamWorkspace)
	}
	st := readFakeState(t, f)
	if len(st.Workspaces) != 1 {
		t.Fatalf("workspaces = %v, want only the owned team workspace", st.Workspaces)
	}
	if len(st.Tabs) != 2 {
		t.Fatalf("tabs = %v, want one tab per member", st.Tabs)
	}
	if st.Tabs[writer.TabID].Label != "writer" || st.Tabs[checker.TabID].Label != "checker" {
		t.Fatalf("tab labels = %+v", st.Tabs)
	}
	if len(st.Panes) != 3 {
		t.Fatalf("panes = %v, want lead plus two member panes", st.Panes)
	}

	f.asMember(t, "writer")
	if Run([]string{"report", "--turn", "1", "--file", f.result}) != 0 {
		t.Fatal("member could not report from its own tab")
	}
}

func TestRuntimeCloseRemovesOwnedWorkspace(t *testing.T) {
	f := setupRuntime(t)
	startWriter(t, f)
	run := f.run(t)
	workspace := run.TeamWorkspace
	f.asMember(t, "writer")
	if Run([]string{"report", "--turn", "1", "--file", f.result}) != 0 {
		t.Fatal("report failed")
	}
	f.asLead(t)
	if Run([]string{"close"}) != 0 {
		t.Fatal("close failed")
	}
	closed := f.run(t)
	if closed.TeamWorkspace != "" || closed.TeamWorkspaceOwned {
		t.Fatalf("closed run kept its owned workspace: %+v", closed)
	}
	st := readFakeState(t, f)
	if _, ok := st.Workspaces[workspace]; ok {
		t.Fatalf("owned workspace %s survived close: %+v", workspace, st.Workspaces)
	}
	if _, ok := st.Panes["w1:p1"]; !ok || len(st.Panes) != 1 {
		t.Fatalf("close touched panes other than team members: %+v", st.Panes)
	}
	if len(st.Tabs) != 0 {
		t.Fatalf("close left tabs behind: %+v", st.Tabs)
	}
}

func TestRuntimeLegacyPaneMemberStillCloses(t *testing.T) {
	f := setupRuntime(t)
	startWriter(t, f)
	run := f.run(t)
	m := run.Members["writer"]
	// Move the member into the lead tab and rewrite the run in the pre-tab
	// shape so the legacy close path stays covered.
	f.mutate(t, func(st map[string]any) {
		pane := st["panes"].(map[string]any)[m.PaneID].(map[string]any)
		pane["tab_id"] = "w1:t1"
		pane["workspace_id"] = "w1"
		tabs := st["tabs"].(map[string]any)
		tabs["w1:t1"] = map[string]any{"tab_id": "w1:t1", "workspace_id": "w1", "label": "1"}
		delete(tabs, m.TabID)
		delete(st["workspaces"].(map[string]any), run.TeamWorkspace)
	})
	legacy, err := store.LoadRun(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	legacy.TeamPane, legacy.TeamWorkspace, legacy.TeamTab = "", "w1", "w1:t1"
	legacy.TeamWorkspaceOwned = false
	legacy.Members["writer"].TabID = ""
	legacy.Members["writer"].WorkspaceID = ""
	if err := store.SaveRun(legacy); err != nil {
		t.Fatal(err)
	}

	f.asLead(t)
	if code := Run([]string{"close", "--cancel", "--reason", "legacy cleanup"}); code != 0 {
		t.Fatal("legacy pane member could not be closed")
	}
	st := readFakeState(t, f)
	if _, ok := st.Panes[m.PaneID]; ok {
		t.Fatalf("legacy member pane survived close: %+v", st.Panes)
	}
	if _, ok := st.Panes["w1:p1"]; !ok {
		t.Fatal("legacy close removed the lead pane")
	}
}

func TestRuntimeReplaceMemberAfterTabWasClosed(t *testing.T) {
	f := setupRuntime(t)
	startWriter(t, f)
	before := f.run(t).Members["writer"]
	f.mutate(t, func(st map[string]any) {
		delete(st["agents"].(map[string]any), before.Agent)
		delete(st["tabs"].(map[string]any), before.TabID)
		delete(st["panes"].(map[string]any), before.PaneID)
	})
	if code := Run([]string{"start", "--name", "writer", "--role", "写作", "--task", f.task, "--replace"}); code != 0 {
		t.Fatal("replace failed after the member tab was closed")
	}
	after := f.run(t).Members["writer"]
	if after.Turn != before.Turn+1 || after.TabID == "" || after.TabID == before.TabID || after.PaneID == before.PaneID {
		t.Fatalf("replacement did not get a new tab: before=%+v after=%+v", before, after)
	}
	st := readFakeState(t, f)
	if tab, ok := st.Tabs[after.TabID]; !ok || tab.Label != "writer" {
		t.Fatalf("replacement tab = %+v", st.Tabs)
	}
	if _, ok := st.Tabs[before.TabID]; ok {
		t.Fatal("replacement reused the closed tab id")
	}
}

func TestRuntimeStartSetupFailureCleansOwnedWorkspace(t *testing.T) {
	f := setupRuntime(t)
	t.Setenv("FAKE_HERDR_FAIL", "tab rename")
	if code := Run([]string{"start", "--name", "writer", "--role", "写作", "--task", f.task}); code == 0 {
		t.Fatal("tab setup failure was hidden")
	}
	st := readFakeState(t, f)
	if len(st.Workspaces) != 0 {
		t.Fatalf("failed first start left a workspace: %+v", st.Workspaces)
	}
	if len(st.Panes) != 1 || st.Panes["w1:p1"].PaneID == "" {
		t.Fatalf("failed first start left panes: %+v", st.Panes)
	}
}

func TestRuntimeSecondMemberTabFailureKeepsTeamWorkspace(t *testing.T) {
	f := setupRuntime(t)
	startWriter(t, f)
	run := f.run(t)
	writer := run.Members["writer"]
	t.Setenv("FAKE_HERDR_FAIL", "tab create")
	if code := Run([]string{"start", "--name", "checker", "--role", "核对", "--task", f.task}); code == 0 {
		t.Fatal("second member tab failure was hidden")
	}
	t.Setenv("FAKE_HERDR_FAIL", "")
	st := readFakeState(t, f)
	if _, ok := st.Workspaces[run.TeamWorkspace]; !ok {
		t.Fatal("second member failure closed the team workspace")
	}
	if _, ok := st.Tabs[writer.TabID]; !ok {
		t.Fatal("second member failure removed the first member tab")
	}
	if got := f.run(t).Members["checker"].State; got != "failed" {
		t.Fatalf("checker state = %q, want failed", got)
	}
}
