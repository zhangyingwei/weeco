package commands

import (
	"fmt"

	"weeco/internal/store"
	"weeco/internal/workspace"
)

func cmdWorkspace(args []string) int {
	if len(args) == 0 {
		fmt.Println("usage: weeco workspace create --requirement R-... [--path PATH] [--base REF]")
		return 1
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "create":
		return cmdWorkspaceCreate(rest)
	case "-h", "--help":
		fmt.Println("usage: weeco workspace create --requirement R-... [--path PATH] [--base REF]")
		return 0
	default:
		return fail(fmt.Errorf("unknown workspace command: %s", sub))
	}
}

func cmdWorkspaceCreate(args []string) int {
	fs, err := parseFlags(args, map[string]bool{
		"project": true, "requirement": true, "path": true, "base": true,
	})
	if err != nil {
		return fail(err)
	}
	if helpRequested(fs) {
		fmt.Println("usage: weeco workspace create --requirement R-... [--path PATH] [--base REF] [--force]")
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
	if req.Workspace != "" && !fs.bool("force") {
		return fail(fmt.Errorf("requirement already has a workspace: %s (use --force to replace the record)", req.Workspace))
	}
	if team, ok, err := store.OpenTeamForRequirement(project.Key, reqID); err != nil {
		return fail(err)
	} else if ok {
		return fail(fmt.Errorf("requirement has open team %s (%s); accept/disband it before changing the workspace", team.ID, team.State))
	}
	target := store.WorktreePath(project.Key, reqID)
	result, err := workspace.Create(project.Root, target, reqID, fs.value("base"), fs.value("path"))
	if err != nil {
		return fail(err)
	}
	if err := store.WithLock(project.Key, func() error {
		return store.Append(project.Key, "requirement", reqID, "requirement_workspace", "boss", map[string]any{
			"workspace":        result.Path,
			"branch":           result.Branch,
			"base_ref":         result.BaseRef,
			"worktree_managed": result.Managed,
		})
	}); err != nil {
		return fail(err)
	}
	fmt.Printf("requirement: %s\nworkspace:   %s\n", reqID, result.Path)
	if result.Branch != "" {
		fmt.Printf("branch:      %s\nbase:        %s\n", result.Branch, result.BaseRef)
	}
	if !result.Managed {
		fmt.Println("mode:        plain directory (worktree is not managed by git)")
	}
	return 0
}
