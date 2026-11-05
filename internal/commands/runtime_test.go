package commands

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type runtimeFixture struct {
	home, root, state, task, result string
}

type runtimeMemberSnapshot struct {
	PaneID    string `json:"pane_id"`
	Session   string `json:"session"`
	Agent     string `json:"agent"`
	Kind      string `json:"kind"`
	Role      string `json:"role"`
	Level     string `json:"level"`
	Model     string `json:"model"`
	Effort    string `json:"effort"`
	State     string `json:"state"`
	Turn      int    `json:"turn"`
	Revision  int    `json:"revision"`
	Report    string `json:"report"`
	LastError string `json:"last_error"`
	Attempts  []struct {
		Agent  string `json:"agent"`
		Model  string `json:"model"`
		Effort string `json:"effort"`
		Error  string `json:"error"`
	} `json:"attempts"`
	AcceptedTurn int    `json:"accepted_turn"`
	AcceptedRev  int    `json:"accepted_revision"`
	AcceptNote   string `json:"accept_note"`
}

type runtimeSnapshot struct {
	ID       string                           `json:"id"`
	LeadPane string                           `json:"lead_pane"`
	Dir      string                           `json:"dir"`
	Closed   bool                             `json:"closed"`
	Members  map[string]runtimeMemberSnapshot `json:"members"`
}

func setupRuntime(t *testing.T) runtimeFixture {
	t.Helper()
	f := runtimeFixture{home: t.TempDir(), root: t.TempDir(), state: filepath.Join(t.TempDir(), "herdr.json")}
	bin := t.TempDir()
	_, source, _, _ := runtime.Caller(0)
	fakeSource := filepath.Join(filepath.Dir(source), "testdata", "fakeherdr", "main.go")
	cmd := exec.Command("go", "build", "-o", filepath.Join(bin, "herdr"), fakeSource)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build fakeherdr: %v\n%s", err, output)
	}
	for _, name := range []string{"claude", "codex", "pi"} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("WEECO_HOME", f.home)
	t.Setenv("FAKE_HERDR_STATE", f.state)
	t.Setenv("HERDR_ENV", "1")
	t.Setenv("HERDR_WORKSPACE_ID", "w1")
	t.Setenv("HERDR_PANE_ID", "w1:p1")
	st := map[string]any{
		"panes":     map[string]any{"w1:p1": map[string]any{"pane_id": "w1:p1", "tab_id": "w1:t1", "workspace_id": "w1", "agent": "claude", "agent_session": map[string]any{"value": "lead-one"}}},
		"agents":    map[string]any{"lead": map[string]any{"name": "lead", "pane_id": "w1:p1", "agent_status": "working", "agent_session": map[string]any{"value": "lead-one"}}},
		"next_pane": 1,
	}
	b, _ := json.Marshal(st)
	if err := os.WriteFile(f.state, b, 0o600); err != nil {
		t.Fatal(err)
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })
	if err := os.Chdir(f.root); err != nil {
		t.Fatal(err)
	}
	// Test helpers use an absolute fake build directory after chdir.
	f.task = filepath.Join(f.root, "task.md")
	f.result = filepath.Join(f.root, "answer.md")
	if err := os.WriteFile(f.task, []byte("根据原始材料写通知。缺失事实留空，不发布。"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f.result, []byte("活动通知：时间待定。来源：给定材料。已核对无编造。"), 0o600); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f runtimeFixture) run(t *testing.T) runtimeSnapshot {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(f.home, "runs", "*", "run.json"))
	if err != nil || len(paths) != 1 {
		t.Fatalf("run files = %v, %v", paths, err)
	}
	b, err := os.ReadFile(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	var run runtimeSnapshot
	if err := json.Unmarshal(b, &run); err != nil {
		t.Fatal(err)
	}
	return run
}

func (f runtimeFixture) asLead(t *testing.T) { t.Helper(); t.Setenv("HERDR_PANE_ID", "w1:p1") }
func (f runtimeFixture) asMember(t *testing.T, name string) {
	t.Helper()
	t.Setenv("HERDR_PANE_ID", f.run(t).Members[name].PaneID)
}

func TestRuntimeMemberGetsFullPermissions(t *testing.T) {
	f := setupRuntime(t)
	if code := Run([]string{"start", "--name", "writer", "--role", "写作", "--task", f.task}); code != 0 {
		t.Fatalf("start failed: %d", code)
	}
	data, err := os.ReadFile(f.state)
	if err != nil {
		t.Fatal(err)
	}
	var st struct {
		Agents map[string]struct {
			NativeArgs []string `json:"native_args"`
			LastPrompt string   `json:"last_prompt"`
		} `json:"agents"`
	}
	if err := json.Unmarshal(data, &st); err != nil {
		t.Fatal(err)
	}
	mode, prompt := "", ""
	hasAllowlist := false
	for _, agent := range st.Agents {
		if agent.LastPrompt != "" {
			prompt = agent.LastPrompt
		}
		for i, arg := range agent.NativeArgs {
			switch arg {
			case "--allowedTools":
				hasAllowlist = true
			case "--permission-mode":
				if i+1 < len(agent.NativeArgs) {
					mode = agent.NativeArgs[i+1]
				}
			}
		}
	}
	if mode != "bypassPermissions" {
		t.Fatalf("member permission mode = %q; members must run with full permissions", mode)
	}
	if hasAllowlist {
		t.Fatal("full-permission members must not also carry an allowlist")
	}
	if !strings.Contains(prompt, "maximum permissions") {
		t.Fatalf("member prompt does not state maximum permissions: %s", prompt)
	}
	for _, old := range []string{"pre-authorized for these two weeco commands only", "auto mode"} {
		if strings.Contains(prompt, old) {
			t.Fatalf("member prompt still describes the old narrow permission model: %s", prompt)
		}
	}
}

