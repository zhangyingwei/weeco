package commands

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"weeco/internal/assets"
	"weeco/internal/config"
	"weeco/internal/herdr"
	"weeco/internal/store"
	"weeco/internal/sysutil"
)

func cmdStart(args []string) int {
	fs, err := runtimeFlags(args, []string{"name", "role", "task", "agent", "level", "model", "dir", "team-pane"}, []string{"replace"}, 0)
	if err != nil {
		return fail(err)
	}
	if helpRequested(fs) {
		fmt.Println("usage: weeco start --name ALIAS --role TEXT --task FILE [--level LEVEL] [--model MODEL] [--agent claude|codex|pi] [--dir PATH] [--team-pane PANE] [--replace] [--run ID]")
		return 0
	}
	name, role := fs.value("name"), fs.value("role")
	if !store.ValidID(name) || name == "lead" || role == "" {
		return fail(fmt.Errorf("--name must be a lowercase alias other than lead; --role is required"))
	}
	agentFlag, levelFlag, modelFlag := fs.value("agent"), fs.value("level"), fs.value("model")

	artist := role == "artist" || role == "画师"
	if artist {
		role = "画师"
		if levelFlag != "" || modelFlag != "" {
			return fail(fmt.Errorf("--level and --model do not apply to the artist role; the artist is always Codex"))
		}
		if agentFlag != "" && agentFlag != "codex" {
			return fail(fmt.Errorf("the artist role must use --agent codex or no --agent"))
		}
	}
	if levelFlag != "" && !config.ValidLevel(levelFlag) {
		return fail(fmt.Errorf("--level must be senior, mid, junior or temp"))
	}
	if agentFlag != "" {
		if _, err := normalizeAgentKind(agentFlag); err != nil {
			return fail(err)
		}
	}
	var legacyAgent string
	if !artist && agentFlag != "" && levelFlag == "" && modelFlag == "" {
		legacyAgent, _ = normalizeAgentKind(agentFlag)
	}

	task, err := readText(fs.value("task"))
	if err != nil {
		return fail(err)
	}
	startDir, err := resolveStartDir(fs.value("dir"))
	if err != nil {
		return fail(err)
	}
	teamPaneFlag := fs.value("team-pane")
	teamWorkspace, teamTab, err := resolveTeamPane(teamPaneFlag)
	if err != nil {
		return fail(err)
	}
	ctx, err := currentRuntime()
	if err != nil {
		return fail(err)
	}

	err = store.RuntimeLock(func() error {
		run, alias, err := resolveRun(ctx, fs.value("run"))
		if errors.Is(err, errNoRun) && fs.value("run") == "" {
			// A retired member pane must not become a recursive team lead.
			runs, e := store.Runs()
			if e != nil {
				return e
			}
			for _, prior := range runs {
				for _, m := range prior.Members {
					if sameMember(ctx, m) {
						return fmt.Errorf("member panes cannot recruit teams")
					}
				}
			}
			id, e := store.NewID("r-")
			if e != nil {
				return e
			}
			dir := startDir
			if dir == "" {
				dir, e = os.Getwd()
				if e != nil {
					return e
				}
				dir, e = sysutil.AbsPath(dir)
				if e != nil {
					return e
				}
			}
			run = &store.Run{ID: id, LeadPane: ctx.Pane, LeadSession: ctx.Session, Workspace: ctx.Workspace, TabID: ctx.Tab, Dir: dir, TeamPane: teamPaneFlag, TeamWorkspace: teamWorkspace, TeamTab: teamTab, Members: map[string]*store.Member{}}
			alias, err = "lead", nil
		}
		if err != nil {
			return err
		}
		if teamPaneFlag != "" {
			if run.TeamPane == "" {
				return fmt.Errorf("run %s has no team pane; --team-pane cannot change an existing run", run.ID)
			}
			if run.TeamPane != teamPaneFlag {
				return fmt.Errorf("run %s uses team pane %s; --team-pane cannot change an existing run", run.ID, run.TeamPane)
			}
		}
		if startDir != "" {
			if run.Dir == "" {
				return fmt.Errorf("run %s has no directory; cannot use --dir", run.ID)
			}
			current, err := sysutil.AbsPath(run.Dir)
			if err != nil {
				return err
			}
			if current != startDir {
				return fmt.Errorf("run %s uses directory %s; --dir cannot change an existing run", run.ID, run.Dir)
			}
		}
		if err := requireLead(alias); err != nil {
			return err
		}
		if run.Closed {
			return fmt.Errorf("run %s is closed", run.ID)
		}
		agents, err := herdr.List("agent", "")
		if err != nil {
			return err
		}

		candidates, level, err := resolveStartCandidates(run, artist, legacyAgent, levelFlag, modelFlag, agentFlag)
		if err != nil {
			return err
		}

		old := run.Members[name]
		if old != nil && !fs.bool("replace") {
			if old.TaskSHA != sysutil.SHA256Bytes(task) || old.Role != role || !memberMatchesCandidates(old, candidates, level, artist) {
				return fmt.Errorf("member %s already exists with different work, role, level or model; use send to change its work or --replace to change its agent", name)
			}
			if old.State == "closed" || old.State == "failed" {
				return fmt.Errorf("member %s is %s; inspect it, then use --replace", name, old.State)
			}
			if err := verifyMember(run, old, agents); err != nil {
				return err
			}
			fmt.Printf("member: %s\nrun: %s\npane: %s\nexisting: yes\nlevel: %s\nmodel: %s\ndir: %s\nteam_pane: %s\nteam_workspace: %s\n", name, run.ID, old.PaneID, displayDash(old.Level), displayDash(old.Model), run.Dir, displayDash(runTeamPane(run)), displayDash(run.TeamWorkspace))
			return nil
		}
		if fs.bool("replace") && old == nil {
			return fmt.Errorf("cannot replace missing member %s", name)
		}
		if old != nil && old.PaneID != "" {
			if herdr.ByPane(agents, old.PaneID) != nil {
				return fmt.Errorf("member %s still has a live agent; close it before replacement", name)
			}
			teamWorkspace, _ := runTeamLocation(run)
			panes, e := herdr.List("pane", teamWorkspace)
			if e != nil {
				return e
			}
			if herdr.ByPane(panes, old.PaneID) != nil {
				return fmt.Errorf("old pane still exists; inspect and close it before replacement")
			}
		}
		m := &store.Member{Name: name, Kind: candidates[0].Agent, Role: role, TaskSHA: sysutil.SHA256Bytes(task), State: "starting", Turn: 1}
		if !artist && legacyAgent == "" {
			m.Level = level
		}
		if old != nil {
			m.Turn = old.Turn + 1
		}
		m.Task = filepath.Join(memberDir(run, m), fmt.Sprintf("task-%d.md", m.Turn))
		if err := sysutil.AtomicWrite(m.Task, task); err != nil {
			return err
		}
		prompt, err := assets.Read(assets.MemberPrompt)
		if err != nil {
			return err
		}
		promptPath := filepath.Join(memberDir(run, m), "member-prompt.md")
		if err := sysutil.AtomicWrite(promptPath, prompt); err != nil {
			return err
		}
		exe, err := os.Executable()
		if err != nil || exe == "" {
			exe = "weeco"
		}
		run.Members[name] = m
		if err := store.SaveRun(run); err != nil {
			return err
		}

		var failures []string
		for _, candidate := range candidates {
			attempt := store.Attempt{Agent: candidate.Agent, Model: candidate.Model, Effort: candidate.Effort, At: sysutil.NowISO()}
			stop, startErr := tryStartMember(run, m, candidate, prompt, promptPath, exe)
			if startErr != nil {
				attempt.Error = startErr.Error()
			}
			m.Attempts = append(m.Attempts, attempt)
			if startErr == nil {
				if err := store.SaveRun(run); err != nil {
					return err
				}
				fmt.Printf("run: %s\nmember: %s\nagent: %s\nkind: %s\nlevel: %s\nmodel: %s\neffort: %s\npane: %s\nturn: %d\nattempts: %d\ndir: %s\nteam_pane: %s\nteam_workspace: %s\ntask: %s\n",
					run.ID, m.Name, m.Agent, m.Kind, displayDash(m.Level), displayDash(m.Model), displayDash(m.Effort), m.PaneID, m.Turn, len(m.Attempts), run.Dir, displayDash(runTeamPane(run)), displayDash(run.TeamWorkspace), m.Task)
				return nil
			}
			m.LastError = startErr.Error()
			failures = append(failures, candidateLabel(candidate)+": "+startErr.Error())
			if stop {
				if m.State == "starting" {
					m.State = "failed"
				}
				if err := store.SaveRun(run); err != nil {
					return fmt.Errorf("%v; save failed: %w", startErr, err)
				}
				if m.PaneID != "" {
					return fmt.Errorf("%v; member %s pane kept for inspection: %s", startErr, m.Name, m.PaneID)
				}
				return startErr
			}
			if err := store.SaveRun(run); err != nil {
				return fmt.Errorf("%v; save failed: %w", startErr, err)
			}
		}
		m.State = "failed"
		m.PaneID, m.Session = "", ""
		m.LastError = "all candidates failed: " + strings.Join(failures, "; ")
		if err := store.SaveRun(run); err != nil {
			return fmt.Errorf("start failed: %s; save failed: %w", m.LastError, err)
		}
		return fmt.Errorf("start failed: %s", m.LastError)
	})
	if err != nil {
		return fail(err)
	}
	return 0
}

