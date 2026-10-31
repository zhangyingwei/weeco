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
