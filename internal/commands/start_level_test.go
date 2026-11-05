package commands

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeProjectConfig(t *testing.T, root, body string) {
	t.Helper()
	path := filepath.Join(root, ".weeco", "config.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func agentNativeArgs(t *testing.T, f runtimeFixture, agentName string) string {
	t.Helper()
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
	return strings.Join(st.Agents[agentName].NativeArgs, " ")
}

func TestRuntimeStartLevelPassesModelArgs(t *testing.T) {
	f := setupRuntime(t)
	writeProjectConfig(t, f.root, `{"levels":{"senior":[{"model":"model-a","effort":"high"}]}}`)
	if code := Run([]string{"start", "--name", "writer", "--role", "写作", "--level", "senior", "--task", f.task}); code != 0 {
		t.Fatal("start with level failed")
	}
	m := f.run(t).Members["writer"]
	if m.Level != "senior" || m.Model != "model-a" || m.Effort != "high" || m.Kind != "claude" {
		t.Fatalf("member = %+v", m)
	}
	args := agentNativeArgs(t, f, m.Agent)
	if !strings.Contains(args, "--model model-a") || !strings.Contains(args, "--effort high") {
		t.Fatalf("native args = %q", args)
	}
	if len(m.Attempts) != 1 || m.Attempts[0].Error != "" {
		t.Fatalf("attempts = %+v", m.Attempts)
	}
}

func TestRuntimeStartDefaultLevelUsesConfig(t *testing.T) {
	f := setupRuntime(t)
	writeProjectConfig(t, f.root, `{"levels":{"mid":[{"model":"default-mid"}]}}`)
	if code := Run([]string{"start", "--name", "writer", "--role", "写作", "--task", f.task}); code != 0 {
		t.Fatal("start failed")
	}
	m := f.run(t).Members["writer"]
	if m.Level != "mid" || m.Model != "default-mid" || m.Effort != "medium" {
		t.Fatalf("member = %+v", m)
	}
}

func TestRuntimeStartModelAndAgentFilters(t *testing.T) {
	f := setupRuntime(t)
	writeProjectConfig(t, f.root, `{"levels":{"mid":[
		{"model":"claude-model"},
		{"agent":"codex","model":"codex-model","effort":"none"}
	]}}`)
	if code := Run([]string{"start", "--name", "writer", "--role", "写作", "--level", "mid", "--model", "codex-model", "--task", f.task}); code != 0 {
		t.Fatal("model filter failed")
	}
	m := f.run(t).Members["writer"]
	if m.Kind != "codex" || m.Model != "codex-model" || m.Effort != "none" {
		t.Fatalf("member = %+v", m)
	}
	args := agentNativeArgs(t, f, m.Agent)
	if !strings.Contains(args, "-m codex-model") || strings.Contains(args, "--model") {
		t.Fatalf("codex native args = %q", args)
	}

	f2 := setupRuntime(t)
	writeProjectConfig(t, f2.root, `{"levels":{"mid":[{"model":"claude-model"}]}}`)
	if code := Run([]string{"start", "--name", "writer", "--role", "写作", "--level", "mid", "--agent", "codex", "--task", f2.task}); code == 0 {
		t.Fatal("agent filter with no match succeeded")
	}
}

func TestRuntimeStartFailsOverToNextCandidate(t *testing.T) {
	f := setupRuntime(t)
	writeProjectConfig(t, f.root, `{"levels":{"mid":[
		{"model":"bad-model"},
		{"model":"good-model"}
	]}}`)
	t.Setenv("FAKE_HERDR_START_FAIL_MODEL", "bad-model")
	if code := Run([]string{"start", "--name", "writer", "--role", "写作", "--level", "mid", "--task", f.task}); code != 0 {
		t.Fatal("failover start failed")
	}
	m := f.run(t).Members["writer"]
	if m.Model != "good-model" || m.State != "running" {
		t.Fatalf("member = %+v", m)
	}
	if len(m.Attempts) != 2 {
		t.Fatalf("attempts = %+v", m.Attempts)
	}
	if !strings.Contains(m.Attempts[0].Error, "injected model startup failure") {
		t.Fatalf("first attempt error = %q", m.Attempts[0].Error)
	}
	if m.Attempts[1].Error != "" {
		t.Fatalf("second attempt error = %q", m.Attempts[1].Error)
	}
	args := agentNativeArgs(t, f, m.Agent)
	if !strings.Contains(args, "--model good-model") {
		t.Fatalf("native args = %q", args)
	}
}

func TestRuntimeStartAllCandidatesFail(t *testing.T) {
	f := setupRuntime(t)
	writeProjectConfig(t, f.root, `{"levels":{"mid":[{"model":"a"},{"model":"b"}]}}`)
	t.Setenv("FAKE_HERDR_START_ERROR", "injected startup failure")
	if code := Run([]string{"start", "--name", "writer", "--role", "写作", "--level", "mid", "--task", f.task}); code == 0 {
		t.Fatal("all candidates failed but start succeeded")
	}
	m := f.run(t).Members["writer"]
	if m.State != "failed" || m.PaneID != "" || len(m.Attempts) != 2 {
		t.Fatalf("member = %+v", m)
	}
	if !strings.Contains(m.LastError, "all candidates failed") {
		t.Fatalf("last error = %q", m.LastError)
	}
}

func TestRuntimeStartStopOnTrustPrompt(t *testing.T) {
	f := setupRuntime(t)
	writeProjectConfig(t, f.root, `{"levels":{"mid":[{"model":"a"},{"model":"b"}]}}`)
	t.Setenv("FAKE_HERDR_START_ERROR", `{"error":{"code":"agent_not_ready","message":"agent is blocked during startup and is not ready for prompts"}}`)
	if code := Run([]string{"start", "--name", "writer", "--role", "写作", "--level", "mid", "--task", f.task}); code == 0 {
		t.Fatal("blocked startup reported success")
	}
	m := f.run(t).Members["writer"]
	if m.State != "failed" || m.PaneID == "" || len(m.Attempts) != 1 {
		t.Fatalf("member = %+v", m)
	}
	if !strings.Contains(m.LastError, "workspace trust") {
		t.Fatalf("missing trust hint: %q", m.LastError)
	}
}

func TestRuntimeArtistUsesCodex(t *testing.T) {
	f := setupRuntime(t)
	if code := Run([]string{"start", "--name", "illustrator", "--role", "artist", "--task", f.task}); code != 0 {
		t.Fatal("artist start failed")
	}
	m := f.run(t).Members["illustrator"]
	if m.Kind != "codex" || m.Role != "画师" || m.Level != "" || m.Model != "" {
		t.Fatalf("artist member = %+v", m)
	}
	args := agentNativeArgs(t, f, m.Agent)
	if strings.Contains(args, "--model") || strings.Contains(args, "-m ") {
		t.Fatalf("artist received model args: %q", args)
	}
	if !strings.Contains(args, "--sandbox workspace-write") {
		t.Fatalf("artist is not a codex member: %q", args)
	}
}

func TestRuntimeArtistRejectsLevelModelAndOtherAgents(t *testing.T) {
	f := setupRuntime(t)
	for _, args := range [][]string{
		{"start", "--name", "illustrator", "--role", "artist", "--level", "mid", "--task", f.task},
		{"start", "--name", "illustrator", "--role", "artist", "--model", "x", "--task", f.task},
		{"start", "--name", "illustrator", "--role", "artist", "--agent", "claude", "--task", f.task},
	} {
		if Run(args) == 0 {
			t.Fatalf("invalid artist start accepted: %v", args)
		}
	}
}

func TestRuntimeLegacyAgentKeepsOldShape(t *testing.T) {
	f := setupRuntime(t)
	if code := Run([]string{"start", "--name", "writer", "--role", "写作", "--task", f.task, "--agent", "codex"}); code != 0 {
		t.Fatal("legacy agent start failed")
	}
	m := f.run(t).Members["writer"]
	if m.Level != "" || m.Model != "" || m.Effort != "" || m.Kind != "codex" {
		t.Fatalf("legacy member = %+v", m)
	}
}

func TestRuntimeAcceptFlow(t *testing.T) {
	f := setupRuntime(t)
	startWriter(t, f)
	f.asMember(t, "writer")
	if code := Run([]string{"report", "--turn", "1", "--file", f.result}); code != 0 {
		t.Fatal("report failed")
	}
	f.asMember(t, "writer")
	if code := Run([]string{"accept", "--turn", "1", "--file", f.result}); code == 0 {
		t.Fatal("member accepted its own work")
	}
	f.asLead(t)
	if code := Run([]string{"accept", "writer", "--turn", "1", "--file", f.result}); code != 0 {
		t.Fatal("lead accept failed")
	}
	m := f.run(t).Members["writer"]
	if m.AcceptedTurn != 1 || m.AcceptedRev != 1 || m.AcceptNote == "" {
		t.Fatalf("accept fields = %+v", m)
	}
	if _, err := os.Stat(m.AcceptNote); err != nil {
		t.Fatalf("accept note missing: %v", err)
	}

	f.asMember(t, "writer")
	if code := Run([]string{"report", "--turn", "1", "--file", f.result}); code != 0 {
		t.Fatal("second report failed")
	}
	f.asLead(t)
	code, output := runtimeOutput(t, "status")
	if code != 0 || !strings.Contains(output, "accepted: stale") {
		t.Fatalf("stale acceptance not shown after a new revision: %d %s", code, output)
	}
	if code := Run([]string{"accept", "writer", "--turn", "1", "--file", f.result}); code != 0 {
		t.Fatal("re-accept failed")
	}
	if code := Run([]string{"send", "writer", "--file", f.task}); code != 0 {
		t.Fatal("send after accept failed")
	}
	code, output = runtimeOutput(t, "status")
	if code != 0 || !strings.Contains(output, "accepted: stale") {
		t.Fatalf("stale acceptance not shown after a new turn: %d %s", code, output)
	}
}

func TestConfigCommands(t *testing.T) {
	f := setupRuntime(t)
	if code, output := runtimeOutput(t, "config", "check"); code != 0 || !strings.Contains(output, "source: built-in") {
		t.Fatalf("config check default = %d %s", code, output)
	}
	if code := Run([]string{"config", "init"}); code != 0 {
		t.Fatal("config init failed")
	}
	path := filepath.Join(f.home, "config.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("global config missing: %v", err)
	}
	if code := Run([]string{"config", "init"}); code == 0 {
		t.Fatal("config init overwrote without --force")
	}
	if code := Run([]string{"config", "init", "--force"}); code != 0 {
		t.Fatal("config init --force failed")
	}
	if code := Run([]string{"config", "init", "--project"}); code != 0 {
		t.Fatal("config init --project failed")
	}
	if _, err := os.Stat(filepath.Join(f.root, ".weeco", "config.json")); err != nil {
		t.Fatalf("project config missing: %v", err)
	}
	writeProjectConfig(t, f.root, `{"levels":{"mid":[{"model":"picked"}]}}`)
	if code, output := runtimeOutput(t, "config", "check"); code != 0 || !strings.Contains(output, "picked") {
		t.Fatalf("config check project = %d %s", code, output)
	}
	writeProjectConfig(t, f.root, `{"levels":{"mid":[{"model":"x"}]},"extra":1}`)
	if code := Run([]string{"config", "check"}); code == 0 {
		t.Fatal("unknown config field accepted")
	}
}
