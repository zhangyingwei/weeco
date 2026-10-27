package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"weeco/internal/assets"
	"weeco/internal/herdr"
	"weeco/internal/model"
	"weeco/internal/store"
	"weeco/internal/sysutil"
	"weeco/internal/workspace"
)

var teamIDRe = regexp.MustCompile(`^T-\d{8}-\d{2,3}$`)

func cmdTeam(args []string) int {
	if len(args) == 0 {
		fmt.Println("usage: weeco team <recruit|list|status|wait|nudge|repair|check|accept|rework|answer|disband> [flags]")
		return 1
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "recruit":
		return cmdTeamRecruit(rest)
	case "list":
		return cmdTeamList(rest)
	case "status":
		return cmdTeamStatus(rest)
	case "wait":
		return cmdTeamWait(rest)
	case "nudge":
		return cmdTeamNudge(rest)
	case "repair":
		return cmdTeamRepair(rest)
	case "check":
		return cmdTeamCheck(rest)
	case "accept":
		return cmdTeamAccept(rest)
	case "rework":
		return cmdTeamRework(rest)
	case "answer":
		return cmdTeamAnswer(rest)
	case "disband":
		return cmdTeamDisband(rest)
	case "-h", "--help":
		fmt.Println("usage: weeco team <recruit|list|status|wait|nudge|repair|check|accept|rework|answer|disband> [flags]")
		return 0
	default:
		return fail(fmt.Errorf("unknown team command: %s", sub))
	}
}

