package app

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var taskIDRe = regexp.MustCompile(`^T-\d{8}-\d{2,3}$`)

func appendEvent(key, taskID, event string, data map[string]any) error {
	path := ledgerPath(key)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if data == nil {
		data = map[string]any{}
	}
	record := Event{TS: nowISO(), Task: taskID, Event: event, Data: data}
	line, err := json.Marshal(record)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(append(line, '\n')); err != nil {
		return err
	}
	return f.Sync()
}

func readEvents(key, taskID string) ([]Event, error) {
	f, err := os.Open(ledgerPath(key))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var events []Event
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var item Event
		if err := json.Unmarshal([]byte(line), &item); err != nil {
			continue
		}
		if taskID == "" || item.Task == taskID {
			events = append(events, item)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return events, nil
}

// foldTask replays the append-only ledger into the current task snapshot.
func foldTask(key, taskID string) (Task, error) {
	task := Task{ID: taskID, State: stateUnknown}
	events, err := readEvents(key, taskID)
	if err != nil {
		return task, err
	}
	for _, item := range events {
		data := item.Data
		if data == nil {
			data = map[string]any{}
		}
		task.LastEvent = item.Event
		task.UpdatedAt = item.TS
		switch item.Event {
		case "created":
			task.State = StateDraft
			task.CreatedAt = item.TS
			task.Goal = asString(data["goal"])
			task.Scope = asStringSlice(data["scope"])
			task.BriefPath = asString(data["brief_path"])
			task.ChecksPath = asString(data["checks_path"])
			task.ChecksSHA256 = asString(data["checks_sha256"])
			task.RepoRoot = asString(data["repo_root"])
		case "contract_approved":
			task.ContractApproved = true
			task.ContractNote = asString(data["note"])
		case "dispatched":
			task.State = StateRunning
			task.Worktree = asString(data["worktree"])
			task.PaneID = asString(data["pane_id"])
			task.TabID = asString(data["tab_id"])
			task.Agent = asString(data["agent"])
			task.Attempt = asInt(data["attempt"])
			if task.Attempt == 0 {
				task.Attempt = 1
			}
			task.BaseSHA = asString(data["base_sha"])
			task.Branch = asString(data["branch"])
			task.WorkspaceID = asString(data["workspace_id"])
		case "prompt_sent":
			task.State = StateRunning
			if seq, ok := data["completion_seq"]; ok && seq != nil {
				n := asInt(seq)
				task.PromptSeq = &n
			}
		case "blocked":
			task.State = StateBlocked
			task.BlockedReason = asString(data["reason"])
		case "blocked_resolved":
			task.State = StateRunning
		case "reported":
			task.State = StateReported
			task.ReportPath = asString(data["report_path"])
			task.HeadSHA = asString(data["head_sha"])
		case "uncertain":
			task.State = StateUncertain
			task.FailureReason = asString(data["reason"])
		case "lost":
			task.State = StateLost
		case "verified":
			task.State = StateVerified
			task.VerifyPath = asString(data["verify_path"])
			task.HeadSHA = asString(data["head_sha"])
		case "verify_failed":
			task.State = StateFailed
			task.VerifyPath = asString(data["verify_path"])
			task.HeadSHA = asString(data["head_sha"])
			task.FailureReason = asString(data["reason"])
		case "budget_exceeded":
			task.State = StateBudgetExceeded
			task.BudgetReason = asString(data["reason"])
		case "accepted":
			task.State = StateAccepted
			task.AcceptedBy = asString(data["by"])
			task.AcceptedAt = item.TS
			task.AcceptNote = asString(data["note"])
		case "rejected":
			task.State = StateRejected
			task.RejectedReason = asString(data["reason"])
		case "retry_requested":
			task.State = StateDraft
			if n := asInt(data["attempt"]); n > 0 {
				task.Attempt = n
			}
			task.RetryReason = asString(data["reason"])
		case "dispatch_failed":
			task.State = StateFailed
			task.FailureReason = asString(data["reason"])
			if v := asString(data["tab_id"]); v != "" {
				task.TabID = v
			}
			if v := asString(data["pane_id"]); v != "" {
				task.PaneID = v
			}
		case "recorded":
			task.RecordPath = asString(data["path"])
			task.MergeCommit = asString(data["merge_commit"])
		case "closed":
			task.State = StateClosed
		}
	}
	return task, nil
}

func listTaskIDs(key string) ([]string, error) {
	dir := filepath.Join(repoDir(key), "tasks")
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, e := range entries {
		if e.IsDir() && taskIDRe.MatchString(e.Name()) {
			ids = append(ids, e.Name())
		}
	}
	sort.Strings(ids)
	return ids, nil
}

func nextTaskID(key string) (string, error) {
	existing := map[string]bool{}
	ids, err := listTaskIDs(key)
	if err != nil {
		return "", err
	}
	for _, id := range ids {
		existing[id] = true
	}
	// A gc'd task keeps its ledger events. Never reuse its id, or folding the
	// new task would replay the old history.
	events, err := readEvents(key, "")
	if err != nil {
		return "", err
	}
	for _, ev := range events {
		if ev.Task != "" {
			existing[ev.Task] = true
		}
	}
	today := todayStamp()
	for i := 1; ; i++ {
		candidate := fmt.Sprintf("T-%s-%02d", today, i)
		if !existing[candidate] {
			return candidate, nil
		}
	}
}