func resolveStartCandidates(run *store.Run, artist bool, legacyAgent, level, model, agent string) ([]config.Candidate, string, error) {
	if artist {
		return []config.Candidate{{Agent: "codex"}}, "", nil
	}
	if legacyAgent != "" {
		return []config.Candidate{{Agent: legacyAgent}}, "", nil
	}
	if level == "" {
		level = config.DefaultLevel
	}
	dir := run.Dir
	if dir == "" {
		var err error
		dir, err = os.Getwd()
		if err != nil {
			return nil, "", err
		}
	}
	cfg, err := config.Load(dir)
	if err != nil {
		return nil, "", err
	}
	candidates, err := cfg.Resolve(level, agent, model)
	if err != nil {
		return nil, "", err
	}
	return candidates, level, nil
}

func memberMatchesCandidates(old *store.Member, candidates []config.Candidate, level string, artist bool) bool {
	if artist {
		return old.Kind == "codex" && old.Role == "画师"
	}
	if old.Level != "" && old.Level != level {
		return false
	}
	for _, candidate := range candidates {
		if old.Kind == candidate.Agent && old.Model == candidate.Model && old.Effort == candidate.Effort {
			return true
		}
		// Runs created before levels existed have no level, model or effort.
		if old.Level == "" && old.Model == "" && old.Effort == "" && old.Kind == candidate.Agent {
			return true
		}
	}
	return false
}