func TestRuntimeLifecycle(t *testing.T) {
	f := setupRuntime(t)
	if code := Run([]string{"start", "--name", "writer", "--role", "写作", "--task", f.task}); code != 0 {
		t.Fatalf("start failed: %d", code)
	}
	run := f.run(t)
	if run.LeadPane != "w1:p1" || len(run.Members) != 1 || run.Members["writer"].PaneID == "" {
		t.Fatalf("run = %+v", run)
	}
	if code := Run([]string{"start", "--name", "writer", "--role", "写作", "--task", f.task}); code != 0 {
		t.Fatal("identical start is not idempotent")
	}
	if len(f.run(t).Members) != 1 {
		t.Fatal("duplicate member created")
	}
	if code := Run([]string{"close"}); code == 0 {
		t.Fatal("running member was closed without cancellation")
	}
	f.asMember(t, "writer")
	if code := Run([]string{"report", "--turn", "1", "--file", f.result}); code != 0 {
		t.Fatal("first submission failed")
	}
	f.asLead(t)
	if code := Run([]string{"send", "writer", "--file", f.task}); code != 0 {
		t.Fatal("rework failed")
	}
	if m := f.run(t).Members["writer"]; m.Turn != 2 || m.State != "running" || m.Report != "" {
		t.Fatalf("stale report retained: %+v", m)
	}
	f.asMember(t, "writer")
	if code := Run([]string{"report", "--turn", "1", "--file", f.result}); code == 0 {
		t.Fatal("late report accepted as current work")
	}
	if code := Run([]string{"report", "--turn", "2", "--file", f.result}); code != 0 {
		t.Fatal("rework submission failed")
	}
	f.asLead(t)
	if code := Run([]string{"status", "--json"}); code != 0 {
		t.Fatal("status failed")
	}
	if code := Run([]string{"close"}); code != 0 {
		t.Fatal("close failed")
	}
	if !f.run(t).Closed {
		t.Fatal("run still open")
	}
	b, _ := os.ReadFile(f.state)
	var st struct {
		Panes map[string]any `json:"panes"`
	}
	_ = json.Unmarshal(b, &st)
	if len(st.Panes) != 1 || st.Panes["w1:p1"] == nil {
		t.Fatalf("close touched the lead: %s", b)
	}
	if _, err := os.Stat(filepath.Join(f.root, "CLAUDE.md")); !os.IsNotExist(err) {
		t.Fatal("runtime requires or writes project integration")
	}
}

func TestRuntimeMemberCannotRecruitOrCloseOthers(t *testing.T) {
	f := setupRuntime(t)
	if Run([]string{"start", "--name", "writer", "--role", "写作", "--task", f.task}) != 0 {
		t.Fatal("start failed")
	}
	f.asMember(t, "writer")
	if Run([]string{"start", "--name", "other", "--role", "研究", "--task", f.task}) == 0 {
		t.Fatal("member recruited a recursive team")
	}
	if Run([]string{"close", "--cancel", "--reason", "stop"}) == 0 {
		t.Fatal("member closed its team")
	}
	if Run([]string{"send", "lead", "--file", f.result}) != 0 {
		t.Fatal("member cannot contact lead")
	}
}

func TestRuntimeBlockedReportAndExplicitResume(t *testing.T) {
	f := setupRuntime(t)
	if Run([]string{"start", "--name", "writer", "--role", "写作", "--task", f.task}) != 0 {
		t.Fatal("start failed")
	}
	f.asMember(t, "writer")
	if Run([]string{"report", "--turn", "1", "--blocked", "--file", f.result}) != 0 {
		t.Fatal("blocked report failed")
	}
	if f.run(t).Members["writer"].State != "blocked" {
		t.Fatal("block not recorded")
	}
	f.asLead(t)
	if Run([]string{"close"}) == 0 {
		t.Fatal("blocked work passed normal close")
	}
	if Run([]string{"send", "writer", "--file", f.task}) != 0 {
		t.Fatal("resume failed")
	}
	f.asMember(t, "writer")
	if Run([]string{"report", "--turn", "2", "--file", f.result}) != 0 {
		t.Fatal("resumed result failed")
	}
}