func cmdTeamRecruit(args []string) int {
	fs, err := parseFlags(args, map[string]bool{
		"project": true, "requirement": true, "purpose": true, "contract": true,
		"acceptance": true, "size": true, "id": true,
	})
	if err != nil {
		return fail(err)
	}
	if helpRequested(fs) {
		fmt.Println("usage: weeco team recruit --requirement R-... --contract FILE --size N [--acceptance FILE] [--purpose NAME] [--id T-...] [--allow-root]")
		return 0
	}
	reqID := fs.value("requirement")
	if reqID == "" {
		return fail(fmt.Errorf("--requirement is required"))
	}
	project, err := resolveProject(fs)
	if err != nil {
		return fail(err)
	}
	req, err := store.LoadRequirement(project.Key, reqID)
	if err != nil {
		return fail(err)
	}
	if !req.IsOpen() {
		return fail(fmt.Errorf("requirement %s is %s", reqID, req.State))
	}
	if open, ok, err := store.OpenTeamForRequirement(project.Key, reqID); err != nil {
		return fail(err)
	} else if ok {
		return fail(fmt.Errorf("requirement already has open team %s (%s); accept/disband it first", open.ID, open.State))
	}
	contractSource := fs.value("contract")
	if contractSource == "" {
		return fail(fmt.Errorf("--contract is required"))
	}
	contractData, err := os.ReadFile(sysutil.ExpandHome(contractSource))
	if err != nil {
		return fail(err)
	}
	if strings.TrimSpace(string(contractData)) == "" {
		return fail(fmt.Errorf("contract is empty: %s", contractSource))
	}
	var acceptanceData []byte
	if acceptanceSource := fs.value("acceptance"); acceptanceSource != "" {
		data, err := os.ReadFile(sysutil.ExpandHome(acceptanceSource))
		if err != nil {
			return fail(err)
		}
		var parsed acceptanceFile
		if err := json.Unmarshal(data, &parsed); err != nil {
			return fail(fmt.Errorf("acceptance file must be a JSON object: %v", err))
		}
		acceptanceData = data
	}
	size, err := parsePositiveInt(fs.value("size"), "size")
	if err != nil {
		return fail(err)
	}
	purpose := fs.value("purpose")
	if purpose == "" {
		purpose = "work"
	}
	workspacePath := req.Workspace
	if workspacePath == "" {
		if (purpose == "plan" || purpose == "implement") && workspace.IsGit(project.Root) && !fs.bool("allow-root") {
			return fail(fmt.Errorf("requirement %s has no workspace; run weeco workspace create --requirement %s first (or pass --allow-root)", reqID, reqID))
		}
		workspacePath = project.Root
	}
	if info, err := os.Stat(workspacePath); err != nil || !info.IsDir() {
		if err == nil {
			err = fmt.Errorf("not a directory")
		}
		return fail(fmt.Errorf("workspace is not usable: %s (%v); run weeco workspace create --requirement %s", workspacePath, err, reqID))
	}
	herdrWorkspace, err := herdr.RequireWorkspace()
	if err != nil {
		return fail(err)
	}
	trustRoot, trusted, err := claudeTrust(workspacePath)
	if err != nil {
		return fail(fmt.Errorf("check Claude workspace trust: %w", err))
	}
	if !trusted {
		return fail(fmt.Errorf("workspace is not trusted by Claude Code: %s\nrun: weeco trust %s\nor start claude in that directory once and accept the trust dialog", workspacePath, trustRoot))
	}
	var teamID, teamDir, contractPath, contractSHA string
	err = store.WithLock(project.Key, func() error {
		teamID = fs.value("id")
		if teamID == "" {
			id, err := store.NextID(project.Key, "T")
			if err != nil {
				return err
			}
			teamID = id
		}
		if !teamIDRe.MatchString(teamID) {
			return fmt.Errorf("invalid team id: %s (expected T-YYYYMMDD-NN)", teamID)
		}
		if _, err := store.LoadTeam(project.Key, teamID); err == nil {
			return fmt.Errorf("team already exists: %s", teamID)
		}
		if open, ok, err := store.OpenTeamForRequirement(project.Key, reqID); err != nil {
			return err
		} else if ok {
			return fmt.Errorf("requirement already has open team %s (%s); accept/disband it first", open.ID, open.State)
		}
		teamDir = store.TeamDir(project.Key, teamID)
		if err := os.MkdirAll(filepath.Join(teamDir, "logs"), 0o755); err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Join(teamDir, "members"), 0o755); err != nil {
			return err
		}
		contractPath = filepath.Join(teamDir, "contract.md")
		if err := os.WriteFile(contractPath, contractData, 0o644); err != nil {
			return err
		}
		if len(acceptanceData) > 0 {
			if err := os.WriteFile(filepath.Join(teamDir, "acceptance.json"), acceptanceData, 0o644); err != nil {
				return err
			}
		}
		contractSHA = sysutil.SHA256Bytes(contractData)
		return store.Append(project.Key, "team", teamID, "team_created", "boss", map[string]any{
			"requirement_id": reqID,
			"purpose":        purpose,
			"contract_path":  contractPath,
			"contract_sha":   contractSHA,
			"workspace":      workspacePath,
			"size":           size,
		})
	})
	if err != nil {
		return fail(err)
	}
	leadPrompt, err := assets.Read(assets.TeamLeadPrompt)
	if err != nil {
		_ = store.WithLock(project.Key, func() error {
			return store.Append(project.Key, "team", teamID, "team_start_failed", "system", map[string]any{"reason": err.Error()})
		})
		return fail(err)
	}
	// Herdr rejects agent start arguments containing control characters, so a
	// multi-line system prompt must go through a file. The team directory is
	// also passed with --add-dir, so the lead can read it.
	leadPromptPath := filepath.Join(teamDir, "lead-system-prompt.md")
	if err := os.WriteFile(leadPromptPath, leadPrompt, 0o644); err != nil {
		_ = store.WithLock(project.Key, func() error {
			return store.Append(project.Key, "team", teamID, "team_start_failed", "system", map[string]any{"reason": "write lead prompt: " + err.Error()})
		})
		return fail(err)
	}
	tabID, paneID, err := herdr.TabCreate(herdrWorkspace, workspacePath, "team/"+teamID)
	if err != nil {
		_ = store.WithLock(project.Key, func() error {
			return store.Append(project.Key, "team", teamID, "team_start_failed", "system", map[string]any{"reason": err.Error()})
		})
		return fail(err)
	}
	leadAgent := model.LeadAgentName(project.Key, teamID)
	cfg := store.LoadConfig()
	if err := herdr.AgentStart(leadAgent, paneID, cfg.AgentKind, cfg.ReadyTimeoutMS, []string{teamDir}, cfg.TeamLeadPermissionMode, leadPromptPath); err != nil {
		_ = store.WithLock(project.Key, func() error {
			return store.Append(project.Key, "team", teamID, "team_start_failed", "system", map[string]any{"reason": err.Error()})
		})
		_ = closeTeamPane(model.Team{PaneID: paneID, TabID: tabID})
		return fail(err)
	}
	readyTimeout := float64(cfg.ReadyTimeoutMS) / 1000.0
	if readyTimeout < 30 {
		readyTimeout = 30
	}
	if err := herdr.AgentWait(paneID, readyTimeout, ""); err != nil {
		_ = store.WithLock(project.Key, func() error {
			return store.Append(project.Key, "team", teamID, "team_start_failed", "system", map[string]any{"reason": err.Error()})
		})
		_ = closeTeamPane(model.Team{PaneID: paneID, TabID: tabID})
		return fail(err)
	}
	if err := store.WithLock(project.Key, func() error {
		return store.Append(project.Key, "team", teamID, "team_started", "system", map[string]any{
			"pane_id":      paneID,
			"tab_id":       tabID,
			"workspace_id": herdrWorkspace,
			"lead_agent":   leadAgent,
		})
	}); err != nil {
		_ = closeTeamPane(model.Team{PaneID: paneID, TabID: tabID})
		return fail(err)
	}
	if err := store.WritePaneContext(store.PaneContext{
		ProjectRoot: project.Root,
		ProjectKey:  project.Key,
		TeamID:      teamID,
		PaneID:      paneID,
		LeadAgent:   leadAgent,
	}); err != nil {
		fmt.Printf("warning: could not write pane context: %v\n", err)
	}
	initial := fmt.Sprintf(`You are the lead of team %s.

Project root: %s
Team ID: %s
Requirement: %s
Purpose: %s
Contract: %s
Team control dir: %s
Workspace: %s
Team size: %d members (excluding you)

Read the contract and the weeco-team-lead skill. You and every employee are Herdr agents in this team's tab/panel; add employees with weeco member recruit, not the Agent/Task tool. Every member, including you, must have a formal name and an ASCII agent name. Decide the roster before recruiting; write each member's task file; use those names in assignments, report paths, and result.md. Do not use placeholder names. Recruit %d members, split the work, assign clear scopes, wait for them with weeco member wait, read their reports, and accept or rework them with weeco member accept/rework.

From this team pane, weeco resolves the project and team from HERDR_PANE_ID:
  weeco member recruit --name "..." --agent ascii-name --role "..." --task <task-file>
  weeco lead status
  weeco lead submit
  weeco lead ask --question "..."
If auto-context is unavailable, use the fallback:
  weeco lead submit --project %s --team %s
  weeco lead ask --project %s --team %s --question "..."

When the team deliverable meets the contract, write %s and run: weeco lead submit.
Do not edit the contract. Do not accept the team yourself; the boss accepts.
`, teamID, project.Root, teamID, reqID, purpose, contractPath, teamDir, workspacePath, size,
		size, project.Root, teamID, project.Root, teamID, filepath.Join(teamDir, "result.md"))
	if err := herdr.AgentPrompt(paneID, initial, false, 0); err != nil {
		_ = store.WithLock(project.Key, func() error {
			return store.Append(project.Key, "team", teamID, "team_lost", "system", map[string]any{"reason": "initial prompt: " + err.Error()})
		})
		_ = store.DeletePaneContext(paneID)
		_ = closeTeamPane(model.Team{PaneID: paneID, TabID: tabID})
		return fail(err)
	}
	if err := store.WithLock(project.Key, func() error {
		return store.Append(project.Key, "team", teamID, "team_prompted", "system", nil)
	}); err != nil {
		return fail(err)
	}
	fmt.Printf("team:      %s\nrequirement: %s\nlead:      %s\npane:      %s\ntab:       %s\nworkspace: %s\ncontract:  %s\n",
		teamID, reqID, leadAgent, paneID, tabID, workspacePath, contractPath)
	fmt.Printf("next:      weeco team wait %s --timeout 1800  (run in the background)\n", teamID)
	return 0
}

