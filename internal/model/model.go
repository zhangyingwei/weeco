// Package model defines the company/team domain: a project contains
// requirements, and each requirement can recruit temporary teams.
package model

import (
	"regexp"
	"strings"
)

type Project struct {
	Root          string `json:"root"`
	Key           string `json:"key"`
	Name          string `json:"name"`
	DefaultBranch string `json:"default_branch,omitempty"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

type RequirementState string

const (
	RequirementOpen       RequirementState = "open"
	RequirementInProgress RequirementState = "in_progress"
	RequirementDone       RequirementState = "done"
	RequirementClosed     RequirementState = "closed"
)

type Requirement struct {
	ID              string           `json:"id"`
	Goal            string           `json:"goal"`
	State           RequirementState `json:"state"`
	Stage           string           `json:"stage,omitempty"`
	Workspace       string           `json:"workspace,omitempty"`
	Branch          string           `json:"branch,omitempty"`
	BaseRef         string           `json:"base_ref,omitempty"`
	WorktreeManaged bool             `json:"worktree_managed,omitempty"`
	CreatedAt       string           `json:"created_at"`
	UpdatedAt       string           `json:"updated_at"`
	LastEvent       string           `json:"last_event,omitempty"`
}

func (r Requirement) IsOpen() bool {
	return r.State != RequirementClosed && r.State != RequirementDone
}

type TeamState string

const (
	TeamDraft     TeamState = "draft"
	TeamActive    TeamState = "active"
	TeamBlocked   TeamState = "blocked"
	TeamSubmitted TeamState = "submitted"
	TeamRework    TeamState = "rework"
	TeamAccepted  TeamState = "accepted"
	TeamDisbanded TeamState = "disbanded"
	TeamLost      TeamState = "lost"
)

type Team struct {
	ID              string    `json:"id"`
	RequirementID   string    `json:"requirement_id"`
	Purpose         string    `json:"purpose"`
	State           TeamState `json:"state"`
	ContractPath    string    `json:"contract_path,omitempty"`
	ContractSHA     string    `json:"contract_sha,omitempty"`
	ResultPath      string    `json:"result_path,omitempty"`
	ResultSHA       string    `json:"result_sha,omitempty"`
	Workspace       string    `json:"workspace,omitempty"`
	Size            int       `json:"size"`
	PaneID          string    `json:"pane_id,omitempty"`
	TabID           string    `json:"tab_id,omitempty"`
	WorkspaceID     string    `json:"workspace_id,omitempty"`
	LeadAgent       string    `json:"lead_agent,omitempty"`
	ReworkCount     int       `json:"rework_count,omitempty"`
	LastIssues      string    `json:"last_issues,omitempty"`
	BlockedQuestion string    `json:"blocked_question,omitempty"`
	CreatedAt       string    `json:"created_at"`
	UpdatedAt       string    `json:"updated_at"`
	SubmittedAt     string    `json:"submitted_at,omitempty"`
	AcceptedAt      string    `json:"accepted_at,omitempty"`
	DisbandedAt     string    `json:"disbanded_at,omitempty"`
	LastEvent       string    `json:"last_event,omitempty"`
}

// IsOpen reports whether the team still occupies the requirement's workspace
// and should block recruiting another team for the same requirement.
func (t Team) IsOpen() bool {
	switch t.State {
	case TeamAccepted, TeamDisbanded:
		return false
	default:
		return true
	}
}

func (t Team) CanSubmit() bool {
	return t.State == TeamActive || t.State == TeamRework
}

func (t Team) CanAccept() bool {
	return t.State == TeamSubmitted
}

func (t Team) CanRework() bool {
	return t.State == TeamSubmitted
}

func (t Team) CanAnswer() bool {
	return t.State == TeamBlocked
}

func (t Team) CanDisband(force bool) bool {
	if force {
		return t.State != TeamDisbanded
	}
	return t.State == TeamAccepted
}

// LeadAgentName is the Herdr agent name for a team lead. Herdr agent names are
// unique among live agents per server, while team IDs are per project, so the
// project key's short hash is included to keep names unique across projects.
func LeadAgentName(projectKey, teamID string) string {
	name := strings.ToLower(teamID) + "-" + projectShortID(projectKey) + "-lead"
	if len(name) > 32 {
		name = strings.TrimRight(name[:32], "-_")
	}
	return name
}

func projectShortID(key string) string {
	if i := strings.LastIndex(key, "-"); i >= 0 {
		if suffix := key[i+1:]; len(suffix) >= 4 && len(suffix) <= 12 {
			return suffix
		}
	}
	if len(key) > 8 {
		return key[:8]
	}
	return key
}

type MemberState string

const (
	MemberActive   MemberState = "active"
	MemberBlocked  MemberState = "blocked"
	MemberAccepted MemberState = "accepted"
	MemberRework   MemberState = "rework"
	MemberLost     MemberState = "lost"
	MemberClosed   MemberState = "closed"
)

// Member is one employee in a team. Employees are Herdr agents in their own
// panes inside the team tab; they are not Claude Code subagents.
type Member struct {
	TeamID     string      `json:"team_id"`
	AgentName  string      `json:"agent_name"`
	Name       string      `json:"name"`
	Role       string      `json:"role"`
	PaneID     string      `json:"pane_id,omitempty"`
	TaskPath   string      `json:"task_path,omitempty"`
	ReportPath string      `json:"report_path,omitempty"`
	State      MemberState `json:"state"`
	LastIssues string      `json:"last_issues,omitempty"`
	CreatedAt  string      `json:"created_at"`
	UpdatedAt  string      `json:"updated_at"`
	AcceptedAt string      `json:"accepted_at,omitempty"`
	ClosedAt   string      `json:"closed_at,omitempty"`
	LastEvent  string      `json:"last_event,omitempty"`
}

func MemberKey(teamID, agentName string) string { return teamID + "/" + agentName }

func (m Member) Key() string { return MemberKey(m.TeamID, m.AgentName) }

// NeedsAttention reports whether the lead still has to accept, rework or close
// this member before the team can be submitted.
func (m Member) NeedsAttention() bool {
	switch m.State {
	case MemberActive, MemberBlocked, MemberRework:
		return true
	default:
		return false
	}
}

var agentNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)

// ValidAgentName matches Herdr's agent-name rule: lowercase letter first, then
// lowercase letters, digits, '-' or '_', at most 32 characters.
func ValidAgentName(name string) bool { return agentNamePattern.MatchString(name) }

// Event is one append-only ledger line.
type Event struct {
	TS     string         `json:"ts"`
	Entity string         `json:"entity"`
	ID     string         `json:"id"`
	Event  string         `json:"event"`
	By     string         `json:"by,omitempty"`
	Data   map[string]any `json:"data"`
}
