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
	Agents        map[string]map[string]any `json:"agents"`
	Panes         map[string]map[string]any `json:"panes"`
	Tabs          map[string]map[string]any `json:"tabs"`
	Workspaces    map[string]map[string]any `json:"workspaces,omitempty"`
	NextPane      int                       `json:"next_pane,omitempty"`
	NextTab       int                       `json:"next_tab,omitempty"`
	NextWorkspace int                       `json:"next_workspace,omitempty"`
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
	if st.Workspaces == nil {
		st.Workspaces = map[string]map[string]any{}
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
	if len(args) >= 2 && args[0] == "workspace" && args[1] == "create" {
		st.NextWorkspace++
		if st.NextWorkspace < 2 {
			st.NextWorkspace = 2
		}
		workspaceID := fmt.Sprintf("w%d", st.NextWorkspace)
		tabID := nextTabID(&st, workspaceID)
		paneID := nextPaneID(&st, workspaceID)
		st.Workspaces[workspaceID] = map[string]any{
			"workspace_id": workspaceID, "label": flagValue(args, "--label"), "active_tab_id": tabID,
		}
		st.Tabs[tabID] = map[string]any{
			"tab_id": tabID, "workspace_id": workspaceID, "label": "1", "number": 1,
		}
		st.Panes[paneID] = map[string]any{
			"pane_id": paneID, "tab_id": tabID, "workspace_id": workspaceID,
			"cwd": flagValue(args, "--cwd"),
		}
		save()
		out(map[string]any{"result": map[string]any{
			"workspace": map[string]any{"workspace_id": workspaceID},
			"tab":       map[string]any{"tab_id": tabID},
			"root_pane": map[string]any{"pane_id": paneID},
		}})
	}
	if len(args) >= 2 && args[0] == "workspace" && args[1] == "list" {
		workspaces := []map[string]any{}
		for _, workspace := range st.Workspaces {
			workspaces = append(workspaces, workspace)
		}
		out(map[string]any{"result": map[string]any{"workspaces": workspaces}})
	}
	if len(args) >= 3 && args[0] == "workspace" && args[1] == "get" {
		workspace, ok := st.Workspaces[args[2]]
		if !ok {
			fail("workspace_not_found", "workspace "+args[2]+" not found")
		}
		out(map[string]any{"result": map[string]any{"workspace": workspace}})
	}
	if len(args) >= 3 && args[0] == "workspace" && args[1] == "close" {
		workspaceID := args[2]
		if _, ok := st.Workspaces[workspaceID]; !ok && !workspaceHasTabs(st, workspaceID) {
			fail("workspace_not_found", "workspace "+workspaceID+" not found")
		}
		for tabID := range st.Tabs {
			if st.Tabs[tabID]["workspace_id"] == workspaceID {
				delete(st.Tabs, tabID)
			}
		}
		for paneID, pane := range st.Panes {
			if pane["workspace_id"] == workspaceID {
				closePane(st, paneID)
			}
		}
		delete(st.Workspaces, workspaceID)
		save()
		out(map[string]any{"result": map[string]any{"type": "ok"}})
	}
	if len(args) >= 2 && args[0] == "tab" && args[1] == "list" {
		workspace := flagValue(args, "--workspace")
		tabs := []map[string]any{}
		for _, tab := range st.Tabs {
			if workspace == "" || tab["workspace_id"] == workspace {
				tabs = append(tabs, tab)
			}
		}
		out(map[string]any{"result": map[string]any{"tabs": tabs}})
	}
	if len(args) >= 2 && args[0] == "tab" && args[1] == "create" {
		workspace := flagValue(args, "--workspace")
		tabID := nextTabID(&st, workspace)
		paneID := nextPaneID(&st, workspace)
		if _, ok := st.Workspaces[workspace]; !ok {
			st.Workspaces[workspace] = map[string]any{"workspace_id": workspace, "label": workspace}
		}
		st.Tabs[tabID] = map[string]any{
			"tab_id": tabID, "workspace_id": workspace,
			"label":  flagValue(args, "--label"),
			"number": st.NextTab,
		}
		st.Panes[paneID] = map[string]any{
			"pane_id": paneID, "tab_id": tabID, "workspace_id": workspace,
			"cwd": flagValue(args, "--cwd"),
		}
		save()
		out(map[string]any{"result": map[string]any{
			"tab":       map[string]any{"tab_id": tabID},
			"root_pane": map[string]any{"pane_id": paneID},
		}})
	}
	if len(args) >= 4 && args[0] == "tab" && args[1] == "rename" {
		if tab, ok := st.Tabs[args[2]]; ok {
			tab["label"] = args[3]
		}
		save()
		out(map[string]any{"result": map[string]any{}})
	}
	if len(args) >= 3 && args[0] == "tab" && args[1] == "close" {
		tabID := args[2]
		if _, ok := st.Tabs[tabID]; !ok {
			fail("tab_not_found", "tab "+tabID+" not found")
		}
		closeTab(&st, tabID)
		save()
		out(map[string]any{"result": map[string]any{}})
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
	if len(args) >= 2 && args[0] == "pane" && args[1] == "list" {
		workspace := flagValue(args, "--workspace")
		panes := []map[string]any{}
		for _, p := range st.Panes {
			if workspace == "" || p["workspace_id"] == workspace {
				panes = append(panes, p)
			}
		}
		out(map[string]any{"result": map[string]any{"panes": panes}})
	}
	if len(args) >= 3 && args[0] == "pane" && args[1] == "close" {
		paneID := args[2]
		if _, ok := st.Panes[paneID]; !ok {
			fail("pane_not_found", "pane "+paneID+" not found")
		}
		closePaneAndPrune(&st, paneID)
		save()
		out(map[string]any{"result": map[string]any{}})
	}
	if len(args) >= 4 && args[0] == "pane" && args[1] == "rename" {
		if pane, ok := st.Panes[args[2]]; ok {
			pane["label"] = args[3]
		}
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

func nextTabID(st *state, workspace string) string {
	for {
		st.NextTab++
		id := fmt.Sprintf("%s:t%d", workspace, st.NextTab)
		if _, ok := st.Tabs[id]; !ok {
			return id
		}
	}
}

func nextPaneID(st *state, workspace string) string {
	for {
		st.NextPane++
		id := fmt.Sprintf("%s:p%d", workspace, st.NextPane)
		if _, ok := st.Panes[id]; !ok {
			return id
		}
	}
}

func closeTab(st *state, tabID string) {
	workspace := ""
	if tab, ok := st.Tabs[tabID]; ok {
		workspace, _ = tab["workspace_id"].(string)
	}
	for paneID, pane := range st.Panes {
		if pane["tab_id"] == tabID {
			closePane(*st, paneID)
		}
	}
	delete(st.Tabs, tabID)
	if workspace != "" {
		pruneWorkspace(st, workspace)
	}
}

func closePaneAndPrune(st *state, paneID string) {
	workspace := ""
	if pane, ok := st.Panes[paneID]; ok {
		workspace, _ = pane["workspace_id"].(string)
	}
	tabID := paneTab(*st, paneID)
	closePane(*st, paneID)
	if tabID == "" {
		return
	}
	for _, pane := range st.Panes {
		if pane["tab_id"] == tabID {
			return
		}
	}
	delete(st.Tabs, tabID)
	if workspace != "" {
		pruneWorkspace(st, workspace)
	}
}

// pruneWorkspace mirrors Herdr closing a workspace once its last tab is gone.
// Untracked workspaces (the test lead's w1) are never pruned.
func pruneWorkspace(st *state, workspace string) {
	if _, ok := st.Workspaces[workspace]; !ok {
		return
	}
	if workspaceHasTabs(*st, workspace) {
		return
	}
	for paneID, pane := range st.Panes {
		if pane["workspace_id"] == workspace {
			closePane(*st, paneID)
		}
	}
	delete(st.Workspaces, workspace)
}

func workspaceHasTabs(st state, workspace string) bool {
	for _, tab := range st.Tabs {
		if tab["workspace_id"] == workspace {
			return true
		}
	}
	return false
}

func fail(code, message string) {
	data, _ := json.Marshal(map[string]any{"error": map[string]any{"code": code, "message": message}})
	fmt.Fprintln(os.Stderr, string(data))
	os.Exit(1)
}

func flagValue(args []string, name string) string {
	for i, arg := range args {
		if arg == name && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}
