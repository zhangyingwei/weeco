package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func verifyFail(key, taskID string, task Task, reason string, extra map[string]any) int {
	tdir, _ := taskDirFor(task)
	verify := map[string]any{
		"task_id":     taskID,
		"passed":      false,
		"reason":      reason,
		"verified_at": nowISO(),
	}
	for k, v := range extra {
		verify[k] = v
	}
	_ = writeJSON(filepath.Join(tdir, "verify.json"), verify)
	data := map[string]any{
		"reason":      reason,
		"verify_path": filepath.Join(tdir, "verify.json"),
	}
	if head := asString(extra["head_sha"]); head != "" {
		data["head_sha"] = head
	}
	_ = appendEvent(key, taskID, "verify_failed", data)
	fmt.Printf("verify: FAILED (%s)\n", reason)
	return 0
}

func cmdVerify(args []string) int {
	fs, err := parseFlags(args, map[string]bool{"repo": true})
	if err != nil {
		return fail(err)
	}
	taskID, err := fs.positional(0, "task")
	if err != nil {
		return fail(err)
	}
	_, key, _, err := resolveRepo(fs.value("repo"))
	if err != nil {
		return fail(err)
	}
	force := fs.bool("force")
	ignoreBudget := fs.bool("ignore-budget")
	var code int
	err = withRepoLock(key, func() error {
		task, err := foldTask(key, taskID)
		if err != nil {
			return err
		}
		if task.Worktree == "" {
			return errorf("task %s has no worktree; run weefleet dispatch first", taskID)
		}
		tdir, err := taskDirFor(task)
		if err != nil {
			return err
		}
		reportPath := filepath.Join(tdir, "report.json")
		if !fileExists(reportPath) {
			return errorf("no report at %s; the worker has not finished", reportPath)
		}
		var report Report
		if _, err := readJSONOr(reportPath, &report); err != nil {
			return errorf("report.json must be an object: %v", err)
		}
		if report.TaskID != taskID {
			code = verifyFail(key, taskID, task, "report task_id mismatch", nil)
			return nil
		}
		switch report.Status {
		case "blocked":
			_ = appendEvent(key, taskID, "blocked", map[string]any{"reason": "worker status: blocked"})
			fmt.Println("state: blocked")
			code = 0
			return nil
		case "failed":
			_ = appendEvent(key, taskID, "verify_failed", map[string]any{"reason": "worker status: failed"})
			fmt.Println("state: failed")
			code = 0
			return nil
		case "done":
		default:
			code = verifyFail(key, taskID, task, "invalid report status: "+report.Status, nil)
			return nil
		}

		wt := task.Worktree
		baseSHA := task.BaseSHA
		headSHA, err := gitHead(wt)
		if err != nil {
			return err
		}
		checks, checksBytes, err := loadChecks(task)
		if err != nil {
			return err
		}
		if task.ChecksSHA256 != "" && sha256Bytes(checksBytes) != task.ChecksSHA256 {
			code = verifyFail(key, taskID, task,
				"checks.json changed after task creation; create a new task instead of weakening acceptance",
				map[string]any{"head_sha": headSHA})
			return nil
		}
		keyForRevision := cacheKey(taskID, headSHA, checksBytes)
		verifyPath := filepath.Join(tdir, "verify.json")

		var cached VerifyResult
		if ok, _ := readJSONOr(verifyPath, &cached); ok && cached.Key == keyForRevision && !force {
			event := "verify_failed"
			if cached.Passed {
				event = "verified"
			}
			data := map[string]any{
				"verify_path": verifyPath,
				"head_sha":    headSHA,
				"cached":      true,
			}
			if !cached.Passed {
				data["reason"] = orDefault(cached.Reason, "cached")
			}
			_ = appendEvent(key, taskID, event, data)
			label := "FAIL"
			if cached.Passed {
				label = "PASS"
			}
			fmt.Printf("verify: %s (cached; no checks re-run)\n", label)
			printVerifySummary(cached)
			code = 0
			return nil
		}

		clean, err := gitIsClean(wt)
		if err != nil {
			return err
		}
		if !clean {
			code = verifyFail(key, taskID, task, "worktree is dirty; commit before verify", map[string]any{"head_sha": headSHA})
			return nil
		}
		if headSHA == baseSHA {
			code = verifyFail(key, taskID, task, "head_sha equals base_sha; no commit was made", map[string]any{"head_sha": headSHA})
			return nil
		}
		trailers := gitTrailers(wt, "HEAD")
		if crossErrors := crossCheck(report, trailers, task, checks, headSHA, baseSHA); len(crossErrors) > 0 {
			code = verifyFail(key, taskID, task, strings.Join(crossErrors, "; "), map[string]any{
				"head_sha": headSHA, "trailers": trailers,
			})
			return nil
		}

		// Budget is cumulative per task. A new commit still counts against it.
		events, err := readEvents(key, taskID)
		if err != nil {
			return err
		}
		var checkRuns []Event
		for _, e := range events {
			if e.Event == "check_run" {
				checkRuns = append(checkRuns, e)
			}
		}
		budget := checks.Budget
		maxRuns := budget.MaxRunsPerCheck
		maxTotal := budget.MaxTotalSeconds
		totalBefore := 0.0
		for _, e := range checkRuns {
			totalBefore += asFloat(e.Data["duration_s"])
		}
		budgetReject := func(reason string) {
			_ = writeJSON(verifyPath, map[string]any{
				"task_id": taskID, "head_sha": headSHA, "passed": false,
				"reason": reason, "budget_exceeded": true,
			})
			_ = appendEvent(key, taskID, "budget_exceeded", map[string]any{"reason": reason, "head_sha": headSHA})
			fmt.Printf("verify: BUDGET EXCEEDED (%s)\n", reason)
			code = 0
		}
		if !ignoreBudget {
			for _, check := range checks.Checks {
				runs := 0
				for _, e := range checkRuns {
					if asString(e.Data["check"]) == check.ID {
						runs++
					}
				}
				if runs >= maxRuns {
					budgetReject(fmt.Sprintf("budget: check %s already ran %d times on this revision", check.ID, runs))
					return nil
				}
			}
			if totalBefore >= maxTotal {
				budgetReject(fmt.Sprintf("budget: task already used %.1fs of %.0fs", totalBefore, maxTotal))
				return nil
			}
		}

		var results []CheckResult
		budgetHit := false
		totalNow := totalBefore
		for _, check := range checks.Checks {
			started := time.Now()
			timeout := time.Duration(check.TimeoutS * float64(time.Second))
			res, err := runCmd([]string{"sh", "-c", check.Cmd}, wt, timeout, nil)
			if err != nil {
				return err
			}
			output := res.Stdout + res.Stderr
			duration := time.Since(started).Seconds()
			totalNow += duration
			logPath := filepath.Join(tdir, "logs", check.ID+".log")
			if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(logPath, []byte(output), 0o644); err != nil {
				return err
			}
			tail := lastLines(output, budget.MaxOutputLines)
			passed := !res.TimedOut && res.ExitCode == check.ExpectExit
			results = append(results, CheckResult{
				ID: check.ID, Cmd: check.Cmd, Exit: res.ExitCode, ExpectExit: check.ExpectExit,
				Passed: passed, TimedOut: res.TimedOut, DurationS: round2(duration),
				Log: relativeTo(logPath, tdir), Tail: tail,
			})
			data := map[string]any{
				"check": check.ID, "head_sha": headSHA, "exit": res.ExitCode,
				"duration_s": round2(duration), "output_bytes": len([]byte(output)),
			}
			if res.TimedOut {
				data["timed_out"] = true
			}
			if err := appendEvent(key, taskID, "check_run", data); err != nil {
				return err
			}
			if !ignoreBudget && totalNow > maxTotal {
				budgetHit = true
				break
			}
		}

		changed, err := gitChangedFiles(wt, baseSHA, headSHA)
		if err != nil {
			return err
		}
		if len(changed) == 0 {
			code = verifyFail(key, taskID, task, "no files changed in base..HEAD", map[string]any{"head_sha": headSHA})
			return nil
		}
		reportedChanged := report.ChangedFiles
		if reportedChanged == nil {
			reportedChanged = []string{}
		}
		var warnings []string
		if !sameStringSet(changed, reportedChanged) {
			warnings = append(warnings, fmt.Sprintf("changed_files mismatch: git=%v report=%v", changed, reportedChanged))
		}
		violations := []string{}
		for _, p := range changed {
			if !matchScope(p, task.Scope) {
				violations = append(violations, p)
			}
		}
		if len(violations) > 0 {
			warnings = append(warnings, fmt.Sprintf("scope violations (warning only): %v", violations))
		}

		var evidence []EvidenceResult
		var evidenceErrors []string
		for _, raw := range report.Evidence {
			path, err := evidencePath(task, raw)
			if err != nil {
				evidenceErrors = append(evidenceErrors, err.Error())
				continue
			}
			digest, err := sha256File(path)
			if err != nil {
				evidenceErrors = append(evidenceErrors, err.Error())
				continue
			}
			info, _ := os.Stat(path)
			size := int64(0)
			if info != nil {
				size = info.Size()
			}
			evidence = append(evidence, EvidenceResult{Path: raw, SHA256: digest, Size: size})
		}

		passed := true
		for _, r := range results {
			if !r.Passed {
				passed = false
			}
		}
		if len(evidenceErrors) > 0 {
			passed = false
		}
		verify := VerifyResult{
			Key: keyForRevision, TaskID: taskID, BaseSHA: baseSHA, HeadSHA: headSHA,
			Passed: passed, Checks: results, Warnings: warnings,
			Evidence: evidence, EvidenceErrors: evidenceErrors, Trailers: trailers,
			VerifiedAt: nowISO(),
		}
		if budgetHit {
			reason := fmt.Sprintf("budget: total checks exceeded %.0fs", maxTotal)
			verify.Passed = false
			verify.BudgetExceeded = true
			verify.Reason = reason
			_ = writeJSON(verifyPath, verify)
			_ = appendEvent(key, taskID, "budget_exceeded", map[string]any{"reason": reason, "head_sha": headSHA})
			fmt.Printf("verify: BUDGET EXCEEDED (%s)\n", reason)
			printVerifySummary(verify)
			code = 0
			return nil
		}
		if !passed {
			var failedIDs []string
			for _, r := range results {
				if !r.Passed {
					failedIDs = append(failedIDs, r.ID)
				}
			}
			reason := "checks failed: " + strings.Join(failedIDs, ", ")
			if len(failedIDs) == 0 {
				reason = "evidence missing"
			}
			if len(evidenceErrors) > 0 {
				reason += "; " + strings.Join(evidenceErrors, "; ")
			}
			verify.Reason = reason
		}
		if err := writeJSON(verifyPath, verify); err != nil {
			return err
		}
		if passed {
			_ = appendEvent(key, taskID, "verified", map[string]any{"verify_path": verifyPath, "head_sha": headSHA})
			note, _ := jsonMarshalCompact(map[string]any{
				"passed": true, "checks": checkIDs(checks), "ts": nowISO(),
			})
			gitNote(wt, "weefleet/verify", note)
		} else {
			_ = appendEvent(key, taskID, "verify_failed", map[string]any{
				"reason": verify.Reason, "verify_path": verifyPath, "head_sha": headSHA,
			})
		}
		label := "FAIL"
		if passed {
			label = "PASS"
		}
		fmt.Printf("verify: %s\n", label)
		printVerifySummary(verify)
		code = 0
		return nil
	})
	if err != nil {
		return fail(err)
	}
	return code
}

