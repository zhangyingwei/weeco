package commands

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"weeco/internal/herdr"
	"weeco/internal/model"
	"weeco/internal/store"
)

func resolveProject(fs *flagSet) (model.Project, error) {
	start := fs.value("project")
	if start == "" {
		wd, err := os.Getwd()
		if err != nil {
			return model.Project{}, err
		}
		start = wd
	}
	project, err := store.FindProject(start)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return model.Project{}, fmt.Errorf("no weeco project found from %s; run: weeco init --root <project>", start)
		}
		return model.Project{}, err
	}
	return project, nil
}

func resolveTeam(fs *flagSet, id string) (model.Project, model.Team, error) {
	project, err := resolveProject(fs)
	if err != nil {
		return model.Project{}, model.Team{}, err
	}
	if id == "" {
		return model.Project{}, model.Team{}, fmt.Errorf("missing team id")
	}
	team, err := store.LoadTeam(project.Key, id)
	if err != nil {
		return model.Project{}, model.Team{}, err
	}
	return project, team, nil
}

// resolveLeadTeam resolves a lead command from --team/--project when given,
// otherwise from the team pane context written by `team recruit`. This keeps
// the lead's normal commands free of flags.
func resolveLeadTeam(fs *flagSet) (model.Project, model.Team, error) {
	paneID := os.Getenv("HERDR_PANE_ID")
	if id := fs.value("team"); id != "" {
		if fs.value("project") != "" {
			return resolveTeam(fs, id)
		}
		if paneID != "" {
			if ctx, err := store.LoadPaneContext(paneID); err == nil {
				if project, err := store.LoadProject(ctx.ProjectKey); err == nil {
					if team, err := store.LoadTeam(project.Key, id); err == nil {
						return project, team, nil
					}
				}
			}
		}
		return resolveTeam(fs, id)
	}
	if paneID == "" {
		return model.Project{}, model.Team{}, fmt.Errorf("no --team and HERDR_PANE_ID is not set; run from the team pane or pass --team")
	}
	ctx, err := store.LoadPaneContext(paneID)
	if err != nil {
		return model.Project{}, model.Team{}, fmt.Errorf("no team context for pane %s; pass --project and --team", paneID)
	}
	if ctx.LeadAgent == "" {
		return model.Project{}, model.Team{}, fmt.Errorf("pane %s is a member pane; lead/member commands run from the team lead pane", paneID)
	}
	project, err := store.LoadProject(ctx.ProjectKey)
	if err != nil {
		return model.Project{}, model.Team{}, fmt.Errorf("pane context points to missing project %s: %v", ctx.ProjectKey, err)
	}
	team, err := store.LoadTeam(project.Key, ctx.TeamID)
	if err != nil {
		return model.Project{}, model.Team{}, err
	}
	if team.PaneID != "" && ctx.PaneID != "" && team.PaneID != ctx.PaneID {
		return model.Project{}, model.Team{}, fmt.Errorf("pane context is stale: pane %s now maps to team %s", paneID, team.ID)
	}
	if team.LeadAgent != "" && ctx.LeadAgent != "" && team.LeadAgent != ctx.LeadAgent {
		return model.Project{}, model.Team{}, fmt.Errorf("pane context is stale: lead %s no longer matches team %s", ctx.LeadAgent, team.ID)
	}
	return project, team, nil
}

func teamIDFrom(fs *flagSet) (string, error) {
	if id := fs.value("team"); id != "" {
		return id, nil
	}
	return fs.positional(0, "team")
}

func parsePositiveInt(text, name string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(text))
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("--%s must be a positive integer", name)
	}
	return n, nil
}

func parseTimeout(text string, fallback float64) float64 {
	if text == "" {
		return fallback
	}
	n, err := strconv.ParseFloat(text, 64)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}

func closeTeamPane(team model.Team) error {
	var errs []string
	if team.TabID != "" {
		if err := herdr.TabClose(team.TabID); err == nil {
			return nil
		} else {
			errs = append(errs, "tab: "+err.Error())
		}
	}
	if team.PaneID != "" {
		if err := herdr.PaneClose(team.PaneID); err != nil {
			errs = append(errs, "pane: "+err.Error())
		} else {
			return nil
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return nil
}

// closeTeamWithMembers closes the whole team tab, which also closes every
// employee pane. If the tab close fails, it falls back to closing each pane
// and removes all pane contexts.
func closeTeamWithMembers(project model.Project, team model.Team) error {
	members, _ := store.MembersForTeam(project.Key, team.ID)
	for _, m := range members {
		_ = store.DeletePaneContext(m.PaneID)
	}
	_ = store.DeletePaneContext(team.PaneID)
	if team.TabID != "" {
		if err := herdr.TabClose(team.TabID); err == nil {
			return nil
		}
	}
	var errs []string
	for _, m := range members {
		if m.PaneID == "" {
			continue
		}
		if err := herdr.PaneClose(m.PaneID); err != nil {
			errs = append(errs, "member pane "+m.PaneID+": "+err.Error())
		}
	}
	if team.PaneID != "" {
		if err := herdr.PaneClose(team.PaneID); err != nil {
			errs = append(errs, "lead pane: "+err.Error())
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return nil
}

func compactJSON(value any) string {
	data, _ := json.Marshal(value)
	return string(data)
}

func printTeamLine(team model.Team) {
	fmt.Printf("%-16s %-14s %-12s %-10s %-4d %-10s %s\n",
		team.ID, team.RequirementID, team.State, team.Purpose, team.Size,
		orDash(team.PaneID), orDash(team.UpdatedAt))
}

func orDash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}
