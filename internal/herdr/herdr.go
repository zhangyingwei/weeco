// Package herdr is the thin adapter over the herdr CLI. A team is one Herdr
// pane: the lead agent runs in that pane and its members are subagents.
package herdr

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"weeco/internal/sysutil"
)

func RequireWorkspace() (string, error) {
	if os.Getenv("HERDR_ENV") != "1" {
		return "", fmt.Errorf("not running inside a Herdr pane (HERDR_ENV != 1); team commands must run from the boss pane")
	}
	workspace := os.Getenv("HERDR_WORKSPACE_ID")
	if workspace == "" {
		return "", fmt.Errorf("HERDR_WORKSPACE_ID is not set; run from a Herdr-managed pane")
	}
	return workspace, nil
}

func Run(args []string, timeout time.Duration) (map[string]any, error) {
	res, err := sysutil.Run(append([]string{"herdr"}, args...), "", timeout, nil)
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
		return nil, fmt.Errorf("herdr %s failed: %s", strings.Join(args, " "), detail)
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

func Agents() map[string]map[string]any {
	data, err := Run([]string{"agent", "list"}, 0)
	if err != nil {
		return map[string]map[string]any{}
	}
	items, _ := resultMap(data)["agents"].([]any)
	out := map[string]map[string]any{}
	for _, item := range items {
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

func Agent(name string) map[string]any { return Agents()[name] }

func AgentStatus(name string) string {
	a := Agent(name)
	if a == nil {
		return ""
	}
	return asString(a["agent_status"])
}

func TabCreate(workspace, cwd, label string) (string, string, error) {
	data, err := Run([]string{
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
		return "", "", fmt.Errorf("herdr tab create returned no tab/pane id: %v", data)
	}
	return tabID, paneID, nil
}

func AgentStart(name, paneID, kind string, timeoutMS int, extraDirs []string, permissionMode, systemPromptFile string) error {
	args := []string{
		"agent", "start", name,
		"--kind", kind,
		"--pane", paneID,
		"--timeout", sysutil.Itoa(timeoutMS),
	}
	var passthrough []string
	if len(extraDirs) > 0 {
		passthrough = append(passthrough, "--add-dir")
		passthrough = append(passthrough, extraDirs...)
	}
	if kind == "claude" && permissionMode != "" {
		passthrough = append(passthrough, "--permission-mode", permissionMode)
	}
	if kind == "claude" && systemPromptFile != "" {
		// Herdr rejects agent start arguments containing control characters, so
		// a multi-line prompt must be passed by file, not inline.
		passthrough = append(passthrough, "--append-system-prompt-file", systemPromptFile)
	}
	if len(passthrough) > 0 {
		args = append(args, "--")
		args = append(args, passthrough...)
	}
	timeout := time.Duration(timeoutMS)*time.Millisecond + 15*time.Second
	if timeout < 30*time.Second {
		timeout = 30 * time.Second
	}
	// A freshly split pane may still be spawning its shell. Herdr rejects
	// agent start with agent_pane_busy until that shell is interactive, so
	// retry briefly instead of failing the member recruitment.
	deadline := time.Now().Add(30 * time.Second)
	for {
		_, err := Run(args, timeout)
		if err == nil {
			return nil
		}
		if !agentPaneBusy(err) || time.Now().After(deadline) {
			return err
		}
		time.Sleep(300 * time.Millisecond)
	}
}

func agentPaneBusy(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "agent_pane_busy") || strings.Contains(msg, "not an available shell")
}

func AgentWait(name string, timeoutS float64, until string) error {
	args := []string{"agent", "wait", name}
	if until != "" {
		args = append(args, "--until", until)
	}
	args = append(args, "--timeout", sysutil.Itoa(int(timeoutS*1000)))
	_, err := Run(args, time.Duration(timeoutS)*time.Second+15*time.Second)
	return err
}

func AgentPrompt(name, text string, wait bool, timeoutS float64) error {
	args := []string{"agent", "prompt", name, text}
	timeout := 60 * time.Second
	if wait {
		args = append(args, "--wait")
		if timeoutS > 0 {
			args = append(args, "--timeout", sysutil.Itoa(int(timeoutS*1000)))
			timeout = time.Duration(timeoutS)*time.Second + 15*time.Second
		}
	}
	_, err := Run(args, timeout)
	return err
}

func PaneClose(paneID string) error {
	_, err := Run([]string{"pane", "close", paneID}, 0)
	return err
}

// PaneSplit creates a sibling pane in the target pane's tab and returns the new
// pane id. Employees are added this way, so a team stays one tab/panel.
func PaneSplit(targetPane, direction, cwd string, ratio float64) (string, error) {
	args := []string{"pane", "split", targetPane, "--direction", direction}
	if ratio > 0 {
		args = append(args, "--ratio", fmt.Sprintf("%.2f", ratio))
	}
	if cwd != "" {
		args = append(args, "--cwd", cwd)
	}
	args = append(args, "--no-focus")
	data, err := Run(args, 0)
	if err != nil {
		return "", err
	}
	pane, _ := resultMap(data)["pane"].(map[string]any)
	paneID := asString(pane["pane_id"])
	if paneID == "" {
		return "", fmt.Errorf("herdr pane split returned no pane id: %v", data)
	}
	return paneID, nil
}

func PaneRename(paneID, label string) error {
	if paneID == "" || label == "" {
		return nil
	}
	_, err := Run([]string{"pane", "rename", paneID, label}, 0)
	return err
}

func TabClose(tabID string) error {
	_, err := Run([]string{"tab", "close", tabID}, 0)
	return err
}

func Panes(workspace string) ([]map[string]any, error) {
	data, err := Run([]string{"pane", "list", "--workspace", workspace}, 0)
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

func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
