package model

import (
	"strings"
	"testing"
)

func TestLeadAgentNameIsProjectScoped(t *testing.T) {
	teamID := "T-20261027-01"
	a := LeadAgentName("weeco-test-aaaa1111", teamID)
	b := LeadAgentName("weeco-test-bbbb2222", teamID)
	if a == b {
		t.Fatalf("lead agent names collide across projects: %s", a)
	}
	if a != LeadAgentName("weeco-test-aaaa1111", teamID) {
		t.Fatalf("lead agent name is not stable: %s", a)
	}
	if len(a) > 32 || len(b) > 32 {
		t.Fatalf("lead agent name too long: %q %q", a, b)
	}
	if !ValidAgentName(a) || !ValidAgentName(b) {
		t.Fatalf("lead agent name is invalid: %q %q", a, b)
	}
	if !strings.HasSuffix(a, "-lead") {
		t.Fatalf("lead agent name should end with -lead: %s", a)
	}
	if !strings.Contains(a, "aaaa1111") {
		t.Fatalf("lead agent name should include the project short id: %s", a)
	}
}

func TestLeadAgentNameTrimsLongTeamID(t *testing.T) {
	name := LeadAgentName("project-abcdef12", "T-20261027-1234")
	if len(name) > 32 {
		t.Fatalf("lead agent name too long: %d %q", len(name), name)
	}
	if !ValidAgentName(name) {
		t.Fatalf("lead agent name is invalid: %q", name)
	}
}
