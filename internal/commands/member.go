package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"weeco/internal/assets"
	"weeco/internal/herdr"
	"weeco/internal/model"
	"weeco/internal/store"
	"weeco/internal/sysutil"
)

var placeholderMemberName = regexp.MustCompile(`(?i)^(员工|成员|employee|member|worker|agent|dev)[-_ ]?\d+$`)

type memberView struct {
	model.Member
	Live   string `json:"live_status"`
	Report string `json:"report"`
}

func buildMemberViews(members []model.Member) []memberView {
	views := make([]memberView, 0, len(members))
	for _, m := range members {
		views = append(views, memberView{
			Member: m, Live: memberLiveStatus(m), Report: presence(m.ReportPath),
		})
	}
	return views
}

func pendingMembers(project model.Project, team model.Team) ([]model.Member, error) {
	members, err := store.MembersForTeam(project.Key, team.ID)
	if err != nil {
		return nil, err
	}
	var pending []model.Member
	for _, m := range members {
		if m.NeedsAttention() {
			pending = append(pending, m)
		}
	}
	return pending, nil
}

func pendingMemberNames(members []model.Member) string {
	var parts []string
	for _, m := range members {
		parts = append(parts, fmt.Sprintf("%s (%s, %s)", m.Name, m.AgentName, m.State))
	}
	return strings.Join(parts, "; ")
}

func cmdMember(args []string) int {
	if len(args) == 0 {
		fmt.Println("usage: weeco member <recruit|status|wait|accept|rework|close> [flags]")
		return 1
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "recruit":
		return cmdMemberRecruit(rest)
	case "status":
		return cmdMemberStatus(rest)
	case "wait":
		return cmdMemberWait(rest)
	case "accept":
		return cmdMemberAccept(rest)
	case "rework":
		return cmdMemberRework(rest)
	case "close":
		return cmdMemberClose(rest)
	case "-h", "--help":
		fmt.Println("usage: weeco member <recruit|status|wait|accept|rework|close> [flags]")
		return 0
	default:
		return fail(fmt.Errorf("unknown member command: %s", sub))
	}
}

