// Package assets embeds the runtime files that `weefleet install` writes.
package assets

import "embed"

//go:embed files
var FS embed.FS

const (
	CoordinatorSkill  = "files/skills/weefleet-coordinator/SKILL.md"
	SlashCommand      = "files/commands/weefleet.md"
	BriefTemplate     = "files/templates/brief.md"
	ChecksTemplate    = "files/templates/checks.json"
	CommitTemplate    = "files/templates/commit.txt"
	ClaudeMDBlock     = "files/claude-md-block.md"
	CoordinatorPrompt = "files/coordinator/system-prompt.md"
)

// Read returns an embedded runtime file.
func Read(name string) ([]byte, error) {
	return FS.ReadFile(name)
}
