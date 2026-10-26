package commands

import (
	"encoding/json"
	"fmt"

	"weeco/internal/model"
	"weeco/internal/store"
)

type statusEntry struct {
	Requirement  model.Requirement `json:"requirement"`
	OpenTeam     *model.Team       `json:"open_team,omitempty"`
	NextSkill    string            `json:"next_skill"`
	NextCommands []string          `json:"next_commands,omitempty"`
}

type statusOutput struct {
	Project      model.Project `json:"project"`
	Requirement  *statusEntry  `json:"requirement,omitempty"`
	Requirements []statusEntry `json:"requirements,omitempty"`
	NextSkill    string        `json:"next_skill,omitempty"`
	NextCommands []string      `json:"next_commands,omitempty"`
}

func cmdStatus(args []string) int {
	fs, err := parseFlags(args, map[string]bool{"project": true})
	if err != nil {
		return fail(err)
	}
	if helpRequested(fs) {
		fmt.Println("usage: weeco status [R-...] [--json]")
		return 0
	}
	project, err := resolveProject(fs)
	if err != nil {
		return fail(err)
	}
	ids, err := store.ListRequirementIDs(project.Key)
	if err != nil {
		return fail(err)
	}
	argID := ""
	if len(fs.positionals) > 0 {
		argID = fs.positionals[0]
	}
	out := statusOutput{Project: project}
	if len(ids) == 0 {
		if fs.bool("json") {
			out.NextSkill = "weeco-requirements"
			printJSON(out)
			return 0
		}
		fmt.Printf("project:    %s\n", project.Root)
		fmt.Println("requirements: (none)")
		fmt.Println("next_skill: weeco-requirements")
		fmt.Println("next:       weeco requirement new --goal \"...\"")
		return 0
	}
	events, err := store.ReadEvents(project.Key)
	if err != nil {
		return fail(err)
	}
	if argID != "" {
		req, ok := store.FoldRequirement(events, argID)
		if !ok {
			return fail(fmt.Errorf("requirement %s not found", argID))
		}
		teams, err := store.TeamsForRequirement(project.Key, argID)
		if err != nil {
			return fail(err)
		}
		entry := buildStatusEntry(req, teams)
		out.Requirement = &entry
		out.NextSkill = entry.NextSkill
		out.NextCommands = entry.NextCommands
	} else if len(ids) == 1 {
		req, ok := store.FoldRequirement(events, ids[0])
		if !ok {
			return fail(fmt.Errorf("requirement %s not found", ids[0]))
		}
		teams, err := store.TeamsForRequirement(project.Key, ids[0])
		if err != nil {
			return fail(err)
		}
		entry := buildStatusEntry(req, teams)
		out.Requirement = &entry
		out.NextSkill = entry.NextSkill
		out.NextCommands = entry.NextCommands
	} else {
		for _, id := range ids {
			req, ok := store.FoldRequirement(events, id)
			if !ok {
				continue
			}
			teams, err := store.TeamsForRequirement(project.Key, id)
			if err != nil {
				return fail(err)
			}
			out.Requirements = append(out.Requirements, buildStatusEntry(req, teams))
		}
		out.NextSkill = "weeco-boss"
		out.NextCommands = []string{"weeco status R-..."}
	}
	if fs.bool("json") {
		printJSON(out)
		return 0
	}
	printStatus(out)
	return 0
}

func buildStatusEntry(req model.Requirement, teams []model.Team) statusEntry {
	var open *model.Team
	for i := range teams {
		if teams[i].IsOpen() {
			open = &teams[i]
			break
		}
	}
	entry := statusEntry{Requirement: req, OpenTeam: open}
	entry.NextSkill = nextSkillFor(req, open)
	entry.NextCommands = nextCommandsFor(req, open)
	return entry
}