func cmdMemberRecruit(args []string) int {
	fs, err := parseFlags(args, map[string]bool{
		"project": true, "team": true, "name": true, "agent": true,
		"role": true, "task": true, "report": true, "timeout": true,
	})
	if err != nil {
		return fail(err)
	}
	if helpRequested(fs) {
		fmt.Println("usage: weeco member recruit --name \"正式名字\" --agent ascii-name --role \"角色\" --task FILE [--report PATH] [--team T-...] [--project PATH]")
		return 0
	}
	project, team, err := resolveLeadTeam(fs)
	if err != nil {
		return fail(err)
	}
	if team.State != model.TeamActive && team.State != model.TeamRework {
		return fail(fmt.Errorf("team %s is %s; can only recruit members while active/rework", team.ID, team.State))
	}
	name := strings.TrimSpace(fs.value("name"))
	agent := strings.TrimSpace(fs.value("agent"))
	role := strings.TrimSpace(fs.value("role"))
	taskSource := strings.TrimSpace(fs.value("task"))
	if name == "" || agent == "" || role == "" || taskSource == "" {
		return fail(fmt.Errorf("--name, --agent, --role and --task are required"))
	}
	if err := validateMemberIdentity(project.Key, team, name, agent, role); err != nil {
		return fail(err)
	}
	taskData, err := os.ReadFile(sysutil.ExpandHome(taskSource))
	if err != nil {
		return fail(fmt.Errorf("read task file: %w", err))
	}
	if strings.TrimSpace(string(taskData)) == "" {
		return fail(fmt.Errorf("task file is empty: %s", taskSource))
	}
	teamDir := store.TeamDir(project.Key, team.ID)
	memberDir := filepath.Join(teamDir, "members", agent)
	if err := os.MkdirAll(memberDir, 0o755); err != nil {
		return fail(err)
	}
	taskPath := filepath.Join(memberDir, "task.md")
	if err := os.WriteFile(taskPath, taskData, 0o644); err != nil {
		return fail(err)
	}
	reportPath := filepath.Join(memberDir, "report.md")
	if v := strings.TrimSpace(fs.value("report")); v != "" {
		if filepath.IsAbs(v) {
			reportPath = v
		} else {
			reportPath = filepath.Join(teamDir, v)
		}
	}
	if err := os.MkdirAll(filepath.Dir(reportPath), 0o755); err != nil {
		return fail(err)
	}
	memberPrompt, err := assets.Read(assets.TeamMemberPrompt)
	if err != nil {
		return fail(err)
	}
	promptPath := filepath.Join(memberDir, "system-prompt.md")
	header := fmt.Sprintf("# 团队成员\n\n正式名字：%s\nagent name：%s\n角色：%s\n团队：%s\n\n", name, agent, role, team.ID)
	if err := os.WriteFile(promptPath, append([]byte(header), memberPrompt...), 0o644); err != nil {
		return fail(err)
	}

	member := model.Member{
		TeamID: team.ID, AgentName: agent, Name: name, Role: role,
		TaskPath: taskPath, ReportPath: reportPath,
	}
	existing, err := store.MembersForTeam(project.Key, team.ID)
	if err != nil {
		return fail(err)
	}
	if err := store.WithLock(project.Key, func() error {
		return store.Append(project.Key, "member", member.Key(), "member_created", "lead", map[string]any{
			"team_id": team.ID, "agent_name": agent, "name": name, "role": role,
			"task_path": taskPath, "report_path": reportPath,
		})
	}); err != nil {
		return fail(err)
	}

	targetPane, direction := memberSplitTarget(team, existing)
	paneID, err := herdr.PaneSplit(targetPane, direction, team.Workspace, 0.5)
	if err != nil && targetPane != team.PaneID {
		paneID, err = herdr.PaneSplit(team.PaneID, "right", team.Workspace, 0.5)
	}
	if err != nil {
		return failMemberStart(project, member, "", "split pane: "+err.Error())
	}
	cfg := store.LoadConfig()
	if err := herdr.AgentStart(agent, paneID, cfg.AgentKind, cfg.ReadyTimeoutMS, []string{teamDir}, cfg.TeamLeadPermissionMode, promptPath); err != nil {
		return failMemberStart(project, member, paneID, "start agent: "+err.Error())
	}
	readyTimeout := float64(cfg.ReadyTimeoutMS) / 1000.0
	if readyTimeout < 30 {
		readyTimeout = 30
	}
	if err := herdr.AgentWait(agent, readyTimeout, ""); err != nil {
		return failMemberStart(project, member, paneID, "wait for agent: "+err.Error())
	}
	member.PaneID = paneID
	_ = herdr.PaneRename(paneID, name+" ("+role+")")
	if err := store.WithLock(project.Key, func() error {
		return store.Append(project.Key, "member", member.Key(), "member_started", "lead", map[string]any{
			"pane_id": paneID, "agent_name": agent, "name": name, "role": role,
		})
	}); err != nil {
		return failMemberStart(project, member, paneID, "record start: "+err.Error())
	}
	if err := store.WritePaneContext(store.PaneContext{
		ProjectRoot: project.Root, ProjectKey: project.Key, TeamID: team.ID,
		PaneID: paneID, MemberAgent: agent, MemberName: name,
	}); err != nil {
		fmt.Printf("warning: could not write member pane context: %v\n", err)
	}

	initial := fmt.Sprintf(`You are %s, agent %s, role %s, in team %s.

Read your task: %s
Workspace: %s
Team control dir: %s
Write your report to: %s

Do not modify the contract. Do not push. Do not touch the main branch. Stay within your task scope.
When done, write the report and stop. The lead will review your report.
`, name, agent, role, team.ID, taskPath, team.Workspace, teamDir, reportPath)
	if err := herdr.AgentPrompt(agent, initial, false, 0); err != nil {
		return failMemberStart(project, member, paneID, "initial prompt: "+err.Error())
	}
	_ = store.WithLock(project.Key, func() error {
		return store.Append(project.Key, "member", member.Key(), "member_prompted", "lead", nil)
	})
	fmt.Printf("member:   %s (%s)\nrole:     %s\npane:     %s\ntask:     %s\nreport:   %s\n",
		name, agent, role, paneID, taskPath, reportPath)
	fmt.Printf("next:     weeco member status %s\n          weeco member wait %s\n", agent, agent)
	return 0
}

