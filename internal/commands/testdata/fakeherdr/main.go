// Command fakeherdr is a minimal Herdr stand-in for weeco tests.
// It keeps its state in the JSON file named by FAKE_HERDR_STATE.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type state struct {
	Agents   map[string]map[string]any `json:"agents"`
	Panes    map[string]map[string]any `json:"panes"`
	Tabs     map[string]map[string]any `json:"tabs"`
	NextPane int                       `json:"next_pane,omitempty"`
}

func main() {
	path := os.Getenv("FAKE_HERDR_STATE")
	st := state{}
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &st)
	}
	if st.Agents == nil {
		st.Agents = map[string]map[string]any{}
	}
	if st.Panes == nil {
		st.Panes = map[string]map[string]any{}
	}
	if st.Tabs == nil {
		st.Tabs = map[string]map[string]any{}
	}
	save := func() {
		data, _ := json.Marshal(st)
		_ = os.WriteFile(path, data, 0o644)
	}
	out := func(v any) {
		data, _ := json.Marshal(v)
		fmt.Println(string(data))
		os.Exit(0)
	}
	args := os.Args[1:]
	if len(args) >= 2 {
		for _, failure := range strings.Split(os.Getenv("FAKE_HERDR_FAIL"), ",") {
			if args[0]+" "+args[1] == failure {
				fmt.Fprintln(os.Stderr, "injected Herdr failure")
				os.Exit(1)
			}
		}
	}
	if msg := os.Getenv("FAKE_HERDR_START_ERROR"); msg != "" && len(args) >= 2 && args[0] == "agent" && args[1] == "start" {
		fmt.Fprintln(os.Stderr, msg)
		os.Exit(1)
	}
	if len(args) >= 1 && args[0] == "--version" {
		fmt.Println("herdr 0.9.3-fake")
		return
	}
	if len(args) >= 2 && args[0] == "integration" && args[1] == "status" {
		fmt.Println("claude: current")
		return
	}
	if len(args) >= 2 && args[0] == "tab" && args[1] == "create" {
		n := len(st.Tabs) + 1
		tabID := fmt.Sprintf("w1:t%d", n)
		st.NextPane++
		paneID := fmt.Sprintf("w1:p%d", st.NextPane)
		st.Tabs[tabID] = map[string]any{
			"tab_id": tabID, "pane_id": paneID,
			"workspace_id": flagValue(args, "--workspace"),
			"cwd":          flagValue(args, "--cwd"),
			"label":        flagValue(args, "--label"),
		}
		st.Panes[paneID] = map[string]any{
			"pane_id": paneID, "tab_id": tabID,
			"workspace_id": flagValue(args, "--workspace"),
		}
		save()
		out(map[string]any{"result": map[string]any{
			"tab":       map[string]any{"tab_id": tabID},
			"root_pane": map[string]any{"pane_id": paneID},
		}})
	}
	if len(args) >= 3 && args[0] == "agent" && args[1] == "start" {
		name := args[2]
		paneID := flagValue(args, "--pane")
		native := []string{}
		for i, arg := range args {
			if arg == "--" {
				native = append(native, args[i+1:]...)
				break
			}
		}
		if failModel := os.Getenv("FAKE_HERDR_START_FAIL_MODEL"); failModel != "" {
			for i, arg := range native {
				if (arg == "--model" || arg == "-m") && i+1 < len(native) && native[i+1] == failModel {
					fmt.Fprintln(os.Stderr, "injected model startup failure")
					os.Exit(1)
				}
			}
		}
		session := map[string]any{"value": "session-" + name}
		kind := flagValue(args, "--kind")
		if kind == "" {
			kind = "claude"
		}
		agent := map[string]any{
			"name": name, "agent": kind, "agent_status": "idle", "completion_seq": 0,
			"pane_id": paneID, "tab_id": paneTab(st, paneID),
			"native_args": native,
		}
		// Herdr 0.9.3 exposes a session for Claude and PI, but not for Codex.
		if kind != "codex" {
			agent["agent_session"] = session
		}
		st.Agents[name] = agent
		if pane := st.Panes[paneID]; pane != nil {
			pane["agent"] = kind
			if kind == "codex" {
				delete(pane, "agent_session")
			} else {
				pane["agent_session"] = session
			}
		}
		save()
		out(map[string]any{"result": map[string]any{}})
	}
	if len(args) >= 2 && args[0] == "agent" && args[1] == "wait" {
		out(map[string]any{"result": map[string]any{}})
	}
	if len(args) >= 2 && args[0] == "agent" && args[1] == "list" {
		agents := []map[string]any{}
		for _, a := range st.Agents {
			agents = append(agents, a)
		}
		out(map[string]any{"result": map[string]any{"agents": agents}})
	}
	if len(args) >= 2 && args[0] == "agent" && args[1] == "prompt" {
		if len(args) >= 4 {
			if agent, ok := findAgent(st, args[2]); ok {
				if agent["agent_status"] == "blocked" {
					data, _ := json.Marshal(map[string]any{"error": map[string]string{"code": "agent_blocked", "message": "agent is blocked; no input was sent"}})
					fmt.Fprintln(os.Stderr, string(data))
					os.Exit(1)
				}
				agent["last_prompt"] = args[3]
				save()
			}
		}
		out(map[string]any{"result": map[string]any{}})
	}
	if len(args) >= 3 && args[0] == "agent" && args[1] == "get" {
		agent, ok := findAgent(st, args[2])
		if !ok {
			agent = map[string]any{}
		}
		out(map[string]any{"result": map[string]any{"agent": agent}})
	}
	if len(args) >= 2 && args[0] == "pane" && args[1] == "split" {
		target := ""
		if len(args) >= 3 && !strings.HasPrefix(args[2], "-") {
			target = args[2]
		}
		if target == "" {
			target = flagValue(args, "--pane")
		}
		st.NextPane++
		paneID := fmt.Sprintf("w1:p%d", st.NextPane)
		tabID := paneTab(st, target)
		workspace := ""
		if p, ok := st.Panes[target]; ok {
			workspace, _ = p["workspace_id"].(string)
		}
		st.Panes[paneID] = map[string]any{
			"pane_id": paneID, "tab_id": tabID, "workspace_id": workspace,
			"cwd": flagValue(args, "--cwd"),
		}
		save()
		out(map[string]any{"result": map[string]any{
			"pane": map[string]any{"pane_id": paneID},
		}})
	}
	if len(args) >= 2 && args[0] == "pane" && args[1] == "list" {
		panes := []map[string]any{}
		for _, p := range st.Panes {
			panes = append(panes, p)
		}
		out(map[string]any{"result": map[string]any{"panes": panes}})
	}
	if len(args) >= 3 && args[0] == "pane" && args[1] == "close" {
		closePane(st, args[2])
		save()
		out(map[string]any{"result": map[string]any{}})
	}
	if len(args) >= 3 && args[0] == "tab" && args[1] == "close" {
		tabID := args[2]
		for paneID, pane := range st.Panes {
			if pane["tab_id"] == tabID {
				closePane(st, paneID)
			}
		}
		delete(st.Tabs, tabID)
		save()
		out(map[string]any{"result": map[string]any{}})
	}
	out(map[string]any{"result": map[string]any{}})
}

func findAgent(st state, target string) (map[string]any, bool) {
	if agent, ok := st.Agents[target]; ok {
		return agent, true
	}
	for _, agent := range st.Agents {
		if fmt.Sprint(agent["pane_id"]) == target {
			return agent, true
		}
	}
	return nil, false
}

func closePane(st state, paneID string) {
	delete(st.Panes, paneID)
	for name, agent := range st.Agents {
		if agent["pane_id"] == paneID {
			delete(st.Agents, name)
		}
	}
}

func paneTab(st state, paneID string) string {
	if p, ok := st.Panes[paneID]; ok {
		if tab, ok := p["tab_id"].(string); ok {
			return tab
		}
	}
	return ""
}

func flagValue(args []string, name string) string {
	for i, arg := range args {
		if arg == name && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}