func cmdTeamList(args []string) int {
	fs, err := parseFlags(args, map[string]bool{"project": true})
	if err != nil {
		return fail(err)
	}
	if helpRequested(fs) {
		fmt.Println("usage: weeco team list [--json]")
		return 0
	}
	project, err := resolveProject(fs)
	if err != nil {
		return fail(err)
	}
	ids, err := store.ListTeamIDs(project.Key)
	if err != nil {
		return fail(err)
	}
	events, err := store.ReadEvents(project.Key)
	if err != nil {
		return fail(err)
	}
	var teams []model.Team
	for _, id := range ids {
		team, ok := store.FoldTeam(events, id)
		if ok {
			teams = append(teams, team)
		}
	}
	if fs.bool("json") {
		data, _ := json.MarshalIndent(teams, "", "  ")
		fmt.Println(string(data))
		return 0
	}
	if len(teams) == 0 {
		fmt.Println("no teams")
		return 0
	}
	fmt.Printf("%-16s %-14s %-12s %-10s %-4s %-10s %s\n", "TEAM", "REQUIREMENT", "STATE", "PURPOSE", "SIZE", "PANE", "UPDATED")
	for _, team := range teams {
		printTeamLine(team)
	}
	return 0
}

func cmdTeamStatus(args []string) int {
	fs, err := parseFlags(args, map[string]bool{"project": true})
	if err != nil {
		return fail(err)
	}
	if helpRequested(fs) {
		fmt.Println("usage: weeco team status T-... [--json]")
		return 0
	}
	teamID, err := fs.positional(0, "team")
	if err != nil {
		return fail(err)
	}
	project, team, err := resolveTeam(fs, teamID)
	if err != nil {
		return fail(err)
	}
	members, err := store.MembersForTeam(project.Key, team.ID)
	if err != nil {
		return fail(err)
	}
	memberViews := buildMemberViews(members)
	live := ""
	if team.LeadAgent != "" {
		live = herdr.AgentStatusFor(team.PaneID, team.LeadAgent)
		if live == "" {
			live = "missing"
		}
	}
	checkPath := filepath.Join(store.TeamDir(project.Key, teamID), "check.json")
	var check *checkResult
	var parsed checkResult
	if ok, _ := sysutil.ReadJSONOr(checkPath, &parsed); ok {
		check = &parsed
	}
	if fs.bool("json") {
		data, _ := json.MarshalIndent(map[string]any{"team": team, "live_status": live, "check": check, "members": memberViews}, "", "  ")
		fmt.Println(string(data))
		return 0
	}
	fmt.Printf("team:        %s\nstate:       %s\nrequirement: %s\npurpose:     %s\nsize:        %d\nworkspace:   %s\nlead:        %s\nlive:        %s\ncontract:    %s\nresult:      %s\n",
		team.ID, team.State, team.RequirementID, team.Purpose, team.Size, orDash(team.Workspace),
		orDash(team.LeadAgent), orDash(live), orDash(team.ContractPath), orDash(team.ResultPath))
	if len(memberViews) > 0 {
		fmt.Printf("members:     %d\n", len(memberViews))
		fmt.Printf("%-18s %-14s %-10s %-10s %-10s %s\n", "AGENT", "NAME", "STATE", "LIVE", "PANE", "REPORT")
		for _, v := range memberViews {
			fmt.Printf("%-18s %-14s %-10s %-10s %-10s %s\n",
				v.AgentName, v.Name, v.State, v.Live, orDash(v.PaneID), v.Report)
		}
	} else {
		fmt.Println("members:     0 (no member panes; legacy subagent team or none recruited yet)")
	}
	if team.ReworkCount > 0 {
		fmt.Printf("rework:      %d\n", team.ReworkCount)
	}
	if team.BlockedQuestion != "" {
		fmt.Printf("question:    %s\n", team.BlockedQuestion)
	}
	if team.LastIssues != "" {
		fmt.Printf("issues:      %s\n", team.LastIssues)
	}
	if check != nil {
		fmt.Println(checkSummary(*check))
	}
	if team.State == model.TeamSubmitted {
		fmt.Println("next:        weeco team check/accept/rework")
	}
	if team.State == model.TeamBlocked {
		fmt.Println("next:        weeco team answer")
	}
	return 0
}

