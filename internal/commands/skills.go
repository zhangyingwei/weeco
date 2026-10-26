package commands

import (
	"weeco/internal/assets"
	"weeco/internal/store"
	"weeco/internal/sysutil"
)

type skillAsset struct {
	name  string
	asset string
	label string
}

// weecoSKills is the single list used by install, doctor and init. Keeping it
// in one place prevents the three commands from disagreeing about a missing
// skill.
var weecoSKills = []skillAsset{
	{"weeco-boss", assets.BossSkill, "boss skill"},
	{"weeco-requirements", assets.RequirementsSkill, "requirements skill"},
	{"weeco-design", assets.DesignSkill, "design skill"},
	{"weeco-plan", assets.PlanSkill, "plan skill"},
	{"weeco-implement", assets.ImplementSkill, "implement skill"},
	{"weeco-review", assets.ReviewSkill, "review skill"},
	{"weeco-deliver", assets.DeliverSkill, "deliver skill"},
	{"weeco-team-lead", assets.TeamLeadSkill, "team lead skill"},
}

func missingWeecoSKills() []string {
	var missing []string
	for _, skill := range weecoSKills {
		if !sysutil.FileExists(store.SkillPath(skill.name)) {
			missing = append(missing, skill.name)
		}
	}
	return missing
}
