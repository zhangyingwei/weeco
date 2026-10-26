package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
)

func weefleetHome() string {
	raw := os.Getenv("WEEFLEET_HOME")
	if raw == "" {
		raw = "~/.weefleet"
	}
	p, err := absPath(raw)
	if err != nil {
		return filepath.Clean(expandHome(raw))
	}
	return p
}

func loadConfig() Config {
	cfg := defaultConfig()
	path := filepath.Join(weefleetHome(), "config.json")
	data, err := os.ReadFile(path)
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
	if v, ok := raw["budget"]; ok {
		var b Budget
		if json.Unmarshal(v, &b) == nil {
			if b.MaxTotalSeconds > 0 {
				cfg.Budget.MaxTotalSeconds = b.MaxTotalSeconds
			}
			if b.MaxRunsPerCheck > 0 {
				cfg.Budget.MaxRunsPerCheck = b.MaxRunsPerCheck
			}
			if b.MaxOutputLines > 0 {
				cfg.Budget.MaxOutputLines = b.MaxOutputLines
			}
		}
	}
	return cfg
}

func ensureConfig() error {
	path := filepath.Join(weefleetHome(), "config.json")
	if fileExists(path) {
		return nil
	}
	return writeJSON(path, map[string]any{
		"worktree_root": nil,
		"agent_kind":    defaultConfig().AgentKind,
	})
}

var nonKeyChars = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func repoKeyFor(repoRoot string) string {
	root, err := filepath.Abs(repoRoot)
	if err != nil {
		root = repoRoot
	}
	base := strings.Trim(nonKeyChars.ReplaceAllString(filepath.Base(root), "-"), "-")
	if base == "" {
		base = "repo"
	}
	// Keep the key stable across releases: it is the first 8 hex chars of
	// SHA-1 of the absolute repo path.
	return base + "-" + sha1Prefix(root, 8)
}

func repoDir(key string) string {
	return filepath.Join(weefleetHome(), "repos", key)
}

func taskDir(key, taskID string) string {
	return filepath.Join(repoDir(key), "tasks", taskID)
}

func ledgerPath(key string) string {
	return filepath.Join(repoDir(key), "ledger.jsonl")
}

func lockPath(key string) string {
	return filepath.Join(repoDir(key), "lock")
}

func worktreePath(key, taskID string) string {
	cfg := loadConfig()
	var root string
	if cfg.WorktreeRoot != nil && strings.TrimSpace(*cfg.WorktreeRoot) != "" {
		root = expandHome(*cfg.WorktreeRoot)
	} else {
		root = filepath.Join(weefleetHome(), "worktrees")
	}
	return filepath.Join(root, key, taskID)
}

// withRepoLock serializes mutations of one repo's ledger and task files.
func withRepoLock(key string, fn func() error) error {
	path := lockPath(key)
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

func requireInitialized(key string) (RepoMeta, error) {
	var meta RepoMeta
	path := filepath.Join(repoDir(key), "repo.json")
	ok, err := readJSONOr(path, &meta)
	if err != nil {
		return meta, err
	}
	if !ok || meta.RepoRoot == "" {
		return meta, errorf("repo %s is not initialized; run: weefleet init --repo <path>", key)
	}
	return meta, nil
}

// resolveRepo finds the git root for --repo (or cwd) and requires init.
func resolveRepo(repoFlag string) (string, string, RepoMeta, error) {
	start := repoFlag
	if start == "" {
		wd, err := os.Getwd()
		if err != nil {
			return "", "", RepoMeta{}, err
		}
		start = wd
	}
	start, err := absPath(start)
	if err != nil {
		return "", "", RepoMeta{}, err
	}
	root, err := gitRepoRoot(start)
	if err != nil {
		return "", "", RepoMeta{}, err
	}
	key := repoKeyFor(root)
	meta, err := requireInitialized(key)
	if err != nil {
		return "", "", RepoMeta{}, err
	}
	return root, key, meta, nil
}
