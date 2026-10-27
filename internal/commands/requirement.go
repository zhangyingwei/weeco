package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"weeco/internal/model"
	"weeco/internal/store"
	"weeco/internal/sysutil"
	"weeco/internal/workspace"
)

var requirementIDRe = regexp.MustCompile(`^R-\d{8}-\d{2,3}$`)

func cmdRequirement(args []string) int {
	if len(args) == 0 {
		fmt.Println("usage: weeco requirement <new|list|show|close> [flags]")
		return 1
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "new":
		return cmdRequirementNew(rest)
	case "list":
		return cmdRequirementList(rest)
	case "show":
		return cmdRequirementShow(rest)
	case "stage":
		return cmdRequirementStage(rest)
	case "close":
		return cmdRequirementClose(rest)
	case "-h", "--help":
		fmt.Println("usage: weeco requirement <new|list|show|stage|close> [flags]")
		return 0
	default:
		return fail(fmt.Errorf("unknown requirement command: %s", sub))
	}
}

func cmdRequirementNew(args []string) int {
	fs, err := parseFlags(args, map[string]bool{
		"project": true, "goal": true, "goal-file": true, "stage": true, "id": true,
	})
	if err != nil {
		return fail(err)
	}
	if helpRequested(fs) {
		fmt.Println("usage: weeco requirement new [--goal TEXT | --goal-file PATH] [--stage NAME] [--id R-YYYYMMDD-NN]")
		return 0
	}
	project, err := resolveProject(fs)
	if err != nil {
		return fail(err)
	}
	goal := fs.value("goal")
	if file := fs.value("goal-file"); file != "" {
		data, err := os.ReadFile(sysutil.ExpandHome(file))
		if err != nil {
			return fail(err)
		}
		goal = string(data)
	}
	goal = strings.TrimSpace(goal)
	if goal == "" {
		return fail(fmt.Errorf("provide --goal or --goal-file"))
	}
	stage := fs.value("stage")
	if stage == "" {
		stage = "requirement"
	}
	var reqID string
	err = store.WithLock(project.Key, func() error {
		reqID = fs.value("id")
		if reqID == "" {
			id, err := store.NextID(project.Key, "R")
			if err != nil {
				return err
			}
			reqID = id
		}
		if !requirementIDRe.MatchString(reqID) {
			return fmt.Errorf("invalid requirement id: %s (expected R-YYYYMMDD-NN)", reqID)
		}
		if _, err := store.LoadRequirement(project.Key, reqID); err == nil {
			return fmt.Errorf("requirement already exists: %s", reqID)
		}
		return store.Append(project.Key, "requirement", reqID, "requirement_created", "boss", map[string]any{
			"goal": goal, "stage": stage,
		})
	})
	if err != nil {
		return fail(err)
	}
	fmt.Printf("requirement: %s\nstage:       %s\n", reqID, stage)
	fmt.Printf("next:        write docs/weeco/requirements/%s.md (goal/scope/non-goals/verdict)\n", reqID)
	fmt.Printf("             weeco team recruit --requirement %s --purpose design --contract <file> --size N\n", reqID)
	return 0
}

func cmdRequirementList(args []string) int {
	fs, err := parseFlags(args, map[string]bool{"project": true})
	if err != nil {
		return fail(err)
	}
	if helpRequested(fs) {
		fmt.Println("usage: weeco requirement list [--json]")
		return 0
	}
	project, err := resolveProject(fs)
	if err != nil {
		return fail(err)
	}
	events, err := store.ReadEvents(project.Key)
	if err != nil {
		return fail(err)
	}
	ids, err := store.ListRequirementIDs(project.Key)
	if err != nil {
		return fail(err)
	}
	var reqs []model.Requirement
	for _, id := range ids {
		req, ok := store.FoldRequirement(events, id)
		if ok {
			reqs = append(reqs, req)
		}
	}
	if fs.bool("json") {
		data, _ := json.MarshalIndent(reqs, "", "  ")
		fmt.Println(string(data))
		return 0
	}
	if len(reqs) == 0 {
		fmt.Println("no requirements")
		return 0
	}
	fmt.Printf("%-16s %-12s %-12s %-12s GOAL\n", "REQUIREMENT", "STATE", "STAGE", "WORKSPACE")
	for _, req := range reqs {
		fmt.Printf("%-16s %-12s %-12s %-12s %s\n", req.ID, req.State, orDash(req.Stage), orDash(req.Workspace), req.Goal)
	}
	return 0
}

