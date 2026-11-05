package commands

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"weeco/internal/store"
	"weeco/internal/sysutil"
)

func runtimeOutput(t *testing.T, args ...string) (int, string) {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	code := Run(args)
	_ = w.Close()
	os.Stdout = old
	data, _ := io.ReadAll(r)
	_ = r.Close()
	return code, string(data)
}

func (f runtimeFixture) mutate(t *testing.T, fn func(map[string]any)) {
	t.Helper()
	data, err := os.ReadFile(f.state)
	if err != nil {
		t.Fatal(err)
	}
	var st map[string]any
	if err := json.Unmarshal(data, &st); err != nil {
		t.Fatal(err)
	}
	fn(st)
	data, _ = json.Marshal(st)
	if err := os.WriteFile(f.state, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func startWriter(t *testing.T, f runtimeFixture) {
	t.Helper()
	if Run([]string{"start", "--name", "writer", "--role", "写作", "--task", f.task}) != 0 {
		t.Fatal("start failed")
	}
}

func TestRuntimeWaitIdleAndOldReportDoNotMeanCompletion(t *testing.T) {
	f := setupRuntime(t)
	startWriter(t, f)
	started := time.Now()
	code, output := runtimeOutput(t, "status", "--wait", "--timeout", "0.04", "--json")
	if code != 0 || !strings.Contains(output, `"reason": "timeout"`) || time.Since(started) < 35*time.Millisecond {
		t.Fatalf("idle treated as done: %d %s", code, output)
	}
	f.asMember(t, "writer")
	if Run([]string{"report", "--turn", "1", "--file", f.result}) != 0 {
		t.Fatal("report failed")
	}
	f.asLead(t)
	code, output = runtimeOutput(t, "status", "--wait", "--timeout", "0.04", "--json")
	if code != 0 || !strings.Contains(output, `"reason": "attention"`) {
		t.Fatalf("new report missed: %s", output)
	}
	code, output = runtimeOutput(t, "status", "--wait", "--timeout", "0.04", "--json")
	if code != 0 || !strings.Contains(output, `"reason": "timeout"`) {
		t.Fatalf("old report repeated: %s", output)
	}
}

func TestRuntimeReworkWaitIgnoresSupersededReport(t *testing.T) {
	f := setupRuntime(t)
	startWriter(t, f)
	f.asMember(t, "writer")
	if Run([]string{"report", "--turn", "1", "--file", f.result}) != 0 {
		t.Fatal("initial submission failed")
	}
	f.asLead(t)
	if code, _ := runtimeOutput(t, "status", "--json"); code != 0 {
		t.Fatal("snapshot failed")
	}
	if Run([]string{"send", "writer", "--file", f.task}) != 0 {
		t.Fatal("rework failed")
	}
	code, output := runtimeOutput(t, "status", "--wait", "--timeout", "0.04", "--json")
	if code != 0 || !strings.Contains(output, `"reason": "timeout"`) {
		t.Fatalf("superseded report woke the new assignment: %d %s", code, output)
	}
	f.asMember(t, "writer")
	if Run([]string{"report", "--turn", "2", "--file", f.result}) != 0 {
		t.Fatal("new submission failed")
	}
	f.asLead(t)
	code, output = runtimeOutput(t, "status", "--wait", "--timeout", "0.04", "--json")
	if code != 0 || !strings.Contains(output, `"reason": "attention"`) || !strings.Contains(output, `"turn": 2`) {
		t.Fatalf("new report was missed: %d %s", code, output)
	}
}

func TestRuntimeAssignmentPinsCLIAndHome(t *testing.T) {
	f := setupRuntime(t)
	startWriter(t, f)
	m := f.run(t).Members["writer"]
	data, err := os.ReadFile(f.state)
	if err != nil {
		t.Fatal(err)
	}
	var st struct {
		Agents map[string]map[string]any `json:"agents"`
	}
	if err := json.Unmarshal(data, &st); err != nil {
		t.Fatal(err)
	}
	prompt, _ := st.Agents[m.Agent]["last_prompt"].(string)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{exe, "WEECO_HOME=", f.home} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("assignment missing %q: %s", want, prompt)
		}
	}
}

