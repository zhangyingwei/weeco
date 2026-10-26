package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func cmdAccept(args []string) int {
	fs, err := parseFlags(args, map[string]bool{"repo": true, "by": true, "note": true})
	if err != nil {
		return fail(err)
	}
	taskID, err := fs.positional(0, "task")
	if err != nil {
		return fail(err)
	}
	repoRoot, key, _, err := resolveRepo(fs.value("repo"))
	if err != nil {
		return fail(err)
	}
	by := orDefault(fs.value("by"), orDefault(gitConfig(repoRoot, "user.name"), "human"))
	var task Task
	err = withRepoLock(key, func() error {
		var err error
		task, err = foldTask(key, taskID)
		if err != nil {
			return err
		}
		if task.State != StateVerified {
			return errorf("task %s is %s, expected verified", taskID, task.State)
		}
		if !fs.bool("yes") {
			fmt.Println("refusing to accept without --yes; review the summary and re-run with --yes")
			printAcceptSummary(key, task)
			return errRefused
		}
		if err := appendEvent(key, taskID, "accepted", map[string]any{
			"by": by, "head_sha": task.HeadSHA, "note": fs.value("note"),
		}); err != nil {
			return err
		}
		if task.Worktree != "" && fileExists(task.Worktree) {
			note, _ := jsonMarshalCompact(map[string]any{
				"by": by, "ts": nowISO(), "head_sha": task.HeadSHA,
			})
			gitNote(task.Worktree, "weefleet/accept", note)
		}
		return nil
	})
	if err == errRefused {
		return 1
	}
	if err != nil {
		return fail(err)
	}
	task, _ = foldTask(key, taskID)
	printAcceptSummary(key, task)
	fmt.Printf("accepted by %s; merge manually, then run: weefleet record %s\n", by, taskID)
	return 0
}

func printAcceptSummary(key string, task Task) {
	tdir, err := taskDirFor(task)
	if err != nil {
		return
	}
	var report Report
	_, _ = readJSONOr(filepath.Join(tdir, "report.json"), &report)
	var verify VerifyResult
	_, _ = readJSONOr(filepath.Join(tdir, "verify.json"), &verify)
	label := "FAIL"
	if verify.Passed {
		label = "PASS"
	}
	fmt.Printf("task:   %s\nstate:  %s\nhead:   %s\nverify: %s\nsummary: %s\n",
		task.ID, task.State, task.HeadSHA, label, report.Summary)
	for _, risk := range report.OpenRisks {
		fmt.Printf("risk:   %s\n", risk)
	}
	if task.Worktree != "" && task.BaseSHA != "" && task.HeadSHA != "" {
		if stat, err := gitDiffStat(task.Worktree, task.BaseSHA, task.HeadSHA); err == nil && stat != "" {
			fmt.Println(stat)
		}
	}
	_ = key
}

func cmdReject(args []string) int {
	fs, err := parseFlags(args, map[string]bool{"repo": true, "reason": true})
	if err != nil {
		return fail(err)
	}
	taskID, err := fs.positional(0, "task")
	if err != nil {
		return fail(err)
	}
	reason := fs.value("reason")
	if reason == "" {
		return fail(errorf("--reason is required"))
	}
	_, key, _, err := resolveRepo(fs.value("repo"))
	if err != nil {
		return fail(err)
	}
	err = withRepoLock(key, func() error {
		task, err := foldTask(key, taskID)
		if err != nil {
			return err
		}
		switch task.State {
		case StateVerified, StateReported, StateFailed:
		default:
			return errorf("task %s is %s; nothing to reject", taskID, task.State)
		}
		return appendEvent(key, taskID, "rejected", map[string]any{"reason": reason})
	})
	if err != nil {
		return fail(err)
	}
	fmt.Printf("rejected: %s\nreason: %s\n", taskID, reason)
	fmt.Println("the pane and worktree are left open for inspection")
	return 0
}

