package commands

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"weeco/internal/assets"
	"weeco/internal/store"
	"weeco/internal/sysutil"
)

type installManifest struct {
	Version     string            `json:"version"`
	Binary      string            `json:"binary"`
	Skills      map[string]string `json:"skills"`
	Command     string            `json:"command"`
	Templates   []string          `json:"templates"`
	InstalledAt string            `json:"installed_at"`
}

func cmdInstall(args []string) int {
	fs, err := parseFlags(args, map[string]bool{
		"bin-dir": true, "skill-dir": true, "command-dir": true,
	})
	if err != nil {
		return fail(err)
	}
	if helpRequested(fs) {
		fmt.Println("usage: weeco install [--bin-dir PATH] [--skill-dir PATH] [--command-dir PATH] [--force] [--dry-run]")
		return 0
	}
	dryRun := fs.bool("dry-run")
	force := fs.bool("force")
	bin := store.BinDir()
	skills := store.SkillDir()
	commands := store.CommandDir()
	if v := fs.value("bin-dir"); v != "" {
		bin = sysutil.ExpandHome(v)
	}
	if v := fs.value("skill-dir"); v != "" {
		skills = sysutil.ExpandHome(v)
	}
	if v := fs.value("command-dir"); v != "" {
		commands = sysutil.ExpandHome(v)
	}
	ownedPaths := map[string]bool{}
	var previous installManifest
	if ok, _ := sysutil.ReadJSONOr(store.ManifestPath(), &previous); ok {
		addOwned := func(p string) {
			if p != "" {
				ownedPaths[filepath.Clean(p)] = true
			}
		}
		addOwned(previous.Binary)
		for _, p := range previous.Skills {
			addOwned(p)
		}
		addOwned(previous.Command)
		for _, p := range previous.Templates {
			addOwned(p)
		}
	}
	if !dryRun {
		if err := store.EnsureConfig(); err != nil {
			return fail(err)
		}
	}
	action := func(format string, a ...any) {
		prefix := ""
		if dryRun {
			prefix = "(dry-run) "
		}
		fmt.Printf(prefix+format+"\n", a...)
	}
	target := filepath.Join(bin, "weeco")
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
			} else if !force && !isWeecoBinary(target) {
				fmt.Printf("refusing to overwrite existing non-weeco binary: %s\n", target)
				fmt.Println("move it away first, or pass --force")
				return 1
			}
		}
		if !dryRun {
			if err := copyFile(self, target); err != nil {
				return fail(err)
			}
		}
		action("install binary: %s", target)
	}
	commandPath := filepath.Join(commands, "weeco.md")
	skillPaths := map[string]string{}
	for _, skill := range weecoSKills {
		path := filepath.Join(skills, skill.name, "SKILL.md")
		data, err := assets.Read(skill.asset)
		if err != nil {
			return fail(err)
		}
		result, err := installFile(path, data, 0o644, force, dryRun, skill.name, ownedPaths[filepath.Clean(path)])
		if err != nil {
			return fail(err)
		}
		skillPaths[skill.name] = path
		action("%s: %s", skill.label, result)
	}
	commandData, err := assets.Read(assets.SlashCommand)
	if err != nil {
		return fail(err)
	}
	result, err := installFile(commandPath, commandData, 0o644, force, dryRun, "weeco", ownedPaths[filepath.Clean(commandPath)])
	if err != nil {
		return fail(err)
	}
	action("slash command: %s", result)
	templates := []struct {
		name  string
		asset string
	}{
		{"contract.md", assets.ContractTemplate},
		{"result.md", assets.ResultTemplate},
		{"acceptance.json", assets.AcceptanceTemplate},
	}
	var templatePaths []string
	for _, t := range templates {
		path := filepath.Join(store.TemplateDir(), t.name)
		data, err := assets.Read(t.asset)
		if err != nil {
			return fail(err)
		}
		result, err := installFile(path, data, 0o644, force, dryRun, "", ownedPaths[filepath.Clean(path)])
		if err != nil {
			return fail(err)
		}
		templatePaths = append(templatePaths, path)
		action("template %s: %s", t.name, result)
	}
	manifest := installManifest{
		Version: Version, Binary: target, Skills: skillPaths,
		Command: commandPath, Templates: templatePaths, InstalledAt: sysutil.NowISO(),
	}
	if !dryRun {
		if err := sysutil.WriteJSON(store.ManifestPath(), manifest); err != nil {
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
	fmt.Println("installed: boss + stage skills, team lead skill, slash command, templates")
	fmt.Println("next:      weeco init --root <project>")
	return 0
}

func installFile(path string, data []byte, mode os.FileMode, force, dryRun bool, ownedMarker string, manifestOwned bool) (string, error) {
	existing, err := os.ReadFile(path)
	if err == nil {
		if bytes.Equal(existing, data) {
			return "unchanged", nil
		}
		owned := manifestOwned || (ownedMarker != "" && strings.Contains(string(existing), ownedMarker))
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
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".weeco-*")
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

func isWeecoBinary(path string) bool {
	res, err := sysutil.Run([]string{path, "version"}, "", 5*time.Second, nil)
	return err == nil && res.ExitCode == 0 && strings.HasPrefix(strings.TrimSpace(res.Stdout), "weeco ")
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