func crossCheck(report Report, trailers map[string]string, task Task, checks ChecksFile, headSHA, baseSHA string) []string {
	var errors []string
	expectedChecks := map[string]bool{}
	for _, id := range checkIDs(checks) {
		expectedChecks[id] = true
	}
	if trailers["Task"] != task.ID {
		errors = append(errors, fmt.Sprintf("trailer Task=%q != %q", trailers["Task"], task.ID))
	}
	if trailers["Attempt"] != itoa(task.Attempt) {
		errors = append(errors, fmt.Sprintf("trailer Attempt=%q != %q", trailers["Attempt"], itoa(task.Attempt)))
	}
	if trailers["Base"] != baseSHA {
		errors = append(errors, fmt.Sprintf("trailer Base=%q != %q", trailers["Base"], baseSHA))
	}
	if trailers["Report"] != "report.json" {
		errors = append(errors, fmt.Sprintf("trailer Report=%q != 'report.json'", trailers["Report"]))
	}
	if trailers["Status"] != report.Status {
		errors = append(errors, fmt.Sprintf("trailer Status=%q != report.status=%q", trailers["Status"], report.Status))
	}
	trailerChecks := map[string]bool{}
	for _, part := range strings.Split(trailers["Checks"], ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			trailerChecks[part] = true
		}
	}
	var missing []string
	for id := range expectedChecks {
		if !trailerChecks[id] {
			missing = append(missing, id)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		var present []string
		for id := range trailerChecks {
			present = append(present, id)
		}
		sort.Strings(present)
		errors = append(errors, fmt.Sprintf("trailer Checks=%v missing %v", present, missing))
	}
	if report.BaseSHA != baseSHA {
		errors = append(errors, fmt.Sprintf("report.base_sha=%q != %q", report.BaseSHA, baseSHA))
	}
	if report.HeadSHA != headSHA {
		errors = append(errors, fmt.Sprintf("report.head_sha=%q != HEAD=%q", report.HeadSHA, headSHA))
	}
	return errors
}