func cmdTeamWait(args []string) int {
	fs, err := parseFlags(args, map[string]bool{"project": true, "timeout": true})
	if err != nil {
		return fail(err)
	}
	if helpRequested(fs) {
		fmt.Println("usage: weeco team wait T-... [--timeout SECONDS]")
		fmt.Println("waits until the team is submitted/blocked/lost, or the timeout expires; run it in the background")
		return 0
	}
	teamID, err := fs.positional(0, "team")
	if err != nil {
		return fail(err)
	}
	project, err := resolveProject(fs)
	if err != nil {
		return fail(err)
	}
	return waitForTeam(project, teamID, parseTimeout(fs.value("timeout"), store.LoadConfig().WaitTimeoutS))
}

func waitForTeam(project model.Project, teamID string, timeoutS float64) int {
	if timeoutS <= 0 {
		timeoutS = store.LoadConfig().WaitTimeoutS
	}
	if timeoutS <= 0 {
		timeoutS = 1800
	}
	deadline := time.Now().Add(time.Duration(timeoutS * float64(time.Second)))
	for {
		team, err := store.LoadTeam(project.Key, teamID)
		if err != nil {
			return fail(err)
		}
		switch team.State {
		case model.TeamSubmitted:
			fmt.Println("state: submitted")
			fmt.Println("next:  weeco team status/check/accept/rework")
			return 0
		case model.TeamBlocked:
			fmt.Println("state: blocked")
			fmt.Printf("question: %s\n", team.BlockedQuestion)
			fmt.Println("next:  weeco team answer")
			return 0
		case model.TeamAccepted, model.TeamDisbanded:
			fmt.Printf("state: %s\n", team.State)
			return 0
		case model.TeamLost:
			fmt.Println("state: lost")
			fmt.Printf("reason: %s\n", team.LastIssues)
			return 0
		}
		if team.LeadAgent == "" {
			return fail(fmt.Errorf("team %s has no lead agent", teamID))
		}
		live := herdr.AgentStatusFor(team.PaneID, team.LeadAgent)
		if live == "blocked" {
			fmt.Printf("state: %s (live status: blocked)\n", team.State)
			fmt.Println("inspect the team pane; the lead may be waiting on a UI question")
			return 0
		}
		if live == "" && herdr.AgentFor(team.PaneID, team.LeadAgent) == nil {
			_ = store.WithLock(project.Key, func() error {
				return store.Append(project.Key, "team", teamID, "team_lost", "system", map[string]any{"reason": "lead agent is missing"})
			})
			fmt.Println("state: lost (lead agent is missing)")
			return 0
		}
		if time.Now().After(deadline) {
			fmt.Printf("state: %s (timeout after %.0fs; still active)\n", team.State, timeoutS)
			fmt.Println("inspect the team pane; start another wait or nudge")
			return 0
		}
		time.Sleep(2 * time.Second)
	}
}

