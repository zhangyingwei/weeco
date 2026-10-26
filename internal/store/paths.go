// Package store owns project registration, paths, the append-only ledger and
// the fold functions that turn ledger events into current snapshots.
package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"

	"weeco/internal/model"
	"weeco/internal/sysutil"
)

type Config struct {
	WorktreeRoot           *string `json:"worktree_root"`
	AgentKind              string  `json:"agent_kind"`
	ReadyTimeoutMS         int     `json:"ready_timeout_ms"`
	WaitTimeoutS           float64 `json:"wait_timeout_s"`
	TeamLeadPermissionMode string  `json:"team_lead_permission_mode"`
}

func DefaultConfig() Config {
	return Config{
		AgentKind:              "claude",
		ReadyTimeoutMS:         60000,
		WaitTimeoutS:           1800,
		TeamLeadPermissionMode: "bypassPermissions",
	}
}

func Home() string {
	raw := os.Getenv("WEECO_HOME")
	if raw == "" {
		raw = "~/.weeco"
	}
	if p, err := sysutil.AbsPath(raw); err == nil {
		return p
	}
	return filepath.Clean(sysutil.ExpandHome(raw))
}

func LoadConfig() Config {
	cfg := DefaultConfig()
	data, err := os.ReadFile(filepath.Join(Home(), "config.json"))
	if err != nil {
		return cfg
	}
	var raw map[string]json.RawMessage
	if json.Unmarshal(data, &raw) != nil {
		return cfg
	}
	if v, ok := raw["worktree_root"]; ok {
		var root *string
		if json.Unmarshal(v, &root) == nil {
			cfg.WorktreeRoot = root
		}
	}
	if v, ok := raw["agent_kind"]; ok {
		var kind string
		if json.Unmarshal(v, &kind) == nil && kind != "" {
			cfg.AgentKind = kind
		}
	}
	if v, ok := raw["ready_timeout_ms"]; ok {
		var n int
		if json.Unmarshal(v, &n) == nil && n > 0 {
			cfg.ReadyTimeoutMS = n
		}
	}
	if v, ok := raw["wait_timeout_s"]; ok {
		var n float64
		if json.Unmarshal(v, &n) == nil && n > 0 {
			cfg.WaitTimeoutS = n
		}
	}
	if v, ok := raw["team_lead_permission_mode"]; ok {
		var mode string
		if json.Unmarshal(v, &mode) == nil && mode != "" {
			cfg.TeamLeadPermissionMode = mode
		}
	}
	return cfg
}

func EnsureConfig() error {
	path := filepath.Join(Home(), "config.json")
	if sysutil.FileExists(path) {
		return nil
	}
	return sysutil.WriteJSON(path, DefaultConfig())
}

var nonKeyChars = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func ProjectKey(root string) string {
	abs, err := filepath.Abs(root)
	if err != nil {
		abs = root
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	}
	base := strings.Trim(nonKeyChars.ReplaceAllString(filepath.Base(abs), "-"), "-")
	if base == "" {
		base = "project"
	}
	return base + "-" + sysutil.SHA1Prefix(abs, 8)
}

func ProjectsDir() string { return filepath.Join(Home(), "projects") }

func ProjectDir(key string) string { return filepath.Join(ProjectsDir(), key) }

func ProjectFile(key string) string { return filepath.Join(ProjectDir(key), "project.json") }

func LedgerPath(key string) string { return filepath.Join(ProjectDir(key), "ledger.jsonl") }

func LockPath(key string) string { return filepath.Join(ProjectDir(key), "lock") }

func RequirementDir(key, reqID string) string {
	return filepath.Join(ProjectDir(key), "requirements", reqID)
}

func TeamDir(key, teamID string) string {
	return filepath.Join(ProjectDir(key), "teams", teamID)
}

func WorktreeRoot() string {
	cfg := LoadConfig()
	if cfg.WorktreeRoot != nil && strings.TrimSpace(*cfg.WorktreeRoot) != "" {
		if root, err := sysutil.AbsPath(*cfg.WorktreeRoot); err == nil {
			return root
		}
		return filepath.Join(sysutil.ExpandHome(*cfg.WorktreeRoot))
	}
	return filepath.Join(Home(), "worktrees")
}