func printVerifySummary(verify VerifyResult) {
	for _, check := range verify.Checks {
		mark := "FAIL"
		if check.Passed {
			mark = "PASS"
		}
		fmt.Printf("  %s: %s exit=%d %gs (log %s)\n", check.ID, mark, check.Exit, check.DurationS, check.Log)
		if !check.Passed && check.Tail != "" {
			for _, line := range strings.Split(lastLines(check.Tail, 10), "\n") {
				fmt.Printf("    %s\n", line)
			}
		}
	}
	for _, warning := range verify.Warnings {
		fmt.Printf("  warning: %s\n", warning)
	}
	for _, err := range verify.EvidenceErrors {
		fmt.Printf("  evidence: %s\n", err)
	}
}

func lastLines(text string, n int) string {
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	if n > 0 && len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

func round2(v float64) float64 {
	return float64(int(v*100+0.5)) / 100
}

func relativeTo(path, base string) string {
	rel, err := filepath.Rel(base, path)
	if err != nil {
		return path
	}
	return rel
}

func sameStringSet(a, b []string) bool {
	as := append([]string{}, a...)
	bs := append([]string{}, b...)
	sort.Strings(as)
	sort.Strings(bs)
	if len(as) != len(bs) {
		return false
	}
	for i := range as {
		if as[i] != bs[i] {
			return false
		}
	}
	return true
}

func jsonMarshalCompact(v any) (string, error) {
	data, err := json.Marshal(v)
	return string(data), err
}
