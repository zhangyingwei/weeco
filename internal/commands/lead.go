package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"weeco/internal/model"
	"weeco/internal/store"
	"weeco/internal/sysutil"
)

func cmdLead(args []string) int {
	if len(args) == 0 {
		fmt.Println("usage: weeco lead <status|submit|ask> [--team T-...] [--project PATH]")
		return 1
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "status":
		return cmdLeadStatus(rest)
	case "submit":
		return cmdLeadSubmit(rest)
	case "ask":
		return cmdLeadAsk(rest)
	case "-h", "--help":
		fmt.Println("usage: weeco lead <status|submit|ask> [--team T-...] [--project PATH]")
		return 0
	default:
		return fail(fmt.Errorf("unknown lead command: %s", sub))
	}
}

func cmdLeadStatus(args []string) int {
	fs, err := parseFlags(args, map[string]bool{"project": true, "team": true})
	if err != nil {
		return fail(err)
	}
	if helpRequested(fs) {
		fmt.Println("usage: weeco lead status [--team T-...] [--project PATH]")
		return 0
	}
	project, team, err := resolveLeadTeam(fs)
	if err != nil {
		return fail(err)
	}
	resultPath := filepath.Join(store.TeamDir(project.Key, team.ID), "result.md")
	if fs.bool("json") {
		data, _ := json.MarshalIndent(map[string]any{
			"project": project.Root, "team": team, "result_path": resultPath,
		}, "", "  ")
		fmt.Println(string(data))
		return 0
	}
	fmt.Printf("project:     %s\nteam:        %s\nstate:       %s\nrequirement: %s\npurpose:     %s\nsize:        %d\nworkspace:   %s\ncontract:    %s\nresult:      %s\n",
		project.Root, team.ID, team.State, team.RequirementID, team.Purpose, team.Size,
		orDash(team.Workspace), orDash(team.ContractPath), resultPath)
	members, _ := store.MembersForTeam(project.Key, team.ID)
	if len(members) > 0 {
		fmt.Printf("members:     %d\n", len(members))
		for _, v := range buildMemberViews(members) {
			fmt.Printf("  %-18s %-14s %-10s live=%-8s report=%s\n", v.AgentName, v.Name, v.State, v.Live, v.Report)
		}
	}
	if team.BlockedQuestion != "" {
		fmt.Printf("question:    %s\n", team.BlockedQuestion)
	}
	if team.LastIssues != "" {
		fmt.Printf("boss issues: %s\n", team.LastIssues)
	}
	fmt.Println("commands:")
	fmt.Println("  weeco member status <agent>")
	fmt.Println("  weeco member wait --all")
	fmt.Println("  weeco lead submit")
	fmt.Println("  weeco lead ask --question \"...\"")
	fmt.Printf("fallback: weeco lead submit --project %s --team %s\n", project.Root, team.ID)
	return 0
}

func cmdLeadSubmit(args []string) int {
	fs, err := parseFlags(args, map[string]bool{"project": true, "team": true})
	if err != nil {
		return fail(err)
	}
	if helpRequested(fs) {
		fmt.Println("usage: weeco lead submit [--team T-...] [--project PATH] [--no-lessons]")
		return 0
	}
	project, team, err := resolveLeadTeam(fs)
	if err != nil {
		return fail(err)
	}
	if team.State == model.TeamSubmitted {
		fmt.Printf("already submitted: %s\n", team.ID)
		return 0
	}
	if !team.CanSubmit() {
		return fail(fmt.Errorf("team %s is %s; cannot submit", team.ID, team.State))
	}
	resultPath := filepath.Join(store.TeamDir(project.Key, team.ID), "result.md")
	if !sysutil.FileExists(resultPath) {
		return fail(fmt.Errorf("result.md is missing: %s", resultPath))
	}
	pending, err := pendingMembers(project, team)
	if err != nil {
		return fail(err)
	}
	if len(pending) > 0 {
		return fail(fmt.Errorf("members still need lead acceptance: %s; run weeco member accept/close from the team lead pane", pendingMemberNames(pending)))
	}
	digest, err := sysutil.SHA256File(resultPath)
	if err != nil {
		return fail(err)
	}
	lessons := "present"
	if data, err := os.ReadFile(resultPath); err != nil {
		return fail(err)
	} else if !strings.Contains(string(data), "经验候选") {
		lessons = "missing"
	}
	if fs.bool("no-lessons") {
		lessons = "none"
	} else if lessons == "missing" {
		fmt.Println("warning: result.md has no 经验候选 section; add it or rerun with --no-lessons")
	}
	if err := store.WithLock(project.Key, func() error {
		return store.Append(project.Key, "team", team.ID, "team_submitted", "lead", map[string]any{
			"result_path": resultPath,
			"result_sha":  digest,
			"lessons":     lessons,
		})
	}); err != nil {
		return fail(err)
	}
	fmt.Printf("submitted: %s\nresult:    %s\n", team.ID, resultPath)
	fmt.Println("boss will review; wait for accept or rework")
	return 0
}

func cmdLeadAsk(args []string) int {
	fs, err := parseFlags(args, map[string]bool{"project": true, "team": true, "question": true})
	if err != nil {
		return fail(err)
	}
	if helpRequested(fs) {
		fmt.Println("usage: weeco lead ask --question \"...\" [--team T-...] [--project PATH]")
		return 0
	}
	question := strings.TrimSpace(fs.value("question"))
	if question == "" {
		return fail(fmt.Errorf("--question is required"))
	}
	project, team, err := resolveLeadTeam(fs)
	if err != nil {
		return fail(err)
	}
	if team.State == model.TeamBlocked {
		fmt.Printf("already waiting for the boss: %s\n", team.ID)
		return 0
	}
	if team.State != model.TeamActive && team.State != model.TeamRework {
		return fail(fmt.Errorf("team %s is %s; cannot ask", team.ID, team.State))
	}
	if err := store.WithLock(project.Key, func() error {
		return store.Append(project.Key, "team", team.ID, "team_blocked", "lead", map[string]any{"question": question})
	}); err != nil {
		return fail(err)
	}
	fmt.Printf("blocked: %s\nquestion: %s\n", team.ID, question)
	fmt.Println("wait for the boss answer")
	return 0
}