func WorktreePath(key, reqID string) string {
	return filepath.Join(WorktreeRoot(), key, reqID)
}

func BinDir() string {
	if v := os.Getenv("WEECO_BIN_DIR"); v != "" {
		return sysutil.ExpandHome(v)
	}
	return filepath.Join(sysutil.HomeDir(), ".local", "bin")
}

func SkillDir() string {
	if v := os.Getenv("WEECO_SKILL_DIR"); v != "" {
		return sysutil.ExpandHome(v)
	}
	return filepath.Join(sysutil.HomeDir(), ".claude", "skills")
}

func CommandDir() string {
	if v := os.Getenv("WEECO_COMMAND_DIR"); v != "" {
		return sysutil.ExpandHome(v)
	}
	return filepath.Join(sysutil.HomeDir(), ".claude", "commands")
}

func TemplateDir() string { return filepath.Join(Home(), "templates") }

func SkillPath(name string) string { return filepath.Join(SkillDir(), name, "SKILL.md") }

func TemplatePath(name string) string { return filepath.Join(TemplateDir(), name) }

func ManifestPath() string { return filepath.Join(Home(), "installed.json") }

func InitProject(root, name string) (model.Project, error) {
	abs, err := sysutil.AbsPath(root)
	if err != nil {
		return model.Project{}, err
	}
	if name == "" {
		name = filepath.Base(abs)
	}
	key := ProjectKey(abs)
	dir := ProjectDir(key)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return model.Project{}, err
	}
	var existing model.Project
	_, _ = sysutil.ReadJSONOr(ProjectFile(key), &existing)
	createdAt := existing.CreatedAt
	if createdAt == "" {
		createdAt = sysutil.NowISO()
	}
	project := model.Project{
		Root:          abs,
		Key:           key,
		Name:          name,
		DefaultBranch: gitDefaultBranch(abs),
		CreatedAt:     createdAt,
		UpdatedAt:     sysutil.NowISO(),
	}
	if err := sysutil.WriteJSON(ProjectFile(key), project); err != nil {
		return model.Project{}, err
	}
	for _, name := range []string{"ledger.jsonl", "lock"} {
		path := filepath.Join(dir, name)
		if !sysutil.FileExists(path) {
			if err := os.WriteFile(path, nil, 0o644); err != nil {
				return model.Project{}, err
			}
		}
	}
	return project, nil
}

func FindProject(start string) (model.Project, error) {
	if start == "" {
		var err error
		start, err = os.Getwd()
		if err != nil {
			return model.Project{}, err
		}
	}
	abs, err := sysutil.AbsPath(start)
	if err != nil {
		return model.Project{}, err
	}
	if info, err := os.Stat(abs); err == nil && !info.IsDir() {
		abs = filepath.Dir(abs)
	}
	for dir := abs; ; dir = filepath.Dir(dir) {
		key := ProjectKey(dir)
		var project model.Project
		ok, err := sysutil.ReadJSONOr(ProjectFile(key), &project)
		if err != nil {
			return model.Project{}, err
		}
		if ok && project.Root != "" {
			return project, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
	}
	return model.Project{}, os.ErrNotExist
}

func LoadProject(key string) (model.Project, error) {
	var project model.Project
	if err := sysutil.ReadJSON(ProjectFile(key), &project); err != nil {
		return project, err
	}
	return project, nil
}

func WithLock(key string, fn func() error) error {
	path := LockPath(key)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	return fn()
}

func gitDefaultBranch(root string) string {
	res, err := sysutil.Run([]string{"git", "-C", root, "symbolic-ref", "--quiet", "--short", "HEAD"}, "", 0, nil)
	if err == nil && res.ExitCode == 0 {
		if branch := strings.TrimSpace(res.Stdout); branch != "" {
			return branch
		}
	}
	res, err = sysutil.Run([]string{"git", "-C", root, "symbolic-ref", "--quiet", "--short", "refs/remotes/origin/HEAD"}, "", 0, nil)
	if err == nil && res.ExitCode == 0 {
		out := strings.TrimSpace(res.Stdout)
		if idx := strings.LastIndex(out, "/"); idx >= 0 {
			return out[idx+1:]
		}
		if out != "" {
			return out
		}
	}
	return ""
}
