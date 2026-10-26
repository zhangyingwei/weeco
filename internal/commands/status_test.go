package commands

import (
	"strings"
	"testing"

	"weeco/internal/model"
)

func TestNextSkillFor(t *testing.T) {
	cases := []struct {
		name string
		req  model.Requirement
		team *model.Team
		want string
	}{
		{"new requirement", model.Requirement{ID: "R-1", State: model.RequirementOpen, Stage: "requirement"}, nil, "weeco-design"},
		{"design", model.Requirement{ID: "R-1", State: model.RequirementOpen, Stage: "design"}, nil, "weeco-design"},
		{"plan", model.Requirement{ID: "R-1", State: model.RequirementOpen, Stage: "plan"}, nil, "weeco-plan"},
		{"implement", model.Requirement{ID: "R-1", State: model.RequirementOpen, Stage: "implement"}, nil, "weeco-implement"},
		{"delivery", model.Requirement{ID: "R-1", State: model.RequirementOpen, Stage: "delivery"}, nil, "weeco-deliver"},
		{"active team", model.Requirement{ID: "R-1", State: model.RequirementOpen, Stage: "design"}, &model.Team{ID: "T-1", State: model.TeamActive}, "weeco-review"},
		{"submitted team", model.Requirement{ID: "R-1", State: model.RequirementOpen, Stage: "design"}, &model.Team{ID: "T-1", State: model.TeamSubmitted}, "weeco-review"},
		{"closed requirement", model.Requirement{ID: "R-1", State: model.RequirementClosed, Stage: "delivery"}, nil, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := nextSkillFor(tc.req, tc.team); got != tc.want {
				t.Fatalf("nextSkillFor = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestNextCommandsForSubmittedTeam(t *testing.T) {
	req := model.Requirement{ID: "R-1", State: model.RequirementOpen, Stage: "design"}
	team := &model.Team{ID: "T-1", State: model.TeamSubmitted}
	commands := strings.Join(nextCommandsFor(req, team), "\n")
	for _, want := range []string{"team check T-1", "team accept T-1", "team rework T-1"} {
		if !strings.Contains(commands, want) {
			t.Fatalf("next commands missing %q:\n%s", want, commands)
		}
	}
}
