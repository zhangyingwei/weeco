// Package config loads the small user-facing model configuration.
//
// Only two things are configurable: the model candidates for each level and
// nothing else. Defaults, retry policy, guard files and role presets stay in
// code so a mistyped user file cannot silently change runtime behavior.
package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"weeco/internal/store"
)

const DefaultLevel = "mid"

var LevelNames = []string{"senior", "mid", "junior", "temp"}

var defaultEffort = map[string]string{
	"senior": "high",
	"mid":    "medium",
	"junior": "low",
	"temp":   "low",
}

var validAgents = map[string]bool{"claude": true, "codex": true, "pi": true}
var validEfforts = map[string]bool{"low": true, "medium": true, "high": true, "none": true}

// Candidate is one model attempt for a level. Agent defaults to claude.
type Candidate struct {
	Agent  string `json:"agent,omitempty"`
	Model  string `json:"model"`
	Effort string `json:"effort,omitempty"`
}

// Config is the resolved set of level candidates.
type Config struct {
	Levels  map[string][]Candidate
	Sources []string
	Legacy  []string
}

type fileConfig struct {
	Levels map[string][]Candidate `json:"levels"`

	// Fields written by v0.11 and earlier. They are accepted so an existing
	// ~/.weeco/config.json does not block startup, but they are ignored: the
	// current runtime uses its own defaults for these settings.
	WorktreeRoot           any    `json:"worktree_root,omitempty"`
	AgentKind              string `json:"agent_kind,omitempty"`
	ReadyTimeoutMS         int    `json:"ready_timeout_ms,omitempty"`
	WaitTimeoutS           int    `json:"wait_timeout_s,omitempty"`
	TeamLeadPermissionMode string `json:"team_lead_permission_mode,omitempty"`
}

// Builtin is the behavior of weeco without any configuration file: Claude with
// the CLI's own default model and no extra model flags.
func Builtin() *Config {
	c := &Config{Levels: make(map[string][]Candidate, len(LevelNames))}
	for _, level := range LevelNames {
		c.Levels[level] = []Candidate{{Agent: "claude", Model: "", Effort: "none"}}
	}
	return c
}

// Load reads the global and project configuration. Missing files are fine;
// malformed files are not.
func Load(projectDir string) (*Config, error) {
	global := filepath.Join(store.Home(), "config.json")
	project := filepath.Join(projectDir, ".weeco", "config.json")
	return LoadFiles(global, project)
}

// LoadFiles is Load with explicit paths, used by tests and config check.
func LoadFiles(globalPath, projectPath string) (*Config, error) {
	cfg := Builtin()
	for _, path := range []string{globalPath, projectPath} {
		if path == "" {
			continue
		}
		fc, err := readFile(path)
		if err != nil {
			return nil, err
		}
		if fc == nil {
			continue
		}
		if err := validateFile(path, fc); err != nil {
			return nil, err
		}
		cfg.Legacy = append(cfg.Legacy, legacyFields(fc)...)
		for level, candidates := range fc.Levels {
			cfg.Levels[level] = applyDefaults(level, candidates)
		}
		cfg.Sources = append(cfg.Sources, path)
	}
	return cfg, nil
}

func readFile(path string) (*fileConfig, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, fmt.Errorf("%s: config file is empty", path)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var fc fileConfig
	if err := dec.Decode(&fc); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%s: unexpected data after the config object", path)
	}
	return &fc, nil
}

func validateFile(path string, fc *fileConfig) error {
	for level, candidates := range fc.Levels {
		if !validLevel(level) {
			return fmt.Errorf("%s: unknown level %q (allowed: %s)", path, level, strings.Join(LevelNames, ", "))
		}
		if len(candidates) == 0 {
			return fmt.Errorf("%s: level %q has no candidates", path, level)
		}
		for i, candidate := range candidates {
			if candidate.Agent == "" {
				candidate.Agent = "claude"
			}
			if !validAgents[candidate.Agent] {
				return fmt.Errorf("%s: levels.%s[%d].agent must be claude, codex or pi", path, level, i)
			}
			if strings.TrimSpace(candidate.Model) == "" {
				return fmt.Errorf("%s: levels.%s[%d].model is required", path, level, i)
			}
			if candidate.Effort != "" && !validEfforts[candidate.Effort] {
				return fmt.Errorf("%s: levels.%s[%d].effort must be low, medium, high or none", path, level, i)
			}
			fc.Levels[level][i] = candidate
		}
	}
	return nil
}