func validateMemberIdentity(key string, team model.Team, name, agent, role string) error {
	if !model.ValidAgentName(agent) {
		return fmt.Errorf("invalid agent name %q: use 1-32 lowercase letters/digits/-/_ starting with a letter", agent)
	}
	if agent == model.LeadAgentName(team.ID) {
		return fmt.Errorf("agent name %q is reserved for the team lead", agent)
	}
	if name == agent {
		return fmt.Errorf("formal name and agent name must differ")
	}
	if strings.EqualFold(name, role) {
		return fmt.Errorf("formal name must not be the role name (%q)", role)
	}
	if placeholderMemberName.MatchString(name) || strings.EqualFold(name, "TBD") || name == "未命名" {
		return fmt.Errorf("formal name %q is a placeholder; give the member a real name", name)
	}
	if _, err := store.LoadMember(key, team.ID, agent); err == nil {
		return fmt.Errorf("member %s already exists in team %s; use a new name", agent, team.ID)
	}
	existing, err := store.MembersForTeam(key, team.ID)
	if err != nil {
		return err
	}
	for _, m := range existing {
		if strings.EqualFold(m.Name, name) {
			return fmt.Errorf("formal name %q is already used by %s; names must be unique in the team", name, m.AgentName)
		}
	}
	return nil
}

func memberSplitTarget(team model.Team, existing []model.Member) (string, string) {
	for i := len(existing) - 1; i >= 0; i-- {
		m := existing[i]
		if m.PaneID == "" || m.State == model.MemberClosed || m.State == model.MemberLost {
			continue
		}
		if herdr.Agent(m.AgentName) != nil {
			return m.PaneID, "down"
		}
	}
	return team.PaneID, "right"
}

func failMemberStart(project model.Project, member model.Member, paneID, reason string) int {
	_ = store.WithLock(project.Key, func() error {
		return store.Append(project.Key, "member", member.Key(), "member_start_failed", "system", map[string]any{"reason": reason})
	})
	if paneID != "" {
		_ = herdr.PaneClose(paneID)
		_ = store.DeletePaneContext(paneID)
	}
	return fail(fmt.Errorf("member %s failed to start: %s", member.AgentName, reason))
}

func cmdMemberStatus(args []string) int {
	fs, err := parseFlags(args, map[string]bool{"project": true, "team": true, "agent": true})
	if err != nil {
		return fail(err)
	}
	if helpRequested(fs) {
		fmt.Println("usage: weeco member status <agent-name> [--json]")
		return 0
	}
	project, team, err := resolveLeadTeam(fs)
	if err != nil {
		return fail(err)
	}
	agent, err := memberAgentArg(fs)
	if err != nil {
		return fail(err)
	}
	member, err := store.LoadMember(project.Key, team.ID, agent)
	if err != nil {
		return fail(err)
	}
	live := memberLiveStatus(member)
	if fs.bool("json") {
		data, _ := json.MarshalIndent(map[string]any{
			"member": member, "live_status": live, "report": presence(member.ReportPath),
		}, "", "  ")
		fmt.Println(string(data))
		return 0
	}
	fmt.Printf("member:   %s (%s)\nrole:     %s\nteam:     %s\nstate:    %s\nlive:     %s\npane:     %s\ntask:     %s\nreport:   %s\n",
		member.Name, member.AgentName, member.Role, member.TeamID, member.State, live,
		orDash(member.PaneID), orDash(member.TaskPath), orDash(member.ReportPath)+" ("+presence(member.ReportPath)+")")
	if member.LastIssues != "" {
		fmt.Printf("issues:   %s\n", member.LastIssues)
	}
	if member.NeedsAttention() {
		fmt.Println("next:     weeco member wait " + member.AgentName)
		fmt.Println("          weeco member accept/rework " + member.AgentName)
	}
	return 0
}

