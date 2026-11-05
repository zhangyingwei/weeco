package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestBuiltinDefaults(t *testing.T) {
	cfg := Builtin()
	for _, level := range LevelNames {
		got := cfg.Levels[level]
		if len(got) != 1 || got[0].Agent != "claude" || got[0].Model != "" || got[0].Effort != "none" {
			t.Fatalf("level %s default = %+v", level, got)
		}
	}
}

func TestLoadMissingFilesUsesDefaults(t *testing.T) {
	dir := t.TempDir()
	cfg, err := LoadFiles(filepath.Join(dir, "global.json"), filepath.Join(dir, "project.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Sources) != 0 {
		t.Fatalf("sources = %v", cfg.Sources)
	}
	if got := cfg.Levels["mid"]; len(got) != 1 || got[0].Model != "" {
		t.Fatalf("mid default = %+v", got)
	}
}

func TestProjectOverridesOnlyNamedLevels(t *testing.T) {
	dir := t.TempDir()
	global := filepath.Join(dir, "global.json")
	project := filepath.Join(dir, "project.json")
	writeConfig(t, global, `{"levels":{"senior":[{"model":"g-senior"}],"mid":[{"model":"g-mid"}]}}`)
	writeConfig(t, project, `{"levels":{"mid":[{"agent":"codex","model":"p-mid","effort":"none"}]}}`)

	cfg, err := LoadFiles(global, project)
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Levels["senior"]; len(got) != 1 || got[0].Model != "g-senior" {
		t.Fatalf("senior inherited = %+v", got)
	}
	if got := cfg.Levels["mid"]; len(got) != 1 || got[0].Model != "p-mid" || got[0].Agent != "codex" || got[0].Effort != "none" {
		t.Fatalf("mid override = %+v", got)
	}
	if len(cfg.Sources) != 2 {
		t.Fatalf("sources = %v", cfg.Sources)
	}
}

func TestEffortDefaultsPerLevel(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	writeConfig(t, path, `{"levels":{"senior":[{"model":"x"}],"mid":[{"model":"x"}],"junior":[{"model":"x"}],"temp":[{"model":"x"}]}}`)
	cfg, err := LoadFiles(path, "")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"senior": "high", "mid": "medium", "junior": "low", "temp": "low"}
	for level, effort := range want {
		if got := cfg.Levels[level][0].Effort; got != effort {
			t.Fatalf("%s effort = %q, want %q", level, got, effort)
		}
	}
}

func TestRejectsUnknownAndInvalidFields(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{"unknown top", `{"levels":{},"extra":1}`, `unknown field "extra"`},
		{"unknown level", `{"levels":{"huge":[{"model":"x"}]}}`, `unknown level "huge"`},
		{"unknown candidate", `{"levels":{"mid":[{"model":"x","strength":"high"}]}}`, `unknown field "strength"`},
		{"empty model", `{"levels":{"mid":[{"agent":"claude"}]}}`, "model is required"},
		{"bad agent", `{"levels":{"mid":[{"agent":"gemini","model":"x"}]}}`, "agent must be claude, codex or pi"},
		{"bad effort", `{"levels":{"mid":[{"model":"x","effort":"ultra"}]}}`, "effort must be low, medium, high or none"},
		{"empty level", `{"levels":{"mid":[]}}`, "has no candidates"},
		{"empty file", `   `, "config file is empty"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "config.json")
			writeConfig(t, path, tc.body)
			_, err := LoadFiles(path, "")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want containing %q", err, tc.want)
			}
		})
	}
}

func TestResolveFilters(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	writeConfig(t, path, `{"levels":{"mid":[
		{"model":"a"},
		{"agent":"codex","model":"b","effort":"none"},
		{"agent":"pi","model":"c","effort":"none"}
	]}}`)
	cfg, err := LoadFiles(path, "")
	if err != nil {
		t.Fatal(err)
	}
	if got, err := cfg.Resolve("mid", "", ""); err != nil || len(got) != 3 {
		t.Fatalf("all = %+v, %v", got, err)
	}
	if got, err := cfg.Resolve("mid", "codex", ""); err != nil || len(got) != 1 || got[0].Model != "b" {
		t.Fatalf("agent filter = %+v, %v", got, err)
	}
	if got, err := cfg.Resolve("mid", "", "c"); err != nil || len(got) != 1 || got[0].Agent != "pi" {
		t.Fatalf("model filter = %+v, %v", got, err)
	}
	if _, err := cfg.Resolve("mid", "", "missing"); err == nil || !strings.Contains(err.Error(), "available:") {
		t.Fatalf("missing model error = %v", err)
	}
	if _, err := cfg.Resolve("huge", "", ""); err == nil {
		t.Fatal("unknown level accepted")
	}
}

func TestLegacyGlobalFieldsAreAcceptedAndIgnored(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	writeConfig(t, path, `{
		"worktree_root": null,
		"agent_kind": "claude",
		"ready_timeout_ms": 60000,
		"wait_timeout_s": 1800,
		"team_lead_permission_mode": "bypassPermissions"
	}`)
	cfg, err := LoadFiles(path, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Legacy) != 4 {
		t.Fatalf("legacy fields = %v", cfg.Legacy)
	}
	if got := cfg.Levels["mid"]; len(got) != 1 || got[0].Model != "" {
		t.Fatalf("mid default = %+v", got)
	}
	if !strings.Contains(cfg.Describe(), "legacy: ignored fields") {
		t.Fatalf("describe missing legacy warning: %s", cfg.Describe())
	}
}

func TestTemplateIsValid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, Template(), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadFiles(path, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Sources) != 1 {
		t.Fatalf("sources = %v", cfg.Sources)
	}
}
