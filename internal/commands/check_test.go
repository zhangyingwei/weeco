package commands

import (
	"path/filepath"
	"strings"
	"testing"

	"weeco/internal/store"
	"weeco/internal/sysutil"
)

func TestTeamCheckRecordsGitAndRequiredFiles(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init")
	runGit(t, root, "config", "user.email", "test@example.com")
	runGit(t, root, "config", "user.name", "Test")
	project, _ := setupProject(t, root)
	if code := Run([]string{"requirement", "new", "--goal", "check git evidence"}); code != 0 {
		t.Fatalf("requirement new failed: %d", code)
	}
	reqIDs, _ := store.ListRequirementIDs(project.Key)
	reqID := reqIDs[0]
	writeTestFile(t, filepath.Join(root, "README.md"), "hello\n")
	writeTestFile(t, filepath.Join(root, "contract.md"), "# Contract\n\nCheck evidence.\n")
	writeTestFile(t, filepath.Join(root, "acceptance.json"), `{
  "commands": [{"id":"A1","cmd":"true","expect_exit":0,"timeout_s":5}],
  "required_files": ["README.md"],
  "require_clean_tree": true,
  "record_git": true
}`)
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-m", "init")
	if code := Run([]string{"team", "recruit", "--requirement", reqID, "--purpose", "design", "--contract", "contract.md", "--acceptance", "acceptance.json", "--size", "1"}); code != 0 {
		t.Fatalf("team recruit failed: %d", code)
	}
	teamIDs, _ := store.ListTeamIDs(project.Key)
	teamID := teamIDs[0]
	if code := Run([]string{"team", "check", teamID}); code != 0 {
		t.Fatalf("team check failed: %d", code)
	}
	var result checkResult
	if err := sysutil.ReadJSON(filepath.Join(store.TeamDir(project.Key, teamID), "check.json"), &result); err != nil {
		t.Fatal(err)
	}
	if !result.Passed {
		t.Fatalf("check should pass: %+v", result)
	}
	if result.Git == nil || !result.Git.Clean || result.Git.Commit == "" || result.Git.Tree == "" {
		t.Fatalf("git evidence missing: %+v", result.Git)
	}
	if len(result.Files) != 1 || !result.Files[0].Exists || result.Files[0].SHA256 == "" {
		t.Fatalf("required file evidence missing: %+v", result.Files)
	}

	writeTestFile(t, filepath.Join(root, "untracked.txt"), "dirty\n")
	if code := Run([]string{"team", "check", teamID}); code != 0 {
		t.Fatalf("second team check failed: %d", code)
	}
	if err := sysutil.ReadJSON(filepath.Join(store.TeamDir(project.Key, teamID), "check.json"), &result); err != nil {
		t.Fatal(err)
	}
	if result.Passed || result.Git == nil || result.Git.Clean {
		t.Fatalf("dirty tree should fail clean-tree check: %+v", result)
	}
	if !strings.Contains(result.Git.Status, "untracked.txt") {
		t.Fatalf("git status should include untracked file: %s", result.Git.Status)
	}
}