func cmdMemberWait(args []string) int {
	fs, err := parseFlags(args, map[string]bool{"project": true, "team": true, "agent": true, "timeout": true})
	if err != nil {
		return fail(err)
	}
	if helpRequested(fs) {
		fmt.Println("usage: weeco member wait <agent-name>|--all [--timeout SECONDS]")
		return 0
	}
	project, team, err := resolveLeadTeam(fs)
	if err != nil {
		return fail(err)
	}
	var targets []model.Member
	if fs.bool("all") {
		members, err := store.MembersForTeam(project.Key, team.ID)
		if err != nil {
			return fail(err)
		}
		for _, m := range members {
			if m.NeedsAttention() {
				targets = append(targets, m)
			}
		}
		if len(targets) == 0 {
			fmt.Println("no members need attention")
			return 0
		}
	} else {
		agent, err := memberAgentArg(fs)
		if err != nil {
			return fail(err)
		}
		m, err := store.LoadMember(project.Key, team.ID, agent)
		if err != nil {
			return fail(err)
		}
		targets = append(targets, m)
	}
	timeout := parseTimeout(fs.value("timeout"), store.LoadConfig().WaitTimeoutS)
	for _, m := range targets {
		if m.State == model.MemberAccepted || m.State == model.MemberClosed || m.State == model.MemberLost {
			fmt.Printf("%s: %s\n", m.AgentName, m.State)
			continue
		}
		if herdr.Agent(m.AgentName) == nil {
			_ = store.WithLock(project.Key, func() error {
				return store.Append(project.Key, "member", m.Key(), "member_lost", "system", map[string]any{"reason": "agent is missing"})
			})
			fmt.Printf("%s: lost (agent is missing)\n", m.AgentName)
			continue
		}
		if live := herdr.AgentStatus(m.AgentName); live == "blocked" {
			fmt.Printf("%s: blocked (live status); inspect the member pane\n", m.AgentName)
			continue
		}
		if err := herdr.AgentWait(m.AgentName, timeout, ""); err != nil {
			fmt.Printf("%s: wait did not settle: %v\n", m.AgentName, err)
			fmt.Printf("  inspect pane %s; run weeco member status %s\n", orDash(m.PaneID), m.AgentName)
			continue
		}
		fresh, err := store.LoadMember(project.Key, team.ID, m.AgentName)
		if err == nil {
			m = fresh
		}
		fmt.Printf("%s: %s (live: %s, report: %s)\n", m.AgentName, m.State, memberLiveStatus(m), presence(m.ReportPath))
	}
	return 0
}

func cmdMemberAccept(args []string) int {
	fs, err := parseFlags(args, map[string]bool{"project": true, "team": true, "agent": true, "note": true})
	if err != nil {
		return fail(err)
	}
	if helpRequested(fs) {
		fmt.Println("usage: weeco member accept <agent-name> --note \"what you checked\"")
		return 0
	}
	project, team, err := resolveLeadTeam(fs)
	if err != nil {
		return fail(err)
	}
	agent, err := memberAgentArg(fs)
	if err != nil {
		return fail(err)
	}
	note := strings.TrimSpace(fs.value("note"))
	if note == "" {
		return fail(fmt.Errorf("--note is required; record what you checked"))
	}
	member, err := store.LoadMember(project.Key, team.ID, agent)
	if err != nil {
		return fail(err)
	}
	if member.State == model.MemberClosed || member.State == model.MemberLost {
		return fail(fmt.Errorf("member %s is %s; cannot accept", agent, member.State))
	}
	if err := store.WithLock(project.Key, func() error {
		return store.Append(project.Key, "member", member.Key(), "member_accepted", "lead", map[string]any{"note": note})
	}); err != nil {
		return fail(err)
	}
	fmt.Printf("accepted: %s (%s)\n", member.Name, agent)
	return 0
}

