package store

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"weeco/internal/model"
	"weeco/internal/sysutil"
)

func Append(key, entity, id, event, by string, data map[string]any) error {
	return AppendEvent(key, model.Event{
		TS: sysutil.NowISO(), Entity: entity, ID: id, Event: event, By: by, Data: data,
	})
}

func AppendEvent(key string, ev model.Event) error {
	if ev.TS == "" {
		ev.TS = sysutil.NowISO()
	}
	if ev.Data == nil {
		ev.Data = map[string]any{}
	}
	path := LedgerPath(key)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	line, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(append(line, '\n')); err != nil {
		return err
	}
	return f.Sync()
}

func ReadEvents(key string) ([]model.Event, error) {
	f, err := os.Open(LedgerPath(key))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var events []model.Event
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var ev model.Event
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			continue
		}
		events = append(events, ev)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return events, nil
}

func NextID(key, prefix string) (string, error) {
	events, err := ReadEvents(key)
	if err != nil {
		return "", err
	}
	used := map[string]bool{}
	for _, ev := range events {
		if ev.ID != "" {
			used[ev.ID] = true
		}
	}
	var dir string
	switch prefix {
	case "R":
		dir = filepath.Join(ProjectDir(key), "requirements")
	case "T":
		dir = filepath.Join(ProjectDir(key), "teams")
	}
	if entries, err := os.ReadDir(dir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				used[entry.Name()] = true
			}
		}
	}
	today := sysutil.TodayStamp()
	for i := 1; ; i++ {
		candidate := fmt.Sprintf("%s-%s-%02d", prefix, today, i)
		if !used[candidate] {
			return candidate, nil
		}
	}
}

func EventsFor(events []model.Event, entity, id string) []model.Event {
	out := make([]model.Event, 0)
	for _, ev := range events {
		if ev.Entity == entity && ev.ID == id {
			out = append(out, ev)
		}
	}
	return out
}

func ListRequirementIDs(key string) ([]string, error) {
	events, err := ReadEvents(key)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, ev := range events {
		if ev.Entity == "requirement" && ev.ID != "" {
			seen[ev.ID] = true
		}
	}
	return sortedKeys(seen), nil
}

func ListTeamIDs(key string) ([]string, error) {
	events, err := ReadEvents(key)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, ev := range events {
		if ev.Entity == "team" && ev.ID != "" {
			seen[ev.ID] = true
		}
	}
	return sortedKeys(seen), nil
}

func sortedKeys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for id := range set {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func FoldRequirement(events []model.Event, id string) (model.Requirement, bool) {
	req := model.Requirement{ID: id, State: model.RequirementOpen}
	found := false
	for _, ev := range EventsFor(events, "requirement", id) {
		if !found {
			found = true
			req.CreatedAt = ev.TS
		}
		req.LastEvent = ev.Event
		req.UpdatedAt = ev.TS
		switch ev.Event {
		case "requirement_created":
			req.Goal = asString(ev.Data["goal"])
			req.Stage = asString(ev.Data["stage"])
			req.State = model.RequirementOpen
		case "requirement_updated":
			if goal := asString(ev.Data["goal"]); goal != "" {
				req.Goal = goal
			}
			if stage := asString(ev.Data["stage"]); stage != "" {
				req.Stage = stage
			}
		case "requirement_workspace":
			req.Workspace = asString(ev.Data["workspace"])
			req.Branch = asString(ev.Data["branch"])
			req.BaseRef = asString(ev.Data["base_ref"])
			req.WorktreeManaged = asBool(ev.Data["worktree_managed"])
		case "requirement_stage":
			req.Stage = asString(ev.Data["stage"])
		case "requirement_done":
			req.State = model.RequirementDone
		case "requirement_closed":
			req.State = model.RequirementClosed
		}
	}
	return req, found
}

func FoldTeam(events []model.Event, id string) (model.Team, bool) {
	team := model.Team{ID: id, State: model.TeamDraft}
	found := false
	for _, ev := range EventsFor(events, "team", id) {
		if !found {
			found = true
			team.CreatedAt = ev.TS
		}
		team.LastEvent = ev.Event
		team.UpdatedAt = ev.TS
		switch ev.Event {
		case "team_created":
			team.RequirementID = asString(ev.Data["requirement_id"])
			team.Purpose = asString(ev.Data["purpose"])
			team.ContractPath = asString(ev.Data["contract_path"])
			team.ContractSHA = asString(ev.Data["contract_sha"])
			team.Workspace = asString(ev.Data["workspace"])
			team.Size = asInt(ev.Data["size"])
			team.State = model.TeamDraft
		case "team_started":
			team.State = model.TeamActive
			team.PaneID = asString(ev.Data["pane_id"])
			team.TabID = asString(ev.Data["tab_id"])
			team.WorkspaceID = asString(ev.Data["workspace_id"])
			team.LeadAgent = asString(ev.Data["lead_agent"])
		case "team_prompted":
			if team.State == model.TeamDraft {
				team.State = model.TeamActive
			}
		case "team_blocked":
			team.State = model.TeamBlocked
			team.BlockedQuestion = asString(ev.Data["question"])
		case "team_answered":
			team.State = model.TeamActive
			team.BlockedQuestion = ""
		case "team_submitted":
			team.State = model.TeamSubmitted
			team.ResultPath = asString(ev.Data["result_path"])
			team.ResultSHA = asString(ev.Data["result_sha"])
			team.SubmittedAt = ev.TS
		case "team_rework":
			team.State = model.TeamRework
			team.LastIssues = asString(ev.Data["issues"])
			team.ReworkCount++
		case "team_accepted":
			team.State = model.TeamAccepted
			team.AcceptedAt = ev.TS
		case "team_disbanded":
			team.State = model.TeamDisbanded
			team.DisbandedAt = ev.TS
		case "team_lost", "team_start_failed":
			team.State = model.TeamLost
			team.LastIssues = asString(ev.Data["reason"])
		case "team_checked":
			// Evidence only; it does not change the team state.
		}
	}
	return team, found
}

func LoadRequirement(key, id string) (model.Requirement, error) {
	events, err := ReadEvents(key)
	if err != nil {
		return model.Requirement{}, err
	}
	req, ok := FoldRequirement(events, id)
	if !ok {
		return req, fmt.Errorf("requirement %s not found", id)
	}
	return req, nil
}

func LoadTeam(key, id string) (model.Team, error) {
	events, err := ReadEvents(key)
	if err != nil {
		return model.Team{}, err
	}
	team, ok := FoldTeam(events, id)
	if !ok {
		return team, fmt.Errorf("team %s not found", id)
	}
	return team, nil
}

func TeamsForRequirement(key, reqID string) ([]model.Team, error) {
	events, err := ReadEvents(key)
	if err != nil {
		return nil, err
	}
	ids, err := ListTeamIDs(key)
	if err != nil {
		return nil, err
	}
	var teams []model.Team
	for _, id := range ids {
		team, ok := FoldTeam(events, id)
		if ok && team.RequirementID == reqID {
			teams = append(teams, team)
		}
	}
	return teams, nil
}

func OpenTeamForRequirement(key, reqID string) (model.Team, bool, error) {
	teams, err := TeamsForRequirement(key, reqID)
	if err != nil {
		return model.Team{}, false, err
	}
	for _, team := range teams {
		if team.IsOpen() {
			return team, true, nil
		}
	}
	return model.Team{}, false, nil
}

func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func asInt(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	case json.Number:
		i, _ := n.Int64()
		return int(i)
	}
	return 0
}

func asBool(v any) bool {
	b, _ := v.(bool)
	return b
}
