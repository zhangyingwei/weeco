// Package assets embeds the runtime files that `weeco install` writes and
// the prompts that `weeco team recruit` injects into a team lead.
package assets

import "embed"

//go:embed files
var FS embed.FS

const (
	BossSkill          = "files/skills/weeco-boss/SKILL.md"
	RequirementsSkill  = "files/skills/weeco-requirements/SKILL.md"
	DesignSkill        = "files/skills/weeco-design/SKILL.md"
	PlanSkill          = "files/skills/weeco-plan/SKILL.md"
	ImplementSkill     = "files/skills/weeco-implement/SKILL.md"
	ReviewSkill        = "files/skills/weeco-review/SKILL.md"
	DeliverSkill       = "files/skills/weeco-deliver/SKILL.md"
	TeamLeadSkill      = "files/skills/weeco-team-lead/SKILL.md"
	TeamLeadPrompt     = "files/team-lead/system-prompt.md"
	TeamMemberPrompt   = "files/team-member/system-prompt.md"
	SlashCommand       = "files/commands/weeco.md"
	ContractTemplate   = "files/templates/contract.md"
	ResultTemplate     = "files/templates/result.md"
	AcceptanceTemplate = "files/templates/acceptance.json"
	LessonsTemplate    = "files/templates/lessons.md"
	ClaudeMDBlock      = "files/claude-md-block.md"
)

func Read(name string) ([]byte, error) {
	return FS.ReadFile(name)
}