func cmdTeamNudge(args []string) int {
	fs, err := parseFlags(args, map[string]bool{"project": true})
	if err != nil {
		return fail(err)
	}
	if helpRequested(fs) {
		fmt.Println("usage: weeco team nudge T-...")
		return 0
	}
	teamID, err := fs.positional(0, "team")
	if err != nil {
		return fail(err)
	}
	project, team, err := resolveTeam(fs, teamID)
	if err != nil {
		return fail(err)
	}
	switch team.State {
	case model.TeamSubmitted, model.TeamAccepted, model.TeamDisbanded:
		fmt.Printf("no nudge needed: team is %s\n", team.State)
		return 0
	case model.TeamBlocked:
		fmt.Println("team is blocked; use: weeco team answer " + team.ID + " --decision \"...\"")
		return 0
	case model.TeamLost:
		fmt.Println("team is lost; use: weeco team repair " + team.ID)
		return 0
	}
	if team.LeadAgent == "" {
		return fail(fmt.Errorf("team %s has no lead agent; run weeco team repair %s", team.ID, team.ID))
	}
	prompt := "BOSS NUDGE: report status now. If employees still need review, run `weeco member wait --all`, read their reports, and run `weeco member accept/rework`. If the team deliverable meets the contract, write result.md and run `weeco lead submit`. If you are blocked, run `weeco lead ask --question \"...\"`. Do not go idle without submit or ask."
	if err := herdr.AgentPrompt(herdr.Target(team.PaneID, team.LeadAgent), prompt, false, 0); err != nil {
		return fail(fmt.Errorf("could not nudge lead: %v", err))
	}
	if err := store.WithLock(project.Key, func() error {
		return store.Append(project.Key, "team", team.ID, "team_nudged", "boss", nil)
	}); err != nil {
		return fail(err)
	}
	fmt.Printf("nudged: %s (%s)\n", team.ID, team.LeadAgent)
	return 0
}