func cmdResolve(args []string) int {
	fs, err := parseFlags(args, map[string]bool{"repo": true, "decision": true, "by": true})
	if err != nil {
		return fail(err)
	}
	taskID, err := fs.positional(0, "task")
	if err != nil {
		return fail(err)
	}
	decision := fs.value("decision")
	if decision == "" {
		return fail(errorf("--decision is required"))
	}
	repoRoot, key, _, err := resolveRepo(fs.value("repo"))
	if err != nil {
		return fail(err)
	}
	send := fs.bool("send")
	if send {
		if _, err := requireHerdr(); err != nil {
			return fail(err)
		}
	}
	by := orDefault(fs.value("by"), orDefault(gitConfig(repoRoot, "user.name"), "human"))
	var paneID string
	sent := false
	err = withRepoLock(key, func() error {
		task, err := foldTask(key, taskID)
		if err != nil {
			return err
		}
		if task.State != StateBlocked {
			return errorf("task %s is %s, expected blocked", taskID, task.State)
		}
		if task.Agent == "" {
			return errorf("task %s has no agent", taskID)
		}
		paneID = task.PaneID
		if send {
			live := herdrAgent(task.Agent)
			if live != nil && asString(live["agent_status"]) == "blocked" {
				fmt.Println("agent is still showing a blocked UI; answer it in the pane first")
				fmt.Printf("pane: %s\n", task.PaneID)
			} else {
				if err := herdrAgentPrompt(task.Agent, "HUMAN DECISION: "+decision, false, 0); err != nil {
					return err
				}
				sent = true
			}
		}
		return appendEvent(key, taskID, "blocked_resolved", map[string]any{
			"decision": decision, "sent": sent, "by": by,
		})
	})
	if err != nil {
		return fail(err)
	}
	fmt.Printf("decision recorded for %s by %s (sent=%t)\n", taskID, by, sent)
	if !sent {
		fmt.Printf("answer the agent in pane %s if needed\n", paneID)
	}
	return 0
}

func cmdRetry(args []string) int {
	fs, err := parseFlags(args, map[string]bool{"repo": true, "reason": true})
	if err != nil {
		return fail(err)
	}
	taskID, err := fs.positional(0, "task")
	if err != nil {
		return fail(err)
	}
	reason := fs.value("reason")
	if reason == "" {
		return fail(errorf("--reason is required"))
	}
	_, key, _, err := resolveRepo(fs.value("repo"))
	if err != nil {
		return fail(err)
	}
	attempt := 0
	err = withRepoLock(key, func() error {
		task, err := foldTask(key, taskID)
		if err != nil {
			return err
		}
		if !retryableStates[task.State] {
			return errorf("task %s is %s; retry is for failed/uncertain/lost", taskID, task.State)
		}
		attempt = task.Attempt
		if attempt < 1 {
			attempt = 1
		}
		attempt++
		return appendEvent(key, taskID, "retry_requested", map[string]any{
			"attempt": attempt, "reason": reason,
		})
	})
	if err != nil {
		return fail(err)
	}
	fmt.Printf("retry ready: %s attempt %d\n", taskID, attempt)
	fmt.Printf("run: weefleet dispatch %s\n", taskID)
	return 0
}

func cmdClose(args []string) int {
	fs, err := parseFlags(args, map[string]bool{"repo": true})
	if err != nil {
		return fail(err)
	}
	taskID, err := fs.positional(0, "task")
	if err != nil {
		return fail(err)
	}
	repoRoot, key, _, err := resolveRepo(fs.value("repo"))
	if err != nil {
		return fail(err)
	}
	force := fs.bool("force")
	var task Task
	var tdir string
	err = withRepoLock(key, func() error {
		var err error
		task, err = foldTask(key, taskID)
		if err != nil {
			return err
		}
		if !terminalStates[task.State] && !force {
			return errorf("task %s is %s; close is for accepted/rejected/closed (use --force to override)", taskID, task.State)
		}
		tdir, _ = taskDirFor(task)
		if task.PaneID != "" {
			if err := herdrPaneClose(task.PaneID); err != nil {
				fmt.Printf("warning: could not close pane %s: %v\n", task.PaneID, err)
			}
		}
		if task.TabID != "" && task.WorkspaceID != "" {
			if panes, err := herdrPanes(task.WorkspaceID); err != nil {
				fmt.Printf("warning: could not inspect/close tab %s: %v\n", task.TabID, err)
			} else {
				remaining := false
				for _, p := range panes {
					if asString(p["tab_id"]) == task.TabID {
						remaining = true
						break
					}
				}
				if !remaining {
					if err := herdrTabClose(task.TabID); err != nil {
						fmt.Printf("warning: could not close tab %s: %v\n", task.TabID, err)
					}
				}
			}
		}
		if task.Worktree != "" && fileExists(task.Worktree) {
			if err := gitWorktreeRemove(repoRoot, task.Worktree, force); err != nil {
				return errorf("could not remove worktree %s: %v\ncommit or discard changes, then retry; use --force to override", task.Worktree, err)
			}
		}
		if fs.bool("delete-branch") && task.Branch != "" {
			if err := gitBranchDelete(repoRoot, task.Branch); err != nil {
				fmt.Printf("warning: could not delete branch %s: %v\n", task.Branch, err)
			}
		}
		return appendEvent(key, taskID, "closed", map[string]any{
			"branch": task.Branch, "head_sha": task.HeadSHA, "merge_commit": task.MergeCommit,
		})
	})
	if err != nil {
		return fail(err)
	}
	fmt.Printf("closed: %s\ntask dir kept: %s\n", taskID, tdir)
	return 0
}