func TestRuntimeTransportFailureIsNotMemberLoss(t *testing.T) {
	f := setupRuntime(t)
	startWriter(t, f)
	t.Setenv("FAKE_HERDR_FAIL", "agent list")
	if Run([]string{"status", "--json"}) == 0 {
		t.Fatal("transport error treated as successful query")
	}
	if m := f.run(t).Members["writer"]; m.State != "running" {
		t.Fatalf("transport error changed member: %+v", m)
	}
}

func TestRuntimeStartFailureCleansOwnedPane(t *testing.T) {
	f := setupRuntime(t)
	t.Setenv("FAKE_HERDR_FAIL", "agent start")
	if Run([]string{"start", "--name", "writer", "--role", "写作", "--task", f.task}) == 0 {
		t.Fatal("start failure hidden")
	}
	data, _ := os.ReadFile(f.state)
	var st struct {
		Panes map[string]any `json:"panes"`
	}
	_ = json.Unmarshal(data, &st)
	if len(st.Panes) != 1 || st.Panes["w1:p1"] == nil {
		t.Fatalf("failed start left pane: %s", data)
	}
}

func TestRuntimeFailedReplacementKeepsUnclosedPane(t *testing.T) {
	for _, failure := range []string{"agent start", "agent prompt"} {
		t.Run(failure, func(t *testing.T) {
			f := setupRuntime(t)
			t.Setenv("FAKE_HERDR_FAIL", failure+",pane close")
			if Run([]string{"start", "--name", "writer", "--role", "写作", "--task", f.task}) == 0 {
				t.Fatal("start failure hidden")
			}
			before := f.run(t).Members["writer"]
			if before.State != "failed" || before.PaneID == "" {
				t.Fatalf("failed cleanup lost pane: %+v", before)
			}
			t.Setenv("FAKE_HERDR_FAIL", "")
			if Run([]string{"start", "--name", "writer", "--role", "写作", "--task", f.task, "--replace"}) == 0 {
				t.Fatal("replacement abandoned an unclosed pane")
			}
			if after := f.run(t).Members["writer"]; after.PaneID != before.PaneID || after.Agent != before.Agent {
				t.Fatalf("replacement lost the owned resource: before=%+v after=%+v", before, after)
			}
			if Run([]string{"close", "--cancel", "--reason", "clean up failed startup"}) != 0 {
				t.Fatal("cannot close the retained failed pane")
			}
			data, err := os.ReadFile(f.state)
			if err != nil {
				t.Fatal(err)
			}
			var st struct {
				Panes map[string]any `json:"panes"`
			}
			if err := json.Unmarshal(data, &st); err != nil {
				t.Fatal(err)
			}
			if len(st.Panes) != 1 || st.Panes["w1:p1"] == nil {
				t.Fatalf("cleanup left an untracked pane: %s", data)
			}
		})
	}
}

func TestRuntimePartialCloseIsRetryable(t *testing.T) {
	f := setupRuntime(t)
	startWriter(t, f)
	f.asMember(t, "writer")
	if Run([]string{"report", "--turn", "1", "--file", f.result}) != 0 {
		t.Fatal("report failed")
	}
	f.asLead(t)
	t.Setenv("FAKE_HERDR_FAIL", "pane close")
	if Run([]string{"close"}) == 0 {
		t.Fatal("close failure hidden")
	}
	if f.run(t).Closed || f.run(t).Members["writer"].State == "closed" {
		t.Fatal("failed close discarded context")
	}
	t.Setenv("FAKE_HERDR_FAIL", "")
	if Run([]string{"close"}) != 0 {
		t.Fatal("retry failed")
	}
}

