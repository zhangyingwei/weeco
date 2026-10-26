package app

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"weefleet/internal/assets"
)

type installManifest struct {
	Version     string   `json:"version"`
	Binary      string   `json:"binary"`
	Launcher    string   `json:"launcher"`
	Skill       string   `json:"skill"`
	Command     string   `json:"command"`
	Templates   []string `json:"templates"`
	InstalledAt string   `json:"installed_at"`
}

func cmdInstall(args []string) int {
	fs, err := parseFlags(args, map[string]bool{
		"bin-dir": true, "skill-dir": true, "command-dir": true,
	})
	if err != nil {
		return fail(err)
	}
	if fs.has("help") {
		fmt.Println("usage: weefleet install [--bin-dir PATH] [--skill-dir PATH] [--command-dir PATH] [--force] [--dry-run]")
		return 0
	}
	dryRun := fs.bool("dry-run")
	force := fs.bool("force")
	bin := binDir()
	skills := skillDir()
	commands := commandDir()
	if v := fs.value("bin-dir"); v != "" {
		bin = expandHome(v)
	}
	if v := fs.value("skill-dir"); v != "" {
		skills = expandHome(v)
	}
	if v := fs.value("command-dir"); v != "" {
		commands = expandHome(v)
	}

	target := filepath.Join(bin, "weefleet")
	launcher := filepath.Join(bin, "weefleet-claude")
	skillPath := filepath.Join(skills, "weefleet-coordinator", "SKILL.md")
	commandPath := filepath.Join(commands, "weefleet.md")
	templates := []struct {
		name string
		src  string
	}{
		{"brief.md", assets.BriefTemplate},
		{"checks.json", assets.ChecksTemplate},
		{"commit.txt", assets.CommitTemplate},
	}
	var templatePaths []string
	for _, t := range templates {
		templatePaths = append(templatePaths, filepath.Join(templateDir(), t.name))
	}

	action := func(format string, a ...any) {
		prefix := ""
		if dryRun {
			prefix = "(dry-run) "
		}
		fmt.Printf(prefix+format+"\n", a...)
	}

	// 1. Copy this binary to the install directory.
	self, err := os.Executable()
	if err != nil {
		return fail(err)
	}
	if resolved, err := filepath.EvalSymlinks(self); err == nil {
		self = resolved
	}
	sameBinary := false
	if targetInfo, err := os.Stat(target); err == nil {
		if selfInfo, err := os.Stat(self); err == nil && os.SameFile(targetInfo, selfInfo) {
			sameBinary = true
		}
	}
	if sameBinary {
		action("binary already in place: %s", target)
	} else {
		if linkInfo, err := os.Lstat(target); err == nil {
			if linkInfo.Mode()&os.ModeSymlink != 0 {
				action("replace symlink at %s", target)
				if !dryRun {
					if err := os.Remove(target); err != nil {
						return fail(err)
					}
				}
			} else if !force {
				if !isWeefleetBinary(target) {
					fmt.Printf("refusing to overwrite existing non-weefleet binary: %s\n", target)
					fmt.Println("move it away first, or pass --force")
					return 1
				}
			}
		}
		if !dryRun {
			if err := copyFile(self, target); err != nil {
				return fail(err)
			}
		}
		action("install binary: %s", target)
	}

	// 2. The launcher is a symlink, so upgrades never leave a stale copy.
	launcherSkipped := false
	if info, err := os.Lstat(launcher); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			if !dryRun {
				_ = os.Remove(launcher)
			}
		} else if !force {
			fmt.Printf("refusing to overwrite existing non-symlink: %s\n", launcher)
			fmt.Println("move it away first, or pass --force")
			launcherSkipped = true
		} else if !dryRun {
			if err := os.Remove(launcher); err != nil {
				return fail(err)
			}
		}
	}
	if !launcherSkipped {
		if !dryRun {
			if err := os.MkdirAll(bin, 0o755); err != nil {
				return fail(err)
			}
			if err := os.Symlink("weefleet", launcher); err != nil && !os.IsExist(err) {
				return fail(err)
			}
		}
		action("launcher: %s -> weefleet", launcher)
	}

	// 3. Skill, slash command and templates come from the embedded assets.
	for _, item := range []struct {
		label string
		path  string
		asset string
		owned string
	}{
		{"skill", skillPath, assets.CoordinatorSkill, "weefleet-coordinator"},
		{"slash command", commandPath, assets.SlashCommand, "weefleet"},
	} {
		data, err := assets.Read(item.asset)
		if err != nil {
			return fail(err)
		}
		result, err := installFile(item.path, data, 0o644, force, dryRun, item.owned)
		if err != nil {
			return fail(err)
		}
		action("%s: %s", item.label, result)
	}
	for _, t := range templates {
		data, err := assets.Read(t.src)
		if err != nil {
			return fail(err)
		}
		result, err := installFile(filepath.Join(templateDir(), t.name), data, 0o644, force, dryRun, "")
		if err != nil {
			return fail(err)
		}
		action("template %s: %s", t.name, result)
	}

	// 4. Record what was installed so a later version can explain itself.
	manifest := installManifest{
		Version: Version, Binary: target, Launcher: launcher,
		Skill: skillPath, Command: commandPath, Templates: templatePaths,
		InstalledAt: nowISO(),
	}
	if !dryRun {
		if err := writeJSON(filepath.Join(weefleetHome(), "installed.json"), manifest); err != nil {
			return fail(err)
		}
	}

	if !pathContains(bin) {
		fmt.Printf("warning: %s is not on PATH; add it to your shell profile\n", bin)
	}
	if dryRun {
		fmt.Println("dry-run: no files changed")
		return 0
	}
	fmt.Println()
	// Doctor is informational here: the install itself succeeded even if an
	// external prerequisite (herdr, claude) is still missing.
	_ = cmdDoctor(nil)
	return 0
}

// installFile writes content unless the file already holds it. A file that
// looks like ours (owned marker) may be updated; any other existing file is
// left alone unless --force is passed. An empty marker means "never assume
// ownership" (used for templates the human may have edited).
func installFile(path string, data []byte, mode os.FileMode, force, dryRun bool, ownedMarker string) (string, error) {
	existing, err := os.ReadFile(path)
	if err == nil {
		if bytes.Equal(existing, data) {
			return "unchanged", nil
		}
		owned := ownedMarker != "" && strings.Contains(string(existing), ownedMarker)
		if !owned && !force {
			return "skipped (file exists and is not ours; use --force)", nil
		}
		if dryRun {
			return "would update", nil
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return "", err
		}
		return "updated", os.WriteFile(path, data, mode)
	}
	if !os.IsNotExist(err) {
		return "", err
	}
	if dryRun {
		return "would create", nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	return "created", os.WriteFile(path, data, mode)
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".weefleet-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := io.Copy(tmp, in); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0o755); err != nil {
		return err
	}
	return os.Rename(tmpName, dst)
}

func isWeefleetBinary(path string) bool {
	res, err := runCmd([]string{path, "version"}, "", 5*time.Second, nil)
	return err == nil && res.ExitCode == 0 && strings.HasPrefix(strings.TrimSpace(res.Stdout), "weefleet ")
}

func pathContains(dir string) bool {
	target := filepath.Clean(dir)
	for _, part := range filepath.SplitList(os.Getenv("PATH")) {
		if filepath.Clean(part) == target {
			return true
		}
	}
	return false
}