// tryStartMember performs one candidate attempt. stop means the pane must be
// kept for inspection and no further candidate should be tried.
func tryStartMember(run *store.Run, m *store.Member, candidate config.Candidate, prompt []byte, promptPath, exe string) (stop bool, err error) {
	m.Kind, m.Model, m.Effort = candidate.Agent, candidate.Model, candidate.Effort
	if _, err := exec.LookPath(candidate.Agent); err != nil {
		return false, fmt.Errorf("%s is not on PATH", candidate.Agent)
	}

	settingsPath, extensionPath := "", ""
	switch candidate.Agent {
	case "claude":
		settingsPath = filepath.Join(memberDir(run, m), "claude-guard-settings.json")
		if err := writeClaudeGuardSettings(settingsPath, exe, run.Dir); err != nil {
			return true, err
		}
	case "pi":
		extensionPath = filepath.Join(memberDir(run, m), "pi-guard-extension.js")
		if err := writePIGuardExtension(extensionPath, exe, run.Dir); err != nil {
			return true, err
		}
	}
	nativeArgs, err := memberNativeArgs(candidate.Agent, run, promptPath, settingsPath, extensionPath, candidate.Model, candidate.Effort)
	if err != nil {
		return true, err
	}
	agentName, err := newAgentName()
	if err != nil {
		return true, err
	}
	m.Agent = agentName
	pane, err := herdr.PaneSplit(runTeamPane(run), "right", run.Dir, 0.5)
	if err != nil {
		return true, err
	}
	m.PaneID = pane
	if err := store.SaveRun(run); err != nil {
		return true, err
	}
	if err := herdr.AgentStart(m.Agent, pane, candidate.Agent, 60000, nativeArgs); err != nil {
		if stopStartError(err) {
			m.State = "failed"
			return true, err
		}
		if closeErr := closeMemberPane(m); closeErr != nil {
			return true, fmt.Errorf("%v; close pane: %w", err, closeErr)
		}
		return false, err
	}
	agents, err := herdr.List("agent", "")
	if err != nil {
		if closeErr := closeMemberPane(m); closeErr != nil {
			return true, fmt.Errorf("%v; close pane: %w", err, closeErr)
		}
		return false, err
	}
	a := herdr.ByPane(agents, pane)
	_, teamTab := runTeamLocation(run)
	if a == nil || herdr.String(a, "tab_id") != teamTab {
		if closeErr := closeMemberPane(m); closeErr != nil {
			return true, fmt.Errorf("agent start returned without a matching live session; close pane: %w", closeErr)
		}
		return false, fmt.Errorf("agent start returned without a matching live session")
	}
	session, agentNameLive := herdr.Session(a), herdr.String(a, "name")
	if agentNameLive != "" && agentNameLive != m.Agent {
		if closeErr := closeMemberPane(m); closeErr != nil {
			return true, fmt.Errorf("agent start returned a different agent name; close pane: %w", closeErr)
		}
		return false, fmt.Errorf("agent start returned a different agent name")
	}
	if session == "" && (candidate.Agent == "claude" || agentNameLive == "") {
		if closeErr := closeMemberPane(m); closeErr != nil {
			return true, fmt.Errorf("agent start returned without a matching live session; close pane: %w", closeErr)
		}
		return false, fmt.Errorf("agent start returned without a matching live session")
	}
	m.Session, m.State = session, "running"
	if err := store.SaveRun(run); err != nil {
		return true, err
	}

	promptText := memberPrompt(run, m, m.Task)
	if candidate.Agent != "claude" {
		promptText = string(prompt) + "\n\n" + promptText
	}
	if err := herdr.AgentPrompt(pane, promptText, false, 0); err != nil {
		m.State = "failed"
		m.LastError = err.Error()
		var commandErr *herdr.CommandError
		if errors.As(err, &commandErr) && commandErr.Code == "agent_blocked" {
			return true, err
		}
		if closeErr := closeMemberPane(m); closeErr != nil {
			return true, fmt.Errorf("%v; close pane: %w", err, closeErr)
		}
		return true, err
	}
	if err := herdr.PaneRename(pane, m.Name+" ("+m.Role+")"); err != nil {
		fmt.Printf("warning: rename: %v\n", err)
	}
	// Keep the member for inspection if the task did not visibly start.
	if err := herdr.AgentWait(pane, []string{"working"}, 15*time.Second); err != nil {
		m.State = "running"
		m.LastError = "task delivery not confirmed: " + err.Error()
		return true, fmt.Errorf("task delivery not confirmed: %v", err)
	}
	m.LastError = ""
	return false, nil
}

func stopStartError(err error) bool {
	message := err.Error()
	for _, marker := range []string{"agent_not_ready", "blocked during startup", "agent_pane_busy", "not an available shell"} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}

func closeMemberPane(m *store.Member) error {
	if m.PaneID == "" {
		return nil
	}
	if err := herdr.PaneClose(m.PaneID); err != nil {
		return err
	}
	m.PaneID, m.Session = "", ""
	return nil
}

func newAgentName() (string, error) {
	suffix, err := store.NewID("m-")
	if err != nil {
		return "", err
	}
	return "weeco-" + suffix[2:], nil
}

func resolveStartDir(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	if !filepath.IsAbs(sysutil.ExpandHome(value)) {
		return "", fmt.Errorf("--dir must be an absolute path")
	}
	dir, err := sysutil.AbsPath(value)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(dir)
	if err != nil {
		return "", fmt.Errorf("--dir: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("--dir must be a directory: %s", dir)
	}
	return dir, nil
}

func candidateLabel(candidate config.Candidate) string {
	model := candidate.Model
	if model == "" {
		model = "<CLI default>"
	}
	return candidate.Agent + "/" + model
}

func displayDash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}
