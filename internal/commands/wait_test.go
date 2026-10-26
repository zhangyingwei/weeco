package commands

import (
	"testing"
	"time"

	"weeco/internal/store"
)

// The boss's background wait must not return just because the lead agent is
// idle between turns. It must stay alive until the team state changes.
func TestTeamWaitDoesNotReturnOnIdleLead(t *testing.T) {
	root := t.TempDir()
	project, _ := setupProject(t, root)
	if code := Run([]string{"requirement", "new", "--goal", "wait for submit"}); code != 0 {
		t.Fatalf("requirement new failed: %d", code)
	}
	reqIDs, _ := store.ListRequirementIDs(project.Key)
	writeTestFile(t, root+"/contract.md", "# Contract\n\nWait.\n")
	if code := Run([]string{"team", "recruit", "--requirement", reqIDs[0], "--contract", "contract.md", "--size", "1"}); code != 0 {
		t.Fatalf("team recruit failed: %d", code)
	}
	teamIDs, _ := store.ListTeamIDs(project.Key)
	teamID := teamIDs[0]

	done := make(chan int, 1)
	go func() { done <- waitForTeam(project, teamID, 5) }()

	select {
	case <-done:
		t.Fatal("team wait returned while the lead was merely idle")
	case <-time.After(300 * time.Millisecond):
	}

	if err := store.WithLock(project.Key, func() error {
		return store.Append(project.Key, "team", teamID, "team_submitted", "lead", map[string]any{})
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("team wait returned %d", code)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("team wait did not observe team_submitted")
	}
}