func nextSkillFor(req model.Requirement, open *model.Team) string {
	if !req.IsOpen() {
		return ""
	}
	if open != nil {
		return "weeco-review"
	}
	switch req.Stage {
	case "requirement", "design":
		return "weeco-design"
	case "plan":
		return "weeco-plan"
	case "implement":
		return "weeco-implement"
	case "delivery":
		return "weeco-deliver"
	default:
		return "weeco-boss"
	}
}

func nextCommandsFor(req model.Requirement, open *model.Team) []string {
	if !req.IsOpen() {
		return []string{"# requirement closed"}
	}
	if open != nil {
		switch open.State {
		case model.TeamSubmitted:
			return []string{
				"weeco team status " + open.ID,
				"weeco team check " + open.ID,
				"weeco team accept " + open.ID + " --note \"...\"",
				"weeco team rework " + open.ID + " --issues \"...\"",
			}
		case model.TeamBlocked:
			return []string{
				"weeco team status " + open.ID,
				"weeco team answer " + open.ID + " --decision \"...\"",
			}
		case model.TeamLost:
			return []string{
				"weeco team repair " + open.ID,
				"weeco team disband " + open.ID + " --force",
				"# then recruit a new team with the same contract",
			}
		default:
			return []string{
				"weeco team wait " + open.ID,
				"weeco team status " + open.ID,
				"weeco team nudge " + open.ID,
			}
		}
	}
	switch req.Stage {
	case "requirement", "design":
		return []string{
			"weeco requirement show " + req.ID,
			"# load weeco-design",
		}
	case "plan":
		if req.Workspace == "" {
			return []string{
				"weeco workspace create --requirement " + req.ID + " --base main",
				"# load weeco-plan",
			}
		}
		return []string{"# load weeco-plan"}
	case "implement":
		if req.Workspace == "" {
			return []string{
				"weeco workspace create --requirement " + req.ID,
				"# load weeco-implement",
			}
		}
		return []string{"# load weeco-implement"}
	case "delivery":
		return []string{
			"weeco requirement show " + req.ID,
			"weeco requirement close " + req.ID + " --remove-worktree",
			"# load weeco-deliver",
		}
	default:
		return []string{"# load weeco-boss"}
	}
}

func printStatus(out statusOutput) {
	fmt.Printf("project:    %s\n", out.Project.Root)
	if out.Requirement != nil {
		entry := *out.Requirement
		req := entry.Requirement
		fmt.Printf("requirement: %s\n", req.ID)
		fmt.Printf("goal:        %s\n", req.Goal)
		fmt.Printf("stage:       %s\n", orDash(req.Stage))
		fmt.Printf("state:       %s\n", req.State)
		if req.Workspace != "" {
			fmt.Printf("workspace:   %s\n", req.Workspace)
		}
		if entry.OpenTeam != nil {
			fmt.Printf("open_team:   %s (%s)\n", entry.OpenTeam.ID, entry.OpenTeam.State)
		} else {
			fmt.Println("open_team:   (none)")
		}
		fmt.Printf("next_skill:  %s\n", orDash(entry.NextSkill))
		fmt.Println("next:")
		for _, command := range entry.NextCommands {
			fmt.Printf("  %s\n", command)
		}
		return
	}
	fmt.Printf("%-16s %-12s %-10s %-18s %s\n", "REQUIREMENT", "STATE", "STAGE", "OPEN TEAM", "NEXT SKILL")
	for _, entry := range out.Requirements {
		open := "-"
		if entry.OpenTeam != nil {
			open = entry.OpenTeam.ID + "(" + string(entry.OpenTeam.State) + ")"
		}
		fmt.Printf("  %-16s %-12s %-10s %-18s %s\n",
			entry.Requirement.ID, entry.Requirement.State, orDash(entry.Requirement.Stage), open, entry.NextSkill)
	}
	fmt.Printf("next_skill:  %s\n", orDash(out.NextSkill))
	fmt.Println("next:")
	for _, command := range out.NextCommands {
		fmt.Printf("  %s\n", command)
	}
}

func printJSON(value any) {
	data, _ := json.MarshalIndent(value, "", "  ")
	fmt.Println(string(data))
}