func cmdTeamRepair(args []string) int {
	fs, err := parseFlags(args, map[string]bool{"project": true})
	if err != nil {
		return fail(err)
	}
	if helpRequested(fs) {
		fmt.Println("usage: weeco team repair T-...")
		return 0
	}
	teamID, err := fs.positional(0, "team")
	if err != nil {
		return fail(err)
	}
	project, team, err := resolveTeam(fs, teamID)
	if err != nil {
		return fail(err)
	}
	teamDir := store.TeamDir(project.Key, team.ID)
	resultPath := filepath.Join(teamDir, "result.md")
	checkPath := filepath.Join(teamDir, "check.json")
	acceptancePath := filepath.Join(teamDir, "acceptance.json")
	live := map[string]any(nil)
	liveStatus := "missing"
	if team.LeadAgent != "" {
		live = herdr.AgentFor(team.PaneID, team.LeadAgent)
		if live != nil {
			liveStatus = "present"
			if status := asStringAny(live["agent_status"]); status != "" {
				liveStatus += " (" + status + ")"
			}
		}
	}
	fmt.Printf("team:        %s\nstate:       %s\nlead:        %s\npane:        %s\nresult:      %s\nacceptance:  %s\ncheck:       %s\n",
		team.ID, team.State, liveStatus, orDash(team.PaneID),
		presence(resultPath), presence(acceptancePath), presence(checkPath))
	members, _ := store.MembersForTeam(project.Key, team.ID)
	for _, m := range members {
		liveMember := memberLiveStatus(m)
		fmt.Printf("member:      %s (%s) state=%s live=%s pane=%s report=%s\n",
			m.Name, m.AgentName, m.State, liveMember, orDash(m.PaneID), presence(m.ReportPath))
		if m.NeedsAttention() && liveMember == "missing" {
			_ = store.WithLock(project.Key, func() error {
				return store.Append(project.Key, "member", m.Key(), "member_lost", "boss", map[string]any{
					"reason": "repair: member agent is missing",
				})
			})
		}
	}
	if team.State != model.TeamAccepted && team.State != model.TeamDisbanded && live == nil {
		if err := store.WithLock(project.Key, func() error {
			return store.Append(project.Key, "team", team.ID, "team_lost", "boss", map[string]any{
				"reason": "repair: lead agent is missing",
			})
		}); err != nil {
			return fail(err)
		}
		fmt.Println("state:       lost")
		fmt.Println("actions:")
		fmt.Printf("  weeco team disband %s --force\n", team.ID)
		fmt.Println("  # then recruit a new team with the same contract")
		return 0
	}
	if team.State == model.TeamSubmitted && !sysutil.FileExists(resultPath) {
		fmt.Printf("warning: team is submitted but result.md is missing: %s\n", resultPath)
	}
	if sysutil.FileExists(acceptancePath) && !sysutil.FileExists(checkPath) {
		fmt.Println("warning: acceptance.json exists but check.json is missing; run: weeco team check " + team.ID)
	}
	if err := store.WithLock(project.Key, func() error {
		return store.Append(project.Key, "team", team.ID, "team_repaired", "boss", nil)
	}); err != nil {
		return fail(err)
	}
	return 0
}

func presence(path string) string {
	if sysutil.FileExists(path) {
		return "present"
	}
	return "missing"
}

func asStringAny(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func writeAcceptanceRecord(project model.Project, team model.Team, note, lessons string) (string, error) {
	teamDir := store.TeamDir(project.Key, team.ID)
	checkPath := filepath.Join(teamDir, "check.json")
	path := filepath.Join(teamDir, "acceptance.md")
	content := fmt.Sprintf(`# Boss acceptance

- team: %s
- requirement: %s
- accepted_at: %s
- result: %s
- result_sha: %s
- check: %s
- lessons: %s
- note: %s

## Note

%s
`, team.ID, team.RequirementID, sysutil.NowISO(), orDash(team.ResultPath), orDash(team.ResultSHA), presence(checkPath), lessons, note, note)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("write acceptance record: %w", err)
	}
	return path, nil
}