func TestRuntimeBlockedDeliveryPreservesCurrentAssignment(t *testing.T) {
	for _, submitted := range []bool{false, true} {
		label := "running"
		if submitted {
			label = "submitted"
		}
		t.Run(label, func(t *testing.T) {
			f := setupRuntime(t)
			startWriter(t, f)
			if submitted {
				f.asMember(t, "writer")
				if Run([]string{"report", "--turn", "1", "--file", f.result}) != 0 {
					t.Fatal("initial submission failed")
				}
				f.asLead(t)
			}
			before := f.run(t).Members["writer"]
			f.mutate(t, func(st map[string]any) {
				st["agents"].(map[string]any)[before.Agent].(map[string]any)["agent_status"] = "blocked"
			})
			if Run([]string{"send", "writer", "--file", f.task}) == 0 {
				t.Fatal("blocked agent accepted work")
			}
			after := f.run(t).Members["writer"]
			if after.Turn != before.Turn || after.State != before.State || after.Report != before.Report {
				t.Fatalf("undelivered work invalidated the current assignment: before=%+v after=%+v", before, after)
			}
			f.mutate(t, func(st map[string]any) {
				st["agents"].(map[string]any)[before.Agent].(map[string]any)["agent_status"] = "working"
			})
			f.asMember(t, "writer")
			if Run([]string{"report", "--turn", "1", "--file", f.result}) != 0 {
				t.Fatal("undelivered instruction made the original turn stale")
			}
		})
	}
}

func TestRuntimeUnconfirmedDeliveryKeepsNewTurn(t *testing.T) {
	f := setupRuntime(t)
	startWriter(t, f)
	f.asMember(t, "writer")
	if Run([]string{"report", "--turn", "1", "--file", f.result}) != 0 {
		t.Fatal("initial submission failed")
	}
	f.asLead(t)
	t.Setenv("FAKE_HERDR_FAIL", "agent prompt")
	if Run([]string{"send", "writer", "--file", f.task}) == 0 {
		t.Fatal("unconfirmed delivery reported success")
	}
	if m := f.run(t).Members["writer"]; m.Turn != 2 || m.State != "running" || m.Report != "" {
		t.Fatalf("ambiguous delivery restored an old completion claim: %+v", m)
	}
}

func TestRuntimeSendRequiresMemberToStart(t *testing.T) {
	f := setupRuntime(t)
	startWriter(t, f)
	f.asMember(t, "writer")
	if Run([]string{"report", "--turn", "1", "--file", f.result}) != 0 {
		t.Fatal("initial submission failed")
	}
	f.asLead(t)
	t.Setenv("FAKE_HERDR_FAIL", "agent wait")
	if Run([]string{"send", "writer", "--file", f.task}) == 0 {
		t.Fatal("send reported success although the member never started")
	}
	m := f.run(t).Members["writer"]
	if m.Turn != 2 || m.State != "running" || m.Report != "" || m.LastError == "" {
		t.Fatalf("unconfirmed send state = %+v", m)
	}
}

func TestRuntimeSendRefusesWorkingMember(t *testing.T) {
	f := setupRuntime(t)
	startWriter(t, f)
	f.asMember(t, "writer")
	if Run([]string{"report", "--turn", "1", "--file", f.result}) != 0 {
		t.Fatal("initial submission failed")
	}
	f.asLead(t)
	m := f.run(t).Members["writer"]
	f.mutate(t, func(st map[string]any) {
		st["agents"].(map[string]any)[m.Agent].(map[string]any)["agent_status"] = "working"
	})
	before := f.run(t).Members["writer"]
	if Run([]string{"send", "writer", "--file", f.task}) == 0 {
		t.Fatal("sent work while the member was still working")
	}
	after := f.run(t).Members["writer"]
	if after.Turn != before.Turn || after.State != before.State || after.Report != before.Report {
		t.Fatalf("refused send changed the assignment: before=%+v after=%+v", before, after)
	}
	f.mutate(t, func(st map[string]any) {
		st["agents"].(map[string]any)[m.Agent].(map[string]any)["agent_status"] = "idle"
	})
	if Run([]string{"send", "writer", "--file", f.task}) != 0 {
		t.Fatal("send failed after the member became idle")
	}
}

func TestRuntimeStartExplainsWorkspaceTrust(t *testing.T) {
	f := setupRuntime(t)
	t.Setenv("FAKE_HERDR_START_ERROR", `{"error":{"code":"agent_not_ready","message":"agent ... is blocked during startup and is not ready for prompts"}}`)
	if Run([]string{"start", "--name", "writer", "--role", "写作", "--task", f.task}) == 0 {
		t.Fatal("blocked startup reported success")
	}
	m := f.run(t).Members["writer"]
	if m.State != "failed" || !strings.Contains(m.LastError, "workspace trust") {
		t.Fatalf("missing workspace trust hint: %+v", m)
	}
}

