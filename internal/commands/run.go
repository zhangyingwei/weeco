package commands

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"weeco/internal/herdr"
	"weeco/internal/store"
	"weeco/internal/sysutil"
)

var errNoRun = errors.New("no active run belongs to this pane; the lead starts a team with weeco start")

type runtimeContext struct {
	Pane      string
	Workspace string
	Tab       string
	Session   string
}

func currentRuntime() (runtimeContext, error) {
	ctx := runtimeContext{Pane: os.Getenv("HERDR_PANE_ID"), Workspace: os.Getenv("HERDR_WORKSPACE_ID")}
	if os.Getenv("HERDR_ENV") != "1" || ctx.Pane == "" || ctx.Workspace == "" {
		return ctx, fmt.Errorf("run from a Herdr agent pane (HERDR_PANE_ID and HERDR_WORKSPACE_ID are required)")
	}
	panes, err := herdr.List("pane", ctx.Workspace)
	if err != nil {
		return ctx, err
	}
	pane := herdr.ByPane(panes, ctx.Pane)
	if pane == nil || herdr.String(pane, "workspace_id") != ctx.Workspace {
		return ctx, fmt.Errorf("current Herdr pane is unavailable")
	}
	ctx.Tab, ctx.Session = herdr.String(pane, "tab_id"), herdr.Session(pane)
	if ctx.Tab == "" || ctx.Session == "" {
		return ctx, fmt.Errorf("current pane has no live agent session; start Claude in Herdr first")
	}
	return ctx, nil
}

func resolveRun(ctx runtimeContext, id string) (*store.Run, string, error) {
	var runs []*store.Run
	if id != "" {
		run, err := store.LoadRun(id)
		if err != nil {
			return nil, "", err
		}
		runs = []*store.Run{run}
	} else {
		var err error
		runs, err = store.Runs()
		if err != nil {
			return nil, "", err
		}
	}
	var found *store.Run
	alias := ""
	for _, run := range runs {
		if run.Closed && id == "" {
			continue
		}
		candidate := ""
		if ctx.Pane == run.LeadPane && ctx.Session == run.LeadSession {
			candidate = "lead"
		}
		for name, m := range run.Members {
			if m.State != "closed" && ctx.Pane == m.PaneID && ctx.Session == m.Session {
				candidate = name
			}
		}
		if candidate == "" || ctx.Workspace != run.Workspace || ctx.Tab != run.TabID {
			continue
		}
		if found != nil {
			return nil, "", fmt.Errorf("multiple runs match this pane; pass --run")
		}
		found, alias = run, candidate
	}
	if found == nil {
		return nil, "", errNoRun
	}
	return found, alias, nil
}

func requireLead(alias string) error {
	if alias != "lead" {
		return fmt.Errorf("only the lead pane may recruit, assign work or close members")
	}
	return nil
}

func readText(path string) ([]byte, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("a non-empty file path is required")
	}
	path = sysutil.ExpandHome(path)
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > 2*1024*1024 {
		return nil, fmt.Errorf("expected a regular text file up to 2 MiB: %s", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(string(data)) == "" {
		return nil, fmt.Errorf("file is empty: %s", path)
	}
	return data, nil
}

func memberDir(run *store.Run, m *store.Member) string {
	return filepath.Join(store.RunDir(run.ID), "members", m.Name)
}

func memberPrompt(run *store.Run, m *store.Member, task string) string {
	exe, err := os.Executable()
	if err != nil {
		exe = "weeco"
	}
	cli := "WEECO_HOME=" + shellQuote(store.Home()) + " " + shellQuote(exe)
	return fmt.Sprintf("weeco run %s; member %s; role: %s; turn: %d\nLead pane: %s\nWorkspace: %s\nRead your task: %s\nOriginal task: %s\n\nRead the original goal and sources in your task. Complete only authorized work. Submit a concise report with actual deliverables, checks, evidence and limitations using:\n%s report --run %s --turn %d --file <absolute-report-file>\nIf blocked, add --blocked and explain the missing input, impact and recommendation. For progress use:\n%s send lead --run %s --file <file>\nYou are a real Herdr agent; do not spawn subagents or recruit members. After reporting, wait for rework or closure.\n", run.ID, m.Name, m.Role, m.Turn, run.LeadPane, run.Dir, task, m.Task, cli, run.ID, m.Turn, cli, run.ID)
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }

func verifyMember(run *store.Run, m *store.Member, agents []map[string]any) error {
	a := herdr.ByPane(agents, m.PaneID)
	if a == nil {
		return fmt.Errorf("member %s is missing; inspect its state before start --replace", m.Name)
	}
	if herdr.Session(a) != m.Session || herdr.String(a, "tab_id") != run.TabID {
		return fmt.Errorf("member %s pane belongs to a different agent session", m.Name)
	}
	return nil
}

func printJSON(v any) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return
	}
	fmt.Println(string(b))
}

func runtimeFlags(args []string, values, bools []string, maxPositionals int) (*flagSet, error) {
	declared := map[string]bool{"run": true}
	for _, name := range values {
		declared[name] = true
	}
	fs, err := parseFlags(args, declared)
	if err != nil {
		return nil, err
	}
	allowed := map[string]bool{"help": true}
	for _, name := range bools {
		allowed[name] = true
	}
	for name := range fs.bools {
		if !allowed[name] {
			return nil, fmt.Errorf("unknown flag --%s", name)
		}
	}
	if len(fs.positionals) > maxPositionals {
		return nil, fmt.Errorf("unexpected arguments: %s", strings.Join(fs.positionals[maxPositionals:], " "))
	}
	return fs, nil
}