func cmdTeamAccept(args []string) int {
	fs, err := parseFlags(args, map[string]bool{"project": true, "note": true})
	if err != nil {
		return fail(err)
	}
	if helpRequested(fs) {
		fmt.Println("usage: weeco team accept T-... --note \"what you checked\" [--lessons-updated|--no-lessons] [--force] [--keep]")
		return 0
	}
	teamID, err := fs.positional(0, "team")
	if err != nil {
		return fail(err)
	}
	note := strings.TrimSpace(fs.value("note"))
	if note == "" {
		return fail(fmt.Errorf("--note is required; record what you checked"))
	}
	project, team, err := resolveTeam(fs, teamID)
	if err != nil {
		return fail(err)
	}
	if team.State == model.TeamAccepted || team.State == model.TeamDisbanded {
		fmt.Printf("already accepted: %s (%s)\n", team.ID, team.State)
		return 0
	}
	if !team.CanAccept() {
		return fail(fmt.Errorf("team %s is %s; expected submitted", teamID, team.State))
	}
	if !fs.bool("force") {
		pending, err := pendingMembers(project, team)
		if err != nil {
			return fail(err)
		}
		if len(pending) > 0 {
			return fail(fmt.Errorf("members still need lead acceptance: %s; run weeco member accept/close from the team lead pane, or --force", pendingMemberNames(pending)))
		}
	}
	if team.ResultPath != "" {
		digest, err := sysutil.SHA256File(team.ResultPath)
		if err != nil && !fs.bool("force") {
			return fail(fmt.Errorf("cannot read submitted result %s: %v (use --force to override)", team.ResultPath, err))
		}
		if err == nil && team.ResultSHA != "" && digest != team.ResultSHA && !fs.bool("force") {
			return fail(fmt.Errorf("result.md changed after submit; ask the lead to submit again, or use --force"))
		}
	}
	lessons := "missing"
	if fs.bool("lessons-updated") {
		lessons = "updated"
	} else if fs.bool("no-lessons") {
		lessons = "none"
	} else {
		fmt.Println("warning: lessons not recorded; pass --lessons-updated or --no-lessons")
	}
	if err := ensureChecksPassed(project, team, fs.bool("force")); err != nil {
		return fail(err)
	}
	acceptancePath, err := writeAcceptanceRecord(project, team, note, lessons)
	if err != nil {
		return fail(err)
	}
	if err := store.WithLock(project.Key, func() error {
		return store.Append(project.Key, "team", teamID, "team_accepted", "boss", map[string]any{
			"note":            note,
			"result_path":     team.ResultPath,
			"acceptance_path": acceptancePath,
			"lessons":         lessons,
		})
	}); err != nil {
		return fail(err)
	}
	fmt.Printf("accepted: %s\nby:       boss\n", teamID)
	if fs.bool("keep") {
		fmt.Println("pane kept (--keep); run weeco team disband when done")
		return 0
	}
	if err := closeTeamWithMembers(project, team); err != nil {
		fmt.Printf("warning: could not close team pane: %v\n", err)
		fmt.Println("team is accepted; run weeco team disband to retry the close")
		return 0
	}
	_ = store.DeletePaneContext(team.PaneID)
	if err := store.WithLock(project.Key, func() error {
		return store.Append(project.Key, "team", teamID, "team_disbanded", "boss", map[string]any{"reason": "accepted"})
	}); err != nil {
		return fail(err)
	}
	fmt.Println("disbanded: yes (worktree/workspace left in place)")
	return 0
}

