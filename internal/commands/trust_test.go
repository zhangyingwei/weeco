package commands

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"weeco/internal/store"
	"weeco/internal/sysutil"
)

func TestClaudeTrustWritesConfigAndBackup(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "claude.json")
	writeTestFile(t, configPath, `{"projects":{"other":{"hasTrustDialogAccepted":false}},"keep":1}`)
	t.Setenv("WEECO_CLAUDE_CONFIG", configPath)

	dir := t.TempDir()
	root, workspace, backup, err := setClaudeTrust(dir)
	if err != nil {
		t.Fatal(err)
	}
	if root != workspace {
		t.Fatalf("non-git root and workspace should match: %q %q", root, workspace)
	}
	if backup == "" {
		t.Fatal("expected a one-time backup")
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	var top map[string]any
	if err := json.Unmarshal(data, &top); err != nil {
		t.Fatal(err)
	}
	if top["keep"] != float64(1) {
		t.Fatalf("unrelated config was not preserved: %v", top)
	}
	projects, _ := top["projects"].(map[string]any)
	entry, _ := projects[root].(map[string]any)
	if entry["hasTrustDialogAccepted"] != true {
		t.Fatalf("trust entry missing: %v", projects)
	}
	if other, _ := projects["other"].(map[string]any); other["hasTrustDialogAccepted"] != false {
		t.Fatalf("other project entry changed: %v", other)
	}

	gotRoot, trusted, err := claudeTrust(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !trusted || gotRoot != root {
		t.Fatalf("claudeTrust = %q, %v; want %q, true", gotRoot, trusted, root)
	}
}

func TestClaudeTrustUsesGitRootForWorktrees(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "claude.json")
	t.Setenv("WEECO_CLAUDE_CONFIG", configPath)

	repo := t.TempDir()
	runGit(t, repo, "init")
	runGit(t, repo, "config", "user.email", "test@example.com")
	runGit(t, repo, "config", "user.name", "Test")
	writeTestFile(t, filepath.Join(repo, "README.md"), "hello\n")
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", "init")
	worktree := filepath.Join(t.TempDir(), "wt")
	runGit(t, repo, "worktree", "add", "--detach", worktree)

	if _, _, _, err := setClaudeTrust(repo); err != nil {
		t.Fatal(err)
	}
	root, trusted, err := claudeTrust(worktree)
	if err != nil {
		t.Fatal(err)
	}
	if !trusted {
		t.Fatalf("linked worktree should inherit trust from the main repository")
	}
	canonicalRepo, err := sysutil.AbsPath(repo)
	if err != nil {
		t.Fatal(err)
	}
	if root != canonicalRepo {
		t.Fatalf("trust root = %q, want %q", root, canonicalRepo)
	}
}

func TestTeamRecruitAutoTrustsByDefault(t *testing.T) {
	root := t.TempDir()
	project, _ := setupProject(t, root)
	configPath := os.Getenv("WEECO_CLAUDE_CONFIG")
	writeTestFile(t, configPath, `{"projects":{}}`)
	if _, trusted, err := claudeTrust(project.Root); err != nil || trusted {
		t.Fatalf("test setup should be untrusted: trusted=%v err=%v", trusted, err)
	}
	reqID, contract := prepareTrustRecruit(t, root)

	recruit := []string{"team", "recruit", "--requirement", reqID, "--purpose", "design", "--contract", contract, "--size", "1"}
	if code := Run(recruit); code != 0 {
		t.Fatalf("team recruit should auto-trust: %d", code)
	}
	teamIDs, err := store.ListTeamIDs(project.Key)
	if err != nil || len(teamIDs) != 1 {
		t.Fatalf("team ids = %v, %v", teamIDs, err)
	}
	if _, trusted, err := claudeTrust(project.Root); err != nil || !trusted {
		t.Fatalf("workspace should be trusted after recruit: trusted=%v err=%v", trusted, err)
	}
}

func TestTeamRecruitNoTrustFails(t *testing.T) {
	root := t.TempDir()
	project, _ := setupProject(t, root)
	configPath := os.Getenv("WEECO_CLAUDE_CONFIG")
	writeTestFile(t, configPath, `{"projects":{}}`)
	reqID, contract := prepareTrustRecruit(t, root)

	recruit := []string{"team", "recruit", "--requirement", reqID, "--purpose", "design", "--contract", contract, "--size", "1", "--no-trust"}
	if code := Run(recruit); code == 0 {
		t.Fatal("team recruit --no-trust should fail when the workspace is not trusted")
	}
	teamIDs, err := store.ListTeamIDs(project.Key)
	if err != nil {
		t.Fatal(err)
	}
	if len(teamIDs) != 0 {
		t.Fatalf("untrusted recruit should not create a team: %v", teamIDs)
	}
}

func TestInitTrustsProjectByDefault(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(t.TempDir(), "claude.json")
	t.Setenv("WEECO_CLAUDE_CONFIG", configPath)
	t.Setenv("WEECO_HOME", t.TempDir())
	if code := Run([]string{"init", "--root", root}); code != 0 {
		t.Fatalf("init failed: %d", code)
	}
	project, err := store.FindProject(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, trusted, err := claudeTrust(project.Root); err != nil || !trusted {
		t.Fatalf("init should trust the project: trusted=%v err=%v", trusted, err)
	}
}

func TestInitNoTrustSkips(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(t.TempDir(), "claude.json")
	t.Setenv("WEECO_CLAUDE_CONFIG", configPath)
	t.Setenv("WEECO_HOME", t.TempDir())
	if code := Run([]string{"init", "--root", root, "--no-trust"}); code != 0 {
		t.Fatalf("init failed: %d", code)
	}
	project, err := store.FindProject(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, trusted, err := claudeTrust(project.Root); err != nil || trusted {
		t.Fatalf("init --no-trust should not trust: trusted=%v err=%v", trusted, err)
	}
}

func prepareTrustRecruit(t *testing.T, root string) (string, string) {
	t.Helper()
	if code := Run([]string{"requirement", "new", "--goal", "trust gate"}); code != 0 {
		t.Fatalf("requirement new failed: %d", code)
	}
	project, err := store.FindProject(root)
	if err != nil {
		t.Fatal(err)
	}
	reqIDs, err := store.ListRequirementIDs(project.Key)
	if err != nil || len(reqIDs) != 1 {
		t.Fatalf("requirement ids = %v, %v", reqIDs, err)
	}
	writeTestFile(t, filepath.Join(root, "contract.md"), "# Contract\n\nTrust gate.\n")
	return reqIDs[0], "contract.md"
}
