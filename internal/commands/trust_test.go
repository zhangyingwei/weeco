package commands

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"weeco/internal/sysutil"
)

func readClaudeConfig(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	return cfg
}

func projectTrusted(t *testing.T, cfg map[string]any, dir string) bool {
	t.Helper()
	projects, _ := cfg["projects"].(map[string]any)
	entry, _ := projects[dir].(map[string]any)
	trusted, _ := entry["hasTrustDialogAccepted"].(bool)
	return trusted
}

func TestAcceptClaudeWorkspaceTrustCreatesConfig(t *testing.T) {
	cfgDir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", cfgDir)
	dir := t.TempDir()
	if err := acceptClaudeWorkspaceTrust(dir); err != nil {
		t.Fatal(err)
	}
	abs, err := sysutil.AbsPath(dir)
	if err != nil {
		t.Fatal(err)
	}
	cfg := readClaudeConfig(t, filepath.Join(cfgDir, ".claude.json"))
	if !projectTrusted(t, cfg, abs) {
		t.Fatalf("project %q is not trusted: %+v", abs, cfg)
	}
	projects, _ := cfg["projects"].(map[string]any)
	if len(projects) != 1 {
		t.Fatalf("non-repository dir should create one project entry: %+v", projects)
	}
}

func TestAcceptClaudeWorkspaceTrustPreservesConfig(t *testing.T) {
	cfgDir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", cfgDir)
	path := filepath.Join(cfgDir, ".claude.json")
	original := `{
  "numStartups": 5,
  "projects": {
    "/other/project": {"hasTrustDialogAccepted": true, "note": "keep"}
  }
}`
	if err := os.WriteFile(path, []byte(original), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := acceptClaudeWorkspaceTrust(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	cfg := readClaudeConfig(t, path)
	if cfg["numStartups"] != float64(5) {
		t.Fatalf("top-level setting lost: %+v", cfg)
	}
	if !projectTrusted(t, cfg, "/other/project") {
		t.Fatalf("existing trust entry lost: %+v", cfg)
	}
	projects, _ := cfg["projects"].(map[string]any)
	other, _ := projects["/other/project"].(map[string]any)
	if other["note"] != "keep" {
		t.Fatalf("existing project fields lost: %+v", other)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o640 {
			t.Fatalf("file mode = %o, want 640", info.Mode().Perm())
		}
	}
}

func TestAcceptClaudeWorkspaceTrustIsIdempotent(t *testing.T) {
	cfgDir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", cfgDir)
	path := filepath.Join(cfgDir, ".claude.json")
	dir := t.TempDir()
	if err := acceptClaudeWorkspaceTrust(dir); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := acceptClaudeWorkspaceTrust(dir); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatalf("second call rewrote the config:\n%s\n---\n%s", first, second)
	}
}

func TestAcceptClaudeWorkspaceTrustRefusesInvalidConfig(t *testing.T) {
	for name, content := range map[string]string{
		"invalid json":     `{"projects":`,
		"invalid projects": `{"projects":[]}`,
	} {
		t.Run(name, func(t *testing.T) {
			cfgDir := t.TempDir()
			t.Setenv("CLAUDE_CONFIG_DIR", cfgDir)
			path := filepath.Join(cfgDir, ".claude.json")
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := acceptClaudeWorkspaceTrust(t.TempDir()); err == nil {
				t.Fatal("invalid config was accepted")
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(data) != content {
				t.Fatalf("invalid config was overwritten: %s", data)
			}
		})
	}
}

func TestAcceptClaudeWorkspaceTrustCoversWorktreeMainCheckout(t *testing.T) {
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not available")
	}
	cfgDir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", cfgDir)
	repo := t.TempDir()
	runGit := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command(git, args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	runGit(repo, "init", "-q")
	if err := os.WriteFile(filepath.Join(repo, "file.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGit(repo, "add", ".")
	runGit(repo, "-c", "user.email=test@example.com", "-c", "user.name=Test", "commit", "-qm", "init")
	worktree := filepath.Join(t.TempDir(), "wt")
	runGit(repo, "worktree", "add", "-q", "--detach", worktree)

	if err := acceptClaudeWorkspaceTrust(worktree); err != nil {
		t.Fatal(err)
	}
	cfg := readClaudeConfig(t, filepath.Join(cfgDir, ".claude.json"))
	for _, dir := range []string{repo, worktree} {
		abs, err := sysutil.AbsPath(dir)
		if err != nil {
			t.Fatal(err)
		}
		if !projectTrusted(t, cfg, abs) {
			t.Fatalf("worktree trust for %q is missing: %+v", abs, cfg)
		}
	}
}

func TestClaudeTrustDirsRejectsHome(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("home directory resolution differs on Windows")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	if _, err := claudeTrustDirs(home); err == nil {
		t.Fatal("home directory should be rejected")
	}
}