func TestRuntimeStartExplainsCodexStartupPrompt(t *testing.T) {
	f := setupRuntime(t)
	t.Setenv("FAKE_HERDR_START_ERROR", `{"error":{"code":"agent_not_ready","message":"agent ... is blocked during startup and is not ready for prompts"}}`)
	if Run([]string{"start", "--name", "writer", "--role", "写作", "--task", f.task, "--agent", "codex"}) == 0 {
		t.Fatal("blocked codex startup reported success")
	}
	m := f.run(t).Members["writer"]
	if m.State != "failed" || !strings.Contains(m.LastError, "Codex pane") {
		t.Fatalf("missing codex startup hint: %+v", m)
	}
	if strings.Contains(m.LastError, "workspace trust") {
		t.Fatalf("codex failure gave a Claude-specific hint: %s", m.LastError)
	}
}

func TestRuntimeStartRequiresMemberToWork(t *testing.T) {
	f := setupRuntime(t)
	t.Setenv("FAKE_HERDR_FAIL", "agent wait")
	if Run([]string{"start", "--name", "writer", "--role", "写作", "--task", f.task}) == 0 {
		t.Fatal("unconfirmed task delivery reported success")
	}
	m := f.run(t).Members["writer"]
	if m.State != "running" || m.PaneID == "" || m.Session == "" || !strings.Contains(m.LastError, "task delivery not confirmed") {
		t.Fatalf("start did not retain the member for inspection: %+v", m)
	}
	data, _ := os.ReadFile(f.state)
	var st struct {
		Panes map[string]any `json:"panes"`
	}
	_ = json.Unmarshal(data, &st)
	if len(st.Panes) != 2 {
		t.Fatalf("unconfirmed start closed the member pane: %s", data)
	}
	f.asMember(t, "writer")
	if Run([]string{"report", "--turn", "1", "--file", f.result}) != 0 {
		t.Fatal("member could not report after start returned unconfirmed")
	}
}

func TestRuntimeStartAgentKinds(t *testing.T) {
	cases := []struct {
		kind       string
		wantArgs   []string
		guardFile  string
		guardMatch string
	}{
		{"claude", []string{"--permission-mode", "bypassPermissions", "--settings"}, "claude-guard-settings.json", "guard --run-dir"},
		{"codex", []string{"--sandbox", "workspace-write", "--ask-for-approval", "never", "check_for_update_on_startup=false", "sandbox_workspace_write.network_access=true"}, "", ""},
		{"pi", []string{"--approve", "--extension"}, "pi-guard-extension.js", "tool_call"},
	}
	for _, tc := range cases {
		t.Run(tc.kind, func(t *testing.T) {
			f := setupRuntime(t)
			name := "writer"
			if code := Run([]string{"start", "--name", name, "--role", "写作", "--task", f.task, "--agent", tc.kind}); code != 0 {
				t.Fatal("start failed")
			}
			run := f.run(t)
			m := run.Members[name]
			if m.Kind != tc.kind {
				t.Fatalf("member kind = %q, want %q", m.Kind, tc.kind)
			}
			data, err := os.ReadFile(f.state)
			if err != nil {
				t.Fatal(err)
			}
			var st struct {
				Agents map[string]struct {
					NativeArgs []string `json:"native_args"`
				} `json:"agents"`
			}
			if err := json.Unmarshal(data, &st); err != nil {
				t.Fatal(err)
			}
			args := strings.Join(st.Agents[m.Agent].NativeArgs, " ")
			for _, want := range tc.wantArgs {
				if !strings.Contains(args, want) {
					t.Fatalf("%s args %q missing %q", tc.kind, args, want)
				}
			}
			if tc.kind == "codex" && strings.Contains(args, "--permission-mode") {
				t.Fatalf("codex args still use claude permissions: %q", args)
			}
			if tc.kind == "codex" && m.Session != "" {
				t.Fatalf("codex member has a session %q; Herdr 0.9.3 does not provide one", m.Session)
			}
			if tc.guardFile != "" {
				path := filepath.Join(f.home, "runs", run.ID, "members", name, tc.guardFile)
				guard, err := os.ReadFile(path)
				if err != nil {
					t.Fatalf("guard file missing: %v", err)
				}
				if !strings.Contains(string(guard), tc.guardMatch) {
					t.Fatalf("guard file %s missing %q: %s", tc.guardFile, tc.guardMatch, guard)
				}
			}
		})
	}
	if f := setupRuntime(t); Run([]string{"start", "--name", "writer", "--role", "写作", "--task", f.task, "--agent", "gemini"}) == 0 {
		t.Fatal("unsupported agent kind accepted")
	}
}

