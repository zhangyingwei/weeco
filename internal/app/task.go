package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var agentNameRe = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)

func validateTaskID(taskID string) error {
	if !taskIDRe.MatchString(taskID) {
		return errorf("invalid task id: %s (expected T-YYYYMMDD-NN)", taskID)
	}
	return nil
}

func agentName(taskID string, attempt int) (string, error) {
	base := strings.ToLower(taskID) + "-impl"
	if attempt > 1 {
		base += fmt.Sprintf("-a%d", attempt)
	}
	if len(base) > 32 {
		base = strings.TrimRight(base[:32], "-_")
	}
	if !agentNameRe.MatchString(base) {
		return "", errorf("invalid agent name derived from task id: %s", base)
	}
	return base, nil
}

// readChecks validates checks.json. The user-level config supplies the budget
// defaults; checks.json overrides any field it sets explicitly.
func readChecks(path string) (ChecksFile, []byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ChecksFile{}, nil, errorf("cannot read checks file %s: %v", path, err)
	}
	return parseChecksWithBase(raw, path, loadConfig().Budget)
}

func parseChecks(raw []byte, path string) (ChecksFile, []byte, error) {
	return parseChecksWithBase(raw, path, defaultConfig().Budget)
}

func parseChecksWithBase(raw []byte, path string, base Budget) (ChecksFile, []byte, error) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return ChecksFile{}, nil, errorf("checks.json must be an object with a 'checks' array: %v", err)
	}
	var items []map[string]any
	if err := json.Unmarshal(obj["checks"], &items); err != nil || len(items) == 0 {
		return ChecksFile{}, nil, errorf("checks.json.checks must be a non-empty array")
	}
	seen := map[string]bool{}
	out := ChecksFile{}
	for _, item := range items {
		cid := strings.TrimSpace(asString(item["id"]))
		cmd := strings.TrimSpace(asString(item["cmd"]))
		if cid == "" || cmd == "" {
			return ChecksFile{}, nil, errorf("each check needs a non-empty id and cmd")
		}
		if seen[cid] {
			return ChecksFile{}, nil, errorf("duplicate check id: %s", cid)
		}
		seen[cid] = true
		expectExit := 0
		if v, ok := item["expect_exit"]; ok {
			expectExit = asInt(v)
		}
		timeoutS := 300.0
		if v, ok := item["timeout_s"]; ok && asFloat(v) > 0 {
			timeoutS = asFloat(v)
		}
		out.Checks = append(out.Checks, Check{ID: cid, Cmd: cmd, ExpectExit: expectExit, TimeoutS: timeoutS})
	}
	out.Budget = base
	if rawBudget, ok := obj["budget"]; ok {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(rawBudget, &fields); err != nil {
			return ChecksFile{}, nil, errorf("checks.json.budget must be an object")
		}
		if v, ok := fields["max_total_seconds"]; ok {
			var f float64
			if err := json.Unmarshal(v, &f); err != nil {
				return ChecksFile{}, nil, errorf("budget.max_total_seconds must be a number")
			}
			out.Budget.MaxTotalSeconds = f
		}
		if v, ok := fields["max_runs_per_check"]; ok {
			var n int
			if err := json.Unmarshal(v, &n); err != nil {
				return ChecksFile{}, nil, errorf("budget.max_runs_per_check must be an integer")
			}
			out.Budget.MaxRunsPerCheck = n
		}
		if v, ok := fields["max_output_lines"]; ok {
			var n int
			if err := json.Unmarshal(v, &n); err != nil {
				return ChecksFile{}, nil, errorf("budget.max_output_lines must be an integer")
			}
			out.Budget.MaxOutputLines = n
		}
	}
	return out, raw, nil
}

func loadChecks(task Task) (ChecksFile, []byte, error) {
	if task.ChecksPath == "" {
		return ChecksFile{}, nil, errorf("task %s has no checks_path", task.ID)
	}
	return readChecks(task.ChecksPath)
}

func checkIDs(checks ChecksFile) []string {
	ids := make([]string, 0, len(checks.Checks))
	for _, c := range checks.Checks {
		ids = append(ids, c.ID)
	}
	return ids
}

func cacheKey(taskID, headSHA string, checksBytes []byte) string {
	return sha256Text(taskID + "\n" + headSHA + "\n" + sha256Bytes(checksBytes))
}

func taskDirFor(task Task) (string, error) {
	if task.ChecksPath != "" {
		return filepath.Dir(task.ChecksPath), nil
	}
	return "", errorf("task has no checks_path; was it created by weefleet new?")
}

func evidencePath(task Task, raw string) (string, error) {
	if filepath.IsAbs(raw) {
		return "", errorf("evidence path must be relative: %s", raw)
	}
	base, err := taskDirFor(task)
	if err != nil {
		return "", err
	}
	candidate := filepath.Join(base, raw)
	if fileExists(candidate) {
		return candidate, nil
	}
	if task.Worktree != "" {
		candidate = filepath.Join(task.Worktree, raw)
		if fileExists(candidate) {
			return candidate, nil
		}
	}
	return "", errorf("evidence file not found: %s", raw)
}

func globToRegex(pattern string) *regexp.Regexp {
	var b strings.Builder
	for i := 0; i < len(pattern); i++ {
		ch := pattern[i]
		switch ch {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				b.WriteString(".*")
				i++
				if i+1 < len(pattern) && pattern[i+1] == '/' {
					i++
				}
			} else {
				b.WriteString("[^/]*")
			}
		case '?':
			b.WriteString("[^/]")
		default:
			b.WriteString(regexp.QuoteMeta(string(ch)))
		}
	}
	re, err := regexp.Compile("^" + b.String() + "$")
	if err != nil {
		return regexp.MustCompile(`^.*$`)
	}
	return re
}

func matchScope(path string, patterns []string) bool {
	path = filepath.ToSlash(path)
	if len(patterns) == 0 {
		return true
	}
	for _, p := range patterns {
		if globToRegex(p).MatchString(path) {
			return true
		}
	}
	return false
}

func renderBrief(taskID, goal string, scope, checkIDList []string, reportPath string) string {
	scopeText := "- 未指定"
	if len(scope) > 0 {
		lines := make([]string, 0, len(scope))
		for _, s := range scope {
			lines = append(lines, "- "+s)
		}
		scopeText = strings.Join(lines, "\n")
	}
	checksText := "（见 checks.json）"
	if len(checkIDList) > 0 {
		checksText = strings.Join(checkIDList, ", ")
	}
	return fmt.Sprintf(`# %s

## 目标
%s

## 只改这些路径
%s

## 不要做
- 不 push，不碰 main
- 不安装新依赖，除非 brief 明确要求
- 不改数据库 schema，除非 brief 明确要求

## 验收
`+"`weefleet verify`"+` 会重跑同目录的 `+"`checks.json`"+`，不要自己改它。
本次 checks：%s

## 交付
- 代码改动（必须 commit，不要留着未提交的改动）
- `+"`report.json`"+`
- 证据文件放在 `+"`evidence/`"+`

## 结束时
1. `+"`git add -A && git commit -m \"<summary>\" -m \"<trailer 段落>\"`"+`
   commit message 最后一段必须带：
   Task: %s
   Attempt: <dispatch 给你的 attempt>
   Checks: %s
   Report: report.json
   Base: <dispatch 给你的 base_sha>
   Status: done
   （做不完就改成 Status: blocked 或 Status: failed）
2. 把 commit sha 写进 report 的 `+"`head_sha`"+`
3. 最后一行输出：REPORT: %s
`, taskID, strings.TrimSpace(goal), scopeText, checksText, taskID, checksText, reportPath)
}
