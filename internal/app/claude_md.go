package app

import (
	"os"
	"path/filepath"
	"strings"

	"weefleet/internal/assets"
)

const (
	claudeMDBegin     = "<!-- weefleet:begin -->"
	claudeMDEnd       = "<!-- weefleet:end -->"
	legacyClaudeBegin = "<!-- fleet:begin -->"
	legacyClaudeEnd   = "<!-- fleet:end -->"
)

func claudeMDBlock() string {
	data, err := assets.Read(assets.ClaudeMDBlock)
	if err != nil {
		return claudeMDBegin + "\n## weefleet\n\n本项目使用 weefleet 管理开发任务。\n" + claudeMDEnd + "\n"
	}
	return string(data)
}

// writeClaudeMD appends a marked weefleet block without touching other content.
// It migrates the block written under the old tool name in place.
func writeClaudeMD(repoRoot string) error {
	path := filepath.Join(repoRoot, "CLAUDE.md")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return os.WriteFile(path, []byte(claudeMDBlock()), 0o644)
	}
	if err != nil {
		return err
	}
	text := string(data)
	if strings.Contains(text, claudeMDBegin) {
		return nil
	}
	if strings.Contains(text, legacyClaudeBegin) && strings.Contains(text, legacyClaudeEnd) {
		start := strings.Index(text, legacyClaudeBegin)
		end := strings.Index(text[start:], legacyClaudeEnd)
		if end >= 0 {
			end = start + end + len(legacyClaudeEnd)
			text = text[:start] + strings.TrimSpace(claudeMDBlock()) + text[end:]
			return os.WriteFile(path, []byte(text), 0o644)
		}
	}
	if !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	text += "\n" + claudeMDBlock()
	return os.WriteFile(path, []byte(text), 0o644)
}
