// Command fakeherdr is a minimal Herdr stand-in for weefleet integration tests.
// It keeps its state in the JSON file named by FAKE_HERDR_STATE.
package main

import (
	"encoding/json"
	"fmt"
	"os"
)

type state struct {
	Agents map[string]map[string]any `json:"agents"`
	Panes  map[string]map[string]any `json:"panes"`
	Tabs   map[string]map[string]any `json:"tabs"`
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
		paneID := fmt.Sprintf("w1:p%d", n)
		st.Tabs[tabID] = map[string]any{"tab_id": tabID, "pane_id": paneID}
		st.Panes[paneID] = map[string]any{"pane_id": paneID, "tab_id": tabID, "workspace_id": "w1"}
		save()
		out(map[string]any{"result": map[string]any{
			"tab":       map[string]any{"tab_id": tabID},
			"root_pane": map[string]any{"pane_id": paneID},
		}})
	}
	if len(args) >= 3 && args[0] == "agent" && args[1] == "start" {
		name := args[2]
		paneID := flagValue(args, "--pane")
		st.Agents[name] = map[string]any{
			"name": name, "agent_status": "idle", "completion_seq": 0,
			"pane_id": paneID, "tab_id": paneTab(st, paneID),
		}
		save()
		out(map[string]any{"result": map[string]any{}})
	}
	if len(args) >= 2 && args[0] == "agent" && args[1] == "wait" {
		out(map[string]any{"result": map[string]any{}})
	}
	if len(args) >= 2 && args[0] == "agent" && args[1] == "list" {
		var agents []map[string]any
		for _, a := range st.Agents {
			agents = append(agents, a)
		}
		out(map[string]any{"result": map[string]any{"agents": agents}})
	}
	if len(args) >= 2 && args[0] == "agent" && args[1] == "prompt" {
		out(map[string]any{"result": map[string]any{}})
	}
	if len(args) >= 3 && args[0] == "agent" && args[1] == "get" {
		agent, ok := st.Agents[args[2]]
		if !ok {
			agent = map[string]any{}
		}
		out(map[string]any{"result": map[string]any{"agent": agent}})
	}
	if len(args) >= 2 && args[0] == "pane" && args[1] == "list" {
		var panes []map[string]any
		for _, p := range st.Panes {
			panes = append(panes, p)
		}
		out(map[string]any{"result": map[string]any{"panes": panes}})
	}
	if len(args) >= 3 && args[0] == "pane" && args[1] == "close" {
		paneID := args[2]
		tabID := paneTab(st, paneID)
		delete(st.Panes, paneID)
		for name, agent := range st.Agents {
			if agent["pane_id"] == paneID {
				delete(st.Agents, name)
			}
		}
		save()
		_ = tabID
		out(map[string]any{"result": map[string]any{}})
	}
	if len(args) >= 3 && args[0] == "tab" && args[1] == "close" {
		delete(st.Tabs, args[2])
		save()
		out(map[string]any{"result": map[string]any{}})
	}
	out(map[string]any{"result": map[string]any{}})
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
