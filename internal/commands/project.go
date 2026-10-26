package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"weeco/internal/assets"
	"weeco/internal/store"
)

const (
	claudeMDBegin = "<!-- weeco:begin -->"
	claudeMDEnd   = "<!-- weeco:end -->"
)

func cmdInit(args []string) int {
	fs, err := parseFlags(args, map[string]bool{"root": true, "name": true})
	if err != nil {
		return fail(err)
	}
	if helpRequested(fs) {
		fmt.Println("usage: weeco init [--root PATH] [--name NAME] [--no-claude-md]")
		return 0
	}
	root := fs.value("root")
	if root == "" {
		root, _ = os.Getwd()
	}
	if err := store.EnsureConfig(); err != nil {
		return fail(err)
	}
	project, err := store.InitProject(root, fs.value("name"))
	if err != nil {
		return fail(err)
	}
	if !fs.bool("no-claude-md") {
		if err := writeClaudeMD(project.Root); err != nil {
			fmt.Printf("claude.md: warning: %v\n", err)
		} else {
			fmt.Printf("claude.md: boss block written to %s\n", filepath.Join(project.Root, "CLAUDE.md"))
		}
	}
	if missing := missingWeecoSKills(); len(missing) > 0 {
		fmt.Printf("warning: weeco skills are missing (%s); run: weeco install\n", strings.Join(missing, ", "))
	}
	fmt.Printf("project: %s\nkey:     %s\nhome:    %s\n", project.Root, project.Key, store.ProjectDir(project.Key))
	if project.DefaultBranch != "" {
		fmt.Printf("branch:  %s\n", project.DefaultBranch)
	} else {
		fmt.Println("branch:  (no git repository; workspaces use directories)")
	}
	fmt.Println("next:    weeco requirement new --goal \"...\"")
	return 0
}

func claudeMDBlock() string {
	data, err := assets.Read(assets.ClaudeMDBlock)
	if err != nil {
		return claudeMDBegin + "\n## weeco\n\n你是老板，不做具体任务；需要产出时招团队。\n" + claudeMDEnd + "\n"
	}
	return string(data)
}

func writeClaudeMD(root string) error {
	path := filepath.Join(root, "CLAUDE.md")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return os.WriteFile(path, []byte(claudeMDBlock()), 0o644)
	}
	if err != nil {
		return err
	}
	text := string(data)
	if strings.Contains(text, claudeMDBegin) {
		start := strings.Index(text, claudeMDBegin)
		endRel := strings.Index(text[start:], claudeMDEnd)
		if endRel < 0 {
			return nil
		}
		end := start + endRel + len(claudeMDEnd)
		block := strings.TrimRight(claudeMDBlock(), "\n")
		if text[start:end] == block {
			return nil
		}
		return os.WriteFile(path, []byte(text[:start]+block+text[end:]), 0o644)
	}
	if !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	return os.WriteFile(path, []byte(text+"\n"+claudeMDBlock()), 0o644)
}