func TestRuntimeStartDirSetsRunDirectory(t *testing.T) {
	f := setupRuntime(t)
	alt := t.TempDir()
	if code := Run([]string{"start", "--name", "writer", "--role", "写作", "--task", f.task, "--dir", alt}); code != 0 {
		t.Fatal("start with --dir failed")
	}
	want, err := sysutil.AbsPath(alt)
	if err != nil {
		t.Fatal(err)
	}
	run := f.run(t)
	if run.Dir != want {
		t.Fatalf("run dir = %q, want %q", run.Dir, want)
	}
	data, err := os.ReadFile(f.state)
	if err != nil {
		t.Fatal(err)
	}
	var st struct {
		Panes map[string]struct {
			Cwd string `json:"cwd"`
		} `json:"panes"`
	}
	if err := json.Unmarshal(data, &st); err != nil {
		t.Fatal(err)
	}
	m := run.Members["writer"]
	if got := st.Panes[m.PaneID].Cwd; got != want {
		t.Fatalf("member pane cwd = %q, want %q", got, want)
	}
	guardPath := filepath.Join(f.home, "runs", run.ID, "members", "writer", "claude-guard-settings.json")
	guard, err := os.ReadFile(guardPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(guard), want) {
		t.Fatalf("guard settings do not use run dir %q: %s", want, guard)
	}
	code, output := runtimeOutput(t, "status", "--json")
	if code != 0 || !strings.Contains(output, `"dir": "`+want+`"`) {
		t.Fatalf("status does not show dir: %d %s", code, output)
	}
}

func TestRuntimeStartDirRejectsDifferentExistingRun(t *testing.T) {
	f := setupRuntime(t)
	startWriter(t, f)
	if code := Run([]string{"start", "--name", "other", "--role", "写作", "--task", f.task, "--dir", t.TempDir()}); code == 0 {
		t.Fatal("start accepted a different directory for an existing run")
	}
	if members := f.run(t).Members; len(members) != 1 {
		t.Fatalf("mismatched --dir changed members: %+v", members)
	}
}

func TestRuntimeStartDirValidation(t *testing.T) {
	f := setupRuntime(t)
	cases := []struct {
		name string
		dir  string
	}{
		{"relative", "relative-dir"},
		{"missing", filepath.Join(t.TempDir(), "missing")},
		{"file", f.task},
	}
	for _, tc := range cases {
		if code := Run([]string{"start", "--name", "writer", "--role", "写作", "--task", f.task, "--dir", tc.dir}); code == 0 {
			t.Errorf("%s --dir accepted: %s", tc.name, tc.dir)
		}
	}
}

func TestRuntimeCodexMemberUsesAgentNameWithoutSession(t *testing.T) {
	f := setupRuntime(t)
	if Run([]string{"start", "--name", "writer", "--role", "写作", "--task", f.task, "--agent", "codex"}) != 0 {
		t.Fatal("codex start failed")
	}
	m := f.run(t).Members["writer"]
	if m.State != "running" || m.Session != "" {
		t.Fatalf("unexpected codex member: %+v", m)
	}
	f.asMember(t, "writer")
	if Run([]string{"report", "--turn", "1", "--file", f.result}) != 0 {
		t.Fatal("codex member could not report without a session")
	}
	f.asLead(t)
	code, output := runtimeOutput(t, "status", "--json")
	if code != 0 || !strings.Contains(output, `"live": "idle"`) {
		t.Fatalf("status did not recognize codex identity: %d %s", code, output)
	}
	f.mutate(t, func(st map[string]any) {
		agents := st["agents"].(map[string]any)
		delete(agents, m.Agent)
		agents["stranger"] = map[string]any{
			"name": "stranger", "pane_id": m.PaneID, "tab_id": "w1:t1", "agent_status": "idle",
		}
	})
	if Run([]string{"send", "writer", "--file", f.task}) == 0 {
		t.Fatal("sent work to a replaced codex agent")
	}
	if Run([]string{"close", "--cancel", "--reason", "cancel"}) == 0 {
		t.Fatal("closed a replaced codex agent")
	}
}