func cmdRecord(args []string) int {
	fs, err := parseFlags(args, map[string]bool{"repo": true, "merge-commit": true})
	if err != nil {
		return fail(err)
	}
	taskID, err := fs.positional(0, "task")
	if err != nil {
		return fail(err)
	}
	repoRoot, key, meta, err := resolveRepo(fs.value("repo"))
	if err != nil {
		return fail(err)
	}
	force := fs.bool("force")
	var docs string
	var mergeCommit string
	err = withRepoLock(key, func() error {
		task, err := foldTask(key, taskID)
		if err != nil {
			return err
		}
		if task.State != StateAccepted && task.State != StateClosed {
			return errorf("task %s is %s, expected accepted or closed", taskID, task.State)
		}
		if current := gitCurrentBranch(repoRoot); current != "" && current != meta.DefaultBranch && !force {
			return errorf("current branch is %s, not %s; run record on the default branch or pass --force", current, meta.DefaultBranch)
		}
		mergeCommit = fs.value("merge-commit")
		if mergeCommit == "" {
			mergeCommit, err = gitHead(repoRoot)
			if err != nil {
				return err
			}
		}
		docs = filepath.Join(repoRoot, "docs", "weefleet", "tasks", taskID+".md")
		if fileExists(docs) && !force {
			return errorf("record already exists: %s (use --force to overwrite)", docs)
		}
		content := renderRecord(key, task, mergeCommit)
		if err := os.MkdirAll(filepath.Dir(docs), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(docs, []byte(content), 0o644); err != nil {
			return err
		}
		relative, _ := filepath.Rel(repoRoot, docs)
		if _, err := gitChecked(repoRoot, "add", relative); err != nil {
			return err
		}
		message := fmt.Sprintf("docs: record %s\n\nTask: %s\nMerge: %s", taskID, taskID, mergeCommit)
		if _, err := gitChecked(repoRoot, "commit", "-m", message, "--", relative); err != nil {
			if strings.Contains(err.Error(), "nothing to commit") {
				fmt.Printf("record unchanged: %s\n", docs)
			} else {
				return err
			}
		}
		return appendEvent(key, taskID, "recorded", map[string]any{
			"path": docs, "merge_commit": mergeCommit,
		})
	})
	if err != nil {
		return fail(err)
	}
	fmt.Printf("record: %s\nmerge:  %s\n", docs, mergeCommit)
	return 0
}

func renderRecord(key string, task Task, mergeCommit string) string {
	tdir, _ := taskDirFor(task)
	var report Report
	_, _ = readJSONOr(filepath.Join(tdir, "report.json"), &report)
	var verify VerifyResult
	_, _ = readJSONOr(filepath.Join(tdir, "verify.json"), &verify)
	brief := ""
	if data, err := os.ReadFile(filepath.Join(tdir, "brief.md")); err == nil {
		brief = string(data)
	}
	var rows []string
	for _, check := range verify.Checks {
		mark := "FAIL"
		if check.Passed {
			mark = "PASS"
		}
		rows = append(rows, fmt.Sprintf("| %s | `%s` | %s (exit %d) | %gs |", check.ID, check.Cmd, mark, check.Exit, check.DurationS))
	}
	checksTable := "| - | - | - | - |"
	if len(rows) > 0 {
		checksTable = strings.Join(rows, "\n")
	}
	risksText := "- 无"
	if len(report.OpenRisks) > 0 {
		risks := make([]string, 0, len(report.OpenRisks))
		for _, r := range report.OpenRisks {
			risks = append(risks, "- "+r)
		}
		risksText = strings.Join(risks, "\n")
	}
	recordState := task.State
	if task.AcceptedBy != "" {
		recordState = "accepted"
	}
	verifyState := "failed"
	if verify.Passed {
		verifyState = "passed"
	}
	acceptNote := task.AcceptNote
	if acceptNote == "" {
		acceptNote = "（无）"
	}
	return fmt.Sprintf(`# %s %s

- 状态：%s
- 分支：%s
- Base：%s
- Head：%s
- Merge：%s
- 接受人：%s
- 接受时间：%s
- 接受说明：%s

## 合同

`+"```text"+`
%s
`+"```"+`

## 验收

| id | 命令 | 结果 | 耗时 |
|---|---|---|---|
%s

## 结果

- verify：%s
- open risks：
%s

## 本地完整记录

`+"`%s`"+`
`, task.ID, report.Summary, recordState, task.Branch, task.BaseSHA, task.HeadSHA,
		mergeCommit, task.AcceptedBy, task.AcceptedAt, acceptNote,
		strings.TrimSpace(brief), checksTable, verifyState, risksText, tdir)
}

func cmdGC(args []string) int {
	fs, err := parseFlags(args, map[string]bool{"repo": true, "older-than": true})
	if err != nil {
		return fail(err)
	}
	olderThan := 90
	if v := fs.value("older-than"); v != "" {
		fmt.Sscanf(v, "%d", &olderThan)
	}
	if fs.bool("delete") && fs.bool("logs-only") {
		return fail(errorf("use either --delete or --logs-only, not both"))
	}
	repoRoot, key, _, err := resolveRepo(fs.value("repo"))
	if err != nil {
		return fail(err)
	}
	cutoff := time.Now().Add(-time.Duration(olderThan) * 24 * time.Hour)
	ids, err := listTaskIDs(key)
	if err != nil {
		return fail(err)
	}
	type candidate struct {
		id  string
		dir string
	}
	var candidates []candidate
	for _, id := range ids {
		task, err := foldTask(key, id)
		if err != nil {
			return fail(err)
		}
		if !terminalStates[task.State] {
			continue
		}
		created := parseISOTime(orDefault(task.CreatedAt, orDefault(task.UpdatedAt, nowISO())))
		if created.After(cutoff) {
			continue
		}
		tdir, err := taskDirFor(task)
		if err != nil {
			continue
		}
		record := filepath.Join(repoRoot, "docs", "weefleet", "tasks", id+".md")
		if fs.bool("delete") && !fileExists(record) && !fs.bool("force") {
			fmt.Printf("skip %s: no repo record (use --force to delete anyway)\n", id)
			continue
		}
		candidates = append(candidates, candidate{id, tdir})
	}
	if len(candidates) == 0 {
		fmt.Println("nothing to clean")
		return 0
	}
	for _, c := range candidates {
		switch {
		case fs.bool("logs-only"):
			for _, sub := range []string{"logs", "evidence"} {
				_ = os.RemoveAll(filepath.Join(c.dir, sub))
			}
			fmt.Printf("removed logs/evidence: %s\n", c.id)
		case fs.bool("delete"):
			if err := os.RemoveAll(c.dir); err != nil {
				return fail(err)
			}
			fmt.Printf("removed task dir: %s\n", c.id)
		default:
			fmt.Printf("candidate: %s %s\n", c.id, c.dir)
		}
	}
	if !fs.bool("delete") && !fs.bool("logs-only") {
		fmt.Println("dry-run; pass --delete or --logs-only to act")
	}
	return 0
}

var errRefused = errorf("refused")