func cmdMemberRework(args []string) int {
	fs, err := parseFlags(args, map[string]bool{"project": true, "team": true, "agent": true, "issues": true})
	if err != nil {
		return fail(err)
	}
	if helpRequested(fs) {
		fmt.Println("usage: weeco member rework <agent-name> --issues \"specific problems\"")
		return 0
	}
	project, team, err := resolveLeadTeam(fs)
	if err != nil {
		return fail(err)
	}
	agent, err := memberAgentArg(fs)
	if err != nil {
		return fail(err)
	}
	issues := strings.TrimSpace(fs.value("issues"))
	if issues == "" {
		return fail(fmt.Errorf("--issues is required"))
	}
	member, err := store.LoadMember(project.Key, team.ID, agent)
	if err != nil {
		return fail(err)
	}
	if member.State == model.MemberClosed || member.State == model.MemberLost {
		return fail(fmt.Errorf("member %s is %s; cannot rework", agent, member.State))
	}
	if err := store.WithLock(project.Key, func() error {
		return store.Append(project.Key, "member", member.Key(), "member_rework", "lead", map[string]any{"issues": issues})
	}); err != nil {
		return fail(err)
	}
	prompt := fmt.Sprintf("LEAD REVIEW: rework required.\n\nIssues:\n%s\n\nUpdate your report: %s\nWhen done, write the report and stop.", issues, member.ReportPath)
	if err := herdr.AgentPrompt(agent, prompt, false, 0); err != nil {
		fmt.Printf("warning: could not prompt member: %v\n", err)
		fmt.Printf("prompt it in pane %s\n", orDash(member.PaneID))
	}
	fmt.Printf("rework: %s (%s)\nissues: %s\n", member.Name, agent, issues)
	return 0
}

func cmdMemberClose(args []string) int {
	fs, err := parseFlags(args, map[string]bool{"project": true, "team": true, "agent": true})
	if err != nil {
		return fail(err)
	}
	if helpRequested(fs) {
		fmt.Println("usage: weeco member close <agent-name>")
		return 0
	}
	project, team, err := resolveLeadTeam(fs)
	if err != nil {
		return fail(err)
	}
	agent, err := memberAgentArg(fs)
	if err != nil {
		return fail(err)
	}
	member, err := store.LoadMember(project.Key, team.ID, agent)
	if err != nil {
		return fail(err)
	}
	if member.State == model.MemberClosed {
		fmt.Printf("member already closed: %s (%s)\n", member.Name, agent)
		return 0
	}
	if member.PaneID != "" {
		if err := herdr.PaneClose(member.PaneID); err != nil {
			fmt.Printf("warning: could not close pane %s: %v\n", member.PaneID, err)
		}
		_ = store.DeletePaneContext(member.PaneID)
	}
	if err := store.WithLock(project.Key, func() error {
		return store.Append(project.Key, "member", member.Key(), "member_closed", "lead", nil)
	}); err != nil {
		return fail(err)
	}
	fmt.Printf("closed: %s (%s)\n", member.Name, agent)
	return 0
}

func memberAgentArg(fs *flagSet) (string, error) {
	agent := strings.TrimSpace(fs.value("agent"))
	if agent == "" {
		var err error
		agent, err = fs.positional(0, "agent-name")
		if err != nil {
			return "", err
		}
	}
	return agent, nil
}

func memberLiveStatus(m model.Member) string {
	if m.AgentName == "" {
		return "unknown"
	}
	agent := herdr.Agent(m.AgentName)
	if agent == nil {
		if m.State == model.MemberClosed {
			return "closed"
		}
		return "missing"
	}
	status := herdr.AgentStatus(m.AgentName)
	if status == "" {
		return "unknown"
	}
	return status
}
