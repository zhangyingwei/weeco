package commands

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"weeco/internal/config"
	"weeco/internal/store"
	"weeco/internal/sysutil"
)

func normalizeAgentKind(kind string) (string, error) {
	if kind == "" {
		return "claude", nil
	}
	switch kind {
	case "claude", "codex", "pi":
		return kind, nil
	default:
		return "", fmt.Errorf("--agent must be claude, codex or pi")
	}
}

// modelArgs maps the user-facing model/effort fields to each agent's native
// flags. The flags were verified against the installed CLIs; keep this table
// in one place so start and config check cannot drift apart.
func modelArgs(kind, model, effort string) ([]string, error) {
	if effort != "" {
		switch effort {
		case "low", "medium", "high", "none":
		default:
			return nil, fmt.Errorf("--effort must be low, medium, high or none")
		}
	}
	var args []string
	switch kind {
	case "claude":
		if model != "" {
			args = append(args, "--model", model)
		}
		if effort != "" && effort != "none" {
			args = append(args, "--effort", effort)
		}
	case "codex":
		if model != "" {
			args = append(args, "-m", model)
		}
		if effort != "" && effort != "none" {
			args = append(args, "-c", "model_reasoning_effort="+effort)
		}
	case "pi":
		if model != "" {
			args = append(args, "--model", model)
		}
		if effort != "" && effort != "none" {
			args = append(args, "--thinking", effort)
		}
	default:
		return nil, fmt.Errorf("--agent must be claude, codex or pi")
	}
	return args, nil
}

func memberNativeArgs(kind string, run *store.Run, promptPath, settingsPath, extensionPath, model, effort string) ([]string, error) {
	controlDir := store.RunDir(run.ID)
	// The runtime lock lives one level above the run directory.
	controlRoot := filepath.Dir(controlDir)
	var args []string
	switch kind {
	case "claude":
		args = []string{
			"--add-dir", controlDir,
			"--permission-mode", "bypassPermissions",
			"--append-system-prompt-file", promptPath,
			"--settings", settingsPath,
		}
	case "codex":
		args = []string{
			"--sandbox", "workspace-write",
			"--ask-for-approval", "never",
			// Avoid a startup update modal that Herdr reports as blocked.
			"-c", "check_for_update_on_startup=false",
			// Codex disables network access by default. Members need it to
			// reach the Herdr socket for report/send; writes stay sandboxed.
			"-c", "sandbox_workspace_write.network_access=true",
			"--add-dir", controlRoot,
		}
	case "pi":
		args = []string{"--approve", "--extension", extensionPath}
	default:
		return nil, fmt.Errorf("--agent must be claude, codex or pi")
	}
	extra, err := modelArgs(kind, model, effort)
	if err != nil {
		return nil, err
	}
	return append(args, extra...), nil
}

// MemberCandidate is the minimal information start needs for one attempt.
type MemberCandidate = config.Candidate

func writeClaudeGuardSettings(path, exe, runDir string) error {
	command := shellQuote(exe) + " guard --run-dir " + shellQuote(runDir)
	settings := map[string]any{
		"hooks": map[string]any{
			"PreToolUse": []any{
				map[string]any{
					"matcher": "Bash",
					"hooks": []any{
						map[string]any{"type": "command", "command": command},
					},
				},
			},
		},
	}
	return sysutil.WriteJSON(path, settings)
}

func writePIGuardExtension(path, exe, runDir string) error {
	exeJSON, _ := json.Marshal(exe)
	runDirJSON, _ := json.Marshal(runDir)
	script := fmt.Sprintf(`import { execFileSync } from "node:child_process";

const EXE = %s;
const RUN_DIR = %s;

export default function (pi) {
  pi.on("tool_call", async (event) => {
    if (event.toolName !== "bash") return;
    const command = event.input && event.input.command;
    if (!command) return;
    try {
      execFileSync(EXE, ["guard", "--run-dir", RUN_DIR], {
        input: JSON.stringify({ tool_name: "bash", tool_input: { command } }),
        encoding: "utf8",
        stdio: ["pipe", "pipe", "pipe"],
      });
    } catch (err) {
      const reason = err && err.stderr ? String(err.stderr).trim() : "weeco guard failed";
      return { block: true, reason: reason || "blocked by weeco guard" };
    }
  });
}
`, exeJSON, runDirJSON)
	return sysutil.AtomicWrite(path, []byte(script))
}