func cmdTeamRework(args []string) int {
	fs, err := parseFlags(args, map[string]bool{"project": true, "issues": true})
	if err != nil {
		return fail(err)
	}
	if helpRequested(fs) {
		fmt.Println("usage: weeco team rework T-... --issues \"specific problems\"")
		return 0
	}
	teamID, err := fs.positional(0, "team")
	if err != nil {
		return fail(err)
	}
	issues := strings.TrimSpace(fs.value("issues"))
	if issues == "" {
		return fail(fmt.Errorf("--issues is required"))
	}
	project, team, err := resolveTeam(fs, teamID)
	if err != nil {
		return fail(err)
	}
	if !team.CanRework() {
		return fail(fmt.Errorf("team %s is %s; expected submitted", teamID, team.State))
	}
	n := team.ReworkCount + 1
	reviewPath := filepath.Join(store.TeamDir(project.Key, teamID), fmt.Sprintf("review-%d.md", n))
	content := fmt.Sprintf("# Boss review %d\n\nTeam: %s\n\n## Issues\n\n%s\n", n, teamID, issues)
	if err := os.WriteFile(reviewPath, []byte(content), 0o644); err != nil {
		return fail(err)
	}
	// The previous check was for the previous submission. Force a fresh check
	// after the team reworks so acceptance never relies on stale evidence.
	_ = os.Remove(filepath.Join(store.TeamDir(project.Key, teamID), "check.json"))
	if err := store.WithLock(project.Key, func() error {
		return store.Append(project.Key, "team", teamID, "team_rework", "boss", map[string]any{
			"issues":      issues,
			"review_path": reviewPath,
			"review_no":   n,
		})
	}); err != nil {
		return fail(err)
	}
	prompt := fmt.Sprintf("BOSS REVIEW: rework required.\n\nIssues:\n%s\n\nReview file: %s\n\nRead the review, coordinate fixes with the team, update %s, then run: weeco lead submit --project %s --team %s",
		issues, reviewPath, filepath.Join(store.TeamDir(project.Key, teamID), "result.md"), project.Root, teamID)
	if err := herdr.AgentPrompt(herdr.Target(team.PaneID, team.LeadAgent), prompt, false, 0); err != nil {
		fmt.Printf("warning: could not prompt lead: %v\n", err)
		fmt.Printf("prompt it in pane %s, or inspect the team with weeco team status\n", team.PaneID)
	}
	fmt.Printf("rework: %s (round %d)\nissues: %s\n", teamID, n, issues)
	return 0
}

func cmdTeamAnswer(args []string) int {
	fs, err := parseFlags(args, map[string]bool{"project": true, "decision": true})
	if err != nil {
		return fail(err)
	}
	if helpRequested(fs) {
		fmt.Println("usage: weeco team answer T-... --decision \"...\"")
		return 0
	}
	teamID, err := fs.positional(0, "team")
	if err != nil {
		return fail(err)
	}
	decision := strings.TrimSpace(fs.value("decision"))
	if decision == "" {
		return fail(fmt.Errorf("--decision is required"))
	}
	project, team, err := resolveTeam(fs, teamID)
	if err != nil {
		return fail(err)
	}
	if !team.CanAnswer() {
		return fail(fmt.Errorf("team %s is %s; expected blocked", teamID, team.State))
	}
	if err := store.WithLock(project.Key, func() error {
		return store.Append(project.Key, "team", teamID, "team_answered", "boss", map[string]any{"decision": decision})
	}); err != nil {
		return fail(err)
	}
	if err := herdr.AgentPrompt(herdr.Target(team.PaneID, team.LeadAgent), "BOSS DECISION: "+decision, false, 0); err != nil {
		fmt.Printf("warning: could not prompt lead: %v\n", err)
		fmt.Printf("answer it in pane %s\n", team.PaneID)
	}
	fmt.Printf("answered: %s\n", teamID)
	return 0
}

func cmdTeamDisband(args []string) int {
	fs, err := parseFlags(args, map[string]bool{"project": true})
	if err != nil {
		return fail(err)
	}
	if helpRequested(fs) {
		fmt.Println("usage: weeco team disband T-... [--force]")
		return 0
	}
	teamID, err := fs.positional(0, "team")
	if err != nil {
		return fail(err)
	}
	project, team, err := resolveTeam(fs, teamID)
	if err != nil {
		return fail(err)
	}
	if team.State == model.TeamDisbanded {
		fmt.Printf("team already disbanded: %s\n", teamID)
		return 0
	}
	force := fs.bool("force")
	if !team.CanDisband(force) {
		return fail(fmt.Errorf("team %s is %s; accept it first or use --force", teamID, team.State))
	}
	if err := closeTeamWithMembers(project, team); err != nil {
		if !force {
			return fail(fmt.Errorf("could not close team pane: %v", err))
		}
		fmt.Printf("warning: could not close team pane: %v\n", err)
	}
	_ = store.DeletePaneContext(team.PaneID)
	if err := store.WithLock(project.Key, func() error {
		return store.Append(project.Key, "team", teamID, "team_disbanded", "boss", map[string]any{"force": force})
	}); err != nil {
		return fail(err)
	}
	fmt.Printf("disbanded: %s\nworktree/workspace: %s\n", teamID, orDash(team.Workspace))
	return 0
}
