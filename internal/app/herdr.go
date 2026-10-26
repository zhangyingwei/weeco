package app

import (
	"encoding/json"
	"os"
	"strings"
	"time"
)

func requireHerdr() (string, error) {
	if os.Getenv("HERDR_ENV") != "1" {
		return "", errorf("not running inside a Herdr-managed pane (HERDR_ENV != 1); run this command from the coordinator pane in Herdr")
	}
	workspace := os.Getenv("HERDR_WORKSPACE_ID")
	if workspace == "" {
		return "", errorf("HERDR_WORKSPACE_ID is not set; run from a Herdr-managed pane")
	}
	return workspace, nil
}

func herdr(args []string, timeout time.Duration) (map[string]any, error) {
	res, err := runCmd(append([]string{"herdr"}, args...), "", timeout, nil)
	if err != nil {
		return nil, err
	}
	if res.ExitCode != 0 {
		detail := strings.TrimSpace(res.Stderr)
		if detail == "" {
			detail = strings.TrimSpace(res.Stdout)
		}
		var parsed struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal([]byte(detail), &parsed) == nil && parsed.Error.Message != "" {
			detail = parsed.Error.Message
		}
		return nil, errorf("herdr %s failed: %s", strings.Join(args, " "), detail)
	}
	text := strings.TrimSpace(res.Stdout)
	if text == "" {
		return map[string]any{}, nil
	}
	var data map[string]any
	if json.Unmarshal([]byte(text), &data) != nil {
		return map[string]any{"raw": text}, nil
	}
	return data, nil
}

func resultMap(data map[string]any) map[string]any {
	if m, ok := data["result"].(map[string]any); ok {
		return m
	}
	return map[string]any{}
}

func herdrAgents() map[string]map[string]any {
	data, err := herdr([]string{"agent", "list"}, 0)
	if err != nil {
		return map[string]map[string]any{}
	}
	agents, _ := resultMap(data)["agents"].([]any)
	out := map[string]map[string]any{}
	for _, item := range agents {
		a, ok := item.(map[string]any)
		if !ok {
			continue
		}
		name := asString(a["name"])
		if name != "" {
			out[name] = a
		}
	}
	return out
}

func herdrAgent(name string) map[string]any {
	return herdrAgents()[name]
}

func herdrTabCreate(workspace, cwd, label string) (string, string, error) {
	data, err := herdr([]string{
		"tab", "create",
		"--workspace", workspace,
		"--cwd", cwd,
		"--label", label,
		"--no-focus",
	}, 0)
	if err != nil {
		return "", "", err
	}
	result := resultMap(data)
	tab, _ := result["tab"].(map[string]any)
	pane, _ := result["root_pane"].(map[string]any)
	tabID := asString(tab["tab_id"])
	paneID := asString(pane["pane_id"])
	if tabID == "" || paneID == "" {
		return "", "", errorf("herdr tab create returned no tab/pane id: %v", data)
	}
	return tabID, paneID, nil
}

func herdrAgentStart(name, paneID, kind string, timeoutMS int, extraDirs []string) error {
	args := []string{
		"agent", "start", name,
		"--kind", kind,
		"--pane", paneID,
		"--timeout", itoa(timeoutMS),
	}
	if len(extraDirs) > 0 {
		// Claude Code needs --add-dir to write report/evidence outside the
		// worktree. Other agents may not support it.
		args = append(args, "--")
		args = append(args, "--add-dir")
		args = append(args, extraDirs...)
	}
	timeout := time.Duration(timeoutMS)*time.Millisecond + 15*time.Second
	if timeout < 30*time.Second {
		timeout = 30 * time.Second
	}
	_, err := herdr(args, timeout)
	return err
}

func herdrAgentWait(name string, timeoutS float64, until string) error {
	args := []string{"agent", "wait", name}
	if until != "" {
		args = append(args, "--until", until)
	}
	args = append(args, "--timeout", itoa(int(timeoutS*1000)))
	_, err := herdr(args, time.Duration(timeoutS)*time.Second+15*time.Second)
	return err
}

func herdrAgentPrompt(name, text string, wait bool, timeoutS float64) error {
	args := []string{"agent", "prompt", name, text}
	timeout := 60 * time.Second
	if wait {
		args = append(args, "--wait")
		if timeoutS > 0 {
			args = append(args, "--timeout", itoa(int(timeoutS*1000)))
			timeout = time.Duration(timeoutS)*time.Second + 15*time.Second
		}
	}
	_, err := herdr(args, timeout)
	return err
}

func herdrPaneClose(paneID string) error {
	_, err := herdr([]string{"pane", "close", paneID}, 0)
	return err
}

func herdrTabClose(tabID string) error {
	_, err := herdr([]string{"tab", "close", tabID}, 0)
	return err
}

func herdrPanes(workspace string) ([]map[string]any, error) {
	data, err := herdr([]string{"pane", "list", "--workspace", workspace}, 0)
	if err != nil {
		return nil, err
	}
	items, _ := resultMap(data)["panes"].([]any)
	var panes []map[string]any
	for _, item := range items {
		if m, ok := item.(map[string]any); ok {
			panes = append(panes, m)
		}
	}
	return panes, nil
}
