// Package herdr adapts the Herdr CLI. Every member runs as a real agent in its
// own pane; transport failures are never reported as an empty list.
package herdr

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"weeco/internal/sysutil"
)

type CommandError struct {
	Code    string
	Message string
}

func (e *CommandError) Error() string { return e.Message }

func Run(args []string, timeout time.Duration) (map[string]any, error) {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	res, err := sysutil.Run(append([]string{"herdr"}, args...), "", timeout, nil)
	if err != nil {
		return nil, err
	}
	if res.ExitCode != 0 || res.TimedOut {
		detail := strings.TrimSpace(res.Stderr)
		if detail == "" {
			detail = strings.TrimSpace(res.Stdout)
		}
		var parsed struct {
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		code := ""
		if json.Unmarshal([]byte(detail), &parsed) == nil {
			if parsed.Error.Message != "" {
				detail = parsed.Error.Message
			}
			if !res.TimedOut {
				code = parsed.Error.Code
			}
		}
		return nil, &CommandError{Code: code, Message: fmt.Sprintf("herdr %s failed: %s", strings.Join(args[:min(2, len(args))], " "), detail)}
	}
	var data map[string]any
	if err := json.Unmarshal([]byte(res.Stdout), &data); err != nil {
		return nil, fmt.Errorf("herdr returned invalid JSON: %w", err)
	}
	if value, ok := data["error"]; ok && value != nil {
		return nil, fmt.Errorf("herdr returned an error: %v", value)
	}
	return data, nil
}

func resultMap(data map[string]any) map[string]any { m, _ := data["result"].(map[string]any); return m }
func String(m map[string]any, key string) string   { s, _ := m[key].(string); return s }
func Session(m map[string]any) string {
	s, _ := m["agent_session"].(map[string]any)
	return String(s, "value")
}

func List(kind, workspace string) ([]map[string]any, error) {
	args := []string{kind, "list"}
	if kind == "pane" && workspace != "" {
		args = append(args, "--workspace", workspace)
	}
	data, err := Run(args, 0)
	if err != nil {
		return nil, err
	}
	items, ok := resultMap(data)[kind+"s"].([]any)
	if !ok {
		return nil, fmt.Errorf("herdr %s list returned no %s array", kind, kind+"s")
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok || String(m, "pane_id") == "" {
			return nil, fmt.Errorf("herdr %s list returned an invalid item", kind)
		}
		out = append(out, m)
	}
	return out, nil
}

func ByPane(items []map[string]any, id string) map[string]any {
	for _, m := range items {
		if String(m, "pane_id") == id {
			return m
		}
	}
	return nil
}

func PaneSplit(target, direction, cwd string, ratio float64) (string, error) {
	args := []string{"pane", "split", target, "--direction", direction, "--cwd", cwd, "--no-focus"}
	if ratio > 0 {
		args = append(args, "--ratio", fmt.Sprintf("%.2f", ratio))
	}
	data, err := Run(args, 0)
	if err != nil {
		return "", err
	}
	pane, _ := resultMap(data)["pane"].(map[string]any)
	id := String(pane, "pane_id")
	if id == "" {
		return "", fmt.Errorf("herdr pane split returned no pane id")
	}
	return id, nil
}

func AgentStart(name, paneID, kind string, timeoutMS int, dirs []string, permissionMode, systemPromptFile string, allowedTools []string) error {
	args := []string{"agent", "start", name, "--kind", kind, "--pane", paneID, "--timeout", sysutil.Itoa(timeoutMS)}
	extra := []string{}
	for _, dir := range dirs {
		extra = append(extra, "--add-dir", dir)
	}
	if permissionMode != "" {
		extra = append(extra, "--permission-mode", permissionMode)
	}
	if len(allowedTools) > 0 {
		extra = append(extra, "--allowedTools", strings.Join(allowedTools, ","))
	}
	if systemPromptFile != "" {
		extra = append(extra, "--append-system-prompt-file", systemPromptFile)
	}
	if len(extra) > 0 {
		args = append(append(args, "--"), extra...)
	}
	deadline := time.Now().Add(30 * time.Second)
	for {
		_, err := Run(args, time.Duration(timeoutMS)*time.Millisecond+15*time.Second)
		if err == nil {
			return nil
		}
		if time.Now().After(deadline) || (!strings.Contains(err.Error(), "agent_pane_busy") && !strings.Contains(err.Error(), "not an available shell")) {
			return err
		}
		time.Sleep(300 * time.Millisecond)
	}
}

func AgentPrompt(target, text string, wait bool, timeoutS float64) error {
	args := []string{"agent", "prompt", target, text}
	timeout := 15 * time.Second
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

func PaneClose(id string) error { _, err := Run([]string{"pane", "close", id}, 0); return err }
func PaneRename(id, label string) error {
	_, err := Run([]string{"pane", "rename", id, label}, 0)
	return err
}