func applyDefaults(level string, candidates []Candidate) []Candidate {
	out := make([]Candidate, len(candidates))
	for i, candidate := range candidates {
		if candidate.Agent == "" {
			candidate.Agent = "claude"
		}
		if candidate.Effort == "" {
			candidate.Effort = defaultEffort[level]
		}
		out[i] = candidate
	}
	return out
}

func legacyFields(fc *fileConfig) []string {
	var fields []string
	if fc.WorktreeRoot != nil {
		fields = append(fields, "worktree_root")
	}
	if fc.AgentKind != "" {
		fields = append(fields, "agent_kind")
	}
	if fc.ReadyTimeoutMS != 0 {
		fields = append(fields, "ready_timeout_ms")
	}
	if fc.WaitTimeoutS != 0 {
		fields = append(fields, "wait_timeout_s")
	}
	if fc.TeamLeadPermissionMode != "" {
		fields = append(fields, "team_lead_permission_mode")
	}
	return fields
}

func validLevel(level string) bool {
	for _, name := range LevelNames {
		if name == level {
			return true
		}
	}
	return false
}

// ValidLevel reports whether level is one of the four built-in names.
func ValidLevel(level string) bool { return validLevel(level) }

// Resolve returns the ordered candidates for a level, optionally filtered by
// agent and model. It fails rather than silently falling back to a different
// model when an explicit filter has no match.
func (c *Config) Resolve(level, agent, model string) ([]Candidate, error) {
	if level == "" {
		level = DefaultLevel
	}
	candidates, ok := c.Levels[level]
	if !ok || len(candidates) == 0 {
		return nil, fmt.Errorf("unknown level %q (allowed: %s)", level, strings.Join(LevelNames, ", "))
	}
	var out []Candidate
	for _, candidate := range candidates {
		if agent != "" && candidate.Agent != agent {
			continue
		}
		if model != "" && candidate.Model != model {
			continue
		}
		out = append(out, candidate)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no candidate in level %q matches agent=%q model=%q; available: %s",
			level, agent, model, FormatCandidates(candidates))
	}
	return out, nil
}

// FormatCandidates renders candidates for errors and config check.
func FormatCandidates(candidates []Candidate) string {
	parts := make([]string, 0, len(candidates))
	for i, candidate := range candidates {
		parts = append(parts, FormatCandidate(candidate, i > 0))
	}
	return strings.Join(parts, " -> ")
}

// FormatCandidate renders one candidate. fallback marks non-first entries.
func FormatCandidate(candidate Candidate, fallback bool) string {
	model := candidate.Model
	if model == "" {
		model = "<CLI default>"
	}
	suffix := ""
	if fallback {
		suffix = ", fallback"
	}
	return fmt.Sprintf("%s (agent=%s, effort=%s%s)", model, candidate.Agent, candidate.Effort, suffix)
}

// Describe is the human-readable result of config check.
func (c *Config) Describe() string {
	source := "built-in"
	if len(c.Sources) > 0 {
		source = strings.Join(c.Sources, " + ")
	}
	var b strings.Builder
	fmt.Fprintf(&b, "source: %s\n", source)
	if len(c.Legacy) > 0 {
		fmt.Fprintf(&b, "legacy: ignored fields: %s\n", strings.Join(c.Legacy, ", "))
	}
	for _, level := range LevelNames {
		fmt.Fprintf(&b, "%-7s %s\n", level+":", FormatCandidates(c.Levels[level]))
	}
	return b.String()
}

// Template is written by config init. It is intentionally valid: an empty
// levels map means "use the built-in defaults".
func Template() []byte {
	data, _ := json.MarshalIndent(fileConfig{Levels: map[string][]Candidate{}}, "", "  ")
	return append(data, '\n')
}

// Example prints a copyable shape without writing invalid placeholder values.
const Example = `{
  "levels": {
    "senior": [
      { "model": "<模型名>", "effort": "high" },
      { "agent": "codex", "model": "<另一个模型名>" }
    ],
    "mid":    [ { "model": "<模型名>", "effort": "medium" } ],
    "junior": [ { "model": "<模型名>", "effort": "low" } ],
    "temp":   [ { "agent": "pi", "model": "<模型名>" } ]
  }
}`