func TestRuntimeRejectsPaneSessionReplacement(t *testing.T) {
	f := setupRuntime(t)
	startWriter(t, f)
	m := f.run(t).Members["writer"]
	f.mutate(t, func(st map[string]any) {
		st["agents"].(map[string]any)[m.Agent].(map[string]any)["agent_session"] = map[string]any{"value": "stranger"}
		st["panes"].(map[string]any)[m.PaneID].(map[string]any)["agent_session"] = map[string]any{"value": "stranger"}
	})
	if Run([]string{"send", "writer", "--file", f.task}) == 0 {
		t.Fatal("sent work to replacement session")
	}
	if Run([]string{"close", "--cancel", "--reason", "cancel"}) == 0 {
		t.Fatal("closed replacement session")
	}
	f.asMember(t, "writer")
	if Run([]string{"report", "--turn", "1", "--file", f.result}) == 0 {
		t.Fatal("replacement session submitted old member result")
	}
}

func TestRuntimeSameAliasesAreIsolatedByLead(t *testing.T) {
	f := setupRuntime(t)
	startWriter(t, f)
	first := f.run(t)
	f.mutate(t, func(st map[string]any) {
		st["panes"].(map[string]any)["w1:p9"] = map[string]any{"pane_id": "w1:p9", "tab_id": "w1:t9", "workspace_id": "w1", "agent_session": map[string]any{"value": "lead-two"}}
		st["agents"].(map[string]any)["lead2"] = map[string]any{"pane_id": "w1:p9", "tab_id": "w1:t9", "agent_status": "working", "agent_session": map[string]any{"value": "lead-two"}}
	})
	t.Setenv("HERDR_PANE_ID", "w1:p9")
	if Run([]string{"start", "--name", "writer", "--role", "写作", "--task", f.task}) != 0 {
		t.Fatal("same alias failed in another lead")
	}
	runs, err := store.Runs()
	if err != nil || len(runs) != 2 {
		t.Fatalf("runs = %v, %v", runs, err)
	}
	if Run([]string{"send", "writer", "--run", first.ID, "--file", f.task}) == 0 {
		t.Fatal("other lead controlled first run")
	}
	for _, run := range runs {
		if run.ID != first.ID && run.Members["writer"].Agent == first.Members["writer"].Agent {
			t.Fatal("agent names collide")
		}
	}
}

func TestRuntimeStrictArgumentsAndCancellation(t *testing.T) {
	f := setupRuntime(t)
	for _, args := range [][]string{
		{"start", "--name", "writer", "--role", "写作", "--task", f.task, "--unknown"},
		{"start", "--name", "../writer", "--role", "写作", "--task", f.task},
		{"status", "--timeout", "NaN"},
	} {
		if Run(args) == 0 {
			t.Fatalf("invalid args accepted: %v", args)
		}
	}
	startWriter(t, f)
	for _, value := range []string{"NaN", "+Inf", "-Inf", "0", "-1"} {
		if Run([]string{"status", "--timeout", value, "--json"}) == 0 {
			t.Fatalf("invalid timeout accepted: %s", value)
		}
	}
	if Run([]string{"close", "--cancel"}) == 0 {
		t.Fatal("cancel without reason accepted")
	}
	if Run([]string{"close", "--cancel", "--reason", "用户取消"}) != 0 {
		t.Fatal("cancellation failed")
	}
	if _, err := os.Stat(filepath.Join(f.root, "config.json")); !os.IsNotExist(err) {
		t.Fatal("created legacy config")
	}
}
