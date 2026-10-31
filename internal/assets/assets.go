// Package assets embeds the weeco Skill and its member instructions.
package assets

import "embed"

//go:embed files/skills/weeco/SKILL.md files/skills/weeco/member-prompt.md
var FS embed.FS

const (
	WeecoSkill   = "files/skills/weeco/SKILL.md"
	MemberPrompt = "files/skills/weeco/member-prompt.md"
)

func Read(name string) ([]byte, error) {
	return FS.ReadFile(name)
}