func cmdRequirementStage(args []string) int {
	fs, err := parseFlags(args, map[string]bool{"project": true, "set": true})
	if err != nil {
		return fail(err)
	}
	if helpRequested(fs) {
		fmt.Println("usage: weeco requirement stage R-... --set requirement|design|plan|implement|review|delivery")
		return 0
	}
	reqID, err := fs.positional(0, "requirement")
	if err != nil {
		return fail(err)
	}
	stage := strings.TrimSpace(fs.value("set"))
	if stage == "" {
		return fail(fmt.Errorf("--set is required"))
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
	if err := store.WithLock(project.Key, func() error {
		return store.Append(project.Key, "requirement", reqID, "requirement_stage", "boss", map[string]any{"stage": stage})
	}); err != nil {
		return fail(err)
	}
	fmt.Printf("requirement: %s\nstage:       %s\n", reqID, stage)
	return 0
}

func cmdRequirementShow(args []string) int {
	fs, err := parseFlags(args, map[string]bool{"project": true})
	if err != nil {
		return fail(err)
	}
	if helpRequested(fs) {
		fmt.Println("usage: weeco requirement show R-...")
		return 0
	}
	reqID, err := fs.positional(0, "requirement")
	if err != nil {
		return fail(err)
	}
	project, err := resolveProject(fs)
	if err != nil {
		return fail(err)
	}
	req, err := store.LoadRequirement(project.Key, reqID)
	if err != nil {
		return fail(err)
	}
	fmt.Printf("requirement: %s\nstate:       %s\nstage:       %s\ngoal:        %s\n", req.ID, req.State, orDash(req.Stage), req.Goal)
	if req.Workspace != "" {
		fmt.Printf("workspace:   %s\nbranch:      %s\nmanaged:     %t\n", req.Workspace, orDash(req.Branch), req.WorktreeManaged)
	}
	teams, err := store.TeamsForRequirement(project.Key, reqID)
	if err != nil {
		return fail(err)
	}
	if len(teams) == 0 {
		fmt.Println("teams:       (none)")
		return 0
	}
	fmt.Println("teams:")
	fmt.Printf("  %-16s %-12s %-10s %-4s %s\n", "TEAM", "STATE", "PURPOSE", "SIZE", "UPDATED")
	for _, team := range teams {
		fmt.Printf("  %-16s %-12s %-10s %-4d %s\n", team.ID, team.State, team.Purpose, team.Size, team.UpdatedAt)
	}
	return 0
}

func cmdRequirementClose(args []string) int {
	fs, err := parseFlags(args, map[string]bool{"project": true})
	if err != nil {
		return fail(err)
	}
	if helpRequested(fs) {
		fmt.Println("usage: weeco requirement close R-... [--remove-worktree] [--lessons-reviewed] [--force]")
		return 0
	}
	reqID, err := fs.positional(0, "requirement")
	if err != nil {
		return fail(err)
	}
	project, err := resolveProject(fs)
	if err != nil {
		return fail(err)
	}
	req, err := store.LoadRequirement(project.Key, reqID)
	if err != nil {
		return fail(err)
	}
	if req.State == model.RequirementClosed {
		fmt.Printf("requirement already closed: %s\n", reqID)
		return 0
	}
	teams, err := store.TeamsForRequirement(project.Key, reqID)
	if err != nil {
		return fail(err)
	}
	var open []string
	for _, team := range teams {
		if team.IsOpen() {
			open = append(open, fmt.Sprintf("%s(%s)", team.ID, team.State))
		}
	}
	if len(open) > 0 && !fs.bool("force") {
		return fail(fmt.Errorf("requirement has open teams: %s; accept/disband them first or use --force", strings.Join(open, ", ")))
	}
	removed := false
	if fs.bool("remove-worktree") && req.Workspace != "" {
		if req.WorktreeManaged {
			if err := workspace.Remove(project.Root, req.Workspace, true, fs.bool("force")); err != nil {
				return fail(err)
			}
			removed = true
		} else {
			fmt.Printf("workspace is external and was left in place: %s\n", req.Workspace)
		}
	}
	lessonsReviewed := fs.bool("lessons-reviewed")
	if !lessonsReviewed {
		lessonsPath := filepath.Join(project.Root, "docs", "weeco", "lessons.md")
		if sysutil.FileExists(lessonsPath) {
			fmt.Printf("warning: lessons not marked reviewed: %s (pass --lessons-reviewed)\n", lessonsPath)
		} else {
			fmt.Println("warning: no docs/weeco/lessons.md; pass --lessons-reviewed only if there is nothing to record")
		}
	}
	if err := store.WithLock(project.Key, func() error {
		return store.Append(project.Key, "requirement", reqID, "requirement_closed", "boss", map[string]any{
			"remove_worktree":  fs.bool("remove-worktree"),
			"removed":          removed,
			"workspace":        req.Workspace,
			"lessons_reviewed": lessonsReviewed,
		})
	}); err != nil {
		return fail(err)
	}
	fmt.Printf("requirement closed: %s\n", reqID)
	if removed {
		fmt.Printf("workspace removed: %s\n", req.Workspace)
	}
	return 0
}
