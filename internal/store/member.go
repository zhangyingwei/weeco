package store

import (
	"fmt"
	"sort"
	"strings"

	"weeco/internal/model"
)

// FoldMember folds all events for one team member. The ledger ID is
// "<team-id>/<agent-name>" so member names only need to be unique per team.
func FoldMember(events []model.Event, key string) (model.Member, bool) {
	member := model.Member{State: model.MemberActive}
	found := false
	for _, ev := range EventsFor(events, "member", key) {
		if !found {
			found = true
			member.CreatedAt = ev.TS
		}
		member.LastEvent = ev.Event
		member.UpdatedAt = ev.TS
		switch ev.Event {
		case "member_created":
			member.TeamID = asString(ev.Data["team_id"])
			member.AgentName = asString(ev.Data["agent_name"])
			member.Name = asString(ev.Data["name"])
			member.Role = asString(ev.Data["role"])
			member.TaskPath = asString(ev.Data["task_path"])
			member.ReportPath = asString(ev.Data["report_path"])
			member.State = model.MemberActive
		case "member_started":
			member.PaneID = asString(ev.Data["pane_id"])
			member.State = model.MemberActive
		case "member_blocked":
			member.State = model.MemberBlocked
			member.LastIssues = asString(ev.Data["question"])
		case "member_answered":
			member.State = model.MemberActive
			member.LastIssues = ""
		case "member_accepted":
			member.State = model.MemberAccepted
			member.AcceptedAt = ev.TS
		case "member_rework":
			member.State = model.MemberRework
			member.LastIssues = asString(ev.Data["issues"])
		case "member_lost", "member_start_failed":
			member.State = model.MemberLost
			member.LastIssues = asString(ev.Data["reason"])
		case "member_closed":
			member.State = model.MemberClosed
			member.ClosedAt = ev.TS
		case "member_prompted":
			// Prompting does not change the member state.
		}
	}
	return member, found
}

// MembersForTeamFromEvents returns members in creation order. It does not read
// the ledger again, so callers can combine it with other folds.
func MembersForTeamFromEvents(events []model.Event, teamID string) []model.Member {
	prefix := teamID + "/"
	seen := map[string]bool{}
	for _, ev := range events {
		if ev.Entity == "member" && strings.HasPrefix(ev.ID, prefix) {
			seen[ev.ID] = true
		}
	}
	var members []model.Member
	for _, key := range sortedKeys(seen) {
		if member, ok := FoldMember(events, key); ok {
			members = append(members, member)
		}
	}
	sort.SliceStable(members, func(i, j int) bool {
		if members[i].CreatedAt != members[j].CreatedAt {
			return members[i].CreatedAt < members[j].CreatedAt
		}
		return members[i].AgentName < members[j].AgentName
	})
	return members
}

func MembersForTeam(key, teamID string) ([]model.Member, error) {
	events, err := ReadEvents(key)
	if err != nil {
		return nil, err
	}
	return MembersForTeamFromEvents(events, teamID), nil
}

func LoadMember(key, teamID, agentName string) (model.Member, error) {
	events, err := ReadEvents(key)
	if err != nil {
		return model.Member{}, err
	}
	member, ok := FoldMember(events, model.MemberKey(teamID, agentName))
	if !ok {
		return member, fmt.Errorf("member %s not found in team %s", agentName, teamID)
	}
	return member, nil
}
