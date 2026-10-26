package store

import (
	"os"
	"path/filepath"
	"testing"

	"weeco/internal/model"
	"weeco/internal/sysutil"
)

func TestProjectRegistrationAndRequirementIDs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("WEECO_HOME", home)
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "nested", "dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	project, err := InitProject(root, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if project.Key == "" || project.Root == "" {
		t.Fatalf("bad project: %+v", project)
	}
	found, err := FindProject(filepath.Join(root, "nested", "dir"))
	if err != nil {
		t.Fatal(err)
	}
	if found.Key != project.Key || found.Root != project.Root {
		t.Fatalf("find project = %+v, want %+v", found, project)
	}
	reqID, err := NextID(project.Key, "R")
	if err != nil {
		t.Fatal(err)
	}
	if reqID != "R-"+todayForTest()+"-01" {
		t.Fatalf("requirement id = %s", reqID)
	}
	if err := Append(project.Key, "requirement", reqID, "requirement_created", "boss", map[string]any{
		"goal": "build", "stage": "requirement",
	}); err != nil {
		t.Fatal(err)
	}
	req, err := LoadRequirement(project.Key, reqID)
	if err != nil {
		t.Fatal(err)
	}
	if req.Goal != "build" || req.State != model.RequirementOpen {
		t.Fatalf("requirement = %+v", req)
	}
	next, err := NextID(project.Key, "R")
	if err != nil {
		t.Fatal(err)
	}
	if next != "R-"+todayForTest()+"-02" {
		t.Fatalf("next requirement id = %s", next)
	}
	teamID, err := NextID(project.Key, "T")
	if err != nil {
		t.Fatal(err)
	}
	if teamID != "T-"+todayForTest()+"-01" {
		t.Fatalf("team id = %s", teamID)
	}
}

func TestTeamFoldLifecycle(t *testing.T) {
	home := t.TempDir()
	t.Setenv("WEECO_HOME", home)
	root := t.TempDir()
	project, err := InitProject(root, "demo")
	if err != nil {
		t.Fatal(err)
	}
	reqID := "R-20261026-01"
	teamID := "T-20261026-01"
	mustAppend(t, project.Key, "requirement", reqID, "requirement_created", map[string]any{"goal": "x"})
	mustAppend(t, project.Key, "team", teamID, "team_created", map[string]any{
		"requirement_id": reqID, "purpose": "design", "size": 2,
		"workspace": "/tmp/ws", "contract_path": "/tmp/contract.md",
	})
	mustAppend(t, project.Key, "team", teamID, "team_started", map[string]any{
		"pane_id": "w1:p1", "tab_id": "w1:t1", "lead_agent": "t-lead",
	})
	team, err := LoadTeam(project.Key, teamID)
	if err != nil {
		t.Fatal(err)
	}
	if team.State != model.TeamActive || team.Size != 2 || team.PaneID != "w1:p1" {
		t.Fatalf("active team = %+v", team)
	}
	open, ok, err := OpenTeamForRequirement(project.Key, reqID)
	if err != nil || !ok || open.ID != teamID {
		t.Fatalf("open team = %+v, %v, %v", open, ok, err)
	}
	mustAppend(t, project.Key, "team", teamID, "team_submitted", map[string]any{"result_path": "/tmp/result.md"})
	mustAppend(t, project.Key, "team", teamID, "team_rework", map[string]any{"issues": "fix it"})
	mustAppend(t, project.Key, "team", teamID, "team_submitted", map[string]any{"result_path": "/tmp/result2.md"})
	mustAppend(t, project.Key, "team", teamID, "team_accepted", map[string]any{"note": "ok"})
	mustAppend(t, project.Key, "team", teamID, "team_disbanded", map[string]any{"reason": "accepted"})
	team, err = LoadTeam(project.Key, teamID)
	if err != nil {
		t.Fatal(err)
	}
	if team.State != model.TeamDisbanded || team.ReworkCount != 1 {
		t.Fatalf("final team = %+v", team)
	}
	if _, ok, err := OpenTeamForRequirement(project.Key, reqID); err != nil || ok {
		t.Fatalf("disbanded team should not be open: ok=%v err=%v", ok, err)
	}
}

func TestPaneContextRoundTrip(t *testing.T) {
	home := t.TempDir()
	t.Setenv("WEECO_HOME", home)
	ctx := PaneContext{
		ProjectRoot: "/tmp/project",
		ProjectKey:  "project-1234",
		TeamID:      "T-20261026-01",
		PaneID:      "w1:p1",
		LeadAgent:   "t-20261026-01-lead",
	}
	if err := WritePaneContext(ctx); err != nil {
		t.Fatal(err)
	}
	got, err := LoadPaneContext("w1:p1")
	if err != nil {
		t.Fatal(err)
	}
	if got.TeamID != ctx.TeamID || got.ProjectKey != ctx.ProjectKey || got.PaneID != ctx.PaneID {
		t.Fatalf("pane context = %+v, want %+v", got, ctx)
	}
	if !sysutil.FileExists(PaneContextPath("w1:p1")) {
		t.Fatal("pane context file missing")
	}
	if err := DeletePaneContext("w1:p1"); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPaneContext("w1:p1"); err == nil {
		t.Fatal("pane context should be gone")
	}
}

func mustAppend(t *testing.T, key, entity, id, event string, data map[string]any) {
	t.Helper()
	if err := Append(key, entity, id, event, "test", data); err != nil {
		t.Fatal(err)
	}
}

func todayForTest() string {
	return sysutil.TodayStamp()
}
