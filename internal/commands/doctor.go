package commands

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"weeco/internal/store"
	"weeco/internal/sysutil"
)

func cmdDoctor(args []string) int {
	fs, err := parseFlags(args, map[string]bool{})
	if err != nil {
		return fail(err)
	}
	if helpRequested(fs) {
		fmt.Println("usage: weeco doctor")
		return 0
	}
	type row struct {
		name     string
		ok       bool
		required bool
		detail   string
	}
	var rows []row
	add := func(name string, ok, required bool, detail string) {
		rows = append(rows, row{name, ok, required, detail})
	}
	for _, tool := range []string{"claude", "herdr"} {
		path, err := exec.LookPath(tool)
		if err != nil {
			add(tool, false, true, "not found in PATH")
		} else {
			add(tool, true, true, path)
		}
	}
	herdrVersion := ""
	if _, err := exec.LookPath("herdr"); err == nil {
		res, _ := sysutil.Run([]string{"herdr", "--version"}, "", 10*time.Second, nil)
		herdrVersion = strings.TrimSpace(res.Stdout + res.Stderr)
	}
	add("herdr runs", herdrVersion != "", true, orDefault(herdrVersion, "herdr --version failed"))
	integration := ""
	if _, err := exec.LookPath("herdr"); err == nil {
		res, _ := sysutil.Run([]string{"herdr", "integration", "status"}, "", 15*time.Second, nil)
		integration = res.Stdout + res.Stderr
	}
	okIntegration := strings.Contains(integration, "claude: current")
	add("claude integration", okIntegration, true, map[bool]string{true: "claude: current", false: "run: herdr integration install claude"}[okIntegration])
	weecoPath, _ := exec.LookPath("weeco")
	add("weeco on PATH", weecoPath != "", false, orDefault(weecoPath, "not on PATH; add the install bin dir"))
	missingSkills := missingWeecoSKills()
	skillDetail := "all installed"
	if len(missingSkills) > 0 {
		skillDetail = "missing: " + strings.Join(missingSkills, ", ") + "; run: weeco install"
	}
	add("weeco skills", len(missingSkills) == 0, false, skillDetail)
	commandPath := filepath.Join(store.CommandDir(), "weeco.md")
	add("slash command /weeco", sysutil.FileExists(commandPath), false, installedDetail(commandPath, "missing "+commandPath+"; run: weeco install"))
	inHerdr := os.Getenv("HERDR_ENV") == "1"
	add("inside Herdr pane", inHerdr, false, map[bool]string{true: "yes", false: "team commands need a Herdr pane"}[inHerdr])
	wd, _ := os.Getwd()
	if project, err := store.FindProject(wd); err == nil {
		add("project initialized", true, false, project.Root+" ("+project.Key+")")
	} else {
		add("project initialized", false, false, "no project from cwd; run: weeco init --root <path>")
	}
	_, gitErr := exec.LookPath("git")
	add("git (optional)", gitErr == nil, false, map[bool]string{true: "available for worktree workspaces", false: "not found; use plain directories"}[gitErr == nil])

	failedRequired, warnings := false, false
	for _, r := range rows {
		mark := "ok"
		if !r.ok {
			if r.required {
				mark = "!!"
				failedRequired = true
			} else {
				mark = "--"
				warnings = true
			}
		}
		fmt.Printf("[%s] %s: %s\n", mark, r.name, r.detail)
	}
	if failedRequired {
		fmt.Println("doctor: required checks failed")
		return 1
	}
	suffix := ""
	if warnings {
		suffix = " (with warnings)"
	}
	fmt.Printf("doctor: ready%s\n", suffix)
	return 0
}

func installedDetail(path, fallback string) string {
	if sysutil.FileExists(path) {
		return "installed"
	}
	return fallback
}

func orDefault(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}
